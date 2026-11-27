---
title: Commit the changes
order: 11
keywords: [commit, push, refused, secrets, protected branch, runs]
screens: [title:title.commit, title:title.commitrefused]
---
# Commit the changes

The question names the number of files and the folder, and lists the files. qhost checks that every stack of the files still loads and that no secret goes along, and commits the files as they are now. For a clone it also pushes the commit. The normal run then deploys their stacks. Without root the question shows the sudo command that Yes runs.

- {yes} commits. {no} cancels, and {enter} takes the marked answer, which is cancel.
- {up} and {down} scroll the question.

When the commit is made, a notice names it and says that the run of the stacks starts now. {showlog} opens the area Runs, {no} closes the notice.

When the commit is refused, a notice says why and that nothing was committed, and the list is read again. The reasons:

- The commit would push secret-like files. Keep secret values in secrets.env, which never leaves the host.
- The remote refused the push, as a protected branch does. Change that at the git platform.
- The folder moved on, or the list no longer matches. Check the new list and commit again.
- A stack of the files no longer loads. Fix the file first.
- A run of a stack of the files is under way. Commit again when it has ended.

{no} or {enter} closes the notice.
