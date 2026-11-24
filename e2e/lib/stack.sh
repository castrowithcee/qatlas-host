#!/bin/sh
# Helpers of the e2e scenario for stacks, runs, findings, notifications, settings, backups and the maintenance window.
# Sourced by run.sh; defines only functions and constants.

hook_url=https://hooks.$domain/notify
hook_signing_value=demo-signing-value-e2e

tui_registry_host=quay.other.example.test

rolled_back_steps=stop,backup,pull,up,check,rollback,stop,restore,revert,push,fast-forward,up,check
# A stack without backup rules is not stopped for a backup, so its run starts at the backup step.
rolled_back_plain_steps=backup,pull,up,check,rollback,stop,restore,revert,push,fast-forward,up,check
# The rollback-failed run is one of demo-cache, which has no backup rules either.
rollback_failed_steps=backup,pull,up,check,rollback,stop,restore,revert,push,stop

# stack_field prints one flat field of a stack from qhost status (empty when it is absent).
stack_field() {
	qd --output json status | sed -n "s/.*\"stack\\.$1\\.$2\": *\"\\([^\"]*\\)\".*/\\1/p"
}

# stack_settled succeeds when the stack has no run in progress and its last run ended with the given result.
stack_settled() {
	[ -z "$(stack_field "$1" active_run)" ] && [ "$(stack_field "$1" last_run)" = "$2" ]
}

# stack_idle succeeds when the stack has no run in progress and has finished at least one.
stack_idle() {
	[ -z "$(stack_field "$1" active_run)" ] && [ -n "$(stack_field "$1" last_run)" ]
}

# run_finished succeeds when the log of the run has its result line and the stack has no run in progress.
run_finished() {
	[ -z "$(stack_field "$1" active_run)" ] && qd stack log "$2" | grep -q ' result '
}

# status_field prints one top-level field of qhost status.
status_field() {
	qd --output json status | sed -n "s/.*\"$1\": *\"\([^\"]*\)\".*/\1/p"
}

# poll_done succeeds when the service has polled since it started.
poll_done() {
	started=$(status_field started_at) polled=$(status_field last_poll)
	[ -n "$started" ] && [ -n "$polled" ] && [ "$(printf '%s\n%s\n' "$polled" "$started" | sort | head -n 1)" = "$started" ]
}

# stack_healthy succeeds when the stack runs and every container with a healthcheck is healthy.
stack_healthy() {
	[ -n "$(docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" --filter health=healthy)" ] &&
		[ -z "$(docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" --filter health=unhealthy --filter health=starting)" ]
}

# log_steps prints the run's own steps of a run log in order, joined by commas; the sub-steps of the backup
# and the restore are left out.
log_steps() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* step //p' |
		grep -x -e stop -e backup -e pull -e up -e check -e rollback -e restore -e revert -e push -e fast-forward |
		tr '\n' ',' | sed 's/,$//'
}

# newest_run prints the id of the newest recorded run of a stack (empty when there is none).
newest_run() {
	# The error column can name another run, so only the first id of the newest line counts.
	qd stack runs "$1" | grep -m 1 -o '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}' | head -n 1
}

# next_run waits for a run of the stack other than the run id $2, active or already finished, and prints its
# id. Recorded runs are listed only once they have ended, so an active run is read from the status.
next_run() {
	i=0
	while :; do
		id=$(stack_field "$1" active_run)
		[ -n "$id" ] || id=$(newest_run "$1")
		if [ -n "$id" ] && [ "$id" != "$2" ]; then
			printf '%s' "$id"
			return 0
		fi
		i=$((i + 1))
		[ "$i" -lt 400 ] || return 1
		sleep 0.3
	done
}

# stack_images prints the images of the running containers of a Compose project on the host.
stack_images() {
	docker exec "$host" docker ps --filter "label=com.docker.compose.project=$1" --format '{{.Image}}'
}

