#!/bin/sh
# Scenario: from the simulated host, clone the demo stacks repository over SSH and push a commit; then check
# the demo image tags and that a demo container turns unhealthy on purpose. The rootless Docker engine of a test user runs
# without a login session, starts a container from the registry mirror under a sub-UID of the user, and the system
# engine does not see that container. Later source inspect reports the containers of a folder in that engine as
# conflicts and lists the engine as not checked while it is stopped. Then install qhost on the host
# from a signed test release with the release installer, which sets up only the host service, and check its user
# and directories, that it has no access to the container engine, that the installing admin joins the group qhost
# and that status names the deploy service as not set up and a deploy command says so. Then set the deploy service
# up with sudo qhost deploy-service setup and check its user qhost-deploy with the group docker, its channel and
# state, that the two services cannot reach each other's state, and that an administrator of the group reads
# status without sudo while a change needs root. The host service serves an administrator of the group without sudo,
# answers a user of the group without sudo not-admin, turns away a user outside the group and a revoked administrator,
# and the check of its action log reports a changed line. Changes of the host follow: sudo qhost host apply changes the
# permissions of a folder, the journal shows it, the undo restores them and the action log names the person behind
# sudo; a user is created, put in a group, locked and deleted, and the protection rules refuse the rest. The terminal
# interface, started without sudo, creates a group and a user with an initial password in it through the real sudo,
# and qhost host users shows the user with the group. An administrator without sudo proposes a user with a group and an
# SSH key with qhost proposal; the interface approves it, asks for the initial password and creates the user, the
# journal shows the change, the undo removes the user, and a process of qhost-deploy is not shown the proposal. The
# deploy service for the rootless engine of the test user is set up through the interface after the command line showed
# its steps, a user in the group docker is refused, the instance runs as the test user against that user's engine, status
# shows it without sudo, only root and the group qhost reach its channel and systemd-analyze verify accepts its unit.
# Then bind the stacks repository with its own deploy key, list the
# stacks, deploy a new commit, show the available image versions per update ceiling without changing anything,
# roll back a commit with a failing healthcheck (data restored from the run's backup, revert commit on the
# remote), leave a stack whose rollback fails stopped and paused until resume, and refuse a changed host key of the git server.
# A second section takes over an installation of the earlier single binding as the first source and binds a second
# repository: its own stack is deployed, a stack named like one of the first source is reported and never deployed.
# Version updates with qhost stack update follow: qhost stack settings set commits and pushes the settings of a
# stack without a run, and they act on the next update: one direct step, a stepwise major jump and a jump whose
# middle step fails. The maintenance window follows on a real, short window: a stack in automatic mode is updated
# without a human, a stack in manual mode is only reported, a paused and a pinned stack stay as they are, and a change with
# window set takes effect in the running service. The terminal interface follows: sudo qhost opens it in a
# tmux pane, an update shows its changelog before it is confirmed and then runs, and two ticked stacks are updated
# one after another with one confirmation. An update that fails after its start is rolled back in its observation time. Notification channels (webhook, Slack, Telegram, e-mail) are
# added with qhost notify: a webhook hears of a deployed and of a rolled-back run, and every channel
# type delivers a test message. A local stack folder gets a remote in the terminal interface: an empty repository
# receives its history, a differing one is shown and left alone without the confirmation. Docker integration tests of the deployer run inside the host
# before all that. The last section resets the service to a fresh state and does the first setup only through
# the terminal interface: deploy key, registry credential, maintenance window, notification channel with its
# test, repository with a confirmed host key, a stack secret and the first deploy; then it restores an older
# backup through the interface and proves that no masked value shows anywhere. Then the update rules of a new stack
# are set in the interface (automatic, ceiling minor): the stacks repository gets a commit with its settings file,
# the next maintenance window updates the stack, and qhost stack settings remove returns it to deploy only. The backup
# of a stack with a named volume is set in the interface (the volume ticked): the settings file gets a commit, and the next
# update run backs up exactly that volume. The HTTP check of a stack is set in the interface: the settings file gets a
# commit with the check, and "clear settings" deletes the file by a commit. At the very
# end the installed program updates itself to a newer signed test release, which replaces and restarts both
# services, renews the changed template of the rootless instance, restarts the instance with the new program and warns
# about an instance unit an administrator overrides, refuses tampered ones, and updates once more with the deploy
# service put aside, which leaves its unit uninstalled. The very last section puts the deploy service back and removes
# both services and the rootless instance with sudo qhost uninstall: the containers of the stacks keep running, the
# stack folders stay unchanged, both units, the instance, its template and the program are gone and the state, the
# users and the group stay; sudo qhost uninstall --purge then removes the three state folders, the state and
# configuration of the instance, the users and the group. The test user, its home, its rootless engine and its
# container stay throughout. Sourced by run.sh scenario.

# signature_asset is the name of the release file with the signature; it must match release.SignatureAsset.
signature_asset=checksums.txt.sig
# instance_service names the deploy service instances, and release_template is their template unit in packaging/.
instance_service=qhost-deploy
release_template=$instance_service@.service

as_admin() {
	docker exec -i -u "$admin" "$host" sh -ec "$*"
}

# as_user runs a command on the host as the test user $1.
as_user() {
	user=$1
	shift
	docker exec -i -u "$user" "$host" sh -c "$*" </dev/null
}

# ensure_user creates the test user $1 without sudo rights and without the group qhost, if it is missing.
ensure_user() {
	docker exec "$host" id "$1" >/dev/null 2>&1 || docker exec "$host" useradd --create-home --shell /bin/bash "$1" ||
		fail "cannot create the test user $1"
}

step() {
	log "scenario: $*"
}

run_scenario() {
	repo=git.$domain:stacks.git

	step 'clone over SSH'
	as_admin "rm -rf ~/stacks && git clone -q $repo ~/stacks"
	as_admin 'cd ~/stacks && test -f demo-stack/compose.yaml && test -f demo-stack/.env && grep -qx "secrets.env" demo-stack/.gitignore && grep -qx "data/" demo-stack/.gitignore' ||
		fail 'the demo repository does not have the expected stack files'
	as_admin 'cd ~/stacks && grep -q "^DEMO_APP_VERSION=" demo-stack/.env && ! git ls-files | grep -q secrets.env' ||
		fail 'the demo repository does not hold the image version as a variable or tracks a secrets file'

	step 'commit and push'
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.1/" demo-stack/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Update demo-stack to 1.0.1" &&
		git push -q origin main'
	local_head=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	remote_head=$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)
	[ "$local_head" = "$remote_head" ] || fail "the pushed commit is not on the server ($local_head vs $remote_head)"
	as_admin "rm -rf ~/stacks2 && git clone -q $repo ~/stacks2 && grep -qx DEMO_APP_VERSION=1.0.1 ~/stacks2/demo-stack/.env" ||
		fail 'a fresh clone does not see the pushed commit'

	step 'the git server offers no shell'
	if as_admin "ssh -o BatchMode=yes git.$domain id" >/dev/null 2>&1; then
		fail 'the git server ran an arbitrary command'
	fi

	step 'registry tags'
	got=$(as_admin "curl -fsS https://registry.$domain/v2/demo/app/tags/list")
	log "scenario: tags $got"
	for tag in $tags; do
		printf '%s' "$got" | grep -q "\"$tag\"" || fail "the registry does not list $tag"
	done

	step 'demo healthcheck turns unhealthy on purpose'
	image=registry.$domain/demo/app:1.0.0
	docker exec "$host" docker rm -f "$prefix-demo-ok" "$prefix-demo-bad" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$prefix-demo-ok" "$image" >/dev/null
	docker exec "$host" docker run -d --name "$prefix-demo-bad" -e DEMO_HEALTH=fail "$image" >/dev/null
	wait_for 'the healthy demo container' 30 health_is "$prefix-demo-ok" healthy
	wait_for 'the unhealthy demo container' 30 health_is "$prefix-demo-bad" unhealthy
	log "scenario: $prefix-demo-ok healthy, $prefix-demo-bad unhealthy"
	docker exec "$host" docker rm -f "$prefix-demo-ok" "$prefix-demo-bad" >/dev/null

	scenario_rootless_engine

	scenario_compose_integration

	scenario_stacks_repo

	scenario_selfupdate

	scenario_uninstall

	log 'scenario: passed'
}

# as_rootless runs a command on the host as $rootless_user, with the environment of its rootless Docker engine.
as_rootless() {
	as_user "$rootless_user" "export XDG_RUNTIME_DIR=/run/user/$rootless_uid DOCKER_HOST=unix:///run/user/$rootless_uid/docker.sock; $*"
}

rootless_health_is() {
	[ "$(as_rootless "docker inspect -f '{{.State.Health.Status}}' $1")" = "$2" ]
}

# scenario_rootless_engine checks the rootless Docker engine of $rootless_user that run.sh up started.
scenario_rootless_engine() {
	step "the rootless Docker engine of $rootless_user runs with lingering and without a login session"
	groups=$(docker exec "$host" id -nG "$rootless_user")
	case " $groups " in *" sudo "* | *" docker "* | *" qhost "*) fail "$rootless_user is in a privileged group: $groups" ;; esac
	[ "$(docker exec "$host" loginctl show-user "$rootless_user" -p Linger --value)" = yes ] ||
		fail "$rootless_user has no lingering"
	[ -z "$(docker exec "$host" loginctl show-user "$rootless_user" -p Sessions --value)" ] ||
		fail "$rootless_user has a login session"
	[ "$(as_rootless 'systemctl --user is-active docker.service')" = active ] ||
		fail "the user unit docker.service of $rootless_user is not active"
	as_rootless "docker info -f '{{.SecurityOptions}}'" | grep -q 'name=rootless' ||
		fail "the engine on /run/user/$rootless_uid/docker.sock is not rootless"

	step 'a container of the rootless engine comes from the registry mirror and runs under a sub-UID'
	rootless_ct=$prefix-rootless-demo
	# A Docker Hub name: only the registry mirror resolves it.
	as_rootless "docker run -d --quiet --name $rootless_ct --user 1234 demo/app:1.0.0" >/dev/null ||
		fail 'the rootless engine cannot start demo/app:1.0.0 from the registry mirror'
	wait_for 'the healthy demo container of the rootless engine' 30 rootless_health_is "$rootless_ct" healthy
	pid=$(as_rootless "docker inspect -f '{{.State.Pid}}' $rootless_ct")
	docker exec "$host" grep -q demoapp "/proc/$pid/cmdline" || fail "process $pid is not the demo app of $rootless_ct"
	sub_start=$(docker exec "$host" awk -F: -v u="$rootless_user" '$1 == u { print $2 }' /etc/subuid)
	got=$(docker exec "$host" stat -c %u "/proc/$pid")
	# The container's user 1 is the first sub-UID.
	[ "$got" -eq $((sub_start + 1233)) ] || fail "the process of $rootless_ct runs as $got, not as the sub-UID $((sub_start + 1233))"
	log "scenario: $rootless_ct runs as uid $got on the host"

	step 'the system engine does not see the container of the rootless engine'
	[ -z "$(docker exec "$host" docker ps -aq --filter "name=$rootless_ct")" ] ||
		fail "the system engine shows $rootless_ct"
	as_rootless "docker rm -f $rootless_ct" >/dev/null
}

# instance_refusals prints how many peers the deploy service instance $ri_unit has refused at its channel.
instance_refusals() {
	docker exec "$host" journalctl -u "$ri_unit" -o cat | grep -c 'refused peer' || true
}

# instance_refused_more succeeds when the instance has refused more peers than $refusals_before.
instance_refused_more() {
	[ "$(instance_refusals)" -gt "$refusals_before" ]
}

# scenario_rootless_instance sets up the deploy service instance for the rootless engine of $rootless_user. The
# command line shows the steps and changes nothing without an answer, the terminal interface refuses a user in the
# group docker and sets the instance up, and a repeated setup has no steps. The instance runs as the user and
# reaches the user's engine inside the sandbox of its unit, status shows it to an administrator of the group qhost
# without sudo, its channel serves only root and that group, and systemd-analyze verify accepts its unit. It needs
# the deploy service of the system engine and leaves the instance running.
scenario_rootless_instance() {
	ri_svc=qhost-deploy
	ri_unit=$ri_svc@$rootless_user.service
	ri_template=/usr/local/lib/systemd/system/$ri_svc@.service
	ri_rule=/etc/tmpfiles.d/$ri_svc-$rootless_user.conf
	ri_channel=/run/$ri_svc-$rootless_user
	ri_state=/var/lib/$ri_svc-$rootless_user
	# An invented user in the group docker, created for the refusal only.
	ri_docker_user=e2e-indocker

	step "the setup for the rootless engine of $rootless_user shows its steps on the command line and changes nothing without an answer"
	if out=$(as_admin "sudo qhost deploy-service setup --rootless $rootless_user --confirm 2>&1 </dev/null"); then
		fail "the setup without an answer succeeded: $out"
	fi
	for want in "Set up a deploy service for the rootless Docker engine of the user $rootless_user" \
		"write $ri_template (mode 0644)" 'systemctl daemon-reload' "write $ri_rule (mode 0644)" \
		"systemd-tmpfiles --create $ri_rule" "systemctl enable --now $ri_unit"; do
		printf '%s' "$out" | grep -qF -- "$want" || fail "the question of the setup does not show '$want': $out"
	done
	if printf '%s' "$out" | grep -q 'enable-linger'; then fail "the setup plans lingering, which $rootless_user has: $out"; fi
	for p in "$ri_template" "$ri_rule" "$ri_channel" "$ri_state"; do
		if docker exec "$host" test -e "$p"; then fail "the setup without an answer created $p"; fi
	done
	log "scenario: the setup question without an answer:"
	printf '%s\n' "$out" | sed 's/^/e2e:   | /'

	step 'the setup refuses a user in the group docker with its reason, in the terminal interface and on the command line'
	docker exec "$host" sh -c "id $ri_docker_user >/dev/null 2>&1 || useradd --no-create-home --shell /usr/sbin/nologin --groups docker $ri_docker_user" ||
		fail "cannot create the test user $ri_docker_user"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send G
	tui_wait 'Which deploy service do you want to set up?' 10
	tui_send r
	tui_wait 'User of the rootless engine (with an engine:' 10
	tui_has "$rootless_user" || fail "the interface does not offer $rootless_user: $(tui_screen)"
	tui_type "$ri_docker_user"
	tui_send Enter
	tui_wait "the user $ri_docker_user is in the group docker" 15
	log 'scenario: the interface refuses the user in the group docker:'
	tui_excerpt "the user $ri_docker_user is in the group docker" 2
	if out=$(qd deploy-service setup --rootless "$ri_docker_user" --confirm --yes); then fail "the setup accepted a user in the group docker: $out"; fi
	printf '%s' "$out" | grep -q "the user $ri_docker_user is in the group docker" || fail "the refusal does not name the group docker: $out"
	if docker exec "$host" test -e "/etc/tmpfiles.d/$ri_svc-$ri_docker_user.conf"; then fail "the refused setup wrote the rule of $ri_docker_user"; fi
	docker exec "$host" userdel "$ri_docker_user" || fail "cannot delete the test user $ri_docker_user"
	log "scenario: the command line refuses the user in the group docker: $out"

	step "the terminal interface sets up the instance for $rootless_user, and a repeated setup has nothing to do"
	tui_send G
	tui_wait 'Which deploy service do you want to set up?' 10
	tui_send r
	tui_wait 'User of the rootless engine (with an engine:' 10
	tui_type "$rootless_user"
	tui_send Enter
	# The dialog wraps the question.
	tui_wait 'Set up the deploy service for the rootless engine of' 15
	tui_has "systemctl enable --now $ri_unit" || fail "the question does not show the commands: $(tui_screen)"
	log 'scenario: the question of the interface:'
	tui_excerpt "Set up the deploy service for the rootless engine" 16
	tui_send y
	tui_wait "the deploy service for the rootless engine of $rootless_user is set up and started" 90
	tui_send G
	tui_wait 'Which deploy service do you want to set up?' 10
	tui_send r
	tui_wait 'User of the rootless engine (with an engine:' 10
	tui_type "$rootless_user"
	tui_send Enter
	tui_wait 'nothing to do: the deploy service is set up already' 15
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin "sudo qhost --output json deploy-service setup --rootless $rootless_user --confirm --yes 2>&1") ||
		fail "the repeated setup on the command line failed: $out"
	printf '%s' "$out" | grep -q '"changed": *false' || fail "the repeated setup reported a change: $out"
	printf '%s' "$out" | grep -q '"steps": *0' || fail "the repeated setup ran steps: $out"
	printf '%s' "$out" | grep -q "\"unit\": *\"$ri_unit\"" || fail "the repeated setup does not name the unit: $out"
	log "scenario: repeated setup on the command line: $(printf '%s' "$out" | tr -d '\n ')"

	step "the instance runs as $rootless_user without the groups docker and qhost and reaches the rootless engine inside its sandbox"
	[ "$(docker exec "$host" systemctl is-active "$ri_unit")" = active ] || fail "$ri_unit is not active"
	[ "$(docker exec "$host" systemctl is-enabled "$ri_unit")" = enabled ] || fail "$ri_unit is not enabled"
	ri_pid=$(docker exec "$host" systemctl show -p MainPID --value "$ri_unit")
	[ -n "$ri_pid" ] && [ "$ri_pid" != 0 ] || fail "$ri_unit has no process"
	[ "$(docker exec "$host" stat -c %U "/proc/$ri_pid")" = "$rootless_user" ] ||
		fail "$ri_unit does not run as $rootless_user: $(docker exec "$host" stat -c %U "/proc/$ri_pid")"
	[ "$(docker exec "$host" awk '/^Uid:/ { print $2 $3 $4 $5 }' "/proc/$ri_pid/status")" = "$rootless_uid$rootless_uid$rootless_uid$rootless_uid" ] ||
		fail "the process of $ri_unit has another user ID than $rootless_uid: $(docker exec "$host" grep '^Uid:' "/proc/$ri_pid/status")"
	for g in docker qhost; do
		if docker exec "$host" grep '^Groups:' "/proc/$ri_pid/status" | grep -qw "$(docker exec "$host" getent group "$g" | cut -d : -f 3)"; then
			fail "the process of $ri_unit holds the group $g"
		fi
	done
	docker exec "$host" sh -c "tr '\\0' '\\n' </proc/$ri_pid/cmdline" | grep -qx -- '--instance' ||
		fail "the process of $ri_unit was not started with --instance: $(docker exec "$host" sh -c "tr '\\0' ' ' </proc/$ri_pid/cmdline")"
	docker exec "$host" sh -c "tr '\\0' '\\n' </proc/$ri_pid/environ" | grep -qx "DOCKER_CONFIG=$ri_state/docker" ||
		fail "the process of $ri_unit has not DOCKER_CONFIG=$ri_state/docker"
	ri_gid=$(docker exec "$host" id -g "$rootless_user")
	# The command runs in the mount namespace of the unit, so it sees the file system the service sees.
	out=$(docker exec "$host" nsenter --target "$ri_pid" --mount --setuid "$rootless_uid" --setgid "$ri_gid" \
		env DOCKER_CONFIG="$ri_state/docker" docker -H "unix:///run/user/$rootless_uid/docker.sock" info -f '{{.SecurityOptions}}' 2>&1) ||
		fail "the sandbox of $ri_unit cannot reach the engine of $rootless_user: $out"
	printf '%s' "$out" | grep -q 'name=rootless' || fail "the engine the sandbox of $ri_unit reaches is not rootless: $out"
	log "scenario: $ri_unit runs as pid $ri_pid, user $rootless_user, and reaches /run/user/$rootless_uid/docker.sock: $out"

	step 'qhost status shows the instance as running, with sudo and without sudo for an administrator of the group qhost'
	out=$(as_admin 'sudo qhost --output json status 2>&1') || fail "sudo qhost status failed: $out"
	printf '%s' "$out" | grep -q "\"instance\.$rootless_user\.deploy_service\": *\"running\"" || fail "sudo qhost status does not show the instance as running: $out"
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed: $out"
	printf '%s' "$out" | grep -q "\"instance\.$rootless_user\.unit\": *\"$ri_unit\"" || fail "qhost status without sudo does not name the unit of the instance: $out"
	printf '%s' "$out" | grep -q "\"instance\.$rootless_user\.deploy_service\": *\"running\"" || fail "qhost status without sudo does not show the instance as running: $out"
	printf '%s' "$out" | grep -q '"deploy_service": *"running"' || fail "qhost status without sudo does not show the deploy service of the system engine as running: $out"
	log "scenario: qhost status without sudo: $(printf '%s' "$out" | tr -d '\n ' | grep -o "\"instance\.$rootless_user\.[^,}]*" | tr '\n' ' ')"

	step 'the channel of the instance serves only root and the group qhost, and its state stays closed'
	[ "$(docker exec "$host" stat -c %U:%G:%a "$ri_channel")" = "$rootless_user:qhost:2750" ] ||
		fail "$ri_channel is $(docker exec "$host" stat -c %U:%G:%a "$ri_channel"), want $rootless_user:qhost:2750"
	[ "$(docker exec "$host" stat -c %G "$(docker exec "$host" sh -c "ls $ri_channel/*.sock | head -n 1")")" = qhost ] ||
		fail "the socket of $ri_unit does not have the group qhost"
	[ "$(docker exec "$host" stat -c %U:%a "$ri_state")" = "$rootless_user:700" ] ||
		fail "$ri_state is $(docker exec "$host" stat -c %U:%a "$ri_state"), want $rootless_user:700"
	ensure_user e2e-outsider
	if out=$(as_user e2e-outsider "ls $ri_channel" 2>&1); then fail "a user outside the group qhost can list $ri_channel: $out"; fi
	printf '%s' "$out" | grep -qi 'permission denied' || fail "e2e-outsider was refused $ri_channel for another reason: $out"
	# The user of the instance owns the channel directory and reaches the socket; the instance turns it away, because it
	# is neither root nor in the group qhost. qhost status asks every instance before the deploy service.
	refusals_before=$(instance_refusals)
	as_user "$rootless_user" 'qhost status' >/dev/null 2>&1 || true
	wait_for "the instance to refuse $rootless_user at its channel" 15 instance_refused_more
	log "scenario: the socket in $ri_channel is $(docker exec "$host" sh -c "stat -c '%U:%G %a' $ri_channel/*.sock"); e2e-outsider cannot enter the channel and the instance refused $rootless_user"

	step "systemd-analyze verify accepts $ri_unit"
	out=$(docker exec "$host" systemd-analyze verify "$ri_unit" 2>&1) || fail "systemd-analyze verify $ri_unit failed: $out"
	log "scenario: systemd-analyze verify $ri_unit passed${out:+: $out}"
}

# qd runs the qhost CLI on the host as root; stderr is merged so a failure message can be checked.
qd() {
	docker exec -i "$host" qhost "$@" 2>&1 </dev/null
}

# qd_fails runs qd, requires a non-zero exit and prints the output.
qd_fails() {
	if out=$(qd "$@"); then
		printf '%s\n' "$out"
		return 1
	fi
	printf '%s\n' "$out"
}

known_hosts_sum() {
	docker exec "$host" sh -c 'sha256sum /var/lib/qhost-deploy/known_hosts 2>/dev/null || echo none'
}

git_ed25519_fingerprint() {
	docker exec "$prefix-git" ssh-keygen -lf /etc/ssh/ssh_host_ed25519_key.pub | cut -d ' ' -f 2
}

# scanned_git_key prints the ed25519 host key the git server offers now.
scanned_git_key() {
	docker exec -u "$admin" "$host" ssh-keyscan -t ed25519 "git.$domain" 2>/dev/null | grep -v "^#" | cut -d ' ' -f 2,3
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

# host_log_file prints the newest day file of the action log of the host service.
host_log_file() {
	docker exec "$host" sh -c 'ls /var/lib/qhost/logs/*.jsonl | tail -n 1'
}

# host_log_rewrite runs the sed expression $1 on the day file $2 in place without replacing the file, so it keeps its
# owner qhost and its mode, and the service keeps appending to it.
host_log_rewrite() {
	docker exec "$host" sh -ec 'sed -E "$1" "$2" >/tmp/qhost-log.jsonl && cat /tmp/qhost-log.jsonl >"$2" && rm -f /tmp/qhost-log.jsonl' sh "$1" "$2"
}

# scenario_host_service proves the access to the host service and its action log: an administrator of the group qhost
# reads the state of the host service and the log without sudo, a user of the group without sudo is told not-admin, a
# user outside the group is turned away at the channel, a revoked administrator is refused at the next request, and
# the check of the log reports a changed line. It runs right after the setup.
scenario_host_service() {
	step 'an administrator of the group qhost reads the state of the host service and its action log without sudo'
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed: $out"
	printf '%s' "$out" | grep -q '"host_service": *"running"' || fail "qhost status without sudo does not show the host service as running: $out"
	printf '%s' "$out" | grep -q '"host_version": *"v0.1.0"' || fail "qhost status without sudo does not show the version of the host service: $out"
	out=$(as_admin 'qhost --output json audit --action control.status 2>&1') || fail "qhost audit without sudo failed: $out"
	printf '%s' "$out" | grep -q '"action": *"control.status"' || fail "qhost audit does not list the status requests: $out"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost/logs)" = qhost:700 ] ||
		fail "/var/lib/qhost/logs is $(docker exec "$host" stat -c %U:%a /var/lib/qhost/logs), want qhost:700"
	[ "$(docker exec "$host" stat -c %U:%a "$(host_log_file)")" = qhost:600 ] || fail 'the day file of the action log is not qhost:600'
	if docker exec "$host" test -e /var/lib/qhost-host/logs; then fail 'the action log was written outside the state of the host service'; fi
	log "scenario: $admin read the host service and its action log without sudo"

	step 'a user of the group qhost without sudo is told not-admin by the host service, a user outside the group is turned away'
	ensure_user e2e-hostreader
	ensure_user e2e-outsider
	docker exec "$host" usermod -aG qhost e2e-hostreader || fail 'cannot add e2e-hostreader to the group qhost'
	reader_uid=$(docker exec "$host" id -u e2e-hostreader)
	for cmd in 'qhost audit' 'qhost audit verify'; do
		if out=$(as_user e2e-hostreader "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-hostreader: $out"; fi
		printf '%s' "$out" | grep -q 'qhost is reserved for administrators (sudo)' || fail "unexpected refusal of '$cmd' for e2e-hostreader: $out"
		if printf '%s' "$out" | grep -q 'control\.\|log\.read\|intact'; then fail "'$cmd' shows data to e2e-hostreader: $out"; fi
		if out=$(as_user e2e-outsider "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-outsider: $out"; fi
		printf '%s' "$out" | grep -q 'may not use the qhost service channel' || fail "unexpected refusal of '$cmd' for e2e-outsider: $out"
	done
	out=$(as_admin 'qhost --output json audit --action control.refuse --result failure 2>&1') || fail "qhost audit of the refusals failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q "\"client_uid\":$reader_uid,\"action\":\"control.refuse\",\"result\":\"failure\",\"error_code\":\"not-admin\"" ||
		fail "the action log does not record e2e-hostreader as refused with not-admin: $out"
	log "scenario: e2e-hostreader (uid $reader_uid, group qhost, no sudo) got not-admin, e2e-outsider was turned away at the channel"

	step 'a revoked administrator is refused by the host service at the next request'
	ensure_user e2e-hostadmin
	docker exec "$host" usermod -aG qhost,sudo e2e-hostadmin || fail 'cannot make e2e-hostadmin an administrator of the group qhost'
	out=$(as_user e2e-hostadmin 'qhost audit verify 2>&1') || fail "qhost audit verify failed for the administrator e2e-hostadmin: $out"
	docker exec "$host" gpasswd -d e2e-hostadmin sudo >/dev/null || fail 'cannot remove e2e-hostadmin from the group sudo'
	if out=$(as_user e2e-hostadmin 'qhost audit verify 2>&1'); then fail "qhost audit verify succeeded after the revocation: $out"; fi
	printf '%s' "$out" | grep -q 'qhost is reserved for administrators (sudo)' || fail "unexpected refusal after the revocation: $out"
	docker exec "$host" id -nG e2e-hostadmin | tr ' ' '\n' | grep -qx qhost || fail 'e2e-hostadmin lost the group qhost, so the refusal proves nothing about sudo'
	log 'scenario: e2e-hostadmin read the log as an administrator and was refused after leaving the group sudo'

	step 'the check of the action log reports a changed line and passes again once the line is back'
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed on the untouched log: $out"
	printf '%s' "$out" | grep -q 'intact' || fail "qhost audit verify does not report the log as intact: $out"
	file=$(host_log_file)
	[ "$(docker exec "$host" sh -c 'wc -l <"$1"' sh "$file")" -ge 3 ] || fail "the day file $file has fewer than three lines"
	uid2=$(docker exec "$host" sed -n '2p' "$file" | sed -E 's/.*"client_uid":([0-9]+).*/\1/')
	case $uid2 in '' | *[!0-9]*) fail "line 2 of $file names no client: $(docker exec "$host" sed -n '2p' "$file")" ;; esac
	host_log_rewrite "2s/\"client_uid\":$uid2([,}])/\"client_uid\":4242424\\1/" "$file" || fail "cannot change line 2 of $file"
	docker exec "$host" sed -n '2p' "$file" | grep -q '"client_uid":4242424' || fail 'line 2 of the action log was not changed'
	if out=$(as_admin 'qhost audit verify 2>&1'); then fail "qhost audit verify passed a changed line: $out"; fi
	printf '%s' "$out" | grep -q 'broken' || fail "qhost audit verify does not mark the day as broken: $out"
	printf '%s' "$out" | grep -q 'entry with sequence [0-9]* (line 3) does not chain from the previous entry' || fail "qhost audit verify does not name the line after the change: $out"
	printf '%s' "$out" | grep -q 'the action log of the host service is broken' || fail "qhost audit verify does not say that the log is broken: $out"
	log "scenario: qhost audit verify on a changed line: $(printf '%s' "$out" | grep broken | head -n 1)"
	host_log_rewrite "2s/\"client_uid\":4242424([,}])/\"client_uid\":$uid2\\1/" "$file" || fail "cannot restore line 2 of $file"
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the line was restored: $out"
	if printf '%s' "$out" | grep -q 'broken'; then fail "qhost audit verify still reports a broken day: $out"; fi
	[ "$(docker exec "$host" stat -c %U:%a "$file")" = qhost:600 ] || fail 'the day file lost its owner or mode'
}

# host_audit_rows prints the host changes of the action log as JSON with every space and line break removed.
host_audit_rows() {
	as_admin 'qhost --output json audit --action host.change --limit 100' | tr -d ' \n'
}

# scenario_host_changes proves the changes of the host through the CLI. A folder gets other permissions with sudo
# qhost host apply, the journal lists the change with what its undo does, the undo brings the permissions back, and
# the action log names the journal entry and the person behind sudo for both. A user is created with its password on
# standard input only, put in a group, locked and deleted, with the group deleted after it. The protection rules
# refuse changing one's own account and the group sudo, a change needs root and --confirm, and neither the
# password nor anything else secret reaches the journal or the action log. It runs after the setup, when the host
# service keeps the action log.
scenario_host_changes() {
	step 'a change of the host needs root and --confirm, and the journal is closed to everyone else'
	dir=/home/$admin/e2e-host-folder
	grp=$(as_admin 'id -gn')
	as_admin "rm -rf '$dir' && mkdir '$dir' && chmod 0755 '$dir'" || fail 'cannot make the test folder'
	if out=$(as_admin "qhost host apply folder.set-permissions '$dir' $admin $grp read-write read nothing --confirm 2>&1"); then
		fail "host apply without root succeeded: $out"
	fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "host apply without root does not say that it needs root: $out"
	if out=$(as_admin 'qhost host journal 2>&1'); then fail "host journal without root succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "host journal without root does not say that it needs root: $out"
	if out=$(as_admin "sudo qhost host apply folder.set-permissions '$dir' $admin $grp read-write read nothing 2>&1"); then
		fail "host apply without --confirm succeeded: $out"
	fi
	printf '%s' "$out" | grep -q -- '--confirm' || fail "host apply without --confirm does not name the flag: $out"
	[ "$(as_admin "stat -c %a '$dir'")" = 755 ] || fail 'a refused change touched the folder'
	reader=$(as_user e2e-hostreader 'qhost host users 2>&1 || true')
	printf '%s' "$reader" | grep -q 'qhost is reserved for administrators (sudo)' || fail "e2e-hostreader reads the users of the host: $reader"
	out=$(as_admin 'qhost --output json host users 2>&1') || fail "qhost host users without sudo failed for an administrator: $out"
	printf '%s' "$out" | grep -q "\"name\": *\"$admin\"" || fail "qhost host users does not list $admin: $out"
	out=$(as_admin "qhost --output json host folder '$dir' 2>&1") || fail "qhost host folder failed: $out"
	printf '%s' "$out" | grep -q '"mode": *"[0-7]*755"' || fail "qhost host folder does not show the mode 755: $out"

	step 'sudo qhost host apply folder.set-permissions changes the permissions, the journal shows the entry and the undo restores them'
	admin_uid=$(docker exec "$host" id -u "$admin")
	out=$(as_admin "sudo qhost --output json host apply folder.set-permissions '$dir' $admin $grp read-write read nothing --confirm 2>&1") ||
		fail "host apply folder.set-permissions failed: $out"
	change_id=$(json_value "$(printf '%s' "$out" | tr -d ' \n')" journal_id)
	[ -n "$change_id" ] || fail "host apply reports no journal entry: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"applied"' || fail "host apply does not report the result applied: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"warning":""' || fail "host apply warns although the host service runs: $out"
	[ "$(as_admin "stat -c %a '$dir'")" = 750 ] || fail "the folder has the mode $(as_admin "stat -c %a '$dir'") after the change, want 750"
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	flat=$(printf '%s' "$journal" | tr -d ' \n')
	printf '%s' "$flat" | grep -q "\"id\":\"$change_id\"" || fail "the journal does not list $change_id: $journal"
	printf '%s' "$flat" | grep -q "\"action\":\"folder.set-permissions\"" || fail "the journal does not name the action: $journal"
	printf '%s' "$flat" | grep -q "\"sudo_uid\":$admin_uid" || fail "the journal does not name the user behind sudo ($admin_uid): $journal"
	printf '%s' "$flat" | grep -q '"reversible":true' || fail "the journal does not offer the undo: $journal"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost-host/journal)" = root:700 ] ||
		fail "the journal folder is $(docker exec "$host" stat -c %U:%a /var/lib/qhost-host/journal), want root:700"
	[ "$(docker exec "$host" stat -c %U:%a "/var/lib/qhost-host/journal/$change_id.json")" = root:600 ] ||
		fail 'the journal entry is not root:600'
	out=$(as_admin "sudo qhost --output json host undo $change_id --confirm 2>&1") || fail "host undo failed: $out"
	undo_id=$(json_value "$(printf '%s' "$out" | tr -d ' \n')" journal_id)
	[ -n "$undo_id" ] && [ "$undo_id" != "$change_id" ] || fail "host undo reports no new journal entry: $out"
	[ "$(as_admin "stat -c %a '$dir'")" = 755 ] || fail "the undo left the mode $(as_admin "stat -c %a '$dir'"), want 755"
	if out=$(as_admin "sudo qhost host undo $change_id --confirm 2>&1"); then fail "a second undo succeeded: $out"; fi
	log "scenario: folder.set-permissions $change_id changed 755 to 750, undo $undo_id restored 755"

	step 'the action log names the person behind sudo and the journal entry of the change and of its undo'
	rows=$(host_audit_rows)
	printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"folder.set-permissions$change_id\"" ||
		fail "the action log does not record $change_id for uid $admin_uid: $rows"
	printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"folder.set-permissions${undo_id}undoof$change_id\"" ||
		fail "the action log does not record the undo $undo_id of $change_id for uid $admin_uid: $rows"
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the changes: $out"
	as_admin "rm -rf '$dir'"

	step 'the protection rules refuse changing the own account, the group sudo and a system path, and leave no journal entry'
	before=$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')
	for refused in "user.lock $admin|you cannot lock yourself" "user.delete $admin|you cannot delete your own account" \
		"user.set-admin $admin off|from yourself" "group.delete sudo|" "group.delete qhost|" "folder.set-permissions /etc $admin $grp read-write read nothing|system path"; do
		args=${refused%%|*}
		want=${refused#*|}
		if out=$(as_admin "sudo qhost --output json host apply $args --confirm 2>&1"); then fail "the rule did not refuse 'host apply $args': $out"; fi
		printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"refused"' || fail "'host apply $args' is not reported as refused: $out"
		[ -z "$want" ] || printf '%s' "$out" | grep -q "$want" || fail "'host apply $args' does not give the reason '$want': $out"
	done
	[ "$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')" = "$before" ] || fail 'a refused change wrote a journal entry'
	[ "$(docker exec "$host" stat -c %a /etc)" = 755 ] || fail 'a refused change touched /etc'
	docker exec "$host" getent group sudo >/dev/null || fail 'the group sudo is gone'

	step 'a user is created with the password on standard input, put in a group, locked and deleted'
	pw=E2e-demo-Pw-7731
	docker exec "$host" sh -c 'id e2e-hostuser >/dev/null 2>&1 && userdel -r e2e-hostuser; getent group e2e-hostgroup >/dev/null && groupdel e2e-hostgroup; rm -rf /home/e2e-hostuser; true'
	out=$(printf '{"password":"%s"}' "$pw" | docker exec -i -u "$admin" "$host" sudo qhost --output json host apply user.create e2e-hostuser --confirm 2>&1) ||
		fail "host apply user.create failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"applied"' || fail "user.create is not reported as applied: $out"
	if printf '%s' "$out" | grep -q "$pw"; then fail "the report of user.create shows the password: $out"; fi
	docker exec "$host" id e2e-hostuser >/dev/null || fail 'the user e2e-hostuser does not exist after user.create'
	out=$(as_admin 'qhost --output json host user e2e-hostuser 2>&1') || fail "qhost host user failed: $out"
	printf '%s' "$out" | grep -q '"name": *"e2e-hostuser"' || fail "qhost host user does not show the new user: $out"
	out=$(as_admin 'sudo qhost --output json host apply group.create e2e-hostgroup --confirm 2>&1') || fail "host apply group.create failed: $out"
	out=$(as_admin 'sudo qhost --output json host apply user.add-groups e2e-hostuser e2e-hostgroup --confirm 2>&1') || fail "host apply user.add-groups failed: $out"
	docker exec "$host" id -nG e2e-hostuser | tr ' ' '\n' | grep -qx e2e-hostgroup || fail "e2e-hostuser is not in e2e-hostgroup: $(docker exec "$host" id -nG e2e-hostuser)"
	out=$(as_admin 'qhost --output json host group e2e-hostgroup 2>&1') || fail "qhost host group failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"members":\[[^]]*"e2e-hostuser"' || fail "qhost host group does not list e2e-hostuser: $out"
	out=$(as_admin 'sudo qhost --output json host apply user.lock e2e-hostuser --confirm 2>&1') || fail "host apply user.lock failed: $out"
	out=$(as_admin 'sudo qhost --output json host user e2e-hostuser 2>&1') || fail "sudo qhost host user failed: $out"
	printf '%s' "$out" | grep -q '"locked": *true' || fail "e2e-hostuser is not shown as locked: $out"
	[ "$(docker exec "$host" passwd -S e2e-hostuser | cut -d ' ' -f 2)" = L ] || fail 'passwd does not report e2e-hostuser as locked'
	out=$(as_admin 'sudo qhost --output json host apply user.delete e2e-hostuser --confirm 2>&1') || fail "host apply user.delete failed: $out"
	if docker exec "$host" id e2e-hostuser >/dev/null 2>&1; then fail 'the user e2e-hostuser exists after user.delete'; fi
	out=$(as_admin 'sudo qhost --output json host apply group.delete e2e-hostgroup --confirm 2>&1') || fail "host apply group.delete failed: $out"
	if docker exec "$host" getent group e2e-hostgroup >/dev/null; then fail 'the group e2e-hostgroup exists after group.delete'; fi
	docker exec "$host" rm -rf /home/e2e-hostuser

	step 'the action log lists every change of the user, and no secret reaches the journal or the action log'
	rows=$(host_audit_rows)
	for action in user.create group.create user.add-groups user.lock user.delete group.delete; do
		printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"$action[0-9]" ||
			fail "the action log does not record $action for uid $admin_uid: $rows"
	done
	if docker exec "$host" grep -rqF "$pw" /var/lib/qhost-host /var/lib/qhost; then fail 'the password reached the journal or the action log'; fi
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the user changes: $out"
	log 'scenario: user e2e-hostuser created, put in e2e-hostgroup, locked and deleted; every change is in the action log with uid '"$admin_uid"
}

# tui_system_tab clicks the sub-tab $1 of the System area on the third line of the screen (header, tab line,
# sub-tab line) and leads the focus into its content.
tui_system_tab() {
	cell=$(tui_screen | awk -v t="$1" 'NR == 3 && (i = index($0, t)) { print i, NR; exit }')
	[ -n "$cell" ] || fail "cannot click the sub-tab '$1', it is not on the sub-tab line: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
	sleep 0.4
	tui_send Down
	sleep 0.3
}

# tui_system_user_create_count prints how many changes user.create of the administrator the action log holds.
tui_system_user_create_count() {
	host_audit_rows | grep -o "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"user\.create[0-9]" | wc -l
}

# scenario_tui_system proves the System area of the interface started without sudo. On Groups the administrator
# creates a group; on Users a user with an initial password in that group. Each change hands the terminal to the
# real sudo: the group through sudo qhost host apply, the user first through sudo -v and then sudo -n with the
# initial password on standard input. qhost host users then shows the user with the group, the password is set and
# must be changed at the first login, and the password shows on no captured screen, in no state of qhost and in no
# system log.
scenario_tui_system() {
	tui_log=$work/tui-system-screens.log
	: >"$tui_log"
	ts_user=e2e-tuiuser
	ts_group=e2e-tuigroup
	ts_start=E2e-tui-Start-4417
	# sudo -v asks for the password of the administrator although the rule of $admin needs none, because the
	# account is also in the group sudo; the scenario answers it as a person would.
	ts_login=E2e-admin-Login-2093
	printf '%s:%s\n' "$admin" "$ts_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	admin_uid=$(docker exec "$host" id -u "$admin")
	docker exec "$host" sh -c "id $ts_user >/dev/null 2>&1 && userdel -r $ts_user; getent group $ts_group >/dev/null && groupdel $ts_group; rm -rf /home/$ts_user; true"
	creates_before=$(tui_system_user_create_count)

	step 'the interface without sudo creates a group on System > Groups through sudo qhost host apply'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Groups
	tui_wait 'MEMBERS' 10
	tui_send a
	tui_wait 'The system gives the group a regular group number.' 10
	tui_type "$ts_group"
	tui_send F2
	tui_wait "Create the group $ts_group." 10
	tui_has 'groupadd' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "Done: Create the group $ts_group." 60
	tui_send Enter
	docker exec "$host" getent group "$ts_group" >/dev/null || fail "the group $ts_group does not exist after the interface created it"

	step 'the interface without sudo creates a user with an initial password in that group, through sudo -v and sudo -n'
	tui_system_tab Users
	tui_wait 'DOCKER' 10
	tui_send a
	tui_wait 'The user must change the initial password at the first login.' 10
	tui_type "$ts_user"
	tui_send Down
	tui_type "$ts_start"
	tui_send Down
	tui_type "$ts_start"
	tui_wait "$ts_group" 10
	tui_click "$ts_group"
	sleep 0.3
	tui_send F2
	tui_wait "Create the user $ts_user" 10
	tui_has "$ts_group" || fail "the commands of the question do not put the user in $ts_group: $(tui_screen)"
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ts_login"
	tui_send Enter
	tui_wait 'Done: Create the user' 60
	tui_send Enter
	i=0
	until tui_screen | grep "$ts_user" | grep -q "$ts_group"; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "the users list does not show $ts_user with $ts_group: $(tui_screen)"
		sleep 0.25
	done
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'qhost host users shows the user with the group; the initial password is set and must be changed'
	out=$(as_admin 'qhost host users 2>&1') || fail "qhost host users failed: $out"
	printf '%s\n' "$out" | grep -F -- "$ts_user" | grep -qF -- "$ts_group" || fail "qhost host users does not show $ts_user with $ts_group: $out"
	out=$(as_admin 'qhost --output json host users 2>&1') || fail "qhost --output json host users failed: $out"
	printf '%s' "$out" | jq -e --arg u "$ts_user" --arg g "$ts_group" '[.. | objects | select(.name? == $u)][0].groups | index($g) != null' >/dev/null ||
		fail "the JSON of qhost host users does not show $ts_user with $ts_group: $out"
	printf '%s' "$ts_start" | docker exec -i "$host" perl -e 'my $p = <STDIN>; open(my $f, "<", "/etc/shadow") or exit 2;
		while (<$f>) { my @r = split /:/; exit(crypt($p, $r[1]) eq $r[1] && $r[2] eq "0" ? 0 : 1) if $r[0] eq $ARGV[0] } exit 3' "$ts_user" ||
		fail "the initial password of $ts_user did not arrive over standard input, or it need not be changed at the first login"
	[ "$(tui_system_user_create_count)" -eq $((creates_before + 1)) ] ||
		fail "the action log does not hold one more user.create of uid $admin_uid behind sudo: $(host_audit_rows)"

	step 'the initial password shows on no captured screen, in no state of qhost and in no system log'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$ts_start" "$tui_log"; then fail 'the initial password shows on a captured screen'; fi
	if grep -qF -- "$ts_login" "$tui_log"; then fail 'the password typed for sudo shows on a captured screen'; fi
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	if docker exec "$host" grep -rqF -- "$ts_start" /var/lib/qhost-host /var/lib/qhost; then fail 'the initial password reached the state of qhost'; fi
	if docker exec "$host" sh -c 'journalctl --no-pager -q 2>/dev/null' | grep -qF -- "$ts_start"; then fail 'the initial password reached the system log'; fi

	docker exec "$host" sh -c "userdel -r $ts_user; groupdel $ts_group; rm -rf /home/$ts_user; true"
	log "scenario: the interface without sudo created $ts_group and $ts_user in it through sudo; qhost host users shows both"
}

# scenario_tui_changes proves System > Folders, Changes and Log of the interface started without sudo. The
# administrator takes the permissions of everyone else from a test folder through sudo qhost host apply; Changes reads
# the journal through sudo qhost host journal on r and lists the change; its undo through sudo qhost host undo brings
# back owner, group and mode, as stat shows; Log shows the change of the host under an intact chain of hashes. The
# rule of $admin lets sudo run a command without the password, so none is asked here.
scenario_tui_changes() {
	tui_log=$work/tui-changes-screens.log
	: >"$tui_log"
	tc_dir=/home/$admin/e2e-tui-folder
	as_admin "rm -rf '$tc_dir' && mkdir '$tc_dir' && chmod 0755 '$tc_dir' && touch '$tc_dir/note.txt'" || fail 'cannot make the test folder'
	tc_before=$(as_admin "stat -c %U:%G:%a '$tc_dir'")
	[ "${tc_before##*:}" = 755 ] || fail "the test folder starts as $tc_before, want the mode 755"

	step 'the interface without sudo takes the permissions of everyone else from a folder on System > Folders'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Folders
	tui_wait 'to choose a folder' 10
	tui_send Enter
	tui_wait 'Type the absolute path of the folder.' 10
	tui_type "$tc_dir"
	tui_send Enter
	tui_wait "Owner $admin may: read and write" 10
	tui_has 'Everyone else may: read' || fail "the folder does not show that everyone else may read: $(tui_screen)"
	tui_send e
	tui_wait 'Everyone else may' 10
	tui_send Down Down Down Down
	sleep 0.2
	tui_send Left
	sleep 0.2
	tui_send F2
	tui_wait 'everyone else nothing.' 10
	tui_has 'chmod' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "Done: Change $tc_dir" 60
	tui_send Enter
	[ "$(as_admin "stat -c %a '$tc_dir'")" = 750 ] || fail "the folder has the mode $(as_admin "stat -c %a '$tc_dir'") after the change, want 750"

	step 'System > Changes reads the journal through sudo on r and lists the change of the folder'
	tui_system_tab Changes
	tui_wait 'only root can read' 10
	tui_send r
	tui_wait "Change $tc_dir" 60
	i=0
	until tui_screen | grep "^> " | grep -q "Change $tc_dir"; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the newest change is not the change of $tc_dir under the cursor: $(tui_screen)"
		sleep 0.25
	done
	tui_screen | grep "^> " | grep -q possible || fail "the change of $tc_dir offers no undo: $(tui_screen)"
	tui_has 'Undo: Restore the previous owner, group and permissions' || fail "the undo of the change is not shown: $(tui_screen)"

	step 'the undo through sudo qhost host undo restores owner, group and mode of the folder'
	tui_send Enter
	tui_wait 'Undo the change: Change' 10
	# The dialog wraps its text, so the sentence may stand on two lines.
	{ tui_has 'The undo itself' && tui_has 'cannot be undone.'; } || fail "the question does not say that the undo cannot be undone: $(tui_screen)"
	tui_has 'chown -h --' || fail "the question does not show the commands of the undo: $(tui_screen)"
	tui_send y
	tui_wait 'Done: Undo the change' 60
	tui_send Enter
	tc_after=$(as_admin "stat -c %U:%G:%a '$tc_dir'")
	[ "$tc_after" = "$tc_before" ] || fail "the undo left the folder as $tc_after, want $tc_before"
	i=0
	until tui_screen | grep "Change $tc_dir" | grep -q undone; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the undone change is not shown as undone: $(tui_screen)"
		sleep 0.25
	done
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s' "$journal" | tr -d ' \n' | grep -q '"result":"undone"' || fail "the journal does not mark the change undone: $journal"

	step 'System > Log shows the change of the host under an intact chain of hashes'
	tui_system_tab Log
	tui_wait 'Chain of hashes intact' 30
	tui_has 'host.change' || fail "the log does not show the change of the host: $(tui_screen)"
	tui_has 'folder.set-permissions' || fail "the log does not name the action of the change: $(tui_screen)"
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the changes from the interface: $out"
	as_admin "rm -rf '$tc_dir'"
	log "scenario: the interface without sudo changed $tc_dir to 750, listed the change on Changes and undid it back to $tc_before"
}

# scenario_tui_host_proposal proves a proposal of the host from submission to undo. An administrator without sudo
# proposes a user in a group with a public SSH key through qhost proposal; the proposal is listed and shown with its
# sentence and the key fingerprint, and a process of qhost-deploy is turned away from the proposals of the host.
# The interface without sudo approves it on System > Proposals: the form asks for the initial password, sudo -v asks
# for the password of the administrator, and sudo qhost host apply --proposal creates the user. The user, the group and
# authorized_keys are checked, the journal lists the change, the proposal stands as applied, and the undo on
# System > Changes removes the user again. The initial password shows in no output, no captured screen, no state of
# qhost and no system log.
scenario_tui_host_proposal() {
	tui_log=$work/tui-host-proposal-screens.log
	: >"$tui_log"
	leaks=$work/host-proposal-outputs.log
	: >"$leaks"
	hp_user=e2e-propuser
	hp_group=e2e-propgroup
	hp_start=E2e-prop-Start-5529
	hp_login=E2e-admin-Login-4471
	hp_keydir=/home/$admin/e2e-propkey
	printf '%s:%s\n' "$admin" "$hp_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	admin_uid=$(docker exec "$host" id -u "$admin")
	docker exec "$host" sh -c "id $hp_user >/dev/null 2>&1 && userdel -r $hp_user; getent group $hp_group >/dev/null && groupdel $hp_group; rm -rf /home/$hp_user; true"
	as_admin "rm -rf '$hp_keydir' && mkdir '$hp_keydir' && ssh-keygen -q -t ed25519 -N '' -C e2e-propkey@example.test -f '$hp_keydir/key'" ||
		fail 'cannot generate the test key'
	hp_key=$(as_admin "cat '$hp_keydir/key.pub'")
	hp_blob=$(printf '%s' "$hp_key" | cut -d ' ' -f 2)
	hp_fp=$(as_admin "ssh-keygen -lf '$hp_keydir/key.pub'" | cut -d ' ' -f 2)
	[ -n "$hp_blob" ] && [ -n "$hp_fp" ] || fail "the test key is not usable: $hp_key"
	out=$(as_admin "sudo qhost --output json host apply group.create $hp_group --confirm 2>&1") || fail "host apply group.create failed: $out"
	creates_before=$(tui_system_user_create_count)

	step 'an administrator without sudo proposes a user with a group and an SSH key, and it is listed and shown'
	content=$(jq -nc --arg u "$hp_user" --arg g "group=$hp_group" --arg k "$hp_key" '{action: "user.create", args: [$u, $g], keys: [$k]}')
	out=$(printf '%s' "$content" | as_admin 'qhost --output json proposal submit --kind host-change 2>&1') ||
		fail "qhost proposal submit --kind host-change without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	hp_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(h-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$hp_id" ] || fail "qhost proposal submit printed no id of the host service: $out"
	printf '%s' "$out" | grep -q '"state": *"open"' || fail "the new proposal is not open: $out"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s\n' "$out" | grep -F -- "$hp_id" | grep -q 'host-change' || fail "qhost proposal list does not show $hp_id as host-change: $out"
	out=$(as_admin "qhost proposal show $hp_id 2>&1") || fail "qhost proposal show without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s' "$out" | grep -qF -- "Create the user $hp_user" || fail "qhost proposal show does not give the sentence of the change: $out"
	printf '%s' "$out" | grep -qF -- "$hp_fp" || fail "qhost proposal show does not show the key fingerprint $hp_fp: $out"
	[ "$(prop_state "$hp_id")" = open ] || fail "the proposal $hp_id is not open"
	if docker exec "$host" id "$hp_user" >/dev/null 2>&1; then fail 'submitting the proposal created the user'; fi

	step 'a process of qhost-deploy is turned away from the proposals of the host'
	if out=$(docker exec -u qhost-deploy "$host" qhost proposal list 2>&1 </dev/null); then
		fail "qhost-deploy listed proposals while the host service holds one: $out"
	fi
	if printf '%s' "$out" | grep -qF -e "$hp_id" -e "$hp_user" -e "$hp_blob" -e e2e-propkey; then fail "the refusal for qhost-deploy shows data of the proposal: $out"; fi
	if out=$(docker exec -u qhost-deploy "$host" qhost proposal show "$hp_id" 2>&1 </dev/null); then
		fail "qhost-deploy shows the proposal of the host: $out"
	fi
	if printf '%s' "$out" | grep -qF -e "$hp_user" -e "$hp_blob" -e e2e-propkey; then fail "the refusal for qhost-deploy shows data of the proposal: $out"; fi

	step 'the interface without sudo approves the proposal on System > Proposals, asks for the initial password and creates the user'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Proposals
	tui_wait "$hp_id" 15
	tui_send Enter
	tui_wait "Proposal $hp_id from $admin" 10
	tui_has 'The change' || fail "the view does not show the change: $(tui_screen)"
	tui_has "Create the user $hp_user" || fail "the view does not show the sentence: $(tui_screen)"
	tui_has 'Commands, as typed by hand' || fail "the view does not show the commands: $(tui_screen)"
	tui_has 'Proposed SSH keys' || fail "the view does not show the proposed keys: $(tui_screen)"
	tui_has 'e2e-propkey@example.test' || fail "the view does not show the comment of the key: $(tui_screen)"
	tui_send y
	tui_wait 'The user must change the initial password at the first login.' 10
	tui_type "$hp_start"
	tui_send Down
	tui_type "$hp_start"
	tui_send F2
	tui_wait "Apply proposal $hp_id to the host" 10
	tui_has 'useradd' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	# sudo -v asks for the password of the administrator; a still valid ticket would skip the prompt.
	i=0
	while :; do
		screen=$(tui_screen)
		case $screen in
		*"[sudo] password for $admin"*)
			tui_type "$hp_login"
			tui_send Enter
			break
			;;
		*"Done: Apply proposal $hp_id"*) break ;;
		esac
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail "neither the password prompt of sudo nor the result appears: $screen"
		sleep 0.25
	done
	tui_wait "Done: Apply proposal $hp_id" 60
	tui_has 'Journal entry' || fail "the result does not name the journal entry: $(tui_screen)"
	tui_send Enter

	step 'the user, the group and authorized_keys are in place, the journal shows the change and the proposal stands as applied'
	docker exec "$host" id "$hp_user" >/dev/null || fail "the user $hp_user does not exist after the approval"
	docker exec "$host" id -nG "$hp_user" | tr ' ' '\n' | grep -qx "$hp_group" || fail "$hp_user is not in $hp_group: $(docker exec "$host" id -nG "$hp_user")"
	[ "$(docker exec "$host" cat "/home/$hp_user/.ssh/authorized_keys" | cut -d ' ' -f 2 | grep -cxF -- "$hp_blob")" = 1 ] ||
		fail "authorized_keys of $hp_user does not hold the proposed key once: $(docker exec "$host" cat "/home/$hp_user/.ssh/authorized_keys" 2>&1)"
	[ "$(docker exec "$host" stat -c %U "/home/$hp_user/.ssh/authorized_keys")" = "$hp_user" ] || fail "authorized_keys of $hp_user is not owned by the user"
	printf '%s' "$hp_start" | docker exec -i "$host" perl -e 'my $p = <STDIN>; open(my $f, "<", "/etc/shadow") or exit 2;
		while (<$f>) { my @r = split /:/; exit(crypt($p, $r[1]) eq $r[1] && $r[2] eq "0" ? 0 : 1) if $r[0] eq $ARGV[0] } exit 3' "$hp_user" ||
		fail "the initial password of $hp_user did not arrive, or it need not be changed at the first login"
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s\n' "$journal" >>"$leaks"
	hp_change=$(printf '%s' "$journal" | jq -r --arg u "$hp_user" '[.. | objects | select(.action? == "user.create" and .result? == "applied" and (.sentence? // "" | contains($u)))][0].id // empty')
	[ -n "$hp_change" ] || fail "the journal has no entry of user.create for $hp_user: $journal"
	printf '%s' "$journal" | jq -e --arg id "$hp_change" --argjson uid "$admin_uid" \
		'[.. | objects | select(.id? == $id)][0] | .result == "applied" and .reversible == true and .sudo_uid == $uid' >/dev/null ||
		fail "the journal entry $hp_change is not applied, reversible and attributed to uid $admin_uid: $journal"
	[ "$(tui_system_user_create_count)" -eq $((creates_before + 1)) ] ||
		fail "the action log does not hold one more user.create of uid $admin_uid behind sudo: $(host_audit_rows)"
	[ "$(prop_state "$hp_id")" = applied ] || fail "the proposal $hp_id is $(prop_state "$hp_id"), want applied"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list failed after the approval: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s\n' "$out" | grep -F -- "$hp_id" | grep -q 'applied' || fail "qhost proposal list does not show $hp_id as applied: $out"

	step 'the undo on System > Changes removes the user again'
	tui_system_tab Changes
	tui_wait 'only root can read' 10
	tui_send r
	tui_wait "Create the user $hp_user" 60
	i=0
	until tui_screen | grep "^> " | grep -q "Create the user $hp_user"; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the newest change is not the creation of $hp_user under the cursor: $(tui_screen)"
		sleep 0.25
	done
	tui_screen | grep "^> " | grep -q possible || fail "the creation of $hp_user offers no undo: $(tui_screen)"
	tui_send Enter
	tui_wait 'Undo the change: ' 10
	tui_send y
	tui_wait 'Done: Undo the change' 60
	tui_send Enter
	if docker exec "$host" id "$hp_user" >/dev/null 2>&1; then fail "the user $hp_user exists after the undo"; fi
	if docker exec "$host" test -e "/home/$hp_user"; then fail "the home of $hp_user exists after the undo"; fi
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s\n' "$journal" >>"$leaks"
	printf '%s' "$journal" | jq -e --arg id "$hp_change" '[.. | objects | select(.id? == $id)][0] | .result == "undone" and .reversible == false' >/dev/null ||
		fail "the journal does not close the undone entry $hp_change: $journal"
	[ "$(prop_state "$hp_id")" = applied ] || fail "the undo changed the state of the proposal $hp_id to $(prop_state "$hp_id")"
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the proposal: $out"

	step 'the initial password shows in no output, no captured screen, no state of qhost and no system log'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$hp_start" "$tui_log" "$leaks"; then fail 'the initial password shows on a captured screen or in an output'; fi
	if grep -qF -- "$hp_login" "$tui_log" "$leaks"; then fail 'the password typed for sudo shows on a captured screen or in an output'; fi
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	if docker exec "$host" grep -rqF -- "$hp_start" /var/lib/qhost-host /var/lib/qhost; then fail 'the initial password reached the state of qhost'; fi
	if docker exec "$host" sh -c 'journalctl --no-pager -q 2>/dev/null' | grep -qF -- "$hp_start"; then fail 'the initial password reached the system log'; fi

	docker exec "$host" sh -c "id $hp_user >/dev/null 2>&1 && userdel -r $hp_user; getent group $hp_group >/dev/null && groupdel $hp_group; rm -rf /home/$hp_user; true"
	as_admin "rm -rf '$hp_keydir'"
	log "scenario: $hp_id proposed $hp_user in $hp_group with a key, the interface approved it as $hp_change and the undo removed the user"
}

# service_log_warn_count prints how many entries of the service log at the level warn or above say that the window
# could not read the stacks, read as the administrator without sudo.
service_log_warn_count() {
	as_admin 'qhost --output json service log --level warn --tail 0' | grep -c 'maintenance window: the stacks cannot be read' || true
}

# service_log_warned succeeds when the service log has more such warnings than $warn_before.
service_log_warned() {
	[ "$(service_log_warn_count)" -gt "$warn_before" ]
}

# scenario_service_log proves the service log: a maintenance window that opens while no stacks source is bound
# writes a warning, and the read command and the terminal interface show it. It runs right after the installation,
# before a source is bound; the caller moves the window away afterwards.
scenario_service_log() {
	step 'a window without a bound source warns, and qhost service log shows the warning without sudo'
	warn_before=$(service_log_warn_count)
	# --start has minute granularity: one minute ahead opens the window within the next minute.
	start=$(date -u -d '+1 minute' +%H:%M)
	out=$(qd --output json window set --days daily --zone UTC --start "$start" --duration 5m) || fail "window set failed: $out"
	wait_for 'the warning of the window in the service log' 150 service_log_warned
	line=$(as_admin 'qhost --output json service log --level warn --tail 0' | grep 'maintenance window: the stacks cannot be read' | tail -n 1)
	printf '%s' "$line" | grep -q '"level": *"warn"' || fail "the entry is not at the level warn: $line"
	printf '%s' "$line" | grep -q '"message": *"maintenance window: the stacks cannot be read; nothing is checked or updated"' ||
		fail "the entry has not the expected message: $line"
	printf '%s' "$line" | grep -q 'no stacks repository is bound' || fail "the entry does not name the cause: $line"
	text=$(as_admin 'qhost service log --level warn --tail 0') || fail "qhost service log failed: $text"
	printf '%s\n' "$text" | grep -q ' WARN  maintenance window: the stacks cannot be read; nothing is checked or updated error="no stacks repository is bound"$' ||
		fail "the text output does not show the warning line: $text"
	[ -z "$(as_admin 'qhost service log --level error --tail 0' | grep 'stacks cannot be read')" ] || fail 'the warning shows at the level error'
	log "scenario: service log: $line"

	step 'the terminal interface shows the same line in the service log of the Runs area'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 3
	tui_wait 'No runs recorded.' 10
	tui_send v
	tui_wait 'Service log' 10
	tui_wait 'WARN maintenance window: the stacks cannot be read; nothing is checked or updated' 20
	log 'scenario: the service log of the interface shows the warning:'
	tui_excerpt 'Service log' 6
	tui_send Escape
	tui_wait 'No runs recorded.' 10
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
}

# scenario_read_access proves the read access without sudo: an administrator of the group qhost reads status,
# stacks, runs with their logs and container logs, and a change is refused without root and has no effect. A user
# of the group without sudo and a user outside the group get no data. It needs runs and running containers.
scenario_read_access() {
	step 'an administrator of the group qhost reads stacks, runs and container logs without sudo'
	run=$(newest_run demo-stack)
	[ -n "$run" ] || fail 'demo-stack has no recorded run to read'
	name=$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-stack --format '{{.Names}}' | head -n 1)
	[ -n "$name" ] || fail 'no container of demo-stack runs'
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost status without sudo does not show demo-stack: $out"
	out=$(as_admin 'qhost stack list 2>&1') || fail "qhost stack list without sudo failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost stack list without sudo does not show demo-stack: $out"
	out=$(as_admin 'qhost stack runs 2>&1') || fail "qhost stack runs without sudo failed: $out"
	printf '%s' "$out" | grep -q "$run" || fail "qhost stack runs without sudo does not show the run $run: $out"
	out=$(as_admin "qhost stack log $run 2>&1") || fail "qhost stack log without sudo failed: $out"
	printf '%s' "$out" | grep -q ' result ' || fail "qhost stack log without sudo does not show the run $run: $out"
	out=$(as_admin "qhost container logs $name --tail 0 2>&1") || fail "qhost container logs without sudo failed: $out"
	printf '%s' "$out" | grep -q 'demo started' || fail "qhost container logs without sudo shows no data: $out"
	if printf '%s' "$out" | grep -qF -- not-a-real-secret-e2e; then fail 'a secret value shows in the container logs without sudo'; fi
	log "scenario: $admin read status, stack list, runs, the log of run $run and container logs without sudo"

	step 'a change without sudo is refused and has no effect'
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	if out=$(as_admin 'qhost stack deploy demo-stack 2>&1'); then fail "qhost stack deploy without sudo succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of a change without sudo is unclear: $out"
	sleep 2
	[ -z "$(stack_field demo-stack active_run)" ] || fail "qhost stack deploy without sudo started a run: $(stack_field demo-stack active_run)"
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')" = "$runs_before" ] || fail 'qhost stack deploy without sudo recorded a run'

	step 'the deploy channel admits root and an administrator of the group qhost; a user of the group without sudo and a user outside the group get no data'
	out=$(qd stack list) || fail "qhost stack list as root failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost stack list as root does not show demo-stack: $out"
	ensure_user e2e-viewer
	ensure_user e2e-outsider
	docker exec "$host" usermod -aG qhost e2e-viewer || fail 'cannot add e2e-viewer to the group qhost'
	if docker exec "$host" id -nG e2e-viewer | tr ' ' '\n' | grep -qx sudo; then fail 'e2e-viewer is in the group sudo'; fi
	if docker exec "$host" id -nG e2e-outsider | tr ' ' '\n' | grep -qx qhost; then fail 'e2e-outsider is in the group qhost'; fi
	for cmd in 'qhost status' 'qhost stack list' 'qhost stack runs' "qhost stack log $run" "qhost container logs $name"; do
		if out=$(as_user e2e-viewer "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-viewer: $out"; fi
		printf '%s' "$out" | grep -q 'reserved for administrators' || fail "unexpected refusal of '$cmd' for e2e-viewer: $out"
		if printf '%s' "$out" | grep -q 'demo-stack\|demo-cache\|demo started'; then fail "'$cmd' shows data to e2e-viewer: $out"; fi
		if out=$(as_user e2e-outsider "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-outsider: $out"; fi
		printf '%s' "$out" | grep -q 'may not use the qhost service channel' || fail "unexpected refusal of '$cmd' for e2e-outsider: $out"
		if printf '%s' "$out" | grep -q 'demo-stack\|demo-cache\|demo started'; then fail "'$cmd' shows data to e2e-outsider: $out"; fi
	done
	log 'scenario: the deploy channel served root and the administrator; e2e-viewer (group qhost, no sudo) was refused as reserved for administrators and e2e-outsider (no group) was turned away at the channel'
}

# scenario_diagnose proves qhost diagnose as an administrator without sudo: the package is a new file of mode 0600
# owned by the administrator, holds all twenty files with the log of a failed run, the service log and the
# containers, and holds neither the stack secret nor a registry token. It needs a failed run and a secret of demo-stack.
scenario_diagnose() {
	step 'qhost diagnose writes a package for an administrator without sudo that holds no secret value'
	diag_secret=not-a-real-secret-e2e
	token=diag-registry-token-e2e
	cred=registry.$tui_registry_host
	home=$(as_admin 'echo "$HOME"')
	file=$home/diag-e2e.tar.gz
	as_admin 'rm -rf ~/diag-e2e.tar.gz ~/diag-e2e'
	out=$(qd_in "$token" credential set "$cred") || fail "credential set failed: $out"
	failed_run=$(qd stack runs | grep 'rolled-back' | grep -m 1 -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	[ -n "$failed_run" ] || fail "there is no failed run to find in the package: $(qd stack runs)"

	out=$(as_admin "qhost diagnose --out $file 2>&1") || fail "qhost diagnose without sudo failed: $out"
	printf '%s' "$out" | grep -qF -- "$file" || fail "qhost diagnose does not name the full path $file: $out"
	printf '%s' "$out" | grep -qF 'holds the host name and paths' || fail "qhost diagnose gives no notice about the host name and paths: $out"
	[ "$(as_admin "stat -c %a:%U $file")" = "600:$admin" ] || fail "the package has the wrong mode or owner: $(as_admin "stat -c %a:%U $file")"
	names=$(as_admin "tar -tzf $file") || fail 'the package is not a readable archive'
	[ "$(printf '%s\n' "$names" | grep -c .)" = 20 ] || fail "the package does not hold 20 files: $names"
	for part in manifest version services sources stacks check runs run-logs service-log containers; do
		for ext in txt json; do
			printf '%s\n' "$names" | grep -qx "$part.$ext" || fail "the package lacks $part.$ext: $names"
		done
	done
	as_admin "mkdir ~/diag-e2e && tar -xzf $file -C ~/diag-e2e" || fail 'the package cannot be unpacked'
	as_admin "grep -qF 'result rolled-back' ~/diag-e2e/run-logs.txt && grep -qF '$failed_run' ~/diag-e2e/run-logs.txt" ||
		fail "run-logs.txt holds no log of the failed run $failed_run"
	as_admin "grep -q '^20[0-9-]*T' ~/diag-e2e/service-log.txt" || fail 'service-log.txt has no entry'
	ct=$(docker exec "$host" docker ps --format '{{.Names}}' | head -n 1)
	[ -n "$ct" ] || fail 'no container runs'
	as_admin "grep -qF '$ct' ~/diag-e2e/containers.txt" || fail "containers.txt does not name $ct"
	for value in "$diag_secret" "$token"; do
		if as_admin "grep -rqF -- '$value' ~/diag-e2e"; then fail 'a secret value or a registry token shows in the package'; fi
	done
	log "scenario: qhost diagnose wrote $file without sudo, with 20 files, the log of $failed_run and no secret value"

	step 'a second qhost diagnose with the same --out fails and leaves the file unchanged'
	sum=$(as_admin "sha256sum $file")
	if out=$(as_admin "qhost diagnose --out $file 2>&1"); then fail "qhost diagnose replaced an existing file: $out"; fi
	[ "$(as_admin "sha256sum $file")" = "$sum" ] || fail 'a refused qhost diagnose changed the file'

	step 'qhost diagnose without --out writes into the folder qhost of the administrator'
	out=$(as_admin 'qhost diagnose 2>&1') || fail "qhost diagnose without --out failed: $out"
	default=$(printf '%s' "$out" | grep -m 1 -o "$home/qhost/qhost-diagnose-[^ ]*\.tar\.gz")
	[ -n "$default" ] || fail "qhost diagnose without --out names no file in $home/qhost: $out"
	[ "$(as_admin "stat -c %a:%U $default")" = "600:$admin" ] || fail "the default package has the wrong mode or owner: $(as_admin "stat -c %a:%U $default")"
	as_admin "rm -rf ~/diag-e2e $file $default"
	out=$(qd credential delete "$cred") || fail "credential delete failed: $out"
	log "scenario: qhost diagnose refused to replace a file and wrote its default file $default"
}

# scenario_proposals proves that an administrator of the group qhost submits a proposal for a stack folder without
# sudo, sees it with list and show and withdraws it, and that the proposed folder stays untouched and unbound.
scenario_proposals() {
	step 'an administrator of the group qhost submits, lists, shows and withdraws a proposal without sudo'
	folder=$(as_admin 'echo $HOME')/proposed-stacks
	as_admin "rm -rf $folder && mkdir -p $folder/demo && printf 'services: {}\\n' > $folder/demo/compose.yaml"
	folder_before=$(as_admin "find $folder -printf '%p %y %s %T@\\n' | sort")
	[ -n "$folder_before" ] || fail 'cannot read the proposed folder'
	out=$(printf '{"path":"%s","url":"ssh://git.example.test/proposed.git"}' "$folder" | as_admin 'qhost --output json proposal submit --kind source-add' 2>&1) ||
		fail "qhost proposal submit without sudo failed: $out"
	id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$id" ] || fail "qhost proposal submit printed no id: $out"
	printf '%s' "$out" | grep -q '"state": *"open"' || fail "the new proposal is not open: $out"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list without sudo failed: $out"
	printf '%s' "$out" | grep -q -- "$id" || fail "qhost proposal list does not show $id: $out"
	out=$(as_admin "qhost proposal show $id 2>&1") || fail "qhost proposal show without sudo failed: $out"
	printf '%s' "$out" | grep -qF -- "$folder" || fail "qhost proposal show does not show the folder $folder: $out"
	out=$(as_admin "qhost --output json proposal withdraw $id 2>&1") || fail "qhost proposal withdraw without sudo failed: $out"
	printf '%s' "$out" | grep -q '"state": *"withdrawn"' || fail "the proposal is not withdrawn: $out"
	out=$(as_admin "qhost --output json proposal show $id 2>&1") || fail "qhost proposal show after the withdrawal failed: $out"
	printf '%s' "$out" | grep -q '"state": *"withdrawn"' || fail "the proposal does not show as withdrawn: $out"
	[ "$(as_admin "find $folder -printf '%p %y %s %T@\\n' | sort")" = "$folder_before" ] || fail "the proposed folder $folder changed"
	out=$(as_admin 'qhost source list 2>&1') || fail "qhost source list without sudo failed: $out"
	if printf '%s' "$out" | grep -qF -- "$folder"; then fail "the proposed folder is bound: $out"; fi
	as_admin "rm -rf $folder"
	log "scenario: proposal $id was submitted, listed, shown and withdrawn without sudo; the folder is unchanged and not bound"
}

# scenario_compose_integration runs the Docker integration tests of the deployer inside the host. The test
# binary is built here and refuses to run without the marker of the simulated host.
scenario_compose_integration() {
	step 'Docker integration tests of the deployer run on the host'
	(cd "$root" && GOOS=linux GOARCH="$(docker exec "$host" dpkg --print-architecture)" CGO_ENABLED=0 \
		go test -c -tags dockerintegration -o "$work/compose.test" ./internal/compose) || fail 'cannot build the integration tests'
	docker cp "$work/compose.test" "$host:/tmp/compose.test"
	out=$(docker exec -e QHOST_E2E_HOST=1 -e "QHOST_E2E_IMAGE=registry.$domain/demo/app:1.0.0" "$host" \
		/tmp/compose.test -test.v -test.count=1 -test.run '^TestDocker' 2>&1 </dev/null) || {
		printf '%s\n' "$out"
		docker exec "$host" rm -f /tmp/compose.test
		fail 'the Docker integration tests failed'
	}
	docker exec "$host" rm -f /tmp/compose.test
	for name in TestDockerHealthyStackIsDeployedAndChecked TestDockerUnhealthyStackFailsTheCheck TestDockerDeployWithoutSecretsFile; do
		printf '%s' "$out" | grep -q -- "--- PASS: $name" || fail "$name did not pass: $out"
	done
	if printf '%s' "$out" | grep -q -- '--- SKIP'; then fail "an integration test was skipped: $out"; fi
	log "scenario: Docker integration tests passed: $(printf '%s' "$out" | grep -E '^(--- |ok|PASS)' | tr '\n' ' ')"
}

# stack_field prints one flat field of a stack from qhost status (empty when it is absent).
stack_field() {
	qd --output json status | sed -n "s/.*\"stack\\.$1\\.$2\": *\"\\([^\"]*\\)\".*/\\1/p"
}

# stack_settled succeeds when the stack has no run in progress and its last run ended with the given result.
stack_settled() {
	[ -z "$(stack_field "$1" active_run)" ] && [ "$(stack_field "$1" last_run)" = "$2" ]
}

# stack_idle succeeds when the stack has no run in progress and has finished at least one.
stack_idle() {
	[ -z "$(stack_field "$1" active_run)" ] && [ -n "$(stack_field "$1" last_run)" ]
}

# run_finished succeeds when the log of the run has its result line and the stack has no run in progress.
run_finished() {
	[ -z "$(stack_field "$1" active_run)" ] && qd stack log "$2" | grep -q ' result '
}

has_failed_commit_finding() {
	stack_field demo-cache findings | grep -q failed-commit
}

# restart_service restarts the deploy service, which makes the engine poll at once.
restart_service() {
	docker exec "$host" systemctl restart qhost-deploy || fail 'cannot restart the deploy service'
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the deploy service to answer' 20 qd status
}

# status_field prints one top-level field of qhost status.
status_field() {
	qd --output json status | sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p"
}

# poll_done succeeds when the service has polled since it started.
poll_done() {
	started=$(status_field started_at) polled=$(status_field last_poll)
	[ -n "$started" ] && [ -n "$polled" ] && [ "$(printf '%s\n%s\n' "$polled" "$started" | sort | head -n 1)" = "$started" ]
}

# stack_healthy succeeds when the stack runs and every container with a healthcheck is healthy.
stack_healthy() {
	[ -n "$(docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" --filter health=healthy)" ] &&
		[ -z "$(docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" --filter health=unhealthy --filter health=starting)" ]
}

# log_steps prints the run's own steps of a run log in order, joined by commas; the sub-steps of the backup
# and the restore are left out.
log_steps() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* step //p' |
		grep -x -e stop -e backup -e pull -e up -e check -e rollback -e restore -e revert -e push -e fast-forward |
		tr '\n' ',' | sed 's/,$//'
}

rolled_back_steps=stop,backup,pull,up,check,rollback,stop,restore,revert,push,fast-forward,up,check
# A stack without backup rules is not stopped for a backup, so its run starts at the backup step.
rolled_back_plain_steps=backup,pull,up,check,rollback,stop,restore,revert,push,fast-forward,up,check
# The rollback-failed run is one of demo-cache, which has no backup rules either.
rollback_failed_steps=backup,pull,up,check,rollback,stop,restore,revert,push,stop

# newest_run prints the id of the newest recorded run of a stack (empty when there is none).
newest_run() {
	# The error column can name another run, so only the first id of the newest line counts.
	qd stack runs "$1" | grep -m 1 -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}' | head -n 1
}

# next_run waits for a run of the stack other than the run id $2, active or already finished, and prints its
# id. Recorded runs are listed only once they have ended, so an active run is read from the status.
next_run() {
	i=0
	while :; do
		id=$(stack_field "$1" active_run)
		[ -n "$id" ] || id=$(newest_run "$1")
		if [ -n "$id" ] && [ "$id" != "$2" ]; then
			printf '%s' "$id"
			return 0
		fi
		i=$((i + 1))
		[ "$i" -lt 400 ] || return 1
		sleep 0.3
	done
}

# admin_commit changes a file of the admin clone with a sed expression and pushes it as a new commit.
admin_commit() {
	as_admin "cd ~/stacks && git pull -q --ff-only && sed -i '$2' $1 &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am '$3' && git push -q origin main"
}

# stack_images prints the images of the running containers of a Compose project on the host.
stack_images() {
	docker exec "$host" docker ps --filter "label=com.docker.compose.project=$1" --format '{{.Image}}'
}

# qd_in runs qd with standard input from the first argument.
qd_in() {
	input=$1
	shift
	printf '%s' "$input" | docker exec -i "$host" qhost "$@" 2>&1
}

# hook_seen succeeds when the webhook receiver recorded a signed request to the path $1 with the event $2 and,
# if given, the stack $3 in its body.
hook_seen() {
	stack_pattern='"body"'
	[ -z "${3:-}" ] || stack_pattern="\\\"stack\\\":\\\"$3\\\""
	docker exec "$prefix-hooks" cat /data/requests.jsonl 2>/dev/null | grep -F "\"path\":\"$1\"" |
		grep -F "\"X-Qhost-Event\":\"$2\"" | grep -F '"X-Qhost-Signature":"' | grep -qF -- "$stack_pattern"
}

# hook_has succeeds when the webhook receiver recorded a signed request to the path $1 for the event $2 whose
# body contains every further fixed string.
hook_has() {
	lines=$(docker exec "$prefix-hooks" cat /data/requests.jsonl 2>/dev/null | grep -F "\"path\":\"$1\"" |
		grep -F "\"X-Qhost-Event\":\"$2\"" | grep -F '"X-Qhost-Signature":"') || return 1
	shift 2
	for part in "$@"; do
		lines=$(printf '%s\n' "$lines" | grep -F -- "$part") || return 1
	done
}

# journal prints the log of the deploy service.
journal() {
	docker exec "$host" journalctl -u qhost-deploy --no-pager -o cat
}

# mail_seen succeeds when the mail server recorded a message containing the fixed string $1.
mail_seen() {
	docker exec "$prefix-mail" cat /data/messages.jsonl 2>/dev/null | grep -F -- "$1" >/dev/null
}

hook_url=https://hooks.$domain/notify
hook_signing_value=demo-signing-value-e2e
slack_url_value=https://hooks.$domain/slack/T000/B000/demo-slack-path-e2e
bot_credential=123456:demo-bot-credential-e2e
mail_login_value=demo-mail-login-e2e

scenario_deploy() {
	secret=not-a-real-secret-e2e

	step 'the runs started by repo set end; secrets.env is not there yet'
	wait_for 'the first run of demo-stack' 120 stack_idle demo-stack
	wait_for 'the first run of demo-cache' 120 stack_idle demo-cache
	log "scenario: first runs: demo-stack $(stack_field demo-stack last_run), demo-cache $(stack_field demo-cache last_run)"

	step 'demo-plain (docker-compose.yml, name: demo-shop, no secrets.env) is deployed as project demo-shop'
	wait_for 'the first run of demo-shop' 120 stack_idle demo-shop
	[ "$(stack_images demo-shop)" = "registry.$domain/demo/app:1.0.0" ] || fail "demo-shop does not run the image 1.0.0: $(stack_images demo-shop)"
	[ -z "$(stack_images demo-plain)" ] || fail "a project named after the folder demo-plain runs: $(stack_images demo-plain)"
	qd --output json stack list | grep -q -- demo-shop || fail 'stack list does not name demo-shop'
	if docker exec "$host" test -e /opt/stacks/demo-plain/secrets.env; then fail 'demo-plain has a secrets.env'; fi

	step 'secrets.env is created on the host, owned by the service user'
	for stack in demo-stack demo-cache; do
		docker exec -u qhost-deploy "$host" sh -ec "umask 077 && printf 'DEMO_SECRET=%s\n' '$secret' >/opt/stacks/$stack/secrets.env"
	done
	dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $dirty"
	[ -z "$dirty" ] || fail "secrets.env made the working copy dirty: $dirty"
	[ "$(docker exec "$host" stat -c %U:%a /opt/stacks/demo-stack/secrets.env)" = qhost-deploy:600 ] || fail 'secrets.env has the wrong owner or mode'

	step 'a webhook channel is added from standard input'
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add hook --type webhook --events succeeded,rolled-back) || fail "notify add failed: $out"
	qd notify list | grep -q 'hook *webhook *succeeded,rolled-back' || fail "notify list does not show the webhook: $(qd notify list)"

	step 'a manual run deploys demo-stack and records the commit'
	out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	commit=$(printf '%s\n' "$out" | sed -n 's/.*"commit": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] && [ -n "$commit" ] || fail "deploy did not report a run and a commit: $out"
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	log_text=$(qd stack log "$run")
	printf '%s' "$log_text" | grep -q ' result succeeded' || fail "the run did not succeed: $log_text"
	[ "$(stack_field demo-stack deployed)" = "$commit" ] || fail "demo-stack is not recorded as deployed at $commit: $(qd --output json status)"
	[ "$(stack_images demo-stack)" = "registry.$domain/demo/app:1.0.1" ] || fail "demo-stack does not run the image 1.0.1: $(stack_images demo-stack)"
	docker exec "$host" docker inspect -f '{{.Config.Env}}' "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)" |
		grep -q "DEMO_SECRET=$secret" || fail 'the container did not get the value of secrets.env'
	log "scenario: deploy succeeded, run $run, deployed commit $commit"
	wait_for 'the webhook for the deployed run' 60 hook_seen /notify succeeded demo-stack
	log "scenario: the webhook receiver was told of the succeeded run"

	step 'the engine deploys a new commit it finds by polling'
	good=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the run of demo-cache' 240 stack_settled demo-cache succeeded
	[ "$(stack_field demo-cache deployed)" = "$good" ] || fail "demo-cache is not recorded as deployed at $good: $(qd --output json status)"
	[ "$(stack_images demo-cache)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-cache does not run the image 1.1.0: $(stack_images demo-cache)"
	log "scenario: poll deployed demo-cache at $good"

	# Here every stack runs its newest commit, so the poll at the restart of the repair has nothing to run.
	step 'a bound installation without the grant gets it at the next root client command'
	docker exec "$host" sh -ec 'rm -f /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf && systemctl daemon-reload' ||
		fail 'cannot remove the stack folder grant'
	if docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks; then fail 'the grant is still active after its removal'; fi
	pid=$(service_pid)
	out=$(qd status) || fail "qhost status failed without the grant: $out"
	printf '%s' "$out" | grep -qF 'has no write grant' || fail "no notice about the missing grant: $out"
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the grant was not written again'
	docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks || fail 'the service has no write access after the repair'
	wait_for 'the service after the repair' 60 service_serving
	[ "$(service_pid)" != "$pid" ] || fail 'the repair did not restart the service'
	out=$(qd status) || fail "qhost status failed: $out"
	if printf '%s' "$out" | grep -qF 'has no write grant'; then fail "the repair repeated: $out"; fi

	step 'a commit with a failing healthcheck is rolled back with a revert commit on the remote branch'
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/" demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Break the demo-cache healthcheck" &&
		git push -q origin main'
	bad=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the rolled-back run of demo-cache' 240 stack_settled demo-cache rolled-back
	runs_out=$(qd stack runs demo-cache)
	failed_run=$(printf '%s\n' "$runs_out" | grep -m 1 -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	printf '%s\n' "$runs_out" | grep -m 1 "$failed_run" | grep -q 'rolled-back' || fail "the newest run of demo-cache is not rolled-back: $runs_out"
	failed_log=$(qd stack log "$failed_run")
	printf '%s' "$failed_log" | grep -q ' result rolled-back' || fail "the run log does not end rolled-back: $failed_log"
	printf '%s' "$failed_log" | grep -q 'unhealthy' || fail "the run log does not name the unhealthy container: $failed_log"
	[ "$(log_steps "$failed_log")" = "$rolled_back_plain_steps" ] || fail "unexpected steps of the rolled-back run: $(log_steps "$failed_log")"
	printf '%s' "$failed_log" | grep -q 'skipped: the stack has no backup rules' || fail "the rollback did not skip the data restore for a stack without backup rules: $failed_log"
	revert=$(as_admin "cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD")
	[ "$revert" != "$bad" ] || fail 'no revert commit reached the remote branch'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$revert" ] || fail 'the revert commit is not the head of the remote branch'
	meta=$(as_admin "cd ~/stacks && git log -1 --format='%an|%ae|%cn|%ce|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|qhost|qhost@localhost.invalid|Revert demo-cache to $(printf '%s' "$good" | cut -c 1-12)|$bad" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert:demo-cache")" = "$(as_admin "cd ~/stacks && git rev-parse $good:demo-cache")" ] ||
		fail 'the revert commit does not restore the demo-cache folder'
	[ "$(stack_field demo-cache deployed)" = "$revert" ] || fail "demo-cache is recorded as deployed at $(stack_field demo-cache deployed), want the revert commit $revert"
	[ "$(stack_images demo-cache)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-cache does not run the previous image 1.1.0: $(stack_images demo-cache)"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the rollback'
	log "scenario: failing healthcheck: run $failed_run rolled back, bad commit $bad reverted by $revert"
	wait_for 'the webhook for the rolled-back run' 60 hook_seen /notify rolled-back demo-cache
	log "scenario: the webhook receiver was told of the rolled-back run"

	step 'a rolled-back commit is not run again by the next poll'
	count=$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	[ "$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$count" ] || fail 'a rolled-back commit was run again by the poll'
	[ "$(stack_field demo-cache deployed)" = "$revert" ] || fail 'the poll changed the deployed commit of demo-cache'
	[ -z "$(stack_field demo-cache pending)" ] || fail "demo-cache has a pending commit after the rollback: $(stack_field demo-cache pending)"
	[ -z "$(stack_field demo-cache findings)" ] || fail "demo-cache has findings after the rollback: $(stack_field demo-cache findings)"

	step 'no run log holds a secret value'
	ids=$(qd stack runs | grep -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	n=0
	for id in $ids; do
		n=$((n + 1))
		if qd stack log "$id" | grep -qF -- "$secret"; then fail "the log of run $id holds the secret value"; fi
	done
	if qd stack log "$(newest_run demo-shop)" | grep -qF -- 'not-a-real-secret'; then fail 'the run log of demo-shop holds its DEMO_PASSWORD value'; fi
	[ "$n" -ge 3 ] || fail "too few runs to check the logs: $n"
	if qd stack runs | grep -qF -- "$secret"; then fail 'qhost stack runs shows the secret value'; fi
	log "scenario: the secret value is in none of $n run logs"

	scenario_notify
}

# scenario_notify proves that every channel type delivers a test message, that a channel is managed with the
# documented errors, and that no secret part of a channel shows in the listing, the run logs or the journal.
scenario_notify() {
	step 'notify test delivers through the webhook channel'
	out=$(qd notify test hook) || fail "notify test hook failed: $out"
	hook_seen /notify test || fail 'the webhook receiver did not record the test message'

	step 'notify test delivers through a Slack channel'
	out=$(qd_in "$slack_url_value
" notify add chat --type slack) || fail "notify add chat failed: $out"
	out=$(qd notify test chat) || fail "notify test chat failed: $out"
	docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F '"path":"/slack/T000/B000/demo-slack-path-e2e"' | grep -qF 'qhost test message' ||
		fail 'the receiver did not record the Slack test message'

	step 'notify test delivers through a Telegram channel'
	out=$(qd_in "$bot_credential
" notify add bot --type telegram --chat-id 4242 --base-url "https://hooks.$domain/telegram") || fail "notify add bot failed: $out"
	out=$(qd notify test bot) || fail "notify test bot failed: $out"
	docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F "\"path\":\"/telegram/bot$bot_credential/sendMessage\"" | grep -qF '4242' ||
		fail 'the receiver did not record the Telegram test message'

	step 'notify test delivers through an SMTP channel with STARTTLS and a login'
	out=$(qd_in "$mail_login_value
" notify add mail --type smtp --host "mail.$domain" --user qhost-mailer --from qhost@example.test \
		--to ops@example.test) || fail "notify add mail failed: $out"
	out=$(qd notify test mail) || fail "notify test mail failed: $out"
	mail_seen 'Subject: qhost test message' || fail 'the mail server did not record the test message'
	mail_seen '"to":["TO:\u003cops@example.test\u003e"]' || fail 'the mail server did not record the recipient'
	mail_seen '"user":"qhost-mailer"' || fail 'the mail server did not record the login name'

	step 'the channel commands report their errors'
	if out=$(qd_in "$slack_url_value
" notify add hook --type slack); then fail 'a duplicate channel was added'; fi
	printf '%s' "$out" | grep -q 'exists' || fail "the duplicate was not explained: $out"
	out=$(qd_fails notify test missing) || fail 'a missing channel was tested'
	printf '%s' "$out" | grep -q 'no such channel' || fail "the missing channel was not explained: $out"
	out=$(qd_fails notify remove missing) || fail 'a missing channel was removed'
	if out=$(qd_in "$slack_url_value
" notify add bad --type slack --events exploded); then fail 'an unknown event was accepted'; fi
	printf '%s' "$out" | grep -q 'event' || fail "the unknown event was not explained: $out"
	if qd notify list | grep -q '^bad '; then fail 'a refused channel was stored'; fi

	step 'no secret part of a channel shows in the listing, the run logs or the journal'
	listing=$(qd notify list; qd --output json notify list)
	runs=$(qd stack runs)
	for id in $(qd stack runs | grep -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'); do
		runs="$runs
$(qd stack log "$id")"
	done
	journal_text=$(journal)
	for value in "$hook_url" "$hook_signing_value" "$slack_url_value" demo-slack-path-e2e "$bot_credential" "$mail_login_value"; do
		for place in listing runs journal_text; do
			eval "text=\$$place"
			if printf '%s' "$text" | grep -qF -- "$value"; then fail "a secret part of a channel shows in $place"; fi
		done
	done
	[ -n "$journal_text" ] || fail 'the journal of the service is empty'

	step 'removing the channels removes their stored secrets'
	for name in hook chat bot mail; do
		out=$(qd notify remove "$name") || fail "notify remove $name failed: $out"
	done
	if qd notify list | grep -q 'webhook\|slack\|telegram\|smtp'; then fail "channels remain: $(qd notify list)"; fi
	left=$(docker exec "$host" sh -c 'ls /var/lib/qhost-deploy/credentials 2>/dev/null | grep -c "^notify" || true')
	[ "$left" = 0 ] || fail "$left notification credentials remain after the channels were removed"
	log 'scenario: all four channel types delivered a test message and no secret part showed anywhere'
}

# fixture_run runs a script as root in a throwaway container of the backup helper image on the host, with the
# data source (a host directory or a volume name) mounted at /data.
fixture_run() {
	docker exec -i "$host" docker run --rm --network none -v "$1:/data" "$backup_image" sh -ec "$2"
}

# fixture_write fills /data with entries of mixed numeric owners and modes.
fixture_write() {
	fixture_run "$1" '
		cd /data
		mkdir -p sub/deep
		printf "alpha\n" >a.txt && chown 1234:1234 a.txt && chmod 640 a.txt
		printf "root only\n" >r.txt && chown 0:0 r.txt && chmod 600 r.txt
		printf "nested\n" >sub/deep/n.bin && chown 4321:5678 sub/deep/n.bin && chmod 755 sub/deep/n.bin
		chown 2000:2000 sub && chmod 750 sub
		ln -s a.txt link'
}

# fixture_change_script alters, removes and adds entries in the current directory so the data differs from
# fixture_write in content, owners and modes.
fixture_change_script() {
	printf '%s' '
		rm -f a.txt
		printf "changed\n" >r.txt && chown 99:99 r.txt && chmod 666 r.txt
		chown 0:0 sub && chmod 777 sub
		rm -rf sub/deep
		printf "extra\n" >extra.txt'
}

fixture_change() {
	fixture_run "$1" "cd /data; $(fixture_change_script)"
}

# fixture_snap prints numeric owner, group, mode, type and link target of every entry, then the checksums.
fixture_snap() {
	docker exec -i "$host" docker run --rm --network none -v "$1:/data:ro" "$backup_image" sh -ec '
		cd /data
		find . -printf "%U:%G:%m:%y %p -> %l\n" | sort -k2
		find . -type f -exec sha256sum {} + | sort -k2'
}

backup_ids() {
	qd stack backup list "$1" | grep -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'
}

stack_running() {
	[ "$(stack_images "$1")" = "registry.$domain/demo/app:1.0.1" ]
}

scenario_backup() {
	bind=/opt/stacks/demo-stack/data
	vol=demo-stack_state
	store=/var/lib/qhost-deploy/backups/demo-stack

	step 'demo-stack data and volume get files with mixed owners and modes'
	wait_for 'demo-stack to be idle' 120 stack_idle demo-stack
	stack_running demo-stack || fail "demo-stack is not running before the backup: $(stack_images demo-stack)"
	fixture_write "$bind" || fail 'cannot write the fixture into data/'
	fixture_write "$vol" || fail 'cannot write the fixture into the volume'
	bind_before=$(fixture_snap "$bind") && vol_before=$(fixture_snap "$vol") || fail 'cannot read the fixture'
	printf '%s\n' "$bind_before" | grep -q '^1234:1234:640:f ./a.txt' || fail "the fixture lacks the owner 1234: $bind_before"
	printf '%s\n' "$bind_before" | grep -q '^0:0:600:f ./r.txt' || fail "the fixture lacks the root file: $bind_before"
	log "scenario: fixture written: $(printf '%s\n' "$bind_before" | grep -c ' ->') entries in data/ and in the volume"

	step 'backup create saves data/, the volume and the command output, and starts the stack again'
	out=$(qd --output json stack backup create demo-stack) || fail "backup create failed: $out"
	b1=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	[ -n "$b1" ] || fail "backup create reported no id: $out"
	printf '%s' "$out" | grep -q '"items": *3' || fail "the backup does not hold three items: $out"
	stack_running demo-stack || fail "demo-stack is not running after the backup: $(stack_images demo-stack)"
	[ "$(docker exec "$host" cat "$store/$b1/command.out")" = backup-command-output ] || fail "command.out of $b1 is wrong: $(docker exec "$host" ls -l "$store/$b1")"
	[ "$(docker exec "$host" stat -c %a:%U "$store/$b1/path-0.tar" "$store/$b1/volume-0.tar" | sort -u)" = 600:qhost-deploy ] ||
		fail "the archives are not private to the service user: $(docker exec "$host" ls -ln "$store/$b1")"
	log "scenario: backup $b1: $(docker exec "$host" sh -c "cd $store/$b1 && ls | tr '\n' ' '")"

	step 'the data changes after the backup'
	fixture_change "$bind" && fixture_change "$vol" || fail 'cannot change the data'
	bind_changed=$(fixture_snap "$bind") && vol_changed=$(fixture_snap "$vol") || fail 'cannot read the changed data'
	[ "$bind_changed" != "$bind_before" ] && [ "$vol_changed" != "$vol_before" ] || fail 'the change did not alter the data'

	step 'restore without confirmation and without a terminal is refused and changes nothing'
	out=$(qd_fails stack backup restore demo-stack "$b1") || fail "backup restore without confirmation succeeded: $out"
	printf '%s' "$out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $out"
	[ "$(fixture_snap "$bind")" = "$bind_changed" ] && [ "$(fixture_snap "$vol")" = "$vol_changed" ] || fail 'the refused restore changed the data'
	stack_running demo-stack || fail 'the refused restore stopped the stack'
	log "scenario: restore refused: $out"

	step 'restore with --yes brings back data/ and the volume byte-identical, owners and modes included'
	out=$(qd stack backup restore demo-stack "$b1" --yes) || fail "backup restore failed: $out"
	[ "$(fixture_snap "$bind")" = "$bind_before" ] || fail "data/ differs after the restore: $(fixture_snap "$bind")"
	[ "$(fixture_snap "$vol")" = "$vol_before" ] || fail "the volume differs after the restore: $(fixture_snap "$vol")"
	stack_running demo-stack || fail "demo-stack is not running after the restore: $(stack_images demo-stack)"
	log "scenario: restored $b1; data/ and volume identical to the backed-up state"

	step 'retention keeps the newest two backups and deletes the oldest'
	out=$(qd --output json stack backup create demo-stack) || fail "second backup failed: $out"
	b2=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	out=$(qd --output json stack backup create demo-stack) || fail "third backup failed: $out"
	b3=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	printf '%s' "$out" | grep -q "\"removed\": *\"$b1\"" || fail "the third backup did not report removing $b1: $out"
	listed=$(backup_ids demo-stack | tr '\n' ' ')
	[ "$listed" = "$b3 $b2 " ] || fail "backup list shows '$listed', want '$b3 $b2 '"
	if docker exec "$host" test -e "$store/$b1"; then fail "the oldest backup $b1 is still in the store"; fi
	[ "$(docker exec "$host" ls "$store" | wc -l)" = 2 ] || fail "the store does not hold exactly two backups: $(docker exec "$host" ls "$store")"
	stack_running demo-stack || fail 'demo-stack is not running after the backups'
	log "scenario: retention: list shows $listed, $b1 deleted"

	step 'a backup path outside the stack directory is refused'
	out=$(qd_fails stack check demo-badpath) || fail "check accepted a path outside the stack: $out"
	printf '%s' "$out" | grep -q -- "'..' is not allowed" || fail "check does not name the path error: $out"
	log "scenario: check refused: $out"

	step 'stack check reports the functions of a stack with settings and of one without'
	out=$(qd --output json stack check demo-stack) || fail "stack check of demo-stack failed: $out"
	case $out in *"$secret"*) fail 'stack check shows a secret value' ;; esac
	printf '%s' "$out" | jq -e '.errors == 0 and (.stacks | length) == 1 and
		(.stacks[0].functions | map(.function)) == ["deploy", "update", "rollback", "backup", "redact"] and
		all(.stacks[0].functions[]; .works) and .stacks[0].functions[1].mode == "manual"' >/dev/null ||
		fail "unexpected check of demo-stack: $out"
	out=$(qd --output json stack check demo-shop) || fail "stack check of demo-shop failed: $out"
	printf '%s' "$out" | jq -e '.errors == 0 and
		([.stacks[0].functions[] | {(.function): .works}] | add) == {deploy: true, update: false, rollback: false, backup: false, redact: true} and
		.stacks[0].functions[1].mode == "none" and (.stacks[0].functions[1].action | length) > 0' >/dev/null ||
		fail "unexpected check of demo-shop: $out"
	log "scenario: check of demo-stack and demo-shop as expected"
	out=$(qd_fails stack backup create demo-badpath) || fail "backup create accepted a path outside the stack: $out"
	printf '%s' "$out" | grep -q -- 'config-invalid' || fail "backup create does not refuse the invalid settings: $out"
	if docker exec "$host" test -e /var/lib/qhost-deploy/backups/demo-badpath; then fail 'a refused backup left a store directory'; fi
	log "scenario: backup create refused: $out"
}

# json_value prints the first string value of a key in compact JSON (empty when the key is absent or empty).
json_value() {
	printf '%s' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -n 1 | cut -d '"' -f 4
}

# updates_expect checks the single row of a 'qhost stack updates' JSON output: current, available, major_available
# (empty for none) and status.
updates_expect() {
	out=$1
	[ "$(json_value "$out" current)" = "$2" ] && [ "$(json_value "$out" available)" = "$3" ] &&
		[ "$(json_value "$out" major_available)" = "$4" ] && [ "$(json_value "$out" status)" = "$5" ] ||
		fail "unexpected updates, want current=$2 available=$3 major_available=$4 status=$5: $out"
	case $out in *2.1.0-rc.1*) fail "a pre-release is offered: $out" ;; esac
}

# demo_stack_settings prints the settings of demo-stack: the backup section the repository starts with and,
# if given, the update section $1, whose backslash escapes printf %b expands.
demo_stack_settings() {
	printf 'version: 1\n%bbackup:\n  paths:\n    - data\n  volumes:\n    - state\n  command: [/bin/echo, backup-command-output]\n  retention:\n    keep: 2\n' "${1:-}"
}

# settings_set replaces the settings of the stack $1 with the YAML $2 from standard input and keeps the JSON
# answer in settings_out. The service commits and pushes them and moves its working copy; no run starts.
settings_set() {
	settings_out=$(qd_in "$2" --output json stack settings set "$1" --file -) || fail "settings set of $1 failed: $settings_out"
}

# scenario_updates shows the update detection against the registry: the stored result, the ceiling of the stack,
# the major hint and a stack without settings. Detection changes nothing; only commits do.
scenario_updates() {
	step 'updates says a stack was never checked'
	out=$(qd --output json stack updates demo-stack) || fail "updates failed: $out"
	[ "$(json_value "$out" status)" = never-checked ] || fail "demo-stack is not reported as never checked: $out"

	step 'updates --refresh asks the registry; the default ceiling is minor and a major version is only a hint'
	out=$(qd --output json stack updates demo-stack --refresh) || fail "updates --refresh failed: $out"
	updates_expect "$out" 1.0.1 1.1.0 5.0.0 update
	[ -n "$(json_value "$out" checked)" ] || fail "the result has no time of the check: $out"
	stored=$(qd --output json stack updates demo-stack) || fail "updates failed: $stored"
	[ "$stored" = "$out" ] || fail "updates without --refresh does not show the stored result: $stored"
	log "scenario: updates $out"

	step 'a stack without settings is listed too'
	out=$(qd --output json stack updates demo-cache --refresh) || fail "updates demo-cache failed: $out"
	updates_expect "$out" 1.1.0 1.1.0 5.0.0 current

	step 'ceiling patch offers no minor step; ceiling major offers 5.0.0 without a hint'
	settings_set demo-stack "$(demo_stack_settings 'update:\n  ceiling: patch\n')"
	out=$(qd --output json stack updates demo-stack --refresh) || fail "updates failed: $out"
	updates_expect "$out" 1.0.1 1.0.1 5.0.0 current
	settings_set demo-stack "$(demo_stack_settings 'update:\n  ceiling: major\n')"
	out=$(qd --output json stack updates demo-stack --refresh) || fail "updates failed: $out"
	updates_expect "$out" 1.0.1 5.0.0 '' update

	step 'the demo-stack settings are restored'
	settings_set demo-stack "$(demo_stack_settings)"
	out=$(qd --output json stack updates demo-stack --refresh) || fail "updates failed: $out"
	updates_expect "$out" 1.0.1 1.1.0 5.0.0 update
	[ "$(stack_images demo-stack)" = "registry.$domain/demo/app:1.0.1" ] || fail "updates changed the running image: $(stack_images demo-stack)"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git log --format=%s -n 1 -- demo-stack/.env')" = 'Update demo-stack to 1.0.1' ] ||
		fail 'the version of demo-stack was changed'
}

# scenario_rollback runs the automatic rollback against the demo stacks: a good deploy with a backup, a bad
# deploy of a stack with data (data restored from the run's own backup, revert commit pushed, previous state
# healthy), and a bad deploy whose rollback fails (stack stopped and paused, deploy refused, resume).
scenario_rollback() {
	bind=/opt/stacks/demo-stack/data
	vol=demo-stack_state
	image=registry.$domain/demo/app

	step 'a good commit deploys demo-stack with a backup of its data'
	prev=$(newest_run demo-stack)
	admin_commit demo-stack/.env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' 'Update demo-stack to 1.1.0'
	good_stack=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-stack "$prev") || fail 'no run of demo-stack started for the good commit'
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the good run did not succeed: $run_log"
	[ "$(log_steps "$run_log")" = stop,backup,pull,up,check ] || fail "unexpected steps of the good run: $(log_steps "$run_log")"
	good_backup=$(printf '%s\n' "$run_log" | sed -n 's/^[^ ]* info backup \([0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}\)$/\1/p')
	[ -n "$good_backup" ] || fail "the good run made no backup: $run_log"
	[ "$(backup_ids demo-stack | head -n 1)" = "$good_backup" ] || fail "backup list does not show $good_backup first: $(qd stack backup list demo-stack)"
	[ "$(stack_field demo-stack deployed)" = "$good_stack" ] || fail "demo-stack is not recorded as deployed at $good_stack"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0: $(stack_images demo-stack)"
	log "scenario: good run $run deployed $good_stack, backup $good_backup"

	step 'a failing commit restores the data of the run backup, reverts the commit and runs the previous state'
	marker=marker-$run
	for target in "$bind" "$vol"; do
		fixture_run "$target" "printf '%s\n' $marker >/data/marker.txt && chown 4242:4242 /data/marker.txt && chmod 604 /data/marker.txt" ||
			fail 'cannot write the marker'
	done
	bind_before=$(fixture_snap "$bind") && vol_before=$(fixture_snap "$vol") || fail 'cannot read the data before the run'
	printf '%s\n' "$bind_before" | grep -q '^4242:4242:604:f ./marker.txt' || fail "the marker is missing: $bind_before"
	old_backups=$(backup_ids demo-stack | tr '\n' ' ')
	prev=$run
	as_admin 'cd ~/stacks && git pull -q --ff-only &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/" demo-stack/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Update demo-stack to 2.0.0 with a broken healthcheck" &&
		git push -q origin main'
	bad_stack=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-stack "$prev") || fail 'no run of demo-stack started for the failing commit'
	# Once the run has its backup, the data changes under the new version; only a restore can undo that.
	i=0
	until qd stack log "$run" | grep -q ' info backup [0-9]\{8\}T'; do
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail "the run $run made no backup"
		sleep 0.2
	done
	docker exec -i "$host" docker run --rm --network none -v "$bind:/data" -v "$vol:/state" "$backup_image" sh -ec \
		"for d in /data /state; do cd \$d; $(fixture_change_script); done" </dev/null || fail 'cannot change the data during the run'
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	[ "$(log_steps "$run_log")" = "$rolled_back_steps" ] || fail "unexpected steps of the rolled-back run: $(log_steps "$run_log")"
	run_backups=$(printf '%s\n' "$run_log" | sed -n 's/^[^ ]* info backup \([0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}\)$/\1/p')
	[ "$(printf '%s\n' "$run_backups" | wc -l)" = 2 ] && [ "$(printf '%s\n' "$run_backups" | sort -u | wc -l)" = 1 ] ||
		fail "the backup and the restore do not name one backup: $run_backups"
	run_backup=$(printf '%s\n' "$run_backups" | head -n 1)
	case " $old_backups " in *" $run_backup "*) fail "the run restored an older backup: $run_backup" ;; esac
	backup_ids demo-stack | grep -qx "$run_backup" || fail "backup list does not know $run_backup: $(qd stack backup list demo-stack)"
	[ "$(fixture_snap "$bind")" = "$bind_before" ] || fail "data/ differs from the backed-up state after the rollback: $(fixture_snap "$bind")"
	[ "$(fixture_snap "$vol")" = "$vol_before" ] || fail "the volume differs from the backed-up state after the rollback: $(fixture_snap "$vol")"
	[ "$(docker exec "$host" cat "$bind/marker.txt")" = "$marker" ] || fail 'the marker of this run backup is not back'
	revert=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	[ "$revert" != "$bad_stack" ] || fail 'no revert commit reached the remote branch'
	meta=$(as_admin "cd ~/stacks && git log -1 --format='%an|%ae|%cn|%ce|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|qhost|qhost@localhost.invalid|Revert demo-stack to $(printf '%s' "$good_stack" | cut -c 1-12)|$bad_stack" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert:demo-stack")" = "$(as_admin "cd ~/stacks && git rev-parse $good_stack:demo-stack")" ] ||
		fail 'the revert commit does not restore the demo-stack folder'
	[ "$(stack_field demo-stack deployed)" = "$revert" ] || fail "demo-stack is not recorded as deployed at the revert commit $revert"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run the previous image 1.1.0: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the rollback'
	log "scenario: run $run rolled back; restored $run_backup (not among $old_backups); revert $revert; data identical to the backed-up state"

	step 'a failing rollback leaves the stack stopped and paused'
	cache_good=$(stack_field demo-cache deployed)
	prev=$(newest_run demo-cache)
	admin_commit demo-cache/.env 's/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' 'Break the demo-cache healthcheck again'
	stuck=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	# The remote refuses every push from now on, so the revert commit cannot be published.
	docker exec "$prefix-git" sh -ec "printf '#!/bin/sh\necho rejected by the e2e hook >&2\nexit 1\n' >/srv/git/stacks.git/hooks/pre-receive &&
		chown git:git /srv/git/stacks.git/hooks/pre-receive && chmod 755 /srv/git/stacks.git/hooks/pre-receive"
	restart_service
	run=$(next_run demo-cache "$prev") || {
		docker exec "$prefix-git" rm -f /srv/git/stacks.git/hooks/pre-receive
		fail 'no run of demo-cache started for the failing commit'
	}
	wait_for "the run $run" 240 run_finished demo-cache "$run" || {
		docker exec "$prefix-git" rm -f /srv/git/stacks.git/hooks/pre-receive
		fail "the run $run did not finish"
	}
	docker exec "$prefix-git" rm -f /srv/git/stacks.git/hooks/pre-receive
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rollback-failed' || fail "the run did not end rollback-failed: $run_log"
	[ "$(log_steps "$run_log")" = "$rollback_failed_steps" ] || fail "unexpected steps of the rollback-failed run: $(log_steps "$run_log")"
	printf '%s' "$run_log" | grep -q 'rejected by the e2e hook' || fail "the run log does not show the remote refusing the push: $run_log"
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$stuck" ] || fail 'the remote branch moved although the push was refused'
	[ -z "$(stack_images demo-cache)" ] || fail "demo-cache is still running after the failed rollback: $(stack_images demo-cache)"
	[ -n "$(stack_field demo-cache paused)" ] || fail "status does not show demo-cache as paused: $(qd --output json status)"
	[ "$(stack_field demo-cache deployed)" = "$cache_good" ] || fail 'the failed rollback changed the deployed commit'
	log "scenario: run $run rollback-failed; demo-cache stopped and $(stack_field demo-cache paused)"

	step 'deploy is refused while the stack is paused and the poll does not run it'
	out=$(qd_fails stack deploy demo-cache) || fail "deploy of a paused stack succeeded: $out"
	printf '%s' "$out" | grep -q 'paused' || fail "the refusal does not name the pause: $out"
	count=$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	[ "$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$count" ] || fail 'the poll ran a paused stack'
	[ -n "$(stack_field demo-cache paused)" ] || fail 'the restart cleared the pause'
	stack_field demo-cache findings | grep -q failed-commit || fail "the failed commit is not marked as a finding: $(stack_field demo-cache findings)"
	log "scenario: deploy refused: $out"

	step 'resume clears the pause, and the stack deploys a fixing commit'
	out=$(qd --output json stack resume demo-cache) || fail "resume failed: $out"
	printf '%s' "$out" | grep -q '"stack": *"demo-cache"' || fail "resume does not name the stack: $out"
	[ -z "$(stack_field demo-cache paused)" ] || fail 'status still shows the pause after resume'
	if stack_field demo-cache findings | grep -q paused; then fail 'the paused finding stays after resume'; fi
	out=$(qd_fails stack resume demo-cache) || fail "a second resume succeeded: $out"
	prev=$(newest_run demo-cache)
	admin_commit demo-cache/.env 's/^DEMO_HEALTH=.*/DEMO_HEALTH=ok/; s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/' 'Fix the demo-cache healthcheck and update to 2.0.0'
	fix=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-cache "$prev") || fail 'no run of demo-cache started for the fixing commit'
	wait_for "the run $run" 240 run_finished demo-cache "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the fixing commit did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-cache deployed)" = "$fix" ] || fail "demo-cache is not recorded as deployed at $fix"
	[ "$(stack_images demo-cache)" = "$image:2.0.0" ] || fail "demo-cache does not run 2.0.0: $(stack_images demo-cache)"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the fix'
	log "scenario: resumed; fixing commit $fix deployed"
}

# json_values prints every string value of a key in JSON output, one per line.
json_values() {
	printf '%s' "$1" | grep -o "\"$2\": *\"[^\"]*\"" | cut -d '"' -f 4
}

# update_reset sets the major strategy $2 of demo-stack, commits the version $1 and waits for its run.
update_reset() {
	settings_set demo-stack "$(demo_stack_settings "update:\\n  major: $2\\n")"
	prev=$(newest_run demo-stack)
	admin_commit demo-stack/.env "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=$1/" "Reset demo-stack to $1"
	restart_service
	run=$(next_run demo-stack "$prev") || fail "no run of demo-stack started for the reset to $1"
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run for the reset to $1 did not succeed: $(qd stack log "$run")"
	[ "$(stack_images demo-stack)" = "registry.$domain/demo/app:$1" ] || fail "demo-stack does not run $1: $(stack_images demo-stack)"
}

# update_commit_only_version requires that a commit of the stacks repo changes one line of demo-stack/.env, the
# image variable, from one version to another.
update_commit_only_version() {
	files=$(as_admin "cd ~/stacks && git show --format= --numstat $1")
	[ "$files" = "$(printf '1\t1\tdemo-stack/.env')" ] || fail "the commit $1 touches more than one line of demo-stack/.env: $files"
	diff_lines=$(as_admin "cd ~/stacks && git show --format= -U0 $1 | grep '^[-+]' | grep -v '^\(---\|+++\)'")
	[ "$diff_lines" = "$(printf -- '-DEMO_APP_VERSION=%s\n+DEMO_APP_VERSION=%s' "$2" "$3")" ] ||
		fail "the commit $1 does not only move DEMO_APP_VERSION from $2 to $3: $diff_lines"
	[ "$(as_admin "cd ~/stacks && git log -1 --format=%s $1")" = "Update demo-stack DEMO_APP_VERSION from $2 to $3" ] ||
		fail "the commit $1 has an unexpected subject: $(as_admin "cd ~/stacks && git log -1 --format=%s $1")"
}

# update_run_backup prints the backup a run log names.
update_run_backup() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* info backup \([0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}\)$/\1/p' | head -n 1
}

# update_steps prints the steps of an update run in order, joined by commas.
update_steps() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* step //p' |
		grep -x -e stop -e backup -e commit -e push -e pull -e up -e check -e rollback -e restore -e revert -e fast-forward |
		tr '\n' ',' | sed 's/,$//'
}

# scenario_update takes version steps with 'qhost stack update' against the real git server and registry: a direct
# update, a stepwise major jump with a commit and a backup per step, and a stepwise jump whose middle step has a
# failing healthcheck. The image 4.0.0 is unhealthy by itself, so no commit but the update's own can break it.
scenario_update() {
	image=registry.$domain/demo/app

	step 'a stack without settings is not updated'
	out=$(qd_fails stack update demo-cache) || fail "update accepted a stack without settings: $out"
	printf '%s' "$out" | grep -q 'has no settings' || fail "the refusal does not say that the stack has no settings: $out"

	step 'stack settings set commits and pushes the settings of demo-stack without a run; equal settings commit nothing'
	direct=$(demo_stack_settings 'update:\n  major: direct\n')
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	deployed_before=$(stack_field demo-stack deployed)
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	settings_set demo-stack "$direct"
	printf '%s' "$settings_out" | grep -q '"changed": *true' || fail "settings set reports no change: $settings_out"
	commit=$(printf '%s\n' "$settings_out" | sed -n 's/.*"commit": *"\([0-9a-f]*\)".*/\1/p')
	[ -n "$commit" ] || fail "settings set reports no commit: $settings_out"
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$commit" ] || fail "the settings commit $commit is not the head of the remote branch"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	meta=$(as_admin "cd ~/stacks && git log -1 --format='%s|%P' $commit")
	[ "$meta" = "qhost: settings of demo-stack|$head_before" ] || fail "the settings commit has the wrong subject or parent: $meta"
	files=$(as_admin "cd ~/stacks && git show --format= --name-only $commit")
	[ "$files" = .qatlas/qhost/demo-stack.yaml ] || fail "the settings commit touches more than the settings file: $files"
	as_admin 'cd ~/stacks && grep -qx "  major: direct" .qatlas/qhost/demo-stack.yaml' ||
		fail "the settings file does not hold major direct: $(as_admin 'cat ~/stacks/.qatlas/qhost/demo-stack.yaml')"
	[ "$(docker exec -u qhost-deploy "$host" git -C /opt/stacks rev-parse HEAD)" = "$commit" ] || fail 'the working copy on the host did not follow the settings commit'
	dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $dirty"
	[ -z "$dirty" ] || fail "the settings left the working copy on the host dirty: $dirty"
	out=$(qd --output json stack settings demo-stack) || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"major": *"direct"' || fail "stack settings does not show major direct: $out"
	settings_set demo-stack "$direct"
	printf '%s' "$settings_out" | grep -q '"changed": *false' || fail "equal settings were committed again: $settings_out"
	if printf '%s' "$settings_out" | grep -q '"commit"'; then fail "equal settings report a commit: $settings_out"; fi
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$commit" ] || fail 'equal settings moved the remote branch'
	[ "$(stack_field demo-stack deployed)" = "$deployed_before" ] || fail 'the settings commit changed the deployed commit of demo-stack'
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$runs_before" ] || fail 'the settings started a run of demo-stack'
	log "scenario: settings commit $commit pushed; the working copy on the host is clean; a second set changed nothing"

	step 'with major direct, an update creates one commit that moves only the image variable, and the stack runs the new version'
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0 before the update: $(stack_images demo-stack)"
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	out=$(qd --output json stack update demo-stack --version 3.0.0) || fail "update to 3.0.0 failed: $out"
	[ "$(json_values "$out" to | tr '\n' ' ')" = '3.0.0 ' ] || fail "a direct update is not one step to 3.0.0: $out"
	run=$(json_values "$out" run | head -n 1)
	commit=$(json_values "$out" commit | head -n 1)
	[ -n "$run" ] && [ -n "$commit" ] || fail "update reported no run and commit: $out"
	run_log=$(qd stack log "$run")
	log "scenario: direct update steps: $(update_steps "$run_log")"
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the update run did not succeed: $run_log"
	[ "$(update_steps "$run_log")" = stop,backup,commit,push,fast-forward,pull,up,check ] || fail "unexpected steps of the update run: $(update_steps "$run_log")"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 1 ] || fail 'the update did not create exactly one commit'
	[ "$(as_admin 'cd ~/stacks && git rev-parse HEAD')" = "$commit" ] || fail "the head of the branch is not the update commit $commit"
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$commit" ] || fail 'the update commit is not the head of the remote branch'
	update_commit_only_version "$commit" 1.1.0 3.0.0
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$((runs_before + 1))" ] || fail 'the update did not make exactly one run'
	[ "$(stack_field demo-stack deployed)" = "$commit" ] || fail "demo-stack is not recorded as deployed at $commit"
	[ "$(stack_images demo-stack)" = "$image:3.0.0" ] || fail "demo-stack does not run 3.0.0: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the update'
	[ -n "$(update_run_backup "$run_log")" ] || fail "the update run made no backup: $run_log"
	log "scenario: direct update: run $run, commit $commit, demo-stack runs 3.0.0"

	step 'a stepwise major jump takes one step per major version, each with its own commit and backup'
	update_reset 1.1.0 stepwise
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	out=$(qd --output json stack update demo-stack --version 3.0.0) || fail "stepwise update to 3.0.0 failed: $out"
	[ "$(json_values "$out" to | tr '\n' ' ')" = '2.0.0 3.0.0 ' ] || fail "the jump to 3.0.0 is not the steps 2.0.0 and 3.0.0: $out"
	[ "$(json_values "$out" result | sort -u)" = succeeded ] || fail "not every step succeeded: $out"
	runs=$(json_values "$out" run)
	commits=$(json_values "$out" commit)
	[ "$(printf '%s\n' "$runs" | sort -u | wc -l)" = 2 ] && [ "$(printf '%s\n' "$commits" | sort -u | wc -l)" = 2 ] ||
		fail "the steps do not have their own run and commit: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 2 ] || fail 'the stepwise update did not create exactly two commits'
	[ "$(as_admin "cd ~/stacks && git log --reverse --format=%H $head_before..HEAD")" = "$commits" ] || fail "the commits on the branch are not the steps' commits: $commits"
	first_commit=$(printf '%s\n' "$commits" | sed -n 1p) second_commit=$(printf '%s\n' "$commits" | sed -n 2p)
	update_commit_only_version "$first_commit" 1.1.0 2.0.0
	update_commit_only_version "$second_commit" 2.0.0 3.0.0
	backups=
	for id in $runs; do
		run_log=$(qd stack log "$id")
		printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the step run $id did not succeed: $run_log"
		[ "$(update_steps "$run_log")" = stop,backup,commit,push,fast-forward,pull,up,check ] || fail "unexpected steps of the run $id: $(update_steps "$run_log")"
		this_backup=$(update_run_backup "$run_log")
		[ -n "$this_backup" ] || fail "the step run $id made no backup"
		case " $backups " in *" $this_backup "*) fail "the step runs share the backup $this_backup" ;; esac
		backup_ids demo-stack | grep -qx "$this_backup" || fail "backup list does not know $this_backup"
		backups="$backups $this_backup"
	done
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$((runs_before + 2))" ] || fail 'the stepwise update did not make exactly two runs'
	[ "$(stack_images demo-stack)" = "$image:3.0.0" ] || fail "demo-stack does not run 3.0.0: $(stack_images demo-stack)"
	[ "$(stack_field demo-stack deployed)" = "$second_commit" ] || fail "demo-stack is not recorded as deployed at $second_commit"
	log "scenario: stepwise update: commits $(printf '%s' "$commits" | tr '\n' ' '), backups$backups"

	step 'a failing middle step ends with a revert commit, the previous step stays, later steps are not attempted'
	update_reset 2.0.0 stepwise
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	deployed_before=$(stack_field demo-stack deployed)
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	out=$(qd_fails --output json stack update demo-stack --version 5.0.0) || fail "update across a failing step succeeded: $out"
	log "scenario: failing jump: $out"
	[ "$(json_values "$out" to | tr '\n' ' ')" = '3.0.0 4.0.0 5.0.0 ' ] || fail "the jump to 5.0.0 is not the steps 3.0.0, 4.0.0 and 5.0.0: $out"
	[ "$(json_values "$out" result | tr '\n' ' ')" = 'succeeded rolled-back skipped ' ] || fail "unexpected results of the steps: $out"
	runs=$(json_values "$out" run)
	[ "$(printf '%s\n' "$runs" | wc -l)" = 2 ] || fail "only the first two steps should have a run: $out"
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$((runs_before + 2))" ] || fail 'the later step was attempted or a step made no run'
	good_run=$(printf '%s\n' "$runs" | sed -n 1p) bad_run=$(printf '%s\n' "$runs" | sed -n 2p)
	good_log=$(qd stack log "$good_run") bad_log=$(qd stack log "$bad_run")
	printf '%s' "$good_log" | grep -q ' result succeeded' || fail "the run of the 3.0.0 step did not succeed: $good_log"
	printf '%s' "$bad_log" | grep -q ' result rolled-back' || fail "the run of the 4.0.0 step did not end rolled-back: $bad_log"
	printf '%s' "$bad_log" | grep -q 'unhealthy' || fail "the failed run does not name the unhealthy container: $bad_log"
	log "scenario: failed step steps: $(update_steps "$bad_log")"
	[ "$(update_steps "$bad_log")" = stop,backup,commit,push,fast-forward,pull,up,check,rollback,stop,restore,revert,push,fast-forward,up,check ] ||
		fail "unexpected steps of the failed step: $(update_steps "$bad_log")"
	[ -n "$(update_run_backup "$bad_log")" ] || fail "the failed step made no backup: $bad_log"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 3 ] || fail 'the failing jump did not leave exactly the step, the failed step and its revert on the branch'
	step_commit=$(as_admin "cd ~/stacks && git log --reverse --format=%H $head_before..HEAD" | sed -n 1p)
	bad_step=$(as_admin "cd ~/stacks && git log --reverse --format=%H $head_before..HEAD" | sed -n 2p)
	revert=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	update_commit_only_version "$step_commit" 2.0.0 3.0.0
	update_commit_only_version "$bad_step" 3.0.0 4.0.0
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$revert" ] || fail 'the revert commit is not the head of the remote branch'
	meta=$(as_admin "cd ~/stacks && git log -1 --format='%an|%ae|%cn|%ce|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|qhost|qhost@localhost.invalid|Revert demo-stack to $(printf '%s' "$step_commit" | cut -c 1-12)|$bad_step" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert:demo-stack")" = "$(as_admin "cd ~/stacks && git rev-parse $step_commit:demo-stack")" ] ||
		fail 'the revert commit does not restore the demo-stack folder of the previous step'
	if [ -n "$(as_admin "cd ~/stacks && git log --format=%s --grep='to 5.0.0'")" ]; then fail 'a commit for 5.0.0 reached the branch'; fi
	[ "$(as_admin "cd ~/stacks && git show $revert:demo-stack/.env | grep '^DEMO_APP_VERSION='")" = DEMO_APP_VERSION=3.0.0 ] || fail 'the branch does not hold 3.0.0'
	[ "$(stack_field demo-stack deployed)" = "$revert" ] || fail "demo-stack is not recorded as deployed at the revert commit $revert"
	[ "$(stack_images demo-stack)" = "$image:3.0.0" ] || fail "demo-stack does not run the previous step 3.0.0: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the rollback'
	[ -z "$deployed_before" ] || [ "$deployed_before" != "$revert" ] || fail 'no revert was made'
	log "scenario: failing jump: run $bad_run rolled back, step commit $step_commit kept, revert $revert, 5.0.0 not attempted"

	step 'the demo-stack settings and version are restored'
	update_reset 1.1.0 stepwise
	settings_set demo-stack "$(demo_stack_settings)"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0 again: $(stack_images demo-stack)"
}

scenario_stacks_repo() {
	setup_service
	scenario_host_service
	scenario_host_changes
	scenario_tui_system
	scenario_tui_changes
	scenario_tui_host_proposal
	scenario_rootless_instance
	url=git.$domain:stacks.git

	scenario_service_log

	# The default window (daily 03:00) must not open in the middle of the other sections on a real clock.
	window_far_away >/dev/null || fail 'cannot move the maintenance window out of the way'

	step 'deploy key'
	out=$(as_admin 'sudo qhost --output json key generate 2>&1') || fail "key generate failed: $out"
	pub=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	shown=$(as_admin 'sudo qhost --output json key show 2>&1' | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	[ "$shown" = "$pub" ] || fail "key show differs from key generate: $shown"
	fingerprint=$(printf '%s\n' "$pub" | docker exec -i "$host" sh -c 'cat >/tmp/deploy-key.pub && ssh-keygen -lf /tmp/deploy-key.pub; rm -f /tmp/deploy-key.pub') ||
		fail "ssh-keygen cannot parse the deploy key: $fingerprint"
	case $fingerprint in *SHA256:*ED25519*) ;; *) fail "unexpected fingerprint: $fingerprint" ;; esac
	log "scenario: deploy key shown and parsed: $fingerprint"
	printf '%s\n' "$pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	log 'scenario: deploy key registered on the git server'

	step 'repo set refuses an unconfirmed host key without a terminal'
	before=$(known_hosts_sum)
	out=$(qd_fails repo set "$url" --dir /opt/stacks --create) || fail "repo set without terminal and fingerprint succeeded: $out"
	printf '%s' "$out" | grep -q -- '--fingerprint' || fail "the refusal does not name --fingerprint: $out"
	[ "$(known_hosts_sum)" = "$before" ] || fail 'a host key was pinned without confirmation'
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created although the host key was not confirmed'; fi

	step 'repo set without --dir and without --create creates no stack folder'
	out=$(qd_fails repo set "$url" --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set without --dir succeeded: $out"
	printf '%s' "$out" | grep -q -- '--dir' || fail "the refusal does not name --dir: $out"
	out=$(qd_fails repo set "$url" --dir /opt/stacks --fingerprint "$(git_ed25519_fingerprint)") ||
		fail "repo set for a missing folder without --create succeeded: $out"
	printf '%s' "$out" | grep -q -- '--create' || fail "the refusal does not name --create: $out"
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created without confirmation'; fi
	if docker exec "$host" test -e /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then fail 'a refused repo set wrote the drop-in'; fi

	step 'repo set refuses a wrong fingerprint'
	out=$(qd_fails repo set "$url" --dir /opt/stacks --create --fingerprint SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA) ||
		fail "repo set with a wrong fingerprint succeeded: $out"
	[ "$(known_hosts_sum)" = "$before" ] || fail 'a host key was pinned despite a wrong fingerprint'
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created despite a wrong fingerprint'; fi

	step 'repo set pins the host key and clones'
	expected=$(git_ed25519_fingerprint)
	out=$(qd --output json repo set "$url" --dir /opt/stacks --create --fingerprint "$expected") || fail "repo set failed: $out"
	printf '%s' "$out" | grep -q "$expected" || fail "repo set does not report the fingerprint $expected: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone: $out"
	docker exec "$host" test -f /opt/stacks/demo-stack/compose.yaml || fail 'the clone is missing in /opt/stacks'
	[ "$(docker exec "$host" stat -c %U:%G:%a /opt/stacks)" = qhost-deploy:qhost-deploy:755 ] ||
		fail "/opt/stacks is $(docker exec "$host" stat -c %U:%G:%a /opt/stacks), want qhost-deploy:qhost-deploy:755"
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the drop-in does not grant /opt/stacks'
	docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks || fail 'the service has no write access to /opt/stacks'
	log "scenario: repo set pinned $expected and cloned"

	step 'repo fetch sees a commit pushed by the admin'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q '"updates_available": *false' || fail "unexpected updates before the push: $out"
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/" demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Update demo-cache to 1.1.0" &&
		git push -q origin main'
	pushed=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q '"updates_available": *true' || fail "repo fetch does not see the new commit: $out"
	printf '%s' "$out" | grep -q "$pushed" || fail "repo fetch does not report the pushed head $pushed: $out"
	log "scenario: repo fetch saw $pushed"

	step 'stack list shows the demo stacks with image versions'
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	log "scenario: stack list $out"
	for want in demo-stack demo-cache DEMO_APP_VERSION=1.0.1 DEMO_APP_VERSION=1.0.0; do
		printf '%s' "$out" | grep -q -- "$want" || fail "stack list does not show $want: $out"
	done

	scenario_deploy

	scenario_containers

	scenario_container_logs

	scenario_read_access

	scenario_proposals

	scenario_container_actions

	scenario_stack_actions

	scenario_repair_start

	scenario_container_monitor

	scenario_backup

	scenario_updates

	scenario_rollback

	scenario_diagnose

	scenario_update

	scenario_window

	scenario_tui

	step 'a changed host key of the git server aborts qhost'
	original_key=$(scanned_git_key)
	pinned=$(known_hosts_sum)
	docker exec "$prefix-git" sh -ec 'mkdir -p /root/hostkeys-orig && mv /etc/ssh/ssh_host_* /root/hostkeys-orig/ && ssh-keygen -A >/dev/null && kill -HUP 1'
	rotated=
	rotate_failure=
	i=0
	while [ "$(scanned_git_key)" = "$original_key" ]; do
		i=$((i + 1))
		if [ "$i" -ge 20 ]; then rotate_failure=1; break; fi
		sleep 1
	done
	fetch_rc=0 set_rc=0 fetch_out= set_out=
	if [ -z "$rotate_failure" ]; then
		new_fp=$(git_ed25519_fingerprint)
		fetch_out=$(qd repo fetch) || fetch_rc=$?
		set_out=$(qd repo set "$url" --dir /opt/stacks --fingerprint "$new_fp") || set_rc=$?
	fi
	after=$(known_hosts_sum)
	# Restore the original keys first, so a failed assertion below leaves the environment as it was.
	docker exec "$prefix-git" sh -ec 'rm -f /etc/ssh/ssh_host_* && mv /root/hostkeys-orig/ssh_host_* /etc/ssh/ && rmdir /root/hostkeys-orig && kill -HUP 1'
	i=0
	while [ "$(scanned_git_key)" != "$original_key" ]; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail 'the original host keys of the git server are not served again'
		sleep 1
	done
	[ -z "$rotate_failure" ] || fail 'the git server did not serve the rotated host key'
	[ "$fetch_rc" -ne 0 ] || fail "repo fetch succeeded against a changed host key: $fetch_out"
	[ "$set_rc" -ne 0 ] || fail "repo set succeeded against a changed host key: $set_out"
	printf '%s' "$fetch_out" | grep -qi 'host key' || fail "repo fetch does not name the host key: $fetch_out"
	printf '%s' "$set_out" | grep -qi 'host key' || fail "repo set does not name the host key: $set_out"
	[ "$after" = "$pinned" ] || fail 'the pinned host key changed'
	log "scenario: changed host key refused: fetch: $fetch_out | set: $set_out"

	step 'the original host key works again'
	qd repo fetch >/dev/null || fail 'repo fetch fails after the host keys were restored'

	scenario_observe_start

	scenario_sources

	scenario_root_source

	scenario_source_inspect

	scenario_source_inspect_engines

	scenario_key_set

	scenario_observe_end

	scenario_source_local

	scenario_source_files

	scenario_source_remote

	scenario_tui_remote

	scenario_repair_end

	scenario_adopt

	scenario_tui_proposals

	scenario_tui_proposal_kinds

	scenario_tui_setup

	scenario_tui_rules

	scenario_tui_backup

	scenario_tui_check
}

# flat_field prints one flat field of qhost status by its full name (empty when it is absent).
flat_field() {
	qd --output json status | sed -n "s/.*\"$1\": *\"\\([^\"]*\\)\".*/\\1/p"
}

collision_seen() {
	flat_field source.opt-stacks2.stack.demo-stack.findings | grep -q name-collision
}

# scenario_sources proves the service with several stack folders. An installation bound to one repository by an
# earlier version is represented by the state layout that version left (stacks-repo.json, the engine state in
# deploy/), made from the current one while the service is stopped; the start must take it over with state and
# deploy key. Then a second repository becomes a second source: its own stack is deployed, a stack named like one of
# the first source is reported with name-collision and never deployed.
scenario_sources() {
	state=/var/lib/qhost-deploy
	url2=git.$domain:stacks2.git

	step 'an installation of the earlier single binding is taken over with its state and deploy key'
	key_before=$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	deployed_before=$(stack_field demo-stack deployed)
	run_before=$(stack_field demo-stack last_run)
	images_before=$(stack_images demo-stack)
	[ -n "$key_before" ] && [ -n "$deployed_before" ] && [ -n "$run_before" ] || fail 'the first source has no key, deployment or run to carry over'
	docker exec "$host" systemctl stop qhost-deploy || fail 'cannot stop the deploy service'
	url1=$(docker exec "$host" sed -n 's/.*"url": *"\([^"]*\)".*/\1/p' "$state/sources.json")
	[ "$url1" = "$url" ] || fail "sources.json does not bind $url: $url1"
	docker exec "$host" test -d "$state/sources/opt-stacks/deploy" || fail 'the engine state of the first source is not below sources/opt-stacks/deploy'
	docker exec "$host" sh -ec "
		mv $state/sources/opt-stacks/deploy $state/deploy
		rmdir $state/sources/opt-stacks $state/sources
		rm $state/sources.json
		printf '{\"version\":1,\"url\":\"$url1\",\"dir\":\"/opt/stacks\"}' >$state/stacks-repo.json
		chown qhost-deploy:qhost-deploy $state/stacks-repo.json && chmod 600 $state/stacks-repo.json" || fail 'cannot lay out the earlier state'
	docker exec "$host" systemctl start qhost-deploy || fail 'cannot start the deploy service on the earlier state'
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the deploy service to answer' 20 qd status
	wait_for 'the poll of the taken-over source' 60 poll_done
	if docker exec "$host" test -e "$state/stacks-repo.json"; then fail 'the old binding file is still the binding'; fi
	if docker exec "$host" test -e "$state/deploy"; then fail 'the engine state was not moved'; fi
	docker exec "$host" test -d "$state/sources/opt-stacks/deploy" || fail 'the engine state is not below sources/opt-stacks'
	docker exec "$host" grep -q '"key": *"deploy-key"' "$state/sources.json" || fail "the first source does not keep the deploy key: $(docker exec "$host" cat "$state/sources.json")"
	[ "$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$key_before" ] || fail 'the deploy key changed with the takeover'
	[ "$(flat_field source.opt-stacks.path)" = /opt/stacks ] || fail "status does not show the first source: $(qd --output json status)"
	[ "$(stack_field demo-stack source)" = opt-stacks ] || fail "demo-stack is not listed with its source: $(qd --output json status)"
	[ "$(stack_field demo-stack deployed)" = "$deployed_before" ] || fail 'the deployed commit of demo-stack was lost with the takeover'
	[ "$(stack_field demo-stack last_run)" = "$run_before" ] || fail 'the takeover started a run of demo-stack'
	qd repo fetch >/dev/null || fail 'repo fetch fails with the taken-over key'
	log 'scenario: the single binding became the source opt-stacks with its state and key'

	step 'a second repository is bound as a second source with its own deploy key'
	out=$(qd --output json key generate --dir /opt/stacks2) || fail "key generate --dir failed: $out"
	pub2=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub2 in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	[ "$pub2" != "$key_before" ] || fail 'the second source got the key of the first'
	[ "$(qd --output json key show --dir /opt/stacks2 | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$pub2" ] || fail 'key show --dir differs from key generate --dir'
	[ "$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$key_before" ] || fail 'a key for another folder replaced the key of the first source'
	printf '%s\n' "$pub2" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo set "$url2" --dir /opt/stacks2 --create --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set for the second source failed: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone the second source: $out"
	docker exec "$host" test -f /opt/stacks2/demo-second/compose.yaml || fail 'the clone is missing in /opt/stacks2'
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks2' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /opt/stacks2'
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(flat_field source.opt-stacks2.path)" = /opt/stacks2 ] || fail "status does not show the second source: $(qd --output json status)"

	step 'the own stack of the second source is deployed'
	wait_for 'the first run of demo-second' 180 stack_settled demo-second succeeded
	[ "$(stack_images demo-second)" = "registry.$domain/demo/app:1.0.0" ] || fail "demo-second does not run the image 1.0.0: $(stack_images demo-second)"
	[ "$(stack_field demo-second source)" = opt-stacks2 ] || fail "demo-second is not listed with its source: $(qd --output json status)"
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks2' || fail "stack list does not show the source of demo-second: $out"

	step 'a stack named like one of the first source is reported with name-collision and never deployed'
	wait_for 'the finding name-collision' 60 collision_seen
	[ "$(flat_field source.opt-stacks2.stack.demo-stack.source)" = opt-stacks2 ] || fail "the blocked entry has no source: $(qd --output json status)"
	[ -z "$(flat_field source.opt-stacks2.stack.demo-stack.deployed)" ] || fail 'the blocked stack was deployed'
	[ -z "$(flat_field source.opt-stacks2.stack.demo-stack.last_run)" ] || fail 'the blocked stack has a run'
	[ "$(stack_field demo-stack source)" = opt-stacks ] || fail 'the stack of the first source lost its name'
	[ "$(stack_field demo-stack deployed)" = "$deployed_before" ] || fail 'the stack of the first source changed'
	[ "$(stack_images demo-stack)" = "$images_before" ] || fail "demo-stack changed its image: $(stack_images demo-stack) instead of $images_before"
	if docker exec "$host" docker ps -a --format '{{.Names}}' --filter label=com.docker.compose.project=demo-stack | grep -q . &&
		docker exec "$host" docker ps -a --filter label=com.docker.compose.project=demo-stack --format '{{.Image}}' | grep -q ':2.0.0$'; then
		fail 'a container of the duplicate demo-stack runs'
	fi
	log 'scenario: second source deployed demo-second; its demo-stack is blocked by name-collision'
}

# source_stack_count prints how many stacks status lists for the source $1.
source_stack_count() {
	qd --output json status | grep -c "\"stack\\.[^.]*\\.source\": *\"$1\"" || true
}

# root_commit changes a file of the admin clone of the third repository and pushes it as a new commit.
root_commit() {
	as_admin "cd ~/stacks3 && git pull -q --ff-only && sed -i '$2' $1 &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am '$3' && git push -q origin main"
}

# scenario_root_source proves a stack folder whose root holds the compose file: the whole folder is exactly one
# stack, a subfolder with a compose file of its own is only reported, a version change by commit is deployed and
# a failing one is rolled back with a revert commit of the whole folder.
scenario_root_source() {
	url3=git.$domain:stacks3.git
	image=registry.$domain/demo/app

	step 'a third repository with its compose file in the root is bound as one more source'
	out=$(qd --output json key generate --dir /opt/stacks3) || fail "key generate --dir failed: $out"
	pub3=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub3 in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	printf '%s\n' "$pub3" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo set "$url3" --dir /opt/stacks3 --create --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set for the third source failed: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone the third source: $out"
	docker exec "$host" test -f /opt/stacks3/compose.yaml || fail 'the clone in /opt/stacks3 has no compose file in its root'

	step 'the root of the source is deployed as exactly one stack; its subfolder is no stack'
	wait_for 'the first run of demo-root' 180 stack_settled demo-root succeeded
	[ "$(source_stack_count opt-stacks3)" = 1 ] || fail "the third source is not exactly one stack: $(qd --output json status)"
	[ "$(stack_field demo-root source)" = opt-stacks3 ] || fail "demo-root is not listed with its source: $(qd --output json status)"
	[ -z "$(stack_field demo-extra source)" ] || fail 'the subfolder with a compose file is a stack'
	[ "$(stack_images demo-root)" = "$image:1.0.0" ] || fail "demo-root does not run the image 1.0.0: $(stack_images demo-root)"
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-root --format '{{.Label "com.docker.compose.project.working_dir"}}')" = /opt/stacks3 ] ||
		fail 'demo-root does not run from the root of the source'
	[ -z "$(stack_images demo-extra)" ] || fail 'a container of the subfolder runs'
	out=$(qd --output json stack check demo-root) || fail "stack check of demo-root failed: $out"
	printf '%s' "$out" | jq -e '.errors == 0 and ([.stacks[].findings[] | select(.rule == "ignored-stack-folder")] | length) == 1' >/dev/null ||
		fail "stack check does not report ignored-stack-folder once: $out"
	as_admin "rm -rf ~/stacks3 && git clone -q $url3 ~/stacks3" || fail 'cannot clone the third repository as admin'

	step 'a version change by commit is deployed'
	prev=$(newest_run demo-root)
	root_commit .env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' 'Update demo-root to 1.1.0'
	good_root=$(as_admin 'cd ~/stacks3 && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-root "$prev") || fail 'no run of demo-root started for the version change'
	wait_for "the run $run" 240 run_finished demo-root "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-root deployed)" = "$good_root" ] || fail "demo-root is not recorded as deployed at $good_root"
	[ "$(stack_images demo-root)" = "$image:1.1.0" ] || fail "demo-root does not run 1.1.0: $(stack_images demo-root)"

	step 'a failing version is rolled back and the revert commit restores the whole folder'
	prev=$run
	root_commit .env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' 'Update demo-root to 2.0.0 with a broken healthcheck'
	bad_root=$(as_admin 'cd ~/stacks3 && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-root "$prev") || fail 'no run of demo-root started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-root "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(as_admin 'cd ~/stacks3 && git pull -q --ff-only && git rev-parse HEAD')
	[ "$revert" != "$bad_root" ] || fail 'no revert commit reached the remote branch'
	meta=$(as_admin "cd ~/stacks3 && git log -1 --format='%an|%ae|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|Revert the stack folder to $(printf '%s' "$good_root" | cut -c 1-12)|$bad_root" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks3 && git rev-parse $revert^{tree}")" = "$(as_admin "cd ~/stacks3 && git rev-parse $good_root^{tree}")" ] ||
		fail 'the revert commit does not restore the whole folder'
	[ "$(stack_field demo-root deployed)" = "$revert" ] || fail "demo-root is not recorded as deployed at the revert commit $revert"
	[ "$(stack_images demo-root)" = "$image:1.1.0" ] || fail "demo-root does not run the previous image 1.1.0: $(stack_images demo-root)"
	log "scenario: demo-root, the root of /opt/stacks3, deployed 1.1.0 and rolled back 2.0.0 with the revert $revert"
}

# scenario_source_inspect proves source inspect on four kinds of folder below /srv/qd-inspect, which no stack folder
# uses: an empty folder to clone into, a clone with an origin, a repository without a remote and files
# without a repository. The answers are parsed as JSON, and the tree of the folders is the same before and after.
scenario_source_inspect() {
	base=/srv/qd-inspect
	step 'source inspect classifies an empty folder, a clone, a local repository and a folder of files'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/empty' '$base/clone' '$base/local' '$base/files'" ||
		fail 'cannot prepare the folders to inspect'
	docker exec "$host" sh -ec "git init -q '$base/clone' && git -C '$base/clone' remote add origin '$url'" ||
		fail 'cannot prepare the clone to inspect'
	docker exec "$host" sh -ec "git init -q '$base/local' && echo demo >'$base/local/notes.txt' && git -C '$base/local' add -A &&
		git -C '$base/local' -c user.name='E2E Inspect' -c user.email=inspect.qhoste2e.test commit -q -m 'First commit'" ||
		fail 'cannot prepare the local repository to inspect'
	printf 'services:\n  app:\n    image: registry.%s/demo/app:1.0.0\n' "$domain" |
		docker exec -i "$host" sh -ec "cat >'$base/files/compose.yaml'" || fail 'cannot write the compose file to inspect'
	tree_before=$(docker exec "$host" find "$base" -printf '%p %m %u %s\n' | sort)

	out=$(qd --output json source inspect "$base/empty" --url "$url") || fail "source inspect of an empty folder failed: $out"
	printf '%s' "$out" | jq -e --arg u "$url" '.case == "clone" and .url == $u and (.access.mode | length) > 0' >/dev/null ||
		fail "unexpected inspection of the empty folder: $out"
	out=$(qd --output json source inspect "$base/clone") || fail "source inspect of a clone failed: $out"
	printf '%s' "$out" | jq -e --arg u "$url" '.case == "adopt-clone" and .url == $u' >/dev/null ||
		fail "unexpected inspection of the clone: $out"
	out=$(qd --output json source inspect "$base/local") || fail "source inspect of a local repository failed: $out"
	printf '%s' "$out" | jq -e '.case == "local" and (has("url") | not)' >/dev/null ||
		fail "unexpected inspection of the local repository: $out"
	out=$(qd --output json source inspect "$base/files") || fail "source inspect of a folder of files failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files | index("compose.yaml")) != null and (.stacks | length) >= 1' >/dev/null ||
		fail "unexpected inspection of the folder of files: $out"
	out=$(qd source inspect "$base/files") || fail "source inspect as text failed: $out"
	printf '%s' "$out" | grep -q 'files' || fail "the text answer does not name the case: $out"

	[ "$(docker exec "$host" find "$base" -printf '%p %m %u %s\n' | sort)" = "$tree_before" ] ||
		fail 'source inspect changed the folders it inspected'
	docker exec "$host" rm -rf "$base" || fail 'cannot remove the inspected folders'
	log 'scenario: source inspect told clone, adopt-clone, local and files and changed nothing'
}

# scenario_source_inspect_engines proves that source inspect finds the containers of a stack folder in the rootless
# engine of $rootless_user, under the project name of the stack and under another one, names the engine in JSON and
# text, and lists the engine as not checked once it is stopped while the rest of the answer stays valid. The engine is
# started again and waited for, and the containers and the folder are removed.
scenario_source_inspect_engines() {
	dir=/srv/qd-inspect-rootless
	step 'source inspect reports containers of the folder in a rootless engine, and the engine as not checked once it stops'
	docker exec "$host" sh -ec "rm -rf '$dir' && mkdir -p '$dir' && chown '$rootless_user:$rootless_user' '$dir' && chmod 0755 '$dir'" ||
		fail 'cannot prepare the folder for the rootless engine'
	printf 'services:\n  app:\n    image: demo/app:1.0.0\n' |
		docker exec -i -u "$rootless_user" "$host" sh -ec "cat >'$dir/compose.yaml'" || fail 'cannot write the compose file'
	out=$(qd --output json source inspect "$dir") || fail "source inspect of the rootless folder failed: $out"
	project=$(printf '%s' "$out" | jq -r '.stacks[0].project') || fail "unexpected inspection of the rootless folder: $out"
	[ -n "$project" ] && [ "$project" != null ] || fail "source inspect names no project for the folder: $out"
	printf '%s' "$out" | jq -e '[.stacks[].conflicts // [] | length] | add == 0' >/dev/null ||
		fail "source inspect reports conflicts before any container runs: $out"
	other=$prefix-rootless-other
	as_rootless "cd '$dir' && docker compose -p '$project' up -d --quiet-pull && docker compose -p '$other' up -d --quiet-pull" >/dev/null 2>&1 ||
		fail 'cannot start the compose projects in the rootless engine'

	out=$(qd --output json source inspect "$dir") || fail "source inspect with the rootless containers failed: $out"
	want=rootless:$rootless_user
	printf '%s' "$out" | jq -e --arg e "$want" --arg p "$project" --arg o "$other" '
		[.stacks[].conflicts // [] | .[] | select(.engine == $e)] as $c
		| ($c | map(.project) | index($p)) != null and ($c | map(.project) | index($o)) != null' >/dev/null ||
		fail "source inspect does not report both projects as conflicts of $want: $out"
	printf '%s' "$out" | jq -e '.engines_not_checked // [] | length == 0' >/dev/null ||
		fail "an engine is not checked while the rootless engine runs: $out"
	out=$(qd source inspect "$dir") || fail "source inspect as text failed: $out"
	printf '%s' "$out" | grep -q "$want" || fail "the text answer does not name the engine $want: $out"

	# The setup tool of the rootless engine may install no socket unit; stop it only when it exists.
	as_rootless 'systemctl --user stop docker.service; if systemctl --user cat docker.socket >/dev/null 2>&1; then systemctl --user stop docker.socket; fi' >/dev/null 2>&1 ||
		fail 'cannot stop the rootless engine'
	out=$(qd --output json source inspect "$dir") || fail "source inspect with the stopped engine failed: $out"
	printf '%s' "$out" | jq -e --arg e "$want" '
		.case == "files" and (.stacks | length) >= 1
		and ([.engines_not_checked // [] | .[] | select(.engine == $e and (.reason | length) > 0)] | length) == 1' >/dev/null ||
		fail "the stopped engine is not listed as not checked, or the answer is invalid: $out"
	qd source inspect "$dir" | grep -q "$want was not checked" || fail 'the text answer does not list the stopped engine as not checked'

	as_rootless 'systemctl --user start docker.service' >/dev/null 2>&1 || fail 'cannot start the rootless engine again'
	wait_for 'the rootless engine to answer again' 60 as_rootless 'docker info'
	as_rootless "cd '$dir' && docker compose -p '$project' down && docker compose -p '$other' down" >/dev/null 2>&1 ||
		fail 'cannot remove the compose projects of the rootless engine'
	[ -z "$(as_rootless 'docker ps -aq')" ] || fail 'the rootless engine still has containers'
	docker exec "$host" rm -rf "$dir" || fail 'cannot remove the folder of the rootless engine'
	log 'scenario: source inspect found the folder containers in the rootless engine and listed it as not checked while stopped'
}

# local_tree prints mode, owner, group and size of every path below /srv/qd-local except .git, which the service
# may touch while it polls.
local_tree() {
	docker exec "$host" find /srv/qd-local -path /srv/qd-local/.git -prune -o -printf '%p %m %u:%g %s\n' | sort
}

# local_git runs git on /srv/qd-local as the person $admin; stderr is merged.
local_git() {
	docker exec -u "$admin" "$host" git -C /srv/qd-local "$@" 2>&1
}

# list_has_path succeeds when source list names the folder $1.
list_has_path() {
	qd --output json source list | jq -e --arg p "$1" '[.sources[].path] | index($p) != null' >/dev/null
}

# live_containers prints the id and the start time of every container of demo-live.
live_containers() {
	docker exec "$host" sh -c "docker ps -q --filter label=com.docker.compose.project=demo-live | sort | xargs -r docker inspect -f '{{.Id}} {{.State.StartedAt}}'"
}

# scenario_source_local proves source add, list and remove on a local repository without a remote below
# /srv/qd-local, which no stack folder uses: the service gets the access it needs, a version committed by a person is
# deployed, a failing one is rolled back by a local revert commit, and removing the source leaves the files and the
# containers alone. Then the existing clone /opt/stacks2 is released and taken over again with its origin and key.
scenario_source_local() {
	base=/srv/qd-local
	image=registry.$domain/demo/app

	step 'a local repository without a remote is prepared as root, with an unversioned secrets file'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/demo-local' && chmod 0755 '$base'" || fail 'cannot create the local folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/demo-local/compose.yaml'"
name: demo-local

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-local
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/demo-local/.env'"
	# demo-live pins its image and is started by hand before the source is added, as an existing stack would be.
	docker exec "$host" sh -ec "mkdir -p '$base/demo-live'"
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/demo-live/compose.yaml'"
name: demo-live

services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-live
      DEMO_VERSION: 1.0.0
      DEMO_HEALTH: ok
EOF2
	printf 'secrets.env\ndata/\n' | docker exec -i "$host" sh -ec "cat >'$base/demo-local/.gitignore'"
	docker exec "$host" sh -ec "cd '$base' && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-local stack' &&
		printf 'DEMO_SECRET=not-a-real-secret\n' >demo-local/secrets.env && chgrp qhost-deploy demo-local/secrets.env &&
		chmod 0640 demo-local/secrets.env" ||
		fail 'cannot create the local repository'
	[ -z "$(docker exec "$host" git -C "$base" remote)" ] || fail 'the local repository has a remote'
	secret_before=$(docker exec "$host" stat -c '%U:%G %a' "$base/demo-local/secrets.env")
	[ "$secret_before" = 'root:qhost-deploy 640' ] || fail "unexpected secrets file: $secret_before"

	step 'source list shows the earlier sources and not the local folder'
	out=$(qd --output json source list) || fail "source list failed: $out"
	printf '%s' "$out" | jq -e '[.sources[].path] | (index("/opt/stacks") != null and index("/opt/stacks2") != null and index("/srv/qd-local") == null)' >/dev/null ||
		fail "unexpected source list: $out"
	out=$(qd source list) || fail "source list as text failed: $out"
	printf '%s' "$out" | grep -qi 'source.opt-stacks.path' || fail "the text list does not show opt-stacks: $out"

	step 'source add without a terminal and without --yes is refused and changes nothing'
	dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$base")
	out=$(qd_fails source add "$base") || fail "source add without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add without --yes does not ask for confirmation: $out"
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base")" = "$dir_before" ] || fail 'a refused source add changed the folder'
	if docker exec "$host" grep -qF "$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'a refused source add wrote the drop-in'
	fi
	if list_has_path "$base"; then fail 'a refused source add bound the folder'; fi

	step 'demo-live is started by hand before the source is added'
	docker exec "$host" docker compose --project-directory "$base/demo-live" up -d >/dev/null 2>&1 || fail 'cannot start demo-live by hand'
	live_before=$(live_containers)
	[ -n "$live_before" ] || fail 'demo-live has no running container'

	step 'source add --yes binds the repository as a local source and gives the service the group access'
	# qd merges stderr, where the preview and the notices go; the JSON answer is the last line.
	out=$(qd --output json source add "$base" --yes) || fail "source add failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .path == "/srv/qd-local"' >/dev/null || fail "unexpected source add result: $out"
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(docker exec "$host" stat -c '%U:%G' "$base")" = root:qhost-deploy ] || fail "the folder has the wrong owner or group: $(docker exec "$host" stat -c '%U:%G' "$base")"
	case $(docker exec "$host" stat -c '%A' "$base") in d???rws*) ;; *) fail "the folder is not group-writable with setgid: $(docker exec "$host" stat -c '%A' "$base")" ;; esac
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base/demo-local/secrets.env")" = "$secret_before" ] || fail 'source add changed the unversioned secrets file'
	[ "$(docker exec "$host" git -C "$base" config core.sharedRepository)" = group ] || fail 'core.sharedRepository is not group'
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /srv/qd-local'
	list_has_path "$base" || fail "source list does not show the local folder: $(qd --output json source list)"
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the local source has the wrong kind'

	step 'demo-local is deployed at 1.0.0'
	i=0
	until stack_settled demo-local succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-local last_run)" != failed ] || fail "the first run of demo-local failed: $(qd stack log "$(newest_run demo-local)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-local: $(qd --output json status | tr ',' '\n' | grep -e qd-local -e demo-local)"
		sleep 1
	done
	[ "$(stack_images demo-local)" = "$image:1.0.0" ] || fail "demo-local does not run 1.0.0: $(stack_images demo-local)"

	step 'the first run of the running demo-live neither stops nor recreates it, and a stack with backup rules still stops before its backup'
	i=0
	until stack_settled demo-live succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-live last_run)" != failed ] || fail "the first run of demo-live failed: $(qd stack log "$(newest_run demo-live)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-live: $(qd --output json status | tr ',' '\n' | grep -e qd-local -e demo-live)"
		sleep 1
	done
	run_log=$(qd stack log "$(newest_run demo-live)")
	[ "$(log_steps "$run_log")" = backup,pull,up,check ] || fail "unexpected steps of the first run of demo-live: $(log_steps "$run_log")"
	if printf '%s\n' "$run_log" | grep -q '^[^ ]* step stop$'; then fail "the first run of demo-live stopped the stack: $run_log"; fi
	[ "$(live_containers)" = "$live_before" ] || fail "the first run changed the containers of demo-live: before $live_before, now $(live_containers)"
	# The runs of demo-stack with its backup rules keep the stop before the backup: see the good run of scenario_backup.

	step 'a person in the group qhost-deploy commits a version; git refuses the folder until it is marked safe'
	docker exec "$host" usermod -aG qhost-deploy "$admin" || fail 'cannot add the admin to the group qhost-deploy'
	as_admin 'id -nG' | tr ' ' '\n' | grep -qx qhost-deploy || fail "the admin has no group qhost-deploy in a new session: $(as_admin 'id -nG')"
	out=$(local_git rev-parse HEAD) && fail "git accepted a folder of another owner without safe.directory: $out"
	printf '%s' "$out" | grep -q 'dubious ownership' || fail "git refused the folder for another reason: $out"
	as_admin "git config --global --add safe.directory $base" || fail 'cannot mark the folder safe'
	local_git rev-parse HEAD >/dev/null || fail "git still refuses the folder: $(local_git rev-parse HEAD)"
	out=$(as_admin "umask 022; mkdir $base/demo-local/probe && stat -c %A $base/demo-local/probe && rmdir $base/demo-local/probe")
	case $out in d???r-s*) ;; *) fail "a folder made with umask 022 is group-writable: $out" ;; esac
	prev=$(newest_run demo-local)
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' demo-local/.env &&
		mkdir demo-local/notes && echo 'kept by a person' >demo-local/notes/info.txt &&
		stat -c %A demo-local/notes | grep -q '^d...rws' && git add -A &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Update demo-local to 1.1.0'" ||
		fail 'the admin could not commit a version in the local folder'
	good_local=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	restart_service
	run=$(next_run demo-local "$prev") || fail 'no run of demo-local started for the version change'
	wait_for "the run $run" 240 run_finished demo-local "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-local deployed)" = "$good_local" ] || fail "demo-local is not recorded as deployed at $good_local"
	[ "$(stack_images demo-local)" = "$image:1.1.0" ] || fail "demo-local does not run 1.1.0: $(stack_images demo-local)"

	step 'a failing version is rolled back by a revert commit of the service, and the folder has no remote'
	prev=$run
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' demo-local/.env &&
		echo 'edited for 2.0.0' >>demo-local/notes/info.txt &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-local to 2.0.0 with a broken healthcheck'" ||
		fail 'the admin could not commit the failing version'
	bad_local=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	restart_service
	run=$(next_run demo-local "$prev") || fail 'no run of demo-local started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-local "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	[ "$revert" != "$bad_local" ] || fail 'no revert commit was made in the local folder'
	meta=$(docker exec "$host" git -C "$base" log -1 --format='%an|%s|%P' "$revert")
	[ "$meta" = "qhost|Revert demo-local to $(printf '%s' "$good_local" | cut -c 1-12)|$bad_local" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ -z "$(docker exec "$host" git -C "$base" remote)" ] || fail 'the rollback gave the local repository a remote'
	docker exec "$host" grep -qx DEMO_APP_VERSION=1.1.0 "$base/demo-local/.env" || fail 'the revert did not restore the version file'
	[ "$(docker exec "$host" cat "$base/demo-local/notes/info.txt")" = 'kept by a person' ] || fail 'the revert did not restore the folder of the person'
	[ "$(stack_images demo-local)" = "$image:1.1.0" ] || fail "demo-local does not run the previous image 1.1.0: $(stack_images demo-local)"

	step 'source remove asks first, then releases only the binding'
	containers_before=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-local | sort)
	[ -n "$containers_before" ] || fail 'demo-local has no running container'
	tree_before=$(local_tree)
	out=$(qd_fails source remove "$base") || fail "source remove without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source remove without --yes does not ask for confirmation: $out"
	list_has_path "$base" || fail 'a refused source remove released the folder'
	out=$(qd --output json source remove "$base" --yes) || fail "source remove failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.id == "srv-qd-local" and .path == "/srv/qd-local"' >/dev/null || fail "unexpected source remove result: $out"
	[ "$(local_tree)" = "$tree_before" ] || fail 'source remove changed a file of the folder'
	[ "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-local | sort)" = "$containers_before" ] ||
		fail 'source remove stopped or replaced a container of demo-local'
	if list_has_path "$base"; then fail "source list still shows the released folder: $(qd --output json source list)"; fi
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'source remove took the folder out of the drop-in'
	out=$(qd_fails source remove "$base" --yes) || fail "source remove of an unbound folder succeeded: $out"
	printf '%s' "$out" | grep -q 'not-found' || fail "source remove of an unbound folder is no not-found error: $out"
	log 'scenario: a local repository was bound with group access, deployed, rolled back by a local revert and released without a change to its files'

	step 'an existing clone is released and taken over again with its origin and its deploy key'
	key2_before=$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")
	[ -n "$key2_before" ] || fail 'the clone has no deploy key before it is released'
	out=$(qd --output json source remove /opt/stacks2 --yes) || fail "source remove of /opt/stacks2 failed: $out"
	if list_has_path /opt/stacks2; then fail 'source list still shows /opt/stacks2'; fi
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$key2_before" ] || fail 'releasing /opt/stacks2 changed its deploy key'
	[ "$(stack_images demo-second)" = "$image:1.0.0" ] || fail 'releasing /opt/stacks2 stopped demo-second'
	head2=$(docker exec "$host" git -c safe.directory=/opt/stacks2 -C /opt/stacks2 rev-parse HEAD)
	out=$(qd --output json source add /opt/stacks2 --yes --fingerprint "$(git_ed25519_fingerprint)") || fail "source add of the clone failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e --arg u "$url2" '.url == $u and .cloned == false and .dir == "/opt/stacks2"' >/dev/null ||
		fail "source add did not take over the clone with its origin: $out"
	wait_for 'the deploy service to answer' 30 qd status
	# The engine state of the source went with the release, so the stack is deployed once more.
	wait_for 'the run of demo-second after the takeover' 180 stack_settled demo-second succeeded
	[ "$(docker exec "$host" git -c safe.directory=/opt/stacks2 -C /opt/stacks2 rev-parse HEAD)" = "$head2" ] || fail 'taking over the clone changed its head'
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$key2_before" ] || fail 'taking over /opt/stacks2 changed its deploy key'
	qd --output json source list | jq -e '[.sources[] | select(.path == "/opt/stacks2" and .kind == "remote")] | length == 1' >/dev/null ||
		fail "source list does not show /opt/stacks2 again: $(qd --output json source list)"
	[ "$(stack_images demo-second)" = "$image:1.0.0" ] || fail 'demo-second does not run after /opt/stacks2 is taken over again'
	[ "$(stack_field demo-second source)" = opt-stacks2 ] || fail "demo-second is not listed with its source: $(qd --output json status)"
	log 'scenario: /opt/stacks2 was released and taken over again with its origin, head and deploy key'
}

# files_state prints the content hash of every file below /srv/qd-files/web except .git, and the type, owner and size
# of every path; modes and groups are left out because source add changes them for the listed files.
files_state() {
	docker exec "$host" sh -ec "cd /srv/qd-files/web && find . -path ./.git -prune -o -type f -exec sha256sum {} + | sort -k2 &&
		find . -path ./.git -prune -o -printf '%p %y %u %s\n' | sort"
}

# files_git runs git on /srv/qd-files/web as root; the folder is marked safe because the service owns .git; stderr is merged.
files_git() {
	docker exec "$host" git -c safe.directory=/srv/qd-files/web -C /srv/qd-files/web "$@" 2>&1
}

# scenario_source_files proves source add on a folder of files without a repository below /srv/qd-files, which no
# stack folder uses: the answer lists the files and what is left out, nothing changes without the confirmation, the
# confirmed add starts a repository with one commit of the listed files and leaves every file as it was, a version
# committed by a person is deployed and a failing one is rolled back by a local revert commit.
scenario_source_files() {
	base=/srv/qd-files/web
	image=registry.$domain/demo/app

	step 'a folder of files without a repository is prepared as root, with a large file the first commit leaves out'
	docker exec "$host" sh -ec "rm -rf /srv/qd-files && mkdir -p '$base' && chmod 0755 /srv/qd-files '$base'" || fail 'cannot create the folder of files'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/compose.yaml'"
name: demo-files

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-files
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	docker exec "$host" sh -ec "head -c 2097152 /dev/zero >'$base/big.bin' && chmod 0644 '$base/big.bin' '$base/compose.yaml' '$base/.env'" ||
		fail 'cannot create the large file'
	[ -z "$(docker exec "$host" ls -A "$base" | grep -x .git)" ] || fail 'the folder of files has a .git'

	step 'source inspect names the files to take, the large file with its reason and the stack'
	out=$(qd --output json source inspect "$base") || fail "source inspect of the folder of files failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files | (index("compose.yaml") != null and index(".env") != null)) and
		([.excluded[] | select(.path == "big.bin" and .reason == "too-large")] | length) == 1 and
		([.stacks[].name] | index("demo-files")) != null' >/dev/null || fail "unexpected inspection of the folder of files: $out"

	step 'source add without a terminal and without --yes is refused and changes nothing'
	dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$base")
	out=$(qd_fails source add "$base") || fail "source add of the files without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add of the files without --yes does not ask for confirmation: $out"
	if docker exec "$host" test -e "$base/.git"; then fail 'a refused source add made a repository'; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base")" = "$dir_before" ] || fail 'a refused source add changed the folder'
	if docker exec "$host" grep -qF "$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'a refused source add wrote the drop-in'
	fi
	if list_has_path "$base"; then fail 'a refused source add bound the folder'; fi

	step 'source add --yes starts a repository with the listed files and changes no file content'
	state_before=$(files_state)
	big_before=$(docker exec "$host" stat -c '%a %U:%G' "$base/big.bin")
	# qd merges stderr, where the preview and the notices go; the JSON answer is the last line.
	out=$(qd --output json source add "$base" --yes) || fail "source add of the files failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .path == "/srv/qd-files/web" and .files == 2' >/dev/null ||
		fail "unexpected source add result for the files: $out"
	wait_for 'the deploy service to answer' 30 qd status
	docker exec "$host" test -d "$base/.git" || fail 'source add made no repository'
	if docker exec "$host" test -e "$base/.gitignore"; then fail 'source add wrote a .gitignore'; fi
	docker exec "$host" grep -qx '/big.bin' "$base/.git/info/exclude" || fail "the large file is not in .git/info/exclude: $(docker exec "$host" cat "$base/.git/info/exclude")"
	[ "$(files_git rev-list --count HEAD)" = 1 ] || fail "the repository does not have exactly one commit: $(files_git log --oneline)"
	[ "$(files_git log -1 --format=%an)" = qhost ] || fail "the first commit has the wrong author: $(files_git log -1 --format=%an)"
	[ "$(files_git symbolic-ref --short HEAD)" = main ] || fail "the branch is not main: $(files_git symbolic-ref --short HEAD)"
	[ "$(files_git ls-files)" = "$(printf '.env\ncompose.yaml')" ] || fail "the first commit holds the wrong files: $(files_git ls-files)"
	[ "$(files_git config core.sharedRepository)" = group ] || fail 'core.sharedRepository is not group'
	[ -z "$(files_git remote)" ] || fail 'the new repository has a remote'
	[ "$(files_state)" = "$state_before" ] || fail 'source add changed the content, the owner or the size of a file'
	[ "$(docker exec "$host" stat -c '%a %U:%G' "$base/big.bin")" = "$big_before" ] || fail 'source add changed the large file that is not listed'
	[ "$(docker exec "$host" stat -c '%U:%G' "$base")" = root:qhost-deploy ] || fail "the folder has the wrong owner or group: $(docker exec "$host" stat -c '%U:%G' "$base")"
	case $(docker exec "$host" stat -c '%A' "$base") in d???rws*) ;; *) fail "the folder is not group-writable with setgid: $(docker exec "$host" stat -c '%A' "$base")" ;; esac
	for f in compose.yaml .env; do
		[ "$(docker exec "$host" stat -c '%G' "$base/$f")" = qhost-deploy ] || fail "$f does not have the group qhost-deploy"
		case $(docker exec "$host" stat -c '%A' "$base/$f") in -???rw*) ;; *) fail "$f is not group-writable: $(docker exec "$host" stat -c '%A' "$base/$f")" ;; esac
	done
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /srv/qd-files/web'
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the folder of files is not a local source'

	step 'demo-files is deployed at 1.0.0'
	i=0
	until stack_settled demo-files succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-files last_run)" != failed ] || fail "the first run of demo-files failed: $(qd stack log "$(newest_run demo-files)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-files: $(qd --output json status | tr ',' '\n' | grep -e qd-files -e demo-files)"
		sleep 1
	done
	[ "$(stack_images demo-files)" = "$image:1.0.0" ] || fail "demo-files does not run 1.0.0: $(stack_images demo-files)"

	step 'a person in the group qhost-deploy commits a version, which is deployed'
	as_admin "git config --global --add safe.directory $base" || fail 'cannot mark the folder of files safe'
	prev=$(newest_run demo-files)
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' .env &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-files to 1.1.0'" ||
		fail 'the admin could not commit a version in the folder of files'
	good_files=$(files_git rev-parse HEAD)
	restart_service
	run=$(next_run demo-files "$prev") || fail 'no run of demo-files started for the version change'
	wait_for "the run $run" 240 run_finished demo-files "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-files deployed)" = "$good_files" ] || fail "demo-files is not recorded as deployed at $good_files"
	[ "$(stack_images demo-files)" = "$image:1.1.0" ] || fail "demo-files does not run 1.1.0: $(stack_images demo-files)"

	step 'a failing version is rolled back by a revert commit of the service, and the folder has no remote'
	prev=$run
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' .env &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-files to 2.0.0 with a broken healthcheck'" ||
		fail 'the admin could not commit the failing version'
	bad_files=$(files_git rev-parse HEAD)
	restart_service
	run=$(next_run demo-files "$prev") || fail 'no run of demo-files started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-files "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(files_git rev-parse HEAD)
	[ "$revert" != "$bad_files" ] || fail 'no revert commit was made in the folder of files'
	meta=$(files_git log -1 --format='%an|%s|%P' "$revert")
	[ "$meta" = "qhost|Revert the stack folder to $(printf '%s' "$good_files" | cut -c 1-12)|$bad_files" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ -z "$(files_git remote)" ] || fail 'the rollback gave the repository a remote'
	[ "$(stack_images demo-files)" = "$image:1.1.0" ] || fail "demo-files does not run the previous image 1.1.0: $(stack_images demo-files)"
	log 'scenario: a folder of files got a repository with group access, deployed 1.1.0 and rolled back 2.0.0 by a local revert'
}

# register_folder_key creates the deploy key of the folder $1 and enters its public part at the git server.
register_folder_key() {
	out=$(qd --output json key generate --dir "$1") || fail "key generate --dir $1 failed: $out"
	pub=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	printf '%s\n' "$pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
}

# remote_head prints the commit of refs/heads/main at the git server for the repository address $1 (empty without).
remote_head() {
	as_admin "git ls-remote $1 refs/heads/main" | cut -f 1
}

# folder_git runs git on the folder $1 as root, marked safe because the service owns .git; stderr is merged.
folder_git() {
	dir=$1
	shift
	docker exec "$host" git -c "safe.directory=$dir" -C "$dir" "$@" 2>&1
}

# tree_sums prints the checksum of every file of the folder $1 outside .git.
tree_sums() {
	docker exec "$host" sh -ec "cd '$1' && find . -path ./.git -prune -o -type f -exec sha256sum {} + | sort -k2"
}

# copy_stacks3 copies the tracked files of /opt/stacks3 without .git to the folder $1, readable by the service.
copy_stacks3() {
	docker exec "$host" sh -ec "rm -rf '$1' && mkdir -p '$1' &&
		git -c safe.directory=/opt/stacks3 -C /opt/stacks3 ls-files -z | tar -C /opt/stacks3 --null -T - -cf - | tar -C '$1' -xf - &&
		chmod -R a+rX '$1'" || fail "cannot copy /opt/stacks3 to $1"
}

# scenario_source_remote proves source remote and source add --url. The folder of files of the scenario before
# becomes a remote source by pushing its history to an empty repository. A repository whose history once held a
# secrets file is not pushed, however it looks now. A folder of files with the files of /opt/stacks3 takes over
# the history of that repository, and one whose .env differs starts on it and is not touched without the
# confirmation. Nothing here pushes with force.
scenario_source_remote() {
	base=/srv/qd-files/web
	url4=git.$domain:stacks4.git
	url5=git.$domain:stacks5.git
	url3=git.$domain:stacks3.git

	step 'an empty remote: source remote without the confirmation changes nothing'
	register_folder_key "$base"
	run_before=$(newest_run demo-files)
	[ -n "$run_before" ] || fail 'demo-files has no run'
	out=$(qd_fails source remote "$base" "$url4" --fingerprint "$(git_ed25519_fingerprint)") || fail "source remote without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source remote without --yes does not ask for confirmation: $out"
	[ -z "$(files_git remote)" ] || fail 'a refused source remote gave the folder a remote'
	[ -z "$(remote_head "$url4")" ] || fail 'a refused source remote pushed'
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'a refused source remote changed the kind of the source'

	step 'source remote --yes pushes the local history to the empty repository'
	head_before=$(files_git rev-parse HEAD)
	out=$(qd --output json source remote "$base" "$url4" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source remote failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e --arg u "$url4" --arg h "$head_before" \
		'.state == "empty" and .pushed == true and .branch == "main" and .url == $u and (.head | length) >= 7 and (.head as $x | $h | startswith($x))' >/dev/null ||
		fail "unexpected source remote result: $out"
	[ "$(remote_head "$url4")" = "$head_before" ] || fail "the repository does not hold the head of the folder: $(remote_head "$url4")"
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | "\(.kind) \(.url)"')" = "remote $url4" ] ||
		fail "the folder is not a remote source with the address: $(qd --output json source list)"
	[ "$(files_git rev-parse HEAD)" = "$head_before" ] || fail 'source remote moved the head of the folder'
	[ "$(newest_run demo-files)" = "$run_before" ] || fail 'source remote started a run of demo-files'
	[ "$(stack_images demo-files)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-files does not run 1.1.0 any more: $(stack_images demo-files)"

	step 'a history that once held a secrets file is not pushed'
	docker exec "$host" sh -ec "rm -rf /srv/qd-secret && mkdir -p /srv/qd-secret/demo-secret && chmod 0755 /srv/qd-secret /srv/qd-secret/demo-secret" ||
		fail 'cannot create the secret folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >/srv/qd-secret/demo-secret/compose.yaml"
name: demo-secret

services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-secret
      DEMO_VERSION: 1.0.0
      DEMO_HEALTH: ok
EOF2
	printf 'DEMO_SECRET=not-a-real-secret\n' | docker exec -i "$host" sh -ec "cat >/srv/qd-secret/demo-secret/secrets.env"
	docker exec "$host" sh -ec "cd /srv/qd-secret && chmod 0644 demo-secret/compose.yaml demo-secret/secrets.env && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-secret stack with a secrets file' &&
		git rm -q demo-secret/secrets.env &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Remove the secrets file'" ||
		fail 'cannot create the secret repository'
	first_secret=$(docker exec "$host" git -C /srv/qd-secret rev-list --max-parents=0 HEAD | cut -c 1-7)
	[ -n "$first_secret" ] || fail 'the secret repository has no first commit'
	[ -z "$(docker exec "$host" git -C /srv/qd-secret ls-files | grep secrets.env)" ] || fail 'the secrets file is still in the last commit'
	out=$(qd --output json source add /srv/qd-secret --yes) || fail "source add of the secret repository failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local"' >/dev/null || fail "unexpected source add result: $out"
	wait_for 'the deploy service to answer' 30 qd status
	register_folder_key /srv/qd-secret
	out=$(qd_fails source remote /srv/qd-secret "$url5" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source remote pushed secrets: $out"
	printf '%s' "$out" | grep -q 'secrets-found' || fail "source remote does not report secrets-found: $out"
	printf '%s' "$out" | grep -q "$first_secret" || fail "the output does not name the commit $first_secret: $out"
	printf '%s' "$out" | grep -q 'secrets.env' || fail "the output does not name secrets.env: $out"
	if printf '%s' "$out" | grep -q 'not-a-real-secret'; then fail 'the output shows the value of the secret'; fi
	[ "$(qd --output json source list | jq -r '.sources[] | select(.path == "/srv/qd-secret") | .kind')" = local ] || fail 'the refused source remote changed the kind of the source'
	[ -z "$(docker exec "$host" git -C /srv/qd-secret remote)" ] || fail 'the refused source remote gave the folder a remote'
	[ -z "$(as_admin "git ls-remote $url5")" ] || fail "the refused push created a branch: $(as_admin "git ls-remote $url5")"
	wait_for 'the first run of demo-secret' 180 stack_settled demo-secret succeeded
	out=$(qd --output json source remove /srv/qd-secret --yes) || fail "source remove of the secret folder failed: $out"
	docker exec "$host" docker compose -p demo-secret down >/dev/null 2>&1 || fail 'cannot remove the containers of demo-secret'
	log 'scenario: source remote pushed the folder of files to an empty repository and refused a history with a secrets file'

	step 'the files of /opt/stacks3 are checked against its repository before they are copied'
	repo_head3=$(remote_head "$url3")
	[ -n "$repo_head3" ] || fail 'stacks3 has no head'
	[ "$(docker exec "$host" git -c safe.directory=/opt/stacks3 -C /opt/stacks3 rev-parse HEAD)" = "$repo_head3" ] ||
		fail 'the head of /opt/stacks3 is not the head of its repository'
	[ -z "$(docker exec "$host" git -c safe.directory=/opt/stacks3 -C /opt/stacks3 status --porcelain)" ] || fail '/opt/stacks3 has local changes'

	step 'a folder of files equal to the repository takes over its history without a push'
	equal=/srv/qd-b-equal
	copy_stacks3 "$equal"
	register_folder_key "$equal"
	out=$(qd --output json source inspect "$equal" --url "$url3") || fail "source inspect of the equal folder failed: $out"
	printf '%s' "$out" | jq -e '.case == "files"' >/dev/null || fail "the equal folder is not a folder of files: $out"
	sums_before=$(tree_sums "$equal")
	out=$(qd --output json source add "$equal" --url "$url3" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source add --url for the equal folder failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "remote" and .state == "equal" and .pushed == false' >/dev/null ||
		fail "unexpected source add result for the equal folder: $out"
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(folder_git "$equal" rev-parse HEAD)" = "$repo_head3" ] || fail "the head of the folder is not the head of the repository: $(folder_git "$equal" rev-parse HEAD)"
	[ "$(folder_git "$equal" remote)" = origin ] || fail "the folder has not exactly the remote origin: $(folder_git "$equal" remote)"
	folder_git "$equal" remote get-url origin | grep -q 'stacks3.git' || fail "origin is not the repository: $(folder_git "$equal" remote get-url origin)"
	[ "$(qd --output json source list | jq -r --arg p "$equal" '.sources[] | select(.path == $p) | .kind')" = remote ] || fail 'the equal folder is not a remote source'
	[ "$(tree_sums "$equal")" = "$sums_before" ] || fail 'the takeover changed the content of a file'
	[ "$(remote_head "$url3")" = "$repo_head3" ] || fail 'taking over an equal remote changed it'
	out=$(qd --output json source remove "$equal" --yes) || fail "source remove of the equal folder failed: $out"

	step 'a folder of files that differs is shown and changes nothing without the confirmation'
	diff_dir=/srv/qd-b-diff
	copy_stacks3 "$diff_dir"
	docker exec "$host" sed -i 's/^DEMO_HEALTH=.*/DEMO_HEALTH=degraded/' "$diff_dir/.env"
	docker exec "$host" grep -qx 'DEMO_HEALTH=degraded' "$diff_dir/.env" || fail 'cannot change the .env of the differing folder'
	register_folder_key "$diff_dir"
	meta_before=$(docker exec "$host" stat -c '%U:%G %a' "$diff_dir")
	out=$(qd_fails source add "$diff_dir" --url "$url3" --fingerprint "$(git_ed25519_fingerprint)") || fail "source add --url for the differing folder without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add --url without --yes does not ask for confirmation: $out"
	printf '%s' "$out" | grep -q '\.env' || fail "the output does not name .env as a differing path: $out"
	if docker exec "$host" test -e "$diff_dir/.git"; then fail 'a refused source add made a repository'; fi
	if list_has_path "$diff_dir"; then fail 'a refused source add bound the folder'; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$diff_dir")" = "$meta_before" ] || fail 'a refused source add changed the owner, group or mode of the folder'
	[ "$(remote_head "$url3")" = "$repo_head3" ] || fail 'a refused source add changed the repository'
	docker exec "$host" rm -rf "$diff_dir" /srv/qd-secret "$equal"
	log 'scenario: source add --url took over the history of an equal repository and left a differing folder alone'
}

# scenario_tui_remote proves the remote of a local stack folder in the Stacks area of the interface. A local folder
# below /srv/qd-tr-a has no deploy key: the interface leads to the key view, the generated key is entered at the git
# server, and the folder then pushes its history to an empty repository after the confirmation. A local folder below
# /srv/qd-tr-b is compared with the repository of /opt/stacks3, which holds other files: the paths are listed and the
# answer to cancel leaves the folder, the service and the repository as they were.
scenario_tui_remote() {
	tui_log=$work/tui-remote-screens.log
	: >"$tui_log"
	tr_a=/srv/qd-tr-a
	tr_b=/srv/qd-tr-b
	tr_url6=git.$domain:stacks6.git
	tr_url3=git.$domain:stacks3.git

	step 'two local folders without a remote are bound'
	for tr_dir in "$tr_a" "$tr_b"; do
		docker exec "$host" sh -ec "rm -rf '$tr_dir' && mkdir -p '$tr_dir/notes' && chmod 0755 '$tr_dir' '$tr_dir/notes' &&
			printf 'notes of %s\n' '$tr_dir' >'$tr_dir/notes/readme.txt' && chmod 0644 '$tr_dir/notes/readme.txt' &&
			cd '$tr_dir' && git init -q -b main && git add -A &&
			git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the notes'" ||
			fail "cannot create the local folder $tr_dir"
		out=$(qd --output json source add "$tr_dir" --yes) || fail "source add of $tr_dir failed: $out"
		printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local"' >/dev/null || fail "unexpected source add result: $out"
		wait_for 'the deploy service to answer' 30 qd status
	done
	repo_head3=$(remote_head "$tr_url3")
	[ -n "$repo_head3" ] || fail 'stacks3 has no head'
	[ -z "$(remote_head "$tr_url6")" ] || fail 'stacks6 is not empty'
	tr_head_a=$(folder_git "$tr_a" rev-parse HEAD)

	step 'the interface leads to the key view for a folder without a deploy key and compares nothing before it exists'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_folder_to "$tr_a"
	tui_wait 'add remote' 10
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_has "$(git_ed25519_fingerprint)" || fail "the host key question does not show the fingerprint: $(tui_screen)"
	tui_send y
	tui_wait 'has no deploy key yet' 30
	tui_send Enter
	tui_wait "stack folder: $tr_a" 10
	tui_key_generate
	tr_out=$(qd --output json key show --dir "$tr_a") || fail "key show --dir $tr_a failed: $tr_out"
	tr_pub=$(key_pub_field "$tr_out")
	case $tr_pub in ssh-ed25519\ *) ;; *) fail "no public key of $tr_a: $tr_out" ;; esac
	[ -z "$(remote_head "$tr_url6")" ] || fail 'the interface pushed before the question'
	[ -z "$(folder_git "$tr_a" remote)" ] || fail 'the interface gave the folder a remote before the question'
	printf '%s\n' "$tr_pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'

	step 'with the key in place the empty repository is compared, n changes nothing and y pushes'
	tui_send Escape
	tui_folder_to "$tr_a"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Push the history of' 30
	tui_has "$tr_a" || fail "the question does not name the folder: $(tui_screen)"
	tui_has 'branch: main' || fail "the comparison does not show the branch: $(tui_screen)"
	tui_send n
	sleep 1
	[ -z "$(remote_head "$tr_url6")" ] || fail 'the answer n pushed'
	[ -z "$(folder_git "$tr_a" remote)" ] || fail 'the answer n gave the folder a remote'
	tui_folder_to "$tr_a"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Push the history of' 30
	tui_send y
	tui_wait 'runs against' 60
	tui_send Enter
	[ "$(remote_head "$tr_url6")" = "$tr_head_a" ] || fail "the repository does not hold the head of the folder: $(remote_head "$tr_url6")"
	[ "$(qd --output json source list | jq -r --arg p "$tr_a" '.sources[] | select(.path == $p) | .kind')" = remote ] ||
		fail "the folder is not a remote source: $(qd --output json source list)"
	[ "$(folder_git "$tr_a" rev-parse HEAD)" = "$tr_head_a" ] || fail 'the push moved the head of the folder'

	step 'a repository with other files is listed and cancel changes nothing'
	register_folder_key "$tr_b"
	tui_folder_to "$tr_b"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url3"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Put the state of the host' 30
	tui_has 'Paths that differ' || fail "the comparison does not list the paths: $(tui_screen)"
	tui_has "$tr_b" || fail "the question does not name the folder: $(tui_screen)"
	tui_send Enter
	sleep 1
	[ "$(remote_head "$tr_url3")" = "$repo_head3" ] || fail 'the answer cancel changed the repository'
	[ -z "$(folder_git "$tr_b" remote)" ] || fail 'the answer cancel gave the folder a remote'
	[ "$(qd --output json source list | jq -r --arg p "$tr_b" '.sources[] | select(.path == $p) | .kind')" = local ] ||
		fail 'the answer cancel changed the kind of the source'
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'the folders are released and removed'
	for tr_dir in "$tr_a" "$tr_b"; do
		out=$(qd --output json source remove "$tr_dir" --yes) || fail "source remove of $tr_dir failed: $out"
	done
	docker exec "$host" rm -rf "$tr_a" "$tr_b"
	wait_for 'the deploy service to answer' 30 qd status
	log 'scenario: the interface pushed a local folder to an empty remote and left a differing remote alone without the confirmation'
}

# key_pub_field prints the public part (type and key, without comment) of the qhost key output $1.
key_pub_field() {
	printf '%s\n' "$1" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p' | cut -d ' ' -f 1,2
}

# key_fp_field prints the fingerprint of the qhost key output $1.
key_fp_field() {
	printf '%s\n' "$1" | sed -n 's/.*"fingerprint": *"\([^"]*\)".*/\1/p'
}

# git_unregister_key removes the public key with the base64 part $1 from the authorized keys of the git server, so
# a later fetch proves the replacement key.
git_unregister_key() {
	docker exec "$prefix-git" sh -ec 'grep -vF -- "$1" /srv/git/.ssh/authorized_keys >/tmp/ak.new || true
		cat /tmp/ak.new >/srv/git/.ssh/authorized_keys && rm -f /tmp/ak.new' sh "$1"
}

# scenario_key_set proves that an existing key becomes the deploy key of a stack folder, through the command line
# for /opt/stacks2 and through the Stacks area for /opt/stacks3, which is released at the end. Both folders have a generated key, so each store is
# a replacement: it is refused without the confirmation (--replace, the answer n) and leaves the key unchanged.
scenario_key_set() {
	tui_log=$work/tui-key-screens.log
	: >"$tui_log"
	kdir=$(docker exec "$host" mktemp -d) || fail 'cannot create a directory for the test keys'
	docker exec "$host" sh -ec "ssh-keygen -q -t ed25519 -N '' -C key-a-e2e -f $kdir/a && ssh-keygen -q -t ed25519 -N '' -C key-b-e2e -f $kdir/b" ||
		fail 'cannot generate the test keys'
	fp_a=$(docker exec "$host" ssh-keygen -lf "$kdir/a.pub" | cut -d ' ' -f 2)
	fp_b=$(docker exec "$host" ssh-keygen -lf "$kdir/b.pub" | cut -d ' ' -f 2)
	body_a=$(docker exec "$host" sed -n 2p "$kdir/a")
	body_b=$(docker exec "$host" sed -n 2p "$kdir/b")
	pub_a=$(docker exec "$host" cut -d ' ' -f 1,2 "$kdir/a.pub")
	pub_b=$(docker exec "$host" cut -d ' ' -f 1,2 "$kdir/b.pub")
	case $fp_a$fp_b in SHA256:*SHA256:*) ;; *) fail "no fingerprints of the test keys: $fp_a $fp_b" ;; esac
	[ -n "$body_a" ] && [ -n "$body_b" ] || fail 'the test keys have no body line'

	step 'key set stores an existing key for /opt/stacks2 only with --replace; a fetch over it succeeds'
	show2=$(qd --output json key show --dir /opt/stacks2) || fail "key show --dir failed: $show2"
	old2=$(key_pub_field "$show2")
	out=$(qd_fails --output json key set --dir /opt/stacks2 --file "$kdir/a") || fail "key set replaced a key without --replace: $out"
	printf '%s' "$out" | grep -q -- '--replace' || fail "the refusal does not name --replace: $out"
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$old2" ] || fail 'a refused key set changed the key'
	out=$(qd --output json key set --dir /opt/stacks2 --file "$kdir/a" --replace) || fail "key set --replace failed: $out"
	[ "$(key_fp_field "$out")" = "$fp_a" ] || fail "the fingerprint of the stored key differs from ssh-keygen: $out vs $fp_a"
	[ "$(key_pub_field "$out")" = "$pub_a" ] || fail "the public key of the stored key differs from the test key: $out"
	[ "$(key_fp_field "$(qd --output json key show --dir /opt/stacks2)")" = "$fp_a" ] || fail 'key show --dir does not show the stored key'
	git_unregister_key "${old2#* }"
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks2.error' || fail "the old key is gone from the git server, yet opt-stacks2 fetched: $out"
	printf '%s\n' "$pub_a" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	if printf '%s' "$out" | grep -q 'opt-stacks2.error'; then fail "opt-stacks2 does not fetch over the stored key: $out"; fi
	log "scenario: key set stored the test key for /opt/stacks2 ($fp_a) and the fetch over it succeeds"

	step 'the interface stores an existing key for /opt/stacks3 after the question; n keeps the key'
	old3=$(key_pub_field "$(qd --output json key show --dir /opt/stacks3)")
	fp_old3=$(key_fp_field "$(qd --output json key show --dir /opt/stacks3)")
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_folder_to /opt/stacks3
	tui_send e
	tui_wait 'stack folder: /opt/stacks3' 10
	tui_send e
	tui_wait 'store an existing key' 10
	tui_send F2
	tui_wait 'Store the deploy key' 10
	docker exec "$host" cat "$kdir/b" | docker exec -i -u "$admin" "$host" tmux -L qdtui load-buffer - || fail 'cannot load the test key into tmux'
	tui_tmux paste-buffer -p -t qdtui || fail 'cannot paste the test key'
	tui_wait '(hidden; lines: ' 10
	tui_has "$body_b" && fail "the pasted private key is on the screen: $(tui_screen)"
	tui_send F2
	tui_wait 'Replace the existing deploy key?' 10
	tui_send n
	sleep 0.5
	tui_has 'Store the deploy key' || fail "answering n closed the form: $(tui_screen)"
	[ "$(key_fp_field "$(qd --output json key show --dir /opt/stacks3)")" = "$fp_old3" ] || fail 'the answer n changed the key of /opt/stacks3'
	tui_send F2
	tui_wait 'Replace the existing deploy key?' 10
	tui_send y
	tui_wait 'The deploy key is stored' 30
	tui_wait "$fp_b" 10
	tui_has "$body_b" && fail "the private key is on the screen: $(tui_screen)"
	tui_send Enter
	out=$(qd --output json key show --dir /opt/stacks3)
	[ "$(key_fp_field "$out")" = "$fp_b" ] || fail "the key of /opt/stacks3 is not the stored key: $out vs $fp_b"
	[ "$(key_pub_field "$out")" = "$pub_b" ] || fail "the public key of /opt/stacks3 differs from the test key: $out"
	git_unregister_key "${old3#* }"
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks3.error' || fail "the old key is gone from the git server, yet opt-stacks3 fetched: $out"
	printf '%s\n' "$pub_b" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	if printf '%s' "$out" | grep -q 'opt-stacks3.error'; then fail "opt-stacks3 does not fetch over the stored key: $out"; fi
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	log "scenario: the interface stored the test key for /opt/stacks3 ($fp_b) after the question, the fetch over it succeeds"

	step 'no private test key shows on a captured screen, in the journal or in a key output'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	journal_text=$(journal)
	for body in "$body_a" "$body_b"; do
		if grep -qF -- "$body" "$tui_log"; then fail 'a private test key shows on a captured screen'; fi
		if printf '%s' "$journal_text" | grep -qF -- "$body"; then fail 'a private test key shows in the journal'; fi
		if printf '%s' "$out$show2" | grep -qF -- "$body"; then fail 'a private test key shows in a key output'; fi
	done
	docker exec "$host" rm -rf "$kdir"
}

# scenario_containers proves that qhost container list and show tell the three kinds apart on the host: the
# deployed demo stack is own, a Compose project from a folder outside the stacks repository is foreign, and a
# container started with docker run is single. The planted environment value must not show anywhere. What the
# scenario starts is removed before the answers are checked, so a failed check leaves nothing behind.
scenario_containers() {
	step 'container list shows own stacks, foreign projects and single containers'
	image=registry.$domain/demo/app:1.0.0
	foreign=$prefix-foreign single=$prefix-single planted=invented-planted-value-e2e
	foreign_dir=/tmp/$foreign
	docker exec "$host" sh -ec "rm -rf '$foreign_dir' && mkdir -p '$foreign_dir'" || fail 'cannot prepare the foreign project'
	printf 'services:\n  app:\n    image: %s\n    labels:\n      - %s\n' "$image" "$label" |
		docker exec -i "$host" sh -ec "cat >'$foreign_dir/compose.yaml'" || fail 'cannot write the compose file of the foreign project'
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$single" --label "$label" -e "E2E_PLANTED=$planted" "$image" >/dev/null ||
		fail 'cannot start the single container'
	docker exec "$host" docker compose -f "$foreign_dir/compose.yaml" -p "$foreign" up -d >/dev/null 2>&1 ||
		fail 'cannot start the foreign project'
	wait_for 'the single container' 30 health_is "$single" healthy
	wait_for 'the foreign project' 30 stack_healthy "$foreign"

	list_rc=0 show_rc=0 table_rc=0 missing_rc=0
	list_json=$(qd --output json container list) || list_rc=$?
	show_json=$(qd --output json container show "$single") || show_rc=$?
	list_table=$(qd container list) || table_rc=$?
	missing_out=$(qd container show "$prefix-no-such-container") || missing_rc=$?
	docker exec "$host" docker compose -f "$foreign_dir/compose.yaml" -p "$foreign" down >/dev/null 2>&1 || true
	docker exec "$host" sh -c "docker rm -f '$single' >/dev/null 2>&1; rm -rf '$foreign_dir'"

	[ "$list_rc" -eq 0 ] || fail "container list failed: $list_json"
	[ "$show_rc" -eq 0 ] || fail "container show failed: $show_json"
	[ "$table_rc" -eq 0 ] || fail "container list (table) failed: $list_table"
	[ "$missing_rc" -ne 0 ] || fail "container show of a missing container succeeded: $missing_out"
	printf '%s' "$list_json" | grep -q '"kind": *"own", *"name": *"demo-stack"' || fail "demo-stack is not an own group: $list_json"
	printf '%s' "$list_json" | grep -q "\"kind\": *\"foreign\", *\"name\": *\"$foreign\"" || fail "$foreign is not a foreign group: $list_json"
	printf '%s' "$list_json" | grep -q "\"working_dir\": *\"$foreign_dir\"" || fail "the foreign group does not show its working directory: $list_json"
	printf '%s' "$list_json" | grep -q "\"kind\": *\"single\", *\"name\": *\"$single\"" || fail "$single is not a single group: $list_json"
	printf '%s' "$list_table" | grep -q "^single  *$single " || fail "the table does not show $single as single: $list_table"
	printf '%s' "$show_json" | grep -q '"E2E_PLANTED"' || fail "container show does not name the planted variable: $show_json"
	printf '%s' "$show_json" | grep -q '"kind": *"single"' || fail "container show does not name the kind: $show_json"
	printf '%s' "$missing_out" | grep -q 'no such container' || fail "the refusal of a missing container is unclear: $missing_out"
	for text in "$list_json" "$show_json" "$list_table"; do
		if printf '%s' "$text" | grep -qF -- "$planted"; then fail 'an environment value shows in the container output'; fi
	done
	log 'scenario: container list showed own, foreign and single; the planted value stayed hidden'
}

# scenario_container_logs proves that qhost container logs reads one container and a whole stack, follows until
# interrupted with exit code 0, and shows the stack's secret only masked although the demo app prints it.
scenario_container_logs() {
	step 'container logs show a container and a stack, follow ends cleanly, secrets stay masked'
	secret=not-a-real-secret-e2e
	name=$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-stack --format '{{.Names}}' | head -n 1)
	[ -n "$name" ] || fail 'no container of demo-stack runs'
	docker exec "$host" docker logs "$name" 2>&1 | grep -qF -- "$secret" || fail 'the demo app does not print its secret, so the mask cannot be checked'

	tail_rc=0 one_rc=0 stack_rc=0 follow_rc=0 json_rc=0 missing_rc=0
	tail_out=$(qd container logs "$name" --tail 20) || tail_rc=$?
	one_out=$(qd container logs "$name" --tail 0) || one_rc=$?
	stack_out=$(qd container logs --stack demo-stack --tail 0) || stack_rc=$?
	json_out=$(qd --output json container logs "$name" --tail 5) || json_rc=$?
	# --foreground: otherwise GNU timeout also signals the process group, so qhost gets the interrupt twice and the
	# second one can kill it (exit 130) after the first has ended the follow.
	follow_out=$(docker exec "$host" timeout --foreground --preserve-status -s INT 6 qhost container logs "$name" --tail 1 --follow 2>&1 </dev/null) || follow_rc=$?
	missing_out=$(qd container logs --stack "$prefix-no-such-stack") || missing_rc=$?

	[ "$tail_rc" -eq 0 ] || fail "container logs --tail 20 failed: $tail_out"
	[ "$(printf '%s\n' "$tail_out" | grep -c .)" -ge 1 ] || fail "container logs --tail 20 shows no line: $tail_out"
	[ "$one_rc" -eq 0 ] || fail "container logs failed: $one_out"
	[ "$stack_rc" -eq 0 ] || fail "container logs --stack failed: $stack_out"
	[ "$json_rc" -eq 0 ] || fail "container logs (json) failed: $json_out"
	[ "$follow_rc" -eq 0 ] || fail "container logs --follow did not end with exit code 0 after the interrupt ($follow_rc): $follow_out"
	[ "$missing_rc" -ne 0 ] || fail "container logs of a missing stack succeeded: $missing_out"
	printf '%s' "$one_out" | grep -q 'demo started' || fail "container logs lacks the start line: $one_out"
	printf '%s' "$one_out" | grep -q '\[stderr\] demo warning on stderr' || fail "container logs does not mark stderr: $one_out"
	printf '%s' "$stack_out" | grep -q "$name .*demo started" || fail "the stack log does not name the container: $stack_out"
	printf '%s' "$json_out" | grep -q '"stream": *"stdout"' || fail "the json log does not name the stream: $json_out"
	[ "$(printf '%s\n' "$follow_out" | grep -c .)" -ge 2 ] || fail "--follow showed no new line within six seconds: $follow_out"
	printf '%s' "$missing_out" | grep -q 'no such stack' || fail "the refusal of a missing stack is unclear: $missing_out"
	for text in "$tail_out" "$one_out" "$stack_out" "$json_out" "$follow_out"; do
		if printf '%s' "$text" | grep -qF -- "$secret"; then fail 'a secret value shows in the container logs'; fi
	done
	log 'scenario: container logs read a container and a stack, followed until the interrupt; the secret stayed masked'
}

# scenario_container_actions proves that qhost container stop, start and restart act on a single container and
# that the question needs --yes without a terminal. The container is removed before the answers are checked.
scenario_container_actions() {
	step 'container stop, start and restart act on a single container'
	image=registry.$domain/demo/app:1.0.0
	single=$prefix-single-act
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$single" --label "$label" "$image" >/dev/null ||
		fail 'cannot start the single container'
	wait_for 'the single container' 30 health_is "$single" healthy

	refuse_rc=0 stop_rc=0 start_rc=0 restart_rc=0 missing_rc=0
	refuse_out=$(qd container stop "$single") || refuse_rc=$?
	refuse_state=$(qd --output json container show "$single")
	stop_out=$(qd container stop "$single" --yes) || stop_rc=$?
	stop_state=$(qd --output json container show "$single")
	start_out=$(qd container start "$single" --yes) || start_rc=$?
	start_state=$(qd --output json container show "$single")
	started_before=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$single")
	restart_out=$(qd container restart "$single" --yes) || restart_rc=$?
	restart_state=$(qd --output json container show "$single")
	started_after=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$single")
	missing_out=$(qd container stop "$prefix-no-such-container" --yes) || missing_rc=$?
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1

	[ "$refuse_rc" -ne 0 ] || fail "container stop without --yes succeeded: $refuse_out"
	printf '%s' "$refuse_out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $refuse_out"
	printf '%s' "$refuse_state" | grep -q '"engine_state": *"running"' || fail "a refused stop changed the container: $refuse_state"
	[ "$stop_rc" -eq 0 ] || fail "container stop failed: $stop_out"
	printf '%s' "$stop_state" | grep -q '"state": *"stopped"' || fail "the container is not stopped after container stop: $stop_state"
	[ "$start_rc" -eq 0 ] || fail "container start failed: $start_out"
	printf '%s' "$start_state" | grep -q '"engine_state": *"running"' || fail "the container does not run after container start: $start_state"
	[ "$restart_rc" -eq 0 ] || fail "container restart failed: $restart_out"
	printf '%s' "$restart_state" | grep -q '"engine_state": *"running"' || fail "the container does not run after container restart: $restart_state"
	[ "$started_before" != "$started_after" ] || fail 'container restart did not restart the container'
	[ "$missing_rc" -ne 0 ] || fail "container stop of a missing container succeeded: $missing_out"
	printf '%s' "$missing_out" | grep -q 'no such container' || fail "the refusal of a missing container is unclear: $missing_out"
	log 'scenario: container stop, start and restart acted on a single container'
}

# field_is succeeds when a flat field of a stack in qhost status has the given value.
field_is() {
	[ "$(stack_field "$1" "$2")" = "$3" ]
}

# running_containers prints how many containers of the stack run.
running_containers() {
	docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" | wc -l
}

# scenario_stack_actions proves that a stop through qhost keeps demo-cache stopped against a new commit and
# the service restart that makes the poll run, that only a start clears the mark and lets the next poll deploy
# the commit, and that a restart restarts demo-stack. Both stacks run again at the end.
scenario_stack_actions() {
	step 'stack stop asks first, stops demo-cache and shows the mark in status and stack list'
	refuse_rc=0
	refuse_out=$(qd stack stop demo-cache) || refuse_rc=$?
	[ "$refuse_rc" -ne 0 ] || fail "stack stop without --yes succeeded: $refuse_out"
	printf '%s' "$refuse_out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $refuse_out"
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'a refused stack stop stopped demo-cache'
	out=$(qd stack stop demo-cache --yes) || fail "stack stop failed: $out"
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache still runs after stack stop'
	[ -n "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-cache)" ] || fail 'stack stop removed the containers of demo-cache'
	[ -n "$(stack_field demo-cache stopped)" ] || fail "status does not show demo-cache as stopped: $(qd --output json status)"
	qd stack list | grep demo-cache | grep -q 'stopped since' || fail "stack list does not show demo-cache as stopped: $(qd stack list)"
	[ -z "$(stack_field demo-stack stopped)" ] || fail 'demo-stack is shown as stopped'
	out=$(qd stack restart demo-cache --yes) && fail "stack restart of a stopped stack succeeded: $out"
	printf '%s' "$out" | grep -q 'qhost stack start' || fail "the refusal does not name stack start: $out"

	out=$(qd stack deploy demo-cache) && fail "stack deploy of a stopped stack succeeded: $out"
	printf '%s' "$out" | grep -q 'qhost stack start' || fail "the refusal of stack deploy does not name stack start: $out"
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'a refused stack deploy started demo-cache'

	step 'a new commit does not start a run of the stopped stack, also not after a service restart'
	count=$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	as_admin 'cd ~/stacks &&
		printf "# stopped-stack marker\n" >>demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Touch demo-cache while it is stopped" &&
		git push -q origin main'
	pushed=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	wait_for 'the pending commit of demo-cache' 60 field_is demo-cache pending "$pushed"
	[ "$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$count" ] || fail 'a run of the stopped stack was started by the poll'
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache runs again after a new commit'
	[ -n "$(stack_field demo-cache stopped)" ] || fail 'the stopped mark did not survive the service restart'
	stack_field demo-cache findings | grep -q stopped || fail "demo-cache has no finding stopped: $(qd --output json status)"

	step 'stack start clears the mark, starts demo-cache and the next poll deploys the commit'
	out=$(qd stack start demo-cache --yes) || fail "stack start failed: $out"
	[ -z "$(stack_field demo-cache stopped)" ] || fail "status still shows demo-cache as stopped: $(qd --output json status)"
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'demo-cache does not run after stack start'
	stack_healthy demo-cache || fail 'demo-cache is not healthy after stack start'
	restart_service
	wait_for 'the run of demo-cache after the start' 240 stack_idle demo-cache
	wait_for 'the commit deployed on demo-cache' 240 field_is demo-cache deployed "$pushed"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the run'
	log "scenario: demo-cache stayed stopped against commit $pushed until stack start"

	step 'stack restart restarts demo-stack'
	ids=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)
	started_before=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	out=$(qd stack restart demo-stack --yes) || fail "stack restart failed: $out"
	started_after=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	[ "$started_before" != "$started_after" ] || fail 'stack restart did not restart demo-stack'
	stack_healthy demo-stack || fail 'demo-stack is not healthy after stack restart'
	out=$(qd stack stop nope --yes) && fail "stack stop of an unknown stack succeeded: $out"
	printf '%s' "$out" | grep -q 'no such stack' || fail "the refusal of an unknown stack is unclear: $out"
	log 'scenario: stack stop, start and restart passed'
}

# has_finding succeeds when the stack shows the finding $2 in qhost status.
has_finding() {
	stack_field "$1" findings | grep -q "$2"
}

# no_finding succeeds when the stack does not show the finding $2.
no_finding() {
	! has_finding "$1" "$2"
}

# scenario_container_monitor proves that the failure monitor reports a container of demo-stack that dies outside
# qhost and then repairs it: the webhook hears container-failed and qhost status shows the finding; qhost
# restarts the container after a minute, and the webhook hears recovered with restarted. The same for a container
# that turns unhealthy. The restart policy is switched off for the kill, so only qhost can bring the container
# back; the monitor reads every 30 seconds and ends a failure after two clean readings.
scenario_container_monitor() {
	step 'a channel for the failure monitor events is added'
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add mon --type webhook --events container-failed,recovered) || fail "notify add failed: $out"
	qd notify list | grep -q 'mon *webhook *container-failed,recovered' || fail "notify list does not show the channel: $(qd notify list)"

	step 'a container killed outside qhost is reported and shown in status'
	wait_for 'demo-stack idle' 120 stack_idle demo-stack
	stack_healthy demo-stack || fail 'demo-stack is not healthy before the kill'
	no_finding demo-stack container-failed || fail "demo-stack shows a failure before the kill: $(qd --output json status)"
	victim=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack | head -n 1)
	[ -n "$victim" ] || fail 'demo-stack has no container to kill'
	docker exec "$host" docker update --restart=no "$victim" >/dev/null || fail 'cannot switch off the restart policy'
	docker exec "$host" docker kill "$victim" >/dev/null || fail 'cannot kill the container'
	wait_for 'the webhook for the failed container' 120 hook_seen /notify container-failed demo-stack
	wait_for 'the finding container-failed in status' 60 has_finding demo-stack container-failed
	stack_field demo-stack findings | grep -q 'exited' || fail "the finding does not name the kind exited: $(qd --output json status)"
	log 'scenario: the killed container was reported and shown as a finding'

	step 'qhost restarts the container: recovered is reported with restarted and the finding ends'
	wait_for 'demo-stack healthy again' 150 stack_healthy demo-stack
	wait_for 'the webhook for the recovered container' 240 hook_has /notify recovered '\"stack\":\"demo-stack\"' '\"kind\":\"exited\"' '\"restarted\":true'
	wait_for 'the finding to end' 60 no_finding demo-stack container-failed
	docker exec "$host" docker update --restart=unless-stopped "$victim" >/dev/null || fail 'cannot restore the restart policy'
	log 'scenario: qhost restarted the killed container and the finding ended'

	step 'a container that turns unhealthy is restarted by qhost'
	docker exec "$host" touch /opt/stacks/demo-stack/data/break-once || fail 'cannot break the health of demo-stack'
	wait_for 'the webhook for the unhealthy container' 240 hook_has /notify container-failed '\"stack\":\"demo-stack\"' '\"kind\":\"unhealthy\"'
	wait_for 'the finding container-failed in status' 60 has_finding demo-stack unhealthy
	wait_for 'the webhook for the recovered container' 360 hook_has /notify recovered '\"stack\":\"demo-stack\"' '\"kind\":\"unhealthy\"' '\"restarted\":true'
	wait_for 'the finding to end' 60 no_finding demo-stack container-failed
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the repair'
	if docker exec "$host" test -e /opt/stacks/demo-stack/data/break-once; then fail 'the restart did not clear the break'; fi
	out=$(qd notify remove mon) || fail "notify remove failed: $out"
	log 'scenario: the unhealthy container was restarted by qhost and recovered'
}

# repair_text prints the repair line of a stack in qhost status.
repair_text() {
	stack_field "$1" repair
}

repair_first_attempt() {
	repair_text repair-demo | grep -q '^1 of 3 attempts, next '
}

repair_halted() {
	repair_text "$1" | grep -q '^halted since '
}

# scenario_repair_start adds the stack repair-demo and breaks it for good: its health check fails while the file
# data/break exists, which a restart does not remove. qhost restarts it 1, 5 and 15 minutes after the failure
# began and then halts. The delays run in real time; scenario_repair_end checks the outcome at the end of the
# run, so they overlap with the other scenarios.
scenario_repair_start() {
	step 'the stack repair-demo is deployed and a channel for recovery-failed is added'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir repair-demo'
	cat <<EOF | as_admin 'cat >"$HOME"/stacks/repair-demo/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: repair-demo
    volumes:
      - ./data:/data
EOF
	as_admin 'cd ~/stacks && cp demo-stack/.gitignore repair-demo/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the repair-demo stack" && git push -q origin main'
	restart_service
	wait_for 'the first run of repair-demo' 240 stack_settled repair-demo succeeded
	wait_for 'repair-demo healthy' 60 stack_healthy repair-demo
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add rep --type webhook --events recovered,recovery-failed) || fail "notify add rep failed: $out"
	[ -z "$(repair_text repair-demo)" ] || fail "a healthy stack shows a repair: $(repair_text repair-demo)"

	step 'repair-demo turns unhealthy for good; the first attempt shows in status'
	docker exec "$host" touch /opt/stacks/repair-demo/data/break || fail 'cannot break repair-demo'
	repair_demo_broken=$(date +%s)
	wait_for 'the finding container-failed of repair-demo' 240 has_finding repair-demo container-failed
	wait_for 'the first repair attempt' 240 repair_first_attempt
	log "scenario: repair-demo $(repair_text repair-demo)"
}

# scenario_repair_end waits until qhost gave up on repair-demo and proves that the halt is shown, reported
# once and released by resume; then it fixes the stack and removes it from the repository.
scenario_repair_end() {
	step 'after three attempts the repair is halted, shown in status and the TUI, and reported once'
	wait_for 'the repair of repair-demo to halt' 1800 repair_halted repair-demo
	log "scenario: repair-demo halted after $(($(date +%s) - repair_demo_broken)) seconds: $(repair_text repair-demo)"
	repair_text repair-demo | grep -q 'after 3 of 3 attempts' || fail "status does not name the attempts: $(repair_text repair-demo)"
	wait_for 'the webhook for the failed recovery' 60 hook_has /notify recovery-failed '\"stack\":\"repair-demo\"' '\"attempts\":3'
	sleep 35
	[ "$(docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F '"path":"/notify"' | grep -F '"X-Qhost-Event":"recovery-failed"' | grep -cF '\"stack\":\"repair-demo\"')" = 1 ] ||
		fail 'recovery-failed was not sent exactly once'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait 'repair-demo' 30
	tui_wait 'halted' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'resume releases the halted repair and prints no pause time'
	docker exec "$host" rm -f /opt/stacks/repair-demo/data/break
	out=$(qd --output json stack resume repair-demo) || fail "stack resume failed: $out"
	printf '%s' "$out" | grep -q '"repair": *"released"' || fail "resume does not say that the repair was released: $out"
	if printf '%s' "$out" | grep -q '0001-01-01\|paused_since'; then fail "resume prints a pause time: $out"; fi
	if repair_halted repair-demo; then fail "status still shows the repair as halted: $(repair_text repair-demo)"; fi
	if qd stack resume repair-demo >/dev/null; then fail 'a second resume succeeded'; fi

	step 'repair-demo is healthy again and leaves the repository'
	wait_for 'repair-demo healthy' 120 stack_healthy repair-demo
	wait_for 'the finding of repair-demo to end' 120 no_finding repair-demo container-failed
	out=$(qd notify remove rep) || fail "notify remove rep failed: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q repair-demo &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the repair-demo stack" && git push -q origin main'
	log 'scenario: resume released the repair; repair-demo is fixed and removed'
}

# scenario_observe_start deploys the stack observe-demo, whose image 1.1.0 turns unhealthy for good 100 seconds
# after it starts, and updates it to 1.1.0 with a marker in its data before and one after the update. The
# observation time of the update (60 minutes by default) is running; qhost restarts the container once after
# the failure began and, if it lasts 5 more minutes, rolls the update back. The delays run in real time;
# scenario_observe_end checks the outcome after the source scenarios, so they overlap with them. observe-demo is
# set up after the terminal interface scenario on purpose: its row and containers stay out of the screens there.
# Every update by qhost starts an observation time, also for the other stacks, and none of them needs
# update.observe: 0: no later step makes such a stack fail except through a run that rolls itself back (the failing
# middle step of scenario_update), a stop or a mark that exempts it from repair, or a failure that one restart ends
# (scenario_container_monitor, which runs before the first update anyway).
scenario_observe_start() {
	step 'the stack observe-demo is deployed and a channel for rolled-back is added'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir observe-demo'
	cat <<EOF | as_admin 'cat >"$HOME"/stacks/observe-demo/compose.yaml'
services:
  app:
    image: registry.$domain/demo/late:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: observe-demo
    volumes:
      - ./data:/data
EOF
	as_admin 'cd ~/stacks && cp demo-stack/.gitignore observe-demo/.gitignore && printf "DEMO_APP_VERSION=1.0.0\n" >observe-demo/.env &&
		printf "version: 1\nbackup:\n  paths:\n    - data\n" >.qatlas/qhost/observe-demo.yaml && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the observe-demo stack" && git push -q origin main'
	restart_service
	wait_for 'the first run of observe-demo' 240 stack_settled observe-demo succeeded
	wait_for 'observe-demo healthy' 60 stack_healthy observe-demo
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add obs --type webhook --events rolled-back,rollback-failed) || fail "notify add obs failed: $out"
	[ -z "$(stack_field observe-demo observe)" ] || fail "a stack that was not updated shows an observation: $(stack_field observe-demo observe)"

	step 'observe-demo is updated to 1.1.0 and shows its observation time; data is marked before and after the update'
	docker exec "$host" sh -c 'echo before >/opt/stacks/observe-demo/data/marker.txt' || fail 'cannot write the marker before the update'
	observe_from=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	out=$(qd --output json stack update observe-demo --version 1.1.0) || fail "update of observe-demo failed: $out"
	observe_update=$(json_values "$out" commit | head -n 1)
	[ -n "$observe_update" ] || fail "update reported no commit: $out"
	observe_update_at=$(date +%s)
	printf '%s' "$out" | grep -q '"result": *"succeeded"' || fail "the update of observe-demo did not succeed: $out"
	[ "$(stack_images observe-demo)" = "registry.$domain/demo/late:1.1.0" ] || fail "observe-demo does not run 1.1.0: $(stack_images observe-demo)"
	docker exec "$host" sh -c 'echo after >/opt/stacks/observe-demo/data/after.txt' || fail 'cannot write the marker after the update'
	case $(stack_field observe-demo observe) in
	until\ *", update $observe_update") ;;
	*) fail "status does not show the observation of $observe_update: $(stack_field observe-demo observe)" ;;
	esac
	log "scenario: observe-demo updated by commit $observe_update; $(stack_field observe-demo observe)"
}

# scenario_observe_end waits until qhost rolled the update of observe-demo back and proves the outcome: the
# rollback as a run of its own with trigger observe, the backup of the update restored, the revert commit on the
# branch, rolled-back with cause observation at the webhook and the previous version running; then it removes
# the stack from the repository.
scenario_observe_end() {
	step 'the failing update of observe-demo is rolled back in its observation time'
	wait_for 'the webhook for the rolled-back update' 1200 hook_has /notify rolled-back '\"stack\":\"observe-demo\"' '\"cause\":\"observation\"' "\\\"commit\\\":\\\"$observe_update\\\""
	log "scenario: observe-demo rolled back $(($(date +%s) - observe_update_at)) seconds after its update"
	wait_for 'the rollback run of observe-demo' 120 stack_settled observe-demo rolled-back
	runs=$(qd stack runs observe-demo) || fail "stack runs observe-demo failed: $runs"
	newest=$(printf '%s\n' "$runs" | sed -n 2p)
	printf '%s\n' "$newest" | grep -q ' observe ' || fail "the newest run of observe-demo is not the rollback of the observation: $runs"
	printf '%s\n' "$newest" | grep -q 'rolled-back' || fail "the rollback run of observe-demo is not rolled-back: $runs"
	run=$(newest_run observe-demo)
	[ -n "$run" ] || fail "no run of observe-demo in: $runs"
	run_log=$(qd stack log "$run") || fail "stack log $run failed: $run_log"
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the rollback run did not end rolled-back: $run_log"
	log "scenario: observe rollback steps: $(update_steps "$run_log")"
	case ,$(update_steps "$run_log"), in
	,rollback,stop,*restore,*revert,*up,check,) ;;
	*) fail "unexpected steps of the rollback run: $(update_steps "$run_log")" ;;
	esac
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	revert=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert^")" = "$observe_update" ] || fail "the revert commit does not follow the update commit $observe_update"
	[ "$(as_admin "cd ~/stacks && git log -1 --format='%an|%ae' $revert")" = 'qhost|qhost@localhost.invalid' ] || fail 'the revert commit is not by qhost'
	as_admin "cd ~/stacks && git log -1 --format=%s $revert" | grep -q '^Revert observe-demo to ' ||
		fail "unexpected subject of the revert commit: $(as_admin "cd ~/stacks && git log -1 --format=%s $revert")"
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert:observe-demo")" = "$(as_admin "cd ~/stacks && git rev-parse $observe_from:observe-demo")" ] ||
		fail 'the revert commit does not restore the observe-demo folder from before the update'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$revert" ] || fail 'the revert commit is not the head of the remote branch'
	[ "$(stack_field observe-demo deployed)" = "$revert" ] || fail "observe-demo is not recorded as deployed at the revert commit $revert"
	[ "$(stack_images observe-demo)" = "registry.$domain/demo/late:1.0.0" ] || fail "observe-demo does not run 1.0.0 again: $(stack_images observe-demo)"
	wait_for 'observe-demo healthy' 60 stack_healthy observe-demo

	step 'the data of the backup of the update is back and the data written since is gone'
	[ "$(docker exec "$host" cat /opt/stacks/observe-demo/data/marker.txt)" = before ] || fail 'the marker from before the update is not back'
	if docker exec "$host" test -e /opt/stacks/observe-demo/data/after.txt; then fail 'the marker written after the update survived the rollback'; fi
	if docker exec "$host" test -e /opt/stacks/observe-demo/data/late-failed; then fail 'the failure state of the update survived the rollback'; fi
	[ -z "$(stack_field observe-demo observe)" ] || fail "status still shows an observation after the rollback: $(stack_field observe-demo observe)"
	wait_for 'the finding of observe-demo to end' 120 no_finding observe-demo container-failed

	step 'observe-demo leaves the repository'
	out=$(qd notify remove obs) || fail "notify remove obs failed: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q observe-demo .qatlas/qhost/observe-demo.yaml &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the observe-demo stack" && git push -q origin main'
	log 'scenario: the update of observe-demo was rolled back by qhost; observe-demo is removed'
}

# selfupdate_field prints one top-level field of qhost update status, run without root.
selfupdate_field() {
	as_admin "qhost --output json update status 2>&1" | sed -n "s/.*\"$1\": *\"\{0,1\}\([^\",}]*\).*/\1/p"
}

# service_pid prints the main process ID of the unit $1; without an argument that of the deploy service.
service_pid() {
	docker exec "$host" systemctl show -p MainPID --value "${1:-qhost-deploy}"
}

# service_entered prints when the unit $1 (the deploy service without an argument) last became active.
service_entered() {
	docker exec "$host" systemctl show -p ActiveEnterTimestampMonotonic --value "${1:-qhost-deploy}"
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

# unit_is_release succeeds when the installed unit $1 equals the one in the archive of the release $2.
unit_is_release() {
	arch=$(docker exec "$host" dpkg --print-architecture)
	tar -xzOf "$work/releases/$2/qhost_${2}_linux_$arch.tar.gz" "lib/systemd/system/$1.service" |
		docker exec -i "$host" cmp -s - "/usr/local/lib/systemd/system/$1.service"
}

# instance_version_is succeeds when qhost status, run without root, shows the version $1 for the instance $su_unit.
instance_version_is() {
	as_admin 'qhost --output json status 2>&1' | grep -q "\"instance\.$rootless_user\.version\": *\"$1\""
}

# instance_template_is succeeds when the installed template of the instances equals the file $1 on this machine.
instance_template_is() {
	docker exec -i "$host" cmp -s - "$su_template" <"$1"
}

# scenario_selfupdate updates the installed services to a newer signed test release and refuses tampered ones.
# The simulated host has the deploy service set up and the instance of scenario_rootless_instance running; every
# update renews its template from the new program and restarts it. At the end the update runs once more on the same
# host with the deploy service put aside, which is the host of an administrator who never set it up. Only
# scenario_uninstall follows; it puts the deploy service back.
scenario_selfupdate() {
	su_unit=$instance_service@$rootless_user.service
	su_template=/usr/local/lib/systemd/system/$release_template
	su_override=/etc/systemd/system/$su_unit
	docker exec "$host" systemctl start qhost qhost-deploy || fail 'cannot start the services'
	wait_for 'the services to answer' 30 service_serving
	[ "$(docker exec "$host" systemctl is-active "$su_unit")" = active ] || fail "the instance $su_unit does not run"

	step 'update status reports the newer test release without root'
	release_publish v0.2.0
	[ "$(docker exec "$host" qhost version 2>&1 | grep -c v0.1.0)" -ge 1 ] || fail "the installed version is not v0.1.0: $(docker exec "$host" qhost version 2>&1)"
	[ "$(selfupdate_field installed)" = v0.1.0 ] || fail "update status: $(as_admin 'qhost --output json update status 2>&1')"
	[ "$(selfupdate_field latest)" = v0.2.0 ] || fail "update status latest: $(as_admin 'qhost --output json update status 2>&1')"
	[ "$(selfupdate_field update_available)" = true ] || fail "update status: $(as_admin 'qhost --output json update status 2>&1')"
	log "scenario: update status: $(as_admin 'qhost --output json update status 2>&1' | tr -d '\n ')"

	step 'update without root and without --confirm changes nothing'
	before=$(installed_sum)
	if out=$(as_admin 'qhost update --yes --confirm 2>&1'); then fail "update without root succeeded: $out"; fi
	if out=$(as_admin 'sudo qhost update --yes 2>&1'); then fail "update without --confirm succeeded: $out"; fi
	[ "$(installed_sum)" = "$before" ] || fail 'a refused update changed the installation'

	step 'sudo qhost update installs the newer release, replaces both units, restarts both services, renews the template of the instance, restarts the instance and warns about its overriding unit'
	# The units of v0.2.0 equal the installed ones, so both are made stale first; only a replacement removes the mark.
	docker exec "$host" sh -ec 'for u in qhost qhost-deploy; do printf "# stale\n" >>/usr/local/lib/systemd/system/$u.service; done; systemctl daemon-reload' ||
		fail 'cannot mark the installed units'
	if unit_is_release qhost v0.2.0 || unit_is_release qhost-deploy v0.2.0; then fail 'the installed units were not made stale'; fi
	# The template v0.2.0 carries has one more line than the installed one. A copy of the installed template in
	# /etc/systemd/system overrides the unit of the instance, as an administrator may do.
	if instance_template_is "$work/releasebuild/$release_template"; then fail 'the installed template equals the one of v0.2.0'; fi
	docker exec "$host" sh -ec "cp $su_template $su_override && systemctl daemon-reload" || fail "cannot write $su_override"
	hpid=$(service_pid qhost) hentered=$(service_entered qhost) pid=$(service_pid) entered=$(service_entered)
	ipid=$(service_pid "$su_unit") ientered=$(service_entered "$su_unit")
	out=$(as_admin 'sudo qhost update --yes --confirm 2>&1') || fail "update failed: $out"
	wait_for 'the updated services' 60 service_serving
	[ "$(service_pid qhost)" != "$hpid" ] && [ "$(service_entered qhost)" != "$hentered" ] || fail 'the host service was not restarted by the update'
	[ "$(service_pid)" != "$pid" ] && [ "$(service_entered)" != "$entered" ] || fail 'the deploy service was not restarted by the update'
	docker exec "$host" qhost version 2>&1 | grep -q v0.2.0 || fail "the installed version is not v0.2.0: $(docker exec "$host" qhost version 2>&1)"
	[ "$(selfupdate_field installed)" = v0.2.0 ] && [ "$(selfupdate_field update_available)" = false ] ||
		fail "update status after the update: $(as_admin 'qhost --output json update status 2>&1')"
	unit_is_release qhost v0.2.0 || fail 'the unit of the host service is not the one of v0.2.0'
	unit_is_release qhost-deploy v0.2.0 || fail 'the unit of the deploy service is not the one of v0.2.0'
	[ "$(docker exec "$host" cat /etc/systemd/system/qhost-deploy.service.d/e2e.conf | grep -c backup-image)" = 1 ] || fail 'the update removed the drop-in'
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the update removed the stack folder grant'
	docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks || fail 'the updated deploy service has no write access to /opt/stacks'
	instance_template_is "$work/releasebuild/$release_template" || fail "$su_template is not the template of v0.2.0"
	wait_for "the instance $su_unit to answer with v0.2.0" 30 instance_version_is v0.2.0
	[ "$(service_pid "$su_unit")" != "$ipid" ] && [ "$(service_entered "$su_unit")" != "$ientered" ] ||
		fail "the instance $su_unit was not restarted by the update"
	printf '%s' "$out" | grep -qF "overrides the installed unit of $su_unit" || fail "the update does not warn about $su_override: $out"
	if printf '%s' "$out" | grep -q 'instances were not all renewed'; then fail "the renewal of the instance failed: $out"; fi
	log "scenario: updated to v0.2.0, host service pid $hpid -> $(service_pid qhost), deploy service pid $pid -> $(service_pid), instance pid $ipid -> $(service_pid "$su_unit"), services answering: $out"
	# Without the override the instance runs from the renewed template.
	docker exec "$host" sh -ec "rm $su_override && systemctl daemon-reload && systemctl restart $su_unit" || fail "cannot remove $su_override"
	[ "$(docker exec "$host" systemctl show -p FragmentPath --value "$su_unit")" = "$su_template" ] ||
		fail "$su_unit does not run from $su_template: $(docker exec "$host" systemctl show -p FragmentPath --value "$su_unit")"
	wait_for "the instance $su_unit to answer" 30 instance_version_is v0.2.0

	step 'a release with a tampered checksums.txt is refused and changes nothing'
	release_build v0.3.0
	good=$work/releases/v0.3.0
	bad=$work/releases/v0.3.0-tampered
	rm -rf "$bad"
	cp -r "$good" "$bad"
	printf '# tampered\n' >>"$bad/checksums.txt"
	cmp -s "$good/checksums.txt" "$bad/checksums.txt" && fail 'the test did not tamper with checksums.txt'
	release_publish v0.3.0 "$bad"
	refuse_update v0.3.0

	step 'a release with a tampered archive is refused and changes nothing'
	rm -rf "$bad"
	cp -r "$good" "$bad"
	printf 'x' >>"$bad/qhost_v0.3.0_linux_$(docker exec "$host" dpkg --print-architecture).tar.gz"
	release_publish v0.3.0 "$bad"
	refuse_update v0.3.0

	step 'a release signed by an unknown key is refused and changes nothing'
	rm -rf "$bad"
	cp -r "$good" "$bad"
	ssh-keygen -q -t ed25519 -N '' -C "$prefix-unknown-key" -f "$work/keys/unknown"
	rm -f "$bad/$signature_asset"
	(cd "$bad" && ssh-keygen -q -Y sign -f "$work/keys/unknown" -n qhost-release checksums.txt)
	release_publish v0.3.0 "$bad"
	refuse_update v0.3.0

	step 'on a host without a deploy service the update replaces and restarts only the host service of the two, installs no deploy unit and still renews and restarts the instance'
	# The deploy service and its unit are put aside on the simulated host only; the drop-ins stay where they are.
	docker exec "$host" sh -ec '
		systemctl disable --now qhost-deploy
		mv /usr/local/lib/systemd/system/qhost-deploy.service /root/qhost-deploy.service.aside
		printf "# stale\n" >>/usr/local/lib/systemd/system/qhost.service
		systemctl daemon-reload' || fail 'cannot put the deploy service aside'
	if unit_is_release qhost v0.3.0; then fail 'the host unit was not made stale'; fi
	[ "$(docker exec "$host" systemctl is-active qhost-deploy || true)" != active ] || fail 'the deploy service still runs'
	as_admin 'qhost --output json status' | grep -q '"deploy_service": *"not set up"' ||
		fail "qhost status does not show the deploy service as not set up: $(as_admin 'qhost --output json status 2>&1')"
	release_publish v0.3.0
	hpid=$(service_pid qhost) hentered=$(service_entered qhost)
	ipid=$(service_pid "$su_unit") ientered=$(service_entered "$su_unit")
	out=$(as_admin 'sudo qhost update --yes --confirm 2>&1') || fail "update without a deploy service failed: $out"
	wait_for 'the updated host service' 60 host_serving
	# v0.3.0 carries the template of the sources again.
	instance_template_is "$root/packaging/$release_template" || fail "$su_template is not the template of v0.3.0"
	wait_for "the instance $su_unit to answer with v0.3.0" 30 instance_version_is v0.3.0
	[ "$(service_pid "$su_unit")" != "$ipid" ] && [ "$(service_entered "$su_unit")" != "$ientered" ] ||
		fail "the instance $su_unit was not restarted by the update"
	if printf '%s' "$out" | grep -qF "overrides the installed unit of $su_unit"; then fail "the update warns about an instance unit that is not overridden: $out"; fi
	if printf '%s' "$out" | grep -q 'instances were not all renewed'; then fail "the renewal of the instance failed: $out"; fi
	[ "$(service_pid qhost)" != "$hpid" ] && [ "$(service_entered qhost)" != "$hentered" ] || fail 'the host service was not restarted by the update'
	docker exec "$host" qhost version 2>&1 | grep -q v0.3.0 || fail "the installed version is not v0.3.0: $(docker exec "$host" qhost version 2>&1)"
	unit_is_release qhost v0.3.0 || fail 'the unit of the host service is not the one of v0.3.0'
	if docker exec "$host" test -e /usr/local/lib/systemd/system/qhost-deploy.service; then fail 'the update installed the unit of the deploy service'; fi
	[ "$(docker exec "$host" systemctl is-active qhost-deploy || true)" != active ] || fail 'the update started the deploy service'
	as_admin 'qhost --output json status' | grep -q '"deploy_service": *"not set up"' ||
		fail "qhost status does not show the deploy service as not set up after the update: $(as_admin 'qhost --output json status 2>&1')"
	log "scenario: updated to v0.3.0 without a deploy service: host service pid $hpid -> $(service_pid qhost), no deploy unit: $out"

	docker exec "$host" systemctl stop qhost
}

# stack_folder_listing prints names, types, modes and numeric owners of everything in the stack folders named in
# $stack_folders, one line each, sorted. The .git folders stay out, because the service writes them on every poll;
# owners stay numeric so that deleting a user changes nothing in the listing.
stack_folder_listing() {
	docker exec "$host" sh -ec '
		for d in "$@"; do
			find "$d" -xdev -name .git -prune -o -printf "%p %y %m %U %G\n"
		done | sort' sh $stack_folders
}

# all_running_containers prints the names of all running containers of the host, sorted.
all_running_containers() {
	docker exec "$host" docker ps --format '{{.Names}}' | sort
}

# rootless_kept succeeds when the test user, its home with the probe file $un_probe, its rootless engine and the
# container $un_ct in it are as they were before the uninstall.
rootless_kept() {
	docker exec "$host" id "$rootless_user" >/dev/null 2>&1 &&
		[ "$(as_rootless "sha256sum $un_probe")" = "$un_probe_sum" ] &&
		[ "$(as_rootless 'systemctl --user is-active docker.service')" = active ] &&
		[ "$(as_rootless "docker inspect -f '{{.State.Running}}' $un_ct")" = true ]
}

# scenario_uninstall puts the deploy service back and proves sudo qhost uninstall: it needs root and --confirm, stops
# both services and the rootless instance and deletes the program, the units, the drop-in of the deploy unit, its
# tmpfiles rule and channel, those of the instance, the template and the manpages, and leaves the state, the journal,
# the state and configuration of the instance, the users, the group, the containers and the stack folders as they were.
# The program is copied away first, because uninstall deletes it; the copy then runs --purge, which deletes the three
# state folders, the state and configuration of the instance, the users and the group. The test user of the instance,
# its home, its rootless engine and its container stay. Nothing runs after it, and no later step may call qhost.
scenario_uninstall() {
	un_unit=$instance_service@$rootless_user.service
	un_template=/usr/local/lib/systemd/system/$release_template
	un_rule=/etc/tmpfiles.d/$instance_service-$rootless_user.conf
	un_channel=/run/$instance_service-$rootless_user
	un_state=/var/lib/$instance_service-$rootless_user
	un_config=/etc/$instance_service-$rootless_user
	un_ct=$prefix-uninstall-rootless-probe
	un_probe=/home/$rootless_user/stacks/probe/compose.yaml
	step 'the deploy service is put back and both services run, with a container beside them'
	docker exec "$host" sh -ec '
		mv /root/qhost-deploy.service.aside /usr/local/lib/systemd/system/qhost-deploy.service
		systemctl daemon-reload
		systemctl enable --now qhost qhost-deploy
		cp /usr/local/bin/qhost /root/qhost.copy' || fail 'cannot put the deploy service back'
	wait_for 'the services to answer' 30 service_serving
	image=registry.$domain/demo/app:1.0.0
	docker exec "$host" docker rm -f "$prefix-uninstall-probe" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$prefix-uninstall-probe" "$image" >/dev/null || fail 'cannot start the probe container'
	containers=$(all_running_containers)
	[ -n "$containers" ] || fail 'no container runs before the uninstall'
	[ "$(docker exec "$host" systemctl is-active "$un_unit")" = active ] || fail "the instance $un_unit does not run before the uninstall"
	for p in "$un_template" "$un_rule" "$un_channel" "$un_state" "$un_config"; do
		docker exec "$host" test -e "$p" || fail "$p is missing before the uninstall"
	done
	as_rootless "docker rm -f $un_ct >/dev/null 2>&1; docker run -d --quiet --name $un_ct demo/app:1.0.0" >/dev/null ||
		fail 'cannot start the probe container in the rootless engine'
	as_rootless "umask 022; mkdir -p ${un_probe%/*} && printf 'services: {}\n' >$un_probe" || fail "cannot write $un_probe"
	un_probe_sum=$(as_rootless "sha256sum $un_probe")
	rootless_kept || fail "the test user, its home, its engine or its container are not ready before the uninstall"
	# The folders are read from the drop-in now, because the uninstall deletes it.
	stack_folders=$(docker exec "$host" sed -n 's/^ReadWritePaths=-//p' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf)
	folders=$(stack_folder_listing)
	[ -n "$folders" ] || fail 'the drop-in of the deploy unit lists no stack folder'
	printf '%s\n' "$folders" | grep -q '^/opt/stacks ' || fail "the stack folders do not include /opt/stacks: $folders"

	step 'uninstall without root, without --confirm and without an answer changes nothing'
	before=$(installed_sum)
	if out=$(as_admin 'qhost uninstall --yes --confirm 2>&1'); then fail "uninstall without root succeeded: $out"; fi
	if out=$(as_admin 'sudo qhost uninstall --yes 2>&1'); then fail "uninstall without --confirm succeeded: $out"; fi
	if out=$(as_admin 'sudo qhost uninstall --confirm 2>&1 </dev/null'); then fail "uninstall without an answer succeeded: $out"; fi
	[ "$(installed_sum)" = "$before" ] || fail 'a refused uninstall changed the installation'
	service_serving || fail 'the services do not answer after a refused uninstall'
	[ "$(docker exec "$host" systemctl is-active "$un_unit")" = active ] || fail "a refused uninstall stopped $un_unit"
	docker exec "$host" test -f "$un_template" || fail "a refused uninstall deleted $un_template"

	step 'sudo qhost uninstall removes both services and the instance and keeps state, users, containers and stack folders'
	out=$(as_admin 'sudo qhost uninstall --yes --confirm 2>&1') || fail "uninstall failed: $out"
	for want in "systemctl disable --now $un_unit" "rm $un_rule" "rm $un_template"; do
		printf '%s' "$out" | grep -qF -- "$want" || fail "the steps of the uninstall do not show '$want': $out"
	done
	for p in "$un_template" "$un_rule" "$un_channel"; do
		if docker exec "$host" test -e "$p" || docker exec "$host" test -L "$p"; then fail "uninstall left $p: $out"; fi
	done
	[ "$(docker exec "$host" systemctl is-active "$un_unit" || true)" != active ] || fail "$un_unit still runs after the uninstall"
	if docker exec "$host" systemctl is-enabled --quiet "$un_unit"; then fail "$un_unit is still enabled after the uninstall"; fi
	if docker exec "$host" systemctl cat "$un_unit" >/dev/null 2>&1; then fail "systemd still knows the unit $un_unit after the uninstall"; fi
	for p in "$un_state" "$un_config"; do
		docker exec "$host" test -d "$p" || fail "uninstall without --purge removed $p"
	done
	rootless_kept || fail "the uninstall changed the test user, its home, its rootless engine or its container"
	for p in /usr/local/bin/qhost /usr/local/lib/systemd/system/qhost.service /usr/local/lib/systemd/system/qhost-deploy.service \
		/etc/tmpfiles.d/qhost-deploy.conf /run/qhost-deploy /usr/local/share/man/man1/qhost.1 \
		/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; do
		if docker exec "$host" test -e "$p" || docker exec "$host" test -L "$p"; then fail "uninstall left $p: $out"; fi
	done
	docker exec "$host" test -f /etc/systemd/system/qhost-deploy.service.d/e2e.conf || fail 'uninstall removed a drop-in it does not own'
	for u in qhost qhost-deploy; do
		[ "$(docker exec "$host" systemctl is-active "$u" || true)" != active ] || fail "$u still runs after the uninstall"
		if docker exec "$host" systemctl is-enabled --quiet "$u"; then fail "$u is still enabled after the uninstall"; fi
		if docker exec "$host" systemctl cat "$u" >/dev/null 2>&1; then fail "systemd still knows the unit $u after the uninstall"; fi
	done
	[ "$(all_running_containers)" = "$containers" ] || fail "the containers changed: $(all_running_containers)"
	[ "$(stack_folder_listing)" = "$folders" ] || fail 'a stack folder changed'
	for p in /var/lib/qhost /var/lib/qhost-deploy /var/lib/qhost-host; do
		docker exec "$host" test -d "$p" || fail "uninstall without --purge removed $p"
	done
	docker exec "$host" sh -ec 'id qhost && id qhost-deploy && getent group qhost && getent group qhost-deploy' >/dev/null ||
		fail 'uninstall without --purge removed a user or a group'
	log "scenario: uninstalled both services, the containers and stack folders are unchanged: $out"

	step 'sudo qhost uninstall --purge removes the three state folders, the state and configuration of the instance, the users and the group'
	out=$(as_admin 'sudo /root/qhost.copy uninstall --purge --yes --confirm 2>&1') || fail "uninstall --purge failed: $out"
	for p in /var/lib/qhost /var/lib/qhost-deploy /var/lib/qhost-host "$un_state" "$un_config"; do
		if docker exec "$host" test -e "$p"; then fail "uninstall --purge left $p: $out"; fi
	done
	for u in qhost qhost-deploy; do
		if docker exec "$host" id "$u" >/dev/null 2>&1; then fail "uninstall --purge left the user $u: $out"; fi
		if docker exec "$host" getent group "$u" >/dev/null 2>&1; then fail "uninstall --purge left the group $u: $out"; fi
	done
	[ "$(all_running_containers)" = "$containers" ] || fail "the purge changed the containers: $(all_running_containers)"
	[ "$(stack_folder_listing)" = "$folders" ] || fail 'the purge changed a stack folder'
	rootless_kept || fail "the purge changed the test user, its home, its rootless engine or its container"
	docker exec "$host" docker rm -f "$prefix-uninstall-probe" >/dev/null 2>&1 || true
	as_rootless "docker rm -f $un_ct" >/dev/null 2>&1 || true
	log "scenario: purged the state, the users and the group, the containers and stack folders are unchanged: $out"
}

# refuse_update requires that sudo qhost update fails for the release $1 and that program, units, version and
# both service processes stay as they were.
refuse_update() {
	before=$(installed_sum) hpid=$(service_pid qhost) hentered=$(service_entered qhost) pid=$(service_pid) entered=$(service_entered)
	if out=$(as_admin 'sudo qhost update --yes --confirm 2>&1'); then fail "update accepted a tampered release: $out"; fi
	[ "$(installed_sum)" = "$before" ] || fail 'a refused update changed the installation'
	[ "$(service_pid qhost)" = "$hpid" ] && [ "$(service_entered qhost)" = "$hentered" ] || fail 'a refused update restarted the host service'
	[ "$(service_pid)" = "$pid" ] && [ "$(service_entered)" = "$entered" ] || fail 'a refused update restarted the deploy service'
	docker exec "$host" qhost version 2>&1 | grep -q v0.2.0 || fail 'a refused update changed the version'
	service_serving || fail 'the services do not answer after a refused update'
	log "scenario: tampered release $1 refused, installation unchanged: $out"
}

# window_field prints one field of qhost window show (empty when it is absent).
window_field() {
	qd --output json window show | sed -n "s/.*\"$1\": *\"\{0,1\}\([^\",}]*\).*/\1/p"
}

# window_far_away opens the window daily at the time twelve hours from now, for one minute, and prints the answer.
window_far_away() {
	qd --output json window set --days daily --zone UTC --start "$(date -u -d '+12 hours' +%H:%M)" --duration 1m
}

# window_log prints what the service logged about the window and the stacks since it started.
window_log() {
	docker exec "$host" journalctl -u qhost-deploy --no-pager -o cat
}

# stack_run_count prints the number of recorded runs of a stack.
stack_run_count() {
	qd stack runs "$1" | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-' || true
}

# window_done succeeds when the service has begun the work of an opening since last_run read $window_last_before
# and has finished it. The default window may already have run once when the service started.
window_done() {
	last=$(window_field last_run)
	[ -n "$last" ] && [ "$last" != "$window_last_before" ] && [ -z "$(window_field running)" ]
}

# scenario_window proves the maintenance window with the real service and a real, short window. demo-stack is in
# automatic mode, demo-cache in manual mode, demo-paused is paused and demo-pinned is pinned, all four offer 1.1.0.
scenario_window() {
	image=registry.$domain/demo/app
	win_stacks='demo-stack demo-cache demo-paused demo-pinned'

	step 'four stacks wait for 1.1.0: automatic, manual, paused and pinned'
	prev_stack=$(newest_run demo-stack) prev_cache=$(newest_run demo-cache)
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	demo_stack_settings 'update:\n  mode: automatic\n' | as_admin 'cat >"$HOME"/stacks/.qatlas/qhost/demo-stack.yaml'
	as_admin 'cd ~/stacks &&
		for s in demo-paused demo-pinned; do
			cp -r demo-cache $s && sed -i "s/demo-cache/$s/" $s/compose.yaml &&
			sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.0/" $s/.env &&
			printf "version: 1\nupdate:\n  mode: automatic\n" >.qatlas/qhost/$s.yaml
		done &&
		printf "version: 1\nupdate:\n  mode: manual\n" >.qatlas/qhost/demo-cache.yaml &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.1/" demo-stack/.env demo-cache/.env &&
		git add -A && git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Prepare the maintenance window stacks" &&
		git push -q origin main'
	setup_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	for pair in "demo-stack $prev_stack" "demo-cache $prev_cache"; do
		set -- $pair
		run=$(next_run "$1" "$2") || fail "no run of $1 started for the window setup commit"
		wait_for "the run $run" 240 run_finished "$1" "$run"
		qd stack log "$run" | grep -q ' result succeeded' || fail "the run of $1 for the window setup did not succeed: $(qd stack log "$run")"
	done
	for stack in demo-paused demo-pinned; do
		wait_for "the first run of $stack" 120 stack_idle "$stack"
		docker exec -u qhost-deploy "$host" sh -ec "umask 077 && printf 'DEMO_SECRET=%s\n' not-a-real-secret-e2e >/opt/stacks/$stack/secrets.env"
		out=$(qd --output json stack deploy "$stack") || fail "deploy of $stack failed: $out"
		run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
		wait_for "the run $run" 240 run_finished "$stack" "$run"
		qd stack log "$run" | grep -q ' result succeeded' || fail "the deploy of $stack did not succeed: $(qd stack log "$run")"
	done
	[ "$(stack_images demo-stack)" = "$image:1.0.1" ] && [ "$(stack_images demo-cache)" = "$image:1.0.1" ] &&
		[ "$(stack_images demo-paused)" = "$image:1.0.0" ] && [ "$(stack_images demo-pinned)" = "$image:1.0.0" ] ||
		fail "the window stacks do not run their starting versions: $(for s in $win_stacks; do stack_images $s; done | tr '\n' ' ')"

	step 'pause and pin mark the stacks'
	out=$(qd --output json stack pause demo-paused) || fail "pause failed: $out"
	out=$(qd --output json stack pin demo-pinned) || fail "pin failed: $out"
	[ -n "$(stack_field demo-paused operator_pause)" ] || fail "status does not show demo-paused as paused: $(qd --output json status)"
	[ -n "$(stack_field demo-pinned pinned)" ] || fail "status does not show demo-pinned as pinned: $(qd --output json status)"
	[ -z "$(stack_field demo-stack operator_pause)$(stack_field demo-stack pinned)$(stack_field demo-cache operator_pause)$(stack_field demo-cache pinned)" ] ||
		fail 'a stack without a mark shows one'

	step 'window set takes effect in the running service without a restart'
	started=$(status_field started_at)
	[ -n "$started" ] || fail 'status shows no start time of the service'
	# --start has minute granularity: two minutes ahead opens the window between one and two minutes from now.
	start=$(date -u -d '+2 minutes' +%H:%M)
	out=$(qd --output json window set --days daily --zone UTC --start "$start" --duration 5m) || fail "window set failed: $out"
	log "scenario: window set: $out"
	[ "$(window_field start)" = "$start" ] && [ "$(window_field duration)" = 5m ] && [ "$(window_field zone)" = UTC ] ||
		fail "window show does not report the new window: $(qd --output json window show)"
	[ "$(window_field open)" = false ] || fail "the window is open before its start: $(qd --output json window show)"
	case $(window_field next_opening) in *T$start:00Z) ;; *) fail "the next opening is not at $start UTC: $(qd --output json window show)" ;; esac
	window_last_before=$(window_field last_run)

	for stack in $win_stacks; do
		eval "runs_$(printf '%s' "$stack" | tr - _)=$(stack_run_count "$stack")"
		eval "deployed_$(printf '%s' "$stack" | tr - _)=$(stack_field "$stack" deployed)"
	done
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	[ "$head_before" = "$setup_commit" ] || fail "the stacks repository moved after the setup commit: $head_before"

	step 'the window opens and updates the automatic stack by itself'
	wait_for 'the window to open' 150 sh -c "docker exec -i $host qhost --output json window show | grep -q '\"open\": *true'"
	log "scenario: window open: $(qd --output json window show | tr -d '\n ')"
	wait_for 'the window work to finish' 300 window_done
	[ "$(window_field open)" = true ] || fail "the window work ran past the end of the window: $(qd --output json window show)"
	[ "$(status_field started_at)" = "$started" ] || fail 'the service restarted'
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "the automatic stack does not run 1.1.0: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the window update'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 1 ] || fail 'the window did not make exactly one commit'
	update_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$update_commit" ] || fail 'the update commit is not on the remote branch'
	update_commit_only_version "$update_commit" 1.0.1 1.1.0
	[ "$(stack_field demo-stack deployed)" = "$update_commit" ] || fail "demo-stack is not recorded as deployed at $update_commit"
	[ "$(stack_run_count demo-stack)" = "$((runs_demo_stack + 1))" ] || fail 'the window did not make exactly one run of demo-stack'
	log "scenario: window updated demo-stack to 1.1.0 with commit $update_commit"

	step 'the manual stack is only reported'
	[ "$(stack_images demo-cache)" = "$image:1.0.1" ] || fail "the manual stack changed: $(stack_images demo-cache)"
	[ "$(stack_run_count demo-cache)" = "$runs_demo_cache" ] || fail 'the window ran the manual stack'
	[ "$(stack_field demo-cache deployed)" = "$deployed_demo_cache" ] || fail 'the window changed the deployed commit of the manual stack'
	[ -z "$(as_admin "cd ~/stacks && git log --format=%H $setup_commit..HEAD -- demo-cache")" ] || fail 'the window committed to the manual stack'
	out=$(qd --output json stack updates demo-cache) || fail "updates demo-cache failed: $out"
	updates_expect "$out" 1.0.1 1.1.0 5.0.0 update
	notice=$(window_log | grep 'event=update-available' | grep 'stack=demo-cache' || true)
	[ -n "$notice" ] || fail "the service log has no update-available line for demo-cache: $(window_log | tail -n 20)"
	log "scenario: log: $(printf '%s' "$notice" | tail -n 1)"

	step 'the paused and the pinned stack stay unchanged, their update stays visible'
	for stack in demo-paused demo-pinned; do
		v=$(printf '%s' "$stack" | tr - _)
		[ "$(stack_images "$stack")" = "$image:1.0.0" ] || fail "$stack changed: $(stack_images "$stack")"
		[ "$(stack_run_count "$stack")" = "$(eval "printf %s \"\$runs_$v\"")" ] || fail "the window ran $stack"
		[ "$(stack_field "$stack" deployed)" = "$(eval "printf %s \"\$deployed_$v\"")" ] || fail "the window changed the deployed commit of $stack"
		[ -z "$(as_admin "cd ~/stacks && git log --format=%H $setup_commit..HEAD -- $stack")" ] || fail "the window committed to $stack"
		out=$(qd --output json stack updates "$stack") || fail "updates $stack failed: $out"
		updates_expect "$out" 1.0.0 1.1.0 5.0.0 update
		window_log | grep 'stack skipped' | grep "stack=$stack" | grep -q 'reason=' || fail "the service log does not say why $stack was skipped"
		log "scenario: log: $(window_log | grep 'stack skipped' | grep "stack=$stack" | tail -n 1)"
	done

	step 'the marks are lifted and the window is moved away'
	qd stack unpause demo-paused >/dev/null || fail 'unpause failed'
	qd stack unpin demo-pinned >/dev/null || fail 'unpin failed'
	out=$(window_far_away) || fail "window set failed: $out"
	[ "$(window_field open)" = false ] || fail "the window is still open after it was moved: $(qd --output json window show)"
}

# The terminal interface is driven in a tmux pane of the administrator, 120 columns by 40 rows. The command
# keeps the pane after qhost ends, so the exit code can be read from the screen.
# The icons of the tab line in the default setting (unicode), which the pane's UTF-8 locale draws; an icon
# stands between the digit and the name.
tui_icon_stacks='▤'
tui_icon_detail='▣'
tui_icon_runs='▶'
tui_icon_settings='⚙'
tui_icon_containers='▦'

tui_tmux() {
	docker exec -u "$admin" "$host" tmux -L qdtui "$@"
}

# tui_screen prints the screen and appends it to the capture log, which the first-setup section searches for
# masked values at its end.
tui_screen() {
	tui_tmux capture-pane -p -t qdtui | tee -a "${tui_log:-/dev/null}" 2>/dev/null
}

tui_send() {
	tui_tmux send-keys -t qdtui "$@"
}

tui_has() {
	tui_screen | grep -qF -- "$1"
}

# tui_wait waits up to $2 seconds for the text $1 on the screen and shows the screen when it does not appear.
tui_wait() {
	i=0
	until tui_has "$1"; do
		i=$((i + 1))
		if [ "$i" -ge $(($2 * 4)) ]; then
			printf '%s\n' "$(tui_screen)" >&2
			fail "timeout waiting for '$1' on the screen of the terminal interface"
		fi
		sleep 0.25
	done
}

# tui_cursor_to moves the cursor of the Stacks list to the stack $1. The list draws no cursor without the focus, so
# the focus must be in the list.
tui_cursor_to() {
	i=0
	until tui_screen | grep -q "^> .*\[.\] $1 "; do
		i=$((i + 1))
		[ "$i" -lt 12 ] || fail "the cursor does not reach $1: $(tui_screen)"
		tui_send Down
		sleep 0.2
	done
}

# tui_folder_to moves the cursor of the Stacks list to the group row of the stack folder $1. The mark before the path
# is a symbol in a terminal.
tui_folder_to() {
	tui_send Home
	i=0
	until tui_screen | grep -q "^> .*\[.\] [^ ]* $1 ("; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "the cursor does not reach the folder $1: $(tui_screen)"
		tui_send Down
		sleep 0.2
	done
}

# tui_click presses and releases the left mouse button at the first cell of the text $1 on the screen, as a
# terminal reports it (SGR, 1-based), and fails when the text is not on the screen. awk counts bytes, so the
# continuation bytes of the icons before the text are taken off; an icon is one cell wide.
tui_click() {
	cell=$(tui_screen | awk -v t="$1" 'i = index($0, t) { p = substr($0, 1, i - 1); gsub(/[\200-\277]/, "", p); print length(p) + 1, NR; exit }')
	[ -n "$cell" ] || fail "cannot click '$1', it is not on the screen: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
}

# tui_wheel turns the mouse wheel $2 notches ($1 is up or down) at the cell $3 (column), $4 (row), 1-based, as
# a terminal reports it (SGR, buttons 64 and 65).
tui_wheel() {
	button=65
	[ "$1" = up ] && button=64
	n=0
	while [ "$n" -lt "$2" ]; do
		tui_send -l "$(printf '\033[<%d;%d;%dM' "$button" "$3" "$4")"
		n=$((n + 1))
	done
}

# tui_click_row clicks the stack $1 in the Stacks list and waits until the cursor marks it.
tui_click_row() {
	tui_click "$1"
	i=0
	until tui_screen | grep -q "^> .*\[.\] $1 "; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the click did not mark $1: $(tui_screen)"
		sleep 0.25
	done
}

# tui_excerpt logs the lines of the screen from the first one containing $1, at most $2 lines.
tui_excerpt() {
	tui_screen | sed -n "/$1/,\$p" | head -n "$2" | sed 's/ *$//; s/^/e2e:   | /'
}

# scenario_tui drives the terminal interface of the running service the way an administrator does. demo-cache
# (manual mode, 1.0.1) is updated alone with the changelog of its image; demo-paused and demo-pinned (1.0.0) are
# ticked and updated together. All three offer 1.1.0 after the window section.
scenario_tui() {
	image=registry.$domain/demo/app

	step 'demo-cache gets a changelog source and a fresh update check'
	settings_set demo-cache "$(printf 'version: 1\nupdate:\n  mode: manual\n  changelog:\n    DEMO_APP_VERSION: qhoste2e-org/demo-image\n')"
	printf '%s' "$settings_out" | grep -q '"changed": *true' || fail "settings set did not add the changelog source: $settings_out"
	qd stack updates --refresh >/dev/null || fail 'updates --refresh failed'
	out=$(qd stack changelog demo-cache) || fail "changelog failed: $out"
	printf '%s' "$out" | grep -q 'invented crash' || fail "the changelog of demo-cache has no release notes: $out"

	step 'sudo qhost opens the interface and lists the demo stacks'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	for name in demo-stack demo-cache demo-paused demo-pinned; do tui_wait "$name" 30; done
	tui_wait 'q quit' 10
	# The interface opens with the focus on the tab line.
	tui_wait 'next area' 10
	log 'scenario: interface open, stacks listed'

	step 'a click on a tab switches the area and a click on a stack row marks it'
	tui_click "3 $tui_icon_runs Runs"
	tui_wait 'TRIGGER' 10
	tui_click "1 $tui_icon_stacks Stacks"
	tui_wait 'CURRENT' 10
	tui_click_row demo-pinned
	# tui_cursor_to only moves down, so the cursor goes back to the first row.
	tui_click_row bookstack
	log 'scenario: mouse clicks switched the area and marked a row'

	step 'u asks first: the question shows the versions and the changelog, nothing changes yet'
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	runs_before=$(stack_run_count demo-cache)
	tui_cursor_to demo-cache
	tui_send u
	tui_wait 'Update demo-cache?' 15
	tui_wait '1.0.1 -> 1.1.0' 15
	tui_wait 'Fixes an invented crash on empty input.' 30
	log 'scenario: update question of demo-cache:'
	tui_excerpt "Update demo-cache" 14
	[ "$(stack_images demo-cache)" = "$image:1.0.1" ] || fail "demo-cache changed before the answer: $(stack_images demo-cache)"
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$head_before" ] || fail 'the question already made a commit'
	[ "$(stack_run_count demo-cache)" = "$runs_before" ] || fail 'the question already started a run'

	step 'a click on the answer runs the update as y does; while it runs the Runs area lists the run and opens its log, which follows it'
	tui_click 'yes, do it'
	tui_send 3
	# The list is read again on r. The run lasts a few seconds, long enough for a poll every 0.3 seconds.
	i=0
	until tui_screen | grep -qE 'demo-cache +[0-9-]+ +[0-9:]+ +running'; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the Runs area never listed the running update: $(tui_screen)"
		tui_send r
		sleep 0.3
	done
	tui_send Home Enter
	tui_wait 'following' 10
	tui_screen | grep -qE '^ +demo-cache +running +[0-9]{8}T' || fail "the opened log is not that of the running update: $(tui_screen)"
	tui_wait 'step ' 10
	log 'scenario: live run log of the update:'
	tui_excerpt 'demo-cache  ' 8
	tui_wait 'update finished: 1 ok, 0 failed' 120

	step 'l opens the log of the finished run; the update made one commit and the stack runs 1.1.0'
	tui_send l
	tui_wait 'result succeeded' 15
	screen_run=$(tui_screen | grep -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}' | head -n 1)
	[ -n "$screen_run" ] && [ "$screen_run" = "$(newest_run demo-cache)" ] ||
		fail "the log on the screen is not the newest run of demo-cache ($screen_run): $(tui_screen)"
	run_log=$(qd stack log "$screen_run")
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the update run did not succeed: $run_log"
	[ "$(update_steps "$run_log")" = backup,commit,push,fast-forward,pull,up,check ] || fail "unexpected steps of the update run: $(update_steps "$run_log")"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 1 ] || fail 'the update did not create exactly one commit'
	tui_commit_only_version "$(as_admin 'cd ~/stacks && git rev-parse HEAD')" demo-cache 1.0.1 1.1.0
	[ "$(stack_images demo-cache)" = "$image:1.1.0" ] || fail "demo-cache does not run 1.1.0: $(stack_images demo-cache)"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the update'
	log "scenario: interface update of demo-cache: run $screen_run, demo-cache runs 1.1.0"

	step 'the wheel scrolls the run log in a small window and the lines of its end come back'
	tui_tmux resize-window -t qdtui -y 12
	i=0
	until tui_screen | grep -q "$screen_run"; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "the run log is not redrawn in the small window: $(tui_screen)"
		sleep 0.25
	done
	tui_send End
	tui_wait 'result succeeded' 10
	tui_wheel up 3 10 6
	i=0
	while tui_has 'result succeeded'; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the wheel did not scroll the run log up: $(tui_screen)"
		sleep 0.25
	done
	tui_wheel down 10 10 6
	tui_wait 'result succeeded' 10
	tui_tmux resize-window -t qdtui -y 40
	tui_wait "$screen_run" 10
	log 'scenario: the wheel scrolled the run log up and back down'

	step 'two ticked stacks are updated with one question, one after another'
	head_before=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	tui_send 1
	tui_wait '[ ] demo-paused' 10
	tui_cursor_to demo-paused
	tui_send Space
	tui_cursor_to demo-pinned
	tui_send Space
	tui_wait '[x] demo-paused' 10
	tui_wait '[x] demo-pinned' 10
	tui_send u
	tui_wait 'Update 2 stacks?' 15
	tui_wait '  demo-pinned' 15
	# The question offers yes only once the changelogs of both stacks are read; an earlier y is ignored.
	tui_wait 'y yes, do it' 60
	log 'scenario: update question of two stacks:'
	tui_excerpt 'Update 2 stacks' 14
	[ "$(stack_images demo-paused)" = "$image:1.0.0" ] && [ "$(stack_images demo-pinned)" = "$image:1.0.0" ] ||
		fail 'a stack changed before the answer'
	tui_send y
	tui_wait 'update finished: 2 ok, 0 failed' 180
	log 'scenario: result of the two-stack update:'
	tui_excerpt 'update finished' 8
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 2 ] || fail 'the multi-update did not create exactly two commits'
	first_commit=$(as_admin "cd ~/stacks && git log --reverse --format=%H $head_before..HEAD" | sed -n 1p)
	second_commit=$(as_admin "cd ~/stacks && git log --reverse --format=%H $head_before..HEAD" | sed -n 2p)
	tui_commit_only_version "$first_commit" demo-paused 1.0.0 1.1.0
	tui_commit_only_version "$second_commit" demo-pinned 1.0.0 1.1.0
	for stack in demo-paused demo-pinned; do
		[ "$(stack_images "$stack")" = "$image:1.1.0" ] || fail "$stack does not run 1.1.0: $(stack_images "$stack")"
		stack_healthy "$stack" || fail "$stack is not healthy after the update"
	done
	[ "$(stack_field demo-paused deployed)" = "$first_commit" ] && [ "$(stack_field demo-pinned deployed)" = "$second_commit" ] ||
		fail 'the stacks are not recorded as deployed at their update commits'
	first_log=$(qd stack log "$(newest_run demo-paused)") second_log=$(qd stack log "$(newest_run demo-pinned)")
	first_end=$(printf '%s\n' "$first_log" | tail -n 1 | cut -d ' ' -f 1) second_start=$(printf '%s\n' "$second_log" | head -n 1 | cut -d ' ' -f 1)
	[ "$(printf '%s\n%s\n' "$first_end" "$second_start" | sort | head -n 1)" = "$first_end" ] ||
		fail "the second run started before the first ended: $first_end, $second_start"
	log "scenario: two stacks updated one after another: $first_commit, $second_commit"

	step 'X asks first and stops demo-cache; status, stack list and the interface show it as stopped'
	# The summary of the update above is still open and takes every key.
	tui_send Escape
	tui_send 1
	tui_wait '[ ] demo-cache' 10
	tui_send Home
	tui_cursor_to demo-cache
	tui_send X
	tui_wait 'Stop demo-cache and keep it stopped until it is started?' 15
	log 'scenario: stop question of demo-cache:'
	tui_excerpt 'Stop demo-cache' 12
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'the question already stopped demo-cache'
	[ -z "$(stack_field demo-cache stopped)" ] || fail 'the question already marked demo-cache as stopped'
	tui_send y
	tui_wait 'stop finished: 1 ok, 0 failed' 120
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache still runs after the stop in the interface'
	[ -n "$(stack_field demo-cache stopped)" ] || fail "status does not show demo-cache as stopped: $(qd --output json status)"
	qd stack list | grep demo-cache | grep -q 'stopped since' || fail "stack list does not show demo-cache as stopped: $(qd stack list)"
	tui_send Escape
	i=0
	until tui_screen | grep -qE 'demo-cache .* stopped '; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the Stacks area does not show demo-cache as stopped: $(tui_screen)"
		tui_send r
		sleep 0.3
	done
	log 'scenario: the Stacks area shows demo-cache as stopped:'
	tui_excerpt 'STACK' 8

	step 'a new commit leaves the stopped demo-cache stopped'
	count=$(stack_run_count demo-cache)
	as_admin 'cd ~/stacks && git pull -q --ff-only &&
		printf "# stopped-stack marker\n" >>demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Touch demo-cache while it is stopped in the interface" &&
		git push -q origin main'
	pushed=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	wait_for 'the pending commit of demo-cache' 60 field_is demo-cache pending "$pushed"
	[ "$(stack_run_count demo-cache)" = "$count" ] || fail 'a run of the stopped demo-cache was started by the poll'
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache runs again after a new commit'
	[ -n "$(stack_field demo-cache stopped)" ] || fail 'the stopped mark did not survive the service restart'
	tui_send r
	tui_send Home
	tui_cursor_to demo-cache
	tui_screen | grep -qE 'demo-cache .* stopped ' || fail "the Stacks area does not show demo-cache as stopped after the commit: $(tui_screen)"

	step 'S asks first and starts demo-cache; the mark is gone and the interface no longer shows it as stopped'
	tui_send S
	tui_wait 'Start demo-cache?' 15
	[ -n "$(stack_field demo-cache stopped)" ] || fail 'the question already lifted the stopped mark'
	tui_send y
	tui_wait 'start finished: 1 ok, 0 failed' 120
	[ -z "$(stack_field demo-cache stopped)" ] || fail "status still shows demo-cache as stopped: $(qd --output json status)"
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'demo-cache does not run after the start in the interface'
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the start'
	tui_send Escape
	tui_send r
	i=0
	while tui_screen | grep -qE 'demo-cache .* stopped '; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the Stacks area still shows demo-cache as stopped: $(tui_screen)"
		tui_send r
		sleep 0.3
	done
	log "scenario: demo-cache stayed stopped against commit $pushed until S started it"

	step 'the Containers area shows a container that is stopped in the background without a key press'
	single=$prefix-single-tui
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$single" --label "$label" "$image:1.0.0" >/dev/null ||
		fail 'cannot start the single container for the Containers area'
	wait_for 'the single container' 30 health_is "$single" healthy
	tui_send Escape
	tui_send 5
	tui_wait "5 $tui_icon_containers Containers" 10
	tui_wait 'single containers' 15
	running_seen=0 stopped_seen=0
	tui_screen | grep -qE " $single +ok " && running_seen=1
	docker exec "$host" docker stop "$single" >/dev/null || stopped_seen=failed
	# No key is sent from here on: the area reads the engine again every 5 seconds on its own.
	i=0
	while [ "$stopped_seen" = 0 ] && [ "$i" -lt 20 ]; do
		tui_screen | grep -qE " $single +stopped " && stopped_seen=1
		i=$((i + 1))
		[ "$stopped_seen" = 1 ] || sleep 1
	done
	final_screen=$(tui_screen)
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1
	[ "$running_seen" = 1 ] || fail "the Containers area does not show $single as ok before the stop: $final_screen"
	[ "$stopped_seen" = 1 ] || fail "the Containers area did not show $single as stopped within 20 seconds without a key press: $final_screen"
	log "scenario: the Containers area followed $single from ok to stopped without a key press"

	step 'the Containers area opens the log of a container, takes a period and a search, and the stack detail opens the log of its stack, both without the secret'
	log_secret=not-a-real-secret-e2e
	ctname=$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-stack --format '{{.Names}}' | head -n 1)
	[ -n "$ctname" ] || fail 'no container of demo-stack runs for the log of the interface'
	# A container that runs for minutes has scrolled its start line out of the view; a restart writes a new one.
	docker exec "$host" docker restart "$ctname" >/dev/null || fail 'cannot restart the container for the log of the interface'
	wait_for 'demo-stack healthy' 60 stack_healthy demo-stack
	tui_send Escape
	tui_send 5
	tui_wait 'own stack demo-stack' 15
	tui_send Home
	i=0
	until tui_screen | grep -q "^> .*$ctname "; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the cursor of the Containers area does not reach $ctname: $(tui_screen)"
		tui_send Down
		sleep 0.2
	done
	tui_send o
	tui_wait 'demo tick' 10
	# The log opens following at its end, so an older container shows its start line, the only one with the
	# secret, only after Home. Home reaches it while it lies within the 500 lines read first (a container up to
	# about 16 minutes old), which covers one scenario run.
	tui_send Home
	tui_wait 'demo started' 10
	tui_screen | grep -qF 'token=[redacted]' || fail "the start line in the log of $ctname shows no masked token: $(tui_screen)"
	tui_has "$log_secret" && fail "the log of $ctname shows the secret: $(tui_screen)"
	tui_send End
	# The demo app writes "demo tick" every 2 seconds: a line that comes in while the log is open was followed.
	tui_wait 'demo tick' 10
	tui_send T Right Tab BSpace BSpace BSpace
	tui_type 1
	tui_send Tab Right F2
	sleep 1
	tui_wait 'demo tick' 10
	tui_screen | grep -qE 'T (period: )?all' && fail "the log still shows no period after the choice: $(tui_screen)"
	tui_send /
	tui_type 'demo tick'
	tui_send Enter
	sleep 0.5
	tui_wait 'demo tick' 10
	tui_send Escape
	tui_send Escape
	tui_wait 'own stack demo-stack' 10
	tui_has 'demo tick' && fail "the log is still open after Escape: $(tui_screen)"
	tui_send 1
	tui_wait 'CURRENT' 10
	tui_send Home
	tui_cursor_to demo-stack
	tui_send Enter
	tui_send o
	tui_wait 'demo tick' 10
	tui_send Home
	tui_wait 'demo started' 10
	tui_screen | grep -q "$ctname .*demo started" || fail "the stack log does not name the container: $(tui_screen)"
	tui_screen | grep -qF 'token=[redacted]' || fail "the start line in the log of demo-stack shows no masked token: $(tui_screen)"
	tui_has "$log_secret" && fail "the log of demo-stack shows the secret: $(tui_screen)"
	tui_send Escape
	tui_send Escape
	log "scenario: the Containers area and the stack detail opened the log of $ctname and of demo-stack, with a period and a search; the secret stayed masked"

	step 'the Containers area stops, starts and restarts a foreign container after a confirmation and shows each new state without a further key press'
	foreign=$prefix-foreign-tui
	docker exec "$host" docker rm -f "$foreign" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$foreign" --label "$label" \
		--label com.docker.compose.project=demo-foreign-tui "$image:1.0.0" >/dev/null ||
		fail 'cannot start the foreign container for the container actions'
	wait_for 'the foreign container' 30 health_is "$foreign" healthy
	tui_send Escape
	tui_send 5
	tui_wait "5 $tui_icon_containers Containers" 10
	tui_wait 'foreign project demo-foreign-tui' 15
	# ct_action presses the action key $1 on the row of $foreign (the group order depends on the language, so the
	# cursor is found by name), confirms the question that starts with $2 and closes the result sheet of the
	# command $3. The dialog wraps the question, so only its first words are matched whole.
	ct_action() {
		tui_send Home
		i=0
		until tui_screen | grep -q "^> .*$foreign "; do
			i=$((i + 1))
			[ "$i" -lt 40 ] || fail "the cursor of the Containers area does not reach $foreign: $(tui_screen)"
			tui_send Down
			sleep 0.2
		done
		tui_send "$1"
		tui_wait "$2 container $foreign" 15
		tui_has 'demo-foreign-tui)?' || fail "the question of $3 does not name the foreign project: $(tui_screen)"
		tui_excerpt "$2 container $foreign" 10
		tui_send y
		tui_wait "$3 finished: 1 ok, 0 failed" 60
		tui_has "$foreign" || fail "the result sheet of $3 does not name $foreign: $(tui_screen)"
		tui_send Escape
		tui_wait "5 $tui_icon_containers Containers" 10
	}
	# ct_expect waits without a key press for the row of $foreign to show the state $1 and checks the engine state $2.
	ct_expect() {
		i=0
		until tui_screen | grep -qE " $foreign +$1 "; do
			i=$((i + 1))
			[ "$i" -lt 20 ] || fail "the Containers area does not show $foreign as $1 after the action: $(tui_screen)"
			sleep 1
		done
		engine=$(docker exec "$host" docker inspect -f '{{.State.Status}}' "$foreign")
		[ "$engine" = "$2" ] || fail "docker shows $foreign as $engine, not $2: $(tui_screen)"
	}
	tui_screen | grep -qE " $foreign +ok " || fail "the Containers area does not show $foreign as ok before the stop: $(tui_screen)"

	ct_action X Stop stop
	ct_expect stopped exited
	log "scenario: the stop through the Containers area left $foreign stopped in the list and exited in docker"

	ct_action S Start start
	ct_expect ok running
	log "scenario: the start through the Containers area left $foreign ok in the list and running in docker"

	started=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$foreign")
	ct_action Z Restart restart
	ct_expect ok running
	[ "$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$foreign")" != "$started" ] ||
		fail "the restart did not restart $foreign: $(tui_screen)"
	docker exec "$host" docker rm -f "$foreign" >/dev/null 2>&1
	log "scenario: the restart through the Containers area left $foreign ok in the list and running in docker with a new start time"

	step 'Tab focuses the tab line, Right and Left switch the area without digits, and Tab leads back to the content'
	tui_send Escape
	# An earlier step left a run log open in Runs; close it so Runs shows its list.
	tui_send 3
	tui_send Escape
	tui_wait 'STARTED' 10
	tui_send 1
	tui_wait "1 $tui_icon_stacks Stacks" 10
	# The tab line has the focus while the footer offers "next area"; the area shows in its content: the
	# Stacks list has the CURRENT column, Stack detail a "mode" row (or the hint that no stack is chosen),
	# Runs the STARTED column.
	tui_send Tab
	tui_wait 'next area' 10
	tui_has 'CURRENT' || fail "the Stacks area is not shown with the focus on the tab line: $(tui_screen)"
	tui_send Right
	i=0
	until tui_screen | grep -qE '^ +(mode +|No stack chosen)'; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "Right did not show the Stack detail area: $(tui_screen)"
		sleep 0.25
	done
	tui_send Right
	tui_wait 'STARTED' 10
	tui_send Left
	i=0
	until tui_screen | grep -qE '^ +(mode +|No stack chosen)'; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "Left did not lead back to the Stack detail area: $(tui_screen)"
		sleep 0.25
	done
	tui_has 'next area' || fail "the tab line lost the focus while switching: $(tui_screen)"
	tui_send Tab
	sleep 0.5
	tui_has 'next area' && fail "Tab does not lead back to the content: $(tui_screen)"
	tui_screen | grep -qE '^ +(mode +|No stack chosen)' || fail "the area changed with Tab: $(tui_screen)"
	log 'scenario: Tab, Right and Left switched the area without digits and Tab led back to the content'

	step 'the arrows lead between the tab line and the content: Down into the list, Up in the first row back, Escape in Runs stays in Runs'
	tui_send 1
	tui_wait 'CURRENT' 10
	tui_send Home
	tui_send Up
	tui_wait 'next area' 10
	tui_send Right Right
	tui_wait 'STARTED' 10
	tui_has 'next area' || fail "Right did not keep the focus on the tab line: $(tui_screen)"
	tui_send Down
	sleep 0.5
	tui_has 'next area' && fail "Down did not lead into the Runs list: $(tui_screen)"
	tui_send Home
	tui_send Up
	tui_wait 'next area' 10
	tui_send Down
	sleep 0.5
	tui_has 'next area' && fail "Down did not lead into the Runs list again: $(tui_screen)"
	tui_send Escape
	tui_wait 'next area' 10
	tui_has 'STARTED' || fail "Escape in Runs left the area: $(tui_screen)"
	log 'scenario: the arrows and Escape led between the tab line and the content'

	step 'Settings General: Deutsch is saved and the interface switches at once, keeps it after a restart, and English is saved back'
	tui_send Escape
	tui_subtab General
	tui_wait 'Start area' 10
	# The language names stand only in the language field, so they are matched without its frame.
	tui_wait 'English' 10
	tui_send Tab Tab
	tui_wait '> Language' 10
	tui_send Right
	tui_wait 'Deutsch' 10
	tui_send F2
	tui_wait 'Die Einstellungen sind gespeichert.' 10
	tui_wait "4 $tui_icon_settings Einstellungen" 10
	tui_send Escape
	tui_wait 'Zugangsdaten' 10
	tui_send 1
	tui_wait "2 $tui_icon_detail Stack-Details" 10
	tui_wait 'q Beenden' 10
	log 'scenario: the interface switched to Deutsch when the General page was saved:'
	tui_excerpt 'qhost' 4
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "2 $tui_icon_detail Stack-Details" 30
	tui_wait 'demo-stack' 30
	tui_wait 'q Beenden' 10
	tui_subtab Allgemein
	tui_wait 'Startbereich' 10
	tui_send Tab Tab
	tui_wait '> Sprache' 10
	tui_send Left
	tui_wait 'English' 10
	tui_send F2
	tui_wait 'The settings are saved.' 10
	tui_send Escape
	tui_wait 'Credentials' 10
	tui_send 1
	tui_wait "2 $tui_icon_detail Stack detail" 10
	tui_has 'Stack-Details' && fail "the interface still shows German after English was saved: $(tui_screen)"
	log 'scenario: Deutsch stayed after the restart of the interface and English was saved back'

	step 'q ends the interface with exit code 0'
	tui_send Escape
	tui_send 1
	sleep 0.3
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
}

# tui_commit_only_version requires that a commit moves only DEMO_APP_VERSION of one stack, from $3 to $4, with
# the subject the update gives it.
tui_commit_only_version() {
	[ "$(as_admin "cd ~/stacks && git show --format= --numstat $1")" = "$(printf '1\t1\t%s/.env' "$2")" ] ||
		fail "the commit $1 does not change exactly one line of $2/.env"
	[ "$(as_admin "cd ~/stacks && git show --format= -U0 $1 | grep '^[-+]' | grep -v '^\(---\|+++\)'")" = "$(printf -- '-DEMO_APP_VERSION=%s\n+DEMO_APP_VERSION=%s' "$3" "$4")" ] ||
		fail "the commit $1 does not only move DEMO_APP_VERSION of $2 from $3 to $4"
	[ "$(as_admin "cd ~/stacks && git log -1 --format=%s $1")" = "Update $2 DEMO_APP_VERSION from $3 to $4" ] ||
		fail "the commit $1 has an unexpected subject: $(as_admin "cd ~/stacks && git log -1 --format=%s $1")"
}

health_is() {
	[ "$(docker exec "$host" docker inspect -f '{{.State.Health.Status}}' "$1")" = "$2" ]
}

# tui_ct_cursor_to moves the cursor of the Containers list to the container $1.
tui_ct_cursor_to() {
	i=0
	until tui_screen | grep -q "^> *$1 "; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "the cursor does not reach $1: $(tui_screen)"
		tui_send Down
		sleep 0.2
	done
}

# scenario_adopt proves that the interface takes over a foreign Compose project: a person starts the project app
# below /srv/qd-foreign with docker compose so that qhost would pass the same project name (Compose derives app
# from the folder) and the same files (compose.yaml and .env, both directly in the folder, which Compose records
# in its labels), so the interface shows no warning. The Containers area shows it as foreign with its folder and
# files, a puts the question of the folder of files, and after yes the folder is a local source with exactly one
# stack whose first deploy leaves the running containers alone (same ids). The project is shut down and the
# source released at the end.
scenario_adopt() {
	base=/srv/qd-foreign/app
	image=registry.$domain/demo/app
	source_id=srv-qd-foreign-app

	step 'a person starts a foreign Compose project below /srv/qd-foreign'
	docker exec "$host" sh -ec "rm -rf /srv/qd-foreign && mkdir -p '$base' && chmod 0755 /srv/qd-foreign '$base'" || fail 'cannot create the foreign project folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/compose.yaml'"
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: foreign-app
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	docker exec "$host" sh -ec "chmod 0644 '$base/compose.yaml' '$base/.env'" || fail 'cannot set the modes of the foreign project files'
	docker exec "$host" sh -ec "cd '$base' && docker compose up -d" >/dev/null 2>&1 || fail 'cannot start the foreign project'
	wait_for 'the foreign project app' 60 stack_healthy app
	ids_before=$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=app | sort)
	[ -n "$ids_before" ] || fail 'the foreign project has no running container'
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=app --format '{{.Label "com.docker.compose.project.working_dir"}}')" = "$base" ] ||
		fail 'the foreign project does not run from its folder'
	if list_has_path "$base"; then fail 'the folder of the foreign project is bound before the takeover'; fi
	[ -z "$(stack_field app source)" ] || fail 'a stack app exists before the takeover'

	step 'the Containers area shows the project as foreign, with its folder and files'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 5
	tui_wait 'foreign project app' 30
	tui_ct_cursor_to app-app-1
	tui_send Enter
	tui_wait 'Compose files' 15
	tui_wait "$base/compose.yaml" 10
	tui_wait 'Env files' 10
	tui_has "$base" || fail "the detail does not show the folder: $(tui_screen)"
	log 'scenario: the detail of a container of the foreign project shows its folder and files:'
	tui_excerpt 'folder' 8

	step 'a opens the takeover; it shows the labels, no warning, and changes nothing before the answer'
	tui_send a
	tui_wait 'Start a git repository in /srv/qd-foreign/app and take it over?' 30
	tui_wait 'Foreign project' 10
	tui_wait 'project: app' 10
	tui_wait "compose files (labels): $base/compose.yaml" 10
	tui_wait "Compose reads the file .env of the folder" 10
	if tui_has 'the running containers'; then fail "the takeover warns although qhost would pass the same project and files: $(tui_screen)"; fi
	if tui_has 'create new containers'; then fail "the takeover warns about new containers: $(tui_screen)"; fi
	if docker exec "$host" test -e "$base/.git"; then fail 'the question made a repository'; fi
	if list_has_path "$base"; then fail 'the question bound the folder'; fi
	[ "$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=app | sort)" = "$ids_before" ] ||
		fail 'the question changed the containers'

	step 'yes adds the folder as a local source with exactly one stack'
	tui_send y
	tui_wait "The stack folder $base is added" 120
	tui_send Enter
	wait_for 'the deploy service to answer' 30 qd status
	list_has_path "$base" || fail "source list does not show the folder: $(qd --output json source list)"
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the folder is not a local source'
	docker exec "$host" test -d "$base/.git" || fail 'the takeover made no repository'
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	printf '%s' "$out" | jq -e --arg s "$source_id" '[.[] | select(.source == $s)] | (length == 1 and .[0].stack == "app")' >/dev/null ||
		fail "stack list does not show exactly one stack app of the folder: $out"
	out=$(qd --output json stack check app) || fail "stack check of app failed: $out"
	printf '%s' "$out" | jq -e --arg s "$source_id" '.errors == 0 and (.stacks | length) == 1 and .stacks[0].stack == "app" and .stacks[0].source == $s' >/dev/null ||
		fail "unexpected check of app: $out"

	step 'the first deploy of app takes over the running containers: same ids, commit deployed'
	i=0
	until stack_settled app succeeded; do
		i=$((i + 1))
		[ "$(stack_field app last_run)" != failed ] || fail "the first run of app failed: $(qd stack log "$(newest_run app)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of app: $(qd --output json status | tr ',' '\n' | grep -e foreign -e '\.app\.')"
		sleep 1
	done
	head=$(docker exec "$host" git -c safe.directory="$base" -C "$base" rev-parse HEAD)
	[ "$(stack_field app deployed)" = "$head" ] || fail "app is not recorded as deployed at $head: $(stack_field app deployed)"
	[ "$(stack_images app)" = "$image:1.0.0" ] || fail "app does not run 1.0.0: $(stack_images app)"
	ids_after=$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=app | sort)
	[ "$ids_after" = "$ids_before" ] || fail "the first deploy replaced the containers: $ids_before vs $ids_after"
	log "scenario: the foreign project app was taken over through the interface; the deploy of $head kept the container ids"

	step 'the project is shut down and the source released'
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(qd --output json source remove "$base" --yes) || fail "source remove of the foreign folder failed: $out"
	if list_has_path "$base"; then fail 'source list still shows the foreign folder'; fi
	docker exec "$host" sh -ec "cd '$base' && docker compose down" >/dev/null 2>&1 || fail 'cannot shut down the foreign project'
	docker exec "$host" rm -rf /srv/qd-foreign
	wait_for 'the deploy service to answer' 30 qd status
}

# prop_submit submits a source-add proposal for the folder $1 with the address $2 as the administrator without sudo
# and prints its id.
prop_submit() {
	out=$(printf '{"path":"%s","url":"%s"}' "$1" "$2" | as_admin 'qhost --output json proposal submit --kind source-add' 2>&1) ||
		fail "qhost proposal submit without sudo failed: $out"
	printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1
}

# prop_state prints the state of the proposal $1 as the administrator without sudo reads it.
prop_state() {
	as_admin "qhost --output json proposal show $1" | sed -n 's/.*"state": *"\([a-z]*\)".*/\1/p' | head -n 1
}

# prop_is succeeds when the proposal $1 is in the state $2.
prop_is() {
	[ "$(prop_state "$1")" = "$2" ]
}

# prop_tui starts the interface under sudo in a fresh tmux session and waits for the notice of the open proposals.
prop_tui() {
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '1 open proposal waits for you' 30
}

# scenario_tui_proposals proves approving a proposal in the interface. An administrator without sudo proposes to
# add a missing folder as a clone; under sudo the interface lists it with its effect, and approving it runs the
# way of adding a folder by hand: the host key is confirmed, the folder is cloned and bound, and the proposal
# is applied. A second proposal is approved after its folder was changed: the approval is refused and nothing
# is bound. The repository holds one file and no stack, so nothing is deployed.
scenario_tui_proposals() {
	pa=/srv/qd-prop-a
	pb=/srv/qd-prop-b
	url7=git.$domain:stacks7.git

	step 'a repository without a stack and a deploy key for the folder of the first proposal'
	docker exec "$host" rm -rf "$pa" "$pb"
	docker exec "$prefix-git" su git -s /bin/sh -c "rm -rf /srv/git/stacks7.git && git init -q --bare --initial-branch=main /srv/git/stacks7.git" ||
		fail 'cannot create the repository stacks7'
	as_admin "rm -rf ~/stacks7 && mkdir -p ~/stacks7/notes && cd ~/stacks7 && printf 'notes\\n' >notes/readme.txt && git init -q -b main &&
		git add -A && git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Add the notes' &&
		git push -q $url7 main" || fail 'cannot fill the repository stacks7'
	register_folder_key "$pa"
	if list_has_path "$pa" || list_has_path "$pb"; then fail 'a folder of the proposals is bound before them'; fi

	step 'a proposal of an administrator without sudo shows in the interface with its effect'
	id1=$(prop_submit "$pa" "$url7")
	[ -n "$id1" ] || fail 'the first proposal has no id'
	prop_tui
	tui_send 6
	tui_wait 'stack folder' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait "path: $pa" 10
	tui_wait "address: $url7" 10
	tui_has "$id1" || fail "the view does not show $id1: $(tui_screen)"
	[ "$(prop_state "$id1")" = open ] || fail 'the proposal is not open before the approval'

	step 'A leads through the clone, the address and the host key to the question; nothing is bound before yes'
	tui_send A
	tui_wait "Clone a repository into $pa?" 30
	tui_send y
	tui_wait 'Address of the repository to clone' 10
	tui_has "$url7" || fail "the address of the proposal is not prefilled: $(tui_screen)"
	tui_send F2
	tui_wait 'Trust this host key for' 60
	tui_has "$(git_ed25519_fingerprint)" || fail "the host key question does not show the fingerprint: $(tui_screen)"
	tui_send y
	tui_wait "Approve proposal $id1 and apply these changes to $pa?" 30
	[ "$(prop_state "$id1")" = open ] || fail 'the proposal left the state open before the last answer'
	if list_has_path "$pa"; then fail 'the folder is bound before the last answer'; fi
	if docker exec "$host" test -e "$pa"; then fail 'the folder exists before the last answer'; fi

	step 'yes binds the folder and the proposal is applied'
	tui_send y
	tui_wait "The stack folder $pa is bound" 120
	tui_send Enter
	wait_for 'the qhost service to answer' 30 qd status
	list_has_path "$pa" || fail "source list does not show $pa: $(qd --output json source list)"
	docker exec "$host" test -f "$pa/notes/readme.txt" || fail 'the clone is missing in the folder of the proposal'
	tui_wait 'applied' 30
	wait_for 'the proposal to be applied' 30 prop_is "$id1" applied
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'a folder that changed between the view and the approval is refused and nothing is bound'
	id2=$(prop_submit "$pb" "$url7")
	[ -n "$id2" ] || fail 'the second proposal has no id'
	prop_tui
	tui_send 6
	tui_wait 'stack folder' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait "path: $pb" 10
	# A folder with a file is a folder of files and no longer a missing one to clone into: the effect differs.
	docker exec "$host" sh -ec "mkdir -p '$pb' && chmod 0755 '$pb' && printf 'late\\n' >'$pb/late.txt' && chmod 0644 '$pb/late.txt'" ||
		fail "cannot change the folder $pb"
	tui_send A
	tui_wait 'changed since it was shown; nothing was applied' 30
	tui_tmux kill-server >/dev/null 2>&1 || true
	[ "$(prop_state "$id2")" = open ] || fail "the refused proposal is not open: $(prop_state "$id2")"
	if list_has_path "$pb"; then fail 'the refused approval bound the folder'; fi
	[ "$(docker exec "$host" ls -A "$pb")" = late.txt ] || fail 'the refused approval changed the folder'

	step 'the folders are released and removed'
	out=$(qd --output json source remove "$pa" --yes) || fail "source remove of $pa failed: $out"
	out=$(as_admin "qhost --output json proposal withdraw $id2 2>&1") || fail "withdrawing the refused proposal failed: $out"
	docker exec "$host" rm -rf "$pa" "$pb"
	as_admin 'rm -rf ~/stacks7'
	docker exec "$prefix-git" rm -rf /srv/git/stacks7.git
	wait_for 'the qhost service to answer' 30 qd status
	log "scenario: proposal $id1 was approved in the interface and applied; proposal $id2 was refused after its folder changed"
}

# prop_submit_kind submits the JSON $2 as a proposal of the kind $1 as the administrator without sudo and prints its
# id.
prop_submit_kind() {
	out=$(printf '%s' "$2" | as_admin "qhost --output json proposal submit --kind $1" 2>&1) ||
		fail "qhost proposal submit --kind $1 without sudo failed: $out"
	printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1
}

# scenario_tui_proposal_kinds proves approving the proposals of the kinds stack-settings, window and notify-channel
# in the interface. An administrator without sudo proposes a changed update ceiling for demo-stack, a new window and
# a webhook channel; a person approves each under sudo, one proposal at a time. The channel carries no secret: the
# person types the address and the signing key into the hidden form of the approval, and the test message reaches
# the webhook receiver. The settings and the window are put back and the channel is removed, so later sections see
# the state they had before.
scenario_tui_proposal_kinds() {
	pk_stack=demo-stack
	pk_file=/opt/stacks/.qatlas/qhost/$pk_stack.yaml
	pk_hook_path=/prop-hook-e2e-path
	pk_hook_url=https://hooks.$domain$pk_hook_path
	pk_hook_key=prop-signing-value-e2e

	step 'the settings of demo-stack, the window and the channels in force are noted'
	pk_yaml=$(docker exec "$host" cat "$pk_file") || fail "cannot read $pk_file"
	[ -n "$pk_yaml" ] || fail "$pk_file is empty"
	out=$(qd --output json stack settings "$pk_stack") || fail "stack settings failed: $out"
	pk_settings=$(printf '%s' "$out" | jq -c '.settings') || fail "stack settings shows no settings: $out"
	[ "$pk_settings" != null ] || fail "$pk_stack has no settings: $out"
	out=$(qd --output json window show) || fail "window show failed: $out"
	pk_days=$(printf '%s' "$out" | jq -r '.days | if type == "array" then join(",") else . end')
	pk_start=$(printf '%s' "$out" | jq -r '.start')
	pk_duration=$(printf '%s' "$out" | jq -r '.duration')
	pk_zone=$(printf '%s' "$out" | jq -r '.zone')
	if qd --output json notify list | grep -q '"prop-hook"'; then fail 'the channel prop-hook exists before the proposal'; fi
	[ "$(as_admin 'qhost --output json proposal list' | jq '(if type == "array" then . else .proposals end) | [.[]? | select(.state == "open")] | length')" = 0 ] ||
		fail 'a proposal is open before the section'

	step 'stack settings: the difference to the settings in force shows, yes saves them'
	pk_ceiling=patch
	[ "$(printf '%s' "$pk_settings" | jq -r '.update.ceiling // ""')" != patch ] || pk_ceiling=minor
	pk_proposed=$(printf '%s' "$pk_settings" | jq -c --arg c "$pk_ceiling" '.update.ceiling = $c')
	id=$(prop_submit_kind stack-settings "{\"stack\":\"$pk_stack\",\"settings\":$pk_proposed}")
	[ -n "$id" ] || fail 'the settings proposal has no id'
	prop_tui
	tui_send 6
	tui_wait 'stack settings' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait "stack: $pk_stack" 10
	tui_wait 'These settings would change:' 10
	tui_wait 'update.ceiling' 10
	tui_wait "after:  \"$pk_ceiling\"" 10
	if printf '%s' "$pk_settings" | jq -e '.backup.command' >/dev/null; then
		tui_wait 'The backup command runs on the host later' 10
	fi
	[ "$(prop_state "$id")" = open ] || fail 'the settings proposal is not open before the approval'
	tui_send A
	tui_wait "and apply these settings to the stack $pk_stack?" 30
	[ "$(prop_state "$id")" = open ] || fail 'the settings proposal left the state open before yes'
	[ "$(docker exec "$host" cat "$pk_file")" = "$pk_yaml" ] || fail 'the settings changed before yes'
	tui_send y
	tui_wait "The settings of the stack $pk_stack are saved." 60
	tui_send Enter
	wait_for 'the settings proposal to be applied' 30 prop_is "$id" applied
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(qd --output json stack settings "$pk_stack") || fail "stack settings failed: $out"
	[ "$(printf '%s' "$out" | jq -r '.settings.update.ceiling')" = "$pk_ceiling" ] || fail "the settings do not hold the ceiling $pk_ceiling: $out"
	[ "$(docker exec -u qhost-deploy "$host" git -C /opt/stacks log -1 --format=%s)" = "qhost: settings of $pk_stack" ] ||
		fail 'the settings were not committed by the service'

	step 'stack settings are put back'
	settings_set "$pk_stack" "$pk_yaml"
	out=$(qd --output json stack settings "$pk_stack") || fail "stack settings failed: $out"
	[ "$(printf '%s' "$out" | jq -cS '.settings')" = "$(printf '%s' "$pk_settings" | jq -cS .)" ] ||
		fail "the settings of $pk_stack were not restored: $out"

	step 'window: the difference to the window in force shows, yes sets it'
	pk_new_start=$(date -u -d '+10 hours' +%H:%M)
	[ "$pk_new_start" != "$pk_start" ] || pk_new_start=$(date -u -d '+9 hours' +%H:%M)
	id=$(prop_submit_kind window "{\"days\":[\"daily\"],\"start\":\"$pk_new_start\",\"duration\":\"2m\",\"zone\":\"UTC\"}")
	[ -n "$id" ] || fail 'the window proposal has no id'
	prop_tui
	tui_send 6
	tui_wait 'maintenance window' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait 'The maintenance window would change:' 10
	tui_wait "after:  \"$pk_new_start\"" 10
	tui_send A
	tui_wait 'and set this maintenance window?' 30
	[ "$(window_field start)" = "$pk_start" ] || fail 'the window changed before yes'
	tui_send y
	tui_wait 'The maintenance window is changed.' 60
	tui_send Enter
	wait_for 'the window proposal to be applied' 30 prop_is "$id" applied
	tui_tmux kill-server >/dev/null 2>&1 || true
	[ "$(window_field start)" = "$pk_new_start" ] && [ "$(window_field duration)" = 2m ] && [ "$(window_field zone)" = UTC ] ||
		fail "the window does not hold the proposal: $(qd --output json window show)"

	step 'window is put back'
	out=$(qd --output json window set --days "$pk_days" --start "$pk_start" --duration "$pk_duration" --zone "$pk_zone") ||
		fail "window set failed: $out"
	[ "$(window_field start)" = "$pk_start" ] && [ "$(window_field duration)" = "$pk_duration" ] && [ "$(window_field zone)" = "$pk_zone" ] ||
		fail "the window was not restored: $(qd --output json window show)"
	[ "$(window_field open)" = false ] || fail "the window is open after it was restored: $(qd --output json window show)"

	step 'notify channel: the person types the secret parts; yes adds an active channel'
	id=$(prop_submit_kind notify-channel '{"name":"prop-hook","type":"webhook","events":["succeeded","rolled-back"]}')
	[ -n "$id" ] || fail 'the channel proposal has no id'
	prop_tui
	tui_send 6
	tui_wait 'notification channel' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait 'The channel would be added' 10
	tui_send A
	tui_wait 'Secret parts of the channel' 30
	tui_wait 'Webhook URL' 10
	tui_type "$pk_hook_url"
	tui_send Tab
	tui_type "$pk_hook_key"
	tui_has "$pk_hook_url" && fail "the address shows on the screen: $(tui_screen)"
	tui_has "$pk_hook_key" && fail "the signing key shows on the screen: $(tui_screen)"
	tui_send F2
	tui_wait 'and add the channel prop-hook (webhook)?' 30
	[ "$(prop_state "$id")" = open ] || fail 'the channel proposal left the state open before yes'
	if qd --output json notify list | grep -q '"prop-hook"'; then fail 'the channel exists before yes'; fi
	tui_send y
	tui_wait 'The channel prop-hook is added.' 60
	tui_send Enter
	wait_for 'the channel proposal to be applied' 30 prop_is "$id" applied
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(qd notify list) || fail "notify list failed: $out"
	printf '%s\n' "$out" | grep '^prop-hook ' | grep -q webhook || fail "the channel prop-hook is not listed as a webhook: $out"
	out=$(qd notify test prop-hook) || fail "notify test prop-hook failed: $out"
	hook_seen "$pk_hook_path" test || fail 'the webhook receiver did not record the test message of the approved channel'
	out=$(as_admin "qhost --output json proposal show $id") || fail "proposal show failed: $out"
	for value in "$pk_hook_url" "$pk_hook_path" "$pk_hook_key"; do
		if printf '%s' "$out" | grep -qF -- "$value"; then fail 'a secret part of the channel is kept on the proposal'; fi
	done
	out=$(qd notify remove prop-hook) || fail "notify remove prop-hook failed: $out"
	if qd --output json notify list | grep -q '"prop-hook"'; then fail 'the channel prop-hook remains'; fi
	log "scenario: the proposals of a stack's settings, the window and a webhook channel were approved in the interface and applied; settings and window are back, the channel is removed"
}

# --- First setup of a fresh service through the terminal interface only ---

tui_login_user=alice-e2e
tui_registry_host=quay.other.example.test
tui_login_value=tui-registry-login-e2e
tui_hook_url_value=https://hooks.$domain/tui-hook-e2e-path
tui_hook_signing_value=tui-signing-value-e2e
tui_stack_value=tuivalue-e2e-0451

# tui_type types text into the interface.
tui_type() {
	tui_tmux send-keys -t qdtui -l -- "$1"
}

# tui_clear empties the text field that has the focus.
tui_clear() {
	tui_send BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace
	tui_send BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace
}

# tui_subtab opens the sub-tab of the Settings area that is named $1, whatever page was open, and leads the focus
# into its content. The digit shows the area with the focus in the content, so the sub-tab line is not focused and
# draws no mark before its first tab; the third line of the screen is that line (header, tab line, sub-tab line).
# The name is clicked, as the page stands where only spaces precede it.
tui_subtab() {
	tui_send 4
	sleep 0.4
	cell=$(tui_screen | awk -v t="$1" 'NR == 3 && (i = index($0, t)) { print i, NR; exit }')
	[ -n "$cell" ] || fail "cannot click the sub-tab '$1', it is not on the sub-tab line: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
	sleep 0.4
	tui_send Down
	sleep 0.3
}

# tui_key_generate runs the form of the deploy key view and chooses to generate; the result dialog is closed.
tui_key_generate() {
	tui_send e
	tui_wait 'store an existing key' 10
	tui_send Right
	sleep 0.3
	tui_send F2
	tui_wait 'The deploy key is generated' 30
	tui_send Enter
}

# tui_pick_to moves the cursor of an open choice to the row that starts with the text $1.
tui_pick_to() {
	i=0
	until tui_screen | grep -q "^> *$1"; do
		i=$((i + 1))
		[ "$i" -lt 12 ] || fail "the cursor does not reach $1: $(tui_screen)"
		tui_send Down
		sleep 0.2
	done
}

# first_setup_reset gives the service a fresh state: no key, credentials, channels, repository or runs, and
# no containers of the earlier sections.
first_setup_reset() {
	docker exec "$host" systemctl stop qhost-deploy || fail 'cannot stop the deploy service'
	docker exec "$host" sh -ec '
		for c in $(docker ps -aq --filter label=com.docker.compose.project); do docker rm -f -v "$c" >/dev/null; done
		for v in $(docker volume ls -q --filter label=com.docker.compose.project); do docker volume rm "$v" >/dev/null; done
		find /opt/stacks /opt/stacks2 /opt/stacks3 /var/lib/qhost-deploy -mindepth 1 -delete' || fail 'cannot reset the state of the service'
	docker exec "$host" systemctl start qhost-deploy ||
		fail 'cannot start the fresh deploy service'
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the fresh deploy service to answer' 20 qd status
}

# first_setup_stack adds the stack demo-echo to the repository: its container name carries the value of the
# secret DEMO_ECHO_TAG, so Compose prints that value when it creates and starts the container.
first_setup_stack() {
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir demo-echo'
	cat <<EOF | as_admin 'cat >"$HOME"/stacks/demo-echo/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    container_name: demo-echo-\${DEMO_ECHO_TAG}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-echo
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
    volumes:
      - ./data:/data
EOF
	printf 'version: 1\nbackup:\n  paths:\n    - data\n  retention:\n    keep: 5\n' | as_admin 'cat >"$HOME"/stacks/.qatlas/qhost/demo-echo.yaml'
	printf 'DEMO_APP_VERSION=1.0.1\nDEMO_HEALTH=ok\n' | as_admin 'cat >"$HOME"/stacks/demo-echo/.env'
	as_admin 'cd ~/stacks && cp demo-cache/.gitignore demo-echo/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the demo-echo stack" && git push -q origin main'
}

# prepare_tui_local makes the repository without a remote that the interface adds as a local stack folder: the demo
# stack demo-tui-local with a committed environment file and a secrets file the service can read but git does not
# track. It stands below $local_dir.
prepare_tui_local() {
	docker exec "$host" sh -ec "rm -rf '$local_dir' && mkdir -p '$local_dir/demo-tui-local' && chmod 0755 '$local_dir'" ||
		fail 'cannot create the local folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$local_dir/demo-tui-local/compose.yaml'"
name: demo-tui-local

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-tui-local
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$local_dir/demo-tui-local/.env'"
	printf 'secrets.env\ndata/\n' | docker exec -i "$host" sh -ec "cat >'$local_dir/demo-tui-local/.gitignore'"
	docker exec "$host" sh -ec "cd '$local_dir' && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-tui-local stack' &&
		printf 'DEMO_SECRET=not-a-real-secret\n' >demo-tui-local/secrets.env && chgrp qhost-deploy demo-tui-local/secrets.env &&
		chmod 0640 demo-tui-local/secrets.env" ||
		fail 'cannot create the local repository'
	[ -z "$(docker exec "$host" git -C "$local_dir" remote)" ] || fail 'the local repository has a remote'
}

# tui_stack_folder prints the folder the Stacks list shows the stack $1 under: the stacks stand below the row of
# their folder, which reads "<mark> <path> (<count>)" and its cells; the mark is "#" in the goldens and a symbol in a
# terminal.
tui_stack_folder() {
	tui_screen | awk -v s="$1" '
		/\[.\] [^ ]+ \// { f = $0; sub(/^.*\[.\] [^ ]+ /, "", f); sub(/ \([0-9]+\).*$/, "", f); next }
		$0 ~ ("\\[.\\] " s " ") { print f; exit }'
}

# tui_wait_stack_folder waits up to 60 seconds until the Stacks list shows the stack $1 under the folder $2.
tui_wait_stack_folder() {
	i=0
	until [ "$(tui_stack_folder "$1")" = "$2" ]; do
		i=$((i + 1))
		if [ "$i" -ge 240 ]; then
			printf '%s\n' "$(tui_screen)" >&2
			fail "the Stacks list does not show $1 under $2"
		fi
		sleep 0.25
	done
}

scenario_tui_setup() {
	tui_log=$work/tui-screens.log
	: >"$tui_log"
	image=registry.$domain/demo/app

	step 'the service gets a fresh state; the stack demo-echo joins the repository'
	first_setup_stack
	first_setup_reset
	if qd key show >/dev/null; then fail 'the fresh service has a deploy key'; fi
	if qd credential list | grep -q registry; then fail "the fresh service has credentials: $(qd credential list)"; fi

	step 'sudo qhost opens the interface of the fresh service'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait 'No stack folders yet' 30

	step 'Stacks: adding the clone stops at the missing deploy key; the key is generated and its public part shown'
	tui_send a
	tui_wait 'Add a stack folder' 10
	tui_type /opt/stacks
	tui_send F2
	tui_wait 'Clone a repository into /opt/stacks?' 10
	tui_send y
	tui_wait 'Address of the repository to clone' 10
	tui_type "git.$domain:stacks.git"
	tui_send F2
	tui_wait 'ssh-ed25519' 60
	tui_send y
	tui_wait '/opt/stacks has no deploy key yet.' 30
	tui_send Enter
	tui_wait 'No deploy key yet' 10
	tui_key_generate
	tui_wait 'Add this public key' 10
	pub=$(tui_screen | grep -o 'ssh-ed25519 [A-Za-z0-9+/=]*' | head -n 1)
	[ -n "$pub" ] || fail "the public key is not on the screen: $(tui_screen)"
	cli_pub=$(qd --output json key show --dir /opt/stacks | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p' | cut -d ' ' -f 1,2)
	[ "$pub" = "$cli_pub" ] || fail "the key on the screen differs from the key of the service: $pub vs $cli_pub"
	printf '%s\n' "$pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	# Escape closes the key view; nothing was added.
	tui_send Escape
	tui_wait 'No stack folders yet' 10
	if list_has_path /opt/stacks; then fail 'the stopped add bound /opt/stacks'; fi
	log 'scenario: deploy key generated through the interface and registered on the git server'

	step 'Settings: a registry credential is set after a question'
	tui_subtab Credentials
	tui_wait 'x remove' 10
	tui_send a
	tui_wait 'Set a credential' 10
	tui_send Tab
	tui_type "$tui_registry_host"
	# Escape on a changed form asks first; the default answer keeps the form and its value.
	tui_send Escape
	tui_wait 'Discard the changes to this form?' 10
	tui_send Enter
	tui_wait "$tui_registry_host" 10
	tui_has 'Set a credential' || fail "cancelling the discard question closed the form: $(tui_screen)"
	# A click takes the focus of a field without typing, and a click on the Save button saves like F2.
	tui_click 'Registry user'
	sleep 0.3
	tui_type "$tui_login_user"
	tui_send Tab
	tui_type "$tui_login_value"
	sleep 0.5
	tui_click '[ F2 save ]'
	tui_wait 'Store the credential' 10
	tui_send y
	tui_wait 'is stored' 20
	tui_send Enter
	tui_wait "registry.$tui_registry_host" 10
	qd credential list | grep -q "registry.$tui_registry_host" || fail "the credential is not stored: $(qd credential list)"

	step 'Settings: the maintenance window is moved away from now'
	win_start=$(date -u -d '+12 hours' +%H:%M)
	tui_subtab 'Maintenance window'
	tui_wait 'e edit' 10
	tui_send e
	tui_wait 'Change the maintenance window' 10
	tui_clear
	tui_type daily
	tui_send Tab
	tui_clear
	tui_type "$win_start"
	tui_send Tab
	tui_clear
	tui_type 1m
	tui_send Tab
	tui_clear
	tui_type UTC
	tui_send F2
	tui_wait 'Change the maintenance window?' 10
	tui_send y
	tui_wait 'The maintenance window is changed' 20
	tui_send Enter
	[ "$(window_field start)" = "$win_start" ] && [ "$(window_field open)" = false ] ||
		fail "the window was not set through the interface: $(qd --output json window show)"

	step 'Settings: a webhook channel is added and its test message is sent'
	# The arrows alone lead there from the text of the window page: up to the sub-tab line, right to the next
	# sub-tab, whose content stands at once, and down into it.
	tui_send Up
	sleep 0.3
	tui_send Right
	tui_wait 's send test' 10
	tui_send Down
	sleep 0.3
	tui_send a
	tui_wait 'Add a notification channel' 10
	tui_type tui-hook
	tui_send F2
	tui_wait 'Settings of the channel' 10
	tui_type "$tui_hook_url_value"
	tui_send Tab
	tui_type "$tui_hook_signing_value"
	tui_send F2
	tui_wait 'Add the channel tui-hook' 10
	tui_send y
	tui_wait 'The channel tui-hook is added' 20
	tui_send Enter
	tui_wait 'tui-hook' 10
	tui_send s
	tui_wait 'Send a test message through tui-hook?' 10
	tui_send y
	tui_wait 'was delivered' 60
	tui_send Enter
	hook_seen /tui-hook-e2e-path test || fail 'the webhook receiver did not record the test message of the interface'

	step 'Stacks: a stack folder is not added while its question is not answered with yes'
	local_dir=/srv/qd-tui-local
	prepare_tui_local
	dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$local_dir")
	secret_before=$(docker exec "$host" stat -c '%U:%G %a' "$local_dir/demo-tui-local/secrets.env")
	tui_send 1
	tui_wait 'No stack folders yet' 10
	tui_send a
	tui_wait 'Add a stack folder' 10
	tui_type "$local_dir"
	tui_send F2
	tui_wait "Take over the repository $local_dir as a local stack folder?" 10
	tui_send n
	tui_wait 'No stack folders yet' 10
	if list_has_path "$local_dir"; then fail 'the answer n bound the folder'; fi
	if docker exec "$host" grep -qF "$local_dir" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'the answer n wrote the drop-in'
	fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$local_dir")" = "$dir_before" ] || fail 'the answer n changed the permissions of the folder'
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$local_dir/demo-tui-local/secrets.env")" = "$secret_before" ] ||
		fail 'the answer n changed the permissions of the secrets file'
	[ -z "$(docker exec "$host" git -C "$local_dir" config core.sharedRepository)" ] || fail 'the answer n changed the repository configuration'

	step 'Stacks: the clone is added as a stack folder after the host key is shown and confirmed'
	tui_send a
	tui_wait 'Add a stack folder' 10
	tui_type /opt/stacks
	tui_send F2
	tui_wait 'Clone a repository into /opt/stacks?' 10
	tui_send y
	tui_wait 'Address of the repository to clone' 10
	tui_type "git.$domain:stacks.git"
	tui_send F2
	expected=$(git_ed25519_fingerprint)
	tui_wait "$(printf '%s' "$expected" | cut -c 1-30)" 60
	tui_wait 'ssh-ed25519' 10
	before=$(known_hosts_sum)
	tui_send y
	tui_wait "Clone git.$domain:stacks.git into /opt/stacks?" 30
	[ "$(known_hosts_sum)" = "$before" ] || fail 'the host key was pinned before the last question'
	tui_send y
	tui_wait 'The stack folder /opt/stacks is bound' 120
	tui_send Enter
	[ "$(known_hosts_sum)" != "$before" ] || fail 'no host key was pinned'
	docker exec "$host" test -f /opt/stacks/demo-echo/compose.yaml || fail 'the clone is missing in /opt/stacks'
	log "scenario: stack folder cloned through the interface, host key $expected confirmed"

	step 'the Stacks area lists the stacks of the new repository'
	tui_send 1
	tui_wait 'demo-echo' 60
	wait_for 'the first run of demo-echo' 180 stack_idle demo-echo

	step 'Stacks: the local repository is added as the second stack folder; the Stacks area lists both folders'
	tui_send 1
	tui_wait '/opt/stacks' 10
	tui_send a
	tui_wait 'Add a stack folder' 10
	tui_type "$local_dir"
	tui_send F2
	tui_wait "Take over the repository $local_dir as a local stack folder?" 10
	tui_send y
	tui_wait "The stack folder $local_dir is added" 120
	tui_send Enter
	list_has_path "$local_dir" || fail "source list does not show the local folder: $(qd --output json source list)"
	docker exec "$host" grep -qx "ReadWritePaths=-$local_dir" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the drop-in does not grant the local folder'
	tui_send 1
	tui_wait_stack_folder demo-echo /opt/stacks
	tui_wait_stack_folder demo-tui-local "$local_dir"
	log 'scenario: the Stacks area lists demo-echo under /opt/stacks and demo-tui-local under the local folder:'
	tui_excerpt 'STACK' 12
	i=0
	until stack_settled demo-tui-local succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-tui-local last_run)" != failed ] || fail "the first run of demo-tui-local failed: $(qd stack log "$(newest_run demo-tui-local)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-tui-local: $(qd --output json status | tr ',' '\n' | grep -e tui-local)"
		sleep 1
	done
	[ "$(stack_images demo-tui-local)" = "$image:1.0.0" ] || fail "demo-tui-local does not run 1.0.0: $(stack_images demo-tui-local)"

	step 'Stack detail: a secret is set after a question and the file holds the value'
	tui_send Home
	tui_cursor_to demo-echo
	tui_send Enter
	tui_wait 'Secrets' 15
	tui_send a
	tui_wait 'Set a secret' 10
	tui_type DEMO_ECHO_TAG
	tui_send Tab
	tui_type "$tui_stack_value"
	sleep 0.5
	tui_send F2
	tui_wait 'the secret DEMO_ECHO_TAG' 10
	if docker exec "$host" test -e /opt/stacks/demo-echo/secrets.env; then fail 'the secret was stored before the answer'; fi
	tui_send y
	tui_wait 'set for the stack demo-echo' 20
	tui_send Enter
	tui_wait 'DEMO_ECHO_TAG' 10
	[ "$(docker exec "$host" cat /opt/stacks/demo-echo/secrets.env)" = "DEMO_ECHO_TAG='$tui_stack_value'" ] ||
		fail "secrets.env does not hold the secret: $(docker exec "$host" cat /opt/stacks/demo-echo/secrets.env)"
	[ "$(docker exec "$host" stat -c %U:%a /opt/stacks/demo-echo/secrets.env)" = qhost-deploy:600 ] || fail 'secrets.env has the wrong owner or mode'
	[ -z "$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain)" ] || fail 'the secret made the working copy dirty'

	step 'Stacks: the first deploy of demo-echo succeeds; Compose prints the value and the run log hides it'
	prev=$(newest_run demo-echo)
	tui_send Escape
	tui_wait 'CURRENT' 10
	tui_cursor_to demo-echo
	tui_send d
	tui_wait 'Deploy demo-echo?' 10
	tui_send y
	tui_wait 'deploy finished: 1 ok, 0 failed' 60
	tui_send l
	tui_wait 'result succeeded' 240
	run=$(next_run demo-echo "$prev") || fail 'no run of demo-echo'
	wait_for "the run $run" 240 run_finished demo-echo "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the deploy of demo-echo did not succeed: $run_log"
	[ "$(stack_images demo-echo)" = "$image:1.0.1" ] || fail "demo-echo does not run 1.0.1: $(stack_images demo-echo)"
	docker exec "$host" docker inspect -f '{{.Name}}' "demo-echo-$tui_stack_value" >/dev/null || fail 'the container name does not carry the value'
	printf '%s' "$run_log" | grep -q 'demo-echo-\[redacted\]' || fail "the run log does not show what Compose printed with the value redacted: $run_log"
	if printf '%s' "$run_log" | grep -qF -- "$tui_stack_value"; then fail 'the run log holds the value of the secret'; fi
	log 'scenario: first deploy of demo-echo through the interface; Compose printed the value and the run log redacted it'
	wait_for 'the webhook for the run of demo-echo' 60 hook_seen /tui-hook-e2e-path succeeded demo-echo

	step 'Stack detail: an older backup is restored after a question'
	docker exec "$host" sh -ec 'mkdir -p /opt/stacks/demo-echo/data && printf first >/opt/stacks/demo-echo/data/marker.txt'
	out=$(qd --output json stack backup create demo-echo) || fail "backup create failed: $out"
	older=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	# Backup ids start with the second of their creation; two in one second would not be ordered by age.
	sleep 2
	docker exec "$host" sh -ec 'printf second >/opt/stacks/demo-echo/data/marker.txt'
	out=$(qd --output json stack backup create demo-echo) || fail "backup create failed: $out"
	newest=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	[ -n "$older" ] && [ "$older" != "$newest" ] || fail "no two backups: $older, $newest"
	tui_send 1
	tui_wait 'CURRENT' 10
	tui_cursor_to demo-echo
	tui_send Enter
	tui_wait 'Backups' 15
	tui_send r
	tui_wait "$newest" 15
	tui_send R
	tui_wait 'Restore a backup' 10
	if tui_screen | grep -q "$newest"; then fail 'the newest backup is offered for a restore'; fi
	tui_pick_to "$older"
	tui_send Enter
	tui_wait 'Restore the backup' 10
	tui_wait 'overwritten' 10
	[ "$(docker exec "$host" cat /opt/stacks/demo-echo/data/marker.txt)" = second ] || fail 'the question already changed the data'
	tui_send y
	tui_wait 'restored for the stack' 600
	tui_send Enter
	[ "$(docker exec "$host" cat /opt/stacks/demo-echo/data/marker.txt)" = first ] ||
		fail "the restore did not bring back the data: $(docker exec "$host" cat /opt/stacks/demo-echo/data/marker.txt)"
	wait_for 'demo-echo to run again after the restore' 120 stack_healthy demo-echo
	log "scenario: the older backup $older of demo-echo was restored through the interface"

	step 'Stacks: x releases the local stack folder only after its question is answered with yes'
	tui_send Escape
	tui_send 1
	tui_folder_to "$local_dir"
	tui_wait 'x release' 10
	tui_send x
	tui_wait "Release the stack folder $local_dir" 10
	list_has_path "$local_dir" || fail "the question already released $local_dir"
	tui_send y
	tui_wait "The stack folder $local_dir is released" 30
	tui_send Enter
	if list_has_path "$local_dir"; then fail "source list still shows $local_dir after the release: $(qd --output json source list)"; fi
	log "scenario: the Stacks area released $local_dir after the question"

	step 'q ends the interface with exit code 0'
	tui_send Escape
	tui_send 1
	sleep 0.3
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'no masked value shows on a screen, in a run log, in the journal, in a listing or in a file but secrets.env'
	[ "$(wc -l <"$tui_log")" -gt 200 ] || fail 'too few screens were captured to check'
	grep -qF DEMO_ECHO_TAG "$tui_log" || fail 'the captured screens do not show the key of the secret'
	runs_text=$(qd stack runs)
	for id in $(qd stack runs | grep -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'); do
		runs_text="$runs_text
$(qd stack log "$id")"
	done
	listing=$(qd notify list; qd --output json notify list; qd credential list; qd --output json credential list; qd --output json status)
	journal_text=$(journal)
	screens_text=$(cat "$tui_log")
	for value in "$tui_login_value" "$tui_hook_url_value" tui-hook-e2e-path "$tui_hook_signing_value" "$tui_stack_value"; do
		for place in screens_text runs_text journal_text listing; do
			eval "text=\$$place"
			if printf '%s' "$text" | grep -qF -- "$value"; then fail "a masked value shows in $place"; fi
		done
	done
	if docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -qF -- "$tui_stack_value"; then
		fail 'the value of the stack secret reached the webhook receiver'
	fi
	holders=$(docker exec "$host" grep -rlF -- "$tui_stack_value" /opt/stacks /var/lib/qhost-deploy 2>/dev/null || true)
	[ "$holders" = /opt/stacks/demo-echo/secrets.env ] || fail "the value of the stack secret stands in: $holders"
	log 'scenario: no masked value on a captured screen, in a run log, the journal or a listing; the secret value only in secrets.env'
}

# scenario_tui_rules sets the update rules of a stack in the stack detail of the interface and lets the next
# maintenance window act on them. It runs after scenario_tui_setup because only then the service is bound to one
# source and every other stack stands on its final version: demo-stack, demo-paused and demo-pinned (the only
# stacks in automatic mode) already run 1.1.0, so the window has nothing to do for them; the section checks that.
# The new stack demo-rules starts on 1.0.1 without a settings file, so it is deployed only. The observation time
# is set to 0 so that no observation stays open for the sections after this one. The window is moved away again
# at the end; the stack stays in the repository, unconfigured.
scenario_tui_rules() {
	tui_log=$work/tui-rules-screens.log
	: >"$tui_log"
	image=registry.$domain/demo/app
	rules_stack=demo-rules
	rules_file=.qatlas/qhost/$rules_stack.yaml
	guard_stacks='demo-stack demo-paused demo-pinned'

	step 'the stack demo-rules joins the repository without settings and is deployed only'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir demo-rules'
	cat <<EOF2 | as_admin 'cat >"$HOME"/stacks/demo-rules/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-rules
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.1\nDEMO_HEALTH=ok\n' | as_admin 'cat >"$HOME"/stacks/demo-rules/.env'
	as_admin 'cd ~/stacks && cp demo-cache/.gitignore demo-rules/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the demo-rules stack" && git push -q origin main'
	restart_service
	wait_for 'the first run of demo-rules' 240 stack_settled "$rules_stack" succeeded
	wait_for 'demo-rules healthy' 60 stack_healthy "$rules_stack"
	[ "$(stack_images "$rules_stack")" = "$image:1.0.1" ] || fail "demo-rules does not run 1.0.1: $(stack_images "$rules_stack")"
	out=$(qd --output json stack settings "$rules_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *false' || fail "demo-rules has settings before the form: $out"
	as_admin "cd ~/stacks && git pull -q --ff-only && test ! -e $rules_file" || fail 'the settings file of demo-rules exists before the form'
	rules_before=$(as_admin 'cd ~/stacks && git rev-parse HEAD')

	step 'Stack detail: the update rules are set to automatic with ceiling minor after a question'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait "$rules_stack" 60
	tui_click_row "$rules_stack"
	tui_send Enter
	tui_wait 'Image versions' 15
	tui_send e
	tui_wait 'Update rules of a stack' 10
	# A stack without settings starts on the defaults: manual, ceiling minor. Right steps through the choices.
	tui_screen | grep -q 'Mode .*manual' || fail "the form of an unconfigured stack does not start on manual: $(tui_screen)"
	i=0
	until tui_screen | grep -q 'Mode .*automatic'; do
		i=$((i + 1))
		[ "$i" -lt 4 ] || fail "the mode does not reach automatic: $(tui_screen)"
		tui_send Right
		sleep 0.3
	done
	tui_screen | grep -q 'Ceiling .*minor' || fail "the ceiling of the form is not minor: $(tui_screen)"
	tui_send Tab Tab Tab
	tui_clear
	tui_type 0
	sleep 0.5
	tui_send F2
	tui_wait "Save the update rules of the stack $rules_stack?" 10
	tui_has 'observation: off' || fail "the question does not show the observation: $(tui_screen)"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')" = "$rules_before" ] || fail 'the question already made a commit'
	tui_send y
	tui_wait "Update rules of the stack $rules_stack saved." 60
	tui_send Enter
	tui_send Escape
	tui_send 1
	sleep 0.3
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'the stacks repository holds one commit with the settings file of demo-rules'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	rules_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-list --count $rules_before..HEAD")" = 1 ] || fail 'the form did not make exactly one commit'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$rules_commit" ] || fail 'the settings commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --name-status $rules_commit")" = "$(printf 'A\t%s' "$rules_file")" ] ||
		fail "the settings commit does not only add $rules_file: $(as_admin "cd ~/stacks && git show --format= --name-status $rules_commit")"
	rules_yaml=$(as_admin "cd ~/stacks && git show $rules_commit:$rules_file")
	printf '%s\n' "$rules_yaml" | grep -Eq '^ +mode: automatic$' || fail "the settings file does not set mode automatic: $rules_yaml"
	printf '%s\n' "$rules_yaml" | grep -Eq '^ +ceiling: minor$' || fail "the settings file does not set ceiling minor: $rules_yaml"
	out=$(qd --output json stack settings "$rules_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *true' || fail "the service does not show the settings of demo-rules: $out"
	log "scenario: the interface committed $rules_file: $(printf '%s' "$rules_yaml" | tr '\n' ' ')"

	step 'the next maintenance window updates demo-rules and no other stack'
	started=$(status_field started_at)
	[ -n "$started" ] || fail 'status shows no start time of the service'
	for stack in $rules_stack $guard_stacks; do
		eval "runs_$(printf '%s' "$stack" | tr - _)=$(stack_run_count "$stack")"
		eval "images_$(printf '%s' "$stack" | tr - _)=$(stack_images "$stack")"
	done
	# --start has minute granularity: two minutes ahead opens the window between one and two minutes from now.
	start=$(date -u -d '+2 minutes' +%H:%M)
	out=$(qd --output json window set --days daily --zone UTC --start "$start" --duration 5m) || fail "window set failed: $out"
	[ "$(window_field open)" = false ] || fail "the window is open before its start: $(qd --output json window show)"
	window_last_before=$(window_field last_run)
	wait_for 'the window to open' 150 sh -c "docker exec -i $host qhost --output json window show | grep -q '\"open\": *true'"
	wait_for 'the window work to finish' 300 window_done
	[ "$(window_field open)" = true ] || fail "the window work ran past the end of the window: $(qd --output json window show)"
	[ "$(status_field started_at)" = "$started" ] || fail 'the service restarted'
	[ "$(stack_images "$rules_stack")" = "$image:1.1.0" ] || fail "demo-rules does not run 1.1.0: $(stack_images "$rules_stack")"
	stack_healthy "$rules_stack" || fail 'demo-rules is not healthy after the window update'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $rules_commit..HEAD")" = 1 ] || fail 'the window did not make exactly one commit'
	update_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$update_commit" ] || fail 'the update commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --numstat $update_commit")" = "$(printf '1\t1\tdemo-rules/.env')" ] ||
		fail "the update commit does not touch only one line of demo-rules/.env: $(as_admin "cd ~/stacks && git show --format= --numstat $update_commit")"
	[ "$(as_admin "cd ~/stacks && git log -1 --format=%s $update_commit")" = 'Update demo-rules DEMO_APP_VERSION from 1.0.1 to 1.1.0' ] ||
		fail "the update commit has an unexpected subject: $(as_admin "cd ~/stacks && git log -1 --format=%s $update_commit")"
	[ "$(stack_field "$rules_stack" deployed)" = "$update_commit" ] || fail "demo-rules is not recorded as deployed at $update_commit"
	[ "$(stack_run_count "$rules_stack")" = "$((runs_demo_rules + 1))" ] || fail 'the window did not make exactly one run of demo-rules'
	for stack in $guard_stacks; do
		v=$(printf '%s' "$stack" | tr - _)
		[ "$(stack_run_count "$stack")" = "$(eval "printf %s \"\$runs_$v\"")" ] || fail "the window ran $stack"
		[ "$(stack_images "$stack")" = "$(eval "printf %s \"\$images_$v\"")" ] || fail "the window changed the image of $stack: $(stack_images "$stack")"
	done
	out=$(window_far_away) || fail "window set failed: $out"
	[ "$(window_field open)" = false ] || fail "the window is still open after it was moved: $(qd --output json window show)"
	log "scenario: the window updated demo-rules to 1.1.0 with commit $update_commit"

	step 'stack settings remove returns demo-rules to deploy only'
	out=$(qd --output json stack settings remove "$rules_stack") || fail "settings remove failed: $out"
	printf '%s' "$out" | grep -q '"removed": *true' || fail "settings remove does not report the removal: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	remove_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-list --count $update_commit..HEAD")" = 1 ] || fail 'settings remove did not make exactly one commit'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$remove_commit" ] || fail 'the removal commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --name-status $remove_commit")" = "$(printf 'D\t%s' "$rules_file")" ] ||
		fail "the removal commit does not only delete $rules_file: $(as_admin "cd ~/stacks && git show --format= --name-status $remove_commit")"
	out=$(qd --output json stack settings "$rules_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *false' || fail "demo-rules still has settings after the removal: $out"
	out=$(qd_fails stack settings remove "$rules_stack") || fail "a second settings remove succeeded: $out"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')" = "$remove_commit" ] || fail 'a second settings remove made a commit'
	wait_for 'demo-rules to be idle' 120 stack_idle "$rules_stack"
	log 'scenario: demo-rules is set to automatic in the interface, updated by the window and returned to deploy only'
}

# scenario_tui_backup ticks a named volume in the backup form of the stack detail and lets the next update run
# back it up. It uses a stack of its own, demo-vol, with one named volume and no bind mount: the stacks of the
# earlier sections are in their final state and the backup rules of demo-stack are part of the backup and rollback
# sections. The observation time is set to 0 with qhost stack settings set before the form (the form leaves the
# update rules as they are), so that no observation stays open for the sections after this one; the stack stays
# in the repository, in manual mode, and nothing else acts on it. The update runs through the command line
# because only its backup step matters here.
scenario_tui_backup() {
	tui_log=$work/tui-backup-screens.log
	: >"$tui_log"
	image=registry.$domain/demo/app
	vol_stack=demo-vol
	vol_file=.qatlas/qhost/$vol_stack.yaml
	vol_name=${vol_stack}_state
	vol_store=/var/lib/qhost-deploy/backups/$vol_stack

	step 'the stack demo-vol with a named volume joins the repository and its observation time is set to 0'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir demo-vol'
	cat <<EOF2 | as_admin 'cat >"$HOME"/stacks/demo-vol/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-vol
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
    volumes:
      - state:/state

volumes:
  state: {}
EOF2
	printf 'DEMO_APP_VERSION=1.0.1\nDEMO_HEALTH=ok\n' | as_admin 'cat >"$HOME"/stacks/demo-vol/.env'
	as_admin 'cd ~/stacks && cp demo-cache/.gitignore demo-vol/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the demo-vol stack" && git push -q origin main'
	restart_service
	wait_for 'the first run of demo-vol' 240 stack_settled "$vol_stack" succeeded
	wait_for 'demo-vol healthy' 60 stack_healthy "$vol_stack"
	[ "$(stack_images "$vol_stack")" = "$image:1.0.1" ] || fail "demo-vol does not run 1.0.1: $(stack_images "$vol_stack")"
	settings_set "$vol_stack" "$(printf 'version: 1\nupdate:\n  observe: 0\n')"
	printf '%s' "$settings_out" | grep -q '"changed": *true' || fail "settings set reports no change: $settings_out"
	vol_marker=marker-$(date +%s)
	fixture_run "$vol_name" "printf '%s\n' $vol_marker >/data/marker.txt" || fail 'cannot write the marker into the volume of demo-vol'
	[ -z "$(backup_ids "$vol_stack" || true)" ] || fail "demo-vol has a backup before the form: $(qd stack backup list "$vol_stack")"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	vol_before=$(as_admin 'cd ~/stacks && git rev-parse HEAD')

	step 'Stack detail: the volume of demo-vol is ticked in the backup form and saved after a question'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait "$vol_stack" 60
	tui_click_row "$vol_stack"
	tui_send Enter
	tui_wait 'Image versions' 15
	tui_send b
	tui_wait 'Backup of a stack' 10
	# The stack has no backup section yet: the volume list offers the named volume of the compose file, unticked.
	# Tab leaves the data directories for the volumes; space ticks the volume under the cursor.
	tui_send Tab
	tui_wait '[ ] state' 5
	tui_send Space
	tui_wait '[x] state' 5
	tui_send F2
	tui_wait "Save the backup of the stack $vol_stack?" 10
	tui_has 'volumes: state' || fail "the question does not show the volume: $(tui_screen)"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')" = "$vol_before" ] || fail 'the question already made a commit'
	tui_send y
	tui_wait "Backup of the stack $vol_stack saved." 60
	tui_send Enter
	tui_send Escape
	tui_send 1
	sleep 0.3
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'the stacks repository holds one commit that only changes the settings file of demo-vol, with the volume under backup'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	vol_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-list --count $vol_before..HEAD")" = 1 ] || fail 'the form did not make exactly one commit'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$vol_commit" ] || fail 'the settings commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --name-status $vol_commit")" = "$(printf 'M\t%s' "$vol_file")" ] ||
		fail "the settings commit does not only change $vol_file: $(as_admin "cd ~/stacks && git show --format= --name-status $vol_commit")"
	vol_yaml=$(as_admin "cd ~/stacks && git show $vol_commit:$vol_file")
	printf '%s\n' "$vol_yaml" | grep -Eq '^backup:$' || fail "the settings file has no backup section: $vol_yaml"
	printf '%s\n' "$vol_yaml" | grep -Eq '^  volumes:$' || fail "the backup section has no volumes: $vol_yaml"
	printf '%s\n' "$vol_yaml" | grep -Eq '^    - state$' || fail "the settings file does not name the volume state under backup.volumes: $vol_yaml"
	printf '%s\n' "$vol_yaml" | grep -Eq '^  observe: 0$' || fail "the form changed the observation time: $vol_yaml"
	out=$(qd --output json stack settings "$vol_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"state"' || fail "the service does not show the volume in the settings of demo-vol: $out"
	[ -z "$(backup_ids "$vol_stack" || true)" ] || fail 'saving the form made a backup'
	log "scenario: the interface committed $vol_file: $(printf '%s' "$vol_yaml" | tr '\n' ' ')"

	step 'the next update run backs up the volume of demo-vol and nothing else'
	vol_runs=$(stack_run_count "$vol_stack")
	out=$(qd --output json stack update "$vol_stack" --version 1.1.0) || fail "update of demo-vol failed: $out"
	vol_run=$(json_values "$out" run | head -n 1)
	[ -n "$vol_run" ] || fail "update reported no run: $out"
	run_log=$(qd stack log "$vol_run")
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the update run did not succeed: $run_log"
	[ "$(update_steps "$run_log")" = stop,backup,commit,push,fast-forward,pull,up,check ] || fail "unexpected steps of the update run: $(update_steps "$run_log")"
	[ "$(stack_run_count "$vol_stack")" = "$((vol_runs + 1))" ] || fail 'the update did not make exactly one run'
	[ "$(stack_images "$vol_stack")" = "$image:1.1.0" ] || fail "demo-vol does not run 1.1.0: $(stack_images "$vol_stack")"
	stack_healthy "$vol_stack" || fail 'demo-vol is not healthy after the update'
	[ -z "$(stack_field "$vol_stack" observe)" ] || fail "demo-vol shows an observation: $(stack_field "$vol_stack" observe)"
	vol_backup=$(update_run_backup "$run_log")
	[ -n "$vol_backup" ] || fail "the update run made no backup: $run_log"
	[ "$(backup_ids "$vol_stack" | tr '\n' ' ')" = "$vol_backup " ] || fail "backup list does not show only $vol_backup: $(qd stack backup list "$vol_stack")"
	vol_items=$(docker exec "$host" ls "$vol_store/$vol_backup" | grep -x -e 'volume-[0-9]*\.tar' -e 'path-[0-9]*\.tar' -e command.out | tr '\n' ' ')
	[ "$vol_items" = 'volume-0.tar ' ] || fail "the backup does not hold exactly one volume archive: $vol_items"
	[ "$(docker exec -i "$host" docker run --rm --network none -v "$vol_store/$vol_backup:/b:ro" "$backup_image" sh -ec \
		'mkdir /t && tar -xf /b/volume-0.tar -C /t && cat /t/marker.txt' </dev/null)" = "$vol_marker" ] ||
		fail "the volume archive of $vol_backup does not hold the marker of the volume"
	log "scenario: the update run $vol_run backed up the volume $vol_name in $vol_backup, with its marker"
}

# scenario_tui_check sets the HTTP check of a stack in the check form of the stack detail and removes the settings
# of the stack again with the action of the detail. It uses demo-rules: the update section left it healthy, idle,
# without a settings file and in no mode that any later section touches, and no section follows this one. The
# address is made up and nothing answers there; the acceptance is the commit with the check, and the check acts
# only on a deployment, which no commit of this section causes (the stack folder itself stays unchanged).
scenario_tui_check() {
	tui_log=$work/tui-check-screens.log
	: >"$tui_log"
	check_stack=demo-rules
	check_file=.qatlas/qhost/$check_stack.yaml
	check_url=http://$check_stack.$domain/health

	step 'the stack demo-rules has no settings before the check form'
	out=$(qd --output json stack settings "$check_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *false' || fail "demo-rules has settings before the check form: $out"
	as_admin "cd ~/stacks && git pull -q --ff-only && test ! -e $check_file" || fail 'the settings file of demo-rules exists before the check form'
	check_before=$(as_admin 'cd ~/stacks && git rev-parse HEAD')

	step 'Stack detail: the HTTP check of demo-rules is switched on and saved after a question'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait "$check_stack" 60
	tui_click_row "$check_stack"
	tui_send Enter
	tui_wait 'Image versions' 15
	tui_send k
	tui_wait 'Check of a stack' 10
	# A stack without settings starts on the container check on and the HTTP check off. Tab leads to the HTTP
	# check; Right steps through its choices; the next fields are the address, the status and the timeout.
	tui_screen | grep -q 'HTTP check .*off' || fail "the HTTP check of an unconfigured stack does not start off: $(tui_screen)"
	tui_send Tab
	i=0
	until tui_screen | grep -Eq 'HTTP check .*[│║] on'; do
		i=$((i + 1))
		[ "$i" -lt 4 ] || fail "the HTTP check does not reach on: $(tui_screen)"
		tui_send Right
		sleep 0.3
	done
	tui_send Tab
	tui_clear
	tui_type "$check_url"
	tui_send Tab Tab
	tui_clear
	tui_type 5s
	sleep 0.5
	tui_send F2
	tui_wait "Save the check of the stack $check_stack?" 10
	tui_has 'HTTP check: http' || fail "the question does not show the HTTP check: $(tui_screen)"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')" = "$check_before" ] || fail 'the question already made a commit'
	tui_send y
	tui_wait "Check of the stack $check_stack saved." 60

	step 'the stacks repository holds one commit that only adds the settings file of demo-rules, with the check'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	check_commit=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-list --count $check_before..HEAD")" = 1 ] || fail 'the form did not make exactly one commit'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$check_commit" ] || fail 'the check commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --name-status $check_commit")" = "$(printf 'A\t%s' "$check_file")" ] ||
		fail "the check commit does not only add $check_file: $(as_admin "cd ~/stacks && git show --format= --name-status $check_commit")"
	check_yaml=$(as_admin "cd ~/stacks && git show $check_commit:$check_file")
	printf '%s\n' "$check_yaml" | grep -Eq '^check:$' || fail "the settings file has no check section: $check_yaml"
	printf '%s\n' "$check_yaml" | grep -Eq '^ +http:$' || fail "the check section has no http probe: $check_yaml"
	printf '%s\n' "$check_yaml" | grep -Eq "^ +url: $check_url\$" || fail "the settings file does not set the address $check_url: $check_yaml"
	printf '%s\n' "$check_yaml" | grep -Eq '^ +timeout: 5s$' || fail "the settings file does not set the timeout 5s: $check_yaml"
	out=$(qd --output json stack settings "$check_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *true' || fail "the service does not show the settings of demo-rules: $out"
	printf '%s' "$out" | grep -qF "$check_url" || fail "the service does not show the address in the settings of demo-rules: $out"
	log "scenario: the interface committed $check_file: $(printf '%s' "$check_yaml" | tr '\n' ' ')"

	step 'Stack detail: the settings of demo-rules are removed after a question'
	tui_send Enter
	tui_wait 'Image versions' 15
	tui_send E
	tui_wait "Remove all settings of the stack $check_stack?" 10
	tui_has 'deletes the settings file' || fail "the question does not name the deletion: $(tui_screen)"
	[ "$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')" = "$check_commit" ] || fail 'the question already made a commit'
	tui_send y
	tui_wait "Settings of the stack $check_stack removed." 60
	tui_send Enter
	tui_send Escape
	tui_send 1
	sleep 0.3
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'the stacks repository holds one commit that only deletes the settings file of demo-rules'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	check_remove=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	[ "$(as_admin "cd ~/stacks && git rev-list --count $check_commit..HEAD")" = 1 ] || fail 'the removal did not make exactly one commit'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$check_remove" ] || fail 'the removal commit is not on the remote branch'
	[ "$(as_admin "cd ~/stacks && git show --format= --name-status $check_remove")" = "$(printf 'D\t%s' "$check_file")" ] ||
		fail "the removal commit does not only delete $check_file: $(as_admin "cd ~/stacks && git show --format= --name-status $check_remove")"
	out=$(qd --output json stack settings "$check_stack") || fail "stack settings failed: $out"
	printf '%s' "$out" | grep -q '"configured": *false' || fail "demo-rules still has settings after the removal: $out"
	wait_for 'demo-rules to be idle' 120 stack_idle "$check_stack"
	log 'scenario: the HTTP check of demo-rules is set in the interface and its settings are removed again, one commit each'
}
