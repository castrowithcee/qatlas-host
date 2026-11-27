package cli

// transitions are the steps of this release that bring the setup and the stored state of an earlier one to it, in
// the order they run. 'qhost update migrate' runs them after every update; this release has none.
var transitions []transition
