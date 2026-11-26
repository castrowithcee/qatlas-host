#!/bin/sh
# Section tui-sudo: the terminal interface without sudo asks for a deploy, a container restart and a change of the
# maintenance window and runs each through the real sudo, which asks for the password; a Ctrl-C there changes nothing.
# Nested data folders, the forms with a secret, the file browser over a stack folder, its uncommitted changes and the
# folders that need sudo follow.
# Needs deploy: the bound /opt/stacks with demo-stack and demo-cache; the choice of the engine in
# scenario_tui_folder_sudo needs the rootless instance, which deploy needs.
# Sourced by run.sh; defines only functions and constants.

# scenario_tui_sudo proves the actions of the interface started without sudo that need no secret: the interface asks
# with the exact command and hands the terminal to the real sudo, which asks for the password itself. The rule of
# $admin lets sudo run a command without the password, so for this scenario it is put aside, and sudo keeps no
# password between two commands (timestamp_timeout=0); the rule that asks is the one of the group sudo. A Ctrl-C at
# the password prompt of a deploy changes nothing and the interface says so. Then a deploy of demo-cache, the
# restart of a single container and a change of the maintenance window each run with the password typed in the
# terminal, with the effect read from the service. A Ctrl-C at the password prompt of a change in the System area
# changes nothing either and is reported as an interruption. The rule and the password are put back at the end.
scenario_tui_sudo() {
	tui_log=$work/tui-sudo-screens.log
	: >"$tui_log"
	sa_login=E2e-admin-Login-6612
	sa_single=$prefix-single-sudo
	image=registry.$domain/demo/app:1.0.0
	printf '%s:%s\n' "$admin" "$sa_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	docker exec "$host" sh -ec "mv /etc/sudoers.d/e2e-admin /etc/sudoers.d/e2e-admin.off &&
		printf 'Defaults:%s timestamp_timeout=0\n' '$admin' >/etc/sudoers.d/e2e-ask && chmod 0440 /etc/sudoers.d/e2e-ask && visudo -c >/dev/null" ||
		fail 'cannot put the sudo rule of the administrator aside'
	if docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1; then fail "sudo runs a command of $admin without a password although the rule is put aside"; fi

	step 'the interface without sudo asks for the deploy; a Ctrl-C at the password prompt of sudo changes nothing'
	prev_run=$(newest_run demo-cache)
	runs_before=$(stack_run_count demo-cache)
	tui_tmux kill-server >/dev/null 2>&1 || true
	# The Ctrl-C reaches the whole process group of the pane; without job control the wrapping shell would end
	# with it once the interface quits, so it traps the signal instead (the trap is not inherited by its children).
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'trap : INT; qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '[ ] demo-cache' 30
	tui_send Home
	tui_cursor_to demo-cache
	tui_send a
	tui_wait 'Deploy demo-cache?' 15
	tui_has 'sudo qhost stack deploy' || fail "the question does not show the command: $(tui_screen)"
	tui_excerpt 'Deploy demo-cache' 12
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_send C-c
	tui_wait 'deploy finished: 0 ok, 1 failed' 30
	tui_screen | tr -s ' \n' '  ' | grep -qiE 'interrupted|did not run it' ||
		fail "the interface does not say that sudo was interrupted: $(tui_screen)"
	tui_has 'qhost-exit' && fail "the interface ended with the Ctrl-C at the password prompt: $(tui_screen)"
	sleep 2
	[ "$(newest_run demo-cache)" = "$prev_run" ] && [ "$(stack_run_count demo-cache)" = "$runs_before" ] && [ -z "$(stack_field demo-cache active_run)" ] ||
		fail "the aborted deploy started a run of demo-cache: $(qd stack runs demo-cache)"
	tui_send Escape
	tui_wait 'CURRENT' 10
	log 'scenario: a Ctrl-C at the password prompt started no run and the interface reported the interruption'

	step 'the interface without sudo deploys demo-cache after the password is typed in the terminal'
	tui_send a
	tui_wait 'Deploy demo-cache?' 15
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_wait_noecho 30
	# The handover cleared the screen: only the new prompt shows, not the one of the Ctrl-C before.
	tui_wait "[sudo] password for $admin" 10
	[ "$(tui_screen | grep -cF "[sudo] password for $admin")" = 1 ] || fail "an old sudo prompt shows after the handover: $(tui_screen)"
	tui_type "$sa_login"
	tui_send Enter
	tui_wait 'deploy finished: 1 ok, 0 failed' 240
	tui_send Escape
	sa_run=$(next_run demo-cache "$prev_run") || fail 'the deploy through sudo started no run of demo-cache'
	wait_for "the run $sa_run" 240 run_finished demo-cache "$sa_run"
	qd stack log "$sa_run" | grep -q ' result succeeded' || fail "the run $sa_run did not succeed: $(qd stack log "$sa_run")"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the deploy through sudo'
	log "scenario: the interface deployed demo-cache through sudo, run $sa_run"

	step 'the Containers area restarts a single container after the password is typed in the terminal'
	docker exec "$host" docker rm -f "$sa_single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$sa_single" --label "$label" "$image" >/dev/null ||
		fail 'cannot start the single container for the restart'
	wait_for 'the single container' 30 health_is "$sa_single" healthy
	sa_started=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$sa_single")
	tui_send 5
	tui_wait "5 $tui_icon_containers Containers" 10
	tui_wait "$sa_single" 15
	tui_send Home
	tui_walk_down "$sa_single" tui_ct_cursor_on "$sa_single"
	tui_send Z
	tui_wait "Restart container $sa_single" 15
	tui_has 'sudo qhost container restart' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_wait_noecho 30
	tui_type "$sa_login"
	tui_send Enter
	tui_wait 'restart finished: 1 ok, 0 failed' 60
	tui_send Escape
	[ "$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$sa_single")" != "$sa_started" ] ||
		fail "the restart through sudo did not restart $sa_single"
	[ "$(docker exec "$host" docker inspect -f '{{.State.Status}}' "$sa_single")" = running ] || fail "$sa_single does not run after the restart"
	docker exec "$host" docker rm -f "$sa_single" >/dev/null 2>&1
	log "scenario: the interface restarted $sa_single through sudo"

	step 'Settings changes the maintenance window after the password is typed in the terminal'
	sa_start=$(date -u -d '+11 hours' +%H:%M)
	[ "$sa_start" != "$(window_field start)" ] || sa_start=$(date -u -d '+10 hours' +%H:%M)
	tui_subtab 'Maintenance window'
	tui_wait 'e host window' 10
	tui_send e
	tui_wait 'Change the host window' 10
	tui_clear
	tui_type daily
	tui_send Tab
	tui_clear
	tui_type "$sa_start"
	tui_send Tab
	tui_clear
	tui_type 1m
	tui_send Tab
	tui_clear
	tui_type UTC
	tui_send F2
	tui_wait 'Change the maintenance window of the host?' 10
	tui_has 'sudo qhost window set' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_wait_noecho 30
	tui_type "$sa_login"
	tui_send Enter
	tui_wait 'The maintenance window is changed' 30
	tui_send Enter
	[ "$(window_field start)" = "$sa_start" ] || fail "the window was not set through sudo: $(qd --output json window show)"
	tui_send Escape
	log "scenario: the interface set the maintenance window to $sa_start through sudo"

	step 'a Ctrl-C at the password prompt of a change in the System area changes nothing and says the run was interrupted'
	sa_group=e2e-sudogroup
	docker exec "$host" sh -c "getent group $sa_group >/dev/null && groupdel $sa_group; true"
	sa_entries=$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal 2>/dev/null | wc -l')
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Groups
	tui_wait 'MEMBERS' 10
	tui_send n
	tui_wait 'The system gives the group a regular group number.' 10
	tui_type "$sa_group"
	tui_send F2
	tui_wait "Create the group $sa_group." 10
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_send C-c
	tui_wait 'interrupted' 30
	tui_screen | tr -s ' \n' '  ' | grep -q 'Nothing was changed' || fail "the interface does not say that nothing was changed: $(tui_screen)"
	tui_has 'no longer holds' && fail "the interface takes the Ctrl-C for a password sudo no longer holds: $(tui_screen)"
	tui_has 'qhost-exit' && fail "the interface ended with the Ctrl-C at the password prompt: $(tui_screen)"
	[ "$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal 2>/dev/null | wc -l')" = "$sa_entries" ] ||
		fail 'the aborted change wrote a journal entry'
	if docker exec "$host" getent group "$sa_group" >/dev/null; then fail "the aborted change created the group $sa_group"; fi
	tui_send Enter
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	log 'scenario: a Ctrl-C at the password prompt of a change in the System area changed nothing and the interface said so'

	step 'the password shows on no captured screen; the rule of the administrator is put back'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$sa_login" "$tui_log"; then fail 'the password typed for sudo shows on a captured screen'; fi
	docker exec "$host" sh -ec 'rm -f /etc/sudoers.d/e2e-ask && mv /etc/sudoers.d/e2e-admin.off /etc/sudoers.d/e2e-admin && visudo -c >/dev/null' ||
		fail 'cannot put the sudo rule of the administrator back'
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1 || fail "sudo of $admin asks for a password again after the rule is put back"
	window_far_away >/dev/null || fail 'cannot move the maintenance window out of the way again'
}

