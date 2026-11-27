#!/bin/sh
# End-to-end environment: a simulated Linux host (systemd + Docker) with a local git platform (SSH, push), an
# OCI registry with demo image tags, a GitHub releases fake, a webhook receiver and a mail server (STARTTLS on port 587).
#
#   e2e/run.sh deps               pull and build the base images and load the Go modules of the old release;
#                                 the only command that needs the internet
#   e2e/run.sh up                 build everything and start the environment, without the internet
#   e2e/run.sh down               remove every container, network, volume, image and temp directory it made
#   e2e/run.sh status             show the environment and the registry tags
#   e2e/run.sh exec [-u user] <command...>   run a command on the simulated host (default user e2e-admin)
#   e2e/run.sh env                print the path of the file with the generated test values (source it)
#   e2e/run.sh requests           print the requests the webhook receiver recorded (JSON lines)
#   e2e/run.sh messages           print the e-mails the mail server recorded (JSON lines)
#   e2e/run.sh scenario [section...]   run the scenario once after up; with names only those sections and the
#                                 sections they need (register and names in e2e/sections.sh)
#   e2e/run.sh acceptance         run the acceptance scenario once after up, on a fresh host (see e2e/acceptance.sh)
#   e2e/run.sh check <scenario [section...]|acceptance>   up, run it, down, also when interrupted; an unknown
#                                 section stops it before up
#
# On the host, e2e-admin reaches the stacks repo with: git clone git.qhoste2e.test:stacks.git
# The test user e2e-rootless runs a rootless Docker engine of its own, without a login session, on the socket
# /run/user/<uid>/docker.sock; it uses the registry mirror like the host's Docker daemon.
# The registry is registry.qhoste2e.test (image demo/app, tags 1.0.0 1.0.1 1.1.0 2.0.0 2.1.0-rc.1 3.0.0 4.0.0
# 5.0.0); a demo container turns unhealthy with DEMO_HEALTH=fail, and the image 4.0.0 is always unhealthy.
# For the acceptance scenario only, it also holds two versions each of the real images of the stacks n8n and
# bookstack (n8nio/n8n, linuxserver/bookstack, library/mariadb), copied from the internet by skopeo when
# acceptance starts; up and scenario need no internet. The newer test tags
# n8nio/n8n:2.48.0 and linuxserver/bookstack:26.10.0, derived images whose healthcheck always fails, are pushed
# only by the acceptance scenario, after its successful update, so the update ceiling cannot reach them earlier.
# A second registry, private.qhoste2e.test, asks for a login (htpasswd with bcrypt, made for the run; the user is
# demo-user, the token is random and stays in the file named by E2E_PRIVATE_LOGIN_FILE). It holds demo/app with
# the tags 1.0.0 and 1.1.0, and the simulated host trusts it like the other registry but has no stored login.
# The registry is also the Docker Hub mirror of the host's Docker daemon; for acceptance it holds busybox by the
# digest of the default backup helper image, so qhost runs a backup there without --backup-image.
#
# Up to four instances run side by side. up takes a free one (1-4), or waits until one is free, and holds it for
# the checkout it runs from until down; every other command acts on the instance of that checkout. E2E_INSTANCE=<n>
# picks an instance explicitly, for example to remove the leftovers of a checkout that is gone.
# Instance n names everything qhoste2e<n>-* and labels it qhost.e2e=<n>; down touches only what carries both,
# so it never reaches another instance. The host names *.qhoste2e.test are aliases inside the instance's own
# internal network, which has no way out; only skopeo reaches the internet, for acceptance. Nothing is published
# on the machine's ports. down keeps the base images of deps. TEST_CPU (systemd format, 100% is one core) and
# TEST_MEM, when set, limit the simulated host container. Set E2E_KEEP_ON_FAIL=1 to keep a failed environment for
# inspection.
# Needs docker (rootless is fine), go, openssl, ssh-keygen.
set -eu

