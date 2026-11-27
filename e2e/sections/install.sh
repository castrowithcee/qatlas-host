#!/bin/sh
# Section install: qhost is installed from a signed test release with the release installer, which sets up only the
# host service; its user and directories, its missing access to the container engine and the group qhost of the
# installing admin are checked, and status and a deploy command name the deploy service as not set up. sudo qhost
# deploy-service setup then sets up the deploy service: it runs as root in its sandbox, with its channel and
# state, the sandbox hides the host service state from it, and an administrator of the group reads status without sudo
# while a change needs root. The host service serves an administrator of the group without sudo, answers a user of the
# group without sudo not-admin, turns away a user outside the group and a revoked administrator, and the check of its
# action log reports a changed line. Changes of the host follow: sudo qhost host apply changes the permissions of a
# folder, the journal shows it, the undo restores them and the action log names the person behind sudo; a user is
# created, put in a group, locked and deleted, and the protection rules refuse the rest.
# Needs no other section: nothing here uses the stacks repository or a container of base.
# Sourced by run.sh; defines only functions and constants.

# host_log_file prints the newest day file of the action log of the host service.
host_log_file() {
	docker exec "$host" sh -c 'ls /var/lib/qhost/logs/*.jsonl | tail -n 1'
}

# host_log_rewrite runs the sed expression $1 on the day file $2 in place without replacing the file, so it keeps its
# owner qhost and its mode, and the service keeps appending to it.
host_log_rewrite() {
	docker exec "$host" sh -ec 'sed -E "$1" "$2" >/tmp/qhost-log.jsonl && cat /tmp/qhost-log.jsonl >"$2" && rm -f /tmp/qhost-log.jsonl' sh "$1" "$2"
}

