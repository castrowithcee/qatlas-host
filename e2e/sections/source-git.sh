#!/bin/sh
# Section source-git: a local stack folder gets a remote: an empty repository receives its history, a differing one
# is shown and left alone without the confirmation, also in the terminal interface; stacks move between sources.
# Needs source-local: scenario_source_remote turns its folder of files /srv/qd-files/web into a remote source; the
# clone /opt/stacks3 comes from sources, which source-local needs.
# Sourced by run.sh; defines only functions and constants.

# tree_sums prints the checksum of every file of the folder $1 outside .git.
tree_sums() {
	docker exec "$host" sh -ec "cd '$1' && find . -path ./.git -prune -o -type f -exec sha256sum {} + | sort -k2"
}

# copy_stacks3 copies the tracked files of /opt/stacks3 without .git to the folder $1, readable by the service.
copy_stacks3() {
	docker exec "$host" sh -ec "rm -rf '$1' && mkdir -p '$1' &&
		git -c safe.directory=/opt/stacks3 -C /opt/stacks3 ls-files -z | tar -C /opt/stacks3 --null -T - -cf - | tar -C '$1' -xf - &&
		chmod -R a+rX '$1'" || fail "cannot copy /opt/stacks3 to $1"
}

# scenario_source_remote proves source remote and source add --url. The folder of files of the scenario before
# becomes a remote source by pushing its history to an empty repository. A repository whose history once held a
# secrets file is not pushed, however it looks now. A folder of files with the files of /opt/stacks3 takes over
# the history of that repository, and one whose .env differs starts on it and is not touched without the
# confirmation. Nothing here pushes with force.
scenario_source_remote() {
	base=/srv/qd-files/web
	url4=git.$domain:stacks4.git
	url5=git.$domain:stacks5.git
	url3=git.$domain:stacks3.git

	step 'an empty remote: source remote without the confirmation changes nothing'
	register_folder_key "$base"
	run_before=$(newest_run demo-files)
	[ -n "$run_before" ] || fail 'demo-files has no run'
	out=$(qd_fails source remote "$base" "$url4" --fingerprint "$(git_ed25519_fingerprint)") || fail "source remote without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source remote without --yes does not ask for confirmation: $out"
	[ -z "$(files_git remote)" ] || fail 'a refused source remote gave the folder a remote'
	[ -z "$(remote_head "$url4")" ] || fail 'a refused source remote pushed'
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | .kind')" = local ] || fail 'a refused source remote changed the kind of the source'

	step 'source remote --yes pushes the local history to the empty repository'
	head_before=$(files_git rev-parse HEAD)
	out=$(qd --output json source remote "$base" "$url4" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source remote failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e --arg u "$url4" --arg h "$head_before" \
		'.state == "empty" and .pushed == true and .branch == "main" and .url == $u and (.head | length) >= 7 and (.head as $x | $h | startswith($x))' >/dev/null ||
		fail "unexpected source remote result: $out"
	[ "$(remote_head "$url4")" = "$head_before" ] || fail "the repository does not hold the head of the folder: $(remote_head "$url4")"
	[ "$(qd --output json source list | jq -r --arg p "$base" '.sources[] | select(.path == $p) | "\(.kind) \(.url)"')" = "remote $url4" ] ||
		fail "the folder is not a remote source with the address: $(qd --output json source list)"
	[ "$(files_git rev-parse HEAD)" = "$head_before" ] || fail 'source remote moved the head of the folder'
	[ "$(newest_run demo-files)" = "$run_before" ] || fail 'source remote started a run of demo-files'
	[ "$(stack_images demo-files)" = "registry.$domain/demo/app:1.1.0" ] || fail "demo-files does not run 1.1.0 any more: $(stack_images demo-files)"

	step 'a history that once held a secrets file is not pushed'
	docker exec "$host" sh -ec "rm -rf /srv/qd-secret && mkdir -p /srv/qd-secret/demo-secret && chmod 0755 /srv/qd-secret /srv/qd-secret/demo-secret" ||
		fail 'cannot create the secret folder'
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >/srv/qd-secret/demo-secret/compose.yaml"
name: demo-secret

services:
  app:
    image: registry.$domain/demo/app:1.0.0
    restart: unless-stopped
    environment:
      DEMO_NAME: demo-secret
      DEMO_VERSION: 1.0.0
      DEMO_HEALTH: ok
EOF2
	printf 'DEMO_SECRET=not-a-real-secret\n' | docker exec -i "$host" sh -ec "cat >/srv/qd-secret/demo-secret/secrets.env"
	docker exec "$host" sh -ec "cd /srv/qd-secret && chmod 0644 demo-secret/compose.yaml demo-secret/secrets.env && git init -q -b main && git add -A &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the demo-secret stack with a secrets file' &&
		git rm -q demo-secret/secrets.env &&
		git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Remove the secrets file'" ||
		fail 'cannot create the secret repository'
	first_secret=$(docker exec "$host" git -C /srv/qd-secret rev-list --max-parents=0 HEAD | cut -c 1-7)
	[ -n "$first_secret" ] || fail 'the secret repository has no first commit'
	[ -z "$(docker exec "$host" git -C /srv/qd-secret ls-files | grep secrets.env)" ] || fail 'the secrets file is still in the last commit'
	out=$(qd --output json source add /srv/qd-secret --yes) || fail "source add of the secret repository failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local"' >/dev/null || fail "unexpected source add result: $out"
	wait_for 'the deploy service to answer' 30 qd status
	register_folder_key /srv/qd-secret
	out=$(qd_fails source remote /srv/qd-secret "$url5" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source remote pushed secrets: $out"
	printf '%s' "$out" | grep -q 'secrets-found' || fail "source remote does not report secrets-found: $out"
	printf '%s' "$out" | grep -q "$first_secret" || fail "the output does not name the commit $first_secret: $out"
	printf '%s' "$out" | grep -q 'secrets.env' || fail "the output does not name secrets.env: $out"
	if printf '%s' "$out" | grep -q 'not-a-real-secret'; then fail 'the output shows the value of the secret'; fi
	[ "$(qd --output json source list | jq -r '.sources[] | select(.path == "/srv/qd-secret") | .kind')" = local ] || fail 'the refused source remote changed the kind of the source'
	[ -z "$(docker exec "$host" git -C /srv/qd-secret remote)" ] || fail 'the refused source remote gave the folder a remote'
	[ -z "$(as_admin "git ls-remote $url5")" ] || fail "the refused push created a branch: $(as_admin "git ls-remote $url5")"
	wait_for 'the first run of demo-secret' 180 stack_settled demo-secret succeeded
	out=$(qd --output json source remove /srv/qd-secret --yes) || fail "source remove of the secret folder failed: $out"
	docker exec "$host" docker compose -p demo-secret down >/dev/null 2>&1 || fail 'cannot remove the containers of demo-secret'
	log 'scenario: source remote pushed the folder of files to an empty repository and refused a history with a secrets file'

	step 'the files of /opt/stacks3 are checked against its repository before they are copied'
	repo_head3=$(remote_head "$url3")
	[ -n "$repo_head3" ] || fail 'stacks3 has no head'
	[ "$(docker exec "$host" git -c safe.directory=/opt/stacks3 -C /opt/stacks3 rev-parse HEAD)" = "$repo_head3" ] ||
		fail 'the head of /opt/stacks3 is not the head of its repository'
	[ -z "$(docker exec "$host" git -c safe.directory=/opt/stacks3 -C /opt/stacks3 status --porcelain)" ] || fail '/opt/stacks3 has local changes'

	step 'a folder of files equal to the repository takes over its history without a push'
	equal=/srv/qd-b-equal
	copy_stacks3 "$equal"
	register_folder_key "$equal"
	out=$(qd --output json source inspect "$equal" --url "$url3") || fail "source inspect of the equal folder failed: $out"
	printf '%s' "$out" | jq -e '.case == "files"' >/dev/null || fail "the equal folder is not a folder of files: $out"
	sums_before=$(tree_sums "$equal")
	out=$(qd --output json source add "$equal" --url "$url3" --fingerprint "$(git_ed25519_fingerprint)" --yes) || fail "source add --url for the equal folder failed: $out"
	printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "remote" and .state == "equal" and .pushed == false' >/dev/null ||
		fail "unexpected source add result for the equal folder: $out"
	# extra/compose.yaml is no stack's file, but the repository versions it, so it is kept and named.
	printf '%s' "$out" | grep -qF 'extra/compose.yaml (versioned in the remote)' || fail "the preview does not name extra/compose.yaml as versioned in the remote: $out"
	# Nothing is left out, so the .gitignore stays as the repository has it and the trees are equal.
	printf '%s' "$out" | grep -qF 'No file in the folder is written.' || fail "the preview of the equal folder announces a change to a file: $out"
	wait_for 'the deploy service to answer' 30 qd status
	[ "$(folder_git "$equal" rev-parse HEAD)" = "$repo_head3" ] || fail "the head of the folder is not the head of the repository: $(folder_git "$equal" rev-parse HEAD)"
	folder_git "$equal" ls-files | grep -qx 'extra/compose.yaml' || fail "the folder does not track extra/compose.yaml: $(folder_git "$equal" ls-files)"
	[ "$(folder_git "$equal" remote)" = origin ] || fail "the folder has not exactly the remote origin: $(folder_git "$equal" remote)"
	folder_git "$equal" remote get-url origin | grep -q 'stacks3.git' || fail "origin is not the repository: $(folder_git "$equal" remote get-url origin)"
	[ "$(qd --output json source list | jq -r --arg p "$equal" '.sources[] | select(.path == $p) | .kind')" = remote ] || fail 'the equal folder is not a remote source'
	[ "$(tree_sums "$equal")" = "$sums_before" ] || fail 'the takeover changed the content of a file'
	[ "$(remote_head "$url3")" = "$repo_head3" ] || fail 'taking over an equal remote changed it'
	docker exec "$host" git -c safe.directory=/opt/stacks3 -C /opt/stacks3 cat-file -e "$(remote_head "$url3"):extra/compose.yaml" ||
		fail 'the repository no longer versions extra/compose.yaml'
	out=$(qd --output json source remove "$equal" --yes) || fail "source remove of the equal folder failed: $out"

	step 'a folder of files that differs is shown and changes nothing without the confirmation'
	diff_dir=/srv/qd-b-diff
	copy_stacks3 "$diff_dir"
	docker exec "$host" sed -i 's/^DEMO_HEALTH=.*/DEMO_HEALTH=degraded/' "$diff_dir/.env"
	docker exec "$host" grep -qx 'DEMO_HEALTH=degraded' "$diff_dir/.env" || fail 'cannot change the .env of the differing folder'
	register_folder_key "$diff_dir"
	meta_before=$(docker exec "$host" stat -c '%U:%G %a' "$diff_dir")
	out=$(qd_fails source add "$diff_dir" --url "$url3" --fingerprint "$(git_ed25519_fingerprint)") || fail "source add --url for the differing folder without --yes succeeded: $out"
	printf '%s' "$out" | grep -q 'confirmation-required' || fail "source add --url without --yes does not ask for confirmation: $out"
	printf '%s' "$out" | grep -q '\.env' || fail "the output does not name .env as a differing path: $out"
	if docker exec "$host" test -e "$diff_dir/.git"; then fail 'a refused source add made a repository'; fi
	if list_has_path "$diff_dir"; then fail 'a refused source add bound the folder'; fi
	[ "$(docker exec "$host" stat -c '%U:%G %a' "$diff_dir")" = "$meta_before" ] || fail 'a refused source add changed the owner, group or mode of the folder'
	[ "$(remote_head "$url3")" = "$repo_head3" ] || fail 'a refused source add changed the repository'
	docker exec "$host" rm -rf "$diff_dir" /srv/qd-secret "$equal"
	log 'scenario: source add --url took over the history of an equal repository and left a differing folder alone'
}

# qd_stdout runs the qhost CLI on the host as root with only standard output captured, so the JSON of a command that
# fails can still be parsed; its standard error goes to $work/qd-stderr.log.
qd_stdout() {
	docker exec -i "$host" qhost "$@" 2>"$work/qd-stderr.log" </dev/null
}

# source_head_is succeeds when the working copy of the folder $1 is at the commit $2.
source_head_is() {
	[ "$(folder_git "$1" rev-parse HEAD)" = "$2" ]
}

# source_follows waits up to $4 seconds until the working copy of the source $2 is at the commit $3, described as $1.
# On a timeout it prints what the service knows of the source and the end of its log, then fails.
source_follows() {
	what=$1 dir=$2 want=$3 tries=$4
	i=0
	until source_head_is "$dir" "$want"; do
		i=$((i + 1))
		if [ "$i" -ge "$tries" ]; then
			printf 'e2e: source %s wanted at %s: %s\n' "$dir" "$want" \
				"$(qd --output json source list | jq -c --arg p "$dir" '.sources[] | select(.path == $p) | {head, remote_head, last_poll, note, poll_error}')" >&2
			printf 'e2e: status of %s:\n%s\n' "$dir" "$(folder_git "$dir" status --porcelain)" >&2
			printf 'e2e: end of the deploy service log:\n%s\n' "$(docker exec "$host" journalctl -u qhost-deploy -n 20 -o cat --no-pager 2>&1)" >&2
			fail "timeout waiting for $what"
		fi
		sleep 1
	done
}

# remote_git runs git on the bare repository stacks.git in the git container as root; stderr is merged.
remote_git() {
	docker exec "$prefix-git" git -c safe.directory='*' --git-dir=/srv/git/stacks.git "$@" 2>&1
}

# commit_has_prefix succeeds when the commit $1 starts with the abbreviated commit $2 (at least 7 characters).
commit_has_prefix() {
	[ "${#2}" -ge 7 ] && case $1 in "$2"*) true ;; *) false ;; esac
}

