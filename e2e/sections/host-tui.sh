#!/bin/sh
# Section host-tui: the terminal interface, started without sudo, creates a group and a user with an initial password
# in it through the real sudo, and qhost host users shows the user with the group. An administrator without sudo
# proposes a user with a group and an SSH key with qhost proposal; the interface approves it, asks for the initial
# password and creates the user, the journal shows the change, the undo removes the user, and a process of
# qhost-deploy is not shown the proposal.
# Needs install: the host service and the group qhost of the installation. No other section needs this one.
# Sourced by run.sh; defines only functions and constants.

# tui_system_user_create_count prints how many changes user.create of the administrator the action log holds.
tui_system_user_create_count() {
	host_audit_rows | grep -o "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"user\.create[0-9]" | wc -l
}

# scenario_tui_system proves the System area of the interface started without sudo. On Groups the administrator
# creates a group; on Users a user with an initial password in that group. Each change hands the terminal to the
# real sudo: the group through sudo qhost host apply, the user first through sudo -v and then sudo -n with the
# initial password on standard input. qhost host users then shows the user with the group, the password is set and
# must be changed at the first login, and the password shows on no captured screen, in no state of qhost and in no
# system log.
scenario_tui_system() {
	tui_log=$work/tui-system-screens.log
	: >"$tui_log"
	ts_user=e2e-tuiuser
	ts_group=e2e-tuigroup
	ts_start=E2e-tui-Start-4417
	# sudo -v asks for the password of the administrator although the rule of $admin needs none, because the
	# account is also in the group sudo; the scenario answers it as a person would.
	ts_login=E2e-admin-Login-2093
	printf '%s:%s\n' "$admin" "$ts_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	admin_uid=$(docker exec "$host" id -u "$admin")
	docker exec "$host" sh -c "id $ts_user >/dev/null 2>&1 && userdel -r $ts_user; getent group $ts_group >/dev/null && groupdel $ts_group; rm -rf /home/$ts_user; true"
	creates_before=$(tui_system_user_create_count)

	step 'the interface without sudo creates a group on System > Groups through sudo qhost host apply'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Groups
	tui_wait 'MEMBERS' 10
	tui_send a
	tui_wait 'The system gives the group a regular group number.' 10
	tui_type "$ts_group"
	tui_send F2
	tui_wait "Create the group $ts_group." 10
	tui_has 'groupadd' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "Done: Create the group $ts_group." 60
	tui_send Enter
	docker exec "$host" getent group "$ts_group" >/dev/null || fail "the group $ts_group does not exist after the interface created it"

	step 'the interface without sudo creates a user with an initial password in that group, through sudo -v and sudo -n'
	tui_system_tab Users
	tui_wait 'DOCKER' 10
	tui_send a
	tui_wait 'The user must change the initial password at the first login.' 10
	tui_type "$ts_user"
	tui_send Down
	tui_type "$ts_start"
	tui_send Down
	tui_type "$ts_start"
	tui_wait "$ts_group" 10
	tui_click "$ts_group"
	sleep 0.3
	tui_send F2
	tui_wait "Create the user $ts_user" 10
	tui_has "$ts_group" || fail "the commands of the question do not put the user in $ts_group: $(tui_screen)"
	tui_send y
	tui_wait "[sudo] password for $admin" 30
	tui_type "$ts_login"
	tui_send Enter
	tui_wait 'Done: Create the user' 60
	tui_send Enter
	i=0
	until tui_screen | grep "$ts_user" | grep -q "$ts_group"; do
		i=$((i + 1))
		[ "$i" -lt 40 ] || fail "the users list does not show $ts_user with $ts_group: $(tui_screen)"
		sleep 0.25
	done
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'qhost host users shows the user with the group; the initial password is set and must be changed'
	out=$(as_admin 'qhost host users 2>&1') || fail "qhost host users failed: $out"
	printf '%s\n' "$out" | grep -F -- "$ts_user" | grep -qF -- "$ts_group" || fail "qhost host users does not show $ts_user with $ts_group: $out"
	out=$(as_admin 'qhost --output json host users 2>&1') || fail "qhost --output json host users failed: $out"
	printf '%s' "$out" | jq -e --arg u "$ts_user" --arg g "$ts_group" '[.. | objects | select(.name? == $u)][0].groups | index($g) != null' >/dev/null ||
		fail "the JSON of qhost host users does not show $ts_user with $ts_group: $out"
	printf '%s' "$ts_start" | docker exec -i "$host" perl -e 'my $p = <STDIN>; open(my $f, "<", "/etc/shadow") or exit 2;
		while (<$f>) { my @r = split /:/; exit(crypt($p, $r[1]) eq $r[1] && $r[2] eq "0" ? 0 : 1) if $r[0] eq $ARGV[0] } exit 3' "$ts_user" ||
		fail "the initial password of $ts_user did not arrive over standard input, or it need not be changed at the first login"
	[ "$(tui_system_user_create_count)" -eq $((creates_before + 1)) ] ||
		fail "the action log does not hold one more user.create of uid $admin_uid behind sudo: $(host_audit_rows)"

	step 'the initial password shows on no captured screen, in no state of qhost and in no system log'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$ts_start" "$tui_log"; then fail 'the initial password shows on a captured screen'; fi
	if grep -qF -- "$ts_login" "$tui_log"; then fail 'the password typed for sudo shows on a captured screen'; fi
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	if docker exec "$host" grep -rqF -- "$ts_start" /var/lib/qhost-host /var/lib/qhost; then fail 'the initial password reached the state of qhost'; fi
	if docker exec "$host" sh -c 'journalctl --no-pager -q 2>/dev/null' | grep -qF -- "$ts_start"; then fail 'the initial password reached the system log'; fi

	docker exec "$host" sh -c "userdel -r $ts_user; groupdel $ts_group; rm -rf /home/$ts_user; true"
	log "scenario: the interface without sudo created $ts_group and $ts_user in it through sudo; qhost host users shows both"
}

