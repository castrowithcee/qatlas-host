#!/bin/sh
# Section uninstall: the deploy service is put back and sudo qhost uninstall removes both services, the webhook
# receiver and the rootless instance: the containers of the stacks keep running, the stack folders stay unchanged,
# the units, the instance, its template and the program are gone, an account qhost-deploy of an earlier setup is
# removed, and the state, the other users and the group stay; sudo
# qhost uninstall --purge then removes the state folders, the state and configuration of the instance, the users and
# the groups. The test user, its home, its rootless engine and its container stay. Nothing runs after it.
# Needs selfupdate: the deploy unit it put aside and the webhook receiver. Needs rootless: the drop-in of the
# instance with its stack folder.
# Sourced by run.sh; defines only functions and constants.

# stack_folder_listing prints names, types, modes and numeric owners of everything in the stack folders named in
# $stack_folders, one line each, sorted. The .git folders stay out, because the service writes them on every poll;
# owners stay numeric so that deleting a user changes nothing in the listing.
stack_folder_listing() {
	docker exec "$host" sh -ec '
		for d in "$@"; do
			find "$d" -xdev -name .git -prune -o -printf "%p %y %m %U %G\n"
		done | sort' sh $stack_folders
}

# all_running_containers prints the names of all running containers of the host, sorted.
all_running_containers() {
	docker exec "$host" docker ps --format '{{.Names}}' | sort
}

# rootless_kept succeeds when the test user, its home with the probe file $un_probe, its rootless engine and the
# container $un_ct in it are as they were before the uninstall.
rootless_kept() {
	docker exec "$host" id "$rootless_user" >/dev/null 2>&1 &&
		[ "$(as_rootless "sha256sum $un_probe")" = "$un_probe_sum" ] &&
		[ "$(as_rootless 'systemctl --user is-active docker.service')" = active ] &&
		[ "$(as_rootless "docker inspect -f '{{.State.Running}}' $un_ct")" = true ]
}

