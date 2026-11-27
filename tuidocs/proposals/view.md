---
title: Reading a proposal
order: 1
keywords: [proposal, content, effect, approve, reject, note, reason]
screens: [proposals/preview]
---
# Reading a proposal

This screen shows one proposal: who submitted it, when, when it expires and its state. A note, a failure or the reason of a rejection follows when there is one. Below, Proposed lists the content in full, exactly as submitted. For an open proposal, Effect if applied now shows what approving would change on the host. The effect is read, never applied. Read every value, especially a backup command, which runs on the host later.

Without root, the effect of a stack folder is worked out when you approve it, and the sudo password is asked then. If the content cannot be read, or this version cannot show the kind, the screen says so and the proposal cannot be approved.

- {approve} approves an open proposal. Nothing is applied before you confirm.
- {reject} rejects an open proposal and asks for an optional reason.
- {edit} sets the expiry of a standing grant before you approve it.
- {up} and {down} scroll.
- {reload} reads the proposal again.
- {back} returns to the list.

Both keys are refused with a note when the proposal is no longer open or this version cannot approve it.