# scenario_tui_deploy_sudo proves the deploy actions of the interface started without sudo: deciding proposals and
# creating a missing data directory. Like scenario_tui_sudo, the rule of $admin is put aside and sudo asks for the
# password at every command (timestamp_timeout=0), which is typed in the terminal. An administrator without sudo
# proposes a new maintenance window; the interface approves it through `sudo qhost proposal approve` and the window
# is read back. A second proposal is rejected with a reason through `sudo qhost proposal reject` and stands as
# rejected. The stack demo-dirs runs as 1000:1000 with bind mounts below data/ that do not exist, so its first run
# waits with the finding data-dir-missing; the stack detail creates the directories through `sudo qhost stack
# datadir` with an owner and a changed mode, and the owner, the mode and the run that follows are read. The password
# shows on no captured screen. The stack is removed again, the rule of the administrator is put back and the
# password is locked again.
scenario_tui_deploy_sudo() {
	tui_log=$work/tui-deploy-sudo-screens.log
	: >"$tui_log"
	ds_login=E2e-admin-Login-8851
	ds_stack=demo-dirs
	ds_dir=/opt/stacks/$ds_stack

	step 'the stack demo-dirs waits for two data directories that do not exist'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir demo-dirs'
	cat <<EOF2 | as_admin 'cat >"$HOME"/stacks/demo-dirs/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:1.0.0
    user: "1000:1000"
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-dirs
    volumes:
      - ./data/db:/data
      - ./data/files:/files
EOF2
	as_admin 'cd ~/stacks && cp demo-stack/.gitignore demo-dirs/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the demo-dirs stack" && git push -q origin main'
	restart_service
	wait_for 'the finding data-dir-missing of demo-dirs' 240 has_finding "$ds_stack" data-dir-missing
	docker exec "$host" test ! -e "$ds_dir/data" || fail 'the data directory of demo-dirs exists before it was created'
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=$ds_stack)" ] ||
		fail 'containers of demo-dirs exist although its data directories are missing'

	step 'sudo asks for the password at every command'
	printf '%s:%s\n' "$admin" "$ds_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	docker exec "$host" sh -ec "mv /etc/sudoers.d/e2e-admin /etc/sudoers.d/e2e-admin.off &&
		printf 'Defaults:%s timestamp_timeout=0\n' '$admin' >/etc/sudoers.d/e2e-ask && chmod 0440 /etc/sudoers.d/e2e-ask && visudo -c >/dev/null" ||
		fail 'cannot put the sudo rule of the administrator aside'
	if docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1; then fail "sudo runs a command of $admin without a password although the rule is put aside"; fi

	step 'the interface without sudo approves a window proposal through sudo qhost proposal approve'
	ds_start=$(date -u -d '+10 hours' +%H:%M)
	[ "$ds_start" != "$(window_field start)" ] || ds_start=$(date -u -d '+9 hours' +%H:%M)
	ds_id=$(prop_submit_kind window "{\"days\":[\"daily\"],\"start\":\"$ds_start\",\"duration\":\"2m\",\"zone\":\"UTC\"}")
	[ -n "$ds_id" ] || fail 'the window proposal has no id'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 6
	tui_wait 'maintenance window' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait 'The maintenance window of the host would change:' 10
	tui_wait "after:  \"$ds_start\"" 10
	tui_send A
	tui_wait 'and set this maintenance window?' 30
	# The command stands at the end of the sheet, below the window.
	for _ in 1 2 3 4 5 6 7 8 9 10; do tui_send Down; done
	tui_wait 'sudo qhost proposal approve' 10
	tui_has '--fingerprint' || fail "the question does not show the fingerprint: $(tui_screen)"
	tui_has '--confirm' || fail "the question does not show --confirm: $(tui_screen)"
	tui_excerpt 'sudo qhost proposal approve' 6
	[ "$(prop_state "$ds_id")" = open ] || fail 'the window proposal left the state open before yes'
	[ "$(window_field start)" != "$ds_start" ] || fail 'the window changed before yes'
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ds_login"
	tui_send Enter
	tui_wait 'The maintenance window is changed.' 60
	tui_send Enter
	wait_for 'the window proposal to be applied' 30 prop_is "$ds_id" applied
	tui_tmux kill-server >/dev/null 2>&1 || true
	[ "$(window_field start)" = "$ds_start" ] && [ "$(window_field duration)" = 2m ] && [ "$(window_field zone)" = UTC ] ||
		fail "the window does not hold the proposal: $(qd --output json window show)"
	log "scenario: the interface without sudo approved $ds_id through sudo; the window starts at $ds_start"

	step 'the interface without sudo rejects a proposal with a reason through sudo qhost proposal reject'
	ds_start2=$(date -u -d '+8 hours' +%H:%M)
	[ "$ds_start2" != "$(window_field start)" ] || ds_start2=$(date -u -d '+7 hours' +%H:%M)
	ds_id2=$(prop_submit_kind window "{\"days\":[\"daily\"],\"start\":\"$ds_start2\",\"duration\":\"3m\",\"zone\":\"UTC\"}")
	[ -n "$ds_id2" ] || fail 'the second window proposal has no id'
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 6
	tui_wait 'maintenance window' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_send N
	tui_wait 'Reject a proposal' 10
	tui_type 'not this host'
	tui_send F2
	tui_wait 'Reject proposal' 10
	tui_has 'sudo qhost proposal reject' || fail "the question does not show the command: $(tui_screen)"
	tui_has 'not this host' || fail "the question does not show the reason: $(tui_screen)"
	tui_excerpt 'Reject proposal' 12
	[ "$(prop_state "$ds_id2")" = open ] || fail 'the second proposal left the state open before yes'
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ds_login"
	tui_send Enter
	tui_wait 'is rejected.' 60
	tui_send Enter
	wait_for 'the second proposal to be rejected' 30 prop_is "$ds_id2" rejected
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin "qhost proposal show $ds_id2 2>&1") || fail "qhost proposal show failed: $out"
	printf '%s' "$out" | grep -qF 'not this host' || fail "qhost proposal show does not hold the reason: $out"
	[ "$(window_field start)" = "$ds_start" ] || fail "the rejection changed the window: $(qd --output json window show)"
	log "scenario: the interface without sudo rejected $ds_id2 through sudo"

	step 'the stack detail without sudo creates the missing data directories through sudo qhost stack datadir'
	ds_prev=$(newest_run "$ds_stack")
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait "[ ] $ds_stack" 30
	tui_send Home
	tui_cursor_to "$ds_stack"
	tui_send Enter
	tui_wait 'Missing data directories' 15
	tui_wait 'owner 1000:1000' 10
	tui_send +
	tui_wait 'Create data directories' 10
	tui_wait '1000:1000' 10
	tui_send Tab
	tui_clear
	tui_type 0700
	tui_send F2
	tui_wait 'Create the missing data directories of the' 10
	tui_has 'sudo qhost stack datadir' || fail "the question does not show the command: $(tui_screen)"
	tui_has '--owner 1000:1000' || fail "the question does not show the owner: $(tui_screen)"
	tui_has '--mode 0700' || fail "the question does not show the changed mode: $(tui_screen)"
	tui_excerpt 'Create the missing data directories' 16
	docker exec "$host" test ! -e "$ds_dir/data" || fail 'the directories exist before yes'
	[ "$(newest_run "$ds_stack")" = "$ds_prev" ] || fail 'a run of demo-dirs started before yes'
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ds_login"
	tui_send Enter
	tui_wait "Data directories created for the stack $ds_stack" 90
	tui_send Enter
	tui_tmux kill-server >/dev/null 2>&1 || true
	for d in db files; do
		[ "$(docker exec "$host" stat -c '%u:%g %a' "$ds_dir/data/$d")" = '1000:1000 700' ] ||
			fail "data/$d has owner and mode '$(docker exec "$host" stat -c '%u:%g %a' "$ds_dir/data/$d")', want '1000:1000 700'"
	done
	ds_run=$(next_run "$ds_stack" "$ds_prev") || fail 'no run of demo-dirs started after the creation'
	wait_for "the run $ds_run" 240 run_finished "$ds_stack" "$ds_run"
	qd stack log "$ds_run" | grep -q ' result succeeded' || fail "the run $ds_run did not succeed: $(qd stack log "$ds_run")"
	no_finding "$ds_stack" data-dir-missing || fail "demo-dirs still shows the finding: $(qd --output json status)"
	[ -n "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=$ds_stack --filter status=running)" ] ||
		fail 'demo-dirs does not run after the creation'
	log "scenario: the interface without sudo created the data directories of $ds_stack through sudo, run $ds_run"

	step 'the password shows on no captured screen; the rule of the administrator is put back'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$ds_login" "$tui_log"; then fail 'the password typed for sudo shows on a captured screen'; fi
	docker exec "$host" sh -ec 'rm -f /etc/sudoers.d/e2e-ask && mv /etc/sudoers.d/e2e-admin.off /etc/sudoers.d/e2e-admin && visudo -c >/dev/null' ||
		fail 'cannot put the sudo rule of the administrator back'
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1 || fail "sudo of $admin asks for a password again after the rule is put back"
	window_far_away >/dev/null || fail 'cannot move the maintenance window out of the way again'

	step 'demo-dirs leaves the repository'
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q demo-dirs &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the demo-dirs stack" && git push -q origin main'
	restart_service
	docker exec "$host" sh -c "docker ps -aq --filter label=com.docker.compose.project=$ds_stack | xargs -r docker rm -f" >/dev/null ||
		fail "cannot remove the containers of $ds_stack"
	docker exec "$host" rm -rf "$ds_dir"
	wait_for 'the deploy service to answer' 30 qd status
	log 'scenario: demo-dirs is removed'
}