# move_project starts a foreign Compose project in the folder $1 from the image tag $2: a bind mount ./data with an
# invented record and a named volume. Its Compose project name is the name of the folder.
move_project() {
	mp_dir=$1 mp_tag=$2
	docker exec "$host" sh -ec "mkdir -p '$mp_dir/data' && chmod 0755 '$mp_dir' '$mp_dir/data'" ||
		fail "cannot create the project folder $mp_dir"
	cat <<EOF2 | docker exec -i "$host" sh -ec "cat >'$mp_dir/compose.yaml'"
services:
  app:
    image: registry.$domain/demo/app:\${DEMO_APP_VERSION}
    restart: unless-stopped
    environment:
      DEMO_NAME: move-app
      DEMO_VERSION: \${DEMO_APP_VERSION}
      DEMO_HEALTH: \${DEMO_HEALTH}
    volumes:
      - ./data:/data
      - state:/state

volumes:
  state:
EOF2
	printf 'DEMO_APP_VERSION=%s\nDEMO_HEALTH=ok\n' "$mp_tag" | docker exec -i "$host" sh -ec "cat >'$mp_dir/.env'"
	printf 'invented record 1\n' | docker exec -i "$host" sh -ec "cat >'$mp_dir/data/records.txt'"
	docker exec "$host" sh -ec "chmod 0644 '$mp_dir/compose.yaml' '$mp_dir/.env' '$mp_dir/data/records.txt'" || fail "cannot set the modes of the files in $mp_dir"
	docker exec "$host" sh -ec "cd '$mp_dir' && docker compose up -d" >/dev/null 2>&1 || fail "cannot start the project in $mp_dir"
	wait_for "the project ${mp_dir##*/}" 60 stack_healthy "${mp_dir##*/}"
}

