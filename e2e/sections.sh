#!/bin/sh
# Register of the e2e scenario. The scenario runs on the host of run.sh up, in sections; each section is a file in
# sections/ whose head says what it proves, which sections it needs and why, and which values it sets for others.
# A unit is a scenario function of a section. A run with section names runs the units of those sections and of all
# they need, transitively, in the order of scenario_register; without names it runs every unit, the full run.
# Sourced by run.sh, also before up; defines only functions and constants.

# The units in the order of the full run, each after its section.
scenario_register='
base           scenario_environment
base           scenario_rootless_engine
base           scenario_compose_integration
install        setup_service
install        scenario_host_service
install        scenario_host_changes
host-tui       scenario_tui_system
host-tui       scenario_tui_changes
host-tui       scenario_tui_files
host-tui       scenario_tui_host_proposal
host-tui       scenario_free_command_proposal
instance       scenario_rootless_instance
instance       scenario_service_log
deploy         bind_source
deploy         deploy_baseline
deploy         scenario_deploy
deploy         scenario_containers
deploy         scenario_container_logs
deploy         scenario_read_access
deploy         scenario_proposals
deploy         scenario_container_actions
deploy         scenario_stack_actions
heal           scenario_repair_start
heal           scenario_repair_attempt
heal           scenario_container_monitor
heal           scenario_repair_second
access         scenario_deploy_audit
access         scenario_grants
access         scenario_grant_proposal
tui-sudo       scenario_tui_sudo
tui-sudo       scenario_tui_deploy_sudo
tui-sudo       scenario_datadir_nested
tui-sudo       scenario_tui_secrets
tui-sudo       scenario_tui_folder_sudo
update         scenario_backup
update         scenario_updates
update         scenario_rollback
update         scenario_diagnose
update         scenario_update
window         scenario_window
window         scenario_window_instances
window         scenario_tui
host-key       scenario_host_key
heal           scenario_observe_start
sources        scenario_sources
sources        scenario_root_source
sources        scenario_source_inspect
rootless       scenario_source_inspect_engines
rootless       scenario_source_rootless
rootless       scenario_source_engine
rootless       scenario_tui_engine
rootless       scenario_tui_containers_engine
rootless       scenario_backup_rootless
rootless       scenario_proposal_engine
rootless       scenario_tui_engine_choice
sources        scenario_key_set
heal           scenario_observe_end
heal           scenario_repair_end
heal           demo_leftovers_clean
source-local   scenario_source_local
source-local   scenario_private_registry
source-local   scenario_source_files
source-git     scenario_source_remote
source-git     scenario_source_move
source-git     scenario_tui_move
source-git     scenario_tui_remote
tui-proposals  scenario_adopt
tui-proposals  scenario_tui_proposals
tui-proposals  scenario_tui_proposal_kinds
first-setup    scenario_tui_setup
first-setup    scenario_tui_rules
first-setup    scenario_tui_backup
first-setup    scenario_tui_check
selfupdate     scenario_selfupdate
uninstall      scenario_uninstall
'

# Each section, followed by the sections it needs, in the order of their first unit.
scenario_needs='
base
install
host-tui       install
instance       install
deploy         base instance
access         deploy
tui-sudo       deploy
heal           deploy
update         deploy
window         update
host-key       deploy
sources        deploy
rootless       deploy
source-local   sources
source-git     source-local
tui-proposals  deploy
first-setup    window sources
selfupdate     deploy
uninstall      selfupdate rootless
'

# scenario_sections prints the names of all sections.
scenario_sections() {
	printf '%s\n' "$scenario_needs" | awk 'NF { print $1 }'
}

# scenario_add adds the section $1 and the sections it needs to $sc_selected.
scenario_add() {
	case " $sc_selected " in *" $1 "*) return 0 ;; esac
	sc_line=$(printf '%s\n' "$scenario_needs" | awk -v s="$1" '$1 == s')
	[ -n "$sc_line" ] || fail "unknown section $1; valid sections: $(scenario_sections | paste -sd ' ' -)"
	sc_selected="$sc_selected $1"
	set -- $sc_line
	shift
	for sc_need in "$@"; do scenario_add "$sc_need"; done
}

# scenario_select sets $sc_selected to the sections $@ and all they need; without names to every section.
scenario_select() {
	sc_selected=
	[ $# -gt 0 ] || set -- $(scenario_sections)
	for sc_name in "$@"; do scenario_add "$sc_name"; done
}

# scenario_units prints the units of the sections in $sc_selected in the order of the full run.
scenario_units() {
	printf '%s\n' "$scenario_register" | awk -v sel=" $sc_selected " 'NF && index(sel, " " $1 " ") { print $2 }'
}

# run_scenario runs the units of the sections $@ and of all they need, or every unit without names. The register is
# checked before the first step: every section of a unit is known and every unit is a function of the sections.
run_scenario() {
	for sc_section in $(printf '%s\n' "$scenario_register" | awk 'NF { print $1 }'); do
		scenario_sections | grep -qx -- "$sc_section" || fail "the register names the unknown section $sc_section"
	done
	scenario_select "$@"
	sc_units=$(scenario_units)
	for sc_unit in $sc_units; do
		case $(LC_ALL=C type "$sc_unit" 2>/dev/null) in
		*function*) ;;
		*) fail "the unit $sc_unit of the register is no function" ;;
		esac
	done
	if [ $# -gt 0 ]; then
		log "scenario: sections $(scenario_sections | awk -v s=" $sc_selected " 'index(s, " " $0 " ")' | paste -sd ' ' -)"
	fi
	for sc_unit in $sc_units; do "$sc_unit"; done
	log 'scenario: passed'
}
