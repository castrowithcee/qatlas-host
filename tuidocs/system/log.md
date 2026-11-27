---
title: Log
order: 11
keywords: [log, action log, audit, chain of hashes, service]
screens: [system/log]
---
# Log

The page shows the action log of the host service, newest first: time, user, action, result and the change it belongs to. Above the list the check of its chain of hashes says whether the log is intact or where it breaks. Administrators in the group qhost read it without sudo. It is read when the page opens, after a change made here and on {reload}, never on a timer.

- {up} and {down} mark an entry, {search} filters the list. {back} clears the filter.
- {instance} switches between the host service and the deploy services. It is offered where more than one exists. A deploy service shows function, stack and origin instead.
- {savelog} saves the lines shown as a file.
- {reload} reads the log again.

If the host service does not run or the interface cannot reach it, the page says so instead of the list. After a change the page says the log is older than the host.
