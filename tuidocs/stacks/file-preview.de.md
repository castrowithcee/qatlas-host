---
title: Eine Datei eines Stack-Ordners
keywords: [Datei, Vorschau, bearbeiten, Secrets, aufdecken, Suche, Umbruch, Zeilennummern]
---
# Eine Datei eines Stack-Ordners

Die Dateiansicht zeigt die geöffnete Datei als Text. Werte in secrets.env sind maskiert, bis du sie aufdeckst. Der Ordner .git und die Einstellungen in .qatlas/qhost werden hier nur gelesen.

- {up}, {down}, {pageup}, {pagedown}, {first} und {last} blättern.
- {search} durchsucht die Datei. {log.next} und {log.prev} springen zwischen Treffern.
- {log.wrap} bricht lange Zeilen um. {text.numbers} zeigt Zeilennummern.
- {text.reveal} deckt geheime Werte auf oder maskiert sie.
- {edit} bearbeitet die Datei. Das Speichern schreibt sie als root und führt einen Journaleintrag. Nichts wird committet: Die Änderung erscheint in den nicht committeten Änderungen des Ordners.
- {back} schließt die Datei und führt zur Ordneransicht zurück.

Einen Datenordner eines laufenden Containers kann dieser überschreiben, stoppe im Zweifel vorher den Stack.