# scenario_tui_changes proves System > Folders, Changes and Log of the interface started without sudo. The
# administrator takes the permissions of everyone else from a test folder through sudo qhost host apply; Changes reads
# the journal through sudo qhost host journal on r and lists the change; its undo through sudo qhost host undo brings
# back owner, group and mode, as stat shows; Log shows the change of the host under an intact chain of hashes. The
# rule of $admin lets sudo run a command without the password, so none is asked here.
scenario_tui_changes() {
	tui_log=$work/tui-changes-screens.log
	: >"$tui_log"
	tc_dir=/home/$admin/e2e-tui-folder
	as_admin "rm -rf '$tc_dir' && mkdir '$tc_dir' && chmod 0755 '$tc_dir' && touch '$tc_dir/note.txt'" || fail 'cannot make the test folder'
	tc_before=$(as_admin "stat -c %U:%G:%a '$tc_dir'")
	[ "${tc_before##*:}" = 755 ] || fail "the test folder starts as $tc_before, want the mode 755"

	step 'the interface without sudo takes the permissions of everyone else from a folder on System > Folders'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Folders
	tui_wait 'to choose a folder' 10
	tui_send Enter
	tui_wait 'Type the absolute path of the folder.' 10
	tui_type "$tc_dir"
	tui_send Enter
	tui_wait "Owner $admin may: read and write" 10
	tui_has 'Everyone else may: read' || fail "the folder does not show that everyone else may read: $(tui_screen)"
	tui_send e
	tui_wait 'Everyone else may' 10
	tui_send Down Down Down Down
	sleep 0.2
	tui_send Left
	sleep 0.2
	tui_send F2
	tui_wait 'everyone else nothing.' 10
	tui_has 'chmod' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	tui_wait "Done: Change $tc_dir" 60
	tui_send Enter
	[ "$(as_admin "stat -c %a '$tc_dir'")" = 750 ] || fail "the folder has the mode $(as_admin "stat -c %a '$tc_dir'") after the change, want 750"

	step 'System > Changes reads the journal through sudo on r and lists the change of the folder'
	tui_system_tab Changes
	tui_wait 'only root can read' 10
	tui_send r
	tui_wait "Change $tc_dir" 60
	i=0
	until tui_screen | grep "^> " | grep -q "Change $tc_dir"; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the newest change is not the change of $tc_dir under the cursor: $(tui_screen)"
		sleep 0.25
	done
	tui_screen | grep "^> " | grep -q possible || fail "the change of $tc_dir offers no undo: $(tui_screen)"
	tui_has 'Undo: Restore the previous owner, group and permissions' || fail "the undo of the change is not shown: $(tui_screen)"

	step 'the undo through sudo qhost host undo restores owner, group and mode of the folder'
	tui_send Enter
	tui_wait 'Undo the change: Change' 10
	# The dialog wraps its text, so the sentence may stand on two lines.
	{ tui_has 'The undo itself' && tui_has 'cannot be undone.'; } || fail "the question does not say that the undo cannot be undone: $(tui_screen)"
	tui_has 'chown -h --' || fail "the question does not show the commands of the undo: $(tui_screen)"
	tui_send y
	tui_wait 'Done: Undo the change' 60
	tui_send Enter
	tc_after=$(as_admin "stat -c %U:%G:%a '$tc_dir'")
	[ "$tc_after" = "$tc_before" ] || fail "the undo left the folder as $tc_after, want $tc_before"
	i=0
	until tui_screen | grep "Change $tc_dir" | grep -q undone; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the undone change is not shown as undone: $(tui_screen)"
		sleep 0.25
	done
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s' "$journal" | tr -d ' \n' | grep -q '"result":"undone"' || fail "the journal does not mark the change undone: $journal"

	step 'System > Log shows the change of the host under an intact chain of hashes'
	tui_system_tab Log
	tui_wait 'Chain of hashes intact' 30
	tui_has 'host.change' || fail "the log does not show the change of the host: $(tui_screen)"
	tui_has 'folder.set-permissions' || fail "the log does not name the action of the change: $(tui_screen)"
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the changes from the interface: $out"
	as_admin "rm -rf '$tc_dir'"
	log "scenario: the interface without sudo changed $tc_dir to 750, listed the change on Changes and undid it back to $tc_before"
}

