---
title: System
keywords: [System, Host, Benutzer, Gruppen, Ordner, Rechte, Journal]
---
# System

System verwaltet den Host selbst: seine Benutzer und Gruppen, die Rechte von Ordnern und die Dateien darunter. Der Bereich liest die Kontodateien und das Dateisystem direkt und funktioniert deshalb auch, solange der Deploy-Dienst nicht eingerichtet ist.

Die Reiter sind Benutzer, Gruppen, Ordner, Vorschläge, Änderungen und Protokoll. In der Reiterzeile gehen {left} und {right} durch sie, {down} betritt die Seite.

- Eine Änderung geschieht nie sofort. Ein Formular oder eine Taste führt zuerst zu einer Rückfrage mit den genauen Befehlen. {yes} führt sie mit sudo aus, {no} verwirft die Änderung.
- Jede Änderung kommt in ein Journal. Der Reiter Änderungen listet es und nimmt eine Änderung zurück, wo das möglich ist.
- Vorschläge anderer Administratoren warten im Reiter Vorschläge, bis du sie freigibst oder ablehnst.

Während die Listen gelesen werden, zeigt der Bereich eine Meldung. Scheitert das Lesen, steht der Grund dort, und {reload} liest erneut.
