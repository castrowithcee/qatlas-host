package helptopics

import "github.com/castrowithcee/qatlas-kit/helptopics"

var promoteTopics = []helptopics.Topic{
	{Name: "promote", Short: "Taking over the image versions of another stack: qhost stack promote, its rules, the preview and the terminal interface", Text: promoteTopic},
}

const promoteTopic = `qhost stack promote <target> or qhost stack promote <source> <target> sets the image versions of the target stack to the versions the source stack runs, as one secured run of the target. It saves the manual way, reading the version in the source and running qhost stack update <target> --version <v> for each image, for a team that runs staging and production as separate stacks. It needs sudo and the service must run.

Stored source:
- The setting update.promote_source of the target (qhost stack settings set, or the update rules form in the terminal interface) names the stack its versions come from, such as production from staging. With it, qhost stack promote <target> needs no source, and the terminal interface preselects it for the key M. Naming a source on the command line overrules the stored one. The stack must be another stack of the host; it need not exist when the setting is saved.
- The setting restricts nothing and follows nothing: qhost stack update, the mode and the trigger of the target work as without it, and a target in automatic mode still updates from the registry. The source serves only the promotion by hand, the display and the notice below.
- qhost stack list shows, in the columns promote_source and promote_versions, the versions the source runs beside the stack's own, taken from the commit the source deployed last. When the source is not proven (rules at the source below) or is unreadable, the column says so instead of a version; a source no stack of the host has is shown as missing. The terminal interface shows the same in a Source column of the Stacks area and in the version section of the stack detail.
- A target in manual mode gets a notice, as for an available update, when the source runs proven a version that is higher than the target's and that a promotion would set, once per version of the source. The service reads the source through its own deploy instance only: a source in another engine's deploy instance gives no notice, but the display and qhost stack promote still work across engines.

What is taken:
- The versions come from the .env of the commit the source deployed last, not from its working copy.
- Images are matched by their repository, not by the name of the variable. Every image version variable of the target whose image the source also runs gets the version of the source. An image only the target runs stays as it is, an image only the source runs is ignored. When the match is ambiguous, because a repository appears in several variables of one stack, the command refuses and names the images.
- Source and target must be on the same host. They may be in different stack folders and in different engines; --engine names the engine of the target and --source-engine that of the source, each needed only when several engines have a stack of that name. Taking over between hosts stays qhost stack update --version.

Rules at the target, the same as for qhost stack update <target> --version:
- A stack without settings, never deployed, with local changes, paused, stopped, pinned or with a run in progress is refused.
- Each version must be in the tag list of the registry and of the same variant as the current one.
- The same version is nothing to do: the command succeeds and says so, and no run is made.
- A lower version is refused, with the hint to use qhost stack rollback to go back to the state before an update run.

Rules at the source:
- The source is refused when its last run did not succeed, when it has a commit its last run failed at, or when it is not healthy: a failure, a halted repair, paused or stopped.
- An observation time that still runs after the source's update does not stop the promotion. The preview names it, because the update of the source is not settled yet.

Preview and run:
- Without --confirm the command shows, per variable of the target, the version it has and the one it gets, the source with its commit and, where it runs, its observation time. It changes nothing.
- The preview names the full source commit, in --output json in the column source_commit, and the command to confirm it.
- With --confirm --source-commit <commit> the command runs with exactly that commit, the one of the preview. When the source has deployed another commit since, it refuses and a new preview is needed. Without --source-commit, --confirm reads the source commit when it runs and binds the run to that.
- The run has the trigger promote in qhost stack runs. Like an update it stops the stack, backs up its data if it has backup rules, commits all new versions to .env in one commit whose message names the source and the versions, pushes it, then pulls, starts and checks the stack. A failing run is rolled back; qhost stack rollback goes back afterwards. The command exits with 1 when the run did not succeed.

In the terminal interface the stack detail has the key M. It asks for the source among the other stacks of the host, shows the same preview with the versions, the source and its observation time, and asks for the confirmation before the run. Without sudo the interface shows the command sudo qhost stack promote and runs it after y; that command carries --source-commit with the commit of the preview.`