base=qhoste2e
instances='1 2 3 4'
domain=qhoste2e.test
hostname_=qhost-e2e-host
admin=e2e-admin
# Fixed in base.Dockerfile.
rootless_user=e2e-rootless
rootless_uid=1001
# The published release whose real self-update the section old-release proves; it is built from its tag.
old_release=v0.6.0
tags='1.0.0 1.0.1 1.1.0 2.0.0 2.1.0-rc.1 3.0.0 4.0.0 5.0.0'
unhealthy_tags='4.0.0'
# The registry with a login: private.$domain, image demo/app, these tags; the user is fixed and the token is made per run.
private_user=demo-user
private_tags='1.0.0 1.1.0'
# Tags of the repository demo/late: 1.0.0 is a healthy demo, 1.1.0 fails 100 seconds after its start.
# Real images copied into the registry as "<source>|<path in the registry>:<tag>", and test images derived from
# one of them as "<registry path>:<base tag>|<new tag>".
real_images='n8nio/n8n:2.46.4|n8nio/n8n:2.46.4
n8nio/n8n:2.47.0|n8nio/n8n:2.47.0
lscr.io/linuxserver/bookstack:26.05.5|linuxserver/bookstack:26.05.5
lscr.io/linuxserver/bookstack:26.09.1|linuxserver/bookstack:26.09.1
mariadb:11.8.8|library/mariadb:11.8.8
mariadb:11.8.9|library/mariadb:11.8.9'
broken_images='n8nio/n8n:2.47.0|2.48.0
linuxserver/bookstack:26.09.1|26.10.0'

here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/.." && pwd)
tmp_base=${TMPDIR:-/tmp}
# Base images of deps, tagged by the content of their Dockerfile, so a change there needs deps again.
base_tag=$(sha256sum "$here/base.Dockerfile" | cut -c 1-12)
base_from='debian:12 debian:12-slim registry:2'
# Fixed, not under TMPDIR: every caller of the same Docker daemon must see the same claims.
claims=/tmp/$base-instances

fail() {
	printf 'e2e: %s\n' "$*" >&2
	exit 1
}

log() {
	printf 'e2e: %s\n' "$*"
}

# use_instance sets the names of instance $1.
use_instance() {
	case $1 in 1 | 2 | 3 | 4) ;; *) fail "unknown instance $1; use 1 to 4" ;; esac
	instance=$1
	prefix=$base$instance
	label=qhost.e2e=$instance
	net=$prefix-net
	host=$prefix-host
	backup_image=$prefix-backup-helper:1
}

# held prints the instance this checkout holds, if any.
held() {
	for n in $instances; do
		if [ "$(cat "$claims/$n/owner" 2>/dev/null)" = "$root" ]; then
			printf '%s\n' "$n"
			return 0
		fi
	done
}

# pick uses E2E_INSTANCE, else the instance this checkout holds; it fails when there is none.
pick() {
	n=${E2E_INSTANCE:-$(held)}
	[ -n "$n" ] || return 1
	use_instance "$n"
}

# claim takes a free instance for this checkout, the one E2E_INSTANCE names if set, and waits until one is free.
claim() {
	mkdir -p "$claims"
	[ -O "$claims" ] || fail "$claims belongs to another user"
	waiting=
	while :; do
		for n in ${E2E_INSTANCE:-$instances}; do
			# mkdir is atomic: of two callers only one gets the instance.
			if mkdir "$claims/$n" 2>/dev/null; then
				printf '%s\n' "$root" >"$claims/$n/owner"
				use_instance "$n"
				return 0
			fi
		done
		[ -n "$waiting" ] || log "no free instance; waiting (holders: $(cat "$claims"/*/owner 2>/dev/null | tr '\n' ' '))"
		waiting=1
		sleep 5
	done
}

release() {
	rm -f "$claims/$instance/owner"
	rmdir "$claims/$instance" 2>/dev/null || true
}

# owned lists "<id> <name>" of the objects of one kind whose name starts with the prefix and that carry the label.
owned() {
	case $1 in
	container) docker ps -a --filter "label=$label" --format '{{.ID}} {{.Names}}' ;;
	network) docker network ls --filter "label=$label" --format '{{.ID}} {{.Name}}' ;;
	volume) docker volume ls --filter "label=$label" --format '{{.Name}} {{.Name}}' ;;
	image) docker images --filter "label=$label" --format '{{.ID}} {{.Repository}}:{{.Tag}}' ;;
	esac | awk -v p="$prefix-" 'index($2, p) == 1'
}

# named lists the objects of one kind by prefix alone, to find leftovers that lost their label.
named() {
	case $1 in
	container) docker ps -a --format '{{.Names}}' ;;
	network) docker network ls --format '{{.Name}}' ;;
	volume) docker volume ls --format '{{.Name}}' ;;
	image) docker images --format '{{.Repository}}:{{.Tag}}' ;;
	esac | awk -v p="$prefix" 'index($0, p) == 1'
}

workdirs() {
	for dir in "$tmp_base"/$prefix.*; do
		[ -d "$dir" ] && [ -O "$dir" ] && [ -f "$dir/.$prefix" ] && printf '%s\n' "$dir"
	done
	return 0
}

