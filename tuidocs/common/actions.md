---
title: Stack actions
order: 1
keywords: [update, deploy, pause, unpause, pin, unpin, resume, check, start, stop, restart, confirm, preview, result]
screens: [action:update, action:deploy, action:pause, action:unpause, action:pin, action:unpin, action:resume, action:check, action:stackstart, action:stackstop, action:stackrestart]
---
# Stack actions

Every action on stacks asks first. The question names the stack, or the number of stacks with their names, and says in one sentence what follows. Nothing reaches the service before you answer. The same questions appear for start, stop and restart in Containers, where they name one container.

- {yes} runs the action.
- {no} cancels. {enter} also cancels, so a stray key does nothing.

Without root, a second question follows. It shows the exact sudo command of each stack, and the actions then run one after another.

## Update preview

Update opens a longer view first. For each stack it lists the versions that change and the release notes. The answer {yes} appears once every changelog is read or has failed. Scroll with {up}, {down}, {pageup}, {pagedown}, {first} and {last}.

## Result

The result lists every stack as ok, failed or not run, with a line on what happened. Start, stop and restart end at the first failed stack and mark the rest as not run. Stacks that failed stay ticked.

- {no} closes the result.
- {showlog} opens the run in Runs. It is offered only where a run was started.

Resume only clears a pause or a halted repair and starts nothing. The key check registries asks the registries and changes nothing in the stacks.