# hook_seen succeeds when the webhook receiver recorded a signed request to the path $1 with the event $2 and,
# if given, the stack $3 in its body.
hook_seen() {
	stack_pattern='"body"'
	[ -z "${3:-}" ] || stack_pattern="\\\"stack\\\":\\\"$3\\\""
	docker exec "$prefix-hooks" cat /data/requests.jsonl 2>/dev/null | grep -F "\"path\":\"$1\"" |
		grep -F "\"X-Qhost-Event\":\"$2\"" | grep -F '"X-Qhost-Signature":"' | grep -qF -- "$stack_pattern"
}

# hook_has succeeds when the webhook receiver recorded a signed request to the path $1 for the event $2 whose
# body contains every further fixed string.
hook_has() {
	lines=$(docker exec "$prefix-hooks" cat /data/requests.jsonl 2>/dev/null | grep -F "\"path\":\"$1\"" |
		grep -F "\"X-Qhost-Event\":\"$2\"" | grep -F '"X-Qhost-Signature":"') || return 1
	shift 2
	for part in "$@"; do
		lines=$(printf '%s\n' "$lines" | grep -F -- "$part") || return 1
	done
}

# fixture_run runs a script as root in a throwaway container of the backup helper image on the host, with the
# data source (a host directory or a volume name) mounted at /data.
fixture_run() {
	docker exec -i "$host" docker run --rm --network none -v "$1:/data" "$backup_image" sh -ec "$2"
}

# fixture_write_script is the script that fills /data of a helper container.
fixture_write_script() {
	printf '%s' '
		cd /data
		mkdir -p sub/deep
		printf "alpha\n" >a.txt && chown 1234:1234 a.txt && chmod 640 a.txt
		printf "root only\n" >r.txt && chown 0:0 r.txt && chmod 600 r.txt
		printf "nested\n" >sub/deep/n.bin && chown 4321:5678 sub/deep/n.bin && chmod 755 sub/deep/n.bin
		chown 2000:2000 sub && chmod 750 sub
		ln -s a.txt link'
}

# fixture_change_script alters, removes and adds entries in the current directory so the data differs from
# fixture_write in content, owners and modes.
fixture_change_script() {
	printf '%s' '
		rm -f a.txt
		printf "changed\n" >r.txt && chown 99:99 r.txt && chmod 666 r.txt
		chown 0:0 sub && chmod 777 sub
		rm -rf sub/deep
		printf "extra\n" >extra.txt'
}

backup_ids() {
	qd stack backup list "$1" | grep -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'
}

# updates_expect checks the single row of a 'qhost stack updates' JSON output: current, available, major_available
# (empty for none) and status.
updates_expect() {
	out=$1
	[ "$(json_value "$out" current)" = "$2" ] && [ "$(json_value "$out" available)" = "$3" ] &&
		[ "$(json_value "$out" major_available)" = "$4" ] && [ "$(json_value "$out" status)" = "$5" ] ||
		fail "unexpected updates, want current=$2 available=$3 major_available=$4 status=$5: $out"
	case $out in *2.1.0-rc.1*) fail "a pre-release is offered: $out" ;; esac
}

# demo_stack_settings prints the settings of demo-stack: the backup section the repository starts with and,
# if given, the update section $1, whose backslash escapes printf %b expands.
demo_stack_settings() {
	printf 'version: 1\n%bbackup:\n  paths:\n    - data\n  volumes:\n    - state\n  command: [/bin/echo, backup-command-output]\n  retention:\n    keep: 2\n' "${1:-}"
}

# settings_set replaces the settings of the stack $1 with the YAML $2 from standard input and keeps the JSON
# answer in settings_out. The service commits and pushes them and moves its working copy; no run starts.
settings_set() {
	settings_out=$(qd_in "$2" --output json stack settings set "$1" --file -) || fail "settings set of $1 failed: $settings_out"
}

