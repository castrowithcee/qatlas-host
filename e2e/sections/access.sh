#!/bin/sh
# Section access: the action log of the deploy service, standing grants and the proposal of a grant.
# Needs deploy: demo-stack and demo-cache deployed and running.
# Sourced by run.sh; defines only functions and constants.

# scenario_deploy_audit proves the action log of the deploy service: a listing by the administrator without sudo and
# a restart by root appear in 'qhost audit --service deploy' with their two different users, the stack of the
# restart and the origin of the command line, and the chain of the log checks out.
scenario_deploy_audit() {
	step 'the action log of the deploy service shows a request without sudo and one with sudo with their users'
	admin_uid=$(as_admin 'id -u')
	[ "$admin_uid" != 0 ] || fail 'the administrator is root'
	out=$(as_admin 'qhost stack list 2>&1') || fail "qhost stack list without sudo failed: $out"
	out=$(qd stack restart demo-stack --yes) || fail "stack restart failed: $out"
	out=$(as_admin 'qhost --output json audit --service deploy --limit 200 2>&1') || fail "qhost audit --service deploy failed: $out"
	flat=$(printf '%s' "$out" | tr -d '\n')
	printf '%s' "$flat" | grep -q "\"command\": *\"stacks.list\"[^}]*\"client_uid\": *$admin_uid\b\|\"client_uid\": *$admin_uid\b[^}]*\"command\": *\"stacks.list\"" ||
		fail "the audit log of the deploy service has no stacks.list request of user $admin_uid: $out"
	printf '%s' "$flat" | grep -q '"command": *"stack.restart"' || fail "the audit log of the deploy service has no stack.restart request: $out"
	printf '%s' "$flat" | grep -q '"stack": *"demo-stack"' || fail "the audit log of the deploy service does not name the stack of the restart: $out"
	printf '%s' "$flat" | grep -q '"client_uid": *0\b' || fail "the audit log of the deploy service has no request of root: $out"
	printf '%s' "$flat" | grep -q '"origin": *"cli"' || fail "the audit log of the deploy service does not show the origin cli: $out"
	printf '%s' "$flat" | grep -q '"service": *"deploy"' || fail "the audit output does not name the service: $out"
	printf '%s' "$flat" | grep -q '"engine": *"system"' || fail "the audit output does not name the engine: $out"
	if printf '%s' "$out" | grep -qF -- not-a-real-secret-e2e; then fail "a secret value shows in the audit log of the deploy service: $out"; fi
	out=$(as_admin 'qhost audit verify --service deploy 2>&1') || fail "qhost audit verify --service deploy failed: $out"
	printf '%s' "$out" | grep -q 'intact' || fail "qhost audit verify --service deploy does not report the log as intact: $out"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost-deploy/audit)" = qhost-deploy:700 ] || fail 'the audit log of the deploy service has the wrong owner or mode'
	log 'scenario: the audit log of the deploy service named both users, the stack and the origin, and its chain is intact'
}

