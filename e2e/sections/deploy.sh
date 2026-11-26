#!/bin/sh
# Section deploy: the stacks repository is bound with its own deploy key and a confirmed host key, the stacks are
# listed and a new commit is deployed; a commit with a failing healthcheck is rolled back with a revert commit on the
# remote. Notification channels (webhook, Slack, Telegram, e-mail) are added with qhost notify: a webhook hears of a
# deployed and of a rolled-back run, and every channel type delivers a test message. Containers, their logs and
# actions, read access without sudo, proposals and the stack actions follow.
# Needs base: repo and the admin clone ~/stacks, on which bind_source pushes and whose demo-stack 1.0.1 the stack list
# expects. Needs instance: the deploy service, and the rootless instance every later section finds running; with a
# second deploy service the interface asks for the engine of a folder (tui_keep_engine).
# Sets url, the address the source is bound to, and secret, the value of secrets.env of demo-stack and demo-cache;
# deploy_baseline leaves good, the commit the poll deployed, for scenario_deploy.
# Sourced by run.sh; defines only functions and constants.

# bind_source binds the stacks repository at /opt/stacks with its own deploy key and a confirmed host key, after
# the refusals of an unconfirmed binding, and pushes demo-cache 1.1.0 for the first deploys. It sets url.
bind_source() {
	url=git.$domain:stacks.git
	# The default window (daily 03:00) must not open in the middle of the other sections on a real clock.
	window_far_away >/dev/null || fail 'cannot move the maintenance window out of the way'

	step 'deploy key'
	out=$(as_admin 'sudo qhost --output json key generate 2>&1') || fail "key generate failed: $out"
	pub=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	shown=$(as_admin 'sudo qhost --output json key show 2>&1' | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	[ "$shown" = "$pub" ] || fail "key show differs from key generate: $shown"
	fingerprint=$(printf '%s\n' "$pub" | docker exec -i "$host" sh -c 'cat >/tmp/deploy-key.pub && ssh-keygen -lf /tmp/deploy-key.pub; rm -f /tmp/deploy-key.pub') ||
		fail "ssh-keygen cannot parse the deploy key: $fingerprint"
	case $fingerprint in *SHA256:*ED25519*) ;; *) fail "unexpected fingerprint: $fingerprint" ;; esac
	log "scenario: deploy key shown and parsed: $fingerprint"
	printf '%s\n' "$pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	log 'scenario: deploy key registered on the git server'

	step 'repo set refuses an unconfirmed host key without a terminal'
	before=$(known_hosts_sum)
	out=$(qd_fails repo set "$url" --dir /opt/stacks --create) || fail "repo set without terminal and fingerprint succeeded: $out"
	printf '%s' "$out" | grep -q -- '--fingerprint' || fail "the refusal does not name --fingerprint: $out"
	[ "$(known_hosts_sum)" = "$before" ] || fail 'a host key was pinned without confirmation'
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created although the host key was not confirmed'; fi

	step 'repo set without --dir and without --create creates no stack folder'
	out=$(qd_fails repo set "$url" --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set without --dir succeeded: $out"
	printf '%s' "$out" | grep -q -- '--dir' || fail "the refusal does not name --dir: $out"
	out=$(qd_fails repo set "$url" --dir /opt/stacks --fingerprint "$(git_ed25519_fingerprint)") ||
		fail "repo set for a missing folder without --create succeeded: $out"
	printf '%s' "$out" | grep -q -- '--create' || fail "the refusal does not name --create: $out"
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created without confirmation'; fi
	if docker exec "$host" test -e /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then fail 'a refused repo set wrote the drop-in'; fi

	step 'repo set refuses a wrong fingerprint'
	out=$(qd_fails repo set "$url" --dir /opt/stacks --create --fingerprint SHA256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA) ||
		fail "repo set with a wrong fingerprint succeeded: $out"
	[ "$(known_hosts_sum)" = "$before" ] || fail 'a host key was pinned despite a wrong fingerprint'
	if docker exec "$host" test -e /opt/stacks; then fail 'the stack folder was created despite a wrong fingerprint'; fi

	step 'repo set pins the host key and clones'
	expected=$(git_ed25519_fingerprint)
	out=$(qd --output json repo set "$url" --dir /opt/stacks --create --fingerprint "$expected") || fail "repo set failed: $out"
	printf '%s' "$out" | grep -q "$expected" || fail "repo set does not report the fingerprint $expected: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone: $out"
	docker exec "$host" test -f /opt/stacks/demo-stack/compose.yaml || fail 'the clone is missing in /opt/stacks'
	[ "$(docker exec "$host" stat -c %U:%G:%a /opt/stacks)" = qhost-deploy:qhost-deploy:755 ] ||
		fail "/opt/stacks is $(docker exec "$host" stat -c %U:%G:%a /opt/stacks), want qhost-deploy:qhost-deploy:755"
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the drop-in does not grant /opt/stacks'
	docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks || fail 'the service has no write access to /opt/stacks'
	log "scenario: repo set pinned $expected and cloned"

	step 'repo fetch sees a commit pushed by the admin'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q '"updates_available": *false' || fail "unexpected updates before the push: $out"
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/" demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Update demo-cache to 1.1.0" &&
		git push -q origin main'
	pushed=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q '"updates_available": *true' || fail "repo fetch does not see the new commit: $out"
	printf '%s' "$out" | grep -q "$pushed" || fail "repo fetch does not report the pushed head $pushed: $out"
	log "scenario: repo fetch saw $pushed"

	step 'stack list shows the demo stacks with image versions'
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	log "scenario: stack list $out"
	for want in demo-stack demo-cache DEMO_APP_VERSION=1.0.1 DEMO_APP_VERSION=1.0.0; do
		printf '%s' "$out" | grep -q -- "$want" || fail "stack list does not show $want: $out"
	done
}

