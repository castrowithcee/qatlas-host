#!/bin/sh
# Section rootless: stack folders of the rootless instance. source inspect reports the containers of a folder in the
# rootless engine as conflicts and lists the engine as not checked while it is stopped; source add --engine, the
# stacks, runs, backups, proposals and the choice of the engine in the interface follow for the instance.
# Needs deploy: the rootless instance, and the folder /opt/stacks and the stack demo-stack of the system engine,
# which scenario_source_rootless refuses for the instance.
# scenario_source_rootless sets sr_base for scenario_source_engine.
# Sourced by run.sh; defines only functions and constants.

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

# sr_stack_runs succeeds when the stack demo-rootless runs in the rootless engine of $rootless_user.
sr_stack_runs() {
	as_rootless "docker ps --filter label=com.docker.compose.project=demo-rootless --format '{{.Image}}'" | grep -qx 'demo/app:1.0.0'
}

# scenario_source_rootless proves source add --engine for the rootless engine of $rootless_user, whose instance
# scenario_rootless_instance set up: an engine without an instance, a folder that is a stack folder of the system
# engine and a stack name that the system engine has are refused and change nothing; a folder with a stack is then
# taken over into the instance, which gets the drop-in with that folder only and the group of its user on the folder,
# deploys the stack in the rootless engine, while the system engine neither lists the folder nor runs the stack. The
# stack stays; scenario_uninstall checks that the uninstall removes the drop-in.
scenario_source_rootless() {
	sr_base=/srv/qd-rootless
	sr_dup=/srv/qd-rootless-dup
	sr_engine=rootless:$rootless_user
	sr_unit=qhost-deploy@$rootless_user.service
	sr_dropin=/etc/systemd/system/$sr_unit.d/10-stack-folders.conf
	sr_sysdropin=/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf

	step 'two folders with a stack each are prepared as root'
	docker exec "$host" sh -ec "rm -rf '$sr_base' '$sr_dup' && mkdir -p '$sr_base/demo-rootless' '$sr_dup/demo-stack' &&
		chmod 0755 '$sr_base' '$sr_dup'" || fail 'cannot create the folders for the rootless engine'
	printf 'name: demo-rootless\nservices:\n  app:\n    image: demo/app:1.0.0\n    restart: unless-stopped\n' |
		docker exec -i "$host" sh -ec "cat >'$sr_base/demo-rootless/compose.yaml'" || fail 'cannot write the compose file of demo-rootless'
	printf 'name: demo-stack\nservices:\n  app:\n    image: demo/app:1.0.0\n' |
		docker exec -i "$host" sh -ec "cat >'$sr_dup/demo-stack/compose.yaml'" || fail 'cannot write the compose file of the duplicate'
	sr_group=$(docker exec "$host" id -gn "$rootless_user") || fail "cannot read the group of $rootless_user"
	sr_dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$sr_base")

	step 'an engine without an instance is refused with its reason and changes nothing'
	out=$(qd_fails source add "$sr_base" --engine rootless:e2e-noinstance --yes) || fail "source add into an engine without an instance succeeded: $out"
	printf '%s' "$out" | grep -q 'no deploy service is set up for the rootless engine of' || fail "the refusal does not give the reason: $out"
	if docker exec "$host" test -e "$sr_dropin"; then fail 'a refused engine wrote the drop-in of the instance'; fi

	step 'a folder that is a stack folder of the system engine is refused'
	out=$(qd_fails source add /opt/stacks --engine "$sr_engine" --yes) || fail "source add of a folder of the system engine succeeded: $out"
	printf '%s' "$out" | grep -q 'overlaps the stack folder /opt/stacks of the system engine' || fail "the refusal does not name the overlap: $out"
	if docker exec "$host" test -e "$sr_dropin"; then fail 'a refused overlap wrote the drop-in of the instance'; fi

	step 'a stack name that the system engine has is refused'
	out=$(qd_fails source add "$sr_dup" --engine "$sr_engine" --yes) || fail "source add of a duplicate stack name succeeded: $out"
	printf '%s' "$out" | grep -q 'engine already has a stack named demo-stack' || fail "the refusal does not name the stack: $out"
	if docker exec "$host" test -e "$sr_dropin"; then fail 'a refused name wrote the drop-in of the instance'; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$sr_dup")" = "root:root 755" ] || fail 'a refused name changed the folder'

	step "source add --engine $sr_engine takes the folder over into the instance"
	out=$(qd --output json source add "$sr_base" --engine "$sr_engine" --yes) || fail "source add into the instance failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e --arg p "$sr_base" '.kind == "local" and .path == $p' >/dev/null || fail "unexpected source add result: $out"
	[ "$(docker exec "$host" cat "$sr_dropin")" = "$(printf '[Service]\nReadWritePaths=-%s' "$sr_base")" ] ||
		fail "the drop-in of the instance is not exactly the folder: $(docker exec "$host" cat "$sr_dropin")"
	[ "$(docker exec "$host" stat -c '%U:%G' "$sr_base")" = "root:$sr_group" ] ||
		fail "the folder has the wrong owner or group: $(docker exec "$host" stat -c '%U:%G' "$sr_base")"
	[ "$(docker exec "$host" systemctl is-active "$sr_unit")" = active ] || fail "$sr_unit does not run after the takeover"
	wait_for 'the stack to run in the rootless engine' 180 sr_stack_runs

	step 'the system engine neither lists the folder nor runs the stack'
	sr_listed=$(qd --output json source list | jq -r --arg p "$sr_base" '[.sources[] | select(.path == $p) | .engine] | join(",")')
	[ "$sr_listed" = "$sr_engine" ] || fail "source list does not show the folder once, with the engine $sr_engine, but as: $sr_listed"
	if docker exec "$host" grep -qF "$sr_base" "$sr_sysdropin"; then fail 'the drop-in of the system service names the folder of the instance'; fi
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-rootless)" ] ||
		fail 'the system engine runs the stack of the instance'
	log "scenario: the folder $sr_base belongs to $sr_unit; demo-rootless runs in the rootless engine only"
}

# sr_run_done succeeds when the instance of $rootless_user records the run $1 of demo-rootless as succeeded.
sr_run_done() {
	qd --output json stack runs demo-rootless --engine "rootless:$rootless_user" |
		jq -e --arg r "$1" 'any(.[]; .run == $r and .result == "succeeded")' >/dev/null
}

