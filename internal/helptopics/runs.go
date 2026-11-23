package helptopics

const runsTopic = `A run deploys a stack at one commit: manually, on a new commit, by the maintenance window, by a webhook, or as the rollback of a failed update. qhost stack runs lists the recorded runs and qhost stack log <run> prints the log of one.

Tag:
- After a run that succeeded, and after a rollback that succeeded, qhost sets the fixed Git tag qhost/<host>/<stack>/<run> on the commit the stack now runs at (after a rollback, the revert commit) and pushes it with the deploy key. <host> is the short host name made safe for Git references and <run> is the run ID, so the newest tag of a host and stack sorts last. A fresh clone shows the latest state of each host and stack with git tag -l 'qhost/*'.
- A tag is never moved or forced, and a failed run sets none. In a stack folder without a remote the tag is set locally and nothing is pushed.
- After a pushed tag, qhost deletes the stack's older tags of this host, locally and in the remote, beyond the retention of the stack's backup rules (keep, maximum age in days), or beyond the latest 50 without backup rules. The newest tag always stays. Tags of other hosts and stacks and tags not set by qhost are never touched. With a local stack folder only local tags are deleted.
- qhost stack runs shows the tag of each run; backups name the tag of their commit besides the commit.
- If the remote refuses the tag or the deletion of an old tag (the deploy key must be allowed to delete tags), the stack gets the finding tag-rejected in qhost status, and the old tag stays. The run stays as it was and starts no rollback. The finding goes away with the next run whose tags go through. See qhost help repository.
- The tag is an anchor in the repository only. A rollback or repair always returns to the state the service stored for the stack, never to a tag, and a tag never makes a commit the deployed one, also not on a rebuilt host.`
