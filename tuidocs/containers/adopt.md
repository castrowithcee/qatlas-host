---
title: Adopt a foreign project
order: 2
keywords: [adopt, take over, foreign project, own source, git repository, move, stack folder]
screens: ["title:title.adopt"]
---
# Adopt a foreign project

Adopting turns a running Compose project into a stack folder of this host. It starts from the labels of the running containers, which name the working directory and the Compose and env files. Nothing changes before the last question.

If a source of the same engine could take the project, a form first asks how to take it over:

- A source of its own makes the folder a stack folder of its own. A folder without a repository gets a new Git repository of its own, with a first commit of its files, and the next questions ask for a remote if you want one. A folder that already is a repository is taken over as it is.
- Moved into the source gives the project a folder inside a source that exists, see the page on the move. A folder with a repository of its own can no longer move into an existing source after it became a source of its own.

{enter} opens the choice, {save} continues, {back} discards the form. Without a choice list the flow goes on as a source of its own.

The next sheets are the questions of adding a stack folder: what was found in the folder, which stacks it holds and what each works for, and the changes after yes. They may ask for the host key and a deploy key, or offer {withremote} and {choose}. Warnings name what a deploy would pass to Compose differently from what runs now: another project name, other Compose files or env files. The first deploy would then create new containers instead of taking over the running ones.

- {yes} makes the changes, {no} or {enter} cancels, {up} and {down} scroll.

A project that cannot be taken over gets a sheet with the reason and no way on. Reasons: the labels name no working directory or a relative one, a file lies outside the folder, the folder does not hold exactly one stack, or its compose file is not directly in the folder. Press {no}, {enter} or {back} to close it.
