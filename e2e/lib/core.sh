#!/bin/sh
# Basic helpers of the e2e scenario: command runners on the host, steps, qhost calls and polling predicates.
# Sourced by run.sh; defines only functions and constants.

as_admin() {
	docker exec -i -u "$admin" "$host" sh -ec "$*"
}

# as_user runs a command on the host as the test user $1.
as_user() {
	user=$1
	shift
	docker exec -i -u "$user" "$host" sh -c "$*" </dev/null
}

# ensure_user creates the test user $1 without sudo rights and without the group qhost, if it is missing.
ensure_user() {
	docker exec "$host" id "$1" >/dev/null 2>&1 || docker exec "$host" useradd --create-home --shell /bin/bash "$1" ||
		fail "cannot create the test user $1"
}

# step logs the elapsed time since scenario_t0 as [+MM:SS] before the step name, so the name follows the first "] ".
# run.sh sets scenario_t0 before the first step of the scenario and of the acceptance run.
step() {
	scenario_elapsed=$(($(date +%s) - scenario_t0))
	log "scenario: [+$(printf '%02d:%02d' $((scenario_elapsed / 60)) $((scenario_elapsed % 60)))] $*"
}

# as_rootless runs a command on the host as $rootless_user, with the environment of its rootless Docker engine.
as_rootless() {
	as_user "$rootless_user" "export XDG_RUNTIME_DIR=/run/user/$rootless_uid DOCKER_HOST=unix:///run/user/$rootless_uid/docker.sock; $*"
}

# qd runs the qhost CLI on the host as root; stderr is merged so a failure message can be checked.
qd() {
	docker exec -i "$host" qhost "$@" 2>&1 </dev/null
}

# qd_fails runs qd, requires a non-zero exit and prints the output.
qd_fails() {
	if out=$(qd "$@"); then
		printf '%s\n' "$out"
		return 1
	fi
	printf '%s\n' "$out"
}

# admin_commit changes a file of the admin clone with a sed expression and pushes it as a new commit.
admin_commit() {
	as_admin "cd ~/stacks && git pull -q --ff-only && sed -i '$2' $1 &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am '$3' && git push -q origin main"
}

# qd_in runs qd with standard input from the first argument.
qd_in() {
	input=$1
	shift
	printf '%s' "$input" | docker exec -i "$host" qhost "$@" 2>&1
}

# journal prints the log of the deploy service.
journal() {
	docker exec "$host" journalctl -u qhost-deploy --no-pager -o cat
}

# json_value prints the first string value of a key in compact JSON (empty when the key is absent or empty).
json_value() {
	printf '%s' "$1" | grep -o "\"$2\":\"[^\"]*\"" | head -n 1 | cut -d '"' -f 4
}

# json_values prints every string value of a key in JSON output, one per line.
json_values() {
	printf '%s' "$1" | grep -o "\"$2\": *\"[^\"]*\"" | cut -d '"' -f 4
}

health_is() {
	[ "$(docker exec "$host" docker inspect -f '{{.State.Health.Status}}' "$1")" = "$2" ]
}