# scenario_grants proves the standing grants of demo-stack: a plan without --confirm stores nothing, sudo qhost grant
# add --confirm allows an administrator of the group qhost one action, the grant is listed by that user without sudo
# and by a stack filter, adding it without sudo is refused, the user restarts demo-stack without sudo (named in the audit
# log) while stop and a restart of demo-cache are refused, and sudo qhost grant remove --confirm takes it away, after
# which the restart is refused too.
scenario_grants() {
	step 'grant add without --confirm shows the plan and stores nothing'
	ensure_user e2e-grantee
	docker exec "$host" usermod -aG qhost,sudo e2e-grantee || fail 'cannot make e2e-grantee an administrator of the group qhost'
	if out=$(as_admin 'sudo qhost grant add e2e-grantee stack.restart demo-stack 2>&1 </dev/null'); then fail "grant add without --confirm succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'e2e-grantee' || fail "the plan does not name the user: $out"
	printf '%s' "$out" | grep -q 'stack.restart' || fail "the plan does not name the action: $out"
	printf '%s' "$out" | grep -q 'demo-stack' || fail "the plan does not name the stack: $out"
	printf '%s' "$out" | grep -q 'backup:' || fail "the plan does not show the backup state: $out"
	out=$(as_user e2e-grantee 'qhost --output json grant list 2>&1') || fail "qhost grant list without sudo failed: $out"
	[ "$(printf '%s' "$out" | jq 'length')" = 0 ] || fail "a plan stored a grant: $out"

	step 'a grant needs sudo to be added'
	if out=$(as_user e2e-grantee 'qhost grant add e2e-grantee stack.restart demo-stack --confirm 2>&1'); then fail "grant add without sudo succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'must run as root' || fail "the refusal of grant add without sudo is unclear: $out"

	step 'sudo qhost grant add --confirm stores the grant and the user lists it without sudo'
	out=$(as_admin 'sudo qhost --output json grant add e2e-grantee stack.restart demo-stack --confirm 2>&1 </dev/null') || fail "grant add failed: $out"
	grant_id=$(printf '%s' "$out" | jq -er '.id') || fail "grant add printed no id: $out"
	printf '%s' "$grant_id" | grep -Eq '^g-[0-9a-f]{8}$' || fail "the grant id $grant_id has an unexpected form"
	out=$(as_user e2e-grantee 'qhost --output json grant list 2>&1') || fail "qhost grant list without sudo failed: $out"
	printf '%s' "$out" | jq -e --arg id "$grant_id" '[.[] | select(.id == $id and .user == "e2e-grantee" and .action == "stack.restart" and .stack == "demo-stack" and .state == "valid")] | length == 1' >/dev/null ||
		fail "qhost grant list does not show the grant $grant_id as valid: $out"
	out=$(as_user e2e-grantee 'qhost --output json grant list --stack demo-cache 2>&1') || fail "qhost grant list --stack failed: $out"
	[ "$(printf '%s' "$out" | jq 'length')" = 0 ] || fail "the stack filter shows a grant of another stack: $out"

	step 'the grant lets e2e-grantee restart demo-stack without sudo and nothing else'
	ids=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)
	started_before=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	out=$(as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1') || fail "stack restart under a grant failed: $out"
	started_after=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	[ "$started_before" != "$started_after" ] || fail 'stack restart under a grant did not restart demo-stack'
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the restart under a grant'
	if out=$(as_user e2e-grantee 'qhost stack stop demo-stack --yes 2>&1'); then fail "stack stop under a restart grant succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of stack stop is unclear: $out"
	if out=$(as_user e2e-grantee 'qhost stack restart demo-cache --yes 2>&1'); then fail "stack restart of another stack under the grant succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of stack restart of another stack is unclear: $out"
	[ "$(running_containers demo-stack)" -gt 0 ] || fail 'the refused stack stop stopped demo-stack'
	[ -z "$(stack_field demo-stack stopped)" ] || fail 'the refused stack stop marked demo-stack as stopped'
	out=$(as_admin 'qhost --output json audit --service deploy --limit 200 2>&1') || fail "qhost audit --service deploy failed: $out"
	printf '%s' "$out" | jq -e --arg id "$grant_id" 'any(.[]; .command == "stack.restart" and .stack == "demo-stack" and .grant == $id)' >/dev/null ||
		fail "the audit log of the deploy service does not name the grant $grant_id on the restart: $out"

	step 'grant remove without --confirm keeps the grant and sudo qhost grant remove --confirm removes it'
	if out=$(as_admin "sudo qhost grant remove $grant_id 2>&1 </dev/null"); then fail "grant remove without --confirm succeeded: $out"; fi
	out=$(as_user e2e-grantee 'qhost --output json grant list 2>&1') || fail "qhost grant list failed after the plan: $out"
	printf '%s' "$out" | jq -e --arg id "$grant_id" 'any(.[]; .id == $id)' >/dev/null || fail "a remove without --confirm took the grant away: $out"
	as_admin "sudo qhost grant remove $grant_id --confirm 2>&1 </dev/null" >/dev/null || fail "grant remove failed for $grant_id"
	out=$(as_user e2e-grantee 'qhost --output json grant list 2>&1') || fail "qhost grant list failed after the removal: $out"
	printf '%s' "$out" | jq -e --arg id "$grant_id" 'any(.[]; .id == $id) | not' >/dev/null || fail "the grant $grant_id is still listed after the removal: $out"
	if out=$(as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1'); then fail "stack restart after the removal of the grant succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of stack restart after the removal is unclear: $out"
	log "scenario: grant $grant_id for e2e-grantee was planned, added, listed without sudo and removed"
}

# scenario_grant_proposal proves a proposal of a standing grant. e2e-grantee submits a request for stack.restart of
# demo-stack without sudo, restarting without a grant is refused, proposal show marks the reason as text of the
# submitter and shows the backup state, sudo qhost proposal approve with the fingerprint and --confirm at a terminal
# adds the grant, and then e2e-grantee restarts demo-stack without sudo. The grant is removed at the end.
scenario_grant_proposal() {
	gp_dir=$(as_admin 'mktemp -d') || fail 'cannot make the folder for the approval output'
	# The approval needs a person at a terminal, so it runs under a pseudo terminal with its output in a file; the
	# command must not hold a single quote.
	gp_tty() { as_admin "script -qec '$1' /dev/null"; }

	step 'without a grant e2e-grantee cannot restart demo-stack'
	if out=$(as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1'); then fail "stack restart without a grant succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of stack restart without a grant is unclear: $out"

	step 'e2e-grantee submits the request without sudo and proposal show marks the reason'
	content=$(jq -nc '{action: "stack.restart", stack: "demo-stack", reason: "e2e: restart after the nightly import"}')
	out=$(printf '%s' "$content" | docker exec -i -u e2e-grantee "$host" sh -c 'qhost --output json proposal submit --kind grant 2>&1') || fail "submit --kind grant failed: $out"
	gp_id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$gp_id" ] || fail "proposal submit printed no id: $out"
	out=$(as_user e2e-grantee "qhost proposal show $gp_id 2>&1") || fail "proposal show failed: $out"
	printf '%s' "$out" | grep -qi 'untrusted_reason' || fail "proposal show does not mark the reason as text of the submitter: $out"
	printf '%s' "$out" | grep -q 'e2e: restart after the nightly import' || fail "proposal show lacks the reason: $out"
	printf '%s' "$out" | grep -qi 'backup' || fail "proposal show lacks the backup state: $out"
	gp_fp=$(as_user e2e-grantee "qhost --output json proposal show $gp_id" | jq -r .fingerprint)
	[ -n "$gp_fp" ] && [ "$gp_fp" != null ] || fail 'proposal show gave no fingerprint'
	if out=$(as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1'); then fail "stack restart worked while the proposal was only open: $out"; fi

	step 'approving needs sudo, a terminal and the right fingerprint'
	if out=$(as_user e2e-grantee "qhost proposal approve $gp_id --fingerprint $gp_fp --confirm 2>&1"); then fail "approve worked without sudo: $out"; fi
	if out=$(as_admin "sudo qhost proposal approve $gp_id --fingerprint $gp_fp --confirm 2>&1"); then fail "approve worked without a terminal: $out"; fi
	if out=$(gp_tty "sudo qhost proposal approve $gp_id --fingerprint wrong --confirm 2>&1"); then fail "approve took a wrong fingerprint: $out"; fi
	if out=$(as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1'); then fail "a refused approval allowed the restart: $out"; fi

	step 'sudo qhost proposal approve with the fingerprint and --confirm adds the grant'
	gp_tty "sudo qhost --output json proposal approve $gp_id --fingerprint $gp_fp --confirm >$gp_dir/out 2>/dev/null" >/dev/null || fail 'proposal approve failed'
	out=$(as_admin "cat $gp_dir/out") || fail 'proposal approve printed no result'
	grant_id=$(printf '%s' "$out" | jq -er '.id') || fail "proposal approve printed no grant id: $out"
	printf '%s' "$out" | jq -e --arg p "$gp_id" '.proposal == $p and .user == "e2e-grantee" and .action == "stack.restart" and .stack == "demo-stack"' >/dev/null ||
		fail "the result of proposal approve is not the grant of e2e-grantee: $out"
	[ "$(prop_state "$gp_id")" = applied ] || fail "the proposal is $(prop_state "$gp_id"), want applied"
	out=$(as_user e2e-grantee 'qhost --output json grant list 2>&1') || fail "qhost grant list failed: $out"
	printf '%s' "$out" | jq -e --arg id "$grant_id" '[.[] | select(.id == $id and .user == "e2e-grantee" and .state == "valid")] | length == 1' >/dev/null ||
		fail "qhost grant list does not show the grant $grant_id: $out"

	step 'e2e-grantee restarts demo-stack without sudo'
	as_user e2e-grantee 'qhost stack restart demo-stack --yes 2>&1' >/dev/null || fail 'stack restart under the approved grant failed'
	stack_healthy demo-stack || fail 'demo-stack is not healthy after the restart under the approved grant'

	as_admin "sudo qhost grant remove $grant_id --confirm 2>&1 </dev/null" >/dev/null || fail "grant remove failed for $grant_id"
	as_admin "rm -rf $gp_dir"
	log "scenario: $gp_id was approved as the grant $grant_id and e2e-grantee restarted demo-stack without sudo"
}