# scenario_datadir_nested proves a nested data directory in the system engine: the stack demo-nested mounts
# ./data/db for the user 1000:1000, so qhost stack datadir has to create the parent data as well. The parent keeps
# the owner of the stack folder and stays searchable for the deploy service, the directory itself gets 1000:1000, and
# the run that follows finds the directory and deploys the stack. The stack is removed again.
scenario_datadir_nested() {
	dn_stack=demo-nested
	dn_dir=/opt/stacks/$dn_stack

	step 'the stack demo-nested mounts the nested directory data/db that does not exist'
	as_admin 'cd ~/stacks && git pull -q --ff-only && mkdir demo-nested'
	cat <<EOF2 | as_admin 'cat >"$HOME"/stacks/demo-nested/compose.yaml'
services:
  app:
    image: registry.$domain/demo/app:1.0.0
    user: "1000:1000"
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-nested
    volumes:
      - ./data/db:/data
EOF2
	as_admin 'cd ~/stacks && cp demo-stack/.gitignore demo-nested/.gitignore && git add -A &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Add the demo-nested stack" && git push -q origin main'
	restart_service
	wait_for 'the finding data-dir-missing of demo-nested' 240 has_finding "$dn_stack" data-dir-missing
	docker exec "$host" test ! -e "$dn_dir/data" || fail 'the data directory of demo-nested exists before it was created'

	step 'qhost stack datadir creates the nested directory and the stack finds it'
	dn_prev=$(newest_run "$dn_stack")
	out=$(qd stack datadir "$dn_stack" --yes) || fail "datadir of demo-nested failed: $out"
	[ "$(docker exec "$host" stat -c %u:%g "$dn_dir/data/db")" = 1000:1000 ] ||
		fail "data/db has the owner $(docker exec "$host" stat -c %u:%g "$dn_dir/data/db"), want 1000:1000"
	[ "$(docker exec "$host" stat -c %u:%g "$dn_dir/data")" = "$(docker exec "$host" stat -c %u:%g "$dn_dir")" ] ||
		fail "the parent data has the owner $(docker exec "$host" stat -c %u:%g "$dn_dir/data"), not the one of the stack folder"
	dn_mode=$(docker exec "$host" stat -c %a "$dn_dir/data")
	[ $((0$dn_mode & 0110)) -eq $((0110)) ] || fail "the parent data is not searchable for its owner and group: $dn_mode"
	dn_run=$(next_run "$dn_stack" "$dn_prev") || fail 'no run of demo-nested started after the creation'
	wait_for "the run $dn_run" 240 run_finished "$dn_stack" "$dn_run"
	qd stack log "$dn_run" | grep -q ' result succeeded' || fail "the run $dn_run did not succeed: $(qd stack log "$dn_run")"
	no_finding "$dn_stack" data-dir-missing || fail "demo-nested still shows the finding: $(qd --output json status)"
	[ -n "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=$dn_stack --filter status=running)" ] ||
		fail 'demo-nested does not run after the creation'
	log "scenario: the nested directory data/db of $dn_stack was created, the parent stays searchable, run $dn_run"

	step 'demo-nested leaves the repository'
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q demo-nested &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the demo-nested stack" && git push -q origin main'
	restart_service
	docker exec "$host" sh -c "docker ps -aq --filter label=com.docker.compose.project=$dn_stack | xargs -r docker rm -f" >/dev/null ||
		fail "cannot remove the containers of $dn_stack"
	docker exec "$host" rm -rf "$dn_dir"
	wait_for 'the deploy service to answer' 30 qd status
	log 'scenario: demo-nested is removed'
}