# deploy_baseline makes the state the later sections build on: the first runs after the binding have ended,
# demo-shop runs, demo-stack and demo-cache have their secrets.env, the channel hook is added, a manual run
# deployed demo-stack and the poll deployed demo-cache 1.1.0. It sets secret and good.
deploy_baseline() {
	secret=not-a-real-secret-e2e

	step 'the runs started by repo set end; secrets.env is not there yet'
	wait_for 'the first run of demo-stack' 120 stack_idle demo-stack
	wait_for 'the first run of demo-cache' 120 stack_idle demo-cache
	log "scenario: first runs: demo-stack $(stack_field demo-stack last_run), demo-cache $(stack_field demo-cache last_run)"

	step 'demo-plain (docker-compose.yml, name: demo-shop, no secrets.env) is deployed as project demo-shop'
	wait_for 'the first run of demo-shop' 120 stack_idle demo-shop
	[ "$(stack_images demo-shop)" = "registry.$domain/demo/app:1.0.0" ] || fail "demo-shop does not run the image 1.0.0: $(stack_images demo-shop)"
	[ -z "$(stack_images demo-plain)" ] || fail "a project named after the folder demo-plain runs: $(stack_images demo-plain)"
	qd --output json stack list | grep -q -- demo-shop || fail 'stack list does not name demo-shop'
	if docker exec "$host" test -e /opt/stacks/demo-plain/secrets.env; then fail 'demo-plain has a secrets.env'; fi

	step 'secrets.env is created on the host, owned by the service user'
	for stack in demo-stack demo-cache; do
		docker exec -u qhost-deploy "$host" sh -ec "umask 077 && printf 'DEMO_SECRET=%s\n' '$secret' >/opt/stacks/$stack/secrets.env"
	done
	dirty=$(docker exec -u qhost-deploy "$host" git -C /opt/stacks status --porcelain 2>&1) || fail "git status failed: $dirty"
	[ -z "$dirty" ] || fail "secrets.env made the working copy dirty: $dirty"
	[ "$(docker exec "$host" stat -c %U:%a /opt/stacks/demo-stack/secrets.env)" = qhost-deploy:600 ] || fail 'secrets.env has the wrong owner or mode'

	step 'a webhook channel is added from standard input'
	out=$(qd_in "$hook_url
$hook_signing_value
" notify add hook --type webhook --events succeeded,rolled-back) || fail "notify add failed: $out"
	qd notify list | grep -q 'hook *webhook *succeeded,rolled-back' || fail "notify list does not show the webhook: $(qd notify list)"

	step 'a manual run deploys demo-stack and records the commit'
	out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	commit=$(printf '%s\n' "$out" | sed -n 's/.*"commit": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] && [ -n "$commit" ] || fail "deploy did not report a run and a commit: $out"
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	log_text=$(qd stack log "$run")
	printf '%s' "$log_text" | grep -q ' result succeeded' || fail "the run did not succeed: $log_text"
	[ "$(stack_field demo-stack deployed)" = "$commit" ] || fail "demo-stack is not recorded as deployed at $commit: $(qd --output json status)"
	[ "$(stack_images demo-stack)" = "registry.$domain/demo/app:1.0.1" ] || fail "demo-stack does not run the image 1.0.1: $(stack_images demo-stack)"
	want=$(docker exec "$host" curl -fsSI -H 'Accept: application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.docker.distribution.manifest.v2+json' \
		"https://registry.$domain/v2/demo/app/manifests/1.0.1" | tr -d '\r' | sed -n 's/^[Dd]ocker-[Cc]ontent-[Dd]igest: *//p')
	case $want in sha256:*) ;; *) fail "the test registry gives no digest for demo/app:1.0.1: $want" ;; esac
	[ "$(stack_field demo-stack image.app.repo_digest)" = "$want" ] ||
		fail "demo-stack does not record the registry digest $want of its image: $(stack_field demo-stack image.app.repo_digest)"
	docker exec "$host" docker inspect -f '{{.Config.Env}}' "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)" |
		grep -q "DEMO_SECRET=$secret" || fail 'the container did not get the value of secrets.env'
	log "scenario: deploy succeeded, run $run, deployed commit $commit"
	tag_line=$(remote_run_tag demo-stack "$run")
	[ "$(printf '%s' "$tag_line" | cut -d ' ' -f 2)" = "$commit" ] || fail "the remote has no tag qhost/<host>/demo-stack/$run on $commit: $tag_line"
	case "$(printf '%s' "$tag_line" | cut -d ' ' -f 1)" in qhost/*/demo-stack/"$run") ;; *) fail "the tag of run $run does not follow the scheme: $tag_line" ;; esac
	qd --output json stack runs demo-stack | jq -e --arg r "$run" --arg t "$(printf '%s' "$tag_line" | cut -d ' ' -f 1)" 'any(.[]; .run == $r and .tag == $t)' >/dev/null ||
		fail "qhost stack runs does not show the tag of run $run"
	as_admin "rm -rf ~/stacks-tags && git clone -q $repo ~/stacks-tags"
	[ "$(as_admin "cd ~/stacks-tags && git tag -l 'qhost/*/demo-stack/*' | tail -n 1")" = "$(printf '%s' "$tag_line" | cut -d ' ' -f 1)" ] ||
		fail "a fresh clone does not list the tag of run $run as the latest of demo-stack: $(as_admin "cd ~/stacks-tags && git tag -l 'qhost/*'")"
	wait_for 'the webhook for the deployed run' 60 hook_seen /notify succeeded demo-stack
	log "scenario: the webhook receiver was told of the succeeded run"

	step 'the engine deploys a new commit it finds by polling'
	good=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the run of demo-cache' 240 stack_deployed_at demo-cache succeeded "$good"
	[ "$(stack_images demo-cache)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-cache does not run the image 1.1.0: $(stack_images demo-cache)"
	log "scenario: poll deployed demo-cache at $good"
}

# scenario_read_access proves the read access without sudo: an administrator of the group qhost reads status,
# stacks, runs with their logs and container logs, and a change is refused without root and has no effect. A user
# of the group without sudo and a user outside the group get no data. It needs runs and running containers.
scenario_read_access() {
	step 'an administrator of the group qhost reads stacks, runs and container logs without sudo'
	run=$(newest_run demo-stack)
	[ -n "$run" ] || fail 'demo-stack has no recorded run to read'
	name=$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-stack --format '{{.Names}}' | head -n 1)
	[ -n "$name" ] || fail 'no container of demo-stack runs'
	out=$(as_admin 'qhost --output json status 2>&1') || fail "qhost status without sudo failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost status without sudo does not show demo-stack: $out"
	out=$(as_admin 'qhost stack list 2>&1') || fail "qhost stack list without sudo failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost stack list without sudo does not show demo-stack: $out"
	out=$(as_admin 'qhost stack runs 2>&1') || fail "qhost stack runs without sudo failed: $out"
	printf '%s' "$out" | grep -q "$run" || fail "qhost stack runs without sudo does not show the run $run: $out"
	out=$(as_admin "qhost stack log $run 2>&1") || fail "qhost stack log without sudo failed: $out"
	printf '%s' "$out" | grep -q ' result ' || fail "qhost stack log without sudo does not show the run $run: $out"
	out=$(as_admin "qhost container logs $name --tail 0 2>&1") || fail "qhost container logs without sudo failed: $out"
	printf '%s' "$out" | grep -q 'demo started' || fail "qhost container logs without sudo shows no data: $out"
	if printf '%s' "$out" | grep -qF -- not-a-real-secret-e2e; then fail 'a secret value shows in the container logs without sudo'; fi
	log "scenario: $admin read status, stack list, runs, the log of run $run and container logs without sudo"

	step 'a change without sudo is refused and has no effect'
	runs_before=$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	if out=$(as_admin 'qhost stack deploy demo-stack 2>&1'); then fail "qhost stack deploy without sudo succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'only root may use the service' || fail "the refusal of a change without sudo is unclear: $out"
	sleep 2
	[ -z "$(stack_field demo-stack active_run)" ] || fail "qhost stack deploy without sudo started a run: $(stack_field demo-stack active_run)"
	[ "$(qd stack runs demo-stack | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')" = "$runs_before" ] || fail 'qhost stack deploy without sudo recorded a run'

	step 'the deploy channel admits root and an administrator of the group qhost; a user of the group without sudo and a user outside the group get no data'
	out=$(qd stack list) || fail "qhost stack list as root failed: $out"
	printf '%s' "$out" | grep -q demo-stack || fail "qhost stack list as root does not show demo-stack: $out"
	ensure_user e2e-viewer
	ensure_user e2e-outsider
	docker exec "$host" usermod -aG qhost e2e-viewer || fail 'cannot add e2e-viewer to the group qhost'
	if docker exec "$host" id -nG e2e-viewer | tr ' ' '\n' | grep -qx sudo; then fail 'e2e-viewer is in the group sudo'; fi
	if docker exec "$host" id -nG e2e-outsider | tr ' ' '\n' | grep -qx qhost; then fail 'e2e-outsider is in the group qhost'; fi
	for cmd in 'qhost status' 'qhost stack list' 'qhost stack runs' "qhost stack log $run" "qhost container logs $name"; do
		if out=$(as_user e2e-viewer "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-viewer: $out"; fi
		printf '%s' "$out" | grep -q 'reserved for administrators' || fail "unexpected refusal of '$cmd' for e2e-viewer: $out"
		if printf '%s' "$out" | grep -q 'demo-stack\|demo-cache\|demo started'; then fail "'$cmd' shows data to e2e-viewer: $out"; fi
		if out=$(as_user e2e-outsider "$cmd 2>&1"); then fail "'$cmd' succeeded for e2e-outsider: $out"; fi
		printf '%s' "$out" | grep -q 'may not use the qhost service channel' || fail "unexpected refusal of '$cmd' for e2e-outsider: $out"
		if printf '%s' "$out" | grep -q 'demo-stack\|demo-cache\|demo started'; then fail "'$cmd' shows data to e2e-outsider: $out"; fi
	done
	log 'scenario: the deploy channel served root and the administrator; e2e-viewer (group qhost, no sudo) was refused as reserved for administrators and e2e-outsider (no group) was turned away at the channel'
}

# scenario_proposals proves that an administrator of the group qhost submits a proposal for a stack folder without
# sudo, sees it with list and show and withdraws it, and that the proposed folder stays untouched and unbound.
scenario_proposals() {
	step 'an administrator of the group qhost submits, lists, shows and withdraws a proposal without sudo'
	folder=$(as_admin 'echo $HOME')/proposed-stacks
	as_admin "rm -rf $folder && mkdir -p $folder/demo && printf 'services: {}\\n' > $folder/demo/compose.yaml"
	folder_before=$(as_admin "find $folder -printf '%p %y %s %T@\\n' | sort")
	[ -n "$folder_before" ] || fail 'cannot read the proposed folder'
	out=$(printf '{"path":"%s","url":"ssh://git.example.test/proposed.git"}' "$folder" | as_admin 'qhost --output json proposal submit --kind source-add' 2>&1) ||
		fail "qhost proposal submit without sudo failed: $out"
	id=$(printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1)
	[ -n "$id" ] || fail "qhost proposal submit printed no id: $out"
	printf '%s' "$out" | grep -q '"state": *"open"' || fail "the new proposal is not open: $out"
	out=$(as_admin 'qhost proposal list 2>&1') || fail "qhost proposal list without sudo failed: $out"
	printf '%s' "$out" | grep -q -- "$id" || fail "qhost proposal list does not show $id: $out"
	out=$(as_admin "qhost proposal show $id 2>&1") || fail "qhost proposal show without sudo failed: $out"
	printf '%s' "$out" | grep -qF -- "$folder" || fail "qhost proposal show does not show the folder $folder: $out"
	out=$(as_admin "qhost --output json proposal withdraw $id 2>&1") || fail "qhost proposal withdraw without sudo failed: $out"
	printf '%s' "$out" | grep -q '"state": *"withdrawn"' || fail "the proposal is not withdrawn: $out"
	out=$(as_admin "qhost --output json proposal show $id 2>&1") || fail "qhost proposal show after the withdrawal failed: $out"
	printf '%s' "$out" | grep -q '"state": *"withdrawn"' || fail "the proposal does not show as withdrawn: $out"
	[ "$(as_admin "find $folder -printf '%p %y %s %T@\\n' | sort")" = "$folder_before" ] || fail "the proposed folder $folder changed"
	out=$(as_admin 'qhost source list 2>&1') || fail "qhost source list without sudo failed: $out"
	if printf '%s' "$out" | grep -qF -- "$folder"; then fail "the proposed folder is bound: $out"; fi
	as_admin "rm -rf $folder"
	log "scenario: proposal $id was submitted, listed, shown and withdrawn without sudo; the folder is unchanged and not bound"
}

# stack_deployed_at succeeds when the stack is settled with the given result at the given deployed commit. Right
# after a restart the last result is still that of an earlier run until the poll starts the new one.
stack_deployed_at() {
	stack_settled "$1" "$2" && [ "$(stack_field "$1" deployed)" = "$3" ]
}

has_failed_commit_finding() {
	stack_field demo-cache findings | grep -q failed-commit
}

# mail_seen succeeds when the mail server recorded a message containing the fixed string $1.
mail_seen() {
	docker exec "$prefix-mail" cat /data/messages.jsonl 2>/dev/null | grep -F -- "$1" >/dev/null
}

slack_url_value=https://hooks.$domain/slack/T000/B000/demo-slack-path-e2e

bot_credential=123456:demo-bot-credential-e2e

mail_login_value=demo-mail-login-e2e

# remote_run_tag prints the tag of the run $2 of the stack $1 and its commit as "<tag> <commit>" from the remote.
remote_run_tag() {
	as_admin "git ls-remote --tags $repo 'refs/tags/qhost/*'" | awk -v suffix="/$1/$2" '
		substr($2, length($2) - length(suffix) + 1) == suffix { sub("^refs/tags/", "", $2); print $2, $1 }'
}

# tag_rejected_gone succeeds once no stack has that finding.
tag_rejected_gone() {
	! tag_rejected_seen
}

# tag_rejected_seen succeeds while some stack of the deploy service has the finding tag-rejected.
tag_rejected_seen() {
	qd --output json status | grep -q 'tag-rejected'
}

scenario_deploy() {
	# Here every stack runs its newest commit, so the poll at the restart of the repair has nothing to run.
	step 'a bound installation without the grant gets it at the next root client command'
	docker exec "$host" sh -ec 'rm -f /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf && systemctl daemon-reload' ||
		fail 'cannot remove the stack folder grant'
	if docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks; then fail 'the grant is still active after its removal'; fi
	pid=$(service_pid)
	out=$(qd status) || fail "qhost status failed without the grant: $out"
	printf '%s' "$out" | grep -qF 'has no write grant' || fail "no notice about the missing grant: $out"
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf ||
		fail 'the grant was not written again'
	docker exec "$host" systemctl show -p ReadWritePaths qhost-deploy | grep -qF /opt/stacks || fail 'the service has no write access after the repair'
	wait_for 'the service after the repair' 60 service_serving
	[ "$(service_pid)" != "$pid" ] || fail 'the repair did not restart the service'
	out=$(qd status) || fail "qhost status failed: $out"
	if printf '%s' "$out" | grep -qF 'has no write grant'; then fail "the repair repeated: $out"; fi

	step 'a commit with a failing healthcheck is rolled back with a revert commit on the remote branch'
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/" demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Break the demo-cache healthcheck" &&
		git push -q origin main'
	bad=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the rolled-back run of demo-cache' 240 stack_settled demo-cache rolled-back
	runs_out=$(qd stack runs demo-cache)
	failed_run=$(printf '%s\n' "$runs_out" | grep -m 1 -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	printf '%s\n' "$runs_out" | grep -m 1 "$failed_run" | grep -q 'rolled-back' || fail "the newest run of demo-cache is not rolled-back: $runs_out"
	failed_log=$(qd stack log "$failed_run")
	printf '%s' "$failed_log" | grep -q ' result rolled-back' || fail "the run log does not end rolled-back: $failed_log"
	printf '%s' "$failed_log" | grep -q 'unhealthy' || fail "the run log does not name the unhealthy container: $failed_log"
	[ "$(log_steps "$failed_log")" = "$rolled_back_plain_steps" ] || fail "unexpected steps of the rolled-back run: $(log_steps "$failed_log")"
	printf '%s' "$failed_log" | grep -q 'skipped: the stack has no backup rules' || fail "the rollback did not skip the data restore for a stack without backup rules: $failed_log"
	revert=$(as_admin "cd ~/stacks && git pull -q --ff-only && git rev-parse HEAD")
	[ "$revert" != "$bad" ] || fail 'no revert commit reached the remote branch'
	[ "$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)" = "$revert" ] || fail 'the revert commit is not the head of the remote branch'
	meta=$(as_admin "cd ~/stacks && git log -1 --format='%an|%ae|%cn|%ce|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|qhost|qhost@localhost.invalid|Revert demo-cache to $(printf '%s' "$good" | cut -c 1-12)|$bad" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks && git rev-parse $revert:demo-cache")" = "$(as_admin "cd ~/stacks && git rev-parse $good:demo-cache")" ] ||
		fail 'the revert commit does not restore the demo-cache folder'
	[ "$(stack_field demo-cache deployed)" = "$revert" ] || fail "demo-cache is recorded as deployed at $(stack_field demo-cache deployed), want the revert commit $revert"
	[ "$(stack_images demo-cache)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-cache does not run the previous image 1.1.0: $(stack_images demo-cache)"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the rollback'
	log "scenario: failing healthcheck: run $failed_run rolled back, bad commit $bad reverted by $revert"
	[ "$(remote_run_tag demo-cache "$failed_run" | cut -d ' ' -f 2)" = "$revert" ] || fail "the tag of the rolled-back run $failed_run is not on the revert commit $revert: $(remote_run_tag demo-cache "$failed_run")"
	wait_for 'the webhook for the rolled-back run' 60 hook_seen /notify rolled-back demo-cache
	log "scenario: the webhook receiver was told of the rolled-back run"

	step 'a rolled-back commit is not run again by the next poll'
	count=$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	[ "$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$count" ] || fail 'a rolled-back commit was run again by the poll'
	[ "$(stack_field demo-cache deployed)" = "$revert" ] || fail 'the poll changed the deployed commit of demo-cache'
	[ -z "$(stack_field demo-cache pending)" ] || fail "demo-cache has a pending commit after the rollback: $(stack_field demo-cache pending)"
	[ -z "$(stack_field demo-cache findings)" ] || fail "demo-cache has findings after the rollback: $(stack_field demo-cache findings)"

	step 'a remote that refuses the run tag gives the stack the finding tag-rejected, the run still succeeds, and the next tag clears it'
	docker exec -i "$prefix-git" sh -ec 'cat >/srv/git/stacks.git/hooks/update && chmod 755 /srv/git/stacks.git/hooks/update' <<'HOOK'
#!/bin/sh
case "$1" in refs/tags/qhost/*) echo 'tag refused by the test hook' >&2; exit 1 ;; esac
HOOK
	# A manual run deploys the head of the source, which the demo-cache commits moved past the last demo-stack run.
	want=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] || fail "deploy did not report a run: $out"
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "a refused tag made the run fail: $(qd stack log "$run")"
	wait_for 'the finding tag-rejected' 30 tag_rejected_seen
	[ -z "$(remote_run_tag demo-stack "$run")" ] || fail "the remote holds the tag of run $run although the hook refuses it"
	[ "$(stack_field demo-stack deployed)" = "$want" ] || fail 'a refused tag changed the deployed commit of demo-stack'
	qd --output json stack runs demo-stack | jq -e --arg r "$run" 'any(.[]; .run == $r and .result == "succeeded" and (.tag // "") == "")' >/dev/null ||
		fail "the run $run does not show as succeeded without a tag"
	docker exec "$prefix-git" rm -f /srv/git/stacks.git/hooks/update || fail 'cannot remove the tag hook'
	out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] || fail "deploy did not report a run: $out"
	wait_for "the run $run" 240 run_finished demo-stack "$run"
	wait_for 'the finding tag-rejected to go' 30 tag_rejected_gone
	[ "$(remote_run_tag demo-stack "$run" | cut -d ' ' -f 2)" = "$want" ] || fail "the remote has no tag for run $run on $want after the hook was removed"
	log "scenario: a refused tag raised tag-rejected and the next run cleared it"

	step 'with a retention of 1 only the newest own run tag of demo-stack stays in the remote; foreign tags stay'
	settings_set demo-stack "$(demo_stack_settings | sed 's/keep: 2/keep: 1/')"
	own_host=$(printf '%s' "$(remote_run_tag demo-stack "$run" | cut -d ' ' -f 1)" | cut -d / -f 2)
	foreign_tags="qhost/other-host/demo-stack/20200101T000000Z-00000000 manual-e2e-tag"
	as_admin "rm -rf ~/stacks-tags && git clone -q $repo ~/stacks-tags"
	for tag in $foreign_tags; do
		as_admin "cd ~/stacks-tags && git tag $tag HEAD && git push -q origin refs/tags/$tag" || fail "cannot set the foreign tag $tag"
	done
	for i in 1 2; do
		out=$(qd --output json stack deploy demo-stack) || fail "deploy failed: $out"
		run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
		[ -n "$run" ] || fail "deploy did not report a run: $out"
		wait_for "the run $run" 240 run_finished demo-stack "$run"
		qd stack log "$run" | grep -q ' result succeeded' || fail "the run $run did not succeed: $(qd stack log "$run")"
	done
	own=$(as_admin "git ls-remote --tags $repo 'refs/tags/qhost/$own_host/demo-stack/*'" | awk '{ print $2 }')
	[ "$own" = "refs/tags/qhost/$own_host/demo-stack/$run" ] || fail "the remote does not hold only the tag of run $run for demo-stack: $own"
	for tag in $foreign_tags; do
		[ -n "$(as_admin "git ls-remote --tags $repo refs/tags/$tag")" ] || fail "the foreign tag $tag was deleted"
	done
	tag_rejected_gone || fail 'tag pruning raised tag-rejected'
	settings_set demo-stack "$(demo_stack_settings)"
	log "scenario: retention 1 left $own and the foreign tags in the remote"

	step 'no run log holds a secret value'
	ids=$(qd stack runs | grep -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}')
	n=0
	for id in $ids; do
		n=$((n + 1))
		if qd stack log "$id" | grep -qF -- "$secret"; then fail "the log of run $id holds the secret value"; fi
	done
	if qd stack log "$(newest_run demo-shop)" | grep -qF -- 'not-a-real-secret'; then fail 'the run log of demo-shop holds its DEMO_PASSWORD value'; fi
	[ "$n" -ge 3 ] || fail "too few runs to check the logs: $n"
	if qd stack runs | grep -qF -- "$secret"; then fail 'qhost stack runs shows the secret value'; fi
	log "scenario: the secret value is in none of $n run logs"

	scenario_notify
}

# scenario_notify proves that every channel type delivers a test message, that a channel is managed with the
# documented errors, and that no secret part of a channel shows in the listing, the run logs or the journal.
scenario_notify() {
	step 'notify test delivers through the webhook channel'
	out=$(qd notify test hook) || fail "notify test hook failed: $out"
	hook_seen /notify test || fail 'the webhook receiver did not record the test message'

	step 'notify test delivers through a Slack channel'
	out=$(qd_in "$slack_url_value
" notify add chat --type slack) || fail "notify add chat failed: $out"
	out=$(qd notify test chat) || fail "notify test chat failed: $out"
	docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F '"path":"/slack/T000/B000/demo-slack-path-e2e"' | grep -qF 'qhost test message' ||
		fail 'the receiver did not record the Slack test message'

	step 'notify test delivers through a Telegram channel'
	out=$(qd_in "$bot_credential
" notify add bot --type telegram --chat-id 4242 --base-url "https://hooks.$domain/telegram") || fail "notify add bot failed: $out"
	out=$(qd notify test bot) || fail "notify test bot failed: $out"
	docker exec "$prefix-hooks" cat /data/requests.jsonl | grep -F "\"path\":\"/telegram/bot$bot_credential/sendMessage\"" | grep -qF '4242' ||
		fail 'the receiver did not record the Telegram test message'

	step 'notify test delivers through an SMTP channel with STARTTLS and a login'
	out=$(qd_in "$mail_login_value
" notify add mail --type smtp --host "mail.$domain" --user qhost-mailer --from qhost@example.test \
		--to ops@example.test) || fail "notify add mail failed: $out"
	out=$(qd notify test mail) || fail "notify test mail failed: $out"
	mail_seen 'Subject: qhost test message' || fail 'the mail server did not record the test message'
	mail_seen '"to":["TO:\u003cops@example.test\u003e"]' || fail 'the mail server did not record the recipient'
	mail_seen '"user":"qhost-mailer"' || fail 'the mail server did not record the login name'

	step 'the channel commands report their errors'
	if out=$(qd_in "$slack_url_value
" notify add hook --type slack); then fail 'a duplicate channel was added'; fi
	printf '%s' "$out" | grep -q 'exists' || fail "the duplicate was not explained: $out"
	out=$(qd_fails notify test missing) || fail 'a missing channel was tested'
	printf '%s' "$out" | grep -q 'no such channel' || fail "the missing channel was not explained: $out"
	out=$(qd_fails notify remove missing --yes) || fail 'a missing channel was removed'
	if out=$(qd_in "$slack_url_value
" notify add bad --type slack --events exploded); then fail 'an unknown event was accepted'; fi
	printf '%s' "$out" | grep -q 'event' || fail "the unknown event was not explained: $out"
	if qd notify list | grep -q '^bad '; then fail 'a refused channel was stored'; fi

	step 'no secret part of a channel shows in the listing, the run logs or the journal'
	listing=$(qd notify list; qd --output json notify list)
	runs=$(qd stack runs)
	for id in $(qd stack runs | grep -o '^[0-9]\{8\}T[0-9]\{6\}Z-[0-9a-f]\{8\}'); do
		runs="$runs
$(qd stack log "$id")"
	done
	journal_text=$(journal)
	for value in "$hook_url" "$hook_signing_value" "$slack_url_value" demo-slack-path-e2e "$bot_credential" "$mail_login_value"; do
		for place in listing runs journal_text; do
			eval "text=\$$place"
			if printf '%s' "$text" | grep -qF -- "$value"; then fail "a secret part of a channel shows in $place"; fi
		done
	done
	[ -n "$journal_text" ] || fail 'the journal of the service is empty'

	step 'removing the channels removes their stored secrets'
	for name in hook chat bot mail; do
		out=$(qd notify remove "$name" --yes) || fail "notify remove $name failed: $out"
	done
	if qd notify list | grep -q 'webhook\|slack\|telegram\|smtp'; then fail "channels remain: $(qd notify list)"; fi
	left=$(docker exec "$host" sh -c 'ls /var/lib/qhost-deploy/credentials 2>/dev/null | grep -c "^notify" || true')
	[ "$left" = 0 ] || fail "$left notification credentials remain after the channels were removed"
	log 'scenario: all four channel types delivered a test message and no secret part showed anywhere'
}

# scenario_containers proves that qhost container list and show tell the three kinds apart on the host: the
# deployed demo stack is own, a Compose project from a folder outside the stacks repository is foreign, and a
# container started with docker run is single. The planted environment value must not show anywhere. What the
# scenario starts is removed before the answers are checked, so a failed check leaves nothing behind.
scenario_containers() {
	step 'container list shows own stacks, foreign projects and single containers'
	image=registry.$domain/demo/app:1.0.0
	foreign=$prefix-foreign single=$prefix-single planted=invented-planted-value-e2e
	foreign_dir=/tmp/$foreign
	docker exec "$host" sh -ec "rm -rf '$foreign_dir' && mkdir -p '$foreign_dir'" || fail 'cannot prepare the foreign project'
	printf 'services:\n  app:\n    image: %s\n    labels:\n      - %s\n' "$image" "$label" |
		docker exec -i "$host" sh -ec "cat >'$foreign_dir/compose.yaml'" || fail 'cannot write the compose file of the foreign project'
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$single" --label "$label" -e "E2E_PLANTED=$planted" "$image" >/dev/null ||
		fail 'cannot start the single container'
	docker exec "$host" docker compose -f "$foreign_dir/compose.yaml" -p "$foreign" up -d >/dev/null 2>&1 ||
		fail 'cannot start the foreign project'
	wait_for 'the single container' 30 health_is "$single" healthy
	wait_for 'the foreign project' 30 stack_healthy "$foreign"

	list_rc=0 show_rc=0 table_rc=0 missing_rc=0
	list_json=$(qd --output json container list) || list_rc=$?
	show_json=$(qd --output json container show "$single") || show_rc=$?
	list_table=$(qd container list) || table_rc=$?
	missing_out=$(qd container show "$prefix-no-such-container") || missing_rc=$?
	docker exec "$host" docker compose -f "$foreign_dir/compose.yaml" -p "$foreign" down >/dev/null 2>&1 || true
	docker exec "$host" sh -c "docker rm -f '$single' >/dev/null 2>&1; rm -rf '$foreign_dir'"

	[ "$list_rc" -eq 0 ] || fail "container list failed: $list_json"
	[ "$show_rc" -eq 0 ] || fail "container show failed: $show_json"
	[ "$table_rc" -eq 0 ] || fail "container list (table) failed: $list_table"
	[ "$missing_rc" -ne 0 ] || fail "container show of a missing container succeeded: $missing_out"
	printf '%s' "$list_json" | grep -q '"kind": *"own", *"engine": *"system", *"name": *"demo-stack"' || fail "demo-stack is not an own group of the system engine: $list_json"
	printf '%s' "$list_json" | grep -q "\"kind\": *\"foreign\", *\"engine\": *\"system\", *\"name\": *\"$foreign\"" || fail "$foreign is not a foreign group of the system engine: $list_json"
	printf '%s' "$list_json" | grep -q "\"working_dir\": *\"$foreign_dir\"" || fail "the foreign group does not show its working directory: $list_json"
	printf '%s' "$list_json" | grep -q "\"kind\": *\"single\", *\"engine\": *\"system\", *\"name\": *\"$single\"" || fail "$single is not a single group of the system engine: $list_json"
	printf '%s' "$list_table" | grep -q "^single  *system  *$single " || fail "the table does not show $single as single of the system engine: $list_table"
	printf '%s' "$show_json" | grep -q '"engine": *"system"' || fail "container show does not name the engine: $show_json"
	printf '%s' "$show_json" | grep -q '"E2E_PLANTED"' || fail "container show does not name the planted variable: $show_json"
	printf '%s' "$show_json" | grep -q '"kind": *"single"' || fail "container show does not name the kind: $show_json"
	printf '%s' "$missing_out" | grep -q 'no such container' || fail "the refusal of a missing container is unclear: $missing_out"
	for text in "$list_json" "$show_json" "$list_table"; do
		if printf '%s' "$text" | grep -qF -- "$planted"; then fail 'an environment value shows in the container output'; fi
	done
	log 'scenario: container list showed own, foreign and single; the planted value stayed hidden'
}

# scenario_container_logs proves that qhost container logs reads one container and a whole stack, follows until
# interrupted with exit code 0, and shows the stack's secret only masked although the demo app prints it.
scenario_container_logs() {
	step 'container logs show a container and a stack, follow ends cleanly, secrets stay masked'
	secret=not-a-real-secret-e2e
	name=$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-stack --format '{{.Names}}' | head -n 1)
	[ -n "$name" ] || fail 'no container of demo-stack runs'
	docker exec "$host" docker logs "$name" 2>&1 | grep -qF -- "$secret" || fail 'the demo app does not print its secret, so the mask cannot be checked'

	tail_rc=0 one_rc=0 stack_rc=0 follow_rc=0 json_rc=0 missing_rc=0
	tail_out=$(qd container logs "$name" --tail 20) || tail_rc=$?
	one_out=$(qd container logs "$name" --tail 0) || one_rc=$?
	stack_out=$(qd container logs --stack demo-stack --tail 0) || stack_rc=$?
	json_out=$(qd --output json container logs "$name" --tail 5) || json_rc=$?
	# --foreground: otherwise GNU timeout also signals the process group, so qhost gets the interrupt twice and the
	# second one can kill it (exit 130) after the first has ended the follow.
	follow_out=$(docker exec "$host" timeout --foreground --preserve-status -s INT 6 qhost container logs "$name" --tail 1 --follow 2>&1 </dev/null) || follow_rc=$?
	missing_out=$(qd container logs --stack "$prefix-no-such-stack") || missing_rc=$?

	[ "$tail_rc" -eq 0 ] || fail "container logs --tail 20 failed: $tail_out"
	[ "$(printf '%s\n' "$tail_out" | grep -c .)" -ge 1 ] || fail "container logs --tail 20 shows no line: $tail_out"
	[ "$one_rc" -eq 0 ] || fail "container logs failed: $one_out"
	[ "$stack_rc" -eq 0 ] || fail "container logs --stack failed: $stack_out"
	[ "$json_rc" -eq 0 ] || fail "container logs (json) failed: $json_out"
	[ "$follow_rc" -eq 0 ] || fail "container logs --follow did not end with exit code 0 after the interrupt ($follow_rc): $follow_out"
	[ "$missing_rc" -ne 0 ] || fail "container logs of a missing stack succeeded: $missing_out"
	printf '%s' "$one_out" | grep -q 'demo started' || fail "container logs lacks the start line: $one_out"
	printf '%s' "$one_out" | grep -q '\[stderr\] demo warning on stderr' || fail "container logs does not mark stderr: $one_out"
	printf '%s' "$stack_out" | grep -q "$name .*demo started" || fail "the stack log does not name the container: $stack_out"
	printf '%s' "$json_out" | grep -q '"stream": *"stdout"' || fail "the json log does not name the stream: $json_out"
	[ "$(printf '%s\n' "$follow_out" | grep -c .)" -ge 2 ] || fail "--follow showed no new line within six seconds: $follow_out"
	printf '%s' "$missing_out" | grep -q 'no such stack' || fail "the refusal of a missing stack is unclear: $missing_out"
	for text in "$tail_out" "$one_out" "$stack_out" "$json_out" "$follow_out"; do
		if printf '%s' "$text" | grep -qF -- "$secret"; then fail 'a secret value shows in the container logs'; fi
	done
	log 'scenario: container logs read a container and a stack, followed until the interrupt; the secret stayed masked'
}

# scenario_container_actions proves that qhost container stop, start and restart act on a single container and
# that the question needs --yes without a terminal. The container is removed before the answers are checked.
scenario_container_actions() {
	step 'container stop, start and restart act on a single container'
	image=registry.$domain/demo/app:1.0.0
	single=$prefix-single-act
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$single" --label "$label" "$image" >/dev/null ||
		fail 'cannot start the single container'
	wait_for 'the single container' 30 health_is "$single" healthy

	refuse_rc=0 stop_rc=0 start_rc=0 restart_rc=0 missing_rc=0
	refuse_out=$(qd container stop "$single") || refuse_rc=$?
	refuse_state=$(qd --output json container show "$single")
	stop_out=$(qd container stop "$single" --yes) || stop_rc=$?
	stop_state=$(qd --output json container show "$single")
	start_out=$(qd container start "$single" --yes) || start_rc=$?
	start_state=$(qd --output json container show "$single")
	started_before=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$single")
	restart_out=$(qd container restart "$single" --yes) || restart_rc=$?
	restart_state=$(qd --output json container show "$single")
	started_after=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' "$single")
	missing_out=$(qd container stop "$prefix-no-such-container" --yes) || missing_rc=$?
	docker exec "$host" docker rm -f "$single" >/dev/null 2>&1

	[ "$refuse_rc" -ne 0 ] || fail "container stop without --yes succeeded: $refuse_out"
	printf '%s' "$refuse_out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $refuse_out"
	printf '%s' "$refuse_state" | grep -q '"engine_state": *"running"' || fail "a refused stop changed the container: $refuse_state"
	[ "$stop_rc" -eq 0 ] || fail "container stop failed: $stop_out"
	printf '%s' "$stop_state" | grep -q '"state": *"stopped"' || fail "the container is not stopped after container stop: $stop_state"
	[ "$start_rc" -eq 0 ] || fail "container start failed: $start_out"
	printf '%s' "$start_state" | grep -q '"engine_state": *"running"' || fail "the container does not run after container start: $start_state"
	[ "$restart_rc" -eq 0 ] || fail "container restart failed: $restart_out"
	printf '%s' "$restart_state" | grep -q '"engine_state": *"running"' || fail "the container does not run after container restart: $restart_state"
	[ "$started_before" != "$started_after" ] || fail 'container restart did not restart the container'
	[ "$missing_rc" -ne 0 ] || fail "container stop of a missing container succeeded: $missing_out"
	printf '%s' "$missing_out" | grep -q 'no such container' || fail "the refusal of a missing container is unclear: $missing_out"
	log 'scenario: container stop, start and restart acted on a single container'
}

# scenario_stack_actions proves that a stop through qhost keeps demo-cache stopped against a new commit and
# the service restart that makes the poll run, that only a start clears the mark and lets the next poll deploy
# the commit, and that a restart restarts demo-stack. Both stacks run again at the end.
scenario_stack_actions() {
	step 'stack stop asks first, stops demo-cache and shows the mark in status and stack list'
	refuse_rc=0
	refuse_out=$(qd stack stop demo-cache) || refuse_rc=$?
	[ "$refuse_rc" -ne 0 ] || fail "stack stop without --yes succeeded: $refuse_out"
	printf '%s' "$refuse_out" | grep -q -- '--yes' || fail "the refusal does not name --yes: $refuse_out"
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'a refused stack stop stopped demo-cache'
	out=$(qd stack stop demo-cache --yes) || fail "stack stop failed: $out"
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache still runs after stack stop'
	[ -n "$(docker exec "$host" docker ps -aq --filter label=com.docker.compose.project=demo-cache)" ] || fail 'stack stop removed the containers of demo-cache'
	[ -n "$(stack_field demo-cache stopped)" ] || fail "status does not show demo-cache as stopped: $(qd --output json status)"
	qd stack list | grep demo-cache | grep -q 'stopped since' || fail "stack list does not show demo-cache as stopped: $(qd stack list)"
	[ -z "$(stack_field demo-stack stopped)" ] || fail 'demo-stack is shown as stopped'
	out=$(qd stack restart demo-cache --yes) && fail "stack restart of a stopped stack succeeded: $out"
	printf '%s' "$out" | grep -q 'qhost stack start' || fail "the refusal does not name stack start: $out"

	out=$(qd stack deploy demo-cache) && fail "stack deploy of a stopped stack succeeded: $out"
	printf '%s' "$out" | grep -q 'qhost stack start' || fail "the refusal of stack deploy does not name stack start: $out"
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'a refused stack deploy started demo-cache'

	step 'a new commit does not start a run of the stopped stack, also not after a service restart'
	count=$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')
	as_admin 'cd ~/stacks && git pull -q --ff-only &&
		printf "# stopped-stack marker\n" >>demo-cache/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Touch demo-cache while it is stopped" &&
		git push -q origin main'
	pushed=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	restart_service
	wait_for 'the poll after the restart' 60 poll_done
	wait_for 'the pending commit of demo-cache' 60 field_is demo-cache pending "$pushed"
	[ "$(qd stack runs demo-cache | grep -c '[0-9]\{8\}T[0-9]\{6\}Z-')" = "$count" ] || fail 'a run of the stopped stack was started by the poll'
	[ "$(running_containers demo-cache)" -eq 0 ] || fail 'demo-cache runs again after a new commit'
	[ -n "$(stack_field demo-cache stopped)" ] || fail 'the stopped mark did not survive the service restart'
	stack_field demo-cache findings | grep -q stopped || fail "demo-cache has no finding stopped: $(qd --output json status)"

	step 'stack start clears the mark, starts demo-cache and the next poll deploys the commit'
	out=$(qd stack start demo-cache --yes) || fail "stack start failed: $out"
	[ -z "$(stack_field demo-cache stopped)" ] || fail "status still shows demo-cache as stopped: $(qd --output json status)"
	[ "$(running_containers demo-cache)" -gt 0 ] || fail 'demo-cache does not run after stack start'
	stack_healthy demo-cache || fail 'demo-cache is not healthy after stack start'
	restart_service
	wait_for 'the run of demo-cache after the start' 240 stack_idle demo-cache
	wait_for 'the commit deployed on demo-cache' 240 field_is demo-cache deployed "$pushed"
	stack_healthy demo-cache || fail 'demo-cache is not healthy after the run'
	log "scenario: demo-cache stayed stopped against commit $pushed until stack start"

	step 'stack restart restarts demo-stack'
	ids=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-stack)
	started_before=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	out=$(qd stack restart demo-stack --yes) || fail "stack restart failed: $out"
	started_after=$(docker exec "$host" docker inspect -f '{{.State.StartedAt}}' $ids)
	[ "$started_before" != "$started_after" ] || fail 'stack restart did not restart demo-stack'
	stack_healthy demo-stack || fail 'demo-stack is not healthy after stack restart'
	out=$(qd stack stop nope --yes) && fail "stack stop of an unknown stack succeeded: $out"
	printf '%s' "$out" | grep -q 'no such stack' || fail "the refusal of an unknown stack is unclear: $out"
	log 'scenario: stack stop, start and restart passed'
}