# sr_idle succeeds when the instance of $rootless_user has no run of demo-rootless in progress and has finished one.
sr_idle() {
	qd --output json status | jq -e --arg p "instance.$rootless_user.stack.demo-rootless." '
		(.[$p + "active_run"] // "") == "" and (.[$p + "last_run"] // "") != ""' >/dev/null
}

# scenario_source_engine proves that commands reach the deploy service of the engine that has the stack, on the
# folder scenario_source_rootless bound: stack list without sudo names both engines, a deploy and a change of the
# settings under sudo act in the rootless instance and not in the system service, and a credential set with --engine
# is listed by that instance only. Container list without sudo shows both engines, and a container restart under sudo acts in
# the engine of the container. The credential is deleted again; the stack, its settings and its run stay.
scenario_source_engine() {
	se_engine=rootless:$rootless_user
	se_cred=registry.qhost-e2e-rootless.test

	step 'stack list without sudo shows the stacks of the system service and of the instance, each with its engine'
	out=$(as_admin 'qhost --output json stack list') || fail "qhost stack list without sudo failed: $out"
	printf '%s' "$out" | jq -e '[.[] | select(.stack == "demo-stack") | .engine] | index("system") != null' >/dev/null ||
		fail "stack list does not show demo-stack with the engine system: $out"
	printf '%s' "$out" | jq -e --arg e "$se_engine" '[.[] | select(.stack == "demo-rootless") | .engine] == [$e]' >/dev/null ||
		fail "stack list does not show demo-rootless once, with the engine $se_engine: $out"

	step 'sudo qhost stack deploy reaches the instance: the run is recorded there and the stack runs in the rootless engine'
	wait_for 'the first run of demo-rootless in the instance' 180 sr_idle
	runs_before=$(qd --output json stack runs --engine system | jq 'length')
	out=$(as_admin 'sudo qhost --output json stack deploy demo-rootless') || fail "sudo qhost stack deploy demo-rootless failed: $out"
	se_run=$(printf '%s' "$out" | jq -r '.run') || fail "unexpected deploy result: $out"
	[ -n "$se_run" ] && [ "$se_run" != null ] || fail "the deploy names no run: $out"
	wait_for "the run $se_run of demo-rootless to succeed in the instance" 180 sr_run_done "$se_run"
	qd --output json stack runs --engine system | jq -e --arg r "$se_run" 'any(.[]; .run == $r) | not' >/dev/null ||
		fail "the system service recorded the run $se_run"
	[ "$(qd --output json stack runs --engine system | jq 'length')" = "$runs_before" ] || fail 'the deploy changed the runs of the system service'
	sr_stack_runs || fail 'demo-rootless does not run in the rootless engine after the deploy'
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-rootless)" ] ||
		fail 'the deploy started demo-rootless in the system engine'

	step 'sudo qhost stack settings set changes the settings held by the instance'
	out=$(qd --output json stack settings demo-rootless) || fail "stack settings of demo-rootless failed: $out"
	printf '%s' "$out" | jq -e '.configured == false' >/dev/null || fail "demo-rootless already has settings: $out"
	out=$(printf 'version: 1\ncheck:\n  containers: false\n' |
		docker exec -i -u "$admin" "$host" sh -ec 'sudo qhost --output json stack settings set demo-rootless --file - 2>&1') ||
		fail "sudo qhost stack settings set demo-rootless failed: $out"
	printf '%s' "$out" | jq -e '.changed == true' >/dev/null || fail "settings set reports no change: $out"
	out=$(qd --output json stack settings demo-rootless) || fail "stack settings of demo-rootless failed after the change: $out"
	printf '%s' "$out" | jq -e '.configured == true and .settings.check.containers == false' >/dev/null ||
		fail "the instance does not hold the new settings: $out"
	docker exec "$host" git -c "safe.directory=$sr_base" -C "$sr_base" log --format=%s | grep -qx 'qhost: settings of demo-rootless' ||
		fail 'the stack folder of the instance has no settings commit'
	out=$(qd_fails stack settings demo-rootless --engine system) || fail "the system service answered for demo-rootless: $out"

	step "container list without sudo shows the containers of both engines, and sudo qhost container restart acts in the engine of the container"
	wait_for 'the instance to be idle' 180 sr_idle
	se_ct=$(as_rootless "docker ps --filter label=com.docker.compose.project=demo-rootless --format '{{.Names}}'" | head -n 1)
	[ -n "$se_ct" ] || fail 'no container of demo-rootless runs in the rootless engine'
	se_sys_started() {
		docker exec "$host" sh -c "docker ps -q --filter label=com.docker.compose.project=demo-stack | xargs -r docker inspect -f '{{.Name}} {{.State.StartedAt}}'"
	}
	out=$(as_admin 'qhost --output json container list 2>/dev/null') || fail "qhost container list without sudo failed: $out"
	printf '%s' "$out" | jq -e --arg e "$se_engine" '
		any(.groups[]; .engine == "system" and .kind == "own" and .name == "demo-stack")
		and any(.groups[]; .engine == $e and .kind == "own" and .name == "demo-rootless" and (.read_only // false) == false)
		and (.notes // [] | any(contains("shown only with sudo")))' >/dev/null ||
		fail "container list without sudo does not show demo-stack and demo-rootless with their engines, or lacks the note: $out"
	out=$(as_admin 'qhost container list 2>&1') || fail "qhost container list without sudo failed: $out"
	printf '%s' "$out" | grep -q "^own  *$se_engine  *demo-rootless " || fail "the table does not show demo-rootless with the engine $se_engine: $out"
	printf '%s' "$out" | grep -q '^own  *system  *demo-stack ' || fail "the table does not show demo-stack with the engine system: $out"
	printf '%s' "$out" | grep -qF 'note: Docker engines without a deploy service are shown only with sudo' ||
		fail "container list without sudo does not print the note: $out"
	out=$(qd --output json container show "$se_ct") || fail "container show $se_ct failed: $out"
	printf '%s' "$out" | jq -e --arg e "$se_engine" '.engine == $e' >/dev/null || fail "container show does not name the engine $se_engine for $se_ct: $out"
	se_started=$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $se_ct")
	se_sys_before=$(se_sys_started)
	[ -n "$se_sys_before" ] || fail 'no container of demo-stack runs in the system engine'
	out=$(as_admin "sudo qhost container restart $se_ct --yes 2>&1") || fail "sudo qhost container restart $se_ct failed: $out"
	[ "$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $se_ct")" != "$se_started" ] || fail "the restart did not restart $se_ct in the rootless engine"
	[ "$(as_rootless "docker inspect -f '{{.State.Running}}' $se_ct")" = true ] || fail "$se_ct does not run after the restart"
	[ "$(se_sys_started)" = "$se_sys_before" ] || fail 'the restart changed the containers of the system engine'
	log "scenario: container list showed both engines without sudo; the restart of $se_ct acted in $se_engine only"

	step "a credential set with --engine $se_engine is listed by that instance only"
	out=$(printf '%s' 'demo-user:demo-token-e2e' |
		docker exec -i -u "$admin" "$host" sh -ec "sudo qhost credential set $se_cred --engine $se_engine 2>&1") ||
		fail "credential set --engine $se_engine failed: $out"
	qd --output json credential list --engine "$se_engine" | jq -e --arg n "$se_cred" 'any(.[]; .name == $n)' >/dev/null ||
		fail "the instance does not list the credential: $(qd credential list --engine "$se_engine")"
	qd --output json credential list | jq -e --arg n "$se_cred" 'any(.[]; .name == $n) | not' >/dev/null ||
		fail "the system service lists the credential of the instance: $(qd credential list)"
	out=$(as_admin "sudo qhost credential delete $se_cred --engine $se_engine 2>&1") || fail "credential delete --engine $se_engine failed: $out"
	qd --output json credential list --engine "$se_engine" | jq -e --arg n "$se_cred" 'any(.[]; .name == $n) | not' >/dev/null ||
		fail 'the credential is still listed by the instance after its deletion'
	log "scenario: stack list, deploy, settings and credential reached the instance of $rootless_user and not the system service"
}

# pe_stack_runs succeeds when the stack demo-proposed runs in the rootless engine of $rootless_user.
pe_stack_runs() {
	as_rootless "docker ps --filter label=com.docker.compose.project=demo-proposed --format '{{.Image}}'" | grep -qx 'demo/app:1.0.0'
}

# pe_idle succeeds when the instance of $rootless_user has no run of demo-proposed in progress and has finished one.
pe_idle() {
	qd --output json status | jq -e --arg p "instance.$rootless_user.stack.demo-proposed." '
		(.[$p + "active_run"] // "") == "" and (.[$p + "last_run"] // "") != ""' >/dev/null
}

# scenario_proposal_engine proves a proposal for the instance of a rootless engine: an administrator without sudo
# submits a source-add proposal with --engine, list and show find it in that instance and name the engine, and
# sudo qhost proposal approve takes the folder over into the instance, which deploys the stack in the rootless
# engine while the system engine neither lists the folder nor runs the stack. The approval runs in a terminal of
# tmux, because only a person at a terminal may approve. The folder is released and the stack is removed again;
# the entry of the folder stays in the drop-in of the instance, which scenario_uninstall removes with it.
scenario_proposal_engine() {
	pe_dir=/srv/qd-proposal-engine
	pe_engine=rootless:$rootless_user
	pe_dropin=/etc/systemd/system/qhost-deploy@$rootless_user.service.d/10-stack-folders.conf

	step 'a folder with a stack is prepared as root'
	docker exec "$host" sh -ec "rm -rf '$pe_dir' && mkdir -p '$pe_dir/demo-proposed' && chmod 0755 '$pe_dir'" ||
		fail 'cannot create the folder of the proposal'
	printf 'name: demo-proposed\nservices:\n  app:\n    image: demo/app:1.0.0\n    restart: unless-stopped\n' |
		docker exec -i "$host" sh -ec "cat >'$pe_dir/demo-proposed/compose.yaml'" || fail 'cannot write the compose file of demo-proposed'
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-proposed)" ] ||
		fail 'the system engine already runs demo-proposed'

	step "an administrator without sudo submits a source-add proposal with --engine $pe_engine"
	out=$(printf '{"path":"%s"}' "$pe_dir" | as_admin "qhost --output json proposal submit --kind source-add --engine $pe_engine" 2>&1) ||
		fail "qhost proposal submit --engine $pe_engine without sudo failed: $out"
	pe_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$pe_id" ] || fail "qhost proposal submit printed no id: $out"
	printf '%s' "$out" | jq -e --arg e "$pe_engine" '.state == "open" and .engine == $e' >/dev/null ||
		fail "the new proposal is not open in the engine $pe_engine: $out"
	out=$(as_admin 'qhost --output json proposal list') || fail "qhost proposal list without sudo failed: $out"
	printf '%s' "$out" | jq -e --arg i "$pe_id" --arg e "$pe_engine" '[.[] | select(.id == $i) | .engine] == [$e]' >/dev/null ||
		fail "proposal list does not show $pe_id once, with the engine $pe_engine: $out"
	out=$(as_admin "qhost --output json proposal show $pe_id") || fail "qhost proposal show without --engine failed: $out"
	printf '%s' "$out" | jq -e --arg e "$pe_engine" --arg p "$pe_dir" '.engine == $e and .state == "open" and .content.path == $p' >/dev/null ||
		fail "proposal show does not find the proposal in the engine $pe_engine: $out"
	if as_admin "qhost proposal show $pe_id --engine system" >/dev/null 2>&1; then fail 'the system service knows the proposal of the instance'; fi
	[ "$(docker exec "$host" ls -A "$pe_dir")" = demo-proposed ] || fail 'the submission changed the folder'
	if list_has_path "$pe_dir"; then fail 'the folder is bound before the approval'; fi

	step "sudo qhost proposal approve takes the folder over into the instance of $pe_engine"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 200 -y 50 "env LANG=C.UTF-8 sh -c 'sudo qhost proposal approve $pe_id; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the approval'
	tui_wait "Approve proposal $pe_id and apply these changes?" 60
	tui_tmux capture-pane -p -S - -t qdtui | grep -qF -- "Engine: $pe_engine" || fail "the approval does not show the engine: $(tui_screen)"
	if list_has_path "$pe_dir"; then fail 'the folder is bound before the last answer'; fi
	tui_send y Enter
	tui_wait 'qhost-exit=0' 120
	tui_tmux kill-server >/dev/null 2>&1 || true
	wait_for 'the proposal to be applied' 30 prop_is "$pe_id" applied

	step 'the folder belongs to the instance, and the stack runs in the rootless engine only'
	pe_listed=$(qd --output json source list | jq -r --arg p "$pe_dir" '[.sources[] | select(.path == $p) | .engine] | join(",")')
	[ "$pe_listed" = "$pe_engine" ] || fail "source list does not show the folder once, with the engine $pe_engine, but as: $pe_listed"
	docker exec "$host" grep -qx "ReadWritePaths=-$pe_dir" "$pe_dropin" || fail "the drop-in of the instance does not list $pe_dir"
	if docker exec "$host" grep -qF "$pe_dir" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'the drop-in of the system service names the folder of the instance'
	fi
	wait_for 'the stack to run in the rootless engine' 180 pe_stack_runs
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-proposed)" ] ||
		fail 'the system engine runs the stack of the proposal'

	step 'the folder is released and its stack removed again'
	wait_for 'the first run of demo-proposed in the instance' 180 pe_idle
	out=$(qd --output json source remove "$pe_dir" --yes) || fail "source remove of $pe_dir failed: $out"
	as_rootless "docker compose -p demo-proposed down" >/dev/null 2>&1 || fail 'cannot remove the compose project demo-proposed'
	[ -z "$(as_rootless "docker ps -aq --filter label=com.docker.compose.project=demo-proposed")" ] || fail 'the rootless engine still has demo-proposed'
	docker exec "$host" rm -rf "$pe_dir" || fail "cannot remove $pe_dir"
	if list_has_path "$pe_dir"; then fail 'the folder is still bound'; fi
	log "scenario: proposal $pe_id was submitted with --engine $pe_engine, approved under sudo and applied in the instance; demo-proposed ran in the rootless engine only"
}

# tc_runs succeeds when the stack $1 runs in the rootless engine of $rootless_user.
tc_runs() {
	as_rootless "docker ps --filter label=com.docker.compose.project=$1 --format '{{.Image}}'" | grep -qx 'demo/app:1.0.0'
}

# tc_idle succeeds when the instance of $rootless_user has no run of the stack $1 in progress and has finished one.
tc_idle() {
	qd --output json status | jq -e --arg p "instance.$rootless_user.stack.$1." '
		(.[$p + "active_run"] // "") == "" and (.[$p + "last_run"] // "") != ""' >/dev/null
}

# tc_has_flat succeeds when the text $1 stands on the screen of the interface once its line breaks and the runs of
# spaces are taken out, as a command that the sheet breaks between its arguments does.
tc_has_flat() {
	tui_screen | tr -s ' \n' '  ' | grep -qF -- "$1"
}

# tc_release releases the folder $2 with the stack $1 in the rootless engine and removes both again.
tc_release() {
	wait_for "the first run of $1 in the instance" 180 tc_idle "$1"
	out=$(qd --output json source remove "$2" --yes) || fail "source remove of $2 failed: $out"
	as_rootless "docker compose -p $1 down" >/dev/null 2>&1 || fail "cannot remove the compose project $1"
	[ -z "$(as_rootless "docker ps -aq --filter label=com.docker.compose.project=$1")" ] || fail "the rootless engine still has $1"
	docker exec "$host" rm -rf "$2" || fail "cannot remove $2"
	if list_has_path "$2"; then fail "the folder $2 is still bound"; fi
}

# scenario_tui_engine_choice proves the choice of the engine in the interface. The interface of $admin without sudo adds
# a folder with a stack: the engine is asked after the inspection, the system engine is preselected because no
# container of the folder runs anywhere, the choice of the rootless engine inspects the folder again for it, the last
# question names the engine and shows the command through sudo with --engine, and the folder is bound in the
# instance of $rootless_user, which runs the stack, while the system engine neither lists the folder nor runs the
# stack. Then a proposal submitted for the instance is approved in the interface under sudo: the view and the
# question show the engine, the folder is added to the instance and the proposal is applied. Both folders and
# stacks are released and removed again.
scenario_tui_engine_choice() {
	tc_engine=rootless:$rootless_user
	tc_dir=/srv/qd-tui-engine
	tc_pdir=/srv/qd-tui-engine-prop
	tc_dropin=/etc/systemd/system/qhost-deploy@$rootless_user.service.d/10-stack-folders.conf
	tui_log=$work/tui-engine-choice-screens.log
	: >"$tui_log"

	step 'two folders with a stack each are prepared as root'
	docker exec "$host" sh -ec "rm -rf '$tc_dir' '$tc_pdir' && mkdir -p '$tc_dir/demo-tuiengine' '$tc_pdir/demo-tuiprop' &&
		chmod 0755 '$tc_dir' '$tc_pdir'" || fail 'cannot create the folders for the choice of the engine'
	printf 'name: demo-tuiengine\nservices:\n  app:\n    image: demo/app:1.0.0\n    restart: unless-stopped\n' |
		docker exec -i "$host" sh -ec "cat >'$tc_dir/demo-tuiengine/compose.yaml'" || fail 'cannot write the compose file of demo-tuiengine'
	printf 'name: demo-tuiprop\nservices:\n  app:\n    image: demo/app:1.0.0\n    restart: unless-stopped\n' |
		docker exec -i "$host" sh -ec "cat >'$tc_pdir/demo-tuiprop/compose.yaml'" || fail 'cannot write the compose file of demo-tuiprop'
	if list_has_path "$tc_dir" || list_has_path "$tc_pdir"; then fail 'a folder of the choice is bound before it'; fi
	for p in demo-tuiengine demo-tuiprop; do
		[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=$p)" ] || fail "the system engine already runs $p"
	done

	step "the interface without sudo asks for the engine after the inspection and inspects the folder again for $tc_engine"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'trap : INT; qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_wait "Engine $tc_engine" 30
	tui_send a
	tui_wait 'Add a stack folder' 10
	tui_type "$tc_dir"
	tui_send F2
	tui_wait 'whose deploy service runs the stacks' 60
	tui_excerpt 'Add a stack folder' 8
	if list_has_path "$tc_dir"; then fail 'the folder is bound at the question for the engine'; fi
	tui_send Right
	i=0
	until tui_has "$tc_engine"; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the choice does not reach $tc_engine: $(tui_screen)"
		sleep 0.25
	done
	tui_send F2
	tui_wait "Start a git repository in $tc_dir and take it over?" 60
	tui_has "engine: $tc_engine" || fail "the question does not show the engine: $(tui_screen)"
	# The command stands at the end of the sheet, below the window.
	for _ in 1 2 3 4 5 6 7 8 9 10; do tui_send Down; done
	tui_wait 'sudo qhost source add' 10
	tc_has_flat "--engine $tc_engine" || fail "the question does not show the command with the engine: $(tui_screen)"
	tui_excerpt 'Start a git repository' 20
	if list_has_path "$tc_dir" || docker exec "$host" test -e "$tc_dir/.git"; then fail 'the folder is bound or changed before the last answer'; fi

	step "yes binds the folder in the instance of $tc_engine and the stack runs in the rootless engine only"
	tui_send y
	tui_wait "The stack folder $tc_dir is added" 120
	tui_send Enter
	tui_tmux kill-server >/dev/null 2>&1 || true
	wait_for 'the deploy service to answer' 60 qd status
	tc_listed=$(qd --output json source list | jq -r --arg p "$tc_dir" '[.sources[] | select(.path == $p) | .engine] | join(",")')
	[ "$tc_listed" = "$tc_engine" ] || fail "source list does not show the folder once, with the engine $tc_engine, but as: $tc_listed"
	docker exec "$host" grep -qx "ReadWritePaths=-$tc_dir" "$tc_dropin" || fail "the drop-in of the instance does not list $tc_dir"
	if docker exec "$host" grep -qF "$tc_dir" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'the drop-in of the system service names the folder of the interface'
	fi
	wait_for 'the stack demo-tuiengine to run in the rootless engine' 180 tc_runs demo-tuiengine
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-tuiengine)" ] ||
		fail 'the system engine runs the stack the interface took over into the instance'
	log "scenario: the interface took $tc_dir over into $tc_engine; demo-tuiengine runs in the rootless engine only"

	step "a proposal for $tc_engine is approved in the interface under sudo with the engine shown"
	out=$(printf '{"path":"%s"}' "$tc_pdir" | as_admin "qhost --output json proposal submit --kind source-add --engine $tc_engine" 2>&1) ||
		fail "qhost proposal submit --engine $tc_engine failed: $out"
	tc_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$tc_id" ] || fail "qhost proposal submit printed no id: $out"
	prop_tui
	tui_send 6
	tui_wait 'stack folder' 15
	tui_send Enter
	tui_wait 'Effect if applied now' 30
	tui_wait "path: $tc_pdir" 10
	tui_has "$tc_id" || fail "the view does not show $tc_id: $(tui_screen)"
	tui_has "$tc_engine" || fail "the view does not show the engine $tc_engine: $(tui_screen)"
	tui_send A
	tui_wait "Approve proposal $tc_id and apply these changes to $tc_pdir?" 30
	tui_has "engine: $tc_engine" || fail "the approval does not show the engine: $(tui_screen)"
	[ "$(prop_state "$tc_id")" = open ] || fail 'the proposal left the state open before the last answer'
	if list_has_path "$tc_pdir"; then fail 'the folder is bound before the last answer'; fi
	tui_send y
	tui_wait "The stack folder $tc_pdir is added" 120
	tui_send Enter
	tui_tmux kill-server >/dev/null 2>&1 || true
	wait_for 'the proposal to be applied' 30 prop_is "$tc_id" applied
	tc_listed=$(qd --output json source list | jq -r --arg p "$tc_pdir" '[.sources[] | select(.path == $p) | .engine] | join(",")')
	[ "$tc_listed" = "$tc_engine" ] || fail "source list does not show the proposed folder once, with the engine $tc_engine, but as: $tc_listed"
	wait_for 'the stack demo-tuiprop to run in the rootless engine' 180 tc_runs demo-tuiprop
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-tuiprop)" ] ||
		fail 'the system engine runs the stack of the proposal'
	log "scenario: proposal $tc_id was approved in the interface and applied in $tc_engine; demo-tuiprop runs in the rootless engine only"

	step 'the folders are released and their stacks removed again'
	tc_release demo-tuiengine "$tc_dir"
	tc_release demo-tuiprop "$tc_pdir"
	wait_for 'the qhost service to answer' 30 qd status
}

# rl_docker runs the Docker client as $rootless_user against its rootless engine, without standard input.
rl_docker() {
	docker exec -i -u "$rootless_user" -e "XDG_RUNTIME_DIR=/run/user/$rootless_uid" \
		-e "DOCKER_HOST=unix:///run/user/$rootless_uid/docker.sock" "$host" docker "$@" </dev/null
}

# rl_fixture_run runs the script $2 in a helper container of the rootless engine with the folder $1 as /data.
rl_fixture_run() {
	rl_docker run --rm --network none -v "$1:/data" "$backup_image" sh -ec "$2"
}

# rl_fixture_snap is fixture_snap for the rootless engine: owners are those the container sees.
rl_fixture_snap() {
	rl_docker run --rm --network none -v "$1:/data:ro" "$backup_image" sh -ec '
		cd /data
		find . -printf "%U:%G:%m:%y %p -> %l\n" | sort -k2
		find . -type f -exec sha256sum {} + | sort -k2'
}

# bkr_field prints a field of demo-rootless in the status of the instance of $rootless_user.
bkr_field() {
	qd --output json status | jq -r --arg k "instance.$rootless_user.stack.demo-rootless.$1" '.[$k] // ""'
}

# bkr_head prints the head commit of the folder of the instance.
bkr_head() {
	docker exec "$host" git -c "safe.directory=$bk_base" -C "$bk_base" rev-parse HEAD
}

# bkr_settled succeeds when the instance has no run of demo-rootless in progress, has deployed the head and ended
# its last run as succeeded.
bkr_settled() {
	[ -z "$(bkr_field active_run)" ] && [ "$(bkr_field deployed)" = "$(bkr_head)" ] && [ "$(bkr_field last_run)" = succeeded ]
}

# bkr_restart restarts the instance, which makes it poll at once, and waits until it answers.
bkr_restart() {
	docker exec "$host" systemctl restart "$bk_unit" || fail "cannot restart $bk_unit"
	wait_for "$bk_unit to answer" 60 qd --output json stack settings demo-rootless
}

# bkr_newest_run prints the newest recorded run of demo-rootless (empty when there is none).
bkr_newest_run() {
	qd --output json stack runs demo-rootless --engine "$bk_engine" | jq -r 'map(.run) | sort | last // ""'
}

# bkr_next_run waits for a run of demo-rootless other than $1, active or ended, and prints its id.
bkr_next_run() {
	i=0
	while :; do
		id=$(bkr_field active_run)
		[ -n "$id" ] || id=$(bkr_newest_run)
		if [ -n "$id" ] && [ "$id" != "$1" ]; then
			printf '%s' "$id"
			return 0
		fi
		i=$((i + 1))
		[ "$i" -lt 400 ] || return 1
		sleep 0.3
	done
}

# bkr_finished succeeds when demo-rootless has no run in progress and the log of the run $1 has its result line.
bkr_finished() {
	[ -z "$(bkr_field active_run)" ] && qd stack log "$1" --engine "$bk_engine" | grep -q ' result '
}

# bkr_settings replaces the settings of demo-rootless with the YAML $1. A run of the stack that is active at that
# moment (the instance polls on its own) makes the call fail; it is repeated then.
bkr_settings() {
	i=0
	until bkr_out=$(qd_in "$1" --output json stack settings set demo-rootless --file -); do
		case $bkr_out in *'already active'*) ;; *) fail "settings set of demo-rootless failed: $bkr_out" ;; esac
		i=$((i + 1))
		[ "$i" -lt 180 ] || fail "settings set of demo-rootless still finds a run active: $bkr_out"
		sleep 1
	done
}