# scenario_host_service proves the access to the host service and its action log: an administrator of the group qhost
# reads the state of the host service and the log without sudo, a user of the group without sudo is told not-admin, a
# user outside the group is turned away at the channel, a revoked administrator is refused at the next request, and
# the check of the log reports a changed line. It runs right after the setup.
scenario_host_service() {
	step 'an administrator of the group qhost reads the state of the host service and its action log without sudo'
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed: $out"
	printf '%s' "$out" | grep -q '"host_service": *"running"' || fail "qhost status without sudo does not show the host service as running: $out"
	printf '%s' "$out" | grep -q '"host_version": *"v0.1.0"' || fail "qhost status without sudo does not show the version of the host service: $out"
	out=$(as_admin 'qhost --output json audit --action control.status 2>&1') || fail "qhost audit without sudo failed: $out"
	printf '%s' "$out" | grep -q '"action": *"control.status"' || fail "qhost audit does not list the status requests: $out"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost/logs)" = qhost:700 ] ||
		fail "/var/lib/qhost/logs is $(docker exec "$host" stat -c %U:%a /var/lib/qhost/logs), want qhost:700"
	[ "$(docker exec "$host" stat -c %U:%a "$(host_log_file)")" = qhost:600 ] || fail 'the day file of the action log is not qhost:600'
	if docker exec "$host" test -e /var/lib/qhost-host/logs; then fail 'the action log was written outside the state of the host service'; fi
	log "scenario: $admin read the host service and its action log without sudo"

	step 'a user of the group qhost without sudo is told not-admin by the host service, a user outside the group is turned away'
	ensure_user e2e-hostreader
	ensure_user e2e-outsider
	docker exec "$host" usermod -aG qhost e2e-hostreader || fail 'cannot add e2e-hostreader to the group qhost'
	reader_uid=$(docker exec "$host" id -u e2e-hostreader)
	for cmd in 'qhost audit' 'qhost audit verify'; do
		if out=$(as_user e2e-hostreader "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-hostreader: $out"; fi
		printf '%s' "$out" | grep -q 'qhost is reserved for administrators (sudo)' || fail "unexpected refusal of '$cmd' for e2e-hostreader: $out"
		if printf '%s' "$out" | grep -q 'control\.\|log\.read\|intact'; then fail "'$cmd' shows data to e2e-hostreader: $out"; fi
		if out=$(as_user e2e-outsider "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-outsider: $out"; fi
		printf '%s' "$out" | grep -q 'may not use the qhost service channel' || fail "unexpected refusal of '$cmd' for e2e-outsider: $out"
	done
	out=$(as_admin 'qhost --output json audit --action control.refuse --result failure 2>&1') || fail "qhost audit of the refusals failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q "\"client_uid\":$reader_uid,\"action\":\"control.refuse\",\"result\":\"failure\",\"error_code\":\"not-admin\"" ||
		fail "the action log does not record e2e-hostreader as refused with not-admin: $out"
	log "scenario: e2e-hostreader (uid $reader_uid, group qhost, no sudo) got not-admin, e2e-outsider was turned away at the channel"

	step 'a revoked administrator is refused by the host service at the next request'
	ensure_user e2e-hostadmin
	docker exec "$host" usermod -aG qhost,sudo e2e-hostadmin || fail 'cannot make e2e-hostadmin an administrator of the group qhost'
	out=$(as_user e2e-hostadmin 'qhost audit verify 2>&1') || fail "qhost audit verify failed for the administrator e2e-hostadmin: $out"
	docker exec "$host" gpasswd -d e2e-hostadmin sudo >/dev/null || fail 'cannot remove e2e-hostadmin from the group sudo'
	if out=$(as_user e2e-hostadmin 'qhost audit verify 2>&1'); then fail "qhost audit verify succeeded after the revocation: $out"; fi
	printf '%s' "$out" | grep -q 'qhost is reserved for administrators (sudo)' || fail "unexpected refusal after the revocation: $out"
	docker exec "$host" id -nG e2e-hostadmin | tr ' ' '\n' | grep -qx qhost || fail 'e2e-hostadmin lost the group qhost, so the refusal proves nothing about sudo'
	log 'scenario: e2e-hostadmin read the log as an administrator and was refused after leaving the group sudo'

	step 'the check of the action log reports a changed line and passes again once the line is back'
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed on the untouched log: $out"
	printf '%s' "$out" | grep -q 'intact' || fail "qhost audit verify does not report the log as intact: $out"
	file=$(host_log_file)
	[ "$(docker exec "$host" sh -c 'wc -l <"$1"' sh "$file")" -ge 3 ] || fail "the day file $file has fewer than three lines"
	uid2=$(docker exec "$host" sed -n '2p' "$file" | sed -E 's/.*"client_uid":([0-9]+).*/\1/')
	case $uid2 in '' | *[!0-9]*) fail "line 2 of $file names no client: $(docker exec "$host" sed -n '2p' "$file")" ;; esac
	host_log_rewrite "2s/\"client_uid\":$uid2([,}])/\"client_uid\":4242424\\1/" "$file" || fail "cannot change line 2 of $file"
	docker exec "$host" sed -n '2p' "$file" | grep -q '"client_uid":4242424' || fail 'line 2 of the action log was not changed'
	if out=$(as_admin 'qhost audit verify 2>&1'); then fail "qhost audit verify passed a changed line: $out"; fi
	printf '%s' "$out" | grep -q 'broken' || fail "qhost audit verify does not mark the day as broken: $out"
	printf '%s' "$out" | grep -q 'entry with sequence [0-9]* (line 3) does not chain from the previous entry' || fail "qhost audit verify does not name the line after the change: $out"
	printf '%s' "$out" | grep -q 'the action log of the host service is broken' || fail "qhost audit verify does not say that the log is broken: $out"
	log "scenario: qhost audit verify on a changed line: $(printf '%s' "$out" | grep broken | head -n 1)"
	host_log_rewrite "2s/\"client_uid\":4242424([,}])/\"client_uid\":$uid2\\1/" "$file" || fail "cannot restore line 2 of $file"
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the line was restored: $out"
	if printf '%s' "$out" | grep -q 'broken'; then fail "qhost audit verify still reports a broken day: $out"; fi
	[ "$(docker exec "$host" stat -c %U:%a "$file")" = qhost:600 ] || fail 'the day file lost its owner or mode'
}