# move_meta prints owner, group and mode of the folder $1 and of its direct children.
move_meta() {
	docker exec "$host" find "$1" -maxdepth 1 -printf '%p %u %g %m\n' | sort
}

# scenario_source_move proves source move. A person runs a foreign Compose project from /opt/qd-move/moveapp, outside
# every source, with the bind mount ./data, an invented data file and a named volume. The preview into /opt/stacks, a
# source with a remote in the git container, shows the target folder, the files a commit would take (compose.yaml and
# .env, not the data) and the lines for the .gitignore, and changes nothing, in the folders, the containers, the source
# and the remote. A source that is a root stack is reported as a block with reason and fix, and so is a .env with a
# secret-like key when the source has a remote, with the key and never the value. With --yes the folder lies in the
# source, the commit is in the remote without the data, the containers run in the new folder on the same data file and
# the named volume is the same, the working copy of the source is clean, and a commit pushed from outside is taken over
# by fast-forward. A second project whose image tag exists only on the host fails its run at the pull: the move is taken
# back, the project runs in its old folder with its data, the remote holds a revert of the commit and the source is clean.
scenario_source_move() {
	base=/opt/qd-move/moveapp
	into=/opt/stacks
	secret_value=not-a-real-secret-4711

	move_url=git.$domain:stacks.git

	step 'a person starts a foreign Compose project with a data folder outside every source'
	docker exec "$host" rm -rf /opt/qd-move || fail 'cannot clear /opt/qd-move'
	move_project "$base" 1.0.0
	ids_before=$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=moveapp | sort)
	[ -n "$ids_before" ] || fail 'the project to move has no running container'
	sums_project=$(tree_sums "$base")
	sums_source=$(tree_sums "$into")
	source_head=$(folder_git "$into" rev-parse HEAD)
	# Earlier scenarios may leave untracked paths in the source; the preview must not add to them.
	source_status=$(folder_git "$into" status --porcelain)
	sources_before=$(qd --output json source list | jq -c '[.sources[] | [.path, .kind, .head]]')
	changes_before=$(qd --output json source changes "$into" | jq -c .changes)
	remote_before=$(remote_head "$move_url")
	[ -n "$remote_before" ] || fail 'the remote of the source has no head'

	step 'the preview shows the target folder, the files and the exclusions'
	out=$(qd --output json source move "$base" --into "$into") || fail "source move failed: $out"
	printf '%s' "$out" | jq -e --arg b "$base" --arg t "$into/moveapp" \
		'.path == $b and .target == $t and .engine == "system" and .source_kind == "remote" and .project == "moveapp" and .new_project == "moveapp" and .movable == true and (.blocks | length) == 0' >/dev/null ||
		fail "unexpected source move result: $out"
	printf '%s' "$out" | jq -e '(.planned_files | index("compose.yaml")) != null and (.planned_files | index(".env")) != null and ([.planned_files[] | select(startswith("data"))] | length) == 0' >/dev/null ||
		fail "the planned files are not compose.yaml and .env: $out"
	printf '%s' "$out" | jq -e '([.excluded[] | select(.path == "data" and .reason == "bind-mount")] | length) == 1 and .ignore.action == "create" and (.ignore.lines | index("/data")) != null' >/dev/null ||
		fail "the data folder is not left out and put in the .gitignore: $out"
	printf '%s' "$out" | jq -e '([.warnings[].code] | index("downtime")) != null' >/dev/null || fail "no downtime warning for a running project: $out"
	out=$(qd source move "$base" --into "$into") || fail "source move in text failed: $out"
	printf '%s' "$out" | grep -qF "$into/moveapp" || fail "the text does not show the target folder: $out"
	printf '%s' "$out" | grep -q '^PLANNED_FILE .*compose.yaml' || fail "the text does not list compose.yaml: $out"

	step 'nothing changed: the containers run on, the folders are as they were'
	[ "$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=moveapp | sort)" = "$ids_before" ] ||
		fail 'the containers of the project changed'
	stack_healthy moveapp || fail 'the project is not healthy after the preview'
	[ "$(tree_sums "$base")" = "$sums_project" ] || fail 'the preview changed the project folder'
	if docker exec "$host" test -e "$base/.gitignore"; then fail 'the preview wrote a .gitignore into the project folder'; fi
	[ "$(tree_sums "$into")" = "$sums_source" ] || fail 'the preview changed the source folder'
	if docker exec "$host" test -e "$into/moveapp"; then fail 'the preview made the target folder'; fi
	[ "$(folder_git "$into" rev-parse HEAD)" = "$source_head" ] || fail 'the preview committed to the source'
	[ "$(folder_git "$into" status --porcelain)" = "$source_status" ] || fail "the preview left changes in the source: $(folder_git "$into" status --porcelain)"
	if docker exec "$host" test -e "$base/.git"; then fail 'the preview made a repository in the project folder'; fi
	[ "$(qd --output json source list | jq -c '[.sources[] | [.path, .kind, .head]]')" = "$sources_before" ] || fail 'the preview changed the sources'
	[ -z "$(stack_field moveapp source)" ] || fail 'the preview made a stack moveapp'
	[ "$(remote_head "$move_url")" = "$remote_before" ] || fail 'the preview changed the remote'

	step 'a secret-like key in .env blocks the move into a source with a remote, by key and never by value'
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\nDEMO_API_TOKEN=%s\n' "$secret_value" | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	out=$(qd --output json source move "$base" --into "$into") || fail "source move with a secret in .env failed: $out"
	printf '%s' "$out" | jq -e '.movable == false and ([.blocks[] | select(.code == "secrets-in-env" and .items == ["DEMO_API_TOKEN"] and (.reason | length) > 0 and (.fix | contains("secrets.env")))] | length) == 1' >/dev/null ||
		fail "the secret in .env is not reported as a block with its key: $out"
	if printf '%s' "$out" | grep -q "$secret_value"; then fail 'the preview shows the value of the secret'; fi
	out=$(qd source move "$base" --into "$into") || fail "source move in text with a secret in .env failed: $out"
	if printf '%s' "$out" | grep -q "$secret_value"; then fail 'the text shows the value of the secret'; fi
	printf 'DEMO_APP_VERSION=1.0.0\nDEMO_HEALTH=ok\n' | docker exec -i "$host" sh -ec "cat >'$base/.env'"
	[ "$(tree_sums "$base")" = "$sums_project" ] || fail 'the project folder is not as it was after the secret check'

	step 'a source that is a root stack is reported as a block with reason and fix'
	out=$(qd --output json source move "$base" --into /opt/stacks3) || fail "source move into the root stack failed: $out"
	printf '%s' "$out" | jq -e '.movable == false and ([.blocks[] | select(.code == "root-stack" and (.reason | length) > 0 and (.fix | length) > 0)] | length) == 1' >/dev/null ||
		fail "the root stack is not reported as a block: $out"
	if docker exec "$host" test -e /opt/stacks3/moveapp; then fail 'the preview made a folder in the root stack'; fi
	[ "$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=moveapp | sort)" = "$ids_before" ] ||
		fail 'the containers of the project changed'

	step 'source move --yes moves the project into the source, pushes the commit and runs the project there'
	data_inode=$(docker exec "$host" stat -c %i "$base/data/records.txt")
	data_sum=$(docker exec "$host" sh -c "sha256sum <'$base/data/records.txt'")
	volume_created=$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' moveapp_state)
	[ -n "$volume_created" ] || fail 'the project has no named volume moveapp_state'
	rc=0
	out=$(qd_stdout --output json source move "$base" --into "$into" --yes) || rc=$?
	[ "$rc" = 0 ] || fail "source move --yes exited $rc: $out $(cat "$work/qd-stderr.log")"
	printf '%s' "$out" | jq -e --arg t "$into/moveapp" \
		'.result == "moved" and .moved == true and .location == $t and .project == "moveapp" and .run_result == "succeeded" and (.commit | length) >= 7 and has("revert") == false and
		([.steps[] | .step + ":" + .result] == ["down:done", "rename:done", "permissions:done", "commit:done", "deploy:done"]) and (.rollback | length) == 0 and (.remaining | length) == 0' >/dev/null ||
		fail "unexpected source move --yes result: $out"
	move_commit=$(printf '%s' "$out" | jq -r .commit)
	move_head=$(remote_head "$move_url")
	commit_has_prefix "$move_head" "$move_commit" || fail "the remote head $move_head is not the commit $move_commit"
	[ "$(remote_git ls-tree -r --name-only "$move_head" moveapp)" = "$(printf 'moveapp/.env\nmoveapp/.gitignore\nmoveapp/compose.yaml')" ] ||
		fail "the remote holds the wrong files of moveapp: $(remote_git ls-tree -r --name-only "$move_head" moveapp)"
	remote_git show "$move_head:moveapp/.gitignore" | grep -qx '/data' || fail 'the .gitignore in the remote does not leave out /data'
	if docker exec "$host" test -e "$base"; then fail 'the old folder is still there'; fi
	docker exec "$host" test -f "$into/moveapp/compose.yaml" || fail 'the folder is not in the source'
	wait_for 'the source at the commit of the move' 60 source_head_is "$into" "$move_head"
	wait_for 'the moved project' 60 stack_healthy moveapp
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=moveapp --format '{{.Label "com.docker.compose.project.working_dir"}}' | sort -u)" = "$into/moveapp" ] ||
		fail 'the containers do not run in the new folder'
	[ "$(docker exec "$host" docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Source}}{{end}}{{end}}' $(docker exec "$host" docker ps -q --filter label=com.docker.compose.project=moveapp))" = "$into/moveapp/data" ] ||
		fail 'the data folder is not mounted from the new folder'
	[ "$(docker exec "$host" stat -c %i "$into/moveapp/data/records.txt")" = "$data_inode" ] || fail 'the data file is not the same file'
	[ "$(docker exec "$host" sh -c "sha256sum <'$into/moveapp/data/records.txt'")" = "$data_sum" ] || fail 'the data file changed'
	[ "$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' moveapp_state)" = "$volume_created" ] || fail 'the named volume is not the same'
	[ "$(stack_field moveapp deployed)" = "$move_head" ] || fail "moveapp is not recorded as deployed at $move_head: $(stack_field moveapp deployed)"

	step 'the working copy of the source is clean and a commit from outside is taken over by fast-forward'
	[ "$(folder_git "$into" status --porcelain)" = "$source_status" ] || fail "the move left changes in the source: $(folder_git "$into" status --porcelain)"
	[ "$(qd --output json source changes "$into" | jq -c .changes)" = "$changes_before" ] || fail "source changes lists the move: $(qd --output json source changes "$into")"
	as_admin "cd ~/stacks && git pull -q --ff-only && printf 'invented note\n' >move-note.txt && git add move-note.txt &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Add a note' && git push -q origin main" ||
		fail 'cannot push a commit from outside'
	foreign=$(remote_head "$move_url")
	[ -n "$foreign" ] && [ "$foreign" != "$move_head" ] || fail 'the commit from outside is not the head of the remote'
	# The poll that the commit of the move woke was the last one; the next follows only after the poll interval of
	# five minutes. The restart makes the engine poll at once.
	restart_service
	source_follows 'the source to take over the commit from outside' "$into" "$foreign" 180
	docker exec "$host" test -f "$into/move-note.txt" || fail 'the fast-forward did not bring the file'
	[ "$(folder_git "$into" status --porcelain)" = "$source_status" ] || fail "the fast-forward left changes in the source: $(folder_git "$into" status --porcelain)"

	step 'the moved project is removed from the source again'
	docker exec "$host" docker compose -p moveapp down -v >/dev/null 2>&1 || fail 'cannot remove the containers of moveapp'
	as_admin "cd ~/stacks && git pull -q --ff-only && git rm -rq moveapp move-note.txt &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Remove the moved project' && git push -q origin main" ||
		fail 'cannot remove the moved project from the remote'
	cleaned=$(remote_head "$move_url")
	restart_service
	source_follows 'the source to drop the moved project' "$into" "$cleaned" 180
	# The data folder is not versioned, so it stays behind.
	docker exec "$host" rm -rf "$into/moveapp"
	[ "$(folder_git "$into" status --porcelain)" = "$source_status" ] || fail "the clean-up left changes in the source: $(folder_git "$into" status --porcelain)"

	step 'a project whose image cannot be pulled at the new place is taken back'
	fbase=/opt/qd-move/movefail
	fimage=registry.$domain/demo/app
	docker exec "$host" sh -ec "docker pull -q $fimage:1.0.0 >/dev/null && docker tag $fimage:1.0.0 $fimage:local-only" ||
		fail 'cannot tag the image only on the host'
	move_project "$fbase" local-only
	fail_meta=$(move_meta "$fbase")
	fail_sum=$(tree_sums "$fbase")
	fail_inode=$(docker exec "$host" stat -c %i "$fbase/data/records.txt")
	fail_volume=$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' movefail_state)
	fail_remote=$(remote_head "$move_url")
	rc=0
	out=$(qd_stdout --output json source move "$fbase" --into "$into" --yes) || rc=$?
	[ "$rc" = 1 ] || fail "source move --yes of the unpullable project exited $rc, not 1: $out $(cat "$work/qd-stderr.log")"
	printf '%s' "$out" | jq -e --arg b "$fbase" \
		'.result == "rolled-back" and .moved == false and .failed_step == "deploy" and .location == $b and .run_result == "failed" and (.commit | length) >= 7 and (.revert | length) >= 7 and
		([.steps[] | .step + ":" + .result] == ["down:done", "rename:done", "permissions:done", "commit:done", "deploy:failed"]) and
		([.rollback[] | select(.result != "done")] | length) == 0 and (.rollback | length) > 0 and (.remaining | length) == 0' >/dev/null ||
		fail "unexpected result of the failed move: $out"
	fail_commit=$(printf '%s' "$out" | jq -r .commit)
	fail_revert=$(printf '%s' "$out" | jq -r .revert)
	revert_head=$(remote_head "$move_url")
	commit_has_prefix "$revert_head" "$fail_revert" || fail "the remote head $revert_head is not the revert $fail_revert"
	commit_has_prefix "$(remote_git rev-parse "$revert_head^")" "$fail_commit" || fail "the revert $revert_head does not follow the commit $fail_commit"
	[ "$(remote_git rev-parse "$revert_head^^")" = "$fail_remote" ] || fail 'the commit of the move does not follow the former head of the remote'
	[ -z "$(remote_git ls-tree --name-only "$revert_head" movefail)" ] || fail 'the tree of the revert still holds movefail'
	[ "$(remote_git rev-parse "$revert_head^{tree}")" = "$(remote_git rev-parse "$fail_remote^{tree}")" ] || fail 'the tree of the revert is not the tree before the move'
	if docker exec "$host" test -e "$into/movefail"; then fail 'the folder is still in the source'; fi
	if docker exec "$host" test -e "$fbase/.gitignore"; then fail 'the old folder got a .gitignore'; fi
	[ "$(move_meta "$fbase")" = "$fail_meta" ] || fail "owner, group or mode of the old folder changed: $(move_meta "$fbase")"
	[ "$(tree_sums "$fbase")" = "$fail_sum" ] || fail 'the old folder changed'
	[ "$(docker exec "$host" stat -c %i "$fbase/data/records.txt")" = "$fail_inode" ] || fail 'the data file is not the same file'
	wait_for 'the project in its old folder' 60 stack_healthy movefail
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=movefail --format '{{.Label "com.docker.compose.project.working_dir"}}' | sort -u)" = "$fbase" ] ||
		fail 'the containers do not run in the old folder'
	[ "$(stack_images movefail)" = "$fimage:local-only" ] || fail "movefail does not run its image: $(stack_images movefail)"
	[ "$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' movefail_state)" = "$fail_volume" ] || fail 'the named volume is not the same'
	wait_for 'the source at the revert' 120 source_head_is "$into" "$(remote_git rev-parse "$revert_head")"
	[ "$(folder_git "$into" status --porcelain)" = "$source_status" ] || fail "the failed move left changes in the source: $(folder_git "$into" status --porcelain)"
	[ "$(qd --output json source changes "$into" | jq -c .changes)" = "$changes_before" ] || fail "source changes lists the failed move: $(qd --output json source changes "$into")"

	docker exec "$host" docker compose -p movefail down -v >/dev/null 2>&1 || fail 'cannot remove the containers of movefail'
	docker exec "$host" docker rmi "$fimage:local-only" >/dev/null 2>&1 || true
	docker exec "$host" rm -rf /opt/qd-move
	log 'scenario: source move previewed, moved a running project into a source with a remote and took a failed move back, and the source followed the remote'
}

