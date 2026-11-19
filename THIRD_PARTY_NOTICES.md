# Third-party notices

## qatlas-sysmin

Parts of this repository are copied from or derived from qatlas-sysmin
(https://github.com/castrowithcee/qatlas-sysmin):

- the command and error handling structure of `internal/cli`, the signal handling of its `run` command and the help topic package `internal/helptopics`
- the protocol frame, size and time limits and the connection handling of `internal/control` and `internal/service`, and the request/answer calls of `internal/client`, reduced to a channel without vault or approvals
- the administrator check against the group database in `internal/admins`, extended to the service group and checked for every request
- the request handling with peer and administrator check, the action log entry per request and the log, verify and host record commands of `internal/hostservice`, and the memory protection of the host service's `run` command, adapted from the daemon and service packages of qsysmin
- the terminal check before the terminal interface starts in `internal/cli` and the colour profile mapping of `internal/tui`
- the hardened git environment, SSH options, output limits and process-group handling of `internal/gitrepo`
- the poll loop with spread and wake-up, the per-stack run lock with dropped parallel triggers, and the state and run-log layout of `internal/engine`, reduced to a deployment engine without approvals, vault or step plans
- the runner with process-group handling and bounded output, the Compose command assembly and the container and HTTP checks of `internal/compose`, reduced to one fixed pull, up and check flow without step plans
- the volume archive through a helper container without network, the backup directory naming, the retention and the restore flow of `internal/backup`, rebuilt around Compose stacks (the backup store, metadata, data-directory handling and confirmation are new)
- the OCI registry client with its reference parsing and fake registry in `internal/registry`, and the version selection of `internal/version`, rebuilt on the shared HTTP and rate-limit packages with redirects refused, and reshaped to a per-variable ceiling, filter and major hint
- the line-based value editing and quoting rules of `internal/envfile`, rebuilt to work on bytes in memory and to follow the `.env` parsing of `internal/stacks`
- the GitHub release notes reader in `internal/changelog` (release list cache, text sanitizing and source address parsing), rebuilt on the shared HTTP and rate-limit packages with redirects refused, and reshaped to a result that always carries the releases page link
- the self-update command in `internal/cli/selfupdate.go` and the trusted release key package `internal/release`, adapted from the update command and release package of qsysmin
- the host read model, the host actions with their runner and journal, and the action log entry type in `internal/host/hostadmin`, `internal/host/hostchange` and `internal/host/auditlog`, copied and adapted (names, paths and the kit version)
- the host read and change commands `host users|user|groups|group|folder` and `host apply|undo|journal` in `internal/cli/host.go` and `internal/cli/hostchange.go`, adapted from the host commands of qsysmin
- the sudo hand-over of the terminal interface with the two-step run of a secret input, the report reading and the failure texts in `internal/tui/hostrun.go` and `internal/tui/hostask.go`, the user forms with their early checks and red questions in `internal/tui/systemusers.go`, the folder page with its path input, the folder form with the count of the entries below it and the red question of a system path in `internal/tui/systemfolders.go`, the changes page with the journal read through sudo and the undo of an entry in `internal/tui/systemchanges.go`, the action log page with the result of the check of its chain in `internal/tui/systemlog.go`, and the command builder `hostRunner` and the restart under `sudo -g` in `internal/cli/tui.go`, adapted from the host area of the qsysmin terminal interface to the forms, dialogs and lists of qatlas-kit
- `.github/workflows/ci.yml`
- the service unit `packaging/qhost.service` and its unit test in `packaging`, adapted to the unit and test of qsysmin
- the end-to-end environment in `e2e/`: `run.sh`, `host.Dockerfile`, `registry.Dockerfile`, `fakes.Dockerfile` and the helper services in `e2e/fakes` (releases fake, webhook receiver, demo application)

Copyright (c) 2026 castrowithcee

qatlas-sysmin is licensed under the GNU Affero General Public License, version 3. The full license text is in
[LICENSE](LICENSE).
