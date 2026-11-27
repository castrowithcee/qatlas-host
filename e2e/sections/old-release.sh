#!/bin/sh
# Section old-release: the release $old_release, built from its tag and signed with the test key, is installed with
# its own installer on a host without qhost. Its deploy service is set up the way it did that, as the user
# qhost-deploy, with the webhook receiver beside it, and deploys a stack of a local source, which gives a run history;
# the engine state is then put into format 1. The real self-update of $old_release then installs a test release of
# the sources: without any other command the deploy service runs as root, the tmpfiles rules, the channel and the
# marker folder name root and qhost-hook, the account and the group qhost-deploy are gone, the state belongs to
# root, the run history stays, the state in format 1 is read, the journal shows the transition deploy-service-as-root,
# status and the terminal interface answer and the manpage of the setup is the one of the program. The .git that
# $old_release made group-writable for its service user is refused with the finding git-unsafe until sudo qhost
# source secure makes it root-only; then a new run succeeds. After a restart of the host, which makes /run afresh, the services start again and the start step
# reports the transition as already done.
# Needs no other section, but a host without qhost: it runs alone or after uninstall, the last section of the full
# run, whose leftover drop-in of the deploy unit it removes.
# Sourced by run.sh; defines only functions and constants.

# or_base is the local source of the section, or_state the engine state of its stack, or_page the manpage that
# the self-update of $old_release leaves as it was, and or_saved a copy of it on the host.
or_base=/srv/qd-old
or_state=/var/lib/qhost-deploy/sources/srv-qd-old/deploy/state.json
or_page=/usr/local/share/man/man1/qhost-deploy-service-setup.1
or_saved=/root/qhost-deploy-service-setup.1.old
# or_new is the version of the test release of the sources, newer than $old_release.
or_new=v0.9.0

# old_release_build builds the release $old_release from its tag in the code repository, signed and trusted like
# the test releases of release_build, with the modules of its own go.mod.
old_release_build() {
	src=$work/oldrelease/src
	rm -rf "$work/oldrelease"
	mkdir -p "$src"
	git -C "$root" archive "$old_release" | tar -x -C "$src" || fail "cannot unpack the tag $old_release"
	release_trust "$src" "$work/oldrelease"
	sed "s|\"targets\": .*|\"targets\": [\"linux/$(docker exec "$host" dpkg --print-architecture)\"],|" \
		"$src/release.json" >"$work/oldrelease/release.json"
	release_make "$src" "$work/oldrelease/release.json" "$work/oldrelease/overlay.json" "$old_release"
}

# or_git_safe succeeds when source list shows no finding git-unsafe for the local source.
or_git_safe() {
	! git_unsafe_shown "$or_base"
}

# or_journal prints the log of the deploy service since the second $1 of the host clock.
or_journal() {
	docker exec "$host" journalctl -u qhost-deploy --no-pager -o cat --since "@$1"
}

# or_owner prints owner, group and mode of the path $1 on the host.
or_owner() {
	docker exec "$host" stat -c %U:%G:%a "$1"
}

