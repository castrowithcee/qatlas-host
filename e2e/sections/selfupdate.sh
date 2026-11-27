#!/bin/sh
# Section selfupdate: the webhook receiver is set up with sudo qhost hook-service setup: it runs as qhost-hook
# without the groups docker and qhost, its marker folder has the owner and group of the contract, status shows it
# with its address and an unknown secret gets 404. sudo qhost hook add gives demo-stack a secret path that is shown
# once; a POST to it updates the stack within 30 seconds, a wrong or a removed secret gets 404. Then the installed
# program updates itself to a newer signed test release, which replaces and restarts the services, runs the transition
# of the new program once, renews the stale unit of the receiver and the changed template of the rootless instance,
# restarts the instance with the new program and warns about an instance unit an administrator overrides, refuses
# tampered ones, and updates once more with the
# deploy service put aside, which leaves its unit uninstalled.
# Needs deploy: the running rootless instance and the bound /opt/stacks with demo-stack for the webhook.
# Sourced by run.sh; defines only functions and constants.

# hook_listening succeeds when the webhook receiver is active and answers an HTTP request on its local address.
hook_listening() {
	[ "$(docker exec "$host" systemctl is-active qhost-hook)" = active ] &&
		docker exec "$host" curl -s -o /dev/null http://127.0.0.1:8480/
}