# scenario_tui_host_proposal proves a proposal of the host from submission to undo. An administrator without sudo
# proposes a user in a group with a public SSH key through qhost proposal; the proposal is listed and shown with its
# sentence and the key fingerprint, and a process of qhost-deploy is turned away from the proposals of the host.
# The interface without sudo approves it on System > Proposals: the form asks for the initial password, sudo -v asks
# for the password of the administrator, and sudo qhost host apply --proposal creates the user. The user, the group and
# authorized_keys are checked, the journal lists the change, the proposal stands as applied, and the undo on
# System > Changes removes the user again. The initial password shows in no output, no captured screen, no state of
# qhost and no system log.
scenario_tui_host_proposal() {
	tui_log=$work/tui-host-proposal-screens.log
	: >"$tui_log"
	leaks=$work/host-proposal-outputs.log
	: >"$leaks"
	hp_user=e2e-propuser
	hp_group=e2e-propgroup
	hp_start=E2e-prop-Start-5529
	hp_login=E2e-admin-Login-4471
	hp_keydir=/home/$admin/e2e-propkey
	printf '%s:%s\n' "$admin" "$hp_login" | docker exec -i "$host" chpasswd || fail "cannot give $admin a password"
	admin_uid=$(docker exec "$host" id -u "$admin")
	docker exec "$host" sh -c "id $hp_user >/dev/null 2>&1 && userdel -r $hp_user; getent group $hp_group >/dev/null && groupdel $hp_group; rm -rf /home/$hp_user; true"
	as_admin "rm -rf '$hp_keydir' && mkdir '$hp_keydir' && ssh-keygen -q -t ed25519 -N '' -C e2e-propkey@example.test -f '$hp_keydir/key'" ||
		fail 'cannot generate the test key'
	hp_key=$(as_admin "cat '$hp_keydir/key.pub'")
	hp_blob=$(printf '%s' "$hp_key" | cut -d ' ' -f 2)
	hp_fp=$(as_admin "ssh-keygen -lf '$hp_keydir/key.pub'" | cut -d ' ' -f 2)
	[ -n "$hp_blob" ] && [ -n "$hp_fp" ] || fail "the test key is not usable: $hp_key"
	out=$(as_admin "sudo qhost --output json host apply group.create $hp_group --confirm 2>&1") || fail "host apply group.create failed: $out"
	creates_before=$(tui_system_user_create_count)

	step 'an administrator without sudo proposes a user with a group and an SSH key, and it is listed and shown'
	content=$(jq -nc --arg u "$hp_user" --arg g "group=$hp_group" --arg k "$hp_key" '{action: "user.create", args: [$u, $g], keys: [$k]}')
	out=$(printf '%s' "$content" | as_admin 'qhost --output json proposal submit --kind host-change 2>&1') ||
		fail "qhost proposal submit --kind host-change without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	hp_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(h-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$hp_id" ] || fail "qhost proposal submit printed no id of the host service: $out"
	printf '%s' "$out" | grep -q '"state": *"open"' || fail "the new proposal is not open: $out"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s\n' "$out" | grep -F -- "$hp_id" | grep -q 'host-change' || fail "qhost proposal list does not show $hp_id as host-change: $out"
	out=$(as_admin "qhost proposal show $hp_id 2>&1") || fail "qhost proposal show without sudo failed: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s' "$out" | grep -qF -- "Create the user $hp_user" || fail "qhost proposal show does not give the sentence of the change: $out"
	printf '%s' "$out" | grep -qF -- "$hp_fp" || fail "qhost proposal show does not show the key fingerprint $hp_fp: $out"
	[ "$(prop_state "$hp_id")" = open ] || fail "the proposal $hp_id is not open"
	if docker exec "$host" id "$hp_user" >/dev/null 2>&1; then fail 'submitting the proposal created the user'; fi

	step 'a process of qhost-deploy is turned away from the proposals of the host'
	if out=$(docker exec -u qhost-deploy "$host" qhost proposal list 2>&1 </dev/null); then
		fail "qhost-deploy listed proposals while the host service holds one: $out"
	fi
	if printf '%s' "$out" | grep -qF -e "$hp_id" -e "$hp_user" -e "$hp_blob" -e e2e-propkey; then fail "the refusal for qhost-deploy shows data of the proposal: $out"; fi
	if out=$(docker exec -u qhost-deploy "$host" qhost proposal show "$hp_id" 2>&1 </dev/null); then
		fail "qhost-deploy shows the proposal of the host: $out"
	fi
	if printf '%s' "$out" | grep -qF -e "$hp_user" -e "$hp_blob" -e e2e-propkey; then fail "the refusal for qhost-deploy shows data of the proposal: $out"; fi

	step 'the interface without sudo approves the proposal on System > Proposals, asks for the initial password and creates the user'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'qhost tui; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 7
	tui_wait 'DOCKER' 15
	tui_system_tab Proposals
	tui_wait "$hp_id" 15
	tui_send Enter
	tui_wait "Proposal $hp_id from $admin" 10
	tui_has 'The change' || fail "the view does not show the change: $(tui_screen)"
	tui_has "Create the user $hp_user" || fail "the view does not show the sentence: $(tui_screen)"
	tui_has 'Commands, as typed by hand' || fail "the view does not show the commands: $(tui_screen)"
	tui_has 'Proposed SSH keys' || fail "the view does not show the proposed keys: $(tui_screen)"
	tui_has 'e2e-propkey@example.test' || fail "the view does not show the comment of the key: $(tui_screen)"
	tui_send y
	tui_wait 'The user must change the initial password at the first login.' 10
	tui_type "$hp_start"
	tui_send Down
	tui_type "$hp_start"
	tui_send F2
	tui_wait "Apply proposal $hp_id to the host" 10
	tui_has 'useradd' || fail "the question does not show the command: $(tui_screen)"
	tui_send y
	# sudo -v asks for the password of the administrator; a still valid ticket would skip the prompt.
	i=0
	while :; do
		screen=$(tui_screen)
		case $screen in
		*"[sudo] password for $admin"*)
			tui_type "$hp_login"
			tui_send Enter
			break
			;;
		*"Done: Apply proposal $hp_id"*) break ;;
		esac
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail "neither the password prompt of sudo nor the result appears: $screen"
		sleep 0.25
	done
	tui_wait "Done: Apply proposal $hp_id" 60
	tui_has 'Journal entry' || fail "the result does not name the journal entry: $(tui_screen)"
	tui_send Enter

	step 'the user, the group and authorized_keys are in place, the journal shows the change and the proposal stands as applied'
	docker exec "$host" id "$hp_user" >/dev/null || fail "the user $hp_user does not exist after the approval"
	docker exec "$host" id -nG "$hp_user" | tr ' ' '\n' | grep -qx "$hp_group" || fail "$hp_user is not in $hp_group: $(docker exec "$host" id -nG "$hp_user")"
	[ "$(docker exec "$host" cat "/home/$hp_user/.ssh/authorized_keys" | cut -d ' ' -f 2 | grep -cxF -- "$hp_blob")" = 1 ] ||
		fail "authorized_keys of $hp_user does not hold the proposed key once: $(docker exec "$host" cat "/home/$hp_user/.ssh/authorized_keys" 2>&1)"
	[ "$(docker exec "$host" stat -c %U "/home/$hp_user/.ssh/authorized_keys")" = "$hp_user" ] || fail "authorized_keys of $hp_user is not owned by the user"
	printf '%s' "$hp_start" | docker exec -i "$host" perl -e 'my $p = <STDIN>; open(my $f, "<", "/etc/shadow") or exit 2;
		while (<$f>) { my @r = split /:/; exit(crypt($p, $r[1]) eq $r[1] && $r[2] eq "0" ? 0 : 1) if $r[0] eq $ARGV[0] } exit 3' "$hp_user" ||
		fail "the initial password of $hp_user did not arrive, or it need not be changed at the first login"
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s\n' "$journal" >>"$leaks"
	hp_change=$(printf '%s' "$journal" | jq -r --arg u "$hp_user" '[.. | objects | select(.action? == "user.create" and .result? == "applied" and (.sentence? // "" | contains($u)))][0].id // empty')
	[ -n "$hp_change" ] || fail "the journal has no entry of user.create for $hp_user: $journal"
	printf '%s' "$journal" | jq -e --arg id "$hp_change" --argjson uid "$admin_uid" \
		'[.. | objects | select(.id? == $id)][0] | .result == "applied" and .reversible == true and .sudo_uid == $uid' >/dev/null ||
		fail "the journal entry $hp_change is not applied, reversible and attributed to uid $admin_uid: $journal"
	[ "$(tui_system_user_create_count)" -eq $((creates_before + 1)) ] ||
		fail "the action log does not hold one more user.create of uid $admin_uid behind sudo: $(host_audit_rows)"
	[ "$(prop_state "$hp_id")" = applied ] || fail "the proposal $hp_id is $(prop_state "$hp_id"), want applied"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list failed after the approval: $out"
	printf '%s\n' "$out" >>"$leaks"
	printf '%s\n' "$out" | grep -F -- "$hp_id" | grep -q 'applied' || fail "qhost proposal list does not show $hp_id as applied: $out"

	step 'the undo on System > Changes removes the user again'
	tui_system_tab Changes
	tui_wait 'only root can read' 10
	tui_send r
	tui_wait "Create the user $hp_user" 60
	i=0
	until tui_screen | grep "^> " | grep -q "Create the user $hp_user"; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the newest change is not the creation of $hp_user under the cursor: $(tui_screen)"
		sleep 0.25
	done
	tui_screen | grep "^> " | grep -q possible || fail "the creation of $hp_user offers no undo: $(tui_screen)"
	tui_send Enter
	tui_wait 'Undo the change: ' 10
	tui_send y
	tui_wait 'Done: Undo the change' 60
	tui_send Enter
	if docker exec "$host" id "$hp_user" >/dev/null 2>&1; then fail "the user $hp_user exists after the undo"; fi
	if docker exec "$host" test -e "/home/$hp_user"; then fail "the home of $hp_user exists after the undo"; fi
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s\n' "$journal" >>"$leaks"
	printf '%s' "$journal" | jq -e --arg id "$hp_change" '[.. | objects | select(.id? == $id)][0] | .result == "undone" and .reversible == false' >/dev/null ||
		fail "the journal does not close the undone entry $hp_change: $journal"
	[ "$(prop_state "$hp_id")" = applied ] || fail "the undo changed the state of the proposal $hp_id to $(prop_state "$hp_id")"
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the proposal: $out"

	step 'the initial password shows in no output, no captured screen, no state of qhost and no system log'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	if grep -qF -- "$hp_start" "$tui_log" "$leaks"; then fail 'the initial password shows on a captured screen or in an output'; fi
	if grep -qF -- "$hp_login" "$tui_log" "$leaks"; then fail 'the password typed for sudo shows on a captured screen or in an output'; fi
	docker exec "$host" usermod -p '!' "$admin" || fail "cannot lock the password of $admin again"
	if docker exec "$host" grep -rqF -- "$hp_start" /var/lib/qhost-host /var/lib/qhost; then fail 'the initial password reached the state of qhost'; fi
	if docker exec "$host" sh -c 'journalctl --no-pager -q 2>/dev/null' | grep -qF -- "$hp_start"; then fail 'the initial password reached the system log'; fi

	docker exec "$host" sh -c "id $hp_user >/dev/null 2>&1 && userdel -r $hp_user; getent group $hp_group >/dev/null && groupdel $hp_group; rm -rf /home/$hp_user; true"
	as_admin "rm -rf '$hp_keydir'"
	log "scenario: $hp_id proposed $hp_user in $hp_group with a key, the interface approved it as $hp_change and the undo removed the user"
}