# scenario_tui_move proves the move of a foreign Compose project into a source that exists, in the Containers area of
# the interface started under sudo, so the command runs directly as root. A person runs the project tuimove from
# /opt/qd-tmove/tuimove, outside every source, with a data folder and a file in a named volume. The takeover offers the
# source of its own and the sources of the engine that are no root stack; choosing /opt/stacks shows the preview with
# the target folder, the files and the downtime warning, and nothing changes before yes. After yes the folder lies in
# the source, the commit is in the remote, the project is a stack of the source and runs from the new folder on the same
# data folder and the same named volume with its file. The project is removed from the source again at the end.
scenario_tui_move() {
	tbase=/opt/qd-tmove/tuimove
	tinto=/opt/stacks
	tmove_url=git.$domain:stacks.git

	step 'a person starts a foreign Compose project with a data folder and a named volume outside every source'
	docker exec "$host" rm -rf /opt/qd-tmove || fail 'cannot clear /opt/qd-tmove'
	move_project "$tbase" 1.0.0
	docker exec "$host" docker run --rm --network none -v tuimove_state:/state "$backup_image" sh -ec "printf 'invented state\n' >/state/note.txt" ||
		fail 'cannot write a file into the named volume'
	tids_before=$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=tuimove | sort)
	[ -n "$tids_before" ] || fail 'the project to move has no running container'
	tsums=$(tree_sums "$tbase")
	tdata_inode=$(docker exec "$host" stat -c %i "$tbase/data/records.txt")
	tvolume_created=$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' tuimove_state)
	tsource_status=$(folder_git "$tinto" status --porcelain)
	tremote_before=$(remote_head "$tmove_url")
	[ -n "$tremote_before" ] || fail 'the remote of the source has no head'

	step 'the takeover offers a source of its own and the source to move into; the preview changes nothing'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 5
	tui_wait 'foreign project tuimove' 30
	tui_ct_cursor_to tuimove-app-1
	tui_send n
	tui_wait 'Take over as' 30
	tui_wait 'a source of its own' 10
	tui_send Enter
	tui_wait "moved into the source $tinto" 10
	tui_walk_down "the source $tinto" grep -Eq "[│║] > *moved into the source $tinto +[│║]"
	tui_send Enter
	tui_send F2
	tui_wait "Move the project tuimove into the source $tinto as the folder $tinto/tuimove?" 60
	tui_wait "target folder: $tinto/tuimove" 10
	tui_wait 'compose.yaml' 10
	tui_wait 'data: bind-mount' 10
	tui_wait 'downtime:' 10
	if tui_has '$ sudo qhost'; then fail "the question shows a sudo command although the interface runs as root: $(tui_screen)"; fi
	[ "$(docker exec "$host" docker ps -q --no-trunc --filter label=com.docker.compose.project=tuimove | sort)" = "$tids_before" ] ||
		fail 'the question changed the containers'
	[ "$(tree_sums "$tbase")" = "$tsums" ] || fail 'the question changed the project folder'
	if docker exec "$host" test -e "$tinto/tuimove"; then fail 'the question made the target folder'; fi
	[ "$(remote_head "$tmove_url")" = "$tremote_before" ] || fail 'the question changed the remote'

	step 'yes moves the project into the source and it runs there on its data'
	tui_send y
	# The dialog wraps the sentence, so its parts are matched apart.
	tui_wait "The project tuimove is moved to $tinto/tuimove" 180
	tui_wait 'deployed there.' 10
	tui_wait "the folder is now in: $tinto/tuimove" 10
	tui_send Enter
	tui_tmux kill-server >/dev/null 2>&1 || true
	wait_for 'the deploy service to answer' 30 qd status
	tmove_head=$(remote_head "$tmove_url")
	[ -n "$tmove_head" ] && [ "$tmove_head" != "$tremote_before" ] || fail 'the move pushed no commit'
	[ "$(remote_git ls-tree -r --name-only "$tmove_head" tuimove)" = "$(printf 'tuimove/.env\ntuimove/.gitignore\ntuimove/compose.yaml')" ] ||
		fail "the remote holds the wrong files of tuimove: $(remote_git ls-tree -r --name-only "$tmove_head" tuimove)"
	if docker exec "$host" test -e "$tbase"; then fail 'the old folder is still there'; fi
	docker exec "$host" test -f "$tinto/tuimove/compose.yaml" || fail 'the folder is not in the source'
	wait_for 'the source at the commit of the move' 60 source_head_is "$tinto" "$tmove_head"
	qd --output json stack list | jq -e '[.[] | select(.stack == "tuimove")] | length == 1' >/dev/null ||
		fail "stack list does not show the stack tuimove: $(qd --output json stack list)"
	wait_for 'the moved project' 60 stack_healthy tuimove
	[ "$(docker exec "$host" docker ps --filter label=com.docker.compose.project=tuimove --format '{{.Label "com.docker.compose.project.working_dir"}}' | sort -u)" = "$tinto/tuimove" ] ||
		fail 'the containers do not run in the new folder'
	[ "$(docker exec "$host" stat -c %i "$tinto/tuimove/data/records.txt")" = "$tdata_inode" ] || fail 'the data file is not the same file'
	[ "$(docker exec "$host" cat "$tinto/tuimove/data/records.txt")" = 'invented record 1' ] || fail 'the data file changed'
	[ "$(docker exec "$host" docker volume inspect -f '{{.CreatedAt}}' tuimove_state)" = "$tvolume_created" ] || fail 'the named volume is not the same'
	[ "$(docker exec "$host" docker run --rm --network none -v tuimove_state:/state:ro "$backup_image" cat /state/note.txt)" = 'invented state' ] ||
		fail 'the file in the named volume is gone'
	[ "$(stack_field tuimove deployed)" = "$tmove_head" ] || fail "tuimove is not recorded as deployed at $tmove_head: $(stack_field tuimove deployed)"
	[ "$(folder_git "$tinto" status --porcelain)" = "$tsource_status" ] || fail "the move left changes in the source: $(folder_git "$tinto" status --porcelain)"

	step 'the moved project is removed from the source again'
	docker exec "$host" docker compose -p tuimove down -v >/dev/null 2>&1 || fail 'cannot remove the containers of tuimove'
	as_admin "cd ~/stacks && git pull -q --ff-only && git rm -rq tuimove &&
		git -c user.name='E2E Admin' -c user.email=admin.qhoste2e.test commit -q -m 'Remove the moved project' && git push -q origin main" ||
		fail 'cannot remove the moved project from the remote'
	tcleaned=$(remote_head "$tmove_url")
	restart_service
	source_follows 'the source to drop the moved project' "$tinto" "$tcleaned" 180
	# The data folder is not versioned, so it stays behind.
	docker exec "$host" rm -rf "$tinto/tuimove" /opt/qd-tmove
	[ "$(folder_git "$tinto" status --porcelain)" = "$tsource_status" ] || fail "the clean-up left changes in the source: $(folder_git "$tinto" status --porcelain)"
	log 'scenario: the interface moved a foreign project into a source that exists, and it runs there on its data'
}

