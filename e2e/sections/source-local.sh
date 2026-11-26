#!/bin/sh
# Section source-local: local folders without a remote as sources, a private registry and folders of files.
# Needs sources: url2 and the clone /opt/stacks2, which scenario_source_local releases and takes over.
# Sourced by run.sh; defines only functions and constants.

# local_tree prints mode, owner, group and size of every path below /srv/qd-local except .git, which the service
# may touch while it polls.
local_tree() {
	docker exec "$host" find /srv/qd-local -path /srv/qd-local/.git -prune -o -printf '%p %m %u:%g %s\n' | sort
}

# local_git runs git on /srv/qd-local as the person $admin; stderr is merged.
local_git() {
	docker exec -u "$admin" "$host" git -C /srv/qd-local "$@" 2>&1
}

# live_containers prints the id and the start time of every container of demo-live.
live_containers() {
	docker exec "$host" sh -c "docker ps -q --filter label=com.docker.compose.project=demo-live | sort | xargs -r docker inspect -f '{{.Id}} {{.State.StartedAt}}'"
}

# scenario_source_local proves source add, list and remove on a local repository without a remote below
# /srv/qd-local, which no stack folder uses: the service gets the access it needs, a version committed by a person is
# deployed, a failing one is rolled back by a local revert commit, and removing the source leaves the files and the
# containers alone. Then the existing clone /opt/stacks2 is released and taken over again with its origin and key.
scenario_source_local() {
	base=/srv/qd-local
	image=registry.$domain/demo/app

	step 'a local repository without a remote is prepared as root, with an unversioned secrets file'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/demo-local' && chmod 0755 '$base'" || fail 'cannot create the local folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/demo-local/compose.yaml'"
name: demo-local

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-local
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/demo-local/.env'"
	# demo-live pins its image and is started by hand before the source is added, as an existing stack would be.
	docker exec "$host" sh -ec "mkdir -p '$base/demo-live'"
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/demo-live/compose.yaml'"
name: demo-live

services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-live
      DEMO_VERSION: 1.0.0
      DEMO_HEALTH: ok
EOF2
	printf 'secrets.env\ndata/\n' | docker exec -i "$host" sh -ec "cat >'$base/demo-local/.gitignore'"
	docker exec "$host" sh -ec "cd '$base' && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-local stack' &&
		printf 'DEMO_SECRET=not-a-real-secret\n' >demo-local/secrets.env && chgrp qhost-deploy demo-local/secrets.env &&
		chmod 0640 demo-local/secrets.env" ||
		fail 'cannot create the local repository'
	[ -z "$(docker exec "$host" git -C "$base" remote)" ] || fail 'the local repository has a remote'
	secret_before=$(docker exec "$host" stat -c '%U:%G %a' "$base/demo-local/secrets.env")
	[ "$secret_before" = 'root:qhost-deploy 640' ] || fail "unexpected secrets file: $secret_before"

	step 'source list shows the earlier sources and not the local folder'
	out=$(qd --output json source list) || fail "source list failed: $out"
	printf '%s' "$out" | jq -e '[.sources[].path] | (index("/opt/stacks") != null and index("/opt/stacks2") != null and index("/srv/qd-local") == null)' >/dev/null ||
		fail "unexpected source list: $out"
	out=$(qd source list) || fail "source list as text failed: $out"
	printf '%s' "$out" | grep -qi 'source.opt-stacks.path' || fail "the text list does not show opt-stacks: $out"

	step 'source add without a terminal and without --yes is refused and changes nothing'
	dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$base")
	out=$(qd_fails source add "$base") || fail "source add without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add without --yes does not ask for confirmation: $out"
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base")" = "$dir_before" ] || fail 'a refused source add changed the folder'
	if docker exec "$host" grep -qF "$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'a refused source add wrote the drop-in'
	fi
	if list_has_path "$base"; then fail 'a refused source add bound the folder'; fi

	step 'demo-live is started by hand before the source is added'
	docker exec "$host" docker compose --project-directory "$base/demo-live" up -d >/dev/null 2>&1 || fail 'cannot start demo-live by hand'
	live_before=$(live_containers)
	[ -n "$live_before" ] || fail 'demo-live has no running container'

	step 'source add --yes binds the repository as a local source and gives the service the group access'
	# qd merges stderr, where the preview and the notices go; the JSON answer is the last line.
	out=$(qd --output json source add "$base" --yes) || fail "source add failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .path == "/srv/qd-local"' >/dev/null || fail "unexpected source add result: $out"
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(docker exec "$host" stat -c '%U:%G' "$base")" = root:qhost-deploy ] || fail "the folder has the wrong owner or group: $(docker exec "$host" stat -c '%U:%G' "$base")"
	case $(docker exec "$host" stat -c '%A' "$base") in d???rws*) ;; *) fail "the folder is not group-writable with setgid: $(docker exec "$host" stat -c '%A' "$base")" ;; esac
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base/demo-local/secrets.env")" = "$secret_before" ] || fail 'source add changed the unversioned secrets file'
	[ "$(docker exec "$host" git -C "$base" config core.sharedRepository)" = group ] || fail 'core.sharedRepository is not group'
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /srv/qd-local'
	list_has_path "$base" || fail "source list does not show the local folder: $(qd --output json source list)"
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the local source has the wrong kind'

	step 'demo-local is deployed at 1.0.0'
	i=0
	until stack_settled demo-local succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-local last_run)" != failed ] || fail "the first run of demo-local failed: $(qd stack log "$(newest_run demo-local)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-local: $(qd --output json status | tr ',' '\n' | grep -e qd-local -e demo-local)"
		sleep 1
	done
	[ "$(stack_images demo-local)" = "$image:1.0.0" ] || fail "demo-local does not run 1.0.0: $(stack_images demo-local)"

	step 'the first run of the running demo-live neither stops nor recreates it, and a stack with backup rules still stops before its backup'
	i=0
	until stack_settled demo-live succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-live last_run)" != failed ] || fail "the first run of demo-live failed: $(qd stack log "$(newest_run demo-live)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-live: $(qd --output json status | tr ',' '\n' | grep -e qd-local -e demo-live)"
		sleep 1
	done
	run_log=$(qd stack log "$(newest_run demo-live)")
	[ "$(log_steps "$run_log")" = backup,pull,up,check ] || fail "unexpected steps of the first run of demo-live: $(log_steps "$run_log")"
	if printf '%s\n' "$run_log" | grep -q '^[^ ]* step stop$'; then fail "the first run of demo-live stopped the stack: $run_log"; fi
	[ "$(live_containers)" = "$live_before" ] || fail "the first run changed the containers of demo-live: before $live_before, now $(live_containers)"
	# The runs of demo-stack with its backup rules keep the stop before the backup: see the good run of scenario_backup.

	step 'a person in the group qhost-deploy commits a version; git refuses the folder until it is marked safe'
	docker exec "$host" usermod -aG qhost-deploy "$admin" || fail 'cannot add the admin to the group qhost-deploy'
	as_admin 'id -nG' | tr ' ' '\n' | grep -qx qhost-deploy || fail "the admin has no group qhost-deploy in a new session: $(as_admin 'id -nG')"
	out=$(local_git rev-parse HEAD) && fail "git accepted a folder of another owner without safe.directory: $out"
	printf '%s' "$out" | grep -q 'dubious ownership' || fail "git refused the folder for another reason: $out"
	as_admin "git config --global --add safe.directory $base" || fail 'cannot mark the folder safe'
	local_git rev-parse HEAD >/dev/null || fail "git still refuses the folder: $(local_git rev-parse HEAD)"
	out=$(as_admin "umask 022; mkdir $base/demo-local/probe && stat -c %A $base/demo-local/probe && rmdir $base/demo-local/probe")
	case $out in d???r-s*) ;; *) fail "a folder made with umask 022 is group-writable: $out" ;; esac
	prev=$(newest_run demo-local)
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' demo-local/.env &&
		mkdir demo-local/notes && echo 'kept by a person' >demo-local/notes/info.txt &&
		stat -c %A demo-local/notes | grep -q '^d...rws' && git add -A &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Update demo-local to 1.1.0'" ||
		fail 'the admin could not commit a version in the local folder'
	good_local=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	restart_service
	run=$(next_run demo-local "$prev") || fail 'no run of demo-local started for the version change'
	wait_for "the run $run" 240 run_finished demo-local "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-local deployed)" = "$good_local" ] || fail "demo-local is not recorded as deployed at $good_local"
	[ "$(stack_images demo-local)" = "$image:1.1.0" ] || fail "demo-local does not run 1.1.0: $(stack_images demo-local)"

	step 'a failing version is rolled back by a revert commit of the service, and the folder has no remote'
	prev=$run
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' demo-local/.env &&
		echo 'edited for 2.0.0' >>demo-local/notes/info.txt &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-local to 2.0.0 with a broken healthcheck'" ||
		fail 'the admin could not commit the failing version'
	bad_local=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	restart_service
	run=$(next_run demo-local "$prev") || fail 'no run of demo-local started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-local "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(docker exec "$host" git -C "$base" rev-parse HEAD)
	[ "$revert" != "$bad_local" ] || fail 'no revert commit was made in the local folder'
	meta=$(docker exec "$host" git -C "$base" log -1 --format='%an|%s|%P' "$revert")
	[ "$meta" = "qhost|Revert demo-local to $(printf '%s' "$good_local" | cut -c 1-12)|$bad_local" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ -z "$(docker exec "$host" git -C "$base" remote)" ] || fail 'the rollback gave the local repository a remote'
	docker exec "$host" grep -qx DEMO_APP_VERSION=1.1.0 "$base/demo-local/.env" || fail 'the revert did not restore the version file'
	[ "$(docker exec "$host" cat "$base/demo-local/notes/info.txt")" = 'kept by a person' ] || fail 'the revert did not restore the folder of the person'
	[ "$(stack_images demo-local)" = "$image:1.1.0" ] || fail "demo-local does not run the previous image 1.1.0: $(stack_images demo-local)"

	step 'uncommitted changes of a stack folder are listed, shown and discarded, and the stack is deployed again'
	prev=$(newest_run demo-local)
	docker exec "$host" sh -ec "cd '$base/demo-local' && printf '# edited by hand\n' >>.env && printf 'scratch\n' >scratch.txt" ||
		fail 'cannot edit the stack folder by hand'
	# A hand edit wakes no poll; the restart starts the one that judges the findings.
	restart_service
	i=0
	until stack_field demo-local findings | grep -q local-changes; do
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail "demo-local has no finding local-changes: $(qd --output json status | tr ',' '\n' | grep demo-local)"
		sleep 1
	done
	out=$(qd --output json source changes "$base") || fail "source changes failed: $out"
	printf '%s' "$out" | jq -e '.path == "/srv/qd-local" and (.changes | sort_by(.path) == [{"path": "demo-local/.env", "kind": "modified", "stack": "demo-local"}, {"path": "demo-local/scratch.txt", "kind": "new", "stack": "demo-local"}])' >/dev/null ||
		fail "unexpected source changes: $out"
	if printf '%s' "$out" | grep -q secrets.env; then fail "source changes lists the host's own secrets file: $out"; fi
	out=$(qd_fails stack deploy demo-local) || fail "a stack with local changes was deployed: $out"
	printf '%s' "$out" | grep -qi 'local.changes' || fail "the refused deploy does not name the local changes: $out"
	out=$(qd --output json source diff "$base" demo-local/.env) || fail "source diff failed: $out"
	printf '%s' "$out" | jq -e '.diff | contains("+# edited by hand")' >/dev/null || fail "unexpected source diff: $out"
	out=$(qd_fails source discard "$base" demo-local/.env demo-local/secrets.env --yes) || fail "source discard accepted the host's own secrets file: $out"
	[ "$(docker exec "$host" tail -n 1 "$base/demo-local/.env")" = '# edited by hand' ] || fail 'a refused source discard changed .env'
	out=$(qd_fails source discard "$base" demo-local/.env ../demo-live/compose.yaml --yes) || fail "source discard accepted a path outside the stack folder: $out"
	out=$(qd --output json source discard "$base" demo-local/.env demo-local/scratch.txt --yes) || fail "source discard failed: $out"
	printf '%s' "$out" | jq -e '.discarded == ["demo-local/.env", "demo-local/scratch.txt"]' >/dev/null || fail "unexpected source discard result: $out"
	docker exec "$host" grep -qx DEMO_APP_VERSION=1.1.0 "$base/demo-local/.env" || fail 'source discard did not restore .env'
	if docker exec "$host" test -e "$base/demo-local/scratch.txt"; then fail 'source discard did not remove the named new file'; fi
	docker exec "$host" test -f "$base/demo-local/secrets.env" || fail 'source discard removed the secrets file of the host'
	[ "$(qd --output json source changes "$base" | jq '.changes | length')" = 0 ] || fail 'the stack folder still has changes after the discard'
	i=0
	while stack_field demo-local findings | grep -q local-changes; do
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail 'the finding local-changes stays after the discard'
		sleep 1
	done
	out=$(qd --output json stack deploy demo-local) || fail "demo-local cannot be deployed after the discard: $out"
	run=$(next_run demo-local "$prev") || fail 'no run of demo-local after the discard'
	wait_for "the run $run" 240 run_finished demo-local "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run after the discard did not succeed: $(qd stack log "$run")"

	step 'source remove asks first, then releases only the binding'
	containers_before=$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-local | sort)
	[ -n "$containers_before" ] || fail 'demo-local has no running container'
	tree_before=$(local_tree)
	out=$(qd_fails source remove "$base") || fail "source remove without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source remove without --yes does not ask for confirmation: $out"
	list_has_path "$base" || fail 'a refused source remove released the folder'
	out=$(qd --output json source remove "$base" --yes) || fail "source remove failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.id == "srv-qd-local" and .path == "/srv/qd-local"' >/dev/null || fail "unexpected source remove result: $out"
	[ "$(local_tree)" = "$tree_before" ] || fail 'source remove changed a file of the folder'
	[ "$(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=demo-local | sort)" = "$containers_before" ] ||
		fail 'source remove stopped or replaced a container of demo-local'
	if list_has_path "$base"; then fail "source list still shows the released folder: $(qd --output json source list)"; fi
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'source remove took the folder out of the drop-in'
	out=$(qd_fails source remove "$base" --yes) || fail "source remove of an unbound folder succeeded: $out"
	printf '%s' "$out" | grep -q 'not-found' || fail "source remove of an unbound folder is no not-found error: $out"
	log 'scenario: a local repository was bound with group access, deployed, rolled back by a local revert and released without a change to its files'

	step 'an existing clone is released and taken over again with its origin and its deploy key'
	key2_before=$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")
	[ -n "$key2_before" ] || fail 'the clone has no deploy key before it is released'
	out=$(qd --output json source remove /opt/stacks2 --yes) || fail "source remove of /opt/stacks2 failed: $out"
	if list_has_path /opt/stacks2; then fail 'source list still shows /opt/stacks2'; fi
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$key2_before" ] || fail 'releasing /opt/stacks2 changed its deploy key'
	[ "$(stack_images demo-second)" = "$image:1.0.0" ] || fail 'releasing /opt/stacks2 stopped demo-second'
	head2=$(docker exec "$host" git -c safe.directory=/opt/stacks2 -C /opt/stacks2 rev-parse HEAD)
	out=$(qd --output json source add /opt/stacks2 --yes --fingerprint "$(git_ed25519_fingerprint)") || fail "source add of the clone failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e --arg u "$url2" '.url == $u and .cloned == false and .dir == "/opt/stacks2"' >/dev/null ||
		fail "source add did not take over the clone with its origin: $out"
	wait_for 'the deploy service to answer' 30 qd status
	# The engine state of the source went with the release, so the stack is deployed once more.
	wait_for 'the run of demo-second after the takeover' 180 stack_settled demo-second succeeded
	[ "$(docker exec "$host" git -c safe.directory=/opt/stacks2 -C /opt/stacks2 rev-parse HEAD)" = "$head2" ] || fail 'taking over the clone changed its head'
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$key2_before" ] || fail 'taking over /opt/stacks2 changed its deploy key'
	qd --output json source list | jq -e '[.sources[] | select(.path == "/opt/stacks2" and .kind == "remote")] | length == 1' >/dev/null ||
		fail "source list does not show /opt/stacks2 again: $(qd --output json source list)"
	[ "$(stack_images demo-second)" = "$image:1.0.0" ] || fail 'demo-second does not run after /opt/stacks2 is taken over again'
	[ "$(stack_field demo-second source)" = opt-stacks2 ] || fail "demo-second is not listed with its source: $(qd --output json status)"
	log 'scenario: /opt/stacks2 was released and taken over again with its origin, head and deploy key'

	step 'source commit pushes exactly the named changes of a clone, the stack runs, and a failing commit is rolled back'
	stack2=/opt/stacks2
	prev=$(newest_run demo-second)
	head2=$(qd --output json source list | jq -r '.sources[] | select(.path == "/opt/stacks2") | .head')
	[ "$head2" = "$(remote_head "$url2")" ] || fail "the head of /opt/stacks2 is not the head of the remote: $head2"
	docker exec "$host" sh -ec "cd '$stack2/demo-second' && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' .env &&
		sed -i 's/^      DEMO_NAME: demo-second\$/      DEMO_NAME: demo-second\n      DEMO_NOTE: committed by qhost/' compose.yaml" ||
		fail 'cannot edit /opt/stacks2 by hand'
	restart_service
	i=0
	until stack_field demo-second findings | grep -q local-changes; do
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail 'demo-second has no finding local-changes'
		sleep 1
	done
	out=$(qd_fails source commit "$stack2" demo-second/.env demo-second/compose.yaml) || fail "source commit without --head succeeded: $out"
	out=$(qd_fails source commit "$stack2" demo-second/.env demo-second/compose.yaml --head "$(printf '%040d' 0)") || fail "source commit with a wrong head succeeded: $out"
	printf '%s' "$out" | grep -q 'head-moved' || fail "a wrong head is no head-moved error: $out"
	out=$(qd_fails source commit "$stack2" demo-second/.env demo-second/secrets.env --head "$head2") || fail "source commit took the host's own secrets file: $out"
	[ "$(remote_head "$url2")" = "$head2" ] || fail 'a refused source commit changed the remote'
	out=$(qd --output json source commit "$stack2" demo-second/.env demo-second/compose.yaml --head "$head2") || fail "source commit failed: $out"
	commit2=$(printf '%s' "$out" | jq -r '.commit')
	printf '%s' "$out" | jq -e '.committed == ["demo-second/.env", "demo-second/compose.yaml"]' >/dev/null || fail "unexpected source commit result: $out"
	[ "$(remote_head "$url2")" = "$commit2" ] || fail "the commit $commit2 is not the head of the remote: $(remote_head "$url2")"
	[ "$(folder_git "$stack2" rev-parse HEAD)" = "$commit2" ] || fail 'the branch of /opt/stacks2 is not at the new commit'
	meta=$(folder_git "$stack2" log -1 --format='%an|%P' "$commit2")
	[ "$meta" = "qhost|$head2" ] || fail "the commit has the wrong author or parent: $meta"
	folder_git "$stack2" show --name-only --format=%B "$commit2" | grep -q 'demo-second/compose.yaml' || fail 'the commit message does not name the files'
	[ "$(folder_git "$stack2" show --name-only --format= "$commit2" | LC_ALL=C sort | tr '\n' ' ')" = 'demo-second/.env demo-second/compose.yaml ' ] ||
		fail "the commit holds more than the named files: $(folder_git "$stack2" show --name-only --format= "$commit2")"
	run=$(next_run demo-second "$prev") || fail 'no run of demo-second after the commit'
	wait_for "the run $run" 240 run_finished demo-second "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run after the commit did not succeed: $(qd stack log "$run")"
	[ "$(stack_images demo-second)" = "$image:1.1.0" ] || fail "demo-second does not run 1.1.0: $(stack_images demo-second)"
	[ "$(stack_field demo-second deployed)" = "$commit2" ] || fail "demo-second is not recorded as deployed at $commit2"
	[ "$(qd --output json source changes "$stack2" | jq '.changes | length')" = 0 ] || fail "changes are left after the commit: $(qd --output json source changes "$stack2")"
	i=0
	while stack_field demo-second findings | grep -q local-changes; do
		i=$((i + 1))
		[ "$i" -lt 120 ] || fail 'the finding local-changes stays after the commit'
		sleep 1
	done

	prev=$run
	head2=$commit2
	docker exec "$host" sed -i 's/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' "$stack2/demo-second/.env" || fail 'cannot break the healthcheck of demo-second'
	out=$(qd --output json source commit "$stack2" demo-second/.env --head "$head2") || fail "source commit of the failing version failed: $out"
	bad2=$(printf '%s' "$out" | jq -r '.commit')
	run=$(next_run demo-second "$prev") || fail 'no run of demo-second for the failing commit'
	wait_for "the run $run" 240 run_finished demo-second "$run"
	printf '%s' "$(qd stack log "$run")" | grep -q ' result rolled-back' || fail "the failing commit was not rolled back: $(qd stack log "$run")"
	revert2=$(folder_git "$stack2" rev-parse HEAD)
	[ "$revert2" != "$bad2" ] || fail 'no revert commit followed the failing commit'
	[ "$(remote_head "$url2")" = "$revert2" ] || fail 'the revert commit is not the head of the remote'
	[ "$(folder_git "$stack2" log -1 --format='%an|%P' "$revert2")" = "qhost|$bad2" ] || fail 'the revert commit has the wrong author or parent'
	docker exec "$host" grep -qx DEMO_HEALTH=ok "$stack2/demo-second/.env" || fail 'the revert did not restore the healthcheck setting'
	[ "$(stack_images demo-second)" = "$image:1.1.0" ] || fail "demo-second does not run the previous image after the rollback: $(stack_images demo-second)"
	log "scenario: source commit pushed $commit2 and the failing commit $bad2 was rolled back by $revert2"
}