# scenario_free_command_proposal proves a proposal of free commands. It is refused while the switch is off. After
# sudo qhost proposal commands enable, the administrator without sudo submits a harmless command, proposal show marks it
# as text of the submitter, qhost proposal approve and host apply point to proposal run, and sudo qhost proposal run
# with the fingerprint and --confirm creates the file as root. The journal holds the commands and cannot be undone, the
# action log names the proposal and the human behind sudo without the command, and with the switch off again the
# next run is refused.
scenario_free_command_proposal() {
	fc_dir=/var/tmp/e2e-free-command
	fc_text="touch $fc_dir/ready"
	docker exec "$host" sh -c "rm -rf '$fc_dir' && mkdir '$fc_dir'" || fail 'cannot prepare the folder of the free command'
	admin_uid=$(docker exec "$host" id -u "$admin")
	# The commands that apply a proposal need a person at a terminal, so they run under a pseudo terminal; -e passes
	# the exit code of the command on. The command must not hold a single quote.
	fc_tty() { as_admin "script -qec '$1' /dev/null"; }
	# The pseudo terminal may echo characters before the output, so a report read with jq goes to a file instead;
	# standard input stays the terminal.
	fc_out=$(as_admin 'mktemp -d') || fail 'cannot create the folder for the report of proposal run'

	step 'a proposal of free commands is refused while the switch is off'
	content=$(jq -nc --arg c "$fc_text" '{commands: [$c], reason: "e2e: leave a marker file"}')
	if out=$(printf '%s' "$content" | as_admin 'qhost --output json proposal submit --kind command 2>&1'); then
		fail "a proposal of free commands was accepted while the switch is off: $out"
	fi
	printf '%s' "$out" | grep -q 'sudo qhost proposal commands enable' || fail "the refusal does not name the switch: $out"
	out=$(as_admin 'qhost --output json proposal commands status 2>&1') || fail "proposal commands status failed: $out"
	printf '%s' "$out" | grep -q '"enabled": *false' || fail "the switch is not off by default: $out"
	if out=$(as_admin 'qhost proposal commands enable 2>&1'); then fail "proposal commands enable worked without sudo: $out"; fi

	step 'sudo qhost proposal commands enable turns the switch on, and the administrator without sudo submits the command'
	out=$(as_admin 'sudo qhost --output json proposal commands enable 2>&1') || fail "sudo qhost proposal commands enable failed: $out"
	out=$(printf '%s' "$content" | as_admin 'qhost --output json proposal submit --kind command 2>&1') || fail "submit --kind command failed: $out"
	fc_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(h-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$fc_id" ] || fail "proposal submit printed no id: $out"
	out=$(as_admin "qhost proposal show $fc_id 2>&1") || fail "proposal show failed: $out"
	printf '%s' "$out" | grep -qF 'unchecked, no computed effect, no undo' || fail "proposal show lacks the notice: $out"
	printf '%s' "$out" | grep -qF -- "$fc_text" || fail "proposal show lacks the command: $out"
	fc_fp=$(as_admin "qhost --output json proposal show $fc_id" | jq -r .fingerprint)
	[ -n "$fc_fp" ] && [ "$fc_fp" != null ] || fail 'proposal show gave no fingerprint'
	docker exec "$host" test ! -e "$fc_dir/ready" || fail 'submitting the proposal ran the command'

	step 'approve and host apply do not take the proposal, and run without --confirm runs nothing'
	if out=$(fc_tty "sudo qhost host apply --proposal $fc_id --fingerprint $fc_fp --confirm 2>&1"); then fail "host apply took a proposal of free commands: $out"; fi
	printf '%s' "$out" | grep -qF 'proposal run' || fail "host apply does not point to proposal run: $out"
	if out=$(as_admin "qhost proposal run $fc_id --fingerprint $fc_fp --confirm 2>&1"); then fail "proposal run worked without sudo: $out"; fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "proposal run without sudo gives the wrong refusal: $out"
	if out=$(as_admin "sudo qhost proposal run $fc_id --fingerprint $fc_fp --confirm 2>&1"); then fail "proposal run worked without a terminal: $out"; fi
	printf '%s' "$out" | grep -q 'needs a terminal' || fail "proposal run without a terminal gives the wrong refusal: $out"
	docker exec "$host" test ! -e "$fc_dir/ready" || fail 'proposal run without a terminal created the file'
	if out=$(fc_tty "sudo qhost proposal run $fc_id --fingerprint $fc_fp 2>&1"); then fail "proposal run without --confirm succeeded: $out"; fi
	if out=$(fc_tty "sudo qhost proposal run $fc_id --fingerprint wrong --confirm 2>&1"); then fail "proposal run took a wrong fingerprint: $out"; fi
	docker exec "$host" test ! -e "$fc_dir/ready" || fail 'a refused run created the file'

	step 'sudo qhost proposal run with the fingerprint and --confirm runs the command as root'
	fc_status=0
	fc_tty "sudo qhost --output json proposal run $fc_id --fingerprint $fc_fp --confirm >$fc_out/run.json 2>/dev/null" || fc_status=$?
	out=$(docker exec "$host" cat "$fc_out/run.json" 2>&1) || fail "the report of proposal run cannot be read: $out"
	[ "$fc_status" -eq 0 ] || fail "proposal run failed with exit code $fc_status: $out"
	docker exec "$host" test -e "$fc_dir/ready" || fail "the file was not created: $out"
	[ "$(docker exec "$host" stat -c %U "$fc_dir/ready")" = root ] || fail 'the command did not run as root'
	fc_journal=$(printf '%s' "$out" | jq -r '.journal_id // empty')
	[ -n "$fc_journal" ] || fail "the report names no journal entry: $out"
	[ "$(prop_state "$fc_id")" = applied ] || fail "the proposal is $(prop_state "$fc_id"), want applied"

	step 'the journal holds the command and refuses the undo, and the action log names the proposal without the command'
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	printf '%s' "$journal" | jq -e --arg id "$fc_journal" --argjson uid "$admin_uid" \
		'[.. | objects | select(.id? == $id)][0] | .action == "proposal.run" and .reversible == false and .sudo_uid == $uid' >/dev/null ||
		fail "the journal entry $fc_journal is not the unreversible run of uid $admin_uid: $journal"
	docker exec "$host" cat "/var/lib/qhost-host/journal/$fc_journal.json" | jq -e --argjson uid "$admin_uid" --arg name "$admin" \
		'.submitter_uid == $uid and .submitter_name == $name and (.exits | length) == 1 and .commands[0] == "touch /var/tmp/e2e-free-command/ready"' >/dev/null ||
		fail "the journal file of $fc_journal does not name $admin (uid $admin_uid) as the submitter with the command and its exit code"
	if out=$(as_admin "sudo qhost host undo $fc_journal --confirm 2>&1"); then fail "host undo took the run of free commands: $out"; fi
	printf '%s' "$out" | grep -q 'cannot be undone' || fail "host undo gives the wrong refusal: $out"
	out=$(as_admin 'qhost --output json audit --action host.change --limit 100 2>&1') || fail "qhost audit failed: $out"
	printf '%s' "$out" | grep -qF "proposal.run $fc_journal proposal $fc_id" || fail "qhost audit does not show the run of $fc_id: $out"
	if printf '%s' "$out" | grep -qF -e "$fc_dir" -e touch; then fail "qhost audit shows the command: $out"; fi
	printf '%s' "$out" | tr -d ' \n' | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\"" || fail "qhost audit does not name the human behind sudo: $out"

	step 'with the switch off again a new proposal is refused and an open one does not run'
	out=$(printf '%s' "$content" | as_admin 'qhost --output json proposal submit --kind command 2>&1') || fail "submit failed with the switch on: $out"
	fc_id2=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(h-[0-9a-f]*\)".*/\1/p' | head -n 1)
	fc_fp2=$(as_admin "qhost --output json proposal show $fc_id2" | jq -r .fingerprint)
	out=$(as_admin 'sudo qhost --output json proposal commands disable 2>&1') || fail "sudo qhost proposal commands disable failed: $out"
	if out=$(fc_tty "sudo qhost proposal run $fc_id2 --fingerprint $fc_fp2 --confirm 2>&1"); then fail "proposal run worked with the switch off: $out"; fi
	printf '%s' "$out" | grep -qF 'proposal commands enable' || fail "the refusal does not name the switch: $out"
	if printf '%s' "$content" | as_admin 'qhost proposal submit --kind command >/dev/null 2>&1'; then fail 'a proposal was accepted with the switch off'; fi
	as_admin "qhost proposal withdraw $fc_id2 >/dev/null 2>&1" || true
	docker exec "$host" rm -rf "$fc_dir" "$fc_out"
	log "scenario: $fc_id ran as $fc_journal behind the switch and cannot be undone"
}