current_work() {
	workdirs | head -n 1
}

require_up() {
	work=$(current_work)
	[ -n "$work" ] || fail 'the environment is not up; run: e2e/run.sh up'
}

down() {
	log "instance $instance: removing containers, networks, volumes, images and temp directories"
	owned container | while read -r id name; do
		docker rm -f -v "$id" >/dev/null && log "removed container $name"
	done
	owned network | while read -r id name; do
		docker network rm "$id" >/dev/null && log "removed network $name"
	done
	owned volume | while read -r id name; do
		docker volume rm "$id" >/dev/null && log "removed volume $name"
	done
	# Removed by tag, not by ID: only our tag goes, never an image that happens to share content.
	owned image | while read -r _ name; do
		docker rmi "$name" >/dev/null && log "removed image $name"
	done
	workdirs | while read -r dir; do
		rm -rf "$dir" && log "removed $dir"
	done

	left=
	for kind in container network volume image; do
		found=$(named "$kind")
		if [ -n "$found" ]; then
			left="$left $kind:$(printf '%s' "$found" | tr '\n' ',')"
		fi
	done
	[ -z "$(workdirs)" ] || left="$left tmp-dir"
	[ -z "$left" ] || fail "leftovers with the prefix $prefix:$left"
	release
	log "clean: nothing with the prefix $prefix remains; instance $instance is free"
}

base_image() {
	printf '%s-base-%s:%s\n' "$base" "$1" "$base_tag"
}

deps() {
	log 'pulling and building the base images (needs the internet)'
	for image in $base_from; do
		docker pull -q "$image" >/dev/null || fail "pulling $image failed"
	done
	for target in host git skopeo; do
		docker build -q --target "$target" -t "$(base_image "$target")" - <"$here/base.Dockerfile" >/dev/null ||
			fail "building the base image $target failed"
	done
	log "base images ready: $(for t in host git skopeo; do base_image "$t"; done | tr '\n' ' ')"
	old_modules
}

# old_modules loads the Go modules of $old_release into the module cache, which its offline build needs.
old_modules() {
	mods=$(mktemp -d "$tmp_base/$base-modules.XXXXXX")
	git -C "$root" archive "$old_release" go.mod go.sum | tar -x -C "$mods" &&
		(cd "$mods" && GOTOOLCHAIN=local go mod download) || {
		rm -rf "$mods"
		fail "loading the Go modules of $old_release failed"
	}
	rm -rf "$mods"
	log "Go modules of $old_release ready"
}

require_base() {
	for image in $base_from $(base_image host) $(base_image git) $(base_image skopeo); do
		docker image inspect "$image" >/dev/null 2>&1 || fail "the base image $image is missing; run: e2e/run.sh deps"
	done
}

# host_limits prints the docker run options that hold the simulated host to the test budget of the run.
host_limits() {
	if [ -n "${TEST_CPU:-}" ]; then
		printf -- '--cpus %s ' "$(awk -v p="${TEST_CPU%\%}" 'BEGIN { printf "%g", p / 100 }')"
	fi
	if [ -n "${TEST_MEM:-}" ]; then
		printf -- '--memory %s --memory-swap %s ' "$TEST_MEM" "$TEST_MEM"
	fi
}

make_keys_and_certs() {
	mkdir -p "$work/keys" "$work/ctx/certs" "$work/ctx/releases"
	# One key pair for the administrator on the simulated host; the git platform accepts only this public key.
	ssh-keygen -q -t ed25519 -N '' -C "$prefix-test-key" -f "$work/keys/admin"

	ca=$work/ctx/certs
	openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 2 \
		-subj "/CN=$prefix test CA" -keyout "$work/keys/ca.key" -out "$ca/ca.crt" 2>/dev/null
	openssl req -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
		-subj "/CN=$prefix helper services" -keyout "$ca/server.key" -out "$work/keys/server.csr" 2>/dev/null
	cat >"$work/keys/server.ext" <<EXT
subjectAltName = DNS:registry.$domain, DNS:private.$domain, DNS:hooks.$domain, DNS:mail.$domain, DNS:releases.$domain, DNS:api.github.com, DNS:github.com
extendedKeyUsage = serverAuth
EXT
	openssl x509 -req -in "$work/keys/server.csr" -CA "$ca/ca.crt" -CAkey "$work/keys/ca.key" -CAcreateserial \
		-days 2 -extfile "$work/keys/server.ext" -out "$ca/server.crt" 2>/dev/null
	# The login of the private registry: "<user>:<token>", made for this run only.
	(umask 077 && printf '%s:demo-%s\n' "$private_user" "$(openssl rand -hex 12)" >"$work/keys/private-login")
}