# bkr_backup_ids prints the ids of the backups of demo-rootless, newest first.
bkr_backup_ids() {
	qd stack backup list demo-rootless --engine "$bk_engine" | grep -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'
}

# bkr_healthy succeeds when a container of demo-rootless in the rootless engine is healthy.
bkr_healthy() {
	[ -n "$(rl_docker ps -q --filter label=com.docker.compose.project=demo-rootless --filter health=healthy)" ]
}

# bkr_dd_finding succeeds when the instance of $rootless_user shows the finding data-dir-missing for demo-rootless-dirs.
bkr_dd_finding() {
	qd --output json status | jq -r --arg k "instance.$rootless_user.stack.demo-rootless-dirs.findings" '.[$k] // ""' | grep -q data-dir-missing
}

bkr_dd_no_finding() {
	! bkr_dd_finding
}

# bkr_dd_healthy succeeds when a container of demo-rootless-dirs in the rootless engine is healthy.
bkr_dd_healthy() {
	[ -n "$(rl_docker ps -q --filter label=com.docker.compose.project=demo-rootless-dirs --filter health=healthy)" ]
}

# bkr_containers prints the ids of the running containers of demo-rootless in the rootless engine.
bkr_containers() {
	rl_docker ps -q --filter label=com.docker.compose.project=demo-rootless | sort
}

