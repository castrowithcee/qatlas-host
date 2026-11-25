#!/bin/sh
# Section update: backups of data and volumes with owners and modes, the update detection per update ceiling without
# changing anything, the automatic rollback (data restored from the run's backup, revert commit on the remote, a
# stack whose rollback fails stopped and paused until resume), qhost diagnose, and version updates with qhost stack
# update: qhost stack settings set commits and pushes the settings of a stack without a run, and they act on the next
# update: one direct step, a stepwise major jump and a jump whose middle step fails. qhost stack rollback then sets
# the stack back to the version before an update: with --restore-data the file written since is gone and the stack is
# pinned and refuses an update; after unpin and a new update, --keep-data leaves such a file.
# Needs deploy: demo-stack on 1.0.1 with secrets.env (secret), and the rolled-back run of deploy that
# scenario_diagnose looks for.
# Sourced by run.sh; defines only functions and constants.

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
	failed_run=$(qd stack runs | grep 'rolled-back' | grep -m 1 -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
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

# fixture_write fills /data with entries of mixed numeric owners and modes.
fixture_write() {
	fixture_run "$1" "$(fixture_write_script)"
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
	printf '%s' "$run_log" | grep -q 'the remote refused the push' || fail "the run log does not show the remote refusing the push: $run_log"
	printf '%s' "$run_log" | grep -q 'rejected by the e2e hook' || fail "the run log does not show the output of the remote hook: $run_log"
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

	scenario_promote

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

	scenario_return
}

# scenario_promote takes the version of demo-stack, which runs 3.0.0, over to demo-cache, which has the same image
# repository and runs 1.1.0: the preview changes nothing, the confirmed command is a run with the trigger promote
# and one commit that names the source, and the return of that run leaves demo-cache on 1.1.0 without settings.
scenario_promote() {
	image=registry.$domain/demo/app

	step 'promote shows the versions of demo-stack for demo-cache without --confirm and changes nothing'
	wait_for 'demo-cache to be idle' 120 stack_idle demo-cache
	settings_set demo-cache "$(printf 'version: 1\nupdate:\n  major: direct\n')"
	[ "$(stack_images demo-cache)" = "$image:1.1.0" ] || fail "demo-cache does not run 1.1.0 before the promotion: $(stack_images demo-cache)"
	out=$(qd_fails --output json stack promote demo-stack demo-cache) || fail "promote without --confirm succeeded: $out"
	printf '%s' "$out" | grep -q '"to": *"3.0.0"' || fail "the preview does not show 3.0.0: $out"
	[ "$(stack_images demo-cache)" = "$image:1.1.0" ] || fail "the preview changed demo-cache: $(stack_images demo-cache)"
	source_commit=$(json_values "$out" source_commit | head -n 1)
	[ -n "$source_commit" ] || fail "the preview names no source commit: $out"

	step 'promote --confirm is a run of demo-cache with the trigger promote and a commit that names demo-stack'
	out=$(qd --output json stack promote demo-stack demo-cache --confirm --source-commit "$source_commit") || fail "promote failed: $out"
	[ "$(json_values "$out" result)" = succeeded ] || fail "the promotion did not succeed: $out"
	run=$(json_values "$out" run | head -n 1)
	commit=$(json_values "$out" commit | head -n 1)
	[ -n "$run" ] && [ -n "$commit" ] || fail "promote reported no run and commit: $out"
	qd --output json stack runs demo-cache | jq -e --arg r "$run" 'any(.[]; .run == $r and .trigger == "promote" and .result == "succeeded")' >/dev/null ||
		fail "the run $run of demo-cache has no trigger promote: $(qd --output json stack runs demo-cache)"
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$commit" ] || fail "the promotion commit $commit is not the head of the remote branch"
	as_admin "cd ~/stacks && git log -1 --format=%s $commit" | grep -q 'from demo-stack' || fail "the commit does not name demo-stack: $(as_admin "cd ~/stacks && git log -1 --format=%s $commit")"
	[ "$(as_admin "cd ~/stacks && git show --format= --numstat $commit")" = "$(printf '1\t1\tdemo-cache/.env')" ] || fail 'the promotion commit touches more than one line of demo-cache/.env'
	[ "$(stack_images demo-cache)" = "$image:3.0.0" ] || fail "demo-cache does not run 3.0.0: $(stack_images demo-cache)"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the promotion'
	out=$(qd stack promote demo-stack demo-cache --confirm --source-commit "$source_commit") || fail "promote of the same version failed: $out"
	printf '%s' "$out" | grep -q 'Nothing to promote' || fail "the same version is not reported as nothing to do: $out"

	step 'the return of the promotion and the removal of the settings leave demo-cache as it was'
	out=$(qd --output json stack rollback demo-cache "$run" --keep-data --yes) || fail "rollback of the promotion failed: $out"
	printf '%s' "$out" | jq -e '.result == "succeeded"' >/dev/null || fail "unexpected answer of the rollback: $out"
	out=$(qd stack unpin demo-cache) || fail "unpin failed: $out"
	out=$(qd --output json stack settings remove demo-cache) || fail "settings remove failed: $out"
	[ "$(stack_images demo-cache)" = "$image:1.1.0" ] || fail "demo-cache does not run 1.1.0 again: $(stack_images demo-cache)"
	log "scenario: promote: run $run, commit $commit"
}

# scenario_return sets demo-stack back with 'qhost stack rollback' after an update by qhost, which has backup rules:
# with --restore-data the file written after the update is gone, the stack is pinned and refuses an update; after
# unpin and a new update, --keep-data leaves a file written after that update. It starts and leaves demo-stack
# on 1.1.0, unpinned, without the marker files.
scenario_return() {
	image=registry.$domain/demo/app
	bind=/opt/stacks/demo-stack/data

	step 'an update of demo-stack to 2.0.0 is followed by a file in its backed-up data directory'
	wait_for 'demo-stack to be idle' 120 stack_idle demo-stack
	out=$(qd --output json stack update demo-stack --version 2.0.0) || fail "update to 2.0.0 failed: $out"
	update_run=$(json_values "$out" run | head -n 1)
	[ -n "$update_run" ] && [ "$(json_values "$out" result)" = succeeded ] || fail "the update to 2.0.0 did not succeed: $out"
	[ "$(stack_images demo-stack)" = "$image:2.0.0" ] || fail "demo-stack does not run 2.0.0: $(stack_images demo-stack)"
	[ -n "$(update_run_backup "$(qd stack log "$update_run")")" ] || fail "the update run $update_run made no backup"
	fixture_run "$bind" 'printf after-update >/data/after-update.txt' || fail 'cannot write the file after the update'

	step 'a return needs exactly one data choice, and without a terminal it needs --yes; both change nothing'
	out=$(qd_fails stack rollback demo-stack "$update_run") || fail "rollback without a data choice succeeded: $out"
	printf '%s' "$out" | grep -q 'exactly one of --restore-data and --keep-data' || fail "the refusal does not name the data choice: $out"
	out=$(qd_fails stack rollback demo-stack "$update_run" --restore-data --keep-data --yes) || fail "rollback with both data choices succeeded: $out"
	printf '%s' "$out" | grep -q 'exactly one of --restore-data and --keep-data' || fail "the refusal does not name the data choice: $out"
	out=$(qd_fails stack rollback demo-stack "$update_run" --restore-data) || fail "rollback without a terminal and without --yes succeeded: $out"
	printf '%s' "$out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $out"
	[ "$(stack_images demo-stack)" = "$image:2.0.0" ] || fail "a refused rollback changed the stack: $(stack_images demo-stack)"
	[ -z "$(stack_field demo-stack pinned)" ] || fail 'a refused rollback pinned the stack'
	fixture_run "$bind" 'test -f /data/after-update.txt' || fail 'a refused rollback removed the file'

	step 'rollback --restore-data returns demo-stack to 1.1.0 with the data of the update, and pins it'
	out=$(qd --output json stack rollback demo-stack "$update_run" --restore-data --yes) || fail "rollback --restore-data failed: $out"
	log "scenario: rollback $out"
	printf '%s' "$out" | jq -e --arg run "$update_run" '.result == "succeeded" and .pinned == true and .update_run == $run and
		.data == "restore" and .versions == "DEMO_APP_VERSION 2.0.0 -> 1.1.0" and (.note | contains("qhost stack unpin demo-stack"))' >/dev/null ||
		fail "unexpected answer of the rollback: $out"
	return_run=$(printf '%s' "$out" | jq -r .run)
	return_log=$(qd stack log "$return_run")
	printf '%s' "$return_log" | grep -q ' result succeeded' || fail "the return run did not succeed: $return_log"
	log "scenario: return steps: $(update_steps "$return_log")"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0 after the rollback: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the rollback'
	fixture_run "$bind" 'test ! -e /data/after-update.txt' || fail 'the file written after the update is still there after --restore-data'
	as_admin 'cd ~/stacks && git pull -q --ff-only'
	[ "$(as_admin "cd ~/stacks && git show HEAD:demo-stack/.env | grep '^DEMO_APP_VERSION='")" = DEMO_APP_VERSION=1.1.0 ] ||
		fail 'the branch does not hold 1.1.0 after the rollback'
	[ "$(stack_field demo-stack last_run)" = succeeded ] || fail "the last run of demo-stack is not succeeded: $(stack_field demo-stack last_run)"

	step 'the pinned demo-stack is shown as pinned and refuses an update'
	[ -n "$(stack_field demo-stack pinned)" ] || fail "status does not show demo-stack as pinned: $(qd --output json status | grep 'stack.demo-stack')"
	out=$(qd_fails stack update demo-stack) || fail "update of the pinned demo-stack succeeded: $out"
	printf '%s' "$out" | grep -q 'pinned' || fail "the refusal does not say that the stack is pinned: $out"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "the refused update changed the stack: $(stack_images demo-stack)"

	step 'after unpin and a new update, rollback --keep-data leaves the file written after that update'
	out=$(qd stack unpin demo-stack) || fail "unpin failed: $out"
	out=$(qd --output json stack update demo-stack --version 2.0.0) || fail "second update to 2.0.0 failed: $out"
	update_run=$(json_values "$out" run | head -n 1)
	[ -n "$update_run" ] && [ "$(json_values "$out" result)" = succeeded ] || fail "the second update to 2.0.0 did not succeed: $out"
	[ "$(stack_images demo-stack)" = "$image:2.0.0" ] || fail "demo-stack does not run 2.0.0 again: $(stack_images demo-stack)"
	fixture_run "$bind" 'printf kept >/data/kept.txt' || fail 'cannot write the file after the second update'
	out=$(qd --output json stack rollback demo-stack "$update_run" --keep-data --yes) || fail "rollback --keep-data failed: $out"
	printf '%s' "$out" | jq -e '.result == "succeeded" and .pinned == true and .data == "keep"' >/dev/null || fail "unexpected answer of the rollback: $out"
	[ "$(stack_images demo-stack)" = "$image:1.1.0" ] || fail "demo-stack does not run 1.1.0 after the second rollback: $(stack_images demo-stack)"
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the second rollback'
	[ "$(fixture_run "$bind" 'cat /data/kept.txt')" = kept ] || fail 'the file written after the update is gone after --keep-data'
	[ -n "$(stack_field demo-stack pinned)" ] || fail 'demo-stack is not pinned after the second rollback'

	step 'demo-stack is released and left on 1.1.0 without the marker files'
	out=$(qd stack unpin demo-stack) || fail "unpin failed: $out"
	[ -z "$(stack_field demo-stack pinned)" ] || fail 'demo-stack is still pinned after unpin'
	fixture_run "$bind" 'rm -f /data/kept.txt /data/after-update.txt' || fail 'cannot remove the marker files'
	log "scenario: return: demo-stack is back on 1.1.0, unpinned"
}
