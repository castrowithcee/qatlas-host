#!/bin/sh
# Section sources: an installation of the earlier single binding is taken over as the first source and a second
# repository is bound: its own stack is deployed, a stack named like one of the first source is reported and never
# deployed. A repository with the stack at its root, source inspect and the deploy keys per folder follow.
# Needs deploy: url and the binding of /opt/stacks, which scenario_sources turns into the earlier layout.
# Sets url2, the address of the second repository, and leaves the clones /opt/stacks2 and /opt/stacks3.
# Sourced by run.sh; defines only functions and constants.

# flat_field prints one flat field of qhost status by its full name (empty when it is absent).
flat_field() {
	qd --output json status | sed -n "s/.*\"$1\": *\"\\([^\"]*\\)\".*/\\1/p"
}

collision_seen() {
	flat_field source.opt-stacks2.stack.demo-stack.findings | grep -q name-collision
}

# scenario_sources proves the service with several stack folders. An installation bound to one repository by an
# earlier version is represented by the state layout that version left (stacks-repo.json, the engine state in
# deploy/), made from the current one while the service is stopped; the start must take it over with state and
# deploy key. Then a second repository becomes a second source: its own stack is deployed, a stack named like one of
# the first source is reported with name-collision and never deployed.
scenario_sources() {
	state=/var/lib/qhost-deploy
	url2=git.$domain:stacks2.git

	step 'an installation of the earlier single binding is taken over with its state and deploy key'
	key_before=$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	deployed_before=$(stack_field demo-stack deployed)
	run_before=$(stack_field demo-stack last_run)
	images_before=$(stack_images demo-stack)
	[ -n "$key_before" ] && [ -n "$deployed_before" ] && [ -n "$run_before" ] || fail 'the first source has no key, deployment or run to carry over'
	docker exec "$host" systemctl stop qhost-deploy || fail 'cannot stop the deploy service'
	url1=$(docker exec "$host" sed -n 's/.*"url": *"\([^"]*\)".*/\1/p' "$state/sources.json")
	[ "$url1" = "$url" ] || fail "sources.json does not bind $url: $url1"
	docker exec "$host" test -d "$state/sources/opt-stacks/deploy" || fail 'the engine state of the first source is not below sources/opt-stacks/deploy'
	docker exec "$host" sh -ec "
		mv $state/sources/opt-stacks/deploy $state/deploy
		rmdir $state/sources/opt-stacks $state/sources
		rm $state/sources.json
		printf '{\"version\":1,\"url\":\"$url1\",\"dir\":\"/opt/stacks\"}' >$state/stacks-repo.json
		chown qhost-deploy:qhost-deploy $state/stacks-repo.json && chmod 600 $state/stacks-repo.json" || fail 'cannot lay out the earlier state'
	docker exec "$host" systemctl start qhost-deploy || fail 'cannot start the deploy service on the earlier state'
	wait_for 'the deploy service channel' 20 docker exec "$host" sh -c 'ls /run/qhost-deploy | grep -q sock'
	wait_for 'the deploy service to answer' 20 qd status
	wait_for 'the poll of the taken-over source' 60 poll_done
	if docker exec "$host" test -e "$state/stacks-repo.json"; then fail 'the old binding file is still the binding'; fi
	if docker exec "$host" test -e "$state/deploy"; then fail 'the engine state was not moved'; fi
	docker exec "$host" test -d "$state/sources/opt-stacks/deploy" || fail 'the engine state is not below sources/opt-stacks'
	docker exec "$host" grep -q '"key": *"deploy-key"' "$state/sources.json" || fail "the first source does not keep the deploy key: $(docker exec "$host" cat "$state/sources.json")"
	[ "$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$key_before" ] || fail 'the deploy key changed with the takeover'
	[ "$(flat_field source.opt-stacks.path)" = /opt/stacks ] || fail "status does not show the first source: $(qd --output json status)"
	[ "$(stack_field demo-stack source)" = opt-stacks ] || fail "demo-stack is not listed with its source: $(qd --output json status)"
	[ "$(stack_field demo-stack deployed)" = "$deployed_before" ] || fail 'the deployed commit of demo-stack was lost with the takeover'
	[ "$(stack_field demo-stack last_run)" = "$run_before" ] || fail 'the takeover started a run of demo-stack'
	qd repo fetch >/dev/null || fail 'repo fetch fails with the taken-over key'
	log 'scenario: the single binding became the source opt-stacks with its state and key'

	step 'a second repository is bound as a second source with its own deploy key'
	out=$(qd --output json key generate --dir /opt/stacks2) || fail "key generate --dir failed: $out"
	pub2=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub2 in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	[ "$pub2" != "$key_before" ] || fail 'the second source got the key of the first'
	[ "$(qd --output json key show --dir /opt/stacks2 | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$pub2" ] || fail 'key show --dir differs from key generate --dir'
	[ "$(qd --output json key show | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')" = "$key_before" ] || fail 'a key for another folder replaced the key of the first source'
	printf '%s\n' "$pub2" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo set "$url2" --dir /opt/stacks2 --create --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set for the second source failed: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone the second source: $out"
	docker exec "$host" test -f /opt/stacks2/demo-second/compose.yaml || fail 'the clone is missing in /opt/stacks2'
	docker exec "$host" grep -qx 'ReadWritePaths=-/opt/stacks2' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf || fail 'the drop-in does not grant /opt/stacks2'
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(flat_field source.opt-stacks2.path)" = /opt/stacks2 ] || fail "status does not show the second source: $(qd --output json status)"

	step 'the own stack of the second source is deployed'
	wait_for 'the first run of demo-second' 180 stack_settled demo-second succeeded
	[ "$(stack_images demo-second)" = "registry.$domain/demo/app:1.0.0" ] || fail "demo-second does not run the image 1.0.0: $(stack_images demo-second)"
	[ "$(stack_field demo-second source)" = opt-stacks2 ] || fail "demo-second is not listed with its source: $(qd --output json status)"
	out=$(qd --output json stack list) || fail "stack list failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks2' || fail "stack list does not show the source of demo-second: $out"

	step 'a stack named like one of the first source is reported with name-collision and never deployed'
	wait_for 'the finding name-collision' 60 collision_seen
	[ "$(flat_field source.opt-stacks2.stack.demo-stack.source)" = opt-stacks2 ] || fail "the blocked entry has no source: $(qd --output json status)"
	[ -z "$(flat_field source.opt-stacks2.stack.demo-stack.deployed)" ] || fail 'the blocked stack was deployed'
	[ -z "$(flat_field source.opt-stacks2.stack.demo-stack.last_run)" ] || fail 'the blocked stack has a run'
	[ "$(stack_field demo-stack source)" = opt-stacks ] || fail 'the stack of the first source lost its name'
	[ "$(stack_field demo-stack deployed)" = "$deployed_before" ] || fail 'the stack of the first source changed'
	[ "$(stack_images demo-stack)" = "$images_before" ] || fail "demo-stack changed its image: $(stack_images demo-stack) instead of $images_before"
	if docker exec "$host" docker ps -a --format '{{.Names}}' --filter label=com.docker.compose.project=demo-stack | grep -q . &&
		docker exec "$host" docker ps -a --filter label=com.docker.compose.project=demo-stack --format '{{.Image}}' | grep -q ':2.0.0$'; then
		fail 'a container of the duplicate demo-stack runs'
	fi
	log 'scenario: second source deployed demo-second; its demo-stack is blocked by name-collision'
}