# scenario_hook_setup sets up the webhook receiver with sudo qhost hook-service setup, while the deploy service is
# set up, and checks what the contract says: no change without --confirm, the user qhost-hook in no other group, the
# process without the groups docker and qhost, the marker folder of the system engine, the restart of the running
# deploy service whose folder the setup creates and no restart on a repeated setup, the status without sudo and
# the answer 404 to an unknown secret. scenario_selfupdate and scenario_uninstall keep it in their checks.
# scenario_hook follows it and proves hook add, list and remove from a POST to the printed path to the update.
scenario_hook_setup() {
	step 'the setup of the webhook receiver needs root and --confirm and changes nothing without them'
	if out=$(as_admin 'qhost hook-service setup --confirm --yes 2>&1'); then fail "the receiver setup without root succeeded: $out"; fi
	if out=$(as_admin 'sudo qhost hook-service setup --yes 2>&1'); then fail "the receiver setup without --confirm succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'systemctl enable --now qhost-hook' || fail "the receiver setup without --confirm does not show its commands: $out"
	printf '%s' "$out" | grep -q 'systemctl try-restart qhost-deploy$' || fail "the receiver setup without --confirm does not show the restart of the deploy service: $out"
	if docker exec "$host" test -e /usr/local/lib/systemd/system/qhost-hook.service; then fail 'a refused receiver setup installed the unit'; fi
	if docker exec "$host" id qhost-hook >/dev/null 2>&1; then fail 'a refused receiver setup created the user qhost-hook'; fi
	as_admin 'qhost --output json status' | grep -q '"hook_service": *"not set up"' ||
		fail "qhost status does not show the receiver as not set up: $(as_admin 'qhost --output json status 2>&1')"

	step 'sudo qhost hook-service setup --confirm --yes sets up the receiver, which runs as qhost-hook without the groups docker and qhost, and restarts the running deploy service'
	pid=$(service_pid)
	out=$(as_admin 'sudo qhost hook-service setup --confirm --yes 2>&1') || fail "the receiver setup failed: $out"
	[ "$(service_pid)" != "$pid" ] || fail 'the receiver setup did not restart the running deploy service, which cannot write its new marker folder'
	wait_service
	pid=$(service_pid)
	wait_for 'the webhook receiver to answer' 30 hook_listening
	hkpid=$(service_pid qhost-hook)
	[ -n "$hkpid" ] && [ "$hkpid" != 0 ] || fail 'the webhook receiver has no process'
	[ "$(docker exec "$host" stat -c %U "/proc/$hkpid")" = qhost-hook ] || fail "the receiver does not run as qhost-hook: $(docker exec "$host" stat -c %U "/proc/$hkpid")"
	if docker exec "$host" grep '^Uid:' "/proc/$hkpid/status" | grep -qw 0; then fail 'the receiver has a root user ID'; fi
	[ "$(docker exec "$host" id -nG qhost-hook)" = qhost-hook ] || fail "the user qhost-hook has other groups: $(docker exec "$host" id -nG qhost-hook)"
	for g in docker qhost; do
		gid=$(docker exec "$host" getent group "$g" | cut -d : -f 3)
		if docker exec "$host" grep '^Groups:' "/proc/$hkpid/status" | tr ' \t' '\n\n' | grep -qx "$gid"; then fail "the receiver process holds the group $g"; fi
		if docker exec "$host" grep '^Gid:' "/proc/$hkpid/status" | tr ' \t' '\n\n' | grep -qx "$gid"; then fail "the receiver process has the group $g as its own"; fi
	done
	log "scenario: qhost-hook runs as pid $hkpid: $(docker exec "$host" ps -o user=,group=,supgrp= -p "$hkpid" | tr -s ' ')"
	[ "$(docker exec "$host" stat -c %U:%G:%a /run/qhost-hook/system)" = qhost-hook:qhost-hook:770 ] ||
		fail "/run/qhost-hook/system is $(docker exec "$host" stat -c %U:%G:%a /run/qhost-hook/system), want qhost-hook:qhost-hook:770"
	[ "$(docker exec "$host" stat -c %U:%G:%a /etc/qhost-hook/config.json)" = root:qhost-hook:644 ] ||
		fail "config.json is $(docker exec "$host" stat -c %U:%G:%a /etc/qhost-hook/config.json), want root:qhost-hook:644"
	out=$(as_admin 'sudo qhost --output json hook-service setup --confirm --yes 2>&1') || fail "the repeated receiver setup failed: $out"
	printf '%s' "$out" | grep -q '"changed": *false' || fail "the repeated receiver setup reported a change: $out"
	[ "$(service_pid qhost-hook)" = "$hkpid" ] || fail 'the repeated receiver setup restarted the receiver'
	[ "$(service_pid)" = "$pid" ] || fail 'the repeated receiver setup restarted the deploy service'

	step 'status without sudo shows the receiver running with its address and its hooks, and an unknown secret gets 404'
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status failed: $out"
	for want in '"hook_service": *"running"' '"hook_listen": *"127.0.0.1:8480"' '"hook_count": *0'; do
		printf '%s' "$out" | grep -q "$want" || fail "qhost status does not show $want: $out"
	done
	out=$(docker exec "$host" curl -sS -o /dev/null -w '%{http_code}' -X POST http://127.0.0.1:8480/hook/invented-unknown-secret) || fail 'the receiver does not answer a POST'
	[ "$out" = 404 ] || fail "an unknown secret got the status $out, want 404"
	log "scenario: webhook receiver set up on 127.0.0.1:8480, status shows it, an unknown secret gets 404"
}

# hook_accepts succeeds when the receiver answers 202 to a POST without content on the path $1. The receiver reads
# its configuration every ten seconds, so a new hook is accepted only after the next reload.
hook_accepts() {
	[ "$(docker exec "$host" curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:8480$1")" = 202 ]
}

# hook_rejects succeeds when the receiver answers 404 to a POST without content on the path $1.
hook_rejects() {
	[ "$(docker exec "$host" curl -sS -o /dev/null -w '%{http_code}' -X POST "http://127.0.0.1:8480$1")" = 404 ]
}

# scenario_hook proves sudo qhost hook add, list and remove with the receiver of scenario_hook_setup. demo-stack is
# reset to 1.0.1 in automatic mode with the update trigger window, which makes hook add warn. With the trigger webhook
# a POST to the printed path makes the service update it to 1.1.0 within 30 seconds; a wrong secret gets 404 and starts
# no run; after hook remove the old secret gets 404. The secret is shown once and never stored.
scenario_hook() {
	image=registry.$domain/demo/app
	step 'demo-stack is at 1.0.1 in automatic mode with the update trigger window'
	if [ "$(stack_images demo-stack)" != "$image:1.0.1" ]; then
		# The fresh clone of scenario_tui_setup has no secrets.env for demo-stack, whose compose file reads DEMO_SECRET.
		docker exec "$host" sh -ec "test -e /opt/stacks/demo-stack/secrets.env ||
			{ umask 077 && printf 'DEMO_SECRET=not-a-real-secret-e2e\n' >/opt/stacks/demo-stack/secrets.env; }" ||
			fail 'cannot write secrets.env of demo-stack'
		prev=$(newest_run demo-stack)
		admin_commit demo-stack/.env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.1/' 'Reset demo-stack to 1.0.1 for the webhook'
		restart_service
		run=$(next_run demo-stack "$prev") || fail 'no run of demo-stack started for the reset to 1.0.1'
		wait_for "the run $run" 240 run_finished demo-stack "$run"
		qd stack log "$run" | grep -q ' result succeeded' || fail "the reset of demo-stack to 1.0.1 did not succeed: $(qd stack log "$run")"
	fi
	settings_set demo-stack "$(demo_stack_settings 'update:\n  mode: automatic\n')"
	[ "$(stack_images demo-stack)" = "$image:1.0.1" ] || fail "demo-stack does not run 1.0.1: $(stack_images demo-stack)"

	step 'hook add needs root, the receiver and a known stack, and a failed call changes nothing'
	if out=$(as_admin 'qhost hook add demo-stack 2>&1'); then fail "hook add without root succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'needs root' || fail "the refusal of hook add without root is unclear: $out"
	if out=$(qd hook add no-such-stack); then fail "hook add for an unknown stack succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'no such stack' || fail "the refusal for an unknown stack is unclear: $out"
	as_admin 'qhost --output json status 2>&1' | grep -q '"hook_count": *0' || fail "the refusals added a hook: $(as_admin 'qhost --output json status 2>&1')"

	step 'with the trigger window, hook add stores the hook, shows the path once and warns that it does not act yet'
	out=$(qd --output json hook add demo-stack) || fail "hook add failed: $out"
	printf '%s' "$out" | grep -q 'warning: .*trigger window' || fail "hook add does not warn about the trigger window: $out"
	printf '%s' "$out" | grep -q 'shown only once' || fail "hook add does not say that the path is shown once: $out"
	hook_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(h-[0-9a-f]\{8\}\)".*/\1/p')
	hook_path=$(printf '%s' "$out" | sed -n 's/.*"path": *"\(\/hook\/[A-Za-z0-9_-]\{43\}\)".*/\1/p')
	[ -n "$hook_id" ] && [ -n "$hook_path" ] || fail "hook add printed no id or path: $out"
	hook_secret=${hook_path#/hook/}
	printf '%s' "$out" | grep -q '"listen": *"127.0.0.1:8480"' || fail "hook add does not print the address of the receiver: $out"
	[ "$(docker exec "$host" stat -c %U:%G:%a /etc/qhost-hook/config.json)" = root:qhost-hook:644 ] ||
		fail "config.json is $(docker exec "$host" stat -c %U:%G:%a /etc/qhost-hook/config.json) after hook add, want root:qhost-hook:644"
	docker exec "$host" cat /etc/qhost-hook/config.json | grep -q "\"id\": *\"$hook_id\"" || fail 'the configuration does not hold the new hook'
	if docker exec "$host" grep -rqF -- "$hook_secret" /etc/qhost-hook; then fail 'the secret is stored in the configuration'; fi
	if docker exec "$host" journalctl --no-pager -u qhost -u qhost-deploy -u qhost-hook | grep -qF -- "$hook_secret"; then
		fail 'the secret shows in the logs of the services'
	fi

	step 'hook list shows the hook without sudo and never a hash or a secret'
	out=$(as_admin 'qhost hook list 2>&1') || fail "hook list without sudo failed: $out"
	printf '%s' "$out" | grep -q "$hook_id *demo-stack *system" || fail "hook list does not show the hook: $out"
	if printf '%s' "$out" | grep -qF -e "$hook_secret" -e sha256; then fail "hook list shows a hash or a secret: $out"; fi
	out=$(as_admin 'qhost --output json hook list 2>&1') || fail "hook list as JSON failed: $out"
	for want in "\"id\": *\"$hook_id\"" '"stack": *"demo-stack"' '"instance": *"system"' '"created": *"20'; do
		printf '%s' "$out" | grep -q "$want" || fail "hook list as JSON lacks $want: $out"
	done
	if printf '%s' "$out" | grep -qF -e "$hook_secret" -e sha256; then fail "hook list as JSON shows a hash or a secret: $out"; fi

	step 'with the trigger webhook, a POST to the printed path makes the service update demo-stack within 30 seconds'
	settings_set demo-stack "$(demo_stack_settings 'update:\n  mode: automatic\n  trigger: webhook\n')"
	runs_before=$(stack_run_count demo-stack)
	# The wrong secret is tried first; the first accepted call then also waits for the receiver to read the new hook.
	wrong=$(printf 'A%.0s' $(seq 43))
	hook_rejects "/hook/$wrong" || fail 'a wrong secret was not answered with 404'
	since=$(date +%s)
	wait_for 'the receiver to accept the new hook' 30 hook_accepts "$hook_path"
	wait_for 'the webhook to update demo-stack' 30 interval_updated 1.1.0
	took=$(($(date +%s) - since))
	[ "$(stack_run_count demo-stack)" = "$((runs_before + 1))" ] || fail 'the webhook did not make exactly one run of demo-stack'
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the webhook update'
	log "scenario: a POST to the hook of demo-stack updated it to 1.1.0 in $took seconds"

	step 'a wrong secret gets 404 and starts no run'
	runs_before=$(stack_run_count demo-stack)
	hook_rejects "/hook/$wrong" || fail 'a wrong secret was not answered with 404'
	# The receiver leaves a marker only after the 404 branch, and the deploy service takes markers on every pass over
	# its folder (every two seconds), which also removes a foreign entry: once the probe is gone, a pass has run.
	docker exec "$host" touch /run/qhost-hook/system/Probe || fail 'cannot lay a probe in the marker folder'
	wait_for 'the service to pass over the marker folder' 30 docker exec "$host" test ! -e /run/qhost-hook/system/Probe
	[ "$(stack_run_count demo-stack)" = "$runs_before" ] || fail 'a wrong secret started a run'
	[ -z "$(stack_field demo-stack active_run)" ] || fail 'a wrong secret started a run in progress'

	step 'hook remove needs root, refuses an unknown id and then the old secret gets 404'
	if out=$(as_admin "qhost hook remove $hook_id --yes 2>&1"); then fail "hook remove without root succeeded: $out"; fi
	if out=$(qd hook remove h-00000000 --yes); then fail "hook remove of an unknown id succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'no such webhook' || fail "the refusal of an unknown id is unclear: $out"
	out=$(qd hook remove "$hook_id" --yes) || fail "hook remove failed: $out"
	qd hook list | grep -q "$hook_id" && fail 'the removed hook is still listed'
	wait_for 'the removed secret to get 404' 30 hook_rejects "$hook_path"
	as_admin 'qhost --output json status 2>&1' | grep -q '"hook_count": *0' || fail "the hook count is not 0 after the removal: $(as_admin 'qhost --output json status 2>&1')"
	log 'scenario: hook add, list and remove proved; the removed secret gets 404'

	step 'demo-stack waits for the window again'
	settings_set demo-stack "$(demo_stack_settings)"
}

# selfupdate_field prints one top-level field of qhost update status, run without root.
selfupdate_field() {
	as_admin "qhost --output json update status 2>&1" | sed -n "s/.*\"$1\": *\"\{0,1\}\([^\",}]*\).*/\1/p"
}

# service_entered prints when the unit $1 (the deploy service without an argument) last became active.
service_entered() {
	docker exec "$host" systemctl show -p ActiveEnterTimestampMonotonic --value "${1:-qhost-deploy}"
}

# unit_is_release succeeds when the installed unit $1 equals the one in the archive of the release $2.
unit_is_release() {
	arch=$(docker exec "$host" dpkg --print-architecture)
	tar -xzOf "$work/releases/$2/qhost_${2}_linux_$arch.tar.gz" "lib/systemd/system/$1.service" |
		docker exec -i "$host" cmp -s - "/usr/local/lib/systemd/system/$1.service"
}

# hook_unit_is_current succeeds when the installed unit of the webhook receiver equals the one in the sources, which
# the program under test carries.
hook_unit_is_current() {
	docker exec -i "$host" cmp -s - /usr/local/lib/systemd/system/qhost-hook.service <"$root/packaging/qhost-hook.service"
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

	scenario_hook_setup

	scenario_hook

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
	if out=$(as_admin 'sudo qhost update --yes 2>&1'); then
		fail "update without --confirm succeeded: $out"
	else
		rc=$?
		[ "$rc" -eq 2 ] || fail "update without --confirm ended with the code $rc, want 2: $out"
	fi
	printf '%s' "$out" | grep -q 'Update qhost from v0.1.0' || fail "update without --confirm does not show its plan: $out"
	printf '%s' "$out" | grep -q 'qhost, qhost-deploy' || fail "update without --confirm does not name the services it restarts: $out"
	printf '%s' "$out" | grep -q 'qhost-hook.service is renewed' || fail "update without --confirm does not name the webhook receiver: $out"
	printf '%s' "$out" | grep -q -- '--confirm' || fail "update without --confirm does not name the flag: $out"
	[ "$(installed_sum)" = "$before" ] || fail 'a refused update changed the installation'

	step 'sudo qhost update installs the newer release, replaces both units, restarts both services, renews the template of the instance, restarts the instance and warns about its overriding unit'
	# The units of v0.2.0 equal the installed ones, so both are made stale first; only a replacement removes the mark.
	docker exec "$host" sh -ec 'for u in qhost qhost-deploy; do printf "# stale\n" >>/usr/local/lib/systemd/system/$u.service; done; systemctl daemon-reload' ||
		fail 'cannot mark the installed units'
	if unit_is_release qhost v0.2.0 || unit_is_release qhost-deploy v0.2.0; then fail 'the installed units were not made stale'; fi
	# The unit of the webhook receiver is not in the archive: the new program replaces it from its own copy.
	docker exec "$host" sh -ec 'printf "# stale\n" >>/usr/local/lib/systemd/system/qhost-hook.service; systemctl daemon-reload' || fail 'cannot mark the unit of the receiver'
	if hook_unit_is_current; then fail 'the unit of the receiver was not made stale'; fi
	hkpid=$(service_pid qhost-hook) hkentered=$(service_entered qhost-hook)
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
	wait_for 'the webhook receiver after the update' 30 hook_listening
	hook_unit_is_current || fail 'the update did not replace the stale unit of the webhook receiver'
	[ "$(service_pid qhost-hook)" != "$hkpid" ] && [ "$(service_entered qhost-hook)" != "$hkentered" ] || fail 'the webhook receiver was not restarted by the update'
	if printf '%s' "$out" | grep -q 'webhook receiver was not renewed'; then fail "the renewal of the webhook receiver failed: $out"; fi
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
	printf '%s' "$out" | grep -q 'transition e2e-marker: applied' || fail "the update did not run the transition of v0.2.0: $out"
	if printf '%s' "$out" | grep -q 'did not all run'; then fail "the transitions of v0.2.0 failed: $out"; fi
	[ "$(docker exec "$host" cat "$transition_marker")" = applied ] ||
		fail "the transition did not run exactly once: $(docker exec "$host" cat "$transition_marker" 2>&1)"
	log "scenario: updated to v0.2.0, host service pid $hpid -> $(service_pid qhost), deploy service pid $pid -> $(service_pid), instance pid $ipid -> $(service_pid "$su_unit"), services answering: $out"
	# Without the override the instance runs from the renewed template.
	docker exec "$host" sh -ec "rm $su_override && systemctl daemon-reload && systemctl restart $su_unit" || fail "cannot remove $su_override"
	[ "$(docker exec "$host" systemctl show -p FragmentPath --value "$su_unit")" = "$su_template" ] ||
		fail "$su_unit does not run from $su_template: $(docker exec "$host" systemctl show -p FragmentPath --value "$su_unit")"
	wait_for "the instance $su_unit to answer" 30 instance_version_is v0.2.0

	step 'sudo qhost update migrate repeats the transitions of the installed release without a second effect'
	out=$(as_admin 'sudo qhost update migrate 2>&1') || fail "the repeated transitions failed: $out"
	printf '%s' "$out" | grep -q 'transition e2e-marker: already done' || fail "the repeat does not report the done transition: $out"
	[ "$(docker exec "$host" cat "$transition_marker")" = applied ] || fail 'the repeated transitions ran the transition again'

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
	hkpid=$(service_pid qhost-hook) hkentered=$(service_entered qhost-hook)
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
	if printf '%s' "$out" | grep -q -e 'transition ' -e 'did not all run'; then fail "v0.3.0 has no transitions, but the update reports some: $out"; fi
	[ "$(docker exec "$host" cat "$transition_marker")" = applied ] || fail 'the update to v0.3.0 touched the marker of the transition'
	[ "$(service_pid qhost)" != "$hpid" ] && [ "$(service_entered qhost)" != "$hentered" ] || fail 'the host service was not restarted by the update'
	docker exec "$host" qhost version 2>&1 | grep -q v0.3.0 || fail "the installed version is not v0.3.0: $(docker exec "$host" qhost version 2>&1)"
	unit_is_release qhost v0.3.0 || fail 'the unit of the host service is not the one of v0.3.0'
	wait_for 'the webhook receiver after the second update' 30 hook_listening
	[ "$(service_pid qhost-hook)" != "$hkpid" ] && [ "$(service_entered qhost-hook)" != "$hkentered" ] ||
		fail 'the webhook receiver was not restarted by the update without a deploy service'
	if docker exec "$host" test -e /usr/local/lib/systemd/system/qhost-deploy.service; then fail 'the update installed the unit of the deploy service'; fi
	[ "$(docker exec "$host" systemctl is-active qhost-deploy || true)" != active ] || fail 'the update started the deploy service'
	as_admin 'qhost --output json status' | grep -q '"deploy_service": *"not set up"' ||
		fail "qhost status does not show the deploy service as not set up after the update: $(as_admin 'qhost --output json status 2>&1')"
	log "scenario: updated to v0.3.0 without a deploy service: host service pid $hpid -> $(service_pid qhost), no deploy unit: $out"

	docker exec "$host" systemctl stop qhost
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
