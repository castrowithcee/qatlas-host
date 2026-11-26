#!/bin/sh
# Section window: the maintenance window on a real, short window: a stack in automatic mode is updated without a
# human, a stack in manual mode is only reported, a paused and a pinned stack stay as they are, and a change with
# window set takes effect in the running service; outside the window, a stack in automatic mode with the update
# trigger interval is updated within two minutes. The host window and the own window of the rootless instance follow.
# Then sudo qhost opens the terminal interface in a tmux pane, an update shows its changelog before it is confirmed
# and then runs, and two ticked stacks are updated one after another with one confirmation.
# Needs update: demo-stack on 1.1.0 as update leaves it, so the setup commit of scenario_window, which resets it to
# 1.0.1, starts a run. scenario_window_instances needs the rootless instance, which deploy needs.
# Sourced by run.sh; defines only functions and constants.

# window_log prints what the service logged about the window and the stacks since it started.
window_log() {
	docker exec "$host" journalctl -u qhost-deploy --no-pager -o cat
}

# scenario_window proves the maintenance window with the real service and a real, short window. demo-stack is in
# automatic mode, demo-cache in manual mode, demo-paused is paused and demo-pinned is pinned, all four offer 1.1.0.
# Afterwards demo-stack, reset to 1.0.1, is updated by the regular check with the update trigger interval and then
# waits for the window again.
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
	start=$(window_start_soon)
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
	wait_for 'the window to open' 150 window_open
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

	step 'demo-stack is reset to 1.0.1 while it waits for the window'
	prev=$(newest_run demo-stack)
	admin_commit demo-stack/.env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.1/' 'Reset demo-stack to 1.0.1'
	restart_service
	run=$(next_run demo-stack "$prev") || fail 'no run of demo-stack started for the reset to 1.0.1'
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run for the reset to 1.0.1 did not succeed: $(qd stack log "$run")"
	[ "$(stack_images demo-stack)" = "$image:1.0.1" ] || fail "demo-stack does not run 1.0.1: $(stack_images demo-stack)"

	step 'with the update trigger interval, the automatic stack is updated outside the window within two minutes'
	settings_set demo-stack "$(demo_stack_settings 'update:\n  mode: automatic\n  trigger: interval\n  interval: 30\n')"
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	runs_before=$(stack_run_count demo-stack)
	since=$(date +%s)
	wait_for 'the update check to update demo-stack' 120 interval_updated 1.1.0
	took=$(($(date +%s) - since))
	[ "$took" -le 120 ] || fail "the update check took $took seconds to update demo-stack"
	[ "$(window_field open)" = false ] || fail "the window opened meanwhile: $(qd --output json window show)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the update check'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git rev-list --count $head_before..HEAD")" = 1 ] || fail 'the update check did not make exactly one commit'
	update_commit_only_version "$(as_admin 'cd ~/stacks && git rev-parse HEAD')" 1.0.1 1.1.0
	[ "$(stack_run_count demo-stack)" = "$((runs_before + 1))" ] || fail 'the update check did not make exactly one run of demo-stack'
	line=$(window_log | grep 'update check: update step finished' | grep 'stack=demo-stack' || true)
	[ -n "$line" ] || fail "the service log has no update step of the update check: $(window_log | tail -n 20)"
	log "scenario: update check updated demo-stack to 1.1.0 in $took seconds: $(printf '%s' "$line" | tail -n 1)"

	step 'demo-stack waits for the window again'
	settings_set demo-stack "$(demo_stack_settings 'update:\n  mode: automatic\n')"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0: $(stack_images demo-stack)"
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

