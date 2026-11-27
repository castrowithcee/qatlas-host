---
title: Containers
order: 6
keywords: [containers, docker, compose, start, stop, restart, engine, foreign project]
screens: [containers]
---
# Containers

The list shows every container of the host, grouped as own stack, foreign project or single containers. A foreign project is a Compose project that no stack of this host owns. A group marked as a conflict has the name of a stack but another folder. Each row shows name, state, image, uptime and restarts. The state is ok, unhealthy, restarting, stopped or starting. The list reads the container engine again every few seconds.

- {enter} opens the detail of the marked container, or folds the marked group.
- {left} and {right} fold and unfold groups, {search} filters the list.
- {containerlogs} opens the log of the marked container.
- {stackstart}, {stackstop} and {stackrestart} start, stop or restart the marked container. A question names its state and what follows, and cancel is the default answer. Without root the question shows the command that runs through sudo. The result shows the state after the action. A container that is not ready in time counts as failed.
- {new} takes over the foreign project of the marked row, see the page on adopting a project.
- {reload} reads the list now, {area} switches the area.

With several engines each engine is a section of its own. An engine without a deploy service is read-only: its containers are only shown, and no key acts on them. Without root such engines are not listed at all. If the engine cannot be read, the screen says why and keeps the last list.
