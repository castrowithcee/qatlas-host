---
title: Stack list
order: 1
keywords: [list, folder, stack, fetch, release, add, deploy key, remote]
screens: [stacks]
---
# Stack list

Every stack folder is a row with its kind (clone or local) and its last poll, and the stacks of the folder stand below it. The detail of the marked folder shows its address and last poll. Without a folder the area points to adding one.

On a folder row:

- {new} adds a stack folder. {enter}, {left} and {right} fold and unfold the folder.
- {fetch} fetches the remote now. Only for a clone.
- {edit} opens the deploy key of the folder. Only for a clone.
- {remote} adds a remote to a local folder.
- {secure} makes the .git of a folder root-only when the row says git is not run there. Without root it runs sudo qhost source secure.
- {delete} releases the folder.
- {files} opens the files of the folder. {changes} opens its changes, where it has any.

On a stack row:

- {enter} opens the stack. {toggle} ticks it for an action.
- {update}, {deploy} and the further letter keys of the footer run an action on the ticked stacks, or on the marked one.

On every row:

- {search} filters the list. {reload} reads it again.
- {setupdeploy} sets up a deploy service, shown where one can be added.
- {area} switches the area.

In the file view of a folder, opened with {files}:

- {enter} opens a folder or a file. {search} filters the entries. A filter that starts with an asterisk searches below the shown folder.
- {back} clears the filter, goes up one folder, and at the root returns to the list.
- {new}, {rename}, {copy} and {delete} change files, {edit} edits the marked one. The folder .git and the settings in .qatlas/qhost are only read here.
- {changes} opens the uncommitted changes, where there are any. {reload} reads the folder again.
