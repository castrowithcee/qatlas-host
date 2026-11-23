#!/bin/sh
# Section instance: the deploy service for the rootless engine of the test user is set up through the interface after
# the command line showed its steps, a user in the group docker is refused, the instance runs as the test user against
# that user's engine, status shows it without sudo, only root and the group qhost reach its channel and
# systemd-analyze verify accepts its unit. A maintenance window without a bound source writes a warning to the
# service log; scenario_service_log must run before any source is bound, which the order of the register keeps.
# Needs install: the setup of the instance requires the deploy service of the system engine.
# Sourced by run.sh; defines only functions and constants.

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

	# The engine of $rootless_user has no deploy service yet and the one of the system engine runs: the only moment
	# of the run to see an engine without a deploy service.
	step "container list shows a container of the engine of $rootless_user without a deploy service only with sudo, read-only"
	ri_engine=rootless:$rootless_user
	ri_ct=$prefix-rootless-ro
	as_rootless "docker run -d --quiet --name $ri_ct demo/app:1.0.0" >/dev/null ||
		fail 'the rootless engine cannot start demo/app:1.0.0 from the registry mirror'
	ri_started=$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $ri_ct")
	out=$(docker exec -i "$host" qhost --output json container list 2>/dev/null </dev/null) || fail "sudo qhost container list failed: $out"
	printf '%s' "$out" | jq -e --arg e "$ri_engine" --arg c "$ri_ct" '
		any(.groups[]; .engine == $e and .name == $c and .kind == "single" and .read_only == true)
		and (.notes // [] | any(contains("has no deploy service")))' >/dev/null ||
		fail "container list as root does not show $ri_ct read-only in $ri_engine, with the note: $out"
	out=$(qd container list) || fail "container list as root failed: $out"
	printf '%s' "$out" | grep -q "^single  *$ri_engine  *$ri_ct " || fail "the table does not show $ri_ct with the engine $ri_engine: $out"
	printf '%s' "$out" | grep -qF "note: the Docker engine $ri_engine has no deploy service" || fail "container list does not say that $ri_engine has no deploy service: $out"
	out=$(as_admin 'qhost --output json container list 2>/dev/null') || fail "qhost container list without sudo failed: $out"
	printf '%s' "$out" | jq -e --arg e "$ri_engine" '
		all((.groups // [])[]; .engine != $e) and (.notes // [] | any(contains("shown only with sudo")))' >/dev/null ||
		fail "container list without sudo shows the engine without a deploy service or lacks the note: $out"
	out=$(as_admin 'qhost container list 2>&1 >/dev/null') || fail "qhost container list without sudo failed: $out"
	printf '%s' "$out" | grep -qF 'note: Docker engines without a deploy service are shown only with sudo' ||
		fail "container list without sudo does not print the note: $out"
	out=$(qd_fails container restart "$ri_ct" --yes) || fail "container restart reached a container of an engine without a deploy service: $out"
	printf '%s' "$out" | grep -q 'has no deploy service' || fail "the refusal does not say that the engine has no deploy service: $out"
	[ "$(as_rootless "docker inspect -f '{{.State.StartedAt}}' $ri_ct")" = "$ri_started" ] || fail "the refused restart changed $ri_ct"
	as_rootless "docker rm -f $ri_ct" >/dev/null || fail "cannot remove $ri_ct"
	log "scenario: container list showed $ri_ct read-only in $ri_engine with sudo and said so without; the restart was refused"

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
	# The service sets up its own Docker configuration at its start: logins come only from the credential helper.
	[ "$(docker exec "$host" cat "$ri_state/docker/config.json")" = '{"credsStore":"qhost"}' ] ||
		fail "$ri_state/docker/config.json is not the credential helper configuration: $(docker exec "$host" cat "$ri_state/docker/config.json" 2>&1)"
	[ "$(docker exec "$host" stat -c %U:%a "$ri_state/docker/config.json")" = "$rootless_user:600" ] ||
		fail "$ri_state/docker/config.json is $(docker exec "$host" stat -c %U:%a "$ri_state/docker/config.json"), want $rootless_user:600"
	docker exec "$host" test -L "$ri_state/docker/bin/docker-credential-qhost" && docker exec "$host" test -x "$ri_state/docker/bin/docker-credential-qhost" ||
		fail "$ri_state/docker/bin/docker-credential-qhost is no link to the program"
	if docker exec "$host" sh -c "tr '\\0' '\\n' </proc/$ri_pid/environ" | grep -q '^DOCKER_CONFIG='; then
		fail "the unit of $ri_unit sets DOCKER_CONFIG, which the service must set itself"
	fi
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
# before a source is bound; bind_source moves the window away afterwards.
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
