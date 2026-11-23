#!/bin/sh
# Helpers of the e2e scenario for the terminal interface driven in a tmux pane.
# Sourced by run.sh; defines only functions and constants.

# The terminal interface is driven in a tmux pane of the administrator, 120 columns by 40 rows. The command
# keeps the pane after qhost ends, so the exit code can be read from the screen.
# The icons of the tab line in the default setting (unicode), which the pane's UTF-8 locale draws; an icon
# stands between the digit and the name.
tui_icon_stacks='▤'
tui_icon_detail='▣'
tui_icon_runs='▶'
tui_icon_settings='⚙'
tui_icon_containers='▦'

# tui_system_tab clicks the sub-tab $1 of the System area on the third line of the screen (header, tab line,
# sub-tab line) and leads the focus into its content.
tui_system_tab() {
	cell=$(tui_screen | awk -v t="$1" 'NR == 3 && (i = index($0, t)) { print i, NR; exit }')
	[ -n "$cell" ] || fail "cannot click the sub-tab '$1', it is not on the sub-tab line: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
	sleep 0.4
	tui_send Down
	sleep 0.3
}

tui_tmux() {
	docker exec -u "$admin" "$host" tmux -L qdtui "$@"
}

# tui_screen prints the screen and appends it to the capture log, which the first-setup section searches for
# masked values at its end.
tui_screen() {
	tui_tmux capture-pane -p -t qdtui | tee -a "${tui_log:-/dev/null}" 2>/dev/null
}

# tui_send sends Escape as its CSI u sequence: a lone ESC byte waits for a following key and merges with it into
# Alt+key, which the interface ignores, while the full sequence reads as Escape at once.
tui_send() {
	tui_send_esc=
	case "${1-}" in -*) ;; *)
		for tui_send_key in "$@"; do
			[ "$tui_send_key" != Escape ] || tui_send_esc=1
		done
	esac
	if [ -z "$tui_send_esc" ]; then
		tui_tmux send-keys -t qdtui "$@"
		return
	fi
	for tui_send_key in "$@"; do
		if [ "$tui_send_key" = Escape ]; then
			tui_tmux send-keys -t qdtui -l "$(printf '\033[27u')"
		else
			tui_tmux send-keys -t qdtui "$tui_send_key"
		fi
	done
}

tui_has() {
	tui_screen | grep -qF -- "$1"
}

# tui_keep_engine waits up to $1 seconds for the choice of the engine, which follows the inspection of a folder
# while more than one deploy service is set up, checks that system is preselected and keeps it.
tui_keep_engine() {
	i=0
	until tui_screen | grep -qE 'whose deploy service runs the stacks|preselected is the engine where containers'; do
		i=$((i + 1))
		if [ "$i" -ge $(($1 * 4)) ]; then
			printf '%s\n' "$(tui_screen)" >&2
			fail 'timeout waiting for the choice of the engine on the screen of the terminal interface'
		fi
		sleep 0.25
	done
	tui_screen | grep -qE '> Engine +│ system' || fail "the choice of the engine does not preselect system: $(tui_screen)"
	tui_send F2
}

# tui_wait waits up to $2 seconds for the text $1 on the screen and shows the screen when it does not appear.
tui_wait() {
	i=0
	until tui_has "$1"; do
		i=$((i + 1))
		if [ "$i" -ge $(($2 * 4)) ]; then
			printf '%s\n' "$(tui_screen)" >&2
			fail "timeout waiting for '$1' on the screen of the terminal interface"
		fi
		sleep 0.25
	done
}

# tui_walk_down presses Down until the command $2..., which reads the screen on its input, succeeds; $1 names the
# target in the failure. The cursor rows count with their screen lines, so rows of the same text stay apart. After
# each key it waits up to two seconds for them to change, as a pending redraw cannot be told from the end of the list
# otherwise. The end of the list ends the walk with a failure, and so do cursor rows seen twice: a list that wraps.
tui_walk_down() {
	tw_target=$1
	shift
	tw_sep=$(printf '\037')
	tw_seen=$tw_sep
	tw_screen=$(tui_screen)
	while ! printf '%s\n' "$tw_screen" | "$@"; do
		tw_cursor=$(printf '%s\n' "$tw_screen" | grep -nE '^(│ |║ )?> ')
		tui_send Down
		tw_n=0
		while :; do
			sleep 0.1
			tw_screen=$(tui_screen)
			tw_now=$(printf '%s\n' "$tw_screen" | grep -nE '^(│ |║ )?> ')
			[ "$tw_now" != "$tw_cursor" ] && break
			tw_n=$((tw_n + 1))
			[ "$tw_n" -lt 20 ] || fail "the cursor does not reach $tw_target, it stops at '${tw_cursor:-no cursor row}': $tw_screen"
		done
		case $tw_seen in
		*"$tw_sep$tw_now$tw_sep"*) fail "the cursor does not reach $tw_target, the list wraps around at '$tw_now': $tw_screen" ;;
		esac
		tw_seen=$tw_seen$tw_now$tw_sep
	done
}