# scenario_uninstall puts the deploy service back and proves sudo qhost uninstall: it needs root and --confirm, stops
# both services and the rootless instance and deletes the program, the units, the drop-in of the deploy unit, its
# tmpfiles rule and channel, those of the instance, the template and the manpages, and leaves the state, the journal,
# the state and configuration of the instance, the users, the group, the containers and the stack folders as they were.
# The program is copied away first, because uninstall deletes it; the copy then runs --purge, which deletes the three
# state folders, the state and configuration of the instance, the users and the group. The test user of the instance,
# its home, its rootless engine and its container stay. Nothing runs after it, and no later step may call qhost.
scenario_uninstall() {
	un_unit=$instance_service@$rootless_user.service
	un_template=/usr/local/lib/systemd/system/$release_template
	un_rule=/etc/tmpfiles.d/$instance_service-$rootless_user.conf
	un_channel=/run/$instance_service-$rootless_user
	un_state=/var/lib/$instance_service-$rootless_user
	un_config=/etc/$instance_service-$rootless_user
	un_dropin=/etc/systemd/system/$un_unit.d/10-stack-folders.conf
	un_ct=$prefix-uninstall-rootless-probe
	un_probe=/home/$rootless_user/stacks/probe/compose.yaml
	step 'the deploy service is put back and both services run, with a container beside them'
	docker exec "$host" sh -ec '
		mv /root/qhost-deploy.service.aside /usr/local/lib/systemd/system/qhost-deploy.service
		systemctl daemon-reload
		systemctl enable --now qhost qhost-deploy
		cp /usr/local/bin/qhost /root/qhost.copy
		groupadd --system qhost-deploy
		useradd --system --no-create-home --shell /usr/sbin/nologin --gid qhost-deploy qhost-deploy' || fail 'cannot put the deploy service back'
	wait_for 'the services to answer' 30 service_serving
	image=registry.$domain/demo/app:1.0.0
	docker exec "$host" docker rm -f "$prefix-uninstall-probe" >/dev/null 2>&1 || true
	docker exec "$host" docker run -d --name "$prefix-uninstall-probe" "$image" >/dev/null || fail 'cannot start the probe container'
	containers=$(all_running_containers)
	[ -n "$containers" ] || fail 'no container runs before the uninstall'
	[ "$(docker exec "$host" systemctl is-active "$un_unit")" = active ] || fail "the instance $un_unit does not run before the uninstall"
	for p in "$un_template" "$un_rule" "$un_channel" "$un_state" "$un_config" "$un_dropin" /usr/local/lib/systemd/system/qhost-hook.service \
		/etc/tmpfiles.d/qhost-hook.conf /run/qhost-hook /etc/qhost-hook /etc/qhost-deploy/window.json; do
		docker exec "$host" test -e "$p" || fail "$p is missing before the uninstall"
	done
	as_rootless "docker rm -f $un_ct >/dev/null 2>&1; docker run -d --quiet --name $un_ct demo/app:1.0.0" >/dev/null ||
		fail 'cannot start the probe container in the rootless engine'
	as_rootless "umask 022; mkdir -p ${un_probe%/*} && printf 'services: {}\n' >$un_probe" || fail "cannot write $un_probe"
	un_probe_sum=$(as_rootless "sha256sum $un_probe")
	rootless_kept || fail "the test user, its home, its engine or its container are not ready before the uninstall"
	# The folders are read from the drop-in now, because the uninstall deletes it.
	stack_folders=$(docker exec "$host" sed -n 's/^ReadWritePaths=-//p' /etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf)
	folders=$(stack_folder_listing)
	[ -n "$folders" ] || fail 'the drop-in of the deploy unit lists no stack folder'
	printf '%s\n' "$folders" | grep -q '^/opt/stacks ' || fail "the stack folders do not include /opt/stacks: $folders"

	step 'uninstall without root, without --confirm and without an answer changes nothing'
	before=$(installed_sum)
	if out=$(as_admin 'qhost uninstall --yes --confirm 2>&1'); then fail "uninstall without root succeeded: $out"; fi
	if out=$(as_admin 'sudo qhost uninstall --yes 2>&1'); then fail "uninstall without --confirm succeeded: $out"; fi
	printf '%s' "$out" | grep -q 'Remove qhost' || fail "uninstall without --confirm does not show its plan: $out"
	printf '%s' "$out" | grep -q 'rm /usr/local/bin/qhost' || fail "uninstall without --confirm does not show its commands: $out"
	for want in 'systemctl disable --now qhost-hook' 'rm /etc/tmpfiles.d/qhost-hook.conf' 'rm -r /run/qhost-hook'; do
		printf '%s' "$out" | grep -qF -- "$want" || fail "uninstall without --confirm does not show '$want': $out"
	done
	if out=$(as_admin 'sudo qhost uninstall --confirm 2>&1 </dev/null'); then fail "uninstall without an answer succeeded: $out"; fi
	[ "$(installed_sum)" = "$before" ] || fail 'a refused uninstall changed the installation'
	service_serving || fail 'the services do not answer after a refused uninstall'
	[ "$(docker exec "$host" systemctl is-active "$un_unit")" = active ] || fail "a refused uninstall stopped $un_unit"
	docker exec "$host" test -f "$un_template" || fail "a refused uninstall deleted $un_template"

	step 'sudo qhost uninstall removes both services and the instance and keeps state, users, containers and stack folders'
	out=$(as_admin 'sudo qhost uninstall --yes --confirm 2>&1') || fail "uninstall failed: $out"
	for want in "systemctl disable --now $un_unit" "rm $un_rule" "rm $un_template"; do
		printf '%s' "$out" | grep -qF -- "$want" || fail "the steps of the uninstall do not show '$want': $out"
	done
	for p in "$un_template" "$un_rule" "$un_channel" "$un_dropin" "${un_dropin%/*}"; do
		if docker exec "$host" test -e "$p" || docker exec "$host" test -L "$p"; then fail "uninstall left $p: $out"; fi
	done
	[ "$(docker exec "$host" systemctl is-active "$un_unit" || true)" != active ] || fail "$un_unit still runs after the uninstall"
	if docker exec "$host" systemctl is-enabled --quiet "$un_unit"; then fail "$un_unit is still enabled after the uninstall"; fi
	if docker exec "$host" systemctl cat "$un_unit" >/dev/null 2>&1; then fail "systemd still knows the unit $un_unit after the uninstall"; fi
	for p in "$un_state" "$un_config"; do
		docker exec "$host" test -d "$p" || fail "uninstall without --purge removed $p"
	done
	rootless_kept || fail "the uninstall changed the test user, its home, its rootless engine or its container"
	for p in /usr/local/bin/qhost /usr/local/lib/systemd/system/qhost.service /usr/local/lib/systemd/system/qhost-deploy.service \
		/etc/tmpfiles.d/qhost-deploy.conf /run/qhost-deploy /usr/local/share/man/man1/qhost.1 \
		/etc/systemd/system/qhost-deploy.service.d/10-stack-folders.conf /usr/local/lib/systemd/system/qhost-hook.service \
		/etc/tmpfiles.d/qhost-hook.conf /run/qhost-hook; do
		if docker exec "$host" test -e "$p" || docker exec "$host" test -L "$p"; then fail "uninstall left $p: $out"; fi
	done
	docker exec "$host" test -f /etc/systemd/system/qhost-deploy.service.d/e2e.conf || fail 'uninstall removed a drop-in it does not own'
	for u in qhost qhost-deploy qhost-hook; do
		[ "$(docker exec "$host" systemctl is-active "$u" || true)" != active ] || fail "$u still runs after the uninstall"
		if docker exec "$host" systemctl is-enabled --quiet "$u"; then fail "$u is still enabled after the uninstall"; fi
		if docker exec "$host" systemctl cat "$u" >/dev/null 2>&1; then fail "systemd still knows the unit $u after the uninstall"; fi
	done
	[ "$(all_running_containers)" = "$containers" ] || fail "the containers changed: $(all_running_containers)"
	[ "$(stack_folder_listing)" = "$folders" ] || fail 'a stack folder changed'
	for p in /var/lib/qhost /var/lib/qhost-deploy /var/lib/qhost-host /etc/qhost-hook; do
		docker exec "$host" test -d "$p" || fail "uninstall without --purge removed $p"
	done
	docker exec "$host" test -f /etc/qhost-deploy/window.json || fail 'uninstall without --purge removed the host window file'
	docker exec "$host" sh -ec 'id qhost && id qhost-hook && getent group qhost && getent group qhost-hook' >/dev/null ||
		fail 'uninstall without --purge removed a user or a group'
	if docker exec "$host" id qhost-deploy >/dev/null 2>&1 || docker exec "$host" getent group qhost-deploy >/dev/null 2>&1; then
		fail 'uninstall left the user or the group qhost-deploy of an earlier setup'
	fi
	log "scenario: uninstalled both services, the containers and stack folders are unchanged: $out"

	step 'sudo qhost uninstall --purge removes the three state folders, the configuration of the deploy services and of the receiver, the state and configuration of the instance, the users and the group'
	out=$(as_admin 'sudo /root/qhost.copy uninstall --purge --yes --confirm 2>&1') || fail "uninstall --purge failed: $out"
	for p in /var/lib/qhost /var/lib/qhost-deploy /etc/qhost-deploy /var/lib/qhost-host /etc/qhost-hook "$un_state" "$un_config"; do
		if docker exec "$host" test -e "$p"; then fail "uninstall --purge left $p: $out"; fi
	done
	for u in qhost qhost-deploy qhost-hook; do
		if docker exec "$host" id "$u" >/dev/null 2>&1; then fail "uninstall --purge left the user $u: $out"; fi
		if docker exec "$host" getent group "$u" >/dev/null 2>&1; then fail "uninstall --purge left the group $u: $out"; fi
	done
	[ "$(all_running_containers)" = "$containers" ] || fail "the purge changed the containers: $(all_running_containers)"
	[ "$(stack_folder_listing)" = "$folders" ] || fail 'the purge changed a stack folder'
	rootless_kept || fail "the purge changed the test user, its home, its rootless engine or its container"
	docker exec "$host" docker rm -f "$prefix-uninstall-probe" >/dev/null 2>&1 || true
	as_rootless "docker rm -f $un_ct" >/dev/null 2>&1 || true
	log "scenario: purged the state, the users and the group, the containers and stack folders are unchanged: $out"
}