write_testenv() {
	cat >"$work/testenv" <<ENV
E2E_WORK=$work
E2E_HOST_CONTAINER=$host
E2E_HOST_ADMIN=$admin
E2E_HOSTNAME=$hostname_
E2E_CA_CERT=$work/ctx/certs/ca.crt
E2E_SSH_KEY=$work/keys/admin
E2E_GIT_URL=git@git.$domain:stacks.git
E2E_REGISTRY=registry.$domain
E2E_IMAGE=registry.$domain/demo/app
E2E_PRIVATE_REGISTRY=private.$domain
E2E_PRIVATE_IMAGE=private.$domain/demo/app
E2E_PRIVATE_TAGS='$private_tags'
E2E_PRIVATE_LOGIN_FILE=$work/keys/private-login
E2E_TAGS='$tags'
E2E_BACKUP_IMAGE=$backup_image
E2E_RELEASES_URL=https://releases.$domain
E2E_GITHUB_API=https://api.github.com
E2E_WEBHOOK_URL=https://hooks.$domain/hook
E2E_CHANGELOG_IMAGE_REPO=qhoste2e-org/demo-image
E2E_CHANGELOG_GIT_REPO=qhoste2e-org/demo-stacks
E2E_N8N_IMAGE=registry.$domain/n8nio/n8n
E2E_BOOKSTACK_IMAGE=registry.$domain/linuxserver/bookstack
E2E_MARIADB_IMAGE=registry.$domain/library/mariadb
ENV
}

build_images() {
	log 'building the fakes'
	(cd "$root" && CGO_ENABLED=0 go build -tags e2efakes -trimpath -o "$work/ctx/fakes" ./e2e/fakes)
	cp -r "$here/demo-stacks" "$here/demo-stacks-second" "$here/demo-stacks-root" "$here/git-sshd.conf" "$here/git-entrypoint.sh" "$work/ctx/"

	# The htpasswd file of the private registry; the login comes from the file, never from an argument.
	login=$(cat "$work/keys/private-login")
	printf '%s' "${login#*:}" | "$work/ctx/fakes" htpasswd "${login%%:*}" >"$work/ctx/htpasswd"

	log 'building the images'
	# --network none: a build that still needs the internet fails here instead of depending on it.
	for name in fakes registry private-registry git skopeo; do
		case $name in git | skopeo) set -- --build-arg BASE="$(base_image "$name")" ;; *) set -- ;; esac
		docker build -q --network none --label "$label" -t "$prefix-$name:latest" "$@" \
			-f "$here/$name.Dockerfile" "$work/ctx" >/dev/null || fail "building the image $name failed"
	done
}

start_services() {
	docker network create --internal --label "$label" "$net" >/dev/null
	docker volume create --label "$label" "$prefix-docker" >/dev/null
	docker volume create --label "$label" "$prefix-containerd" >/dev/null
	docker volume create --label "$label" "$prefix-rootless" >/dev/null
	docker volume create --label "$label" "$prefix-registry" >/dev/null
	docker volume create --label "$label" "$prefix-private-registry" >/dev/null
	docker volume create --label "$label" "$prefix-git" >/dev/null

	docker run -d --name "$prefix-registry" --label "$label" --network "$net" \
		--network-alias "registry.$domain" -v "$prefix-registry:/var/lib/registry" "$prefix-registry:latest" >/dev/null
	docker run -d --name "$prefix-private-registry" --label "$label" --network "$net" \
		--network-alias "private.$domain" -v "$prefix-private-registry:/var/lib/registry" \
		"$prefix-private-registry:latest" >/dev/null
	docker run -d --name "$prefix-git" --label "$label" --network "$net" --network-alias "git.$domain" \
		-e GIT_AUTHORIZED_KEY="$(cat "$work/keys/admin.pub")" -v "$prefix-git:/srv/git" "$prefix-git:latest" >/dev/null
	docker run -d --name "$prefix-hooks" --label "$label" --network "$net" --network-alias "hooks.$domain" \
		"$prefix-fakes:latest" webhook >/dev/null
	docker run -d --name "$prefix-mail" --label "$label" --network "$net" --network-alias "mail.$domain" \
		"$prefix-fakes:latest" mail >/dev/null
	docker run -d --name "$prefix-releases" --label "$label" --network "$net" \
		--network-alias "releases.$domain" --network-alias api.github.com --network-alias github.com \
		"$prefix-fakes:latest" releases >/dev/null

	docker run -d --name "$host" --label "$label" --hostname "$hostname_" --network "$net" \
		--privileged --cgroupns=private --tmpfs /run --tmpfs /run/lock $(host_limits) \
		-v "$prefix-docker:/var/lib/docker" -v "$prefix-containerd:/var/lib/containerd" \
		-v "$prefix-rootless:/home/$rootless_user/.local/share/docker" \
		"$(base_image host)" >/dev/null
}

