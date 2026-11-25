#!/bin/sh
# Helpers of the e2e scenario for the host, the installed services and the signed test release.
# Sourced by run.sh; defines only functions and constants.

# signature_asset is the name of the release file with the signature; it must match release.SignatureAsset.
signature_asset=checksums.txt.sig
# instance_service names the deploy service instances, and release_template is their template unit in packaging/.
instance_service=qhost-deploy
release_template=$instance_service@.service

known_hosts_sum() {
	docker exec "$host" sh -c 'sha256sum /var/lib/qhost-deploy/known_hosts 2>/dev/null || echo none'
}

git_ed25519_fingerprint() {
	docker exec "$prefix-git" ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub | cut -d ' ' -f 2
}

# release_build builds the fake release $1 (a tag) for the architecture of the host into $work/releases/$1 and
# signs its checksums with the test key. The binary trusts that key through a source overlay of the key list, so
# release.json and the sources stay as they are. With changed-template as $2 the binary also carries a template unit
# of the deploy service instances with one more comment line, $work/releasebuild/$release_template, the same way.
release_build() {
	tag=$1
	arch=$(docker exec "$host" dpkg --print-architecture)
	dir=$work/releasebuild
	overlay=$dir/overlay.json
	if [ ! -f "$overlay" ]; then
		mkdir -p "$dir"
		ssh-keygen -q -t ed25519 -N '' -C "$prefix-release-test-key" -f "$work/keys/release"
		printf 'qhost-release %s\n' "$(cut -d ' ' -f 1,2 "$work/keys/release.pub")" >"$dir/allowed-signers"
		sed "s|^var extraKeyLines string\$|var extraKeyLines = \"$(cut -d ' ' -f 1,2 "$work/keys/release.pub")\"|" \
			"$root/internal/release/release.go" >"$dir/release.go"
		grep -q '^var extraKeyLines = "ssh-ed25519 ' "$dir/release.go" || fail 'cannot add the test key to the release key list'
		printf '{"Replace":{"%s":"%s"}}' "$root/internal/release/release.go" "$dir/release.go" >"$dir/overlay.json"
		sed "s|\"targets\": .*|\"targets\": [\"linux/$arch\"],|" "$root/release.json" >"$dir/release.json"
	fi
	if [ "${2:-}" = changed-template ]; then
		overlay=$dir/overlay-template.json
		{ cat "$root/packaging/$release_template" && printf '# Changed by the self-update test of the template.\n'; } >"$dir/$release_template"
		printf '{"Replace":{"%s":"%s","%s":"%s"}}' "$root/internal/release/release.go" "$dir/release.go" \
			"$root/packaging/$release_template" "$dir/$release_template" >"$overlay"
	fi
	rel_signers=$(realpath --relative-to="$root" "$dir/allowed-signers")
	(cd "$root" && SOURCE_DATE_EPOCH=1700000000 GOFLAGS="${GOFLAGS:+$GOFLAGS }-overlay=$overlay" \
		go run github.com/castrowithcee/qatlas-kit/cmd/releasebuild -config "$dir/release.json" -program qhost \
		-namespace qhost-release -signature-asset "$signature_asset" -allowed-signers "$rel_signers" \
		-releases-url https://github.com/castrowithcee/qatlas-host/releases -version "$tag" -out "$work/releases/$tag" >/dev/null) ||
		fail "cannot build the test release $tag"
	(cd "$work/releases/$tag" && ssh-keygen -q -Y sign -f "$work/keys/release" -n qhost-release checksums.txt) ||
		fail "cannot sign the test release $tag"
}

# release_publish makes the directory $work/releases/$1 (or the directory $2) the release $1 of the releases fake.
release_publish() {
	docker exec "$prefix-releases" rm -rf "/srv/releases/$1"
	docker cp "${2:-$work/releases/$1}" "$prefix-releases:/srv/releases/$1" || fail "cannot publish the test release $1"
}

# setup_service installs qhost from the fake release, checks the host service the installer started and the state
# without a deploy service, then sets the deploy service up the way an administrator does. With the argument
# default-backup-image the deploy service keeps the unit as installed and backs up with its default helper image;
# otherwise a drop-in names the local helper image, and the second release for the self-update section is built too,
# with a changed template of the instances.
setup_service() {
	step 'install qhost from a signed test release with the published installer'
	release_build v0.1.0
	[ "${1:-}" = default-backup-image ] || release_build v0.2.0 changed-template
	release_publish v0.1.0
	as_admin 'curl -fsSL -o /tmp/install.sh https://github.com/castrowithcee/qatlas-host/releases/latest/download/install.sh &&
		sudo sh /tmp/install.sh && rm -f /tmp/install.sh' || fail 'the installer failed'
	wait_for 'the host service channel' 20 docker exec "$host" sh -c 'ls /run/qhost | grep -q sock'
	log 'scenario: host service running'

	step 'after the installation only the host service runs, as qhost without the group docker, and keeps its directories closed'
	[ "$(docker exec "$host" systemctl is-active qhost)" = active ] || fail 'the host service is not active'
	pid=$(docker exec "$host" systemctl show -p MainPID --value qhost)
	[ -n "$pid" ] && [ "$pid" != 0 ] || fail 'the host service has no process'
	[ "$(docker exec "$host" stat -c %U "/proc/$pid")" = qhost ] || fail "the host service does not run as qhost: $(docker exec "$host" stat -c %U "/proc/$pid")"
	if docker exec "$host" grep '^Uid:' "/proc/$pid/status" | grep -qw 0; then fail 'the host service has a root user ID'; fi
	if docker exec "$host" id -nG qhost | tr ' ' '\n' | grep -qx docker; then fail 'the user qhost is in the group docker'; fi
	if docker exec "$host" grep '^Groups:' "/proc/$pid/status" | grep -qw "$(docker exec "$host" getent group docker | cut -d : -f 3)"; then
		fail 'the host service process holds the group docker'
	fi
	[ "$(docker exec "$host" stat -c %U:%G:%a /run/qhost)" = qhost:qhost:750 ] ||
		fail "/run/qhost is $(docker exec "$host" stat -c %U:%G:%a /run/qhost), want qhost:qhost:750"
	[ "$(docker exec "$host" stat -c %a /var/lib/qhost)" = 700 ] || fail "/var/lib/qhost has the mode $(docker exec "$host" stat -c %a /var/lib/qhost), want 700"
	ensure_user e2e-outsider
	if as_user e2e-outsider 'ls /run/qhost' >/dev/null 2>&1; then fail 'a user outside the group qhost can list /run/qhost'; fi
	[ "$(docker exec "$host" systemctl is-enabled qhost-deploy 2>&1 || true)" != enabled ] || fail 'the installer enabled the deploy service'
	[ "$(docker exec "$host" systemctl is-active qhost-deploy 2>&1 || true)" != active ] || fail 'the deploy service runs after the installation'
	if docker exec "$host" test -e /usr/local/lib/systemd/system/qhost-deploy.service; then fail 'the installer installed the unit of the deploy service'; fi
	if docker exec "$host" id qhost-deploy >/dev/null 2>&1; then fail 'the installer created the user qhost-deploy'; fi
	log "scenario: qhost runs as pid $pid, user qhost, without docker; /run/qhost qhost:qhost 750, /var/lib/qhost 700; no deploy service"

	step 'the installing admin is in the group qhost and sees the deploy service as not set up, with and without sudo'
	docker exec "$host" id -nG "$admin" | tr ' ' '\n' | grep -qx qhost || fail "the installer did not add $admin to the group qhost: $(docker exec "$host" id -nG "$admin")"
	as_admin 'id -nG' | tr ' ' '\n' | grep -qx qhost || fail "$admin has no group qhost in a new session: $(as_admin 'id -nG')"
	out=$(as_admin 'sudo qhost --output json status 2>&1') || fail "sudo qhost status failed before the setup: $out"
	printf '%s' "$out" | grep -q '"deploy_service": *"not set up"' || fail "sudo qhost status does not name the deploy service as not set up: $out"
	printf '%s' "$out" | grep -q '"host_service": *"running"' || fail "sudo qhost status does not show the host service: $out"
	out_plain=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed for an administrator of the group qhost: $out_plain"
	printf '%s' "$out_plain" | grep -q '"deploy_service": *"not set up"' || fail "qhost status without sudo does not name the deploy service as not set up: $out_plain"
	printf '%s' "$out_plain" | grep -q '"host_service": *"running' || fail "qhost status without sudo does not show the host service: $out_plain"
	log "scenario: qhost status before the setup, with sudo: $(printf '%s' "$out" | tr -d '\n ')"
	log "scenario: qhost status before the setup, without sudo: $(printf '%s' "$out_plain" | tr -d '\n ')"

	step 'a command of the deploy service before the setup ends with a clear message and exit code 3'
	for cmd in 'qhost stack list' 'qhost window show' 'qhost stack runs'; do
		out=$(docker exec "$host" sh -c "$cmd 2>&1 </dev/null; echo exit=\$?")
		printf '%s\n' "$out" | grep -qx 'exit=3' || fail "'$cmd' before the setup did not end with exit code 3: $out"
		printf '%s' "$out" | grep -q 'the deploy service is not set up; run sudo qhost deploy-service setup' || fail "'$cmd' before the setup does not say that the deploy service is not set up: $out"
		if printf '%s' "$out" | grep -qi 'cannot talk\|connect\|no such file\|refused\|dial'; then fail "'$cmd' before the setup shows a raw connection error: $out"; fi
	done
	out=$(as_admin 'qhost stack list 2>&1 || true')
	printf '%s' "$out" | grep -q 'not set up' || fail "'qhost stack list' without sudo does not say that the deploy service is not set up: $out"

	step 'the setup of the deploy service needs --confirm and root, then runs as qhost-deploy and changes nothing when repeated'
	if out=$(as_admin 'qhost deploy-service setup --confirm --yes 2>&1'); then fail "the setup without root succeeded: $out"; fi
	printf '%s' "$out" | grep -qi 'needs root' || fail "the setup without root does not say that it needs root: $out"
	if out=$(as_admin 'sudo qhost deploy-service setup --yes 2>&1'); then fail "the setup without --confirm succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'Set up the deploy service' || fail "the setup without --confirm does not show its plan: $out"
	printf '%s' "$out" | grep -q 'systemctl enable --now qhost-deploy' || fail "the setup without --confirm does not show its commands: $out"
	if docker exec "$host" test -e /usr/local/lib/systemd/system/qhost-deploy.service; then fail 'a refused setup installed the unit'; fi
	out=$(as_admin 'sudo qhost --output json deploy-service setup --confirm --yes 2>&1') || fail "the setup failed: $out"
	printf '%s' "$out" | grep -q '"changed": *true' || fail "the first setup did not report a change: $out"
	# The backup helper image is a local one, so a drop-in (which an update keeps) names it.
	if [ "${1:-}" != default-backup-image ]; then
		docker exec "$host" sh -ec '
			install -d /etc/systemd/system/qhost-deploy.service.d
			printf "[Service]\nExecStart=\nExecStart=/usr/local/bin/qhost deploy-service run --backup-image '"$backup_image"'\n" >/etc/systemd/system/qhost-deploy.service.d/e2e.conf
			systemctl daemon-reload' || fail 'cannot write the drop-in'
		docker exec "$host" systemctl restart qhost-deploy || fail 'cannot restart the deploy service'
	fi
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the deploy service to answer' 20 qd status
	out=$(as_admin 'sudo qhost --output json deploy-service setup --confirm --yes 2>&1') || fail "the repeated setup failed: $out"
	printf '%s' "$out" | grep -q '"changed": *false' || fail "the repeated setup reported a change: $out"
	log "scenario: deploy service set up; repeated setup: $(printf '%s' "$out" | tr -d '\n ')"

	step 'the deploy service runs as qhost-deploy with the group docker and keeps its directories closed'
	[ "$(docker exec "$host" systemctl is-active qhost-deploy)" = active ] || fail 'the deploy service is not active'
	[ "$(docker exec "$host" systemctl is-enabled qhost-deploy)" = enabled ] || fail 'the deploy service is not enabled'
	[ "$(docker exec "$host" systemctl is-active qhost)" = active ] || fail 'the host service stopped with the setup'
	[ "$(docker exec "$host" systemctl show -p MainPID --value qhost)" = "$pid" ] || fail 'the setup restarted the host service'
	dpid=$(docker exec "$host" systemctl show -p MainPID --value qhost-deploy)
	[ -n "$dpid" ] && [ "$dpid" != 0 ] || fail 'the deploy service has no process'
	[ "$(docker exec "$host" stat -c %U "/proc/$dpid")" = qhost-deploy ] || fail "the deploy service does not run as qhost-deploy: $(docker exec "$host" stat -c %U "/proc/$dpid")"
	if docker exec "$host" grep '^Uid:' "/proc/$dpid/status" | grep -qw 0; then fail 'the deploy service has a root user ID'; fi
	docker exec "$host" id -nG qhost-deploy | tr ' ' '\n' | grep -qx docker || fail 'the user qhost-deploy is not in the group docker'
	docker exec "$host" grep '^Groups:' "/proc/$dpid/status" | grep -qw "$(docker exec "$host" getent group docker | cut -d : -f 3)" ||
		fail 'the deploy service process does not hold the group docker'
	[ "$(docker exec "$host" stat -c %U:%G:%a /run/qhost-deploy)" = qhost-deploy:qhost:2750 ] ||
		fail "/run/qhost-deploy is $(docker exec "$host" stat -c %U:%G:%a /run/qhost-deploy), want qhost-deploy:qhost:2750"
	[ "$(docker exec "$host" stat -c %G "$(docker exec "$host" sh -c 'ls /run/qhost-deploy/*.sock | head -n 1')")" = qhost ] ||
		fail 'the socket of the deploy service does not have the group qhost'
	[ "$(docker exec "$host" stat -c %a /var/lib/qhost-deploy)" = 700 ] || fail "/var/lib/qhost-deploy has the mode $(docker exec "$host" stat -c %a /var/lib/qhost-deploy), want 700"
	[ "$(docker exec "$host" stat -c %U:%G:%a /etc/qhost-deploy)" = root:root:755 ] ||
		fail "/etc/qhost-deploy is $(docker exec "$host" stat -c %U:%G:%a /etc/qhost-deploy), want root:root:755"
	if as_user e2e-outsider 'ls /run/qhost-deploy' >/dev/null 2>&1; then fail 'a user outside the group qhost can list /run/qhost-deploy'; fi
	log "scenario: qhost-deploy runs as pid $dpid, user qhost-deploy, group docker; /run/qhost-deploy qhost-deploy:qhost 2750, /var/lib/qhost-deploy 700"

	step 'the two services stay apart: qhost-deploy cannot reach the state or the channel of the host service and qhost cannot read the state of the deploy service'
	if docker exec "$host" id -nG qhost-deploy | tr ' ' '\n' | grep -qx qhost; then fail 'the user qhost-deploy is in the group qhost'; fi
	if out=$(docker exec -u qhost-deploy "$host" ls /var/lib/qhost 2>&1); then fail "qhost-deploy lists /var/lib/qhost: $out"; fi
	printf '%s' "$out" | grep -qi 'permission denied' || fail "qhost-deploy was refused /var/lib/qhost for another reason: $out"
	if out=$(docker exec -u qhost-deploy "$host" ls /run/qhost 2>&1); then fail "qhost-deploy lists /run/qhost: $out"; fi
	printf '%s' "$out" | grep -qi 'permission denied' || fail "qhost-deploy was refused /run/qhost for another reason: $out"
	if out=$(docker exec -u qhost "$host" ls /var/lib/qhost-deploy 2>&1); then fail "qhost lists /var/lib/qhost-deploy: $out"; fi
	printf '%s' "$out" | grep -qi 'permission denied' || fail "qhost was refused /var/lib/qhost-deploy for another reason: $out"

	step 'the admin, with and without sudo, reads the status of both services'
	out=$(as_admin 'sudo qhost status 2>&1') || fail "sudo qhost status failed: $out"
	printf '%s' "$out" | grep -qi 'running\|started\|uptime\|version' || fail "sudo qhost status does not show the services: $out"
	out_plain=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed for an administrator of the group qhost: $out_plain"
	printf '%s' "$out_plain" | grep -q '"deploy_service": *"running"' || fail "qhost status without sudo does not show the deploy service: $out_plain"
	printf '%s' "$out_plain" | grep -q '"host_service": *"running' || fail "qhost status without sudo does not show the host service: $out_plain"
	log "scenario: qhost status answers with and without sudo: $(printf '%s' "$out_plain" | tr -d '\n ')"
}

# host_audit_rows prints the host changes of the action log as JSON with every space and line break removed.
host_audit_rows() {
	as_admin 'qhost --output json audit --action host.change --limit 100' | tr -d ' \n'
}

# restart_service restarts the deploy service, which makes the engine poll at once.
restart_service() {
	docker exec "$host" systemctl restart qhost-deploy || fail 'cannot restart the deploy service'
	wait_service
}

# wait_service waits for the channel of the deploy service and its answer after a start.
wait_service() {
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the deploy service to answer' 20 qd status
}

# service_pid prints the main process ID of the unit $1; without an argument that of the deploy service.
service_pid() {
	docker exec "$host" systemctl show -p MainPID --value "${1:-qhost-deploy}"
}

# installed_sum identifies the installed program and the units systemd uses, with their drop-ins.
installed_sum() {
	docker exec "$host" sh -c 'sha256sum /usr/local/bin/qhost; systemctl cat qhost 2>&1 | sha256sum; systemctl cat qhost-deploy 2>&1 | sha256sum'
}

# host_serving succeeds when the host service is active and answers root.
host_serving() {
	[ "$(docker exec "$host" systemctl is-active qhost)" = active ] &&
		docker exec "$host" qhost --output json status 2>&1 </dev/null | grep -q '"host_service": *"running"'
}

# service_serving succeeds when both services are active and the deploy service answers.
service_serving() {
	host_serving && [ "$(docker exec "$host" systemctl is-active qhost-deploy)" = active ] && qd status >/dev/null
}
