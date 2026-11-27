---
title: Move a project into a source
order: 3
keywords: [move, preview, source, blocks, warnings, rollback]
screens: ["title:move.title"]
---
# Move a project into a source

This moves a foreign project into a source that exists, chosen in the adopt form. The preview and the move run as a command through sudo, or directly as root, and the last question shows the command. The preview changes nothing.

The preview shows the folder, the source, the target folder, the project and its engine. It lists the files of the first commit, what is left out, blocks and warnings. A block means the move cannot be done as it is, with what to do about it, for example a source on another file system. With a block the sheet only closes with {no}, {enter} or {back}, and nothing was changed.

Without blocks the question asks to move the project. If you answer {yes}, the project goes down, its folder is renamed into the source, committed (and pushed when the source has a remote) and deployed there. Data is never copied or deleted. A warning such as downtime is shown but does not stop the move. {no} cancels, {up} and {down} scroll.

The result names where the folder is now. If a step fails, everything is taken back and the result names the step and the reason. If the way back does not finish either, the result says so, lists its steps and what is left to do by hand. The container list is read again in every case.
