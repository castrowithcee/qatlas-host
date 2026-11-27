---
title: Add a stack folder
order: 2
keywords: [add, folder, inspect, clone, adopt, local, git, engine, host key, rejected]
screens: [title:title.folderadd]
---
# Add a stack folder

The form asks for the absolute path of a folder. The folder is only inspected, nothing changes before the last question. What the inspection finds decides the questions that follow, and each one lists what it found and what Yes changes.

- Missing or empty folder: Clone a repository into it. Continue asks for the address, then shows the host key.
- Clone with an origin remote: Take it over with its address. Continue shows the host key and checks the deploy key.
- Repository without a remote: Take it over as a local folder.
- Files without a repository: Start a git repository with a first commit of the listed files. A second look at the folder comes first, and nothing changes if its list differs by then.
- A folder that cannot be taken over, or has no file for a first commit, only says why. Close it with {no}.

Keys of the form:

- {save} inspects the folder. {back} discards the form, after a question.
- {next} and {prev} move between the fields.

Keys of a question:

- {yes} makes the changes listed below. The last question shows the command it runs, and without root it asks for the sudo password first.
- {no} cancels. {enter} takes the marked answer, which is cancel.
- {withremote} takes a folder of files in together with a remote, and asks for its address.
- {choose} picks unclear files for the first commit.
- {up} and {down} scroll the question.

With a second engine the form Engine comes first. {left} and {right} step through the engines, {enter} opens the list, and {save} takes the choice. A folder is inspected again for any engine other than system.

The host key is shown with its fingerprint: compare it with the git platform before {yes}. Without a deploy key for the folder the key view opens instead, and nothing is added.
