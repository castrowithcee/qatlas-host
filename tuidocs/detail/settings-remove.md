---
title: Remove the settings of a stack
order: 6
keywords: [settings, remove, clear, deploy only]
screens: ["title:title.settingsremove"]
---
# Remove the settings of a stack

This question asks whether to remove all settings of the stack: update rules, backup, compose files and check. {yes} creates a commit that deletes the settings file in the stack folder and pushes it where the folder has a remote. Afterwards the stack is deployed but never updated. This cannot be undone from here.

- {yes} removes the settings.
- {no} cancels and changes nothing. {enter} does the same, because cancel is the default answer.

A stack without settings gets a message instead. It is already deploy only, and nothing is removed.