# ts_watch_start starts two watchers on the host. The first records every process whose command line holds a line of
# the pattern file /tmp/qd-ts-pattern in /tmp/qd-ts-hits, until /tmp/qd-ts-stop appears or a minute passed. grep
# reads the pattern from the file, so the watcher itself never has the value in a command line. The second watches
# the state folders of qhost until /tmp/qd-ts-stop appears and records in /tmp/qd-ts-fs-hits the path of every file
# outside the credential store that held a value at any moment, even one deleted right after the write: it links
# each new or changed file to a private folder of root on the same file system at the first event, so the content
# survives the delete, and reads that link. Its state stands in /tmp/qd-ts-fs-status (ready, then done). inotify is
# used directly because inotifywait hands the event over too late for a file that lives a fraction of a
# millisecond. A folder created while the watcher runs is swept right after its watch is added, so a file that is
# written and deleted in the first instants of a new folder can still escape it.
ts_watch_start() {
	docker exec -i "$host" sh -c 'cat >/tmp/qd-ts-watch.sh' <<'EOF' || fail 'cannot write the watcher of the command lines'
end=$(( $(date +%s) + 60 ))
while [ "$(date +%s)" -lt "$end" ] && [ ! -e /tmp/qd-ts-stop ]; do
	for f in /proc/[0-9]*/cmdline; do tr "\000" " " <"$f" 2>/dev/null; echo; done | grep -F -f /tmp/qd-ts-pattern >>/tmp/qd-ts-hits
	sleep 0.02
done
EOF
	docker exec -i "$host" sh -c 'cat >/tmp/qd-ts-fswatch.pl' <<'EOF' || fail 'cannot write the watcher of the state folders'
use strict;
use warnings;
my @watched = ('/var/lib/qhost-deploy', '/var/lib/qhost', '/var/lib/qhost-host');
my $allowed = '/var/lib/qhost-deploy/credentials';
my $keep = '/var/lib/qd-ts-keep';
my $status = '/tmp/qd-ts-fs-status';
my $hits = '/tmp/qd-ts-fs-hits';
sub setstatus { open(my $s, '>', $status) or exit 1; print $s "$_[0]\n"; close $s; }
sub under { my ($p, $d) = @_; return $p eq $d || index($p, "$d/") == 0; }
sub fatal { setstatus('failed'); exit 1; }

open(my $pf, '<', '/tmp/qd-ts-pattern') or fatal();
my @pat = grep { length } map { chomp; $_ } <$pf>;
close $pf;
my $maxlen = 0;
for (@pat) { $maxlen = length if length > $maxlen; }

mkdir($keep, 0700) or fatal();
chmod(0700, $keep);
my ($sys_init, $sys_watch) = (`uname -m` =~ /aarch64|arm64/) ? (26, 27) : (294, 254);
my $ifd = syscall($sys_init, 0);
fatal() if $ifd < 0;
open(my $in, '<&=', $ifd) or fatal();
my %dirs;

my (%held, %paths, %hit, $n);
sub note { return if $hit{$_[0]}++; open(my $h, '>>', $hits) or return; print $h "$_[0]\n"; close $h; }
sub check {
	my ($ino) = @_;
	open(my $fh, '<', "$keep/$held{$ino}") or return;
	my ($carry, $buf, $found) = ('', '', 0);
	while (!$found && sysread($fh, $buf, 1 << 20)) {
		my $data = $carry . $buf;
		for my $p (@pat) { if (index($data, $p) >= 0) { $found = 1; last; } }
		$carry = $maxlen > 1 ? substr($data, -($maxlen - 1)) : '';
	}
	close $fh;
	note($_) for $found ? keys %{ $paths{$ino} } : ();
}
# hold links the file to a private name before anything else, so that a delete right after the write cannot take
# the content away.
sub hold {
	my ($path, $verify, $fresh) = @_;
	return if under($path, $allowed);
	my $ino = $fresh ? undef : (stat($path))[1];
	if (!defined $ino || !defined $held{$ino}) {
		my $name = ++$n;
		if (!link($path, "$keep/$name")) { note("$path (could not be held)") if -e $path; return; }
		$ino = (stat("$keep/$name"))[1];
		if (defined $held{$ino}) { unlink("$keep/$name"); } else { $held{$ino} = $name; }
	}
	$paths{$ino}{$path} = 1;
	check($ino) if $verify;
}
sub sweep {
	my ($dir) = @_;
	return if under($dir, $allowed);
	opendir(my $dh, $dir) or return;
	for my $e (grep { $_ ne '.' && $_ ne '..' } readdir $dh) {
		my $p = "$dir/$e";
		if (-l $p) { next; } elsif (-d $p) { sweep($p); } else { hold($p, 1); }
	}
	closedir $dh;
}
# IN_MODIFY | IN_CLOSE_WRITE | IN_MOVED_TO | IN_CREATE
sub watch {
	my ($dir) = @_;
	return if under($dir, $allowed);
	my $p = $dir;
	my $wd = syscall($sys_watch, $ifd, $p, 0x2 | 0x8 | 0x80 | 0x100);
	fatal() if $wd < 0;
	$dirs{$wd} = $dir;
	opendir(my $dh, $dir) or return;
	for my $e (grep { $_ ne '.' && $_ ne '..' } readdir $dh) {
		my $q = "$dir/$e";
		watch($q) if !-l $q && -d $q;
	}
	closedir $dh;
}
my $overflow = 0;
sub handle {
	my ($wd, $mask, $name) = @_;
	if ($wd < 0) { $overflow = 1 if $mask & 0x4000; return; }
	return unless defined $dirs{$wd} && length $name;
	my $path = "$dirs{$wd}/$name";
	if ($mask & 0x40000000) {
		if ($mask & (0x100 | 0x80)) { watch($path); sweep($path); }
		return;
	}
	hold($path, $mask & (0x8 | 0x80), $mask & (0x100 | 0x80));
}

my ($rbuf, $deadline) = ('', time + 1200);
sub pump {
	my ($wait) = @_;
	my $rin = '';
	vec($rin, $ifd, 1) = 1;
	return unless select($rin, undef, undef, $wait) > 0;
	my $got = sysread($in, my $chunk, 65536);
	fatal() unless $got;
	$rbuf .= $chunk;
	while (length($rbuf) >= 16) {
		my ($wd, $mask, undef, $len) = unpack('lLLL', $rbuf);
		last if length($rbuf) < 16 + $len;
		handle($wd, $mask, unpack('Z*', substr($rbuf, 16, $len)));
		substr($rbuf, 0, 16 + $len) = '';
	}
}
watch($_) for grep { -d } @watched;
setstatus('ready');
until (-e '/tmp/qd-ts-stop' || time > $deadline) { pump(0.05); }
my $early = !-e '/tmp/qd-ts-stop';
select(undef, undef, undef, 0.5);
pump(0) for 1 .. 20;
check($_) for keys %held;
setstatus($early ? 'early' : $overflow ? 'overflow' : 'done');
EOF
	printf '%s\n' "$@" | docker exec -i "$host" sh -c 'cat >/tmp/qd-ts-pattern; rm -rf /tmp/qd-ts-stop /tmp/qd-ts-fs-status /tmp/qd-ts-fs-hits /var/lib/qd-ts-keep; : >/tmp/qd-ts-hits; : >/tmp/qd-ts-fs-hits' ||
		fail 'cannot write the pattern of the watcher'
	docker exec -d "$host" sh /tmp/qd-ts-watch.sh || fail 'cannot start the watcher of the command lines'
	docker exec -d "$host" perl /tmp/qd-ts-fswatch.pl || fail 'cannot start the watcher of the state folders'
	i=0
	until [ "$(docker exec "$host" cat /tmp/qd-ts-fs-status 2>/dev/null)" = ready ]; do
		i=$((i + 1))
		[ "$i" -lt 100 ] || fail "the watcher of the state folders is not ready: $(docker exec "$host" cat /tmp/qd-ts-fs-status 2>&1)"
		sleep 0.1
	done
}

# ts_paste types $1 into the interface through a tmux buffer filled on standard input, so the value stands in no
# command line, unlike tui_type.
ts_paste() {
	printf '%s' "$1" | docker exec -i -u "$admin" "$host" tmux -L qdtui load-buffer - || fail 'cannot load a value into tmux'
	tui_tmux paste-buffer -p -t qdtui || fail 'cannot paste a value'
}

# ts_watch_stop ends both watchers, waits until the one of the state folders has read its last event and removes the
# files of both except /tmp/qd-ts-hits, /tmp/qd-ts-fs-hits and /tmp/qd-ts-fs-status, which the step reads and removes.
ts_watch_stop() {
	docker exec "$host" sh -c 'touch /tmp/qd-ts-stop' || fail 'cannot stop the watchers'
	i=0
	while [ "$(docker exec "$host" cat /tmp/qd-ts-fs-status 2>/dev/null)" = ready ]; do
		i=$((i + 1))
		[ "$i" -lt 300 ] || fail 'the watcher of the state folders does not end'
		sleep 0.1
	done
	sleep 1
	docker exec "$host" sh -c 'rm -rf /var/lib/qd-ts-keep /tmp/qd-ts-pattern /tmp/qd-ts-watch.sh /tmp/qd-ts-fswatch.pl /tmp/qd-ts-stop'
}