# scenario_backup_rootless proves backup, restore and rollback with data in the rootless engine of $rootless_user, for
# the stack demo-rootless that scenario_source_rootless bound to the instance. The instance gets the local helper image
# in its own image store and a drop-in that names it; the drop-in is removed at the end, because the uninstall
# removes the folder of the drop-ins only when it is empty. At the end the stack runs with the settings it had before,
# without backup rules.
scenario_backup_rootless() {
	bk_base=/srv/qd-rootless
	bk_dir=$bk_base/demo-rootless
	bk_data=$bk_dir/data
	bk_engine=rootless:$rootless_user
	bk_unit=qhost-deploy@$rootless_user.service
	bk_dropin=/etc/systemd/system/$bk_unit.d/20-e2e-backup-image.conf
	bk_store=/var/lib/qhost-deploy-$rootless_user/backups/demo-rootless
	bk_group=$(docker exec "$host" id -gn "$rootless_user") || fail "cannot read the group of $rootless_user"
	bk_sub=$(docker exec "$host" awk -F: -v u="$rootless_user" '$1 == u { print $2 }' /etc/subuid)
	[ -n "$bk_sub" ] || fail "$rootless_user has no sub-UIDs"

	step 'the helper image goes into the image store of the rootless engine and the instance names it in a drop-in'
	docker exec "$host" sh -ec "docker save -o /tmp/e2e-backup-helper.tar '$backup_image' && chmod 644 /tmp/e2e-backup-helper.tar" ||
		fail 'cannot save the helper image'
	rl_docker load -q -i /tmp/e2e-backup-helper.tar >/dev/null || fail 'the rootless engine cannot load the helper image'
	docker exec "$host" rm -f /tmp/e2e-backup-helper.tar
	rl_docker image inspect "$backup_image" >/dev/null || fail 'the rootless engine does not have the helper image'
	docker exec "$host" sh -ec "printf '[Service]\nExecStart=\nExecStart=/usr/local/bin/qhost deploy-service run --instance $rootless_user --backup-image $backup_image\n' >'$bk_dropin' &&
		systemctl daemon-reload" || fail 'cannot write the drop-in of the instance'
	docker exec "$host" test -f "/etc/systemd/system/$bk_unit.d/10-stack-folders.conf" || fail 'the drop-in of the stack folders is gone'
	bkr_restart
	wait_for 'the instance to be idle' 180 sr_idle

	step 'demo-rootless gets a data directory owned by the user of the instance, backup rules and a checked healthcheck'
	docker exec "$host" install -d -o "$rootless_user" -g "$bk_group" -m 0755 "$bk_data" || fail 'cannot create the data directory'
	cat <<'EOF2' | docker exec -i "$host" sh -ec "cat >'$bk_dir/compose.yaml'"
name: demo-rootless

services:
  app:
    image: demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_HEALTH: ok
    volumes:
      - ./data:/data
EOF2
	printf 'data/\n' | docker exec -i "$host" sh -ec "cat >'$bk_dir/.gitignore'" || fail 'cannot write the ignore file'
	docker exec "$host" sh -ec "chgrp '$bk_group' '$bk_dir/compose.yaml' '$bk_dir/.gitignore' && chmod 664 '$bk_dir/compose.yaml' '$bk_dir/.gitignore'" ||
		fail 'cannot give the group access to the stack files'
	as_rootless "cd '$bk_dir' && git -c safe.directory='$bk_base' add -A &&
		git -c safe.directory='$bk_base' -c user.name='E2E User' -c user.email=user.qhoste2e.test commit -q -m 'Give demo-rootless a data directory'" ||
		fail 'the user of the instance cannot commit the data directory'
	bkr_settings 'version: 1
backup:
  paths:
    - data
  retention:
    keep: 10
check:
  containers: true
'
	printf '%s' "$bkr_out" | jq -e '.changed == true' >/dev/null || fail "settings set reports no change: $bkr_out"
	bkr_restart
	wait_for 'the instance to deploy the head of demo-rootless' 240 bkr_settled
	sr_stack_runs || fail 'demo-rootless does not run in the rootless engine'
	log "scenario: demo-rootless deployed at $(bkr_head) with a data directory and backup rules"

	step 'the data of demo-rootless gets files with mixed owners; the container sees them as written'
	bk_mounts=$(rl_docker inspect -f '{{range .Mounts}}{{.Destination}}{{end}}' "$(bkr_containers | head -n 1)") || fail 'cannot inspect the container'
	[ "$bk_mounts" = /data ] || fail "the container of demo-rootless does not mount the data directory: $bk_mounts"
	rl_fixture_run "$bk_data" "$(fixture_write_script)" || fail 'cannot write the fixture into the data directory'
	bk_before=$(rl_fixture_snap "$bk_data") || fail 'cannot read the fixture'
	printf '%s\n' "$bk_before" | grep -q '^1234:1234:640:f ./a.txt' || fail "the fixture lacks the owner 1234: $bk_before"
	printf '%s\n' "$bk_before" | grep -q '^0:0:600:f ./r.txt' || fail "the fixture lacks the root file: $bk_before"
	[ "$(docker exec "$host" stat -c %u "$bk_data/a.txt")" -eq $((bk_sub + 1233)) ] ||
		fail "a.txt has the host owner $(docker exec "$host" stat -c %u "$bk_data/a.txt"), want $((bk_sub + 1233))"
	[ "$(docker exec "$host" stat -c %u "$bk_data/r.txt")" -eq "$rootless_uid" ] ||
		fail "r.txt has the host owner $(docker exec "$host" stat -c %u "$bk_data/r.txt"), want $rootless_uid"

	step 'backup create saves the data and the metadata name the user namespace of the engine'
	out=$(qd --output json stack backup create demo-rootless) || fail "backup create failed: $out"
	b1=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	[ -n "$b1" ] || fail "backup create reported no id: $out"
	printf '%s' "$out" | grep -q '"items": *1' || fail "the backup does not hold one item: $out"
	sr_stack_runs || fail 'demo-rootless does not run after the backup'
	[ -n "$(bkr_containers)" ] || fail 'demo-rootless has no container after the backup'
	bk_meta=$(docker exec "$host" cat "$bk_store/$b1/meta.json") || fail "cannot read the metadata of $b1"
	printf '%s' "$bk_meta" | jq -e --argjson uid "$rootless_uid" '.schema == 2 and .identity.engine == "rootless" and
		.identity.uid_map[0].inside == 0 and .identity.uid_map[0].outside == $uid and .identity.uid_map[0].count == 1 and
		(.identity.gid_map | length) >= 1' >/dev/null || fail "unexpected metadata of $b1: $bk_meta"
	[ "$(docker exec "$host" stat -c %a:%U "$bk_store/$b1/meta.json" "$bk_store/$b1/path-0.tar" | sort -u)" = "600:$rootless_user" ] ||
		fail "the backup is not private to the user of the instance: $(docker exec "$host" ls -ln "$bk_store/$b1")"
	log "scenario: backup $b1 of demo-rootless in the namespace $(printf '%s' "$bk_meta" | jq -c .identity.uid_map)"

	step 'the data changes and the restore brings back data and owners as the container sees them'
	rl_fixture_run "$bk_data" "cd /data; $(fixture_change_script)" || fail 'cannot change the data'
	bk_changed=$(rl_fixture_snap "$bk_data") || fail 'cannot read the changed data'
	[ "$bk_changed" != "$bk_before" ] || fail 'the change did not alter the data'
	out=$(qd stack backup restore demo-rootless "$b1" --yes) || fail "backup restore failed: $out"
	[ "$(rl_fixture_snap "$bk_data")" = "$bk_before" ] || fail "the data differs after the restore: $(rl_fixture_snap "$bk_data")"
	[ "$(docker exec "$host" stat -c %u "$bk_data/a.txt")" -eq $((bk_sub + 1233)) ] || fail 'a.txt has another host owner after the restore'
	sr_stack_runs || fail 'demo-rootless does not run after the restore'
	log "scenario: restored $b1; the data and owners are those of the backed-up state"

	step 'a backup with a foreign ID mapping is refused as a whole and changes nothing'
	out=$(qd --output json stack backup create demo-rootless) || fail "second backup failed: $out"
	b2=$(printf '%s\n' "$out" | sed -n 's/.*"backup": *"\([^"]*\)".*/\1/p')
	[ -n "$b2" ] && [ "$b2" != "$b1" ] || fail "the second backup has no new id: $out"
	bk_meta2=$bk_store/$b2/meta.json
	bk_orig=$(docker exec "$host" cat "$bk_meta2") || fail "cannot read the metadata of $b2"
	printf '%s' "$bk_orig" | jq '.identity.uid_map[0].outside += 1' | docker exec -i "$host" sh -ec "cat >'$bk_meta2'" ||
		fail "cannot change the metadata of $b2"
	rl_fixture_run "$bk_data" "cd /data; $(fixture_change_script)" || fail 'cannot change the data again'
	bk_changed=$(rl_fixture_snap "$bk_data") || fail 'cannot read the changed data'
	bk_ctr_before=$(bkr_containers)
	out=$(qd_fails stack backup restore demo-rootless "$b2" --yes) || fail "the restore of a foreign backup succeeded: $out"
	printf '%s' "$out" | grep -q 'belongs to another engine' || fail "the refusal does not say that the backup belongs to another engine: $out"
	printf '%s' "$out" | grep -q 'nothing was changed' || fail "the refusal does not say that nothing was changed: $out"
	[ "$(rl_fixture_snap "$bk_data")" = "$bk_changed" ] || fail 'the refused restore changed the data'
	[ "$(bkr_containers)" = "$bk_ctr_before" ] && sr_stack_runs || fail 'the refused restore stopped or replaced the container of demo-rootless'
	log "scenario: foreign backup refused: $out"
	printf '%s' "$bk_orig" | docker exec -i "$host" sh -ec "cat >'$bk_meta2'" || fail "cannot put the metadata of $b2 back"
	out=$(qd stack backup restore demo-rootless "$b2" --yes) || fail "the restore of $b2 with its own metadata failed: $out"
	[ "$(rl_fixture_snap "$bk_data")" = "$bk_before" ] || fail 'the restore with the original metadata did not bring the data back'

	step 'a failing update is rolled back with the data of the backup of its run'
	wait_for 'the instance to be idle' 180 sr_idle
	rl_fixture_run "$bk_data" "printf '%s\n' rootless-marker >/data/marker.txt && chown 4242:4242 /data/marker.txt && chmod 604 /data/marker.txt" ||
		fail 'cannot write the marker'
	bk_before=$(rl_fixture_snap "$bk_data") || fail 'cannot read the data before the run'
	printf '%s\n' "$bk_before" | grep -q '^4242:4242:604:f ./marker.txt' || fail "the marker is missing: $bk_before"
	bk_old=$(bkr_backup_ids | tr '\n' ' ')
	bk_good=$(bkr_head)
	prev=$(bkr_newest_run)
	as_rootless "cd '$bk_dir' && sed -i 's/DEMO_HEALTH: ok/DEMO_HEALTH: fail/' compose.yaml &&
		git -c safe.directory='$bk_base' -c user.name='E2E User' -c user.email=user.qhoste2e.test commit -q -am 'Break the healthcheck of demo-rootless'" ||
		fail 'the user of the instance cannot commit the failing version'
	bk_bad=$(bkr_head)
	[ "$bk_bad" != "$bk_good" ] || fail 'the failing version has no commit'
	bkr_restart
	run=$(bkr_next_run "$prev") || fail 'no run of demo-rootless started for the failing commit'
	# Once the run has its backup, the data changes under the new version; only a restore can undo that.
	i=0
	until qd stack log "$run" --engine "$bk_engine" | grep -q ' info backup [0-9]\{8\}T'; do
		i=$((i + 1))
		[ "$i" -lt 600 ] || fail "the run $run made no backup"
		sleep 0.2
	done
	rl_docker run --rm --network none -v "$bk_data:/data" "$backup_image" sh -ec "cd /data; $(fixture_change_script)" ||
		fail 'cannot change the data during the run'
	wait_for "the run $run" 300 bkr_finished "$run"
	run_log=$(qd stack log "$run" --engine "$bk_engine")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	run_backups=$(printf '%s\n' "$run_log" | sed -n 's/^[^ ]* info backup \([0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}\)$/\1/p')
	[ "$(printf '%s\n' "$run_backups" | wc -l)" = 2 ] && [ "$(printf '%s\n' "$run_backups" | sort -u | wc -l)" = 1 ] ||
		fail "the backup and the restore do not name one backup: $run_backups"
	run_backup=$(printf '%s\n' "$run_backups" | head -n 1)
	case " $bk_old " in *" $run_backup "*) fail "the run restored an older backup: $run_backup" ;; esac
	[ "$(rl_fixture_snap "$bk_data")" = "$bk_before" ] || fail "the data differs from the backed-up state after the rollback: $(rl_fixture_snap "$bk_data")"
	[ "$(docker exec "$host" cat "$bk_data/marker.txt")" = rootless-marker ] || fail 'the marker of the run backup is not back'
	revert=$(bkr_head)
	[ "$revert" != "$bk_bad" ] || fail 'no revert commit was made'
	docker exec "$host" grep -q 'DEMO_HEALTH: ok' "$bk_dir/compose.yaml" || fail 'the revert did not restore the compose file'
	[ "$(bkr_field deployed)" = "$revert" ] || fail "demo-rootless is not recorded as deployed at the revert commit $revert"
	wait_for 'demo-rootless to run healthy again' 60 bkr_healthy
	sr_stack_runs || fail 'demo-rootless does not run its previous image after the rollback'
	log "scenario: run $run of demo-rootless rolled back; restored $run_backup; data identical to the backed-up state; revert $revert"

	step 'a data directory for container user 1234 in the rootless engine gets the host owner the user ID mapping gives, with nested parents'
	bk_sub_gid=$(docker exec "$host" awk -F: -v u="$rootless_user" '$1 == u { print $2 }' /etc/subgid)
	[ -n "$bk_sub_gid" ] || fail "$rootless_user has no sub-GIDs"
	bk_dd=demo-rootless-dirs
	bk_ddir=$bk_base/$bk_dd
	bk_dd_owner="$((bk_sub + 1233)):$((bk_sub_gid + 1233))"
	docker exec "$host" install -d -o "$rootless_user" -g "$bk_group" -m 0775 "$bk_ddir" || fail "cannot create the folder of $bk_dd"
	cat <<'EOF2' | docker exec -i "$host" sh -ec "cat >'$bk_ddir/compose.yaml'"
