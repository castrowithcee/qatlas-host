---
title: Ablauf einer Freigabe
keywords: [Freigabe, Ablauf, Datum, Dauerfreigabe]
---
# Ablauf einer Freigabe

Eine Dauerfreigabe erlaubt einem Benutzer, eine Aktion auf einem Stack ohne sudo auszuführen. Dieses Formular setzt den letzten Tag, an dem sie gilt, als JJJJ-MM-TT, bevor du freigibst. Das Datum gilt in UTC, und die Freigabe gilt bis zum Ende dieses Tages. Es darf den angefragten Ablauf beibehalten oder verkürzen, nie verlängern. Ein leeres Feld behält den angefragten Ablauf. Hat der Vorschlag keinen angefragt, ist jedes gültige Datum erlaubt.

Das Datum gilt nur für deine Freigabe und erscheint in der Wirkung. Die Ansicht markiert es dann als nicht den angefragten.

- {save} behält das Datum.
- {next} und {prev} wechseln zwischen Feld und Schaltflächen.
- {back} bricht ab und behält den bisherigen Ablauf.