# scenario_tui_secrets proves the forms with a secret of the interface started without sudo: the stack detail sets a
# stack secret and Settings stores a registry login. Each asks with the exact command without the value, hands the
# terminal to the real sudo for the password (sudo -v) and runs the command without a prompt (sudo -n) with the value
# on standard input. The scenario sets it up like scenario_tui_system and unlike scenario_tui_sudo: the rule of
# $admin stays, so sudo -v asks (the account is also in the group sudo) and sudo -n runs after it; the timestamp of
# sudo is removed before each change so that sudo asks again. The password is set for the scenario and locked again
# at the end. The values must show in no command line, in no captured screen, in no log of the host and no state of
# the host service; the secret stands in secrets.env and the login is listed. Both are removed afterwards.
scenario_tui_secrets() {
	tui_log=$work/tui-secrets-screens.log
	: >"$tui_log"
	ts_login=E2e-admin-Login-5581
	ts_key=E2E_TUI_SECRET
	ts_secret=tui-secret-e2e-7731
	ts_reg_host=e2e-secret.example.test
	ts_reg_user=e2e-secret-user
	ts_reg_token=tui-regtoken-e2e-5528
	ts_env=/opt/stacks/demo-stack/secrets.env
	printf '%s:%s\n' "$admin" "$ts_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	docker exec "$host" test -e "$ts_env" && ts_had_env=yes || ts_had_env=no
	wait_for 'demo-stack idle' 120 stack_idle demo-stack
	ts_watch_start "$ts_secret" "$ts_reg_token"
	# The watcher must see a value in a command line at all.
	docker exec -d "$host" sh -c 'sleep 3' "$ts_secret" || fail 'cannot start the control process of the watcher'
	i=0
	until [ -n "$(docker exec "$host" cat /tmp/qd-ts-hits)" ]; do
		i=$((i + 1))
		[ "$i" -lt 100 ] || fail 'the watcher does not see the value in the command line of a control process'
		sleep 0.1
	done
	# The control process lives three seconds; the hits are cleared only after it ended.
	sleep 4
	docker exec "$host" sh -c ': >/tmp/qd-ts-hits'

	step 'the stack detail sets a secret without sudo: the question shows the command without the value, sudo -v asks, sudo -n runs it'
	docker exec -u "$admin" "$host" sudo -K || fail "cannot remove the sudo timestamp of $admin"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'trap : INT; qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '[ ] demo-stack' 30
	tui_send Home
	tui_cursor_to demo-stack
	tui_send Enter
	tui_wait 'Secrets' 15
	tui_send n
	tui_wait 'Set a secret' 10
	tui_type "$ts_key"
	tui_send Tab
	ts_paste "$ts_secret"
	sleep 0.5
	tui_send F2
	tui_wait "Set the secret $ts_key of the stack demo-stack?" 10
	tui_has 'sudo qhost stack secret set' || fail "the question does not show the command: $(tui_screen)"
	tui_has "$ts_secret" && fail "the question shows the value: $(tui_screen)"
	tui_excerpt "Set the secret $ts_key" 12
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ts_login"
	tui_send Enter
	tui_wait "set for the stack demo-stack" 60
	tui_send Enter
	# The expected line goes on standard input: on a command line the watcher would catch the value.
	printf '%s\n' "$ts_key='$ts_secret'" | docker exec -i "$host" grep -qxF -f - "$ts_env" || fail "secrets.env does not hold the secret: $(docker exec "$host" cat "$ts_env")"
	[ "$(docker exec "$host" stat -c %U:%a "$ts_env")" = qhost-deploy:600 ] || fail 'secrets.env has the wrong owner or mode'
	qd stack secret list demo-stack | grep -q "$ts_key" || fail "the key is not listed: $(qd stack secret list demo-stack)"
	log "scenario: the interface set $ts_key of demo-stack through sudo"

	step 'Settings stores a registry login without sudo the same way and qhost credential list shows it'
	docker exec -u "$admin" "$host" sudo -K || fail "cannot remove the sudo timestamp of $admin"
	tui_send Escape
	tui_wait 'CURRENT' 10
	tui_subtab Credentials
	tui_wait 'd remove' 10
	tui_send n
	tui_wait 'Set a credential' 10
	tui_send Tab
	tui_type "$ts_reg_host"
	tui_send Tab
	tui_type "$ts_reg_user"
	tui_send Tab
	ts_paste "$ts_reg_token"
	sleep 0.5
	tui_send F2
	tui_wait "Store the credential registry.$ts_reg_host?" 10
	tui_has 'sudo qhost credential set' || fail "the question does not show the command: $(tui_screen)"
	tui_has "$ts_reg_token" && fail "the question shows the token: $(tui_screen)"
	tui_excerpt 'Store the credential' 12
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ts_login"
	tui_send Enter
	tui_wait 'is stored' 60
	tui_send Enter
	tui_wait "registry.$ts_reg_host" 10
	qd credential list | grep -q "registry.$ts_reg_host" || fail "qhost credential list does not show the login: $(qd credential list)"
	[ "$(docker exec "$host" cat "/var/lib/qhost-deploy/credentials/registry.$ts_reg_host")" = "$ts_reg_user:$ts_reg_token" ] ||
		fail 'the stored login is not <user>:<token>'
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	log "scenario: the interface stored registry.$ts_reg_host through sudo"

	step 'the values show in no command line, no captured screen, no log of the host and no state of qhost'
	ts_watch_stop
	[ -z "$(docker exec "$host" cat /tmp/qd-ts-hits)" ] || fail "a value was in a command line: $(docker exec "$host" cat /tmp/qd-ts-hits | sed "s/$ts_secret/<value>/; s/$ts_reg_token/<token>/")"
	docker exec "$host" rm -f /tmp/qd-ts-hits
	[ "$(docker exec "$host" cat /tmp/qd-ts-fs-status)" = done ] ||
		fail "the watcher of the state folders did not cover the whole window: $(docker exec "$host" cat /tmp/qd-ts-fs-status 2>&1)"
	[ -z "$(docker exec "$host" cat /tmp/qd-ts-fs-hits)" ] || fail "a value was written to a file outside the credential store: $(docker exec "$host" cat /tmp/qd-ts-fs-hits | sed "s/$ts_secret/<value>/g; s/$ts_reg_token/<token>/g")"
	docker exec "$host" rm -f /tmp/qd-ts-fs-hits /tmp/qd-ts-fs-status
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	for v in "$ts_secret" "$ts_reg_token" "$ts_login"; do
		if grep -qF -- "$v" "$tui_log"; then fail 'a value or the password shows on a captured screen'; fi
		if docker exec "$host" sh -c 'journalctl --no-pager -q 2>/dev/null' | grep -qF -- "$v"; then fail 'a value or the password reached the journal of the host, including that of sudo'; fi
		if docker exec "$host" grep -rqF -- "$v" /var/log /var/lib/qhost-host /var/lib/qhost; then fail 'a value or the password reached a log or the state of qhost'; fi
		if qd service log --tail 0 | grep -qF -- "$v"; then fail 'a value or the password reached the service log'; fi
	done
	holders=$(docker exec "$host" grep -rlF -- "$ts_reg_token" /var/lib/qhost-deploy 2>/dev/null || true)
	[ "$holders" = "/var/lib/qhost-deploy/credentials/registry.$ts_reg_host" ] || fail "the token stands in: $holders"
	holders=$(docker exec "$host" grep -rlF -- "$ts_secret" /var/lib/qhost-deploy /opt/stacks 2>/dev/null || true)
	[ "$holders" = "$ts_env" ] || fail "the value of the stack secret stands in: $holders"
	log 'scenario: no value in a command line, a captured screen, the system journal, a log or the state; the token only in the credential store, the secret only in secrets.env'

	step 'the secret, the login, the password and the sudo timestamp are put back'
	out=$(qd stack secret delete demo-stack "$ts_key" --yes) || fail "secret delete failed: $out"
	[ "$ts_had_env" = yes ] || docker exec "$host" rm -f "$ts_env"
	out=$(qd credential delete "registry.$ts_reg_host" --yes) || fail "credential delete failed: $out"
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	docker exec -u "$admin" "$host" sudo -K || fail "cannot remove the sudo timestamp of $admin"
	docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1 || fail "sudo of $admin asks for a password after the scenario"
}