# wait_for polls every quarter second. The limit $2 counts seconds of pause between tries, so a slow probe stretches
# it as it did with one try per second and no wait times out earlier than before.
wait_for() {
	what=$1 tries=$(($2 * 4))
	shift 2
	i=0
	until "$@" >/dev/null 2>&1; do
		i=$((i + 1))
		[ "$i" -lt "$tries" ] || fail "timeout waiting for $what"
		sleep 0.25
	done
}

host_booted() {
	state=$(docker exec "$host" systemctl is-system-running 2>/dev/null || true)
	[ "$state" = running ] || [ "$state" = degraded ]
}

git_host_key() {
	docker exec "$prefix-git" cat /etc/ssh/ssh_host_ed25519_key.pub
}

prepare_host() {
	wait_for 'the host to boot' 90 host_booted
	docker cp "$work/ctx/certs/ca.crt" "$host:/usr/local/share/ca-certificates/$prefix-ca.crt"
	docker exec "$host" update-ca-certificates >/dev/null
	# The inner Docker daemon reads per-registry CAs without a restart.
	docker exec "$host" mkdir -p "/etc/docker/certs.d/registry.$domain"
	docker cp "$work/ctx/certs/ca.crt" "$host:/etc/docker/certs.d/registry.$domain/ca.crt"
	docker exec "$host" mkdir -p "/etc/docker/certs.d/private.$domain"
	docker cp "$work/ctx/certs/ca.crt" "$host:/etc/docker/certs.d/private.$domain/ca.crt"
	wait_for 'the inner Docker daemon' 60 docker exec "$host" docker info

	wait_for 'the git host key' 30 git_host_key
	ssh_dir=$work/ssh-admin
	mkdir -p "$ssh_dir"
	cp "$work/keys/admin" "$ssh_dir/id_ed25519"
	printf 'Host git.%s\n\tUser git\n\tIdentityFile /home/%s/.ssh/id_ed25519\n' "$domain" "$admin" >"$ssh_dir/config"
	printf 'git.%s %s\n' "$domain" "$(git_host_key | cut -d ' ' -f 1,2)" >"$ssh_dir/known_hosts"
	docker cp "$ssh_dir" "$host:/home/$admin/.ssh"
	docker exec "$host" sh -c "chown -R $admin:$admin /home/$admin/.ssh && chmod 700 /home/$admin/.ssh && chmod 600 /home/$admin/.ssh/id_ed25519"
	rm -rf "$ssh_dir"
}

# prepare_rootless starts the rootless Docker engine of the test user with the user unit and socket of
# dockerd-rootless-setuptool.sh; lingering keeps it running without a login session. It trusts the test CA and uses
# the registry mirror like the system engine.
prepare_rootless() {
	log "starting the rootless Docker engine of $rootless_user"
	docker exec "$host" loginctl enable-linger "$rootless_user"
	wait_for "the user manager of $rootless_user" 30 docker exec "$host" systemctl is-active "user@$rootless_uid.service"
	# docker exec leaves the umask at 0, which would make the unit and the daemon.json writable for everyone.
	docker exec -i -u "$rootless_user" -e XDG_RUNTIME_DIR="/run/user/$rootless_uid" "$host" sh -ec "
		umask 022
		mkdir -p ~/.config/docker/certs.d/registry.$domain
		cat >~/.config/docker/certs.d/registry.$domain/ca.crt
		cp /etc/docker/daemon.json ~/.config/docker/daemon.json
		dockerd-rootless-setuptool.sh install" <"$work/ctx/certs/ca.crt" >"$work/rootless-setup.log" 2>&1 ||
		fail "setting up the rootless Docker engine failed: $(tail -n 20 "$work/rootless-setup.log")"
	wait_for 'the rootless Docker engine' 60 \
		docker exec -u "$rootless_user" "$host" docker -H "unix:///run/user/$rootless_uid/docker.sock" info
}

