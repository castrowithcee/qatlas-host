---
title: Take over versions
order: 10
keywords: [promote, take over, versions, source stack, images]
screens: ["title:title.promotepick", "title:title.promote"]
---
# Take over versions

{promote} sets the image versions of this stack to those another stack runs. It opens a list of the other stacks of the host, with the update rules' promote source marked first when one is set. Choosing a stack reads it and asks the service for a preview. If this stack already has the versions for the images they share, a message says there is nothing to take over.

Otherwise a question shows the source stack and its commit, whether the source is still under observation, whether the image fingerprints are compared, and each variable with its old and new version.

- {yes} stops the stack, backs it up if it has backup rules, writes the new versions into its .env in one commit that is pushed, then starts the stack and checks it. A failing run is rolled back. Only images both stacks run are set. The service refuses when the source deployed another commit since the preview.
- {no} or {enter} cancels and changes nothing.
- In the list, {enter} chooses, {back} leaves it without a change.
