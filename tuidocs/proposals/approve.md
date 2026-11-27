---
title: Approving a proposal
order: 2
keywords: [approve, apply, confirm, sudo, host key, engine]
screens: ["title:title.propapprove"]
---
# Approving a proposal

This question asks whether to approve the proposal and apply the changes shown below it, such as the settings, the maintenance window, the channel, the grant or the stack folder with its permission and service changes. Right before applying, the proposal and its effect are read again. If either differs from what you saw, nothing is applied and the view shows the proposal as it is now. Yes marks the proposal as approved, makes the changes and keeps the result on the proposal. Without root, the command to run through sudo is shown and the sudo password is asked.

Approving a stack folder can take several steps first: a continue question after the inspection, the engine to use when the host has more than one, and whether to trust the host key of the server. Each step names what follows and can be cancelled.

- {yes} confirms the step.
- {no} cancels and changes nothing.
- {up} and {down} scroll the details.

If the state changed while you decided, a message says that nothing was applied.
