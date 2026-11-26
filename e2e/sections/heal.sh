#!/bin/sh
# Section heal: qhost repairs a failing container and halts after three attempts, the failure monitor reports and
# restarts a dead or unhealthy container, and an update that fails after its start is rolled back in its observation
# time. The repair and the observation run in real time: in the full run their start units run early and their end
# units late, so the delays overlap with other sections; in a run of this section the end units follow at once and
# wait the time themselves, scenario_repair_end up to 7 minutes and scenario_observe_end up to 10 minutes.
# Needs deploy: the bound /opt/stacks, the admin clone and demo-stack running with its data folder.
# scenario_repair_start sets repair_demo_broken and scenario_observe_start sets observe_from, observe_update and
# observe_update_at for the end units of this section.
# The waits are shortened for the test (see setup_service). Restarts of the deploy service shift the repair (see
# scenario_repair_start) and the states between the attempts show only for a while, so the order of the register is
# part of the proof. The first attempt comes about 35 seconds after the break and shows until the second, about
# 130 seconds after the break, as the first restart waits a minute for a health the container never reaches, so
# scenario_repair_attempt follows at once. scenario_container_monitor fills the time to the second attempt, which
# shows for 150 seconds; scenario_repair_second checks it before access and tui-sudo, which restarts the service.
# The third attempt and the halt, about 5 minutes after the break, fall into the later sections; a restart moves a
# due attempt or the halt to about 40 seconds after it. scenario_observe_end and scenario_repair_end run after
# rootless, before source-local and source-git restart the service often.
# Sourced by run.sh; defines only functions and constants.

# stack_removed succeeds when qhost status marks the stack as removed from the repository.
stack_removed() {
	qd --output json status | jq -e --arg k "stack.$1.removed" '.[$k] == true' >/dev/null
}

# stack_leftovers_clean waits until the service has noticed that the stack $1 left the repository, which it only
# records: it stops no container and deletes no folder. Then it removes the containers and the unversioned folder
# $2 (data and the like) and proves that nothing of the stack is left on the host. The caller makes the service
# poll first.
stack_leftovers_clean() {
	wait_for "the service to report $1 as removed" 120 stack_removed "$1"
	docker exec "$host" sh -c "docker ps -aq --filter label=com.docker.compose.project=$1 | xargs -r docker rm -f" >/dev/null ||
		fail "cannot remove the containers of $1"
	docker exec "$host" rm -rf "$2" || fail "cannot remove $2"
	if docker exec "$host" test -e "$2"; then fail "$2 is left on the host"; fi
}

# demo_leftovers_clean removes what observe-demo and repair-demo leave on the host once scenario_observe_end and
# scenario_repair_end took them out of the repository. It runs after both, as its restart of the deploy service
# would delay the rollback or the halt that the other one still waits for.
demo_leftovers_clean() {
	restart_service
	stack_leftovers_clean observe-demo /opt/stacks/observe-demo
	if docker exec "$host" test -e /opt/stacks/.qatlas/qhost/observe-demo.yaml; then fail 'the settings of observe-demo are left on the host'; fi
	stack_leftovers_clean repair-demo /opt/stacks/repair-demo
}