# source_stack_count prints how many stacks status lists for the source $1.
source_stack_count() {
	qd --output json status | grep -c "\"stack\\.[^.]*\\.source\": *\"$1\"" || true
}

# root_commit changes a file of the admin clone of the third repository and pushes it as a new commit.
root_commit() {
	as_admin "cd ~/stacks3 && git pull -q --ff-only && sed -i '$2' $1 &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -am '$3' && git push -q origin main"
}

# scenario_root_source proves a stack folder whose root holds the compose file: the whole folder is exactly one
# stack, a subfolder with a compose file of its own is only reported, a version change by commit is deployed and
# a failing one is rolled back with a revert commit of the whole folder.
scenario_root_source() {
	url3=git.$domain:stacks3.git
	image=registry.$domain/demo/app

	step 'a third repository with its compose file in the root is bound as one more source'
	out=$(qd --output json key generate --dir /opt/stacks3) || fail "key generate --dir failed: $out"
	pub3=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub3 in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	printf '%s\n' "$pub3" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo set "$url3" --dir /opt/stacks3 --create --fingerprint "$(git_ed25519_fingerprint)") || fail "repo set for the third source failed: $out"
	printf '%s' "$out" | grep -q '"cloned": *true' || fail "repo set did not clone the third source: $out"
	docker exec "$host" test -f /opt/stacks3/compose.yaml || fail 'the clone in /opt/stacks3 has no compose file in its root'

	step 'the root of the source is deployed as exactly one stack; its subfolder is no stack'
	wait_for 'the first run of demo-root' 180 stack_settled demo-root succeeded
	[ "$(source_stack_count opt-stacks3)" = 1 ] || fail "the third source is not exactly one stack: $(qd --output json status)"
	[ "$(stack_field demo-root source)" = opt-stacks3 ] || fail "demo-root is not listed with its source: $(qd --output json status)"
	[ -z "$(stack_field demo-extra source)" ] || fail 'the subfolder with a compose file is a stack'
	[ "$(stack_images demo-root)" = "$image:1.0.0" ] || fail "demo-root does not run the image 1.0.0: $(stack_images demo-root)"
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=demo-root --format '{{.Label "com.docker.compose.project.working_dir"}}')" = /opt/stacks3 ] ||
		fail 'demo-root does not run from the root of the source'
	[ -z "$(stack_images demo-extra)" ] || fail 'a container of the subfolder runs'
	out=$(qd --output json stack check demo-root) || fail "stack check of demo-root failed: $out"
	printf '%s' "$out" | jq -e '.errors == 0 and ([.stacks[].findings[] | select(.rule == "ignored-stack-folder")] | length) == 1' >/dev/null ||
		fail "stack check does not report ignored-stack-folder once: $out"
	as_admin "rm -rf ~/stacks3 && git clone -q $url3 ~/stacks3" || fail 'cannot clone the third repository as admin'

	step 'a version change by commit is deployed'
	prev=$(newest_run demo-root)
	root_commit .env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.1.0/' 'Update demo-root to 1.1.0'
	good_root=$(as_admin 'cd ~/stacks3 && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-root "$prev") || fail 'no run of demo-root started for the version change'
	wait_for "the run $run" 240 run_finished demo-root "$run"
	qd stack log "$run" | grep -q ' result succeeded' || fail "the run of the version change did not succeed: $(qd stack log "$run")"
	[ "$(stack_field demo-root deployed)" = "$good_root" ] || fail "demo-root is not recorded as deployed at $good_root"
	[ "$(stack_images demo-root)" = "$image:1.1.0" ] || fail "demo-root does not run 1.1.0: $(stack_images demo-root)"

	step 'a failing version is rolled back and the revert commit restores the whole folder'
	prev=$run
	root_commit .env 's/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=2.0.0/; s/^DEMO_HEALTH=.*/DEMO_HEALTH=fail/' 'Update demo-root to 2.0.0 with a broken healthcheck'
	bad_root=$(as_admin 'cd ~/stacks3 && git rev-parse HEAD')
	restart_service
	run=$(next_run demo-root "$prev") || fail 'no run of demo-root started for the failing commit'
	wait_for "the run $run" 240 run_finished demo-root "$run"
	run_log=$(qd stack log "$run")
	printf '%s' "$run_log" | grep -q ' result rolled-back' || fail "the run did not end rolled-back: $run_log"
	revert=$(as_admin 'cd ~/stacks3 && git pull -q --ff-only && git rev-parse HEAD')
	[ "$revert" != "$bad_root" ] || fail 'no revert commit reached the remote branch'
	meta=$(as_admin "cd ~/stacks3 && git log -1 --format='%an|%ae|%s|%P' $revert")
	[ "$meta" = "qhost|qhost@localhost.invalid|Revert the stack folder to $(printf '%s' "$good_root" | cut -c 1-12)|$bad_root" ] ||
		fail "the revert commit has the wrong author, subject or parent: $meta"
	[ "$(as_admin "cd ~/stacks3 && git rev-parse $revert^{tree}")" = "$(as_admin "cd ~/stacks3 && git rev-parse $good_root^{tree}")" ] ||
		fail 'the revert commit does not restore the whole folder'
	[ "$(stack_field demo-root deployed)" = "$revert" ] || fail "demo-root is not recorded as deployed at the revert commit $revert"
	[ "$(stack_images demo-root)" = "$image:1.1.0" ] || fail "demo-root does not run the previous image 1.1.0: $(stack_images demo-root)"
	log "scenario: demo-root, the root of /opt/stacks3, deployed 1.1.0 and rolled back 2.0.0 with the revert $revert"
}