name: demo-rootless-dirs

services:
  app:
    image: demo/app:1.0.0
    user: "1234"
    restart: unless-stopped
    volumes:
      - ./data/db:/data
EOF2
	printf 'data/\n' | docker exec -i "$host" sh -ec "cat >'$bk_ddir/.gitignore'" || fail "cannot write the ignore file of $bk_dd"
	docker exec "$host" sh -ec "chgrp '$bk_group' '$bk_ddir/compose.yaml' '$bk_ddir/.gitignore' && chmod 664 '$bk_ddir/compose.yaml' '$bk_ddir/.gitignore'" ||
		fail "cannot give the group access to the files of $bk_dd"
	as_rootless "cd '$bk_base' && git -c safe.directory='$bk_base' add -A &&
		git -c safe.directory='$bk_base' -c user.name='E2E User' -c user.email=user.qhoste2e.test commit -q -m 'Add demo-rootless-dirs'" ||
		fail "the user of the instance cannot commit $bk_dd"
	bkr_restart
	wait_for "the finding data-dir-missing of $bk_dd in the instance" 240 bkr_dd_finding
	docker exec "$host" test ! -e "$bk_ddir/data" || fail "the data directory of $bk_dd exists before it was created"
	out=$(qd_fails stack datadir "$bk_dd" --engine "$bk_engine") || fail "datadir without --yes did not stop: $out"
	printf '%s' "$out" | grep -qF "data/db (container user 1234:1234, host owner $bk_dd_owner)" ||
		fail "the command does not show the container user and the host owner $bk_dd_owner: $out"
	docker exec "$host" test ! -e "$bk_ddir/data" || fail "the command without --yes created the directories of $bk_dd"
	out=$(qd stack datadir "$bk_dd" --engine "$bk_engine" --yes) || fail "datadir of $bk_dd failed: $out"
	[ "$(docker exec "$host" stat -c %u:%g "$bk_ddir/data/db")" = "$bk_dd_owner" ] ||
		fail "data/db has the owner $(docker exec "$host" stat -c %u:%g "$bk_ddir/data/db"), want $bk_dd_owner"
	[ "$(docker exec "$host" stat -c %u "$bk_ddir/data")" -eq "$rootless_uid" ] ||
		fail "the parent data has the owner $(docker exec "$host" stat -c %u "$bk_ddir/data"), want $rootless_uid"
	bk_dd_mode=$(docker exec "$host" stat -c %a "$bk_ddir/data")
	[ $((0$bk_dd_mode & 0110)) -eq $((0110)) ] || fail "the parent data is not searchable for its owner and group: $bk_dd_mode"
	wait_for "$bk_dd to run healthy in the rootless engine" 240 bkr_dd_healthy
	rl_docker run --rm --network none --user 1234 -v "$bk_ddir/data/db:/data" "$backup_image" sh -ec 'echo written >/data/probe && cat /data/probe' |
		grep -qx written || fail 'container user 1234 cannot write into the data directory'
	[ "$(docker exec "$host" stat -c %u "$bk_ddir/data/db/probe")" -eq $((bk_sub + 1233)) ] ||
		fail "the file of container user 1234 has the host owner $(docker exec "$host" stat -c %u "$bk_ddir/data/db/probe")"
	wait_for "the finding of $bk_dd to clear" 120 bkr_dd_no_finding
	log "scenario: $bk_dd got data/db for container user 1234 as host owner $bk_dd_owner, parent data owned by the instance user, and runs healthy"

	step "$bk_dd leaves the instance again"
	as_rootless "cd '$bk_base' && git -c safe.directory='$bk_base' rm -r -q '$bk_dd' &&
		git -c safe.directory='$bk_base' -c user.name='E2E User' -c user.email=user.qhoste2e.test commit -q -m 'Remove demo-rootless-dirs'" ||
		fail "the user of the instance cannot remove $bk_dd"
	bkr_restart
	as_rootless "docker ps -aq --filter label=com.docker.compose.project=$bk_dd | xargs -r docker rm -f" >/dev/null ||
		fail "cannot remove the containers of $bk_dd"
	docker exec "$host" rm -rf "$bk_ddir" || fail "cannot remove the folder of $bk_dd"
	wait_for 'the instance to be idle' 180 sr_idle
	sr_stack_runs || fail 'demo-rootless does not run after demo-rootless-dirs left'

	step 'the drop-in of the helper image is removed again and the instance answers'
	bkr_settings 'version: 1