# update_commit_only_version requires that a commit of the stacks repo changes one line of demo-stack/.env, the
# image variable, from one version to another.
update_commit_only_version() {
	files=$(as_admin "cd ~/stacks && git show --format= --numstat $1")
	[ "$files" = "$(printf '1\t1\tdemo-stack/.env')" ] || fail "the commit $1 touches more than one line of demo-stack/.env: $files"
	diff_lines=$(as_admin "cd ~/stacks && git show --format= -U0 $1 | grep '^[-+]' | grep -v '^\(---\|+++\)'")
	[ "$diff_lines" = "$(printf -- '-DEMO_APP_VERSION=%s\n+DEMO_APP_VERSION=%s' "$2" "$3")" ] ||
		fail "the commit $1 does not only move DEMO_APP_VERSION from $2 to $3: $diff_lines"
	[ "$(as_admin "cd ~/stacks && git log -1 --format=%s $1")" = "Update demo-stack DEMO_APP_VERSION from $2 to $3" ] ||
		fail "the commit $1 has an unexpected subject: $(as_admin "cd ~/stacks && git log -1 --format=%s $1")"
}

# update_run_backup prints the backup a run log names.
update_run_backup() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* info backup \([0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}\)$/\1/p' | head -n 1
}

# update_steps prints the steps of an update run in order, joined by commas.
update_steps() {
	printf '%s\n' "$1" | sed -n 's/^[^ ]* step //p' |
		grep -x -e stop -e backup -e commit -e push -e pull -e up -e check -e rollback -e restore -e revert -e fast-forward |
		tr '\n' ',' | sed 's/,$//'
}

# list_has_path succeeds when source list names the folder $1.
list_has_path() {
	qd --output json source list | jq -e --arg p "$1" '[.sources[].path] | index($p) != null' >/dev/null
}

# field_is succeeds when a flat field of a stack in qhost status has the given value.
field_is() {
	[ "$(stack_field "$1" "$2")" = "$3" ]
}

# running_containers prints how many containers of the stack run.
running_containers() {
	docker exec "$host" docker ps -q --filter "label=com.docker.compose.project=$1" | wc -l
}

# has_finding succeeds when the stack shows the finding $2 in qhost status.
has_finding() {
	stack_field "$1" findings | grep -q "$2"
}

# no_finding succeeds when the stack does not show the finding $2.
no_finding() {
	! has_finding "$1" "$2"
}

# window_field prints one field of the line of qhost window show for the engine $2 (default system); empty when
# the field is absent.
window_field() {
	qd --output json window show | jq -r --arg f "$1" --arg e "${2:-system}" '.[] | select(.engine == $e) | if .[$f] == null then empty else .[$f] end'
}

# window_open succeeds while the window of the system engine is open.
window_open() {
	[ "$(window_field open)" = true ]
}

# window_start_soon prints a window start (HH:MM, UTC) for a window that opens soon. --start has minute granularity,
# so the window opens at that minute's start: in 60 - S seconds when the clock reads S seconds, after "+1 minute".
# That leaves the checks between window set and the opening (several qhost calls) enough time only while S is
# at most 30; later in the minute the start is one minute further out.
window_start_soon() {
	t=$(date -u -d '+1 minute' +%H:%M:%S)
	if [ "${t##*:}" -le 30 ]; then
		printf '%s\n' "${t%:*}"
	else
		date -u -d '+2 minutes' +%H:%M
	fi
}

# window_far_away opens the window daily at the time twelve hours from now, for one minute, and prints the answer.
window_far_away() {
	qd --output json window set --days daily --zone UTC --start "$(date -u -d '+12 hours' +%H:%M)" --duration 1m
}

# stack_run_count prints the number of recorded runs of a stack.
stack_run_count() {
	qd stack runs "$1" | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-' || true
}

# window_done succeeds when the service has begun the work of an opening since last_run read $window_last_before
# and has finished it. The default window may already have run once when the service started.
window_done() {
	last=$(window_field last_run)
	[ -n "$last" ] && [ "$last" != "$window_last_before" ] && [ -z "$(window_field running)" ]
}

# interval_updated succeeds once demo-stack runs the version $1 and has no run in progress.
interval_updated() {
	[ "$(stack_images demo-stack)" = "registry.$domain/demo/app:$1" ] && [ -z "$(stack_field demo-stack active_run)" ]
}