# scenario_source_inspect proves source inspect on four kinds of folder below /srv/qd-inspect, which no stack folder
# uses: an empty folder to clone into, a clone with an origin, a repository without a remote and files
# without a repository. The answers are parsed as JSON, and the tree of the folders is the same before and after.
scenario_source_inspect() {
	base=/srv/qd-inspect
	step 'source inspect classifies an empty folder, a clone, a local repository and a folder of files'
	docker exec "$host" sh -ec "rm -rf '$base' && mkdir -p '$base/empty' '$base/clone' '$base/local' '$base/files'" ||
		fail 'cannot prepare the folders to inspect'
	docker exec "$host" sh -ec "git init -q '$base/clone' && git -C '$base/clone' remote add origin '$url'" ||
		fail 'cannot prepare the clone to inspect'
	docker exec "$host" sh -ec "git init -q '$base/local' && echo demo >'$base/local/notes.txt' && git -C '$base/local' add -A &&
		git -C '$base/local' -c user.name='E2E Inspect' -c user.email=inspect.qhoste2e.test commit -q -m 'First commit'" ||
		fail 'cannot prepare the local repository to inspect'
	printf 'services:\n  app:\n    image: registry.%s/demo/app:1.0.0\n' "$domain" |
		docker exec -i "$host" sh -ec "cat >'$base/files/compose.yaml'" || fail 'cannot write the compose file to inspect'
	tree_before=$(docker exec "$host" find "$base" -printf '%p %m %u %s\n' | sort)

	out=$(qd --output json source inspect "$base/empty" --url "$url") || fail "source inspect of an empty folder failed: $out"
	printf '%s' "$out" | jq -e --arg u "$url" '.case == "clone" and .url == $u and (.access.mode | length) > 0' >/dev/null ||
		fail "unexpected inspection of the empty folder: $out"
	out=$(qd --output json source inspect "$base/clone") || fail "source inspect of a clone failed: $out"
	printf '%s' "$out" | jq -e --arg u "$url" '.case == "adopt-clone" and .url == $u' >/dev/null ||
		fail "unexpected inspection of the clone: $out"
	out=$(qd --output json source inspect "$base/local") || fail "source inspect of a local repository failed: $out"
	printf '%s' "$out" | jq -e '.case == "local" and (has("url") | not)' >/dev/null ||
		fail "unexpected inspection of the local repository: $out"
	out=$(qd --output json source inspect "$base/files") || fail "source inspect of a folder of files failed: $out"
	printf '%s' "$out" | jq -e '.case == "files" and (.planned_files | index("compose.yaml")) != null and (.stacks | length) >= 1' >/dev/null ||
		fail "unexpected inspection of the folder of files: $out"
	out=$(qd source inspect "$base/files") || fail "source inspect as text failed: $out"
	printf '%s' "$out" | grep -q 'files' || fail "the text answer does not name the case: $out"

	[ "$(docker exec "$host" find "$base" -printf '%p %m %u %s\n' | sort)" = "$tree_before" ] ||
		fail 'source inspect changed the folders it inspected'
	docker exec "$host" rm -rf "$base" || fail 'cannot remove the inspected folders'
	log 'scenario: source inspect told clone, adopt-clone, local and files and changed nothing'
}