# scenario_tui_stack_files proves the file browser over a stack folder in the interface started without sudo, with
# a script as the editor set through VISUAL, which qhost does not know. Like scenario_tui_file_edit the rule of
# $admin stays and timestamp_timeout=0 makes every sudo -v ask for the password, which is typed in the terminal. In
# demo-stack, whose container reads DEMO_SECRET, secrets.env is only root's: the preview asks to read it with sudo and
# shows the value masked; the edit writes a new value through sudo, the question after it offers to deploy now, and
# the deploy through sudo gives the container the new value. Then .env is edited: owner, group and mode stay, the
# browser shows the line about uncommitted changes and the row of the folder in Stacks shows the note. The change is
# discarded again, secrets.env and the container get their old value back, the rule and the locked password are put
# back, and neither password nor a secret shows on a captured screen.
scenario_tui_stack_files() {
	tui_log=$work/tui-stack-files-screens.log
	: >"$tui_log"
	sf_login=E2e-admin-Login-4417
	sf_dir=/opt/stacks/demo-stack
	sf_secrets=$sf_dir/secrets.env
	sf_env=$sf_dir/.env
	sf_editor=/home/$admin/e2e-fake-stack-editor.sh
	sf_new=tui-stack-secret-e2e-3392
	sf_mark=E2E_STACK_FILES=5527
	docker exec "$host" test -f "$sf_secrets" || fail "$sf_secrets does not exist"
	sf_old_line=$(docker exec "$host" cat "$sf_secrets")
	sf_old=${sf_old_line#DEMO_SECRET=}
	[ -n "$sf_old" ] && [ "$sf_old" != "$sf_old_line" ] || fail "$sf_secrets does not hold a single DEMO_SECRET line"
	[ "$(docker exec "$host" stat -c %U:%a "$sf_secrets")" = qhost-deploy:600 ] || fail "$sf_secrets has the wrong owner or mode"
	# The copy of the file for the editor lies in memory only when /dev/shm is tmpfs.
	[ "$(docker exec "$host" stat -f -c %T /dev/shm)" = tmpfs ] || fail '/dev/shm of the host is no tmpfs, so a secret is not edited'
	sf_dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $sf_dirty"
	[ -z "$sf_dirty" ] || fail "the stack folder has uncommitted changes before the scenario: $sf_dirty"
	sf_stat=$(docker exec "$host" stat -c %U:%G:%a "$sf_env")
	printf '%s\n' '#!/bin/sh' "printf '%s\\n' \"\$(cat '$sf_editor.text')\" >\"\$1\"" | as_admin "cat >'$sf_editor' && chmod 0755 '$sf_editor'" ||
		fail 'cannot make the script that stands in for the editor'
	printf '%s\n' "DEMO_SECRET=$sf_new" | as_admin "cat >'$sf_editor.text'" || fail 'cannot make the text of the editor'
	printf '%s:%s\n' "$admin" "$sf_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	docker exec "$host" sh -ec "printf 'Defaults:%s timestamp_timeout=0\n' '$admin' >/etc/sudoers.d/e2e-ask && chmod 0440 /etc/sudoers.d/e2e-ask &&
		visudo -c >/dev/null" || fail 'cannot make sudo ask for the password each time'

	step 'the preview of secrets.env asks to read it with sudo and masks the value'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 VISUAL=$sf_editor sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '[ ] demo-stack' 30
	tui_folder_to /opt/stacks
	tui_send Q
	tui_wait 'demo-stack/' 15
	tui_browse_to demo-stack
	tui_wait 'compose.yaml' 10
	tui_browse_mark secrets.env
	tui_send Enter
	tui_wait 'Only root may read' 15
	# A read runs as sudo qhost host file, which the rule of $admin lets through without a password.
	tui_send y
	tui_wait '••••••' 30
	tui_has "$sf_old" && fail "the preview shows the value of secrets.env: $(tui_screen)"
	tui_excerpt 'DEMO_SECRET' 4
	tui_send Escape

	step 'the editor changes secrets.env through sudo; the question afterwards offers to deploy, which gives the container the new value'
	tui_send e
	tui_wait 'Only root may read' 15
	tui_send y
	tui_wait 'Unknown editor' 30
	tui_send y
	tui_wait 'Write the edited file' 30
	tui_has "file-write $sf_secrets" || fail "the question does not show the command: $(tui_screen)"
	tui_has "$sf_new" && fail "the question shows the new value: $(tui_screen)"
	tui_send y
	tui_sudo_password "$sf_login"
	tui_wait 'Done: Write the edited file' 60
	tui_send Enter
	tui_wait 'secrets.env saved' 15
	tui_has 'deploy now' || fail "the question does not offer to deploy now: $(tui_screen)"
	[ "$(docker exec "$host" cat "$sf_secrets")" = "DEMO_SECRET=$sf_new" ] || fail "$sf_secrets does not hold the new value"
	[ "$(docker exec "$host" stat -c %U:%a "$sf_secrets")" = qhost-deploy:600 ] || fail "$sf_secrets has the wrong owner or mode after the write"
	sf_prev=$(newest_run demo-stack)
	tui_send y
	tui_wait 'Deploy demo-stack?' 15
	tui_has 'sudo qhost stack deploy' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait 'deploy finished: 1 ok, 0 failed' 240
	tui_send Escape
	sf_run=$(next_run demo-stack "$sf_prev") || fail 'the deploy through sudo started no run of demo-stack'
	wait_for "the run $sf_run" 240 run_finished demo-stack "$sf_run"
	qd stack log "$sf_run" | grep -q ' result succeeded' || fail "the run $sf_run did not succeed: $(qd stack log "$sf_run")"
	docker exec "$host" docker inspect -f '{{.Config.Env}}' "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)" |
		grep -q "DEMO_SECRET=$sf_new" || fail 'the container did not get the new value of secrets.env'
	tui_tmux kill-server >/dev/null 2>&1 || true
	log "scenario: the interface changed secrets.env of demo-stack through sudo and deployed it, run $sf_run"

	step 'the editor changes .env: owner, group and mode stay, the browser and the row of the folder name the uncommitted change'
	printf '%s\n' "DEMO_APP_VERSION=1.0.0" "DEMO_HEALTH=ok" "$sf_mark" | as_admin "cat >'$sf_editor.text'" || fail 'cannot make the text of the editor'
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 VISUAL=$sf_editor sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '[ ] demo-stack' 30
	tui_folder_to /opt/stacks
	tui_send Q
	tui_wait 'demo-stack/' 15
	tui_browse_to demo-stack
	tui_wait 'compose.yaml' 10
	tui_browse_mark .env
	tui_send e
	tui_wait 'Unknown editor' 15
	tui_send y
	tui_wait 'Write the edited file' 30
	tui_has "file-write $sf_env" || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_sudo_password "$sf_login"
	tui_wait 'Done: Write the edited file' 60
	tui_send Enter
	docker exec "$host" grep -qxF "$sf_mark" "$sf_env" || fail "$sf_env does not hold the new text: $(docker exec "$host" cat "$sf_env")"
	[ "$(docker exec "$host" stat -c %U:%G:%a "$sf_env")" = "$sf_stat" ] ||
		fail "$sf_env is $(docker exec "$host" stat -c %U:%G:%a "$sf_env") after the write, want $sf_stat"
	tui_wait 'This folder has uncommitted changes' 30
	i=0
	until tui_screen | grep -q '/opt/stacks (.*uncommitted'; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the row of the folder shows no note about uncommitted changes: $(tui_screen)"
		tui_send Escape
		sleep 0.4
	done
	tui_has 'updates leave it out until they are committed' || fail "the panel of the folder does not say that updates leave it out: $(tui_screen)"
	tui_excerpt '/opt/stacks (' 1
	tui_tmux kill-server >/dev/null 2>&1 || true
	log 'scenario: the interface changed .env of demo-stack and the folder row shows the uncommitted change'

	step 'the change is discarded, secrets.env and the container get the old value back, the rule of the administrator is put back'
	docker exec -u qhost-deploy "$host" git -C /opt/stacks checkout -- demo-stack/.env || fail "cannot discard the change of $sf_env"
	sf_dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $sf_dirty"
	[ -z "$sf_dirty" ] || fail "the stack folder is not clean after the discard: $sf_dirty"
	printf '%s\n' "$sf_old_line" | docker exec -i -u qhost-deploy "$host" sh -ec "umask 077 && cat >'$sf_secrets'" || fail "cannot restore $sf_secrets"
	out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
	sf_run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	[ -n "$sf_run" ] || fail "deploy did not report a run: $out"
	wait_for "the run $sf_run" 240 run_finished demo-stack "$sf_run"
	qd stack log "$sf_run" | grep -q ' result succeeded' || fail "the run $sf_run did not succeed: $(qd stack log "$sf_run")"
	as_admin "rm -f '$sf_editor' '$sf_editor.text'"
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	for v in "$sf_login" "$sf_new" "$sf_old"; do
		if grep -qF -- "$v" "$tui_log"; then fail 'the password or a secret shows on a captured screen'; fi
	done
	docker exec "$host" sh -ec 'rm -f /etc/sudoers.d/e2e-ask && visudo -c >/dev/null' || fail 'cannot remove the rule that makes sudo ask'
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	docker exec -u "$admin" "$host" sudo -K || fail "cannot remove the sudo timestamp of $admin"
	docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1 || fail "sudo of $admin asks for a password after the scenario"
}

# scenario_tui_stack_changes proves the view of the uncommitted changes of a stack folder in the interface started
# without sudo. compose.yaml and .env of demo-stack are changed on the host beforehand: scenario_tui_stack_files proves
# the edit through the browser, this one what follows it. The rule of $admin stays, so the commands through sudo ask
# for no password. = on the row of /opt/stacks lists both files, enter shows the diff of .env through sudo qhost source
# diff, > commits both through sudo qhost source commit, the remote gets the commit and its run deploys demo-stack.
# A further change of .env is discarded through sudo qhost source discard. The commit is reverted through the clone
# of the administrator at the end, so the files and the clone are as before and the later sections build on them.
scenario_tui_stack_changes() {
	tui_log=$work/tui-stack-changes-screens.log
	: >"$tui_log"
	sc_env=/opt/stacks/demo-stack/.env
	sc_compose=/opt/stacks/demo-stack/compose.yaml
	sc_mark=E2E_CHANGES=6181
	sc_more=E2E_DISCARD=6182
	sc_dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $sc_dirty"
	[ -z "$sc_dirty" ] || fail "the stack folder has uncommitted changes before the scenario: $sc_dirty"
	sc_env_before=$(docker exec "$host" cat "$sc_env") && sc_compose_before=$(docker exec "$host" cat "$sc_compose") ||
		fail 'cannot read the files of demo-stack'
	sc_head=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks rev-parse HEAD) || fail 'cannot read the head of /opt/stacks'
	docker exec -u qhost-deploy "$host" sh -ec "printf '%s\n' '$sc_mark' >>'$sc_env' && printf '# %s\n' '$sc_mark' >>'$sc_compose'" ||
		fail 'cannot change the files of demo-stack'

	step 'the changes of /opt/stacks list both files, and the diff of .env comes through sudo'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '[ ] demo-stack' 30
	tui_folder_to /opt/stacks
	tui_wait 'This folder has uncommitted changes' 30
	tui_send -l '='
	tui_wait 'Uncommitted changes of /opt/stacks' 15
	tui_wait 'demo-stack/compose.yaml' 15
	tui_has 'demo-stack/.env' || fail "the changes do not list .env: $(tui_screen)"
	tui_has 'demo-stack/secrets.env' && fail "the changes list secrets.env: $(tui_screen)"
	tui_send Enter
	tui_wait 'Show the diff of demo-stack/.env?' 15
	tui_has 'sudo qhost source diff' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "+$sc_mark" 30
	tui_excerpt "+$sc_mark" 2
	tui_send Escape
	tui_wait 'Uncommitted changes of /opt/stacks' 15

	step 'the commit goes to the remote and its run deploys demo-stack'
	sc_prev=$(newest_run demo-stack)
	tui_send -l '>'
	tui_wait 'Commit 2 files of /opt/stacks?' 15
	tui_has 'sudo qhost source commit' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait 'Committed 2 files as' 120
	sc_commit=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks rev-parse HEAD) || fail 'cannot read the head of /opt/stacks'
	[ "$sc_commit" != "$sc_head" ] || fail 'the commit left the head of /opt/stacks as it was'
	sc_remote=$(docker exec "$prefix-git" git -c safe.directory='*' --git-dir=/srv/git/stacks.git rev-parse main 2>&1) ||
		fail "cannot read the remote: $sc_remote"
	[ "$sc_remote" = "$sc_commit" ] || fail "the remote has $sc_remote, want the commit $sc_commit"
	sc_files=$(docker exec "$prefix-git" git -c safe.directory='*' --git-dir=/srv/git/stacks.git diff-tree --no-commit-id --name-only -r main 2>&1)
	[ "$sc_files" = "$(printf 'demo-stack/.env\ndemo-stack/compose.yaml')" ] || fail "the commit holds the wrong files: $sc_files"
	sc_dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $sc_dirty"
	[ -z "$sc_dirty" ] || fail "the stack folder has uncommitted changes after the commit: $sc_dirty"
	tui_send l
	tui_wait "[3 $tui_icon_runs Runs]" 15
	sc_run=$(next_run demo-stack "$sc_prev") || fail 'the commit started no run of demo-stack'
	wait_for "the run $sc_run" 240 run_finished demo-stack "$sc_run"
	qd stack log "$sc_run" | grep -q ' result succeeded' || fail "the run $sc_run did not succeed: $(qd stack log "$sc_run")"
	log "scenario: the interface committed .env and compose.yaml of demo-stack as $sc_commit, pushed it, and run $sc_run deployed it"

	step 'a further change of .env is discarded through sudo'
	docker exec -u qhost-deploy "$host" sh -ec "printf '%s\n' '$sc_more' >>'$sc_env'" || fail "cannot change $sc_env"
	tui_send 1
	tui_wait 'Uncommitted changes of /opt/stacks' 15
	tui_send R
	tui_wait 'demo-stack/.env' 30
	tui_send d
	tui_wait 'Discard the changes of demo-stack/.env?' 15
	tui_has 'sudo qhost source discard' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait 'Discarded the changes of demo-stack/.env.' 60
	docker exec "$host" grep -qxF "$sc_more" "$sc_env" && fail "$sc_env still holds the discarded line"
	docker exec "$host" grep -qxF "$sc_mark" "$sc_env" || fail "$sc_env lost the committed line"
	sc_dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $sc_dirty"
	[ -z "$sc_dirty" ] || fail "the stack folder is not clean after the discard: $sc_dirty"
	tui_tmux kill-server >/dev/null 2>&1 || true
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	log 'scenario: the interface discarded a further change of .env'

	step 'the commit is reverted through the clone of the administrator, and demo-stack runs on the files from before'
	sc_prev=$(newest_run demo-stack)
	as_admin "cd ~/stacks && git pull -q --ff-only && [ \"\$(git rev-parse HEAD)\" = '$sc_commit' ] &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test revert --no-edit HEAD >/dev/null && git push -q origin main" ||
		fail "cannot revert the commit $sc_commit"
	restart_service
	sc_run=$(next_run demo-stack "$sc_prev") || fail 'the revert started no run of demo-stack'
	wait_for "the run $sc_run" 240 run_finished demo-stack "$sc_run"
	qd stack log "$sc_run" | grep -q ' result succeeded' || fail "the run $sc_run did not succeed: $(qd stack log "$sc_run")"
	[ "$(docker exec "$host" cat "$sc_env")" = "$sc_env_before" ] && [ "$(docker exec "$host" cat "$sc_compose")" = "$sc_compose_before" ] ||
		fail 'the files of demo-stack differ from before the scenario'
}