# scenario_container_monitor proves that the failure monitor reports a container that dies outside qhost and then
# repairs it: the webhook hears container-failed and qhost status shows the finding; qhost restarts the container
# after seconds, and the webhook hears recovered with restarted. The same for a container that turns unhealthy.
# The two cases run at once on two stacks, demo-cache is killed and demo-stack turns unhealthy, so their real-time
# waits overlap; a stack's first failure in an hour is repaired after 15 seconds, a later one only 45 seconds
# after the restart before. The restart policy is switched off for the kill, so only qhost can bring the container
# back; the monitor reads every 5 seconds and ends a failure after two clean readings.
scenario_container_monitor() {
	step 'a channel for the failure monitor events is added'
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add mon --type webhook --events container-failed,recovered) || fail "notify add failed: $out"
	qd notify list | grep -q 'mon *webhook *container-failed,recovered' || fail "notify list does not show the channel: $(qd notify list)"

	step 'a container killed outside qhost is reported and shown in status'
	for stack in demo-cache demo-stack; do
		wait_for "$stack idle" 120 stack_idle "$stack"
		stack_healthy "$stack" || fail "$stack is not healthy before its failure"
		no_finding "$stack" container-failed || fail "$stack shows a failure before its failure: $(qd --output json status)"
	done
	victim=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-cache | head -n 1)
	[ -n "$victim" ] || fail 'demo-cache has no container to kill'
	docker exec "$host" docker update --restart=no "$victim" >/dev/null || fail 'cannot switch off the restart policy'
	docker exec "$host" docker kill "$victim" >/dev/null || fail 'cannot kill the container'
	docker exec "$host" touch /opt/stacks/demo-stack/data/break-once || fail 'cannot break the health of demo-stack'
	wait_for 'the webhook for the failed container' 120 hook_seen /notify container-failed demo-cache
	wait_for 'the finding container-failed in status' 60 has_finding demo-cache container-failed
	stack_field demo-cache findings | grep -q 'exited' || fail "the finding does not name the kind exited: $(qd --output json status)"
	log 'scenario: the killed container was reported and shown as a finding'

	# The unhealthy finding of demo-stack lasts only until one restart ends it, so it is checked before the
	# recovery of demo-cache, which stays recorded.
	step 'a container that turns unhealthy is restarted by qhost'
	wait_for 'the webhook for the unhealthy container' 90 hook_has /notify container-failed '\"stack\":\"demo-stack\"' '\"kind\":\"unhealthy\"'
	wait_for 'the finding container-failed in status' 60 has_finding demo-stack unhealthy
	wait_for 'the webhook for the recovered container' 150 hook_has /notify recovered '\"stack\":\"demo-stack\"' '\"kind\":\"unhealthy\"' '\"restarted\":true'
	wait_for 'the finding to end' 60 no_finding demo-stack container-failed
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the repair'
	if docker exec "$host" test -e /opt/stacks/demo-stack/data/break-once; then fail 'the restart did not clear the break'; fi
	log 'scenario: the unhealthy container was restarted by qhost and recovered'

	step 'qhost restarts the container: recovered is reported with restarted and the finding ends'
	wait_for 'demo-cache healthy again' 90 stack_healthy demo-cache
	wait_for 'the webhook for the recovered container' 90 hook_has /notify recovered '\"stack\":\"demo-cache\"' '\"kind\":\"exited\"' '\"restarted\":true'
	wait_for 'the finding to end' 60 no_finding demo-cache container-failed
	docker exec "$host" docker update --restart=unless-stopped "$victim" >/dev/null || fail 'cannot restore the restart policy'
	out=$(qd notify remove mon --yes) || fail "notify remove failed: $out"
	log 'scenario: qhost restarted the killed container and the finding ended'
}

# repair_text prints the repair line of a stack in qhost status.
repair_text() {
	stack_field "$1" repair
}

repair_first_attempt() {
	repair_text repair-demo | grep -q '^1 of 3 attempts, next '
}

repair_second_attempt() {
	repair_text repair-demo | grep -q '^2 of 3 attempts, next '
}

repair_halted() {
	repair_text "$1" | grep -q '^halted since '
}

# scenario_repair_start adds the stack repair-demo and breaks it for good: its health check fails while the file
# data/break exists, which a restart does not remove. qhost restarts it 15 seconds after the failure began, then 45
# and 150 seconds after the attempt before, and then halts. The delays run in real time; scenario_repair_attempt,
# scenario_repair_second and scenario_repair_end check them later, so they overlap with the other scenarios. The
# monitor keeps the time a container has been unhealthy in memory only: a restart of the deploy service ends the
# failure for two readings and starts it anew, which delays the next attempt or the halt when the restart falls
# shortly before it.
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
}

# scenario_repair_attempt waits for the first repair attempt of repair-demo. No scenario between
# scenario_repair_start and this one restarts the deploy service, so the attempt comes on time.
scenario_repair_attempt() {
	wait_for 'the finding container-failed of repair-demo' 90 has_finding repair-demo container-failed
	wait_for 'the first repair attempt' 90 repair_first_attempt
	log "scenario: repair-demo $(repair_text repair-demo)"
}

