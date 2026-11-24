#!/bin/sh
# Helpers of the e2e scenario for stack sources: the git repositories and folders behind them.
# Sourced by run.sh; defines only functions and constants.

# files_git runs git on /srv/qd-files/web as root; the folder is marked safe because the service owns .git; stderr is merged.
files_git() {
	docker exec "$host" git -c safe.directory=/srv/qd-files/web -C /srv/qd-files/web "$@" 2>&1
}

# register_folder_key creates the deploy key of the folder $1 and enters its public part at the git server.
register_folder_key() {
	out=$(qd --output json key generate --dir "$1") || fail "key generate --dir $1 failed: $out"
	pub=$(printf '%s\n' "$out" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p')
	case $pub in ssh-ed25519\ *) ;; *) fail "no public key in: $out" ;; esac
	printf '%s\n' "$pub" | docker exec -i "$prefix-git" sh -c 'cat >>/srv/git/.ssh/authorized_keys'
}

# remote_head prints the commit of refs/heads/main at the git server for the repository address $1 (empty without).
remote_head() {
	as_admin "git ls-remote $1 refs/heads/main" | cut -f 1
}

# folder_git runs git on the folder $1 as root, marked safe because the service owns .git; stderr is merged.
folder_git() {
	dir=$1
	shift
	docker exec "$host" git -c "safe.directory=$dir" -C "$dir" "$@" 2>&1
}

# key_pub_field prints the public part (type and key, without comment) of the qhost key output $1.
key_pub_field() {
	printf '%s\n' "$1" | sed -n 's/.*"public_key": *"\([^"]*\)".*/\1/p' | cut -d ' ' -f 1,2
}
