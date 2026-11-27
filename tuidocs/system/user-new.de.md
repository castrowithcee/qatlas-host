---
title: Neuer Benutzer
keywords: [neuer Benutzer, anlegen, Startpasswort, SSH-Schlüssel]
---
# Neuer Benutzer

Das Formular legt ein lokales Konto an. Der Benutzer muss das Startpasswort beim ersten Login ändern.

- Name: Kleinbuchstaben, Ziffern, _ und -, höchstens 32. Der Name muss frei sein und darf nicht root sein.
- Startpasswort und Wiederholen: bei der Eingabe verborgen, nie angezeigt und nie Teil einer Befehlszeile. Beide müssen übereinstimmen.
- Admin: Administrator mit sudo und Zugang zu qhost.
- Docker: Zugang zu Docker, und das ist praktisch root.
- Weitere Gruppen: {toggle} wählt eine Gruppe, auch mehrere auf einmal.
- SSH-Schlüssel: optional, ein öffentlicher Schlüssel je Zeile im Format von authorized_keys, danach nicht angezeigt.

{next} und {prev} wechseln zwischen den Feldern, {left} und {right} gehen durch eine Auswahl. {save} prüft die Eingaben und führt zur Rückfrage mit den Befehlen. Eine falsche Eingabe bleibt mit ihrem Grund im Formular. {back} verwirft das Formular.
