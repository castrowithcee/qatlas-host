#!/bin/sh
# Section tui-proposals: the interface takes over a foreign Compose project and approves proposals of every kind.
# Needs deploy: /opt/stacks with the settings of demo-stack; the choice of the engine in scenario_adopt needs the
# rootless instance, which deploy needs.
# Sourced by run.sh; defines only functions and constants.

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
	tui_adopt_own
	tui_keep_engine 30
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
	pk_days=$(window_field days)
	pk_start=$(window_field start)
	pk_duration=$(window_field duration)
	pk_zone=$(window_field zone)
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
	tui_wait 'The maintenance window of the host would change:' 10
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
	out=$(qd notify remove prop-hook --yes) || fail "notify remove prop-hook failed: $out"
	if qd --output json notify list | grep -q '"prop-hook"'; then fail 'the channel prop-hook remains'; fi
	log "scenario: the proposals of a stack's settings, the window and a webhook channel were approved in the interface and applied; settings and window are back, the channel is removed"
}