# push_images builds the demo image once per tag inside the host and pushes it to the registry.
push_images() {
	log "pushing the demo image tags: $tags"
	docker exec "$host" mkdir -p /tmp/demo-image
	docker cp "$work/ctx/fakes" "$host:/tmp/demo-image/fakes"
	docker cp "$here/demo-image.Dockerfile" "$host:/tmp/demo-image/Dockerfile"
	for tag in $tags; do
		target=healthy
		case " $unhealthy_tags " in *" $tag "*) target=unhealthy ;; esac
		docker exec -w /tmp/demo-image "$host" sh -ec "
			docker build -q --target $target --build-arg VERSION=$tag -t registry.$domain/demo/app:$tag . >/dev/null
			docker push -q registry.$domain/demo/app:$tag >/dev/null
			docker rmi registry.$domain/demo/app:$tag >/dev/null"
	done
	for tag in 1.0.0 1.1.0; do
		target=healthy
		[ "$tag" != 1.1.0 ] || target=late
		docker exec -w /tmp/demo-image "$host" sh -ec "
			docker build -q --target $target --build-arg VERSION=$tag -t registry.$domain/demo/late:$tag . >/dev/null
			docker push -q registry.$domain/demo/late:$tag >/dev/null
			docker rmi registry.$domain/demo/late:$tag >/dev/null"
	done
	docker exec "$host" rm -rf /tmp/demo-image
}

# push_private_images builds the demo image once per tag inside the host and pushes it to the private registry.
# The login lives in a throw-away Docker configuration folder that goes with the run; the host's own Docker
# configuration never holds it, so a pull of these images fails until qhost has the login stored.
push_private_images() {
	log "pushing the demo image tags to the registry with a login: $private_tags"
	login=$(cat "$work/keys/private-login")
	docker exec "$host" mkdir -p /tmp/private-image
	docker cp "$work/ctx/fakes" "$host:/tmp/private-image/fakes"
	docker cp "$here/demo-image.Dockerfile" "$host:/tmp/private-image/Dockerfile"
	printf '%s' "${login#*:}" | docker exec -i -w /tmp/private-image "$host" sh -ec "
		export DOCKER_CONFIG=/tmp/private-docker
		docker login -u '${login%%:*}' --password-stdin private.$domain >/dev/null
		for tag in $private_tags; do
			docker build -q --target healthy --build-arg VERSION=\$tag -t private.$domain/demo/app:\$tag . >/dev/null
			docker push -q private.$domain/demo/app:\$tag >/dev/null
			docker rmi private.$domain/demo/app:\$tag >/dev/null
		done
		docker logout private.$domain >/dev/null
		cd /
		rm -rf /tmp/private-image /tmp/private-docker" || fail 'pushing the images to the private registry failed'
}

# backup_helper_ref prints the default helper image of backups and the namespace check, name and digest.
backup_helper_ref() {
	ref=$(sed -n 's/^\tHelperImage = "\(busybox@sha256:[0-9a-f]*\)"$/\1/p' "$root/internal/compose/datadirs.go")
	[ -n "$ref" ] || fail 'the default helper image is not found in internal/compose/datadirs.go'
	printf '%s\n' "$ref"
}

# copy_image copies one image between registries with skopeo in a short-lived container on the simulated
# network; further arguments go to skopeo copy.
copy_image() {
	from=$1 to=$2
	shift 2
	docker create --rm --label "$label" --name "$prefix-skopeo" --network "$net" "$prefix-skopeo:latest" \
		skopeo copy "$@" "docker://$from" "docker://registry.$domain/$to" >/dev/null
	# The test network is internal; only skopeo gets a way out, to reach the source registry.
	docker network connect bridge "$prefix-skopeo"
	docker start -a "$prefix-skopeo" >/dev/null || fail "copying $from failed"
}

# push_real_images fills the registry with the real images of the stacks n8n and bookstack and with the
# busybox of the default backup helper, which Docker Hub names resolve to through the host's registry mirror.
push_real_images() {
	log 'copying the real images of n8n, bookstack and mariadb into the registry (needs the internet)'
	printf '%s\n' "$real_images" | while IFS='|' read -r from to; do
		log "copying $from"
		copy_image "$from" "$to"
	done
	ref=$(backup_helper_ref)
	log "copying the backup helper $ref"
	# --all keeps the multi-platform index, so the digest stays the one the reference names.
	copy_image "$ref" library/busybox:1.37.0 --all --preserve-digests
}

