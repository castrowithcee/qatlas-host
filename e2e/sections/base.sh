#!/bin/sh
# Section base: from the simulated host, the demo stacks repository is cloned over SSH and a commit is pushed; the
# demo image tags are checked and a demo container turns unhealthy on purpose. The rootless Docker engine of a test
# user runs without a login session, starts a container from the registry mirror under a sub-UID of the user, and the
# system engine does not see that container. The Docker integration tests of the deployer run inside the host.
# Needs no other section.
# Sets repo, the address of the stacks repository, and leaves the admin clone ~/stacks with demo-stack on 1.0.1.
# Sourced by run.sh; defines only functions and constants.

# scenario_environment checks what run.sh up built: the stacks repository over SSH, the registry and the demo
# image. It sets repo and pushes the commit that puts demo-stack on 1.0.1.
scenario_environment() {
	repo=git.$domain:stacks.git

	step 'clone over SSH'
	as_admin "rm -rf ~/stacks && git clone -q $repo ~/stacks"
	as_admin 'cd ~/stacks && test -f demo-stack/compose.yaml && test -f demo-stack/.env && grep -qx "secrets.env" demo-stack/.gitignore && grep -qx "data/" demo-stack/.gitignore' ||
		fail 'the demo repository does not have the expected stack files'
	as_admin 'cd ~/stacks && grep -q "^DEMO_APP_VERSION=" demo-stack/.env && ! git ls-files | grep -q secrets.env' ||
		fail 'the demo repository does not hold the image version as a variable or tracks a secrets file'

	step 'commit and push'
	as_admin 'cd ~/stacks &&
		sed -i "s/^DEMO_APP_VERSION=.*/DEMO_APP_VERSION=1.0.1/" demo-stack/.env &&
		git -c user.name="E2E Admin" -c user.email=admin.qhoste2e.test commit -q -am "Update demo-stack to 1.0.1" &&
		git push -q origin main'
	local_head=$(as_admin 'cd ~/stacks && git rev-parse HEAD')
	remote_head=$(as_admin "git ls-remote $repo refs/heads/main" | cut -f 1)
	[ "$local_head" = "$remote_head" ] || fail "the pushed commit is not on the server ($local_head vs $remote_head)"
	as_admin "rm -rf ~/stacks2 && git clone -q $repo ~/stacks2 && grep -qx DEMO_APP_VERSION=1.0.1 ~/stacks2/demo-stack/.env" ||
		fail 'a fresh clone does not see the pushed commit'

	step 'the git server offers no shell'
	if as_admin "ssh -o BatchMode=yes git.$domain id" >/dev/null 2>&1; then
		fail 'the git server ran an arbitrary command'
	fi

	step 'registry tags'
	got=$(as_admin "curl -fsS https://registry.$domain/v2/demo/app/tags/list")
	log "scenario: tags $got"
	for tag in $tags; do
		printf '%s' "$got" | grep -q "\"$tag\"" || fail "the registry does not list $tag"
	done

	step 'demo healthcheck turns unhealthy on purpose'
	image=registry.$domain/demo/app:1.0.0
	docker exec "$host" docker rm -f "$prefix-demo-ok" "$prefix-demo-bad" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$prefix-demo-ok" "$image" >/dev/null
	docker exec "$host" docker run -d --name "$prefix-demo-bad" -e DEMO_HEALTH=fail "$image" >/dev/null
	wait_for 'the healthy demo container' 30 health_is "$prefix-demo-ok" healthy
	wait_for 'the unhealthy demo container' 30 health_is "$prefix-demo-bad" unhealthy
	log "scenario: $prefix-demo-ok healthy, $prefix-demo-bad unhealthy"
	docker exec "$host" docker rm -f "$prefix-demo-ok" "$prefix-demo-bad" >/dev/null
}

rootless_health_is() {
	[ "$(as_rootless "docker inspect -f '{{.State.Health.Status}}' $1")" = "$2" ]
}