# scenario_private_registry proves that the deploy service pulls from a registry with a login only with the login
# stored by qhost credential set: a stack from private.$domain fails at the pull step without it and is deployed with
# it, the update check finds the newer tag there, and the login stays in the credential store and nowhere else. It uses
# its own local repository below /srv/qd-private and removes the stack, the source, the credential and the folder
# at its end.
scenario_private_registry() {
	base=/srv/qd-private
	image=private.$domain/demo/app
	cred=registry.private.$domain
	private_login=$(cat "$work/keys/private-login")
	private_token=${private_login#*:}

	step 'the Docker configuration of the service holds only the credential helper'
	[ "$(docker exec "$host" cat /var/lib/qhost-deploy/docker/config.json)" = '{"credsStore":"qhost"}' ] ||
		fail "the Docker configuration of the service is not the credential helper configuration: $(docker exec "$host" cat /var/lib/qhost-deploy/docker/config.json 2>&1)"
	[ "$(docker exec "$host" stat -c %U:%a /var/lib/qhost-deploy/docker/config.json)" = qhost-deploy:600 ] ||
		fail "config.json of the service is $(docker exec "$host" stat -c %U:%a /var/lib/qhost-deploy/docker/config.json), want qhost-deploy:600"
	docker exec "$host" test -x /var/lib/qhost-deploy/docker/bin/docker-credential-qhost || fail 'the service has no credential helper link'

	step 'a local repository with a stack from the registry with a login is prepared'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/demo-private' && chmod 0755 '$base'" || fail 'cannot create the private folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/demo-private/compose.yaml'"
name: demo-private

services:
  app:
    image: $image:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-private
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/demo-private/.env'"
	docker exec "$host" sh -ec "cd '$base' && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-private stack'" ||
		fail 'cannot create the private repository'
	if qd credential list | grep -qF "$cred"; then fail "a login for $cred is stored before the test: $(qd credential list)"; fi
	out=$(qd --output json source add "$base" --yes) || fail "source add failed: $out"

	step 'without a stored login the first run fails at the pull step'
	wait_for 'the first run of demo-private' 180 stack_settled demo-private failed
	run=$(newest_run demo-private)
	[ -n "$run" ] || fail 'demo-private has no recorded run'
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result failed pull:' || fail "the run did not fail at the pull step: $run_log"
	case ",$(log_steps "$run_log")," in *,pull,*) ;; *) fail "the run has no pull step: $run_log" ;; esac
	case ",$(log_steps "$run_log")," in *,up,*) fail "the run went on after the failed pull: $run_log" ;; esac
	runs_json=$(qd --output json stack runs demo-private) || fail "stack runs failed: $runs_json"
	printf '%s' "$runs_json" | grep -q 'pull: [^"]*denied access; store a login as credential '"$cred"' (see qhost help credentials)\|pull: [^"]*denied access; store a login as credential '"$cred"' for ' ||
		fail "the run record does not name the login $cred: $runs_json"
	[ -z "$(stack_images demo-private)" ] || fail "demo-private runs without a login: $(stack_images demo-private)"

	step 'with the stored login a manual run deploys the stack'
	out=$(qd_in "$private_login" credential set "$cred") || fail "credential set failed: $out"
	qd credential list | grep -qF "$cred" || fail "credential list does not show $cred: $(qd credential list)"
	out=$(qd --output json stack deploy demo-private) || fail "deploy failed: $out"
	run=$(printf '%s\n' "$out" | sed -n 's/.*"run": *"\([^"]*\)".*/\1/p')
	[ -n "$run" ] || fail "deploy did not report a run: $out"
	wait_for "the run $run" 240 run_finished demo-private "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result succeeded' || fail "the run with the stored login did not succeed: $run_log"
	[ "$(log_steps "$run_log")" = backup,pull,up,check ] || fail "unexpected steps of the run: $(log_steps "$run_log")"
	[ "$(stack_images demo-private)" = "$image:1.0.0" ] || fail "demo-private does not run 1.0.0: $(stack_images demo-private)"
	stack_healthy demo-private || fail 'demo-private is not healthy'

	step 'the update check finds the newer tag in the registry with a login'
	out=$(qd --output json stack updates demo-private --refresh) || fail "updates failed: $out"
	updates_expect "$out" 1.0.0 1.1.0 '' update

	step 'the login stays in the credential store: no copy in the Docker configuration, the logs or the journal'
	[ "$(docker exec "$host" cat /var/lib/qhost-deploy/docker/config.json)" = '{"credsStore":"qhost"}' ] ||
		fail 'the Docker configuration of the service changed after the pull'
	if docker exec "$host" grep -rqF -- "$private_token" /var/lib/qhost-deploy/docker; then fail 'the token is in the Docker configuration of the service'; fi
	if journal | grep -qF -- "$private_token"; then fail 'the token is in the journal'; fi
	if qd stack log "$run" | grep -qF -- "$private_token"; then fail 'the token is in the run log'; fi
	log 'scenario: a stack from the registry with a login failed at the pull without the stored login, deployed with it, and its update check found 1.1.0'

	step 'the stack, the source, the login and the folder are removed again'
	docker exec "$host" docker compose -p demo-private down >/dev/null 2>&1 || fail 'cannot stop demo-private'
	docker exec "$host" docker rmi "$image:1.0.0" >/dev/null 2>&1 || true
	out=$(qd --output json source remove "$base" --yes) || fail "source remove failed: $out"
	out=$(qd credential delete "$cred" --yes) || fail "credential delete failed: $out"
	docker exec "$host" rm -rf "$base" || fail "cannot remove $base"
}