# or_moved checks what the move of the deploy service to root leaves, after the update and after a restart of the
# host: the service as root, the tmpfiles rule, the channel and the marker folder, no account or group qhost-deploy,
# the state of root and the run history with the run $or_run.
or_moved() {
	[ "$(docker exec "$host" stat -c %U "/proc/$(service_pid)")" = root ] ||
		fail "the deploy service does not run as root: $(docker exec "$host" stat -c %U "/proc/$(service_pid)")"
	[ "$(docker exec "$host" cat /etc/tmpfiles.d/qhost-deploy.conf)" = 'd /run/qhost-deploy 2750 root qhost -' ] ||
		fail "the tmpfiles rule of the channel does not name root: $(docker exec "$host" cat /etc/tmpfiles.d/qhost-deploy.conf)"
	[ "$(or_owner /run/qhost-deploy)" = root:qhost:2750 ] || fail "/run/qhost-deploy is $(or_owner /run/qhost-deploy), want root:qhost:2750"
	[ "$(or_owner /run/qhost-hook/system)" = qhost-hook:qhost-hook:770 ] ||
		fail "/run/qhost-hook/system is $(or_owner /run/qhost-hook/system), want qhost-hook:qhost-hook:770"
	if docker exec "$host" id qhost-deploy >/dev/null 2>&1; then fail 'the user qhost-deploy exists'; fi
	if docker exec "$host" getent group qhost-deploy >/dev/null; then fail 'the group qhost-deploy exists'; fi
	[ -z "$(docker exec "$host" find /var/lib/qhost-deploy -not -user root -o -not -group root)" ] ||
		fail 'the state of the deploy service does not all belong to root'
	out=$(qd stack runs old-demo) || fail "qhost stack runs fails: $out"
	printf '%s' "$out" | grep -q "$or_run" || fail "the run history of old-demo lost the run $or_run: $out"
	[ "$(stack_run_count old-demo)" -ge "$or_runs" ] || fail "the run history of old-demo is shorter than before the update: $out"
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo fails: $out"
	printf '%s' "$out" | grep -q '"deploy_service": *"running"' || fail "qhost status does not show the deploy service running: $out"
	if printf '%s' "$out" | grep -q 'format number'; then fail "qhost status refuses a format number: $out"; fi
}