# key_fp_field prints the fingerprint of the qhost key output $1.
key_fp_field() {
	printf '%s\n' "$1" | sed -n 's/.*"fingerprint": *"\([^"]*\)".*/\1/p'
}

# git_unregister_key removes the public key with the base64 part $1 from the authorized keys of the git server, so
# a later fetch proves the replacement key.
git_unregister_key() {
	docker exec "$prefix-git" sh -ec 'grep -vF -- "$1" /srv/git/.ssh/authorized_keys >/tmp/ak.new || true
		cat /tmp/ak.new >/srv/git/.ssh/authorized_keys && rm -f /tmp/ak.new' sh "$1"
}

# scenario_key_set proves that an existing key becomes the deploy key of a stack folder, through the command line
# for /opt/stacks2 and through the Stacks area for /opt/stacks3, which is released at the end. Both folders have a generated key, so each store is
# a replacement: it is refused without the confirmation (--replace, the answer n) and leaves the key unchanged.
scenario_key_set() {
	tui_log=$work/tui-key-screens.log
	: >"$tui_log"
	kdir=$(docker exec "$host" mktemp -d) || fail 'cannot create a directory for the test keys'
	docker exec "$host" sh -ec "ssh-keygen -q -t ed25519 -N '' -C key-a-e2e -f $kdir/a && ssh-keygen -q -t ed25519 -N '' -C key-b-e2e -f $kdir/b" ||
		fail 'cannot generate the test keys'
	fp_a=$(docker exec "$host" ssh-keygen -lf "$kdir/a.pub" | cut -d ' ' -f 2)
	fp_b=$(docker exec "$host" ssh-keygen -lf "$kdir/b.pub" | cut -d ' ' -f 2)
	body_a=$(docker exec "$host" sed -n 2p "$kdir/a")
	body_b=$(docker exec "$host" sed -n 2p "$kdir/b")
	pub_a=$(docker exec "$host" cut -d ' ' -f 1,2 "$kdir/a.pub")
	pub_b=$(docker exec "$host" cut -d ' ' -f 1,2 "$kdir/b.pub")
	case $fp_a$fp_b in SHA256:*SHA256:*) ;; *) fail "no fingerprints of the test keys: $fp_a $fp_b" ;; esac
	[ -n "$body_a" ] && [ -n "$body_b" ] || fail 'the test keys have no body line'

	step 'key set stores an existing key for /opt/stacks2 only with --replace; a fetch over it succeeds'
	show2=$(qd --output json key show --dir /opt/stacks2) || fail "key show --dir failed: $show2"
	old2=$(key_pub_field "$show2")
	out=$(qd_fails --output json key set --dir /opt/stacks2 --file "$kdir/a") || fail "key set replaced a key without --replace: $out"
	printf '%s' "$out" | grep -q -- '--replace' || fail "the refusal does not name --replace: $out"
	[ "$(key_pub_field "$(qd --output json key show --dir /opt/stacks2)")" = "$old2" ] || fail 'a refused key set changed the key'
	out=$(qd --output json key set --dir /opt/stacks2 --file "$kdir/a" --replace) || fail "key set --replace failed: $out"
	[ "$(key_fp_field "$out")" = "$fp_a" ] || fail "the fingerprint of the stored key differs from ssh-keygen: $out vs $fp_a"
	[ "$(key_pub_field "$out")" = "$pub_a" ] || fail "the public key of the stored key differs from the test key: $out"
	[ "$(key_fp_field "$(qd --output json key show --dir /opt/stacks2)")" = "$fp_a" ] || fail 'key show --dir does not show the stored key'
	git_unregister_key "${old2#* }"
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks2.error' || fail "the old key is gone from the git server, yet opt-stacks2 fetched: $out"
	printf '%s\n' "$pub_a" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	if printf '%s' "$out" | grep -q 'opt-stacks2.error'; then fail "opt-stacks2 does not fetch over the stored key: $out"; fi
	log "scenario: key set stored the test key for /opt/stacks2 ($fp_a) and the fetch over it succeeds"

	step 'the interface stores an existing key for /opt/stacks3 after the question; n keeps the key'
	old3=$(key_pub_field "$(qd --output json key show --dir /opt/stacks3)")
	fp_old3=$(key_fp_field "$(qd --output json key show --dir /opt/stacks3)")
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_folder_to /opt/stacks3
	tui_send e
	tui_wait 'stack folder: /opt/stacks3' 10
	tui_send e
	tui_wait 'store an existing key' 10
	tui_send F2
	tui_wait 'Store the deploy key' 10
	docker exec "$host" cat "$kdir/b" | docker exec -i -u "$admin" "$host" tmux -L qdtui load-buffer - || fail 'cannot load the test key into tmux'
	tui_tmux paste-buffer -p -t qdtui || fail 'cannot paste the test key'
	tui_wait '(hidden; lines: ' 10
	tui_has "$body_b" && fail "the pasted private key is on the screen: $(tui_screen)"
	tui_send F2
	tui_wait 'Replace the existing deploy key?' 10
	tui_send n
	sleep 0.5
	tui_has 'Store the deploy key' || fail "answering n closed the form: $(tui_screen)"
	[ "$(key_fp_field "$(qd --output json key show --dir /opt/stacks3)")" = "$fp_old3" ] || fail 'the answer n changed the key of /opt/stacks3'
	tui_send F2
	tui_wait 'Replace the existing deploy key?' 10
	tui_send y
	tui_wait 'The deploy key is stored' 30
	tui_wait "$fp_b" 10
	tui_has "$body_b" && fail "the private key is on the screen: $(tui_screen)"
	tui_send Enter
	out=$(qd --output json key show --dir /opt/stacks3)
	[ "$(key_fp_field "$out")" = "$fp_b" ] || fail "the key of /opt/stacks3 is not the stored key: $out vs $fp_b"
	[ "$(key_pub_field "$out")" = "$pub_b" ] || fail "the public key of /opt/stacks3 differs from the test key: $out"
	git_unregister_key "${old3#* }"
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	printf '%s' "$out" | grep -q 'opt-stacks3.error' || fail "the old key is gone from the git server, yet opt-stacks3 fetched: $out"
	printf '%s\n' "$pub_b" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
	out=$(qd --output json repo fetch) || fail "repo fetch failed: $out"
	if printf '%s' "$out" | grep -q 'opt-stacks3.error'; then fail "opt-stacks3 does not fetch over the stored key: $out"; fi
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true
	log "scenario: the interface stored the test key for /opt/stacks3 ($fp_b) after the question, the fetch over it succeeds"

	step 'no private test key shows on a captured screen, in the journal or in a key output'
	[ "$(wc -l <"$tui_log")" -gt 40 ] || fail 'too few screens were captured to check'
	journal_text=$(journal)
	for body in "$body_a" "$body_b"; do
		if grep -qF -- "$body" "$tui_log"; then fail 'a private test key shows on a captured screen'; fi
		if printf '%s' "$journal_text" | grep -qF -- "$body"; then fail 'a private test key shows in the journal'; fi
		if printf '%s' "$out$show2" | grep -qF -- "$body"; then fail 'a private test key shows in a key output'; fi
	done
	docker exec "$host" rm -rf "$kdir"
}