# scenario_host_changes proves the changes of the host through the CLI. A folder gets other permissions with sudo
# qhost host apply, the journal lists the change with what its undo does, the undo brings the permissions back, and
# the action log names the journal entry and the person behind sudo for both. A user is created with its password on
# standard input only, put in a group, locked and deleted, with the group deleted after it. The protection rules
# refuse changing one's own account and the group sudo, a change needs root and --confirm, and neither the
# password nor anything else secret reaches the journal or the action log. It runs after the setup, when the host
# service keeps the action log.
scenario_host_changes() {
	step 'a change of the host needs root and --confirm, and the journal is closed to everyone else'
	dir=/home/$admin/e2e-host-folder
	grp=$(as_admin 'id -gn')
	as_admin "rm -rf '$dir' && mkdir '$dir' && chmod 0755 '$dir'" || fail 'cannot make the test folder'
	if out=$(as_admin "qhost host apply folder.set-permissions '$dir' $admin $grp read-write read nothing --confirm 2>&1"); then
		fail "host apply without root succeeded: $out"
	fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "host apply without root does not say that it needs root: $out"
	if out=$(as_admin 'qhost host journal 2>&1'); then fail "host journal without root succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "host journal without root does not say that it needs root: $out"
	entries_before=$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal 2>/dev/null | wc -l')
	if out=$(as_admin "sudo qhost host apply folder.set-permissions '$dir' $admin $grp read-write read nothing 2>&1"); then
		fail "host apply without --confirm succeeded: $out"
	else
		rc=$?
		[ "$rc" -eq 2 ] || fail "host apply without --confirm ended with the code $rc, want 2: $out"
	fi
	printf '%s' "$out" | grep -q -- '--confirm' || fail "host apply without --confirm does not name the flag: $out"
	printf '%s' "$out" | grep -q -- 'chmod' || fail "host apply without --confirm does not show its commands: $out"
	printf '%s' "$out" | grep -q -- 'These commands run as root' || fail "host apply without --confirm does not show its plan: $out"
	[ "$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal 2>/dev/null | wc -l')" = "$entries_before" ] ||
		fail 'host apply without --confirm wrote a journal entry'
	[ "$(as_admin "stat -c %a '$dir'")" = 755 ] || fail 'a refused change touched the folder'
	reader=$(as_user e2e-hostreader 'qhost host users 2>&1 || true')
	printf '%s' "$reader" | grep -q 'qhost is reserved for administrators (sudo)' || fail "e2e-hostreader reads the users of the host: $reader"
	out=$(as_admin 'qhost --output json host users 2>&1') || fail "qhost host users without sudo failed for an administrator: $out"
	printf '%s' "$out" | grep -q "\"name\": *\"$admin\"" || fail "qhost host users does not list $admin: $out"
	out=$(as_admin "qhost --output json host folder '$dir' 2>&1") || fail "qhost host folder failed: $out"
	printf '%s' "$out" | grep -q '"mode": *"[0-7]*755"' || fail "qhost host folder does not show the mode 755: $out"

	step 'sudo qhost host apply folder.set-permissions changes the permissions, the journal shows the entry and the undo restores them'
	admin_uid=$(docker exec "$host" id -u "$admin")
	out=$(as_admin "sudo qhost --output json host apply folder.set-permissions '$dir' $admin $grp read-write read nothing --confirm 2>&1") ||
		fail "host apply folder.set-permissions failed: $out"
	change_id=$(json_value "$(printf '%s' "$out" | tr -d ' \n')" journal_id)
	[ -n "$change_id" ] || fail "host apply reports no journal entry: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"applied"' || fail "host apply does not report the result applied: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"warning":""' || fail "host apply warns although the host service runs: $out"
	[ "$(as_admin "stat -c %a '$dir'")" = 750 ] || fail "the folder has the mode $(as_admin "stat -c %a '$dir'") after the change, want 750"
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	flat=$(printf '%s' "$journal" | tr -d ' \n')
	printf '%s' "$flat" | grep -q "\"id\":\"$change_id\"" || fail "the journal does not list $change_id: $journal"
	printf '%s' "$flat" | grep -q "\"action\":\"folder.set-permissions\"" || fail "the journal does not name the action: $journal"
	printf '%s' "$flat" | grep -q "\"sudo_uid\":$admin_uid" || fail "the journal does not name the user behind sudo ($admin_uid): $journal"
	printf '%s' "$flat" | grep -q '"reversible":true' || fail "the journal does not offer the undo: $journal"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost-host/journal)" = root:700 ] ||
		fail "the journal folder is $(docker exec "$host" stat -c %U:%a /var/lib/qhost-host/journal), want root:700"
	[ "$(docker exec "$host" stat -c %U:%a "/var/lib/qhost-host/journal/$change_id.json")" = root:600 ] ||
		fail 'the journal entry is not root:600'
	out=$(as_admin "sudo qhost --output json host undo $change_id --confirm 2>&1") || fail "host undo failed: $out"
	undo_id=$(json_value "$(printf '%s' "$out" | tr -d ' \n')" journal_id)
	[ -n "$undo_id" ] && [ "$undo_id" != "$change_id" ] || fail "host undo reports no new journal entry: $out"
	[ "$(as_admin "stat -c %a '$dir'")" = 755 ] || fail "the undo left the mode $(as_admin "stat -c %a '$dir'"), want 755"
	if out=$(as_admin "sudo qhost host undo $change_id --confirm 2>&1"); then fail "a second undo succeeded: $out"; fi
	log "scenario: folder.set-permissions $change_id changed 755 to 750, undo $undo_id restored 755"

	step 'the action log names the person behind sudo and the journal entry of the change and of its undo'
	rows=$(host_audit_rows)
	printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"folder.set-permissions$change_id\"" ||
		fail "the action log does not record $change_id for uid $admin_uid: $rows"
	printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"folder.set-permissions${undo_id}undoof$change_id\"" ||
		fail "the action log does not record the undo $undo_id of $change_id for uid $admin_uid: $rows"
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the changes: $out"
	as_admin "rm -rf '$dir'"

	step 'sudo qhost host apply file.write writes a file under /etc in place from standard input, host file shows it, the journal keeps no content and the undo restores it'
	sample=/etc/qhost-e2e-sample.conf
	docker exec "$host" sh -c "printf 'qhost-e2e-old' >'$sample' && chmod 0640 '$sample'" || fail "cannot make $sample"
	inode=$(docker exec "$host" stat -c %i "$sample")
	if out=$(printf 'qhost-e2e-new' | as_admin "sudo qhost --output json host apply file.write '$sample' --confirm 2>&1"); then
		fail "file.write under /etc without system-path succeeded: $out"
	fi
	printf '%s' "$out" | grep -q 'system path' || fail "file.write under /etc without system-path does not name the system path: $out"
	[ "$(docker exec "$host" cat "$sample")" = qhost-e2e-old ] || fail 'a refused file.write changed the file'
	out=$(printf 'qhost-e2e-new' | as_admin "sudo qhost --output json host apply file.write '$sample' system-path --confirm 2>&1") ||
		fail "host apply file.write failed: $out"
	flat=$(printf '%s' "$out" | tr -d ' \n')
	write_id=$(json_value "$flat" journal_id)
	[ -n "$write_id" ] || fail "host apply file.write reports no journal entry: $out"
	printf '%s' "$flat" | grep -q '"result":"applied"' || fail "host apply file.write does not report the result applied: $out"
	if printf '%s' "$out" | grep -q 'qhost-e2e-new'; then fail "the report of file.write shows the content: $out"; fi
	[ "$(docker exec "$host" cat "$sample")" = qhost-e2e-new ] || fail "$sample does not hold the new content"
	[ "$(docker exec "$host" stat -c %i:%a "$sample")" = "$inode:640" ] || fail "file.write did not keep the inode and the mode of $sample"
	out=$(as_admin "sudo qhost --output json host file '$sample' 2>&1") || fail "sudo qhost host file failed: $out"
	flat=$(printf '%s' "$out" | tr -d ' \n')
	[ "$(json_value "$flat" content)" = qhost-e2e-new ] || fail "host file does not show the new content: $out"
	out=$(as_admin "sudo qhost --output json host file /etc 2>&1") || fail "sudo qhost host file /etc failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"name":"qhost-e2e-sample.conf","kind":"file"' || fail "host file /etc does not list the sample file: $out"
	if out=$(as_admin "qhost host file '$sample' 2>&1"); then fail "host file without root succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "host file without root does not say that it needs root: $out"
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	flat=$(printf '%s' "$journal" | tr -d ' \n')
	printf '%s' "$flat" | grep -q "\"id\":\"$write_id\",[^}]*\"action\":\"file.write\"" || fail "the journal does not list the file.write $write_id: $journal"
	if printf '%s' "$journal" | grep -q 'qhost-e2e-\(old\|new\)'; then fail "the journal listing shows file content: $journal"; fi
	out=$(as_admin "sudo qhost --output json host undo $write_id --confirm 2>&1") || fail "host undo of file.write failed: $out"
	[ "$(docker exec "$host" cat "$sample")" = qhost-e2e-old ] || fail "the undo did not restore the previous content of $sample"
	[ "$(docker exec "$host" stat -c %i:%a "$sample")" = "$inode:640" ] || fail "the undo did not keep the inode and the mode of $sample"
	if printf '%s' "$out" | grep -q 'qhost-e2e-\(old\|new\)'; then fail "the report of the undo shows file content: $out"; fi
	docker exec "$host" rm -f "$sample"
	log "scenario: file.write $write_id wrote $sample in place, the undo restored the previous content"

	step 'sudo qhost host apply file.copy, file.remove and file.rename work on a folder tree, the undo restores it and an existing target is refused'
	tree=/srv/qhost-e2e-tree
	copy=/srv/qhost-e2e-tree-copy
	moved=/srv/qhost-e2e-tree-moved
	docker exec "$host" sh -c "rm -rf '$tree' '$copy' '$moved' && mkdir -p '$tree/sub' && printf 'qhost-e2e-top' >'$tree/top.txt' && printf 'qhost-e2e-deep' >'$tree/sub/deep.txt' && ln -s top.txt '$tree/link'" ||
		fail "cannot make $tree"
	out=$(as_admin "sudo qhost --output json host apply file.copy '$tree' '$copy' --confirm 2>&1") || fail "host apply file.copy failed: $out"
	flat=$(printf '%s' "$out" | tr -d ' \n')
	copy_id=$(json_value "$flat" journal_id)
	[ -n "$copy_id" ] || fail "host apply file.copy reports no journal entry: $out"
	printf '%s' "$flat" | grep -q '"result":"applied"' || fail "host apply file.copy does not report the result applied: $out"
	if printf '%s' "$out" | grep -q 'qhost-e2e-\(top\|deep\)'; then fail "the report of file.copy shows file content: $out"; fi
	[ "$(docker exec "$host" cat "$copy/top.txt")" = qhost-e2e-top ] || fail 'the copy does not hold top.txt'
	[ "$(docker exec "$host" cat "$copy/sub/deep.txt")" = qhost-e2e-deep ] || fail 'the copy does not hold sub/deep.txt'
	[ "$(docker exec "$host" readlink "$copy/link")" = top.txt ] || fail "the link of the copy points to $(docker exec "$host" readlink "$copy/link"), want top.txt"
	[ "$(docker exec "$host" cat "$tree/top.txt")" = qhost-e2e-top ] || fail 'file.copy changed the source'
	if out=$(as_admin "sudo qhost --output json host apply file.copy '$tree' '$copy' --confirm 2>&1"); then fail "file.copy onto an existing target succeeded: $out"; fi
	if out=$(as_admin "sudo qhost --output json host apply file.rename '$tree' '$copy' --confirm 2>&1"); then fail "file.rename onto an existing target succeeded: $out"; fi
	[ "$(docker exec "$host" cat "$tree/top.txt")" = qhost-e2e-top ] || fail 'a refused file.rename changed the source'
	out=$(as_admin "sudo qhost --output json host apply file.remove '$copy' --confirm 2>&1") || fail "host apply file.remove failed: $out"
	flat=$(printf '%s' "$out" | tr -d ' \n')
	remove_id=$(json_value "$flat" journal_id)
	[ -n "$remove_id" ] || fail "host apply file.remove reports no journal entry: $out"
	if docker exec "$host" test -e "$copy" || docker exec "$host" test -L "$copy"; then fail "$copy exists after file.remove"; fi
	if printf '%s' "$out" | grep -q 'qhost-e2e-\(top\|deep\)'; then fail "the report of file.remove shows file content: $out"; fi
	journal=$(as_admin 'sudo qhost --output json host journal 2>&1') || fail "sudo qhost host journal failed: $journal"
	flat=$(printf '%s' "$journal" | tr -d ' \n')
	printf '%s' "$flat" | grep -q "\"id\":\"$remove_id\",[^}]*\"action\":\"file.remove\"" || fail "the journal does not list the file.remove $remove_id: $journal"
	if printf '%s' "$journal" | grep -q 'qhost-e2e-\(top\|deep\)'; then fail "the journal listing shows file content: $journal"; fi
	out=$(as_admin "sudo qhost --output json host undo $remove_id --confirm 2>&1") || fail "host undo of file.remove failed: $out"
	[ "$(docker exec "$host" cat "$copy/top.txt")" = qhost-e2e-top ] || fail 'the undo did not restore top.txt'
	[ "$(docker exec "$host" cat "$copy/sub/deep.txt")" = qhost-e2e-deep ] || fail 'the undo did not restore sub/deep.txt'
	[ "$(docker exec "$host" readlink "$copy/link")" = top.txt ] || fail 'the undo did not restore the link as a link'
	if printf '%s' "$out" | grep -q 'qhost-e2e-\(top\|deep\)'; then fail "the report of the undo shows file content: $out"; fi
	out=$(as_admin "sudo qhost --output json host apply file.rename '$copy' '$moved' --confirm 2>&1") || fail "host apply file.rename failed: $out"
	rename_id=$(json_value "$(printf '%s' "$out" | tr -d ' \n')" journal_id)
	[ -n "$rename_id" ] || fail "host apply file.rename reports no journal entry: $out"
	docker exec "$host" test -d "$moved" || fail "$moved does not exist after file.rename"
	if docker exec "$host" test -e "$copy"; then fail "$copy exists after file.rename"; fi
	out=$(as_admin "sudo qhost --output json host undo $rename_id --confirm 2>&1") || fail "host undo of file.rename failed: $out"
	[ "$(docker exec "$host" cat "$copy/sub/deep.txt")" = qhost-e2e-deep ] || fail 'the undo did not move the folder back'
	if docker exec "$host" test -e "$moved"; then fail "$moved exists after the undo of file.rename"; fi
	docker exec "$host" rm -rf "$tree" "$copy" "$moved"
	log "scenario: file.copy $copy_id copied a folder tree, file.remove $remove_id removed it, the undo restored it, file.rename $rename_id moved it and the undo moved it back"

	step 'sudo qhost host apply file.remove refuses a file of the journal as locked, the file and the entries stay'
	entry="/var/lib/qhost-host/journal/$rename_id.json"
	docker exec "$host" test -f "$entry" || fail "the journal has no file $entry"
	before=$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')
	sum=$(docker exec "$host" sha256sum "$entry")
	if out=$(as_admin "sudo qhost --output json host apply file.remove '$entry' --confirm 2>&1"); then fail "file.remove of a journal file succeeded: $out"; fi
	printf '%s' "$out" | grep -q locked || fail "file.remove of a journal file does not say locked: $out"
	[ "$(docker exec "$host" sha256sum "$entry")" = "$sum" ] || fail 'the refused file.remove changed the journal file'
	[ "$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')" = "$before" ] || fail 'the refused file.remove changed the number of journal entries'

	step 'the protection rules refuse changing the own account, the group sudo and a system path, and leave no journal entry'
	before=$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')
	for refused in "user.lock $admin|you cannot lock yourself" "user.delete $admin|you cannot delete your own account" \
		"user.set-admin $admin off|from yourself" "group.delete sudo|" "group.delete qhost|" "folder.set-permissions /etc $admin $grp read-write read nothing|system path"; do
		args=${refused%%|*}
		want=${refused#*|}
		if out=$(as_admin "sudo qhost --output json host apply $args --confirm 2>&1"); then fail "the rule did not refuse 'host apply $args': $out"; fi
		printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"refused"' || fail "'host apply $args' is not reported as refused: $out"
		[ -z "$want" ] || printf '%s' "$out" | grep -q "$want" || fail "'host apply $args' does not give the reason '$want': $out"
	done
	[ "$(docker exec "$host" sh -c 'ls /var/lib/qhost-host/journal | wc -l')" = "$before" ] || fail 'a refused change wrote a journal entry'
	[ "$(docker exec "$host" stat -c %a /etc)" = 755 ] || fail 'a refused change touched /etc'
	docker exec "$host" getent group sudo >/dev/null || fail 'the group sudo is gone'

	step 'a user is created with the password on standard input, put in a group, locked and deleted'
	pw=E2e-demo-Pw-7731
	docker exec "$host" sh -c 'id e2e-hostuser >/dev/null 2>&1 && userdel -r e2e-hostuser; getent group e2e-hostgroup >/dev/null && groupdel e2e-hostgroup; rm -rf /home/e2e-hostuser; true'
	out=$(printf '{"password":"%s"}' "$pw" | docker exec -i -u "$admin" "$host" sudo qhost --output json host apply user.create e2e-hostuser --confirm 2>&1) ||
		fail "host apply user.create failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"result":"applied"' || fail "user.create is not reported as applied: $out"
	if printf '%s' "$out" | grep -q "$pw"; then fail "the report of user.create shows the password: $out"; fi
	docker exec "$host" id e2e-hostuser >/dev/null || fail 'the user e2e-hostuser does not exist after user.create'
	out=$(as_admin 'qhost --output json host user e2e-hostuser 2>&1') || fail "qhost host user failed: $out"
	printf '%s' "$out" | grep -q '"name": *"e2e-hostuser"' || fail "qhost host user does not show the new user: $out"
	out=$(as_admin 'sudo qhost --output json host apply group.create e2e-hostgroup --confirm 2>&1') || fail "host apply group.create failed: $out"
	out=$(as_admin 'sudo qhost --output json host apply user.add-groups e2e-hostuser e2e-hostgroup --confirm 2>&1') || fail "host apply user.add-groups failed: $out"
	docker exec "$host" id -nG e2e-hostuser | tr ' ' '\n' | grep -qx e2e-hostgroup || fail "e2e-hostuser is not in e2e-hostgroup: $(docker exec "$host" id -nG e2e-hostuser)"
	out=$(as_admin 'qhost --output json host group e2e-hostgroup 2>&1') || fail "qhost host group failed: $out"
	printf '%s' "$out" | tr -d ' \n' | grep -q '"members":\[[^]]*"e2e-hostuser"' || fail "qhost host group does not list e2e-hostuser: $out"
	out=$(as_admin 'sudo qhost --output json host apply user.lock e2e-hostuser --confirm 2>&1') || fail "host apply user.lock failed: $out"
	out=$(as_admin 'sudo qhost --output json host user e2e-hostuser 2>&1') || fail "sudo qhost host user failed: $out"
	printf '%s' "$out" | grep -q '"locked": *true' || fail "e2e-hostuser is not shown as locked: $out"
	[ "$(docker exec "$host" passwd -S e2e-hostuser | cut -d ' ' -f 2)" = L ] || fail 'passwd does not report e2e-hostuser as locked'
	out=$(as_admin 'sudo qhost --output json host apply user.delete e2e-hostuser --confirm 2>&1') || fail "host apply user.delete failed: $out"
	if docker exec "$host" id e2e-hostuser >/dev/null 2>&1; then fail 'the user e2e-hostuser exists after user.delete'; fi
	out=$(as_admin 'sudo qhost --output json host apply group.delete e2e-hostgroup --confirm 2>&1') || fail "host apply group.delete failed: $out"
	if docker exec "$host" getent group e2e-hostgroup >/dev/null; then fail 'the group e2e-hostgroup exists after group.delete'; fi
	docker exec "$host" rm -rf /home/e2e-hostuser

	step 'the action log lists every change of the user, and no secret reaches the journal or the action log'
	rows=$(host_audit_rows)
	for action in user.create group.create user.add-groups user.lock user.delete group.delete; do
		printf '%s' "$rows" | grep -q "\"client_uid\":$admin_uid,\"action\":\"host.change\",\"result\":\"success\"[^}]*\"change\":\"$action[0-9]" ||
			fail "the action log does not record $action for uid $admin_uid: $rows"
	done
	if docker exec "$host" grep -rqF "$pw" /var/lib/qhost-host /var/lib/qhost; then fail 'the password reached the journal or the action log'; fi
	out=$(as_admin 'qhost audit verify 2>&1') || fail "qhost audit verify failed after the user changes: $out"
	log 'scenario: user e2e-hostuser created, put in e2e-hostgroup, locked and deleted; every change is in the action log with uid '"$admin_uid"
}
