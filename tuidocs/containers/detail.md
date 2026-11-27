---
title: Container details
order: 1
keywords: [detail, ports, mounts, networks, labels, compose files, env files]
screens: [containers/detail]
---
# Container details

The detail shows one container: its group, state (with the state of the engine and its health check), image, uptime and restarts. Below it are the sections Ports, Mounts, Networks, Labels and Environment variables, which lists names only and never values. A container of a foreign project also shows its folder and the sections Compose files and Env files, as the project's labels name them. A section without entries says none. The screen reads again every few seconds.

- {up}, {down}, {pageup} and {pagedown} scroll the page.
- {containerlogs} opens the log of this container.
- {stackstart}, {stackstop} and {stackrestart} start, stop or restart it, after a question as in the list. The detail shows the new state afterwards.
- {new} takes over the foreign project of this container, see the page on adopting a project.
- {reload} reads the detail now.
- {back} returns to the list.
