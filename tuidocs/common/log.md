---
title: Container and service log
order: 2
keywords: [log, output, container, service, level, diagnose, follow, period, save]
screens: [containers/clog, detail/clog, runs/clog]
---
# Container and service log

This view shows lines that arrive over time. In Containers it shows the output of one container. In the stack detail it merges the output of all containers of the stack. In Runs it shows the log of the deploy service itself, with the level of each entry.

- {up}, {down}, {pageup}, {pagedown}, {first} and {last} scroll.
- {log.follow} follows the newest line.
- {search} searches the lines, {log.next} and {log.prev} jump between matches.
- {log.filter} chooses the period of the lines.
- {log.wrap} wraps long lines, {log.time} shows the time column.
- {savelog} saves the lines the view holds as a new file and tells where it went.
- {reload} reads again.
- {back} closes the log.

## Deploy service

- {loglevel} cycles the lowest level shown: all levels, info, warn, error.
- {diagnose} collects a diagnostic package and saves it as a file. The package holds the host name and paths, so check it before you pass it on. A second press while it runs only says so.