# scenario_tui_remote proves the remote of a local stack folder in the Stacks area of the interface. A local folder
# below /srv/qd-tr-a has no deploy key: the interface leads to the key view, the generated key is entered at the git
# server, and the folder then pushes its history to an empty repository after the confirmation. A local folder below
# /srv/qd-tr-b is compared with the repository of /opt/stacks3, which holds other files: the paths are listed and the
# answer to cancel leaves the folder, the service and the repository as they were.
scenario_tui_remote() {
	tui_log=$work/tui-remote-screens.log
	: >"$tui_log"
	tr_a=/srv/qd-tr-a
	tr_b=/srv/qd-tr-b
	tr_url6=git.$domain:stacks6.git
	tr_url3=git.$domain:stacks3.git

	step 'two local folders without a remote are bound'
	for tr_dir in "$tr_a" "$tr_b"; do
		docker exec "$host" sh -ec "rm -rf '$tr_dir' && mkdir -p '$tr_dir/notes' && chmod 0755 '$tr_dir' '$tr_dir/notes' &&
			printf 'notes of %s\n' '$tr_dir' >'$tr_dir/notes/readme.txt' && chmod 0644 '$tr_dir/notes/readme.txt' &&
			cd '$tr_dir' && git init -q -b main && git add -A &&
			git -c user.name='E2E Root' -c user.email=root.qhoste2e.test commit -q -m 'Add the notes'" ||
			fail "cannot create the local folder $tr_dir"
		out=$(qd --output json source add "$tr_dir" --yes) || fail "source add of $tr_dir failed: $out"
		printf '%s\n' "$out" | tail -n 1 | jq -e '.kind == "local"' >/dev/null || fail "unexpected source add result: $out"
		wait_for 'the deploy service to answer' 30 qd status
	done
	repo_head3=$(remote_head "$tr_url3")
	[ -n "$repo_head3" ] || fail 'stacks3 has no head'
	[ -z "$(remote_head "$tr_url6")" ] || fail 'stacks6 is not empty'
	tr_head_a=$(folder_git "$tr_a" rev-parse HEAD)

	step 'the interface leads to the key view for a folder without a deploy key and compares nothing before it exists'
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_send 1
	tui_folder_to "$tr_a"
	tui_wait 'add remote' 10
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_has "$(git_ed25519_fingerprint)" || fail "the host key question does not show the fingerprint: $(tui_screen)"
	tui_send y
	tui_wait 'has no deploy key yet' 30
	tui_send Enter
	tui_wait "stack folder: $tr_a" 10
	tui_key_generate
	tr_out=$(qd --output json key show --dir "$tr_a") || fail "key show --dir $tr_a failed: $tr_out"
	tr_pub=$(key_pub_field "$tr_out")
	case $tr_pub in ssh-ed25519\ *) ;; *) fail "no public key of $tr_a: $tr_out" ;; esac
	[ -z "$(remote_head "$tr_url6")" ] || fail 'the interface pushed before the question'
	[ -z "$(folder_git "$tr_a" remote)" ] || fail 'the interface gave the folder a remote before the question'
	printf '%s\n' "$tr_pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'

	step 'with the key in place the empty repository is compared, n changes nothing and y pushes'
	tui_send Escape
	tui_folder_to "$tr_a"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Push the history of' 30
	tui_has "$tr_a" || fail "the question does not name the folder: $(tui_screen)"
	tui_has 'branch: main' || fail "the comparison does not show the branch: $(tui_screen)"
	tui_send n
	sleep 1
	[ -z "$(remote_head "$tr_url6")" ] || fail 'the answer n pushed'
	[ -z "$(folder_git "$tr_a" remote)" ] || fail 'the answer n gave the folder a remote'
	tui_folder_to "$tr_a"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url6"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Push the history of' 30
	tui_send y
	tui_wait 'runs against' 60
	tui_send Enter
	[ "$(remote_head "$tr_url6")" = "$tr_head_a" ] || fail "the repository does not hold the head of the folder: $(remote_head "$tr_url6")"
	[ "$(qd --output json source list | jq -r --arg p "$tr_a" '.sources[] | select(.path == $p) | .kind')" = remote ] ||
		fail "the folder is not a remote source: $(qd --output json source list)"
	[ "$(folder_git "$tr_a" rev-parse HEAD)" = "$tr_head_a" ] || fail 'the push moved the head of the folder'

	step 'a repository with other files is listed and cancel changes nothing'
	register_folder_key "$tr_b"
	tui_folder_to "$tr_b"
	tui_send O
	tui_wait 'Add a remote' 10
	tui_type "$tr_url3"
	tui_send F2
	tui_wait 'Trust this host key for' 30
	tui_send y
	tui_wait 'Put the state of the host' 30
	tui_has 'Paths that differ' || fail "the comparison does not list the paths: $(tui_screen)"
	tui_has "$tr_b" || fail "the question does not name the folder: $(tui_screen)"
	tui_send Enter
	sleep 1
	[ "$(remote_head "$tr_url3")" = "$repo_head3" ] || fail 'the answer cancel changed the repository'
	[ -z "$(folder_git "$tr_b" remote)" ] || fail 'the answer cancel gave the folder a remote'
	[ "$(qd --output json source list | jq -r --arg p "$tr_b" '.sources[] | select(.path == $p) | .kind')" = local ] ||
		fail 'the answer cancel changed the kind of the source'
	tui_send Escape
	tui_send q
	tui_wait 'qhost-exit=0' 10
	tui_tmux kill-server >/dev/null 2>&1 || true

	step 'the folders are released and removed'
	for tr_dir in "$tr_a" "$tr_b"; do
		out=$(qd --output json source remove "$tr_dir" --yes) || fail "source remove of $tr_dir failed: $out"
	done
	docker exec "$host" rm -rf "$tr_a" "$tr_b"
	wait_for 'the deploy service to answer' 30 qd status
	log 'scenario: the interface pushed a local folder to an empty remote and left a differing remote alone without the confirmation'
}
