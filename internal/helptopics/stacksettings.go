package helptopics

const stackSettingsTopic = `The settings of a stack say how qhost updates, backs up and checks it. They live in the stacks repository as .qatlas/qhost/<stack>.yaml in its root, one file per stack, named by the stack's Compose project name. A stack without settings is deployed but never updated.

The settings are shown and changed only in the terminal interface or with qhost stack settings <stack>, qhost stack settings set <stack> and qhost stack settings remove <stack>; nobody edits the file by hand. The stack detail of the terminal interface has the form Update rules (key e): mode, ceiling, major jump, the observation time in minutes (0 off), the trigger (maintenance window, polling or webhook only) with the interval in seconds for polling, and, per image version variable, a tag filter and a changelog repository. The form Backup (key b) sets the data directories, the volumes ticked from the named volumes of the stack's compose files (a saved volume the files no longer have stays and is marked), the command as a list of arguments, and the retention (keep, maximum age in days). The form Compose files (key I) sets the compose files as a list that can be reordered, at most 16; an empty list removes the entry and leaves the files to the stack's own detection. The form Check (key k) sets the container check on or off and the HTTP check: with it on, the address, the expected status (100 to 599, 200 by default) and the timeout (1s to 10m, 10s by default); with it off the three fields are ignored and the section holds no probe. The same form sets whether promotions into the stack compare the image fingerprints (promote.compare_images). Each form saves its own section only: saving asks first, commits, and in a remote stack folder pushes, and leaves the other sections as they are. Remove deletes the settings file by a commit, with all its sections, and leaves the stack deployed but never updated; the stack detail offers it as the action Remove settings (key E) after a question marked as far-reaching, and cancelling changes nothing. A stack without settings is reported and nothing is committed. qhost checks them against the stack, writes the file, commits it and pushes the commit. A settings commit does not touch the stack folder and starts no run; the settings act on the next update or deploy. The file is read as data only: unknown fields, a missing version, or a path that is absolute or leaves the stack folder are rejected.

Fields (all optional except version):
- version: 1
- compose.files: the compose files of the stack as a list of relative paths inside the stack folder, in merge order like COMPOSE_FILE; it takes precedence over COMPOSE_FILE in .env and over the default names
- update.mode: manual (default) or automatic
- update.trigger: window (default, the maintenance window checks the stack), interval (qhost queries the registries of the stack regularly, also outside the window) or webhook (an external call starts the check); the maintenance window handles only stacks with the trigger window. With interval or webhook, a check that finds an update updates a stack in automatic mode at once, outside the window and kept back by the same marks as in it, and reports a stack in manual mode once per newly available version. A registry that limits requests or cannot be reached doubles the wait of its stacks after each such check, up to 30 minutes (a longer interval stays as it is), until a check of it succeeds
- update.interval: seconds between two registry queries, from 30 to 86400 (default 60); allowed only with update.trigger: interval and refused otherwise
- update.ceiling: patch, minor (default) or major; the largest version step taken; versions are MAJOR.MINOR.PATCH (SemVer or CalVer), and with CalVer the first part, for example the year, counts as major
- update.major: stepwise (default, one major version at a time) or direct
- update.observe: observation time in minutes after an update by qhost (default 60, 0 off, at most 1440); a failure of the stack in it rolls the update back (see qhost help containers); a commit that did not come from qhost starts none; qhost status shows a running one as observe with its end and the update commit; a version whose update failed or was rolled back in it is locked for its variable and automatic updates skip it (qhost status shows failed_versions) until a higher version succeeds or qhost stack update <stack> --version <that version> succeeds
- update.tags.<VARIABLE>: regular expression a candidate tag must match; VARIABLE is the image version variable in .env
- update.changelog.<VARIABLE>: GitHub repository (owner/name or a github.com address) whose releases are shown as changelog; it takes precedence over the image label org.opencontainers.image.source
- backup.paths: data directories below data/ in the stack folder, for example data/db; other paths are refused when a backup runs
- backup.volumes: volume keys of the Compose file (the top-level volumes: names), not Docker volume names
- backup.command: a command as a list of arguments, never a shell line (see qhost help backups)
- backup.retention.keep: number of newest backups to keep (default 7 when no retention is set)
- backup.retention.max_age_days: age limit in days; a backup is kept only while it satisfies every limit that is set
- check.containers: require all containers to be running (default true)
- check.http.url: http or https URL to probe
- check.http.status: expected status code (default 200)
- check.http.timeout: probe timeout from 1s to 10m (default 10s)
- promote.compare_images: true or false (default false); a promotion into the stack compares the image fingerprints with the source (see qhost help promote)
- backup needs at least one of paths, volumes, command

Example:
version: 1
update:
  mode: automatic
  ceiling: minor
  tags:
    DEMO_APP_VERSION: '^[0-9]+\.[0-9]+\.[0-9]+$'
backup:
  paths: [data/db]
  retention: {keep: 5}
check:
  http: {url: 'http://localhost:8080/health', status: 200, timeout: 5s}

Image versions stand as variables in the stack's .env, for example image: registry.example.test/demo/app:${DEMO_APP_VERSION}. A fixed or floating tag such as latest leaves the image deploy only; qhost stack check names it (see qhost help stack-capabilities).`
