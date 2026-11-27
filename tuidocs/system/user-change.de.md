---
title: Benutzer ändern
keywords: [Benutzer ändern, Admin, Docker, sperren, Passwort, Gruppen, SSH-Schlüssel]
---
# Benutzer ändern

Die Auswahl listet, was sich für den markierten Benutzer ändern lässt. {up} und {down} wählen, {search} filtert, {enter} trifft die Wahl, {back} schließt die Auswahl.

- Zum Administrator machen oder Administratorrechte nehmen: Administrator mit sudo und Zugang zu qhost. Du kannst sie dir nicht selbst nehmen, und der letzte Administrator bleibt einer.
- Zugang zu Docker geben oder nehmen, oder Zugang zu qhost für einen Administrator ohne ihn.
- Sperren oder Entsperren: Ein gesperrter Benutzer kann sich weder mit Passwort noch mit SSH-Schlüssel anmelden. Laufende Sitzungen bleiben offen.
- Zu Gruppen hinzufügen, Aus Gruppen entfernen, Neues Passwort setzen, SSH-Schlüssel hinzufügen und SSH-Schlüssel entfernen öffnen ein Formular. Danach folgt die Rückfrage mit den Befehlen.

In den Formularen wählt {toggle} eine Gruppe, {left} und {right} gehen durch eine Auswahl, {next} und {prev} wechseln zwischen den Feldern, {save} führt zur Rückfrage und {back} verwirft das Formular.

- Neues Passwort: zweimal getippt und verborgen. Die Auswahl Beim Login ändern lässt den Benutzer beim nächsten Login ein neues wählen.
- SSH-Schlüssel hinzufügen: genau einen öffentlichen Schlüssel einfügen.
- SSH-Schlüssel entfernen: den Schlüssel am Fingerprint wählen.
