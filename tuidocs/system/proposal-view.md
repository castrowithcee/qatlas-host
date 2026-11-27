---
title: Proposal of a change
order: 13
keywords: [proposal, approve, initial password, free commands, run, result]
screens: ["title:title.hostproposal", "title:title.hostproppassword", "title:title.freerun", "title:title.freeresult"]
---
# Proposal of a change

## Proposal

The sheet shows who proposed it and in which state, the change as a sentence, a red warning where it reaches root, a system account, the Docker right or a system path, and the exact commands as typed by hand. {up} and {down} scroll.

- {yes} approves an open proposal that passes the check. It only goes on to the question that shows the commands, and nothing is applied before you answer that one.
- {no} closes the sheet. So do {back} and {enter}.

## Initial password

A proposal for a new user never holds the password. The form asks for it twice, hidden while typed. {save} goes on to the question, {back} discards the form.

## Free commands

For a proposal of free commands the sheet shows the reason and the commands word for word, and {yes} reads run. The question names the number of commands. {yes} runs them as root with sudo, one after the other. The run stops at the first one that fails, and nothing can be undone. {no} drops it.

## Result of the commands

The sheet lists every command with its exit code and output, and marks those that did not run. {no}, {back} and {enter} close it. {up} and {down} scroll.
