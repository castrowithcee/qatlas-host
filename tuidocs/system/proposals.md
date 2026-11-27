---
title: Proposals
order: 12
keywords: [proposals, approve, reject, host change, free commands]
screens: [system/proposals]
---
# Proposals

Administrators and agents can propose a change of the host. The list shows each proposal with its kind, who sent it, when, when it expires and its state. Only open proposals can be decided.

- {up} and {down} mark a proposal, {search} filters the list. {back} clears the filter.
- {enter} shows the proposal with its change and commands.
- {approve} goes on to the approval of an open proposal.
- {reject} asks to reject an open proposal. The question comes first, {yes} rejects with sudo and {no} drops it. Nothing on the host changes.
- {reload} reads the proposals again.

A proposal of free commands carries commands that nothing checks. It can be run only while Free proposals is on in Settings, General.

If the host or a proposal changed since the list was read, the page asks you to press {reload}.
