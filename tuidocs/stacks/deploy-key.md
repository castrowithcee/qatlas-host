---
title: Deploy key
order: 7
keywords: [deploy key, public key, fingerprint, ssh, git platform]
screens: [stacks/key]
---
# Deploy key

The view shows the deploy key of the marked stack folder: its fingerprint and its public key. You enter the public key as a deploy key on the git platform, so that the host may read the repository. A remote you push to needs write access. The private key is never shown.

Without a key the view says so and offers to store or generate one. The same view opens when you add a clone or a remote for a folder that has no key yet, and nothing is added then.

- {edit} sets the key: store one you have, or generate a new one.
- {back} returns to the list.
- {reload} reads the key again.
