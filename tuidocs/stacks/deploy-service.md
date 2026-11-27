---
title: Set up the deploy service
order: 9
keywords: [deploy service, setup, repair, not running, channel refused, rootless, system engine, docker, root]
screens: [stacks/setup, detail/setup, runs/setup, containers/setup, proposals/setup, title:setup.title]
---
# Set up the deploy service

Until a deploy service is set up, the areas that need it show this note instead of their content. The deploy service rolls out and updates your stacks, runs as its own system user apart from the host service and has access to Docker. The note shows the command line for the setup too.

{setupdeploy} opens the dialog. In the Stacks area it is also offered on a host that has a service already, to add the one of a rootless engine.

1. Choose the deploy service. The system engine gets a service with its own system user. A rootless engine gets a service that runs as the user who owns that engine.
2. For a rootless engine, give the user. The users that have an engine are suggested, and another name can be typed. Show steps goes on.
3. The last question lists the steps that run as root, and the warnings to read first. Nothing runs before you answer Yes.

Keys:

- {yes} sets up the system engine, or confirms the last question.
- {rootless} chooses the rootless engine.
- {no} cancels. {enter} takes the marked answer.
- {up} and {down} pick a suggested user. {next} completes it.

Without root the setup asks for the sudo password before it runs. If the service is set up already, the dialog only says so.

## A service that does not answer

The interface starts even when a deploy service that is set up does not answer. The same areas then show the cause instead of the setup: the service is not running, or its channel refused the connection because the owner or mode of the channel folder, or the user the service runs as, is not what the host expects. Settings and System stay usable.

{setupdeploy} repairs the deploy service of the system engine. The dialog shows what is wrong as the exact steps that run as root, and nothing runs before you answer Yes. Without root it asks for the sudo password first. Once the repair worked, the note disappears at the next reading.

If the setup finds no step to repair, the note says so and offers no repair. The service log then names the reason: System, Log shows it, and the command line shows it with journalctl -u qhost-deploy. A rootless instance that does not answer is named in the area and is not repaired here.