# push_broken_images derives the test images whose healthcheck fails from a real image in the registry.
push_broken_images() {
	log 'building the test images with a failing healthcheck'
	docker exec "$host" mkdir -p /tmp/broken-image
	docker cp "$here/broken-image.Dockerfile" "$host:/tmp/broken-image/Dockerfile"
	printf '%s\n' "$broken_images" | while IFS='|' read -r base tag; do
		repo=${base%:*}
		docker exec -w /tmp/broken-image "$host" sh -ec "
			docker build -q --build-arg BASE=registry.$domain/$base -t registry.$domain/$repo:$tag . >/dev/null
			docker push -q registry.$domain/$repo:$tag >/dev/null
			docker rmi registry.$domain/$repo:$tag >/dev/null
			# The base image may be in use by a running stack and then stays.
			docker rmi registry.$domain/$base >/dev/null 2>&1 || true"
	done
	docker exec "$host" rm -rf /tmp/broken-image
}

# registry_entries lists "<path>:<tag>" of every real image push_real_images copies.
registry_entries() {
	printf '%s\n' "$real_images" | while IFS='|' read -r _ to; do
		printf '%s\n' "$to"
	done
}

# load_backup_helper gives the host the image of the backup helper containers. The host has no internet, so the
# Debian base of the host image (GNU tar, find, rm, sh) is copied in from the machine's Docker.
load_backup_helper() {
	log "loading the backup helper image $backup_image"
	docker image inspect debian:12 >/dev/null 2>&1 || fail 'the base image debian:12 is not available'
	docker save debian:12 | docker exec -i "$host" docker load -q >/dev/null
	docker exec "$host" sh -ec "docker tag debian:12 $backup_image && docker rmi debian:12 >/dev/null"
}

# http_code prints the status code the host gets for a URL.
http_code() {
	docker exec "$host" curl -sS -o /dev/null -w '%{http_code}' "$@"
}

