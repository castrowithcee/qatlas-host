---
title: Deploy-Schlüssel
keywords: [Deploy-Schlüssel, öffentlicher Schlüssel, Fingerabdruck, SSH, Git-Plattform]
---
# Deploy-Schlüssel

Die Ansicht zeigt den Deploy-Schlüssel des gewählten Stack-Ordners: seinen Fingerabdruck und seinen öffentlichen Schlüssel. Den öffentlichen Schlüssel trägst du bei der Git-Plattform als Deploy-Key ein, damit der Host das Repository lesen darf. Ein Remote, in das du pushst, braucht Schreibrecht. Der private Schlüssel wird nie angezeigt.

Ohne Schlüssel sagt die Ansicht das und bietet an, einen zu hinterlegen oder zu erzeugen. Dieselbe Ansicht öffnet sich, wenn du einen Klon oder ein Remote für einen Ordner ohne Schlüssel hinzufügst, und es wird dann nichts hinzugefügt.

- {edit} setzt den Schlüssel: einen vorhandenen hinterlegen oder einen neuen erzeugen.
- {back} führt zur Liste zurück.
- {reload} liest den Schlüssel neu.
