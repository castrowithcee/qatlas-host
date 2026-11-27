---
title: Runs
order: 3
keywords: [runs, history, update, rollback, filter, log]
screens: [runs]
---
# Runs

This list shows the runs of the deploy service, newest first: the stack, when the run started, its result, what triggered it, how long it took, its run ID and tag. A run that is still going shows as running.

- {enter} opens the log of the run under the cursor.
- {filter} limits the list to the stack marked in Stacks. Press it again to show all stacks. Without a marked stack it only shows a warning.
- {rollback} returns the stack to the state before the run. It works only on a successful update or promotion run and opens a question about the data of the stack. Any other run only shows a warning.
- {servicelog} opens the log of the deploy service.
- {search} filters the list by what you type. {enter} keeps the filter and {back} clears it.
- {up}, {down}, {pageup}, {pagedown}, {first} and {last} move the cursor.
- {reload} reads the runs again.
- {area} switches to another area.