# files_state prints the content hash of every file below /srv/qd-files/web except .git and .gitignore, and the type,
# owner and size of every other path; modes and groups are left out because source add changes them for the listed
# files, and .gitignore is the one file it extends.
files_state() {
	docker exec "$host" sh -ec "cd /srv/qd-files/web && find . -path ./.git -prune -o -path ./.gitignore -prune -o -type f -exec sha256sum {} + | sort -k2 &&
		find . -path ./.git -prune -o -path ./.gitignore -prune -o -printf '%p %y %u %s\n' | sort"
}

# scenario_source_files proves source add on a folder of files without a repository below /srv/qd-files, which no
# stack folder uses: the answer lists the files, what is left out and the lines for .gitignore, nothing changes without
# the confirmation, the confirmed add starts a repository with one commit of only the files the stack refers to and the
# .gitignore, which keeps its own lines and makes a clone ignore what is left out, and leaves every other file as it
# was, a version committed by a person is deployed and a failing one is rolled back by a local revert commit. A second
# folder holds a stack whose .env the service cannot read, which brings no file, and gets a new .gitignore.
scenario_source_files() {
	base=/srv/qd-files/web
	image=registry.$domain/demo/app

	step 'a folder of files without a repository is prepared as root, with a .gitignore, backup copies, a log, data and a writable file the first commit leaves out'
	docker exec "$host" sh -ec "rm -rf /srv/qd-files && mkdir -p '$base' && chmod 0755 /srv/qd-files '$base'" || fail 'cannot create the folder of files'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/compose.yaml'"
name: demo-files

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-files
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
    volumes:
      - ./data:/srv/demo-data
      - ./acme.json:/srv/demo-acme.json
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	files_ignore_own=$(printf '# kept by the operator\n*.swp')
	printf '%s\n' "$files_ignore_own" | docker exec -i "$host" sh -ec "cat >'$base/.gitignore'"
	docker exec "$host" sh -ec "head -c 2097152 /dev/zero >'$base/big.bin' && mkdir -p '$base/data' && echo x >'$base/data/db.bin' &&
		echo '{}' >'$base/acme.json' && echo old >'$base/app.log' && echo notes >'$base/notes.txt' &&
		cp '$base/compose.yaml' '$base/compose.yaml.bak' && cp '$base/.env' '$base/.env.bak.1' &&
		chmod 0644 '$base/big.bin' '$base/compose.yaml' '$base/.env' '$base/.gitignore' '$base/compose.yaml.bak' '$base/.env.bak.1' '$base/app.log' '$base/notes.txt' '$base/acme.json' '$base/data/db.bin'" ||
		fail 'cannot create the files of the folder'
	[ -z "$(docker exec "$host" ls -A "$base" | grep -x .git)" ] || fail 'the folder of files has a .git'

	step 'source inspect names the referenced files to take, everything else with its reason and the lines for .gitignore'
	out=$(qd --output json source inspect "$base") || fail "source inspect of the folder of files failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files == [".env", ".gitignore", "compose.yaml"]) and
		.ignore.action == "extend" and
		.ignore.lines == ["/.env.bak.1", "/acme.json", "/app.log", "/big.bin", "/compose.yaml.bak", "/data", "/notes.txt"] and
		([.excluded[] | select((.path == "big.bin" or .path == "compose.yaml.bak" or .path == ".env.bak.1" or .path == "app.log" or
			.path == "notes.txt") and .reason == "not-referenced")] | length) == 5 and
		([.excluded[] | select(.path == "acme.json" and .reason == "writable-file")] | length) == 1 and
		([.excluded[] | select(.path == "data" and .reason == "bind-mount")] | length) == 1 and
		([.stacks[].name] | index("demo-files")) != null' >/dev/null || fail "unexpected inspection of the folder of files: $out"

	step 'source add without a terminal and without --yes is refused and changes nothing'
	dir_before=$(docker exec "$host" stat -c '%U:%G %a' "$base")
	out=$(qd_fails source add "$base") || fail "source add of the files without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add of the files without --yes does not ask for confirmation: $out"
	if docker exec "$host" test -e "$base/.git"; then fail 'a refused source add made a repository'; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base")" = "$dir_before" ] || fail 'a refused source add changed the folder'
	if docker exec "$host" grep -qF "$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf; then
		fail 'a refused source add wrote the drop-in'
	fi
	if list_has_path "$base"; then fail 'a refused source add bound the folder'; fi

	step 'source add --yes starts a repository with the listed files and the extended .gitignore and changes no other file content'
	state_before=$(files_state)
	ignore_owner_before=$(docker exec "$host" stat -c '%U' "$base/.gitignore")
	big_before=$(docker exec "$host" stat -c '%a %U:%G' "$base/big.bin")
	# qd merges stderr, where the preview and the notices go; the JSON answer is the last line.
	out=$(qd --output json source add "$base" --yes) || fail "source add of the files failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .path == "/srv/qd-files/web" and .files == 3' >/dev/null ||
		fail "unexpected source add result for the files: $out"
	printf '%s' "$out" | grep -qF 'Add 7 lines to the block marked by qhost at the end of .gitignore' || fail "the preview does not show the lines for .gitignore: $out"
	wait_for 'the deploy service to answer' 30 qd status
	docker exec "$host" test -d "$base/.git" || fail 'source add made no repository'
	ignore_now=$(docker exec "$host" cat "$base/.gitignore")
	[ "$(printf '%s\n' "$ignore_now" | head -n 2)" = "$files_ignore_own" ] || fail ".gitignore lost its own lines: $ignore_now"
	[ "$(printf '%s\n' "$ignore_now" | grep -c '^# BEGIN qhost')" = 1 ] || fail ".gitignore has not exactly one block of qhost: $ignore_now"
	[ "$(docker exec "$host" stat -c '%U' "$base/.gitignore")" = "$ignore_owner_before" ] || fail 'source add changed the owner of .gitignore'
	[ "$(files_git rev-list --count HEAD)" = 1 ] || fail "the repository does not have exactly one commit: $(files_git log --oneline)"
	[ "$(files_git log -1 --format=%an)" = qhost ] || fail "the first commit has the wrong author: $(files_git log -1 --format=%an)"
	[ "$(files_git symbolic-ref --short HEAD)" = main ] || fail "the branch is not main: $(files_git symbolic-ref --short HEAD)"
	[ "$(files_git ls-files)" = "$(printf '.env\n.gitignore\ncompose.yaml')" ] || fail "the first commit holds the wrong files: $(files_git ls-files)"
	[ "$(files_git show HEAD:.gitignore)" = "$ignore_now" ] || fail "the first commit does not hold the .gitignore of the folder: $(files_git show HEAD:.gitignore)"
	[ -z "$(files_git status --porcelain)" ] || fail "the new repository sees a local change: $(files_git status --porcelain)"
	[ -z "$(docker exec "$host" sh -c "cat '$base/.git/info/exclude' 2>/dev/null")" ] || fail 'source add wrote the left-out paths to .git/info/exclude'
	# A fresh clone, as when a host is restored from the repository, ignores what the first commit left out.
	clone=$(docker exec "$host" mktemp -d) || fail 'cannot create a temporary folder for the clone'
	# The service owns .git, and git takes safe.directory for the upload-pack of a local clone only from a config file.
	printf '[safe]\n\tdirectory = %s\n\tdirectory = %s/.git\n' "$base" "$base" | docker exec -i "$host" sh -ec "cat >'$clone/gitconfig'"
	docker exec -e GIT_CONFIG_GLOBAL="$clone/gitconfig" "$host" git clone -q "$base" "$clone/web" || fail 'cannot clone the folder of files'
	for left in big.bin compose.yaml.bak .env.bak.1 app.log notes.txt acme.json data; do
		docker exec "$host" git -C "$clone/web" check-ignore -q -- "$left" || fail "a clone does not ignore $left: $(docker exec "$host" cat "$clone/web/.gitignore")"
	done
	if docker exec "$host" git -C "$clone/web" check-ignore -q -- compose.yaml; then fail 'a clone ignores compose.yaml'; fi
	docker exec "$host" rm -rf "$clone"
	[ "$(files_git config core.sharedRepository)" = group ] || fail 'core.sharedRepository is not group'
	[ -z "$(files_git remote)" ] || fail 'the new repository has a remote'
	[ "$(files_state)" = "$state_before" ] || fail 'source add changed the content, the owner or the size of a file'
	[ "$(docker exec "$host" stat -c '%a %U:%G' "$base/big.bin")" = "$big_before" ] || fail 'source add changed the large file that is not listed'
	[ "$(docker exec "$host" stat -c '%U:%G' "$base")" = root:qhost-deploy ] || fail "the folder has the wrong owner or group: $(docker exec "$host" stat -c '%U:%G' "$base")"
	case $(docker exec "$host" stat -c '%A' "$base") in d???rws*) ;; *) fail "the folder is not group-writable with setgid: $(docker exec "$host" stat -c '%A' "$base")" ;; esac
	for f in compose.yaml .env; do
		[ "$(docker exec "$host" stat -c '%G' "$base/$f")" = qhost-deploy ] || fail "$f does not have the group qhost-deploy"
		case $(docker exec "$host" stat -c '%A' "$base/$f") in -???rw*) ;; *) fail "$f is not group-writable: $(docker exec "$host" stat -c '%A' "$base/$f")" ;; esac
	done
	docker exec "$host" grep -qx "ReadWritePaths=-$base" /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /srv/qd-files/web'
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'the folder of files is not a local source'

	step 'demo-files is deployed at 1.0.0'
	i=0
	until stack_settled demo-files succeeded; do
		i=$((i + 1))
		[ "$(stack_field demo-files last_run)" != failed ] || fail "the first run of demo-files failed: $(qd stack log "$(newest_run demo-files)")"
		[ "$i" -lt 180 ] || fail "timeout waiting for the first run of demo-files: $(qd --output json status | tr ',' '\n' | grep -e qd-files -e demo-files)"
		sleep 1
	done
	[ "$(stack_images demo-files)" = "$image:1.0.0" ] || fail "demo-files does not run 1.0.0: $(stack_images demo-files)"

	step 'a person in the group qhost-deploy commits a version, which is deployed'
	as_admin "git config --global --add safe.directory $base" || fail 'cannot mark the folder of files safe'
	prev=$(newest_run demo-files)
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' .env &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-files to 1.1.0'" ||
		fail 'the admin could not commit a version in the folder of files'
	good_files=$(files_git rev-parse HEAD)
	restart_service
	run=$(next_run demo-files "$prev") || fail 'no run of demo-files started for the version change'
	wait_for "the run $run" 240 run_finished demo-files "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-files deployed)" = "$good_files" ] || fail "demo-files is not recorded as deployed at $good_files"
	[ "$(stack_images demo-files)" = "$image:1.1.0" ] || fail "demo-files does not run 1.1.0: $(stack_images demo-files)"

	step 'a failing version is rolled back by a revert commit of the service, and the folder has no remote'
	prev=$run
	as_admin "umask 002; cd $base && sed -i 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' .env &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am 'Update demo-files to 2.0.0 with a broken healthcheck'" ||
		fail 'the admin could not commit the failing version'
	bad_files=$(files_git rev-parse HEAD)
	restart_service
	run=$(next_run demo-files "$prev") || fail 'no run of demo-files started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-files "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(files_git rev-parse HEAD)
	[ "$revert" != "$bad_files" ] || fail 'no revert commit was made in the folder of files'
	meta=$(files_git log -1 --format='%an|%s|%P' "$revert")
	[ "$meta" = "qhost|Revert the stack folder to $(printf '%s' "$good_files" | cut -c 1-12)|$bad_files" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ -z "$(files_git remote)" ] || fail 'the rollback gave the repository a remote'
	[ "$(stack_images demo-files)" = "$image:1.1.0" ] || fail "demo-files does not run the previous image 1.1.0: $(stack_images demo-files)"
	log 'scenario: a folder of files got a repository with group access, deployed 1.1.0 and rolled back 2.0.0 by a local revert'
	scenario_source_files_include
	scenario_source_files_unreadable
}