# scenario_window_instances proves the host window and the own window of an instance: sudo qhost window set without
# --engine writes /etc/qhost-deploy/window.json, which the system service and the rootless instance both read and
# follow without a warning; an own window for the rootless instance changes only that one and survives a change of the
# host window; window reset lets it follow the host again; a damaged file gives the default and a warning without
# stopping the services; without sudo the host window stays as it is. It leaves the host window far away like
# scenario_window.
scenario_window_instances() {
	wi_engine=rootless:$rootless_user
	wi_file=/etc/qhost-deploy/window.json
	wi_start() { date -u -d "+$1 hours" +%H:%M; }
	wi_a=$(wi_start 5) wi_b=$(wi_start 7) wi_c=$(wi_start 9)

	step 'sudo qhost window set without --engine writes the host window, which both instances follow'
	out=$(qd --output json window set --days daily --zone UTC --start "$wi_a" --duration 7m) || fail "window set failed: $out"
	printf '%s' "$out" | jq -e --arg s "$wi_a" '.origin == "host" and .start == $s and .duration == "7m" and .zone == "UTC"' >/dev/null ||
		fail "window set does not print the host window: $out"
	[ "$(docker exec "$host" stat -c %U:%G:%a "$wi_file")" = root:root:644 ] ||
		fail "$wi_file is $(docker exec "$host" stat -c %U:%G:%a "$wi_file"), want root:root:644"
	[ "$(docker exec "$host" stat -c %U:%G:%a "${wi_file%/*}")" = root:root:755 ] ||
		fail "${wi_file%/*} is $(docker exec "$host" stat -c %U:%G:%a "${wi_file%/*}"), want root:root:755"
	docker exec "$host" grep -q '"schema": 1' "$wi_file" || fail "$wi_file carries no format number: $(docker exec "$host" cat "$wi_file")"
	for u in qhost-deploy "$rootless_user"; do
		docker exec -u "$u" "$host" grep -q "\"start\": \"$wi_a\"" "$wi_file" || fail "the user $u of a deploy service cannot read $wi_file"
	done
	for e in system "$wi_engine"; do
		[ "$(window_field origin "$e")" = host ] && [ "$(window_field start "$e")" = "$wi_a" ] && [ "$(window_field duration "$e")" = 7m ] ||
			fail "the engine $e does not follow the host window: $(qd --output json window show)"
		[ -z "$(window_field warning "$e")" ] || fail "the engine $e warns about the host window: $(window_field warning "$e")"
	done
	out=$(docker exec "$host" journalctl --no-pager -o cat -u qhost-deploy -u "qhost-deploy@$rootless_user.service" | grep -F 'the host window file cannot be used' || true)
	[ -z "$out" ] || fail "a deploy service logged that it cannot use the host window file: $out"
	[ "$(qd --output json window show | jq length)" = 2 ] || fail "window show does not list exactly the two instances: $(qd --output json window show)"
	qd window show | grep -q "^$wi_engine .*host" || fail "the table does not show the origin of $wi_engine: $(qd window show)"

	step 'an own window for the rootless instance changes only that instance'
	out=$(qd --output json window set --engine "$wi_engine" --start "$wi_b" --duration 3m) || fail "own window set failed: $out"
	printf '%s' "$out" | jq -e --arg e "$wi_engine" --arg s "$wi_b" '.engine == $e and .origin == "own" and .start == $s and .duration == "3m" and .zone == "UTC"' >/dev/null ||
		fail "own window set does not print the own window: $out"
	[ "$(window_field origin "$wi_engine")" = own ] && [ "$(window_field start "$wi_engine")" = "$wi_b" ] ||
		fail "the rootless instance has no own window: $(qd --output json window show)"
	[ "$(window_field origin)" = host ] && [ "$(window_field start)" = "$wi_a" ] ||
		fail "the own window changed the system service: $(qd --output json window show)"
	docker exec "$host" grep -q "\"start\": \"$wi_a\"" "$wi_file" || fail "the own window changed $wi_file"

	step 'a change of the host window reaches the system service and not the own window'
	qd window set --start "$wi_c" >/dev/null || fail 'host window set failed'
	[ "$(window_field start)" = "$wi_c" ] && [ "$(window_field origin)" = host ] || fail "the system service does not follow: $(qd --output json window show)"
	[ "$(window_field start "$wi_engine")" = "$wi_b" ] && [ "$(window_field origin "$wi_engine")" = own ] ||
		fail "the host window reached an own window: $(qd --output json window show)"

	step 'window reset lets the rootless instance follow the host window again'
	out=$(qd --output json window reset --engine "$wi_engine") || fail "window reset failed: $out"
	[ "$(window_field origin "$wi_engine")" = host ] && [ "$(window_field start "$wi_engine")" = "$wi_c" ] ||
		fail "the rootless instance does not follow the host after the reset: $(qd --output json window show)"
	qd_fails window reset | grep -q -- '--engine' || fail 'window reset without --engine does not ask for it'

	step 'without sudo the host window stays as it is and window show still works'
	if out=$(as_admin 'qhost window set --start 04:00 2>&1'); then fail "window set without sudo succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'needs root' || fail "window set without sudo does not say it needs root: $out"
	[ "$(window_field start)" = "$wi_c" ] || fail "window set without sudo changed the host window: $(qd --output json window show)"
	as_admin 'qhost --output json window show' | jq -e 'length == 2' >/dev/null || fail 'window show without sudo does not list both instances'

	step 'a damaged host file gives the default window and a warning, and window set keeps the file'
	docker exec "$host" cp -p "$wi_file" "$wi_file.aside" || fail 'cannot keep the host file aside'
	docker exec "$host" sh -ec "printf '{\"schema\": 1, \"days\": [' >$wi_file"
	for e in system "$wi_engine"; do
		[ "$(window_field origin "$e")" = host ] && [ "$(window_field start "$e")" = 03:00 ] && [ -n "$(window_field warning "$e")" ] ||
			fail "the engine $e does not use the default with a warning: $(qd --output json window show)"
	done
	out=$(qd_fails window set --start 04:00) || fail "window set over a damaged file succeeded: $out"
	[ "$(docker exec "$host" cat "$wi_file")" = '{"schema": 1, "days": [' ] || fail "window set changed the damaged file: $(docker exec "$host" cat "$wi_file")"
	docker exec "$host" mv "$wi_file.aside" "$wi_file" || fail 'cannot put the host file back'
	[ "$(window_field start)" = "$wi_c" ] && [ -z "$(window_field warning)" ] || fail "the window is not back after the file is fixed: $(qd --output json window show)"
	[ "$(status_field started_at)" != "" ] || fail 'status does not answer after the damaged file'

	step 'the window is moved away again'
	window_far_away >/dev/null || fail 'cannot move the maintenance window out of the way'
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

	step 'U asks first: the question shows the versions and the changelog, nothing changes yet'
	head_before=$(as_admin 'cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD')
	runs_before=$(stack_run_count demo-cache)
	tui_cursor_to demo-cache
	tui_send U
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
	# The list is read again on R. The run lasts a few seconds, long enough for a poll every 0.3 seconds.
	i=0
	until tui_screen | grep -qE 'demo-cache +[0-9-]+ +[0-9:]+ +running'; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the Runs area never listed the running update: $(tui_screen)"
		tui_send R
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
	tui_send U
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
		tui_send R
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
	tui_send R
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
	tui_send R
	i=0
	while tui_screen | grep -qE 'demo-cache .* stopped '; do
		i=$((i + 1))
		[ "$i" -lt 30 ] || fail "the Stacks area still shows demo-cache as stopped: $(tui_screen)"
		tui_send R
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
	tui_walk_down "$ctname" tui_ct_cursor_on "$ctname"
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
		tui_walk_down "$foreign" tui_ct_cursor_on "$foreign"
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