check_services() {
	log 'checking the helper services from the host'
	api=$(docker exec "$host" curl -fsS https://api.github.com/repos/castrowithcee/qatlas-host/releases) ||
		fail 'the releases fake is not reachable over trusted HTTPS'
	[ -n "$api" ] || fail 'the releases fake answered nothing'
	got=$(docker exec "$host" curl -fsS "https://registry.$domain/v2/demo/app/tags/list") ||
		fail 'the registry is not reachable over trusted HTTPS'
	for tag in $tags; do
		printf '%s' "$got" | grep -q "\"$tag\"" || fail "the registry does not list the tag $tag"
	done
	[ "$(http_code "https://private.$domain/v2/")" = 401 ] ||
		fail 'the private registry does not ask for a login'
	got=$(docker exec "$host" curl -fsS -u "$(cat "$work/keys/private-login")" "https://private.$domain/v2/demo/app/tags/list") ||
		fail 'the private registry does not accept its login over trusted HTTPS'
	for tag in $private_tags; do
		printf '%s' "$got" | grep -q "\"$tag\"" || fail "the private registry does not list the tag $tag"
	done
	[ "$(http_code -X POST -d '{"check":true}' "https://hooks.$domain/hook")" = 204 ] ||
		fail 'the webhook receiver does not accept a request'
	docker exec "$host" curl -fsS "https://hooks.$domain/_e2e/requests" | grep -q '"check\\":true' ||
		fail 'the webhook receiver did not record the request'
	docker exec -u "$admin" "$host" ssh -o BatchMode=yes "git.$domain" 2>&1 | grep -qi 'interactive git shell is not enabled' ||
		fail 'the git server does not answer over SSH'
}

# prepare_real_images copies the real images into the registry and checks that the host reaches them.
prepare_real_images() {
	push_real_images
	for entry in $(registry_entries); do
		got=$(docker exec "$host" curl -fsS "https://registry.$domain/v2/${entry%:*}/tags/list") ||
			fail "the registry does not list ${entry%:*}"
		printf '%s' "$got" | grep -q "\"${entry#*:}\"" || fail "the registry does not list the tag $entry"
	done
	# The Docker Hub name of the backup helper resolves through the mirror, by digest, without the internet.
	ref=$(backup_helper_ref)
	docker exec "$host" docker pull -q "$ref" >/dev/null || fail "the host cannot pull $ref through the registry mirror"
	docker exec "$host" docker rmi "$ref" >/dev/null
}

up() {
	for tool in docker go jq openssl ssh-keygen; do
		command -v "$tool" >/dev/null 2>&1 || fail "$tool is required"
	done
	require_base
	n=$(held)
	if [ -n "$n" ]; then use_instance "$n"; else claim; fi
	log "instance $instance for $root"
	[ -z "$(named container)$(named network)$(named volume)$(named image)$(workdirs)" ] ||
		fail "something with the prefix $prefix exists already; run: e2e/run.sh down"

	work=$(mktemp -d "$tmp_base/$prefix.XXXXXX")
	: >"$work/.$prefix"
	done_=
	trap 'if [ -z "$done_" ]; then
		if [ -n "${E2E_KEEP_ON_FAIL:-}" ]; then printf "e2e: up failed; environment kept, remove it with: e2e/run.sh down\n" >&2
		else printf "e2e: up failed; tearing down\n" >&2; down || true; fi
	fi' EXIT
	trap 'exit 1' HUP INT TERM

	make_keys_and_certs
	write_testenv
	build_images
	start_services
	prepare_host
	prepare_rootless
	push_images
	push_private_images
	load_backup_helper
	check_services
	done_=1
	log "up; test values: $work/testenv"
}

status() {
	require_up
	owned container | while read -r _ name; do
		printf '%s %s\n' "$name" "$(docker inspect -f '{{.State.Status}}' "$name")"
	done
	printf 'registry tags: %s\n' "$(docker exec "$host" curl -fsS "https://registry.$domain/v2/demo/app/tags/list" 2>/dev/null || true)"
	for repo in n8nio/n8n linuxserver/bookstack library/mariadb library/busybox; do
		printf 'registry tags %s: %s\n' "$repo" "$(docker exec "$host" curl -fsS "https://registry.$domain/v2/$repo/tags/list" 2>/dev/null || true)"
	done
	printf 'test values: %s\n' "$work/testenv"
}

# check runs up, the scenario $1 with the sections after it and down in one go; down also runs when the run is
# interrupted or times out.
check() {
	kind=${1:-}
	[ $# -eq 0 ] || shift
	case $kind in
	scenario)
		. "$here/sections.sh"
		scenario_select "$@"
		;;
	acceptance) [ $# -eq 0 ] || fail 'check acceptance takes no sections' ;;
	*) fail 'check needs scenario or acceptance' ;;
	esac
	"$0" up || exit 1
	trap '"$0" down; exit 143' HUP INT TERM
	rc=0
	"$0" "$kind" "$@" || rc=$?
	trap - HUP INT TERM
	"$0" down || [ "$rc" -ne 0 ] || rc=1
	exit "$rc"
}

# source_lib loads the shared helpers of the scenarios from e2e/lib/.
source_lib() {
	for lib in core host stack source tui; do
		. "$here/lib/$lib.sh"
	done
}

main() {
	cmd=${1:-}
	[ $# -eq 0 ] || shift
	case $cmd in
	status | env | requests | messages | scenario | acceptance | exec)
		pick || fail 'this checkout holds no instance; run: e2e/run.sh up'
		;;
	esac
	case $cmd in
	deps) deps ;;
	check) check "$@" ;;
	up) up ;;
	down)
		if pick; then down; else log "this checkout holds no instance; nothing to remove"; fi
		;;
	status) status ;;
	env)
		require_up
		printf '%s\n' "$work/testenv"
		;;
	requests)
		require_up
		docker exec "$prefix-hooks" cat /data/requests.jsonl 2>/dev/null || true
		;;
	messages)
		require_up
		docker exec "$prefix-mail" cat /data/messages.jsonl 2>/dev/null || true
		;;
	scenario)
		require_up
		source_lib
		. "$here/sections.sh"
		for section in "$here"/sections/*.sh; do . "$section"; done
		scenario_t0=$(date +%s)
		run_scenario "$@"
		;;
	acceptance)
		require_up
		source_lib
		. "$here/acceptance.sh"
		prepare_real_images
		scenario_t0=$(date +%s)
		run_acceptance
		;;
	exec)
		require_up
		user=$admin
		if [ "${1:-}" = -u ]; then
			[ $# -ge 2 ] || fail 'exec -u needs a user'
			user=$2
			shift 2
		fi
		[ $# -gt 0 ] || fail 'exec needs a command'
		if [ -t 0 ]; then
			exec docker exec -it -u "$user" "$host" "$@"
		fi
		exec docker exec -i -u "$user" "$host" "$@"
		;;
	*)
		sed -n '2,45p' "$0" | sed 's/^# \{0,1\}//'
		[ -z "$cmd" ] || exit 2
		;;
	esac
}

main "$@"