# scenario_source_files_include proves the choice of an unclear file: the folder below /srv/qd-include mounts acme.json
# writable, so the first commit leaves it out as writable-file (as the folder before shows). A path that is not
# unclear is refused and changes nothing; --include acme.json with --yes takes the file into the first commit.
scenario_source_files_include() {
	base=/srv/qd-include

	step 'a folder of files with a writable single-file mount is prepared as root'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base' && chmod 0755 '$base'" || fail 'cannot create the folder with a writable file'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/compose.yaml'"
name: demo-include

services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-include
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
    volumes:
      - ./acme.json:/srv/demo-acme.json
EOF2
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	docker exec "$host" sh -ec "echo '{}' >'$base/acme.json' && echo notes >'$base/notes.txt' &&
		chmod 0644 '$base/compose.yaml' '$base/.env' '$base/acme.json' '$base/notes.txt'" || fail 'cannot create the files of the folder with a writable file'

	step 'source inspect leaves the writable file out, and with --include plans it and names it as included'
	out=$(qd --output json source inspect "$base") || fail "source inspect of the folder with a writable file failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files == [".env", "compose.yaml"]) and
		([.excluded[] | select(.path == "acme.json" and .reason == "writable-file")] | length) == 1 and (.included // []) == []' >/dev/null ||
		fail "unexpected inspection without a choice: $out"
	out=$(qd --output json source inspect "$base" --include acme.json) || fail "source inspect --include failed: $out"
	printf '%s' "$out" | jq -e '(.planned_files == [".env", "acme.json", "compose.yaml"]) and
		([.included[] | select(.path == "acme.json" and .reason == "writable-file")] | length) == 1 and
		([.excluded[] | select(.path == "acme.json")] | length) == 0' >/dev/null || fail "unexpected inspection with a choice: $out"

	step 'a path that is not an unclear file is refused and nothing is created'
	for bad in notes.txt compose.yaml missing.txt; do
		out=$(qd_fails source add "$base" --include "$bad" --yes) || fail "source add --include $bad succeeded: $out"
		if docker exec "$host" test -e "$base/.git"; then fail "a refused source add --include $bad made a repository"; fi
		if list_has_path "$base"; then fail "a refused source add --include $bad bound the folder"; fi
	done

	step 'source add --include acme.json --yes takes the file into the first commit'
	out=$(qd --output json source add "$base" --include acme.json --yes) || fail "source add --include failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .files == 4' >/dev/null || fail "unexpected source add --include result: $out"
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(folder_git "$base" ls-files)" = "$(printf '.env\n.gitignore\nacme.json\ncompose.yaml')" ] ||
		fail "the first commit holds the wrong files: $(folder_git "$base" ls-files)"
	if docker exec "$host" grep -qx '/acme.json' "$base/.gitignore"; then fail 'the chosen file is in .gitignore'; fi
	docker exec "$host" grep -qx '/notes.txt' "$base/.gitignore" || fail 'notes.txt is not in .gitignore'
	wait_for 'the stack with the chosen file to be deployed' 180 stack_settled demo-include succeeded

	step 'the folder with the chosen file is released and its containers are removed'
	out=$(qd --output json source remove "$base" --yes) || fail "source remove of the folder with a writable file failed: $out"
	docker exec "$host" docker compose -p demo-include down >/dev/null 2>&1 || fail 'cannot remove the containers of demo-include'
	log 'scenario: an unclear file chosen with --include went into the first commit'
}

# source_polled_clean succeeds when the source of the folder $1 has been polled and reports no poll error.
source_polled_clean() {
	qd --output json source list | jq -e --arg p "$1" '[.sources[] | select(.path == $p and .last_poll != null and (.poll_error // "") == "")] | length == 1' >/dev/null
}

# scenario_source_files_unreadable proves that a stack whose .env the service cannot read brings no file: the folder
# below /srv/qd-multi holds a readable stack and one whose .env is 0600 root. The answer names the remedy and no value
# of the readable .env, the first commit holds only the files of the readable stack, the folder of the other one stays
# out, and the source polls without an error. The folder gets no remote, so its .env with a secret-like value is
# never pushed.
scenario_source_files_unreadable() {
	base=/srv/qd-multi

	step 'a folder with a readable stack and a stack whose .env the service cannot read is prepared as root'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/qd-multi-ok' '$base/qd-multi-locked' && chmod 0755 '$base' '$base/qd-multi-ok' '$base/qd-multi-locked'" ||
		fail 'cannot create the folder with two stacks'
	for stack in qd-multi-ok qd-multi-locked; do
		cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$base/$stack/compose.yaml'"
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: $stack
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
EOF2
		printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/$stack/.env'"
	done
	# The value is made up; it must never show in an answer.
	um_secret=e2e-not-a-real-secret-4711
	printf 'DEMO_API_TOKEN=%s\n' "$um_secret" | docker exec -i "$host" sh -ec "cat >>'$base/qd-multi-ok/.env'"
	docker exec "$host" sh -ec "echo notes >'$base/qd-multi-ok/notes.txt' && echo notes >'$base/qd-multi-locked/notes.txt' &&
		chmod 0644 '$base/qd-multi-ok/compose.yaml' '$base/qd-multi-ok/.env' '$base/qd-multi-ok/notes.txt' '$base/qd-multi-locked/compose.yaml' '$base/qd-multi-locked/notes.txt' &&
		chown root:root '$base/qd-multi-locked/.env' && chmod 0600 '$base/qd-multi-locked/.env'" || fail 'cannot set the modes of the stack files'

	step 'source inspect takes the readable stack only and names how to make the other one readable'
	out=$(qd --output json source inspect "$base") || fail "source inspect of the folder with two stacks failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files == ["qd-multi-ok/.env", "qd-multi-ok/compose.yaml"]) and
		([.excluded[] | select(.path == "qd-multi-locked" and .reason == "unreadable-stack")] | length) == 1 and
		([.excluded[] | select(.path == "qd-multi-ok/notes.txt" and .reason == "not-referenced")] | length) == 1 and
		([.stacks[] | select(.name == "qd-multi-locked") | .findings[] | select(.rule == "unreadable-stack" and (.fix | contains("read permission")))] | length) == 1' >/dev/null ||
		fail "unexpected inspection of the folder with two stacks: $out"
	if printf '%s' "$out" | grep -qF "$um_secret"; then fail "source inspect shows a value of .env: $out"; fi
	out=$(qd source inspect "$base") || fail "the text source inspect of the folder with two stacks failed: $out"
	if printf '%s' "$out" | grep -qF "$um_secret"; then fail "the text source inspect shows a value of .env: $out"; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base/qd-multi-locked/.env")" = 'root:root 600' ] || fail 'source inspect changed the unreadable .env'

	step 'source add --yes starts a repository without a file of the unreadable stack, and the source polls without an error'
	out=$(qd --output json source add "$base" --yes) || fail "source add of the folder with two stacks failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local" and .files == 3' >/dev/null || fail "unexpected source add result for two stacks: $out"
	wait_for 'the deploy service to answer' 30 qd status
	# The folder had no .gitignore; the one created holds the paths left out and is part of the first commit.
	[ "$(folder_git "$base" ls-files)" = "$(printf '.gitignore\nqd-multi-ok/.env\nqd-multi-ok/compose.yaml')" ] ||
		fail "the first commit holds the wrong files: $(folder_git "$base" ls-files)"
	for left in qd-multi-locked qd-multi-ok/notes.txt; do
		docker exec "$host" grep -qx "/$left" "$base/.gitignore" || fail "$left is not in the new .gitignore: $(docker exec "$host" cat "$base/.gitignore")"
	done
	[ -z "$(folder_git "$base" status --porcelain)" ] || fail "the folder with two stacks sees a local change: $(folder_git "$base" status --porcelain)"
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$base/qd-multi-locked/.env")" = 'root:root 600' ] || fail 'source add changed the unreadable .env'
	wait_for 'the first poll of the folder with two stacks' 120 source_polled_clean "$base"
	wait_for 'the readable stack to be deployed' 180 stack_settled qd-multi-ok succeeded

	step 'the folder with two stacks is released and its containers are removed'
	out=$(qd --output json source remove "$base" --yes) || fail "source remove of the folder with two stacks failed: $out"
	docker exec "$host" docker compose -p qd-multi-ok down >/dev/null 2>&1 || fail 'cannot remove the containers of qd-multi-ok'
	log 'scenario: a stack whose .env the service cannot read brought no file into the first commit'
}