check:
  containers: false
'
	printf '%s' "$bkr_out" | jq -e '.changed == true' >/dev/null || fail "settings set reports no change: $bkr_out"
	docker exec "$host" sh -ec "rm -f '$bk_dropin' && systemctl daemon-reload" || fail 'cannot remove the drop-in of the instance'
	docker exec "$host" test -f "/etc/systemd/system/$bk_unit.d/10-stack-folders.conf" || fail 'the drop-in of the stack folders is gone'
	bkr_restart
	wait_for 'the instance to be idle' 180 sr_idle
	sr_stack_runs || fail 'demo-rootless does not run after the drop-in is removed'
	qd --output json stack settings demo-rootless | jq -e '.configured == true and .settings.backup == null' >/dev/null ||
		fail "demo-rootless still has backup rules: $(qd --output json stack settings demo-rootless)"
}

# te_new_run succeeds when the instance lists a run of demo-rootless that is not in $te_old, and sets te_run to it.
te_new_run() {
	te_run=$(qd --output json stack runs demo-rootless --engine "$te_engine" |
		jq -r --argjson o "$te_old" '[.[].run | select(. as $r | $o | index($r) | not)][0] // empty') && [ -n "$te_run" ]
}

# scenario_tui_engine proves the Stacks area with two engines for the interface of $admin started without sudo: the
# stack demo-rootless stands in the section of its engine, and a deploy from there asks for the command with
# --engine, runs it through sudo and acts in the instance of $rootless_user: the run is recorded there and not in the
# system service, and the stack runs in the rootless engine only.
scenario_tui_engine() {
	te_engine=rootless:$rootless_user

	step 'the Stacks area shows demo-rootless in the section of the rootless engine'
	wait_for 'the instance to be idle' 180 sr_idle
	runs_before=$(qd --output json stack runs --engine system | jq 'length')
	te_old=$(qd --output json stack runs demo-rootless --engine "$te_engine" | jq -c '[.[].run]')
	tui_log=$work/tui-engine-screens.log
	: >"$tui_log"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait "Engine $te_engine" 30
	tui_wait 'demo-rootless' 30
	tui_send Home
	tui_cursor_to demo-rootless
	tui_excerpt 'demo-rootless' 12

	step 'a deploy from the section runs through sudo in the instance'
	tui_send d
	tui_wait 'Deploy demo-rootless?' 15
	# The dialog may break the command after --engine.
	tui_has 'sudo qhost stack deploy --output json --engine' && tui_has "$te_engine -- demo-rootless" ||
		fail "the question does not show the command with the engine: $(tui_screen)"
	tui_send y
	tui_wait 'deploy finished: 1 ok, 0 failed' 240
	tui_send Escape
	tui_tmux kill-server >/dev/null 2>&1 || true
	# A run is listed once it has ended.
	wait_for 'the run of the deploy in the instance' 240 te_new_run
	wait_for "the run $te_run of demo-rootless to succeed in the instance" 180 sr_run_done "$te_run"
	[ "$(qd --output json stack runs --engine system | jq 'length')" = "$runs_before" ] || fail 'the deploy of the interface changed the runs of the system service'
	sr_stack_runs || fail 'demo-rootless does not run in the rootless engine after the deploy of the interface'
	[ -z "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-rootless)" ] ||
		fail 'the deploy of the interface started demo-rootless in the system engine'
	log "scenario: the interface deployed demo-rootless from its engine section, run $te_run in the instance"
}

