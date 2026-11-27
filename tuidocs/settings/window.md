---
title: Maintenance window
order: 3
keywords: [maintenance window, days, start, duration, time zone, host window, own window]
screens: ["settings/window", "title:title.window", "title:title.window.own", "title:title.window.ownset", "title:title.window.reset"]
---
# Maintenance window

This tab shows, for each deploy service, the days, start, duration and time zone of the window that applies, whether it is open now, the next opening and the last work. A label says where the window comes from: the host window, which every service without an own one follows, or an own window.

- {edit} opens Change the host window. {new} opens Own window of the chosen instance. Both have the fields Days (daily or weekdays such as mon,thu), Start (HH:MM), Duration (1m to 12h) and Time zone (local or an IANA name). {next} and {prev} move between the fields, {save} checks the form, {back} discards it. Only changed values are sent. A host window form without a change is refused.
- Saving asks for confirmation. A host window change reaches every service without an own window within seconds. An own window changes only its instance, which follows the host window again after a reset. {yes} confirms, {no} cancels.
- {delete} asks to let the instance follow the host window again. It removes the own window and leaves the host window alone. An instance without an own window is only told so.
- {instance} chooses the instance for the own window where several services exist. {reload} reads the windows again.
