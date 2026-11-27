---
title: General
order: 1
keywords: [general, theme, language, mouse, icons, editor, navigation, free proposals]
screens: ["settings/general", "title:title.freeswitch"]
---
# General

The General tab is one form with the preferences of this interface: Theme, Start area, Language, Mouse, Icons, Navigation and Editor. Free proposals appears too once the host service has answered.

- {next} and {prev} move between the fields.
- {left} and {right} choose a value in a field with choices. Text fields take typing.
- {save} stores the values. They apply at once, except the start area, which applies from the next start. If this device has no place for the preferences, the form says so and stores nothing.
- {back} discards the changes and returns to the tab line. With unsaved changes it asks first, and {yes} drops them.

Free proposals decides whether agents may propose commands that qhost does not check and cannot undo. Nothing runs before an administrator runs a proposal. Saving a changed value asks about it and runs with sudo: {yes} switches it, {no} leaves it as it was.