# scenario_rootless_engine checks the rootless Docker engine of $rootless_user that run.sh up started.
scenario_rootless_engine() {
	step "the rootless Docker engine of $rootless_user runs with lingering and without a login session"
	groups=$(docker exec "$host" id -nG "$rootless_user")
	case " $groups " in *" sudo "* | *" docker "* | *" qhost "*) fail "$rootless_user is in a privileged group: $groups" ;; esac
	[ "$(docker exec "$host" loginctl show-user "$rootless_user" -p Linger --value)" = yes ] ||
		fail "$rootless_user has no lingering"
	[ -z "$(docker exec "$host" loginctl show-user "$rootless_user" -p Sessions --value)" ] ||
		fail "$rootless_user has a login session"
	[ "$(as_rootless 'systemctl --user is-active docker.service')" = active ] ||
		fail "the user unit docker.service of $rootless_user is not active"
	as_rootless "docker info -f '{{.SecurityOptions}}'" | grep -q 'name=rootless' ||
		fail "the engine on /run/user/$rootless_uid/docker.sock is not rootless"

	step 'a container of the rootless engine comes from the registry mirror and runs under a sub-UID'
	rootless_ct=$prefix-rootless-demo
	# A Docker Hub name: only the registry mirror resolves it.
	as_rootless "docker run -d --quiet --name $rootless_ct --user 1234 demo/app:1.0.0" >/dev/null ||
		fail 'the rootless engine cannot start demo/app:1.0.0 from the registry mirror'
	wait_for 'the healthy demo container of the rootless engine' 30 rootless_health_is "$rootless_ct" healthy
	pid=$(as_rootless "docker inspect -f '{{.State.Pid}}' $rootless_ct")
	docker exec "$host" grep -q demoapp "/proc/$pid/cmdline" || fail "process $pid is not the demo app of $rootless_ct"
	sub_start=$(docker exec "$host" awk -F: -v u="$rootless_user" '$1 == u { print $2 }' /etc/subuid)
	got=$(docker exec "$host" stat -c %u "/proc/$pid")
	# The container's user 1 is the first sub-UID.
	[ "$got" -eq $((sub_start + 1233)) ] || fail "the process of $rootless_ct runs as $got, not as the sub-UID $((sub_start + 1233))"
	log "scenario: $rootless_ct runs as uid $got on the host"

	step 'the system engine does not see the container of the rootless engine'
	[ -z "$(docker exec "$host" docker ps -aq --filter "name=$rootless_ct")" ] ||
		fail "the system engine shows $rootless_ct"
	as_rootless "docker rm -f $rootless_ct" >/dev/null
}

# scenario_compose_integration runs the Docker integration tests of the deployer inside the host. The test
# binary is built here and refuses to run without the marker of the simulated host.
scenario_compose_integration() {
	step 'Docker integration tests of the deployer run on the host'
	(cd "$root" && GOOS=linux GOARCH="$(docker exec "$host" dpkg --print-architecture)" CGO_ENABLED=0 \
		go test -c -tags dockerintegration -o "$work/compose.test" ./internal/compose) || fail 'cannot build the integration tests'
	docker cp "$work/compose.test" "$host:/tmp/compose.test"
	out=$(docker exec -e QHOST_E2E_HOST=1 -e "QHOST_E2E_IMAGE=registry.$domain/demo/app:1.0.0" "$host" \
		/tmp/compose.test -test.v -test.count=1 -test.run '^TestDocker' 2>&1 </dev/null) || {
		printf '%s\n' "$out"
		docker exec "$host" rm -f /tmp/compose.test
		fail 'the Docker integration tests failed'
	}
	docker exec "$host" rm -f /tmp/compose.test
	for name in TestDockerHealthyStackIsDeployedAndChecked TestDockerUnhealthyStackFailsTheCheck TestDockerDeployWithoutSecretsFile; do
		printf '%s' "$out" | grep -q -- "--- PASS: $name" || fail "$name did not pass: $out"
	done
	if printf '%s' "$out" | grep -q -- '--- SKIP'; then fail "an integration test was skipped: $out"; fi
	log "scenario: Docker integration tests passed: $(printf '%s' "$out" | grep -E '^(--- |ok|PASS)' | tr '\n' ' ')"
}