# scenario_old_update installs $old_release, sets up its deploy service as the user qhost-deploy with a run history,
# puts the engine state into format 1 and updates with the self-update of $old_release to the test release $or_new
# of the sources, then checks the host without any other command.
scenario_old_update() {
	step "the release $old_release is built from its tag and the test release $or_new from the sources"
	if docker exec "$host" test -e /usr/local/bin/qhost; then
		fail 'qhost is installed; the section old-release needs a host without it, so run it alone or after uninstall'
	fi
	old_release_build
	release_build "$or_new"
	# The drop-in of the scenario that uninstall leaves names flags $old_release may not know.
	docker exec "$host" sh -ec 'rm -rf /etc/systemd/system/qhost-deploy.service.d && systemctl daemon-reload' ||
		fail 'cannot remove the drop-in of the deploy unit'

	step "$old_release is installed with its installer and sets up its deploy service as the user qhost-deploy, with the webhook receiver"
	release_publish "$old_release"
	as_admin 'curl -fsSL -o /tmp/install.sh https://github.com/castrowithcee/qatlas-host/releases/latest/download/install.sh &&
		sudo sh /tmp/install.sh && rm -f /tmp/install.sh' || fail "the installer of $old_release failed"
	docker exec "$host" qhost version 2>&1 | grep -q "$old_release" || fail "the installed version is not $old_release: $(docker exec "$host" qhost version 2>&1)"
	out=$(as_admin 'sudo qhost deploy-service setup --confirm --yes 2>&1') || fail "the setup of $old_release failed: $out"
	# As in setup_service, the backups use the local helper image.
	docker exec "$host" sh -ec '
		mkdir -p /etc/systemd/system/qhost-deploy.service.d
		printf "[Service]\nExecStart=\nExecStart=/usr/local/bin/qhost deploy-service run --backup-image '"$backup_image"'\n" >/etc/systemd/system/qhost-deploy.service.d/e2e.conf
		systemctl daemon-reload
		systemctl restart qhost-deploy' || fail 'cannot write the drop-in'
	wait_service
	out=$(as_admin 'sudo qhost hook-service setup --confirm --yes 2>&1') || fail "the receiver setup of $old_release failed: $out"
	wait_service
	[ "$(docker exec "$host" stat -c %U "/proc/$(service_pid)")" = qhost-deploy ] ||
		fail "the deploy service of $old_release does not run as qhost-deploy: $(docker exec "$host" stat -c %U "/proc/$(service_pid)")"
	[ "$(or_owner /run/qhost-deploy)" = qhost-deploy:qhost:2750 ] || fail "/run/qhost-deploy is $(or_owner /run/qhost-deploy), want qhost-deploy:qhost:2750"
	[ "$(or_owner /run/qhost-hook/system)" = qhost-hook:qhost-deploy:770 ] ||
		fail "/run/qhost-hook/system is $(or_owner /run/qhost-hook/system), want qhost-hook:qhost-deploy:770"
	docker exec "$host" grep -q ' qhost-deploy qhost -$' /etc/tmpfiles.d/qhost-deploy.conf ||
		fail "the tmpfiles rule of $old_release does not name qhost-deploy: $(docker exec "$host" cat /etc/tmpfiles.d/qhost-deploy.conf)"

	step "the deploy service of $old_release deploys old-demo of a local source, which gives a run history"
	docker exec "$host" sh -ec "rm -rf '$or_base' && mkdir -p '$or_base/old-demo'" || fail "cannot create $or_base"
	cat <<EOF | docker exec -i "$host" sh -ec "umask 022 && cat >'$or_base/old-demo/compose.yaml'"
name: old-demo

services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: old-demo
      DEMO_VERSION: 1.0.0
      DEMO_HEALTH: ok
EOF
	docker exec "$host" sh -ec "umask 022 && cd '$or_base' && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the old-demo stack'" ||
		fail "cannot create the repository $or_base"
	out=$(qd --output json source add "$or_base" --yes) || fail "source add of $old_release failed: $out"
	i=0
	until stack_settled old-demo succeeded; do
		i=$((i + 1))
		[ "$(stack_field old-demo last_run)" != failed ] || fail "the run of old-demo failed: $(qd stack log "$(newest_run old-demo)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the run of old-demo: $(qd --output json status | tr ',' '\n' | grep old-demo)"
		sleep 1
	done
	or_run=$(newest_run old-demo) or_runs=$(stack_run_count old-demo)
	[ -n "$or_run" ] && [ "$or_runs" -ge 1 ] || fail "old-demo has no recorded run: $(qd stack runs old-demo)"

	step 'the engine state is put into format 1, which the releases before format 2 wrote'
	docker exec "$host" systemctl stop qhost-deploy || fail 'cannot stop the deploy service'
	# Format 2 only added optional fields; without them the state is one as format 1 left it. The file is written
	# in place, so it keeps its owner and mode.
	state=$(docker exec "$host" cat "$or_state") || fail "cannot read $or_state"
	state=$(printf '%s' "$state" | jq '.schema = 1 | walk(if type == "object" then del(.failed_versions, .images) else . end)') ||
		fail "cannot change $or_state"
	printf '%s' "$state" | jq -e --arg run "$or_run" '.schema == 1 and ([.runs[].id] | index($run)) != null' >/dev/null ||
		fail "the state in format 1 lacks the run $or_run: $state"
	printf '%s\n' "$state" | docker exec -i "$host" sh -c 'cat >"$1"' sh "$or_state" || fail "cannot write $or_state"
	# $old_release keeps running with a state it cannot read; its polls fail until the update.
	docker exec "$host" systemctl start qhost-deploy || fail 'cannot start the deploy service'
	wait_for "the deploy service of $old_release" 20 docker exec "$host" sh -c 'systemctl is-active -q qhost-deploy && ls /run/qhost-deploy | grep -q sock'
	docker exec "$host" cp "$or_page" "$or_saved" || fail "cannot keep $or_page"
	log "scenario: $old_release runs the deploy service as qhost-deploy; old-demo has $or_runs runs, the newest is $or_run; state in format 1"

	step "sudo qhost update of $old_release installs $or_new, which moves the deploy service to root without any other command"
	release_publish "$or_new"
	since=$(docker exec "$host" date +%s)
	out=$(as_admin 'sudo qhost update --yes --confirm 2>&1') || fail "the update of $old_release failed: $out"
	wait_for 'the updated services' 60 service_serving
	docker exec "$host" qhost version 2>&1 | grep -q "$or_new" || fail "the installed version is not $or_new: $(docker exec "$host" qhost version 2>&1)"
	wait_for 'the webhook receiver after the update' 30 hook_listening
	if printf '%s' "$out" | grep -q -e 'was not renewed' -e 'were not all renewed'; then fail "the renewal by the new program failed: $out"; fi
	or_moved
	or_journal "$since" | grep -q 'transition deploy-service-as-root: applied' ||
		fail "the journal does not show the transition: $(or_journal "$since")"
	or_journal "$since" | grep -q 'deploy service to root: userdel qhost-deploy' ||
		fail "the journal does not show the steps of the transition: $(or_journal "$since")"
	log "scenario: updated from $old_release to $or_new; start step: $(or_journal "$since" | grep -e '^transition ' -e '^manpages: ' | tr '\n' ';')"

	step 'the manpage of the setup is the one of the program, which the self-update left as it was'
	# Only the .TH line carries the date the page was made.
	pages=$(docker exec "$host" sh -c '
		d=$(mktemp -d) && qhost manpage --dir "$d" >/dev/null || exit 1
		grep -v "^\.TH " "$d/qhost-deploy-service-setup.1" >"$d/want"
		for f in "$1" "$2"; do
			if grep -v "^\.TH " "$f" | cmp -s - "$d/want"; then echo same; else echo differs; fi
		done
		rm -rf "$d"' sh "$or_saved" "$or_page" | tr '\n' ' ')
	[ "$pages" = 'differs same ' ] ||
		fail "want the page of $old_release to differ from the one of the program and the installed one to equal it, got: $pages"
	docker exec "$host" rm -f "$or_saved"

	step 'the terminal interface starts and shows the stacks of the deploy service'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait old-demo 30
	if tui_has 'qhost-exit='; then fail "the interface ended: $(tui_screen)"; fi
	tui_tmux kill-server >/dev/null 2>&1 || true

	step "the .git that $old_release made group-writable is refused until source secure makes it root-only, then a run succeeds"
	wait_for 'old-demo without a run in progress' 60 stack_idle old-demo
	out=$(qd_fails stack deploy old-demo) || fail "a run in the group-writable .git started: $out"
	printf '%s' "$out" | grep -qF 'git is not run in this folder: .git can be written by a group or by others' ||
		fail "the refused run does not name the group-writable .git: $out"
	wait_for 'the finding git-unsafe' 60 git_unsafe_shown "$or_base"
	out=$(as_admin "sudo qhost source secure $or_base --yes 2>&1") || fail "source secure failed: $out"
	[ -z "$(docker exec "$host" find "$or_base/.git" \( ! -user root -o ! -group root -o -perm /022 \) -print)" ] ||
		fail ".git is not root-only after source secure: $(docker exec "$host" find "$or_base/.git" \( ! -user root -o ! -group root -o -perm /022 \) -print | head -n 5)"
	[ -z "$(docker exec "$host" git -c safe.directory="$or_base" -C "$or_base" config --get core.sharedRepository || true)" ] ||
		fail 'source secure left core.sharedRepository'
	# The finding goes at the next poll; the restart starts one.
	restart_service
	wait_for 'the finding git-unsafe to go' 60 or_git_safe
	wait_for 'old-demo without a run in progress' 60 stack_idle old-demo
	out=$(qd --output json stack deploy old-demo) || fail "a run after source secure failed to start: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] || fail "deploy after source secure reported no run: $out"
	wait_for "the run $run after source secure" 240 run_finished old-demo "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run after source secure did not succeed: $(qd stack log "$run")"
	or_runs=$(stack_run_count old-demo)
}

# scenario_old_reboot restarts the host container of scenario_old_update, whose /run is a tmpfs: systemd boots,
# the tmpfiles rules make the channel and the marker folder afresh, and the services start; the start step of the
# deploy service reports the transition as already done.
scenario_old_reboot() {
	step 'after a restart of the host the services start again and the start step reports the transition as already done'
	since=$(docker exec "$host" date +%s)
	docker restart -t 60 "$host" >/dev/null || fail 'cannot restart the host'
	wait_for 'the host to boot' 90 host_booted
	wait_for 'the services after the restart' 90 service_serving
	wait_for 'the webhook receiver after the restart' 30 hook_listening
	or_moved
	or_journal "$since" | grep -q 'transition deploy-service-as-root: already done' ||
		fail "the start step after the restart does not report the done transition: $(or_journal "$since")"
	if or_journal "$since" | grep -q -e 'transition .*: applied' -e 'manpages: updated'; then
		fail "the start step after the restart changed something: $(or_journal "$since")"
	fi
	log "scenario: after the restart of the host: $(or_journal "$since" | grep '^transition ' | tr '\n' ';')"
}
