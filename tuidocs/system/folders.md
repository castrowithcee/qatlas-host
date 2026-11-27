---
title: Folders
order: 7
keywords: [folders, files, browser, permissions, new, rename, copy, delete, edit, search]
screens: [system/folders]
---
# Folders

The browser shows the whole host from the root: name, size, change time, permissions and owner. A .git folder is only read.

- {up} and {down} mark an entry, {enter} opens a folder or shows a file.
- {back} goes up one folder. At the root it leads to the tab line.
- {search} filters the folder. A filter that starts with * searches below the shown folder.
- {info} shows the permissions of the marked entry.
- {edit} edits a file in your editor.
- {new}, {rename}, {copy} and {delete} change entries. Each asks first and shows the exact commands.
- {reload} reads the folder again.

If a folder or file cannot be read without root, the browser asks to read it with sudo. Nothing is changed by reading.