# scenario_repair_second waits for the second repair attempt of repair-demo. The failure monitor scenario before it
# restarts no deploy service, while the scenarios after it do so often and would delay the attempt.
scenario_repair_second() {
	wait_for 'the second repair attempt' 150 repair_second_attempt
	log "scenario: repair-demo $(repair_text repair-demo)"
}

# scenario_repair_end waits until qhost gave up on repair-demo and proves that the halt is shown, reported
# once and released by resume; then it fixes the stack and removes it from the repository.
scenario_repair_end() {
	step 'after three attempts the repair is halted, shown in status and the TUI, and reported once'
	wait_for 'the repair of repair-demo to halt' 420 repair_halted repair-demo
	log "scenario: repair-demo halted after $(($(date +%s) - repair_demo_broken)) seconds: $(repair_text repair-demo)"
	repair_text repair-demo | grep -q 'after 3 of 3 attempts' || fail "status does not name the attempts: $(repair_text repair-demo)"
	wait_for 'the webhook for the failed recovery' 60 hook_has /notify recovery-failed '\"stack\":\"repair-demo\"' '\"attempts\":3'
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
	# A repeated recovery-failed could only come from a monitor cycle after the halt. The finding ended and recovered
	# was sent by such cycles, so the count is final here.
	wait_for 'the webhook for the recovery of repair-demo' 60 hook_has /notify recovered '\"stack\":\"repair-demo\"'
	[ "$(docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F '"path":"/notify"' | grep -F '"X-Qhost-Event":"recovery-failed"' | grep -cF '\"stack\":\"repair-demo\"')" = 1 ] ||
		fail 'recovery-failed was not sent exactly once'
	out=$(qd notify remove rep --yes) || fail "notify remove rep failed: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q repair-demo &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the repair-demo stack" && git push -q origin main'
	log 'scenario: resume released the repair; repair-demo is fixed and removed'
}

# scenario_observe_start deploys the stack observe-demo, whose image 1.1.0 turns unhealthy for good 100 seconds
# after it starts, and updates it to 1.1.0 with a marker in its data before and one after the update. The
# observation time of the update (60 minutes by default) is running; qhost restarts the container once after
# the failure began and, if it lasts 45 more seconds, rolls the update back (a failure that ended with the
# restart counts again once the container is unhealthy for 15 seconds, about 2 minutes after the restart).
# The delays run in real time;
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
	wait_for 'the webhook for the rolled-back update' 600 hook_has /notify rolled-back '\"stack\":\"observe-demo\"' '\"cause\":\"observation\"' "\\\"commit\\\":\\\"$observe_update\\\""
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

	step 'the rolled-back version is locked: the status shows it and an automatic check starts no second run'
	[ "$(stack_field observe-demo failed_versions)" = DEMO_APP_VERSION=1.1.0 ] ||
		fail "status does not show the locked version: $(stack_field observe-demo failed_versions)"
	settings_set observe-demo 'version: 1
update:
  mode: automatic
  trigger: interval
  interval: 30
backup:
  paths:
    - data
'
	runs_before=$(stack_run_count observe-demo)
	# One check per 30 seconds finds 1.1.0 in the registry; the lock keeps it from planning that version again.
	sleep 35
	[ "$(stack_run_count observe-demo)" = "$runs_before" ] || fail "a check started a run for the locked version: $(qd stack runs observe-demo)"
	[ "$(stack_images observe-demo)" = "registry.$domain/demo/late:1.0.0" ] || fail "observe-demo left 1.0.0: $(stack_images observe-demo)"
	[ "$(stack_field observe-demo failed_versions)" = DEMO_APP_VERSION=1.1.0 ] || fail 'the lock is gone after the check'

	step 'observe-demo leaves the repository'
	out=$(qd notify remove obs --yes) || fail "notify remove obs failed: $out"
	as_admin 'cd ~/stacks && git pull -q --ff-only && git rm -r -q observe-demo .qatlas/qhost/observe-demo.yaml &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -m "Remove the observe-demo stack" && git push -q origin main'
	log 'scenario: the update of observe-demo was rolled back by qhost; observe-demo is removed'
}