# tui_cursor_to moves the cursor of the Stacks list to the stack $1. The list draws no cursor without the focus, so
# the focus must be in the list.
tui_cursor_to() {
	tui_walk_down "$1" grep -Eq "^(│ |║ )?> .*\[.\] $1 "
}

# tui_folder_to moves the cursor of the Stacks list to the group row of the stack folder $1. The mark before the path
# is a symbol in a terminal.
tui_folder_to() {
	tui_send Home
	tui_walk_down "the folder $1" grep -Eq "^(│ |║ )?> .*\[.\] [^ ]* $1 \("
}

# tui_click presses and releases the left mouse button at the first cell of the text $1 on the screen, as a
# terminal reports it (SGR, 1-based), and fails when the text is not on the screen. awk counts bytes, so the
# continuation bytes of the icons before the text are taken off; an icon is one cell wide.
tui_click() {
	cell=$(tui_screen | awk -v t="$1" 'i = index($0, t) { p = substr($0, 1, i - 1); gsub(/[\200-\277]/, "", p); print length(p) + 1, NR; exit }')
	[ -n "$cell" ] || fail "cannot click '$1', it is not on the screen: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
}

# tui_click_row clicks the stack $1 in the Stacks list and waits until the cursor marks it.
tui_click_row() {
	tui_click "$1"
	i=0
	until tui_screen | grep -q "^\(│ \|║ \)\{0,1\}> .*\[.\] $1 "; do
		i=$((i + 1))
		[ "$i" -lt 20 ] || fail "the click did not mark $1: $(tui_screen)"
		sleep 0.25
	done
}

# tui_excerpt logs the lines of the screen from the first one containing $1, at most $2 lines.
tui_excerpt() {
	tui_screen | sed -n "/$1/,\$p" | head -n "$2" | sed 's/ *$//; s/^/e2e:   | /'
}

# tui_ct_cursor_on tells whether the cursor of the Containers list is on the container $1. With engine sections
# the row sits inside the frame of its section, so it starts with that frame.
tui_ct_cursor_on() {
	tui_screen | grep -Eq "^(│ |║ )?> .*$1 "
}

# tui_ct_cursor_to moves the cursor of the Containers list to the container $1.
tui_ct_cursor_to() {
	tui_walk_down "$1" grep -Eq "^(│ |║ )?> *$1 "
}

# prop_state prints the state of the proposal $1 as the administrator without sudo reads it.
prop_state() {
	as_admin "qhost --output json proposal show $1" | sed -n 's/.*"state": *"\([a-z]*\)".*/\1/p' | head -n 1
}

# prop_is succeeds when the proposal $1 is in the state $2.
prop_is() {
	[ "$(prop_state "$1")" = "$2" ]
}

# prop_tui starts the interface under sudo in a fresh tmux session and waits for the notice of the open proposals.
prop_tui() {
	tui_tmux kill-server >/dev/null 2>&1 || true
	tui_tmux new-session -d -s qdtui -x 120 -y 40 "env LANG=C.UTF-8 sh -c 'sudo qhost; echo qhost-exit=\$?; sleep 600'" ||
		fail 'cannot start tmux for the interface'
	tui_wait "1 $tui_icon_stacks Stacks" 30
	tui_wait '1 open proposal waits for you' 30
}

# prop_submit_kind submits the JSON $2 as a proposal of the kind $1 as the administrator without sudo and prints its
# id.
prop_submit_kind() {
	out=$(printf '%s' "$2" | as_admin "qhost --output json proposal submit --kind $1" 2>&1) ||
		fail "qhost proposal submit --kind $1 without sudo failed: $out"
	printf '%s' "$out" | sed -n 's/.*"id": *"\(p-[0-9a-f]*\)".*/\1/p' | head -n 1
}

# tui_type types text into the interface.
tui_type() {
	tui_tmux send-keys -t qdtui -l -- "$1"
}

# tui_clear empties the text field that has the focus.
tui_clear() {
	tui_send BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace
	tui_send BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace BSpace
}

# tui_subtab opens the sub-tab of the Settings area that is named $1, whatever page was open, and leads the focus
# into its content. The digit shows the area with the focus in the content, so the sub-tab line is not focused and
# draws no mark before its first tab; the third line of the screen is that line (header, tab line, sub-tab line).
# The name is clicked, as the page stands where only spaces precede it.
tui_subtab() {
	tui_send 4
	sleep 0.4
	cell=$(tui_screen | awk -v t="$1" 'NR == 3 && (i = index($0, t)) { print i, NR; exit }')
	[ -n "$cell" ] || fail "cannot click the sub-tab '$1', it is not on the sub-tab line: $(tui_screen)"
	set -- $cell
	tui_send -l "$(printf '\033[<0;%d;%dM\033[<0;%d;%dm' "$1" "$2" "$1" "$2")"
	sleep 0.4
	tui_send Down
	sleep 0.3
}

# tui_key_generate runs the form of the deploy key view and chooses to generate; the result dialog is closed.
tui_key_generate() {
	tui_send e
	tui_wait 'store an existing key' 10
	tui_send Right
	sleep 0.3
	tui_send F2
	tui_wait 'The deploy key is generated' 30
	tui_send Enter
}