# tc_cursor_engine prints the engine of the section whose marked row is $1, or nothing.
tc_cursor_engine() {
	tui_screen | awk -v name="$1" '
		/ Engine / { match($0, /Engine [^ ]+/); eng = substr($0, RSTART + 7, RLENGTH - 7) }
		/^(│ |║ )?> / { if (index($0, " " name " ")) print eng }'
}

# tc_cursor_in_engine tells whether the marked row of the screen on its input is $tc_ct in the section of $tc_engine.
tc_cursor_in_engine() {
	[ "$(awk -v name="$tc_ct" '
		/ Engine / { match($0, /Engine [^ ]+/); eng = substr($0, RSTART + 7, RLENGTH - 7) }
		/^(│ |║ )?> / { if (index($0, " " name " ")) print eng }')" = "$tc_engine" ]
}

# scenario_tui_containers_engine proves the Containers area with two engines for the interface of $admin started
# without sudo: the container of demo-rootless stands in the section of the rootless engine, a container of the same
# name in the system engine in the other, and a restart of the marked row goes through sudo with --engine and
# restarts the container of the rootless engine only.
scenario_tui_containers_engine() {
	tc_engine=rootless:$rootless_user
	tc_ct=$(as_rootless "docker ps --filter label=com.docker.compose.project=demo-rootless --format '{{.Names}}'" | head -n 1)
	[ -n "$tc_ct" ] || fail 'no container of demo-rootless runs in the rootless engine'

	step 'the Containers area shows the container of the rootless engine in its section, beside a container of the same name in the system engine'
	docker exec "$host" docker rm -f "$tc_ct" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$tc_ct" "registry.$domain/demo/app:1.0.0" >/dev/null ||
		fail "cannot start a container called $tc_ct in the system engine"
	tc_started=$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $tc_ct")
	tc_sys_started=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$tc_ct")
	tui_log=$work/tui-containers-engine-screens.log
	: >"$tui_log"
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 5
	tui_wait "5 $tui_icon_containers Containers" 10
	tui_wait "Engine $tc_engine" 30
	tui_wait 'Engine system' 30
	tui_send Home
	tui_walk_down "$tc_ct in $tc_engine" tc_cursor_in_engine
	tui_excerpt "Engine $tc_engine" 12

	step 'a restart from the section runs through sudo with --engine and restarts the container of that engine only'
	tui_send Z
	tui_wait "Restart container $tc_ct" 15
	# The dialog may break the command after --engine.
	tui_has 'sudo qhost container restart --yes --output json --engine' && tui_has "$tc_engine --" ||
		fail "the question does not show the command with the engine: $(tui_screen)"
	tui_send y
	tui_wait 'restart finished: 1 ok, 0 failed' 120
	tui_send Escape
	tui_tmux kill-server >/dev/null 2>&1 || true
	[ "$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $tc_ct")" != "$tc_started" ] ||
		fail "the restart of the interface did not restart $tc_ct in $tc_engine"
	[ "$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$tc_ct")" = "$tc_sys_started" ] ||
		fail "the restart of the interface restarted the container $tc_ct of the system engine"
	docker exec "$host" docker rm -f "$tc_ct" >/dev/null || fail "cannot remove $tc_ct from the system engine"
	log "scenario: the interface restarted $tc_ct in $tc_engine from its section and left the container of that name in the system engine alone"
}
