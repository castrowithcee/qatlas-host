---
title: Versionen übernehmen
keywords: [Übernahme, übernehmen, Versionen, Quell-Stack, Images]
---
# Versionen übernehmen

{promote} setzt die Image-Versionen dieses Stacks auf die, die ein anderer Stack betreibt. Es öffnet eine Liste der anderen Stacks des Hosts, mit der Übernahme-Quelle der Update-Regeln zuerst markiert, wenn eine gesetzt ist. Die Wahl eines Stacks liest ihn und holt beim Dienst eine Vorschau. Hat dieser Stack für die gemeinsamen Images schon die Versionen, sagt eine Meldung, dass nichts zu übernehmen ist.

Sonst zeigt eine Frage den Quell-Stack mit seinem Commit, ob die Quelle noch unter Beobachtung steht, ob die Fingerabdrücke der Images verglichen werden und jede Variable mit alter und neuer Version.

- {yes} stoppt den Stack, sichert ihn, wenn er Backup-Regeln hat, schreibt die neuen Versionen in einem Commit in seine .env, der gepusht wird, und startet und prüft den Stack. Ein fehlschlagender Lauf wird zurückgenommen. Gesetzt werden nur Images, die beide Stacks betreiben. Der Dienst verweigert, wenn die Quelle seit der Vorschau einen anderen Commit ausgerollt hat.
- {no} oder {enter} bricht ab und ändert nichts.
- In der Liste wählt {enter}, {back} verlässt sie ohne Änderung.
