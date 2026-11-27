package cli

// transitions are the steps of this release that bring the setup and the stored state of an earlier one to it, in
// the order they run. 'qhost update migrate' runs them after every update; the start step of the deploy service
// runs only deployServiceTransitions. stackGitTransition stays out of this list: migrate runs while the service of
// the earlier release still runs, which may write the .git it would secure.
var transitions = []transition{deployRootTransition}
