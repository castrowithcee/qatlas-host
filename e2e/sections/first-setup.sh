#!/bin/sh
# Section first-setup: the service is reset to a fresh state and the first setup runs only through the terminal
# interface: deploy key, registry credential, maintenance window, notification channel with its test, repository
# with a confirmed host key, a stack secret and the first deploy; then an older backup is restored through the
# interface and no masked value shows anywhere. The update rules of a new stack are set in the interface (automatic,
# ceiling minor): the stacks repository gets a commit with its settings file, the next maintenance window updates the
# stack, and qhost stack settings remove returns it to deploy only. The backup of a stack with a named volume is set
# in the interface (the volume ticked): the settings file gets a commit, and the next update run backs up exactly
# that volume. The HTTP check of a stack is set in the interface: the settings file gets a commit with the check, and
# "clear settings" deletes the file by a commit.
# Needs window: the stacks repository as update, window and tui leave it, with demo-stack, demo-paused and demo-pinned
# on 1.1.0 (scenario_tui_rules). Needs sources: first_setup_reset empties /opt/stacks2 and /opt/stacks3, which must
# exist.
# Sourced by run.sh; defines only functions and constants.

tui_login_user=alice-e2e

tui_login_value=tui-registry-login-e2e

tui_hook_url_value=https://hooks.$domain/tui-hook-e2e-path

tui_hook_signing_value=tui-signing-value-e2e

tui_stack_value=tuivalue-e2e-0451

# tui_pick_to moves the cursor of an open choice to the row that starts with the text $1.
tui_pick_to() {
	tui_walk_down "$1" grep -Eq "^> *$1"
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
	tui_keep_engine 30
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
	tui_wait 'e host window' 10
	tui_send e
	tui_wait 'Change the host window' 10
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
	tui_wait 'Change the maintenance window of the host?' 10
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
	tui_keep_engine 30
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
	tui_keep_engine 30
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
	tui_keep_engine 30
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
	for id in $(qd stack runs | grep -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'); do
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
	wait_for 'the window to open' 150 window_open
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
