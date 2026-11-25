package helptopics

import "github.com/castrowithcee/qatlas-kit/helptopics"

var promoteTopics = []helptopics.Topic{
	{Name: "promote", Short: "Taking over the image versions of another stack: qhost stack promote, its rules, the preview and the terminal interface", Text: promoteTopic},
}

const promoteTopic = `qhost stack promote <source> <target> sets the image versions of the target stack to the versions the source stack runs, as one secured run of the target. It saves the manual way, reading the version in the source and running qhost stack update <target> --version <v> for each image, for a team that runs staging and production as separate stacks. It needs sudo and the service must run.

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
- With --confirm the command runs, bound to the source commit of the preview. When the source has deployed another commit since, it refuses and a new preview is needed.
- The run has the trigger promote in qhost stack runs. Like an update it stops the stack, backs up its data if it has backup rules, commits all new versions to .env in one commit whose message names the source and the versions, pushes it, then pulls, starts and checks the stack. A failing run is rolled back; qhost stack rollback goes back afterwards. The command exits with 1 when the run did not succeed.

In the terminal interface the stack detail has the key M. It asks for the source among the other stacks of the host, shows the same preview with the versions, the source and its observation time, and asks for the confirmation before the run. Without sudo the interface shows the command sudo qhost stack promote and runs it after y; that command reads the source again when it runs.`