# fs_password hands the terminal password to the sudo prompt that follows an answer of the interface.
fs_password() {
	tui_wait "[sudo] password for $admin" 30
	tui_type "$fs_login"
	tui_send Enter
}

# scenario_tui_folder_sudo proves adding a stack folder and taking over a foreign Compose project in the interface
# started without sudo. Like scenario_tui_sudo, the rule of $admin is put aside and sudo asks for the password at
# every command (timestamp_timeout=0), which is typed in the terminal. A new folder is cloned from the repository
# stacks8 (one stack, demo-fsudo): the path is inspected through sudo, the host key is read through sudo after a
# question, and the last question shows `sudo qhost source add`; before its yes nothing is bound and the folder does
# not exist. Afterwards the folder is bound and its stack deployed. A foreign Compose project below
# /srv/qd-fsudo-foreign is taken over the same way. Both folders are released and removed at the end, the rule of the
# administrator is put back and the password is locked again.
scenario_tui_folder_sudo() {
	tui_log=$work/tui-folder-sudo-screens.log
	: >"$tui_log"
	fs_login=E2e-admin-Login-7743
	fs_clone=/srv/qd-fsudo-clone
	fs_url=git.$domain:stacks8.git
	fs_base=/srv/qd-fsudo-foreign/fsapp
	fs_stack=demo-fsudo
	fs_proj=fsapp
	image=registry.$domain/demo/app

	step 'a repository with one stack, a deploy key for the new folder, and sudo asking for the password at every command'
	docker exec "$host" rm -rf "$fs_clone" /srv/qd-fsudo-foreign
	docker exec "$prefix-git" su git -s /bin/sh -c "rm -rf /srv/git/stacks8.git && git init -q --bare --initial-branch=main /srv/git/stacks8.git" ||
		fail 'cannot create the repository stacks8'
	as_admin "rm -rf ~/stacks8 && mkdir -p ~/stacks8/$fs_stack && cd ~/stacks8 &&
		printf 'name: $fs_stack\\n\\nservices:\\n  app:\\n    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}\\n    restart: unless-stopped\\n    environment:\\n      DEMO_NAME: $fs_stack\\n      DEMO_VERSION: \${DEMO_APP_VERSION}\\n      DEMO_HEALTH: \${DEMO_HEALTH}\\n' >$fs_stack/compose.yaml &&
		printf 'DEMO_APP_VERSION=1.0.0\\nDEMO_HEALTH=ok\\n' >$fs_stack/.env &&
		printf 'secrets.env\\ndata/\\n' >$fs_stack/.gitignore && git init -q -b main &&
		git add -A && git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Add the $fs_stack stack' &&
		git push -q $fs_url main" || fail 'cannot fill the repository stacks8'
	[ -z "$(stack_field "$fs_stack" source)" ] || fail "a stack $fs_stack exists before the folder"
	if list_has_path "$fs_clone"; then fail 'the new folder is bound before the interface adds it'; fi
	register_folder_key "$fs_clone"
	printf '%s:%s\n' "$admin" "$fs_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	docker exec "$host" sh -ec "mv /etc/sudoers.d/e2e-admin /etc/sudoers.d/e2e-admin.off &&
		printf 'Defaults:%s timestamp_timeout=0\n' '$admin' >/etc/sudoers.d/e2e-ask && chmod 0440 /etc/sudoers.d/e2e-ask && visudo -c >/dev/null" ||
		fail 'cannot put the sudo rule of the administrator aside'
	if docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1; then fail "sudo runs a command of $admin without a password although the rule is put aside"; fi

	step 'the interface without sudo clones a new folder: inspection, host key and the last question with their commands, nothing bound before yes'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'trap : INT; qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait '[ ] demo-cache' 30
	tui_send n
	tui_wait 'Add a stack folder' 10
	tui_type "$fs_clone"
	tui_send F2
	fs_password
	tui_keep_engine 60
	tui_wait "Clone a repository into $fs_clone?" 60
	tui_send y
	tui_wait 'Address of the repository to clone' 10
	tui_type "$fs_url"
	tui_send F2
	tui_wait 'Read the host key of the server' 15
	tui_has 'sudo qhost repo probe' || fail "the question does not show the probe command: $(tui_screen)"
	tui_excerpt 'Read the host key' 14
	if list_has_path "$fs_clone" || docker exec "$host" test -e "$fs_clone"; then fail 'the folder is bound or exists at the probe question'; fi
	tui_send y
	fs_password
	tui_wait 'Trust this host key for' 60
	tui_has "$(git_ed25519_fingerprint)" || fail "the host key question does not show the fingerprint: $(tui_screen)"
	tui_send y
	tui_wait "Clone $fs_url into $fs_clone?" 30
	tui_has 'sudo qhost source add' || fail "the last question does not show the source add command: $(tui_screen)"
	tui_has "$(git_ed25519_fingerprint)" || fail "the last question does not show the fingerprint: $(tui_screen)"
	tui_has '--expect-effect' || fail "the last question does not show the expected effect: $(tui_screen)"
	tui_excerpt "Clone $fs_url" 20
	if list_has_path "$fs_clone"; then fail 'the folder is bound before the last answer'; fi
	if docker exec "$host" test -e "$fs_clone"; then fail 'the folder exists before the last answer'; fi
	[ -z "$(stack_field "$fs_stack" source)" ] || fail "the stack $fs_stack exists before the last answer"

	step 'yes binds the folder and deploys its stack'
	tui_send y
	fs_password
	tui_wait "The stack folder $fs_clone is bound" 120
	tui_send Enter
	wait_for 'the deploy service to answer' 60 qd status
	list_has_path "$fs_clone" || fail "source list does not show $fs_clone: $(qd --output json source list)"
	docker exec "$host" test -f "$fs_clone/$fs_stack/compose.yaml" || fail 'the clone is missing in the new folder'
	wait_for "the first run of $fs_stack" 240 stack_settled "$fs_stack" succeeded
	[ "$(stack_images "$fs_stack")" = "$image:1.0.0" ] || fail "$fs_stack does not run 1.0.0: $(stack_images "$fs_stack")"
	wait_for "$fs_stack healthy" 60 stack_healthy "$fs_stack"
	log "scenario: the interface without sudo cloned $fs_clone through sudo and deployed $fs_stack"
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'a person starts a foreign Compose project; the interface without sudo takes it over'
	docker exec "$host" sh -ec "rm -rf /srv/qd-fsudo-foreign && mkdir -p '$fs_base' && chmod 0755 /srv/qd-fsudo-foreign '$fs_base'" ||
		fail 'cannot create the foreign project folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$fs_base/compose.yaml'"
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: foreign-fsapp
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$fs_base/.env'"
	docker exec "$host" sh -ec "chmod 0644 '$fs_base/compose.yaml' '$fs_base/.env'" || fail 'cannot set the modes of the foreign project files'
	docker exec "$host" sh -ec "cd '$fs_base' && docker compose up -d" >/dev/null 2>&1 || fail 'cannot start the foreign project'
	wait_for 'the foreign project fsapp' 60 stack_healthy "$fs_proj"
	fs_ids=$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project="$fs_proj" | sort)
	[ -n "$fs_ids" ] || fail 'the foreign project has no running container'
	if list_has_path "$fs_base"; then fail 'the folder of the foreign project is bound before the takeover'; fi
	[ -z "$(stack_field "$fs_proj" source)" ] || fail "a stack $fs_proj exists before the takeover"
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'trap : INT; qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 5
	tui_wait "foreign project $fs_proj" 30
	tui_ct_cursor_to "$fs_proj-app-1"
	tui_send Enter
	tui_wait 'Compose files' 15
	tui_send n
	tui_adopt_own
	fs_password
	tui_keep_engine 60
	tui_wait "Start a git repository in $fs_base and take it over?" 60
	# The command stands at the end of the sheet, below the window.
	for _ in 1 2 3 4 5 6 7 8 9 10; do tui_send Down; done
	tui_wait 'sudo qhost source add' 10
	tui_has '--expect-effect' || fail "the question does not show the expected effect: $(tui_screen)"
	tui_excerpt 'Start a git repository' 20
	if docker exec "$host" test -e "$fs_base/.git"; then fail 'the question made a repository'; fi
	if list_has_path "$fs_base"; then fail 'the question bound the folder'; fi
	[ "$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project="$fs_proj" | sort)" = "$fs_ids" ] ||
		fail 'the question changed the containers'
	tui_send y
	fs_password
	tui_wait "The stack folder $fs_base is added" 120
	tui_send Enter
	wait_for 'the deploy service to answer' 60 qd status
	list_has_path "$fs_base" || fail "source list does not show the folder: $(qd --output json source list)"
	[ "$(qd --output json source list | jq -r --arg p "$fs_base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the foreign folder is not a local source'
	docker exec "$host" test -d "$fs_base/.git" || fail 'the takeover made no repository'
	fs_source=$(printf '%s' "${fs_base#/}" | tr / -)
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	printf '%s' "$out" | jq -e --arg s "$fs_source" --arg n "$fs_proj" '[.[] | select(.source == $s)] | (length == 1 and .[0].stack == $n)' >/dev/null ||
		fail "stack list does not show exactly one stack $fs_proj of the folder: $out"
	tui_tmux kill-server >/dev/null 2>&1 || true
	log "scenario: the interface without sudo took over the foreign project $fs_proj through sudo"

	step 'the password shows on no captured screen; folders and projects are released, the rule of the administrator is put back'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$fs_login" "$tui_log"; then fail 'the password typed for sudo shows on a captured screen'; fi
	docker exec "$host" sh -ec 'rm -f /etc/sudoers.d/e2e-ask && mv /etc/sudoers.d/e2e-admin.off /etc/sudoers.d/e2e-admin && visudo -c >/dev/null' ||
		fail 'cannot put the sudo rule of the administrator back'
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	docker exec -u "$admin" "$host" sudo -n true >/dev/null 2>&1 || fail "sudo of $admin asks for a password again after the rule is put back"
	for fs_dir in "$fs_clone" "$fs_base"; do
		out=$(qd --output json source remove "$fs_dir" --yes) || fail "source remove of $fs_dir failed: $out"
		if list_has_path "$fs_dir"; then fail "source list still shows $fs_dir"; fi
	done
	docker exec "$host" sh -ec "cd '$fs_base' && docker compose down" >/dev/null 2>&1 || fail 'cannot shut down the foreign project'
	docker exec "$host" sh -c "docker ps -aq --filter label=com.docker.compose.project=$fs_stack | xargs -r docker rm -f" >/dev/null ||
		fail "cannot remove the containers of $fs_stack"
	docker exec "$host" rm -rf "$fs_clone" /srv/qd-fsudo-foreign
	wait_for 'the deploy service to answer' 30 qd status
}

# tui_wait_noecho waits up to $1 seconds until the terminal of the interface no longer echoes, as sudo turns the echo
# off for its password prompt; text typed before would show on the screen.
tui_wait_noecho() {
	tui_pane_tty=$(tui_tmux display -p -t qdtui '#{pane_tty}') || fail 'cannot read the terminal of the interface'
	i=0
	until docker exec -u "$admin" "$host" stty -F "$tui_pane_tty" -a 2>/dev/null | grep -qw -- -echo; do
		i=$((i + 1))
		[ "$i" -lt $(($1 * 4)) ] || fail "the terminal of the interface still echoes after $1 seconds: $(tui_screen)"
		sleep 0.25
	done
}
