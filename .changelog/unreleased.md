<!--
Release notes for the next release. Add a line in the SAME commit that makes the
change. At a release the line moves into CHANGELOG.md and this file is emptied
again (docs/agents/releases.md).

What goes in: a change a user or the operator of the Home Assistant app can see
or feel. Not tests, CI, scripts, docs, lint fixes.
Reverted before the release? Delete the line. A bug that arose after the last
release gets no line, because no user has seen it. To tell the two apart, run
`git tag --contains <sha>` on the commit that caused the bug: an empty result
means no release carries it.

How to write it: one line, German, in the words of the user interface. Name what
changed for the user, not how it was built. Add "(#123)" when an issue drove the
change. Under "Fehlerkorrekturen" write what works now and name what went wrong
before, so a reader knows the bug.

Sections:
- Neue Funktionen: something that did not exist before.
- Verbesserungen: something existing gets better or more convenient.
- Fehlerkorrekturen: something went wrong and works now.
- Wartung: what the operator must know when updating: Go version, base image,
  environment variables, ports, data paths, migrations, security updates. The
  dependency updates of Dependabot are ONE line, added at the release, not one
  per update. The same for refactorings: if the release has any, ONE line
  "Interne Aufräumarbeiten am Code ohne sichtbare Änderung", added at the
  release, never one line per refactoring.

Optional first section "Vor dem Update": ONE short notice for the top of the
release notes, written as plain text starting with "**Vor dem Update:**" (the
release script turns it into a quote block). Only when a migration cannot be
undone (no way back to the old version) or when numbers of months already
recorded change (balance, settlement, calculation). Say what happens and what
changes, not what the user must do. Leave it empty otherwise, the Wartung line
stays as the detail.

Keep every section, an empty one included. Do not change the headings.
-->

## Vor dem Update

## Neue Funktionen

- Neue Seite "Sonderkosten": Rechnungen erfassen, korrigieren und löschen, nach Abrechnungsmonat gruppiert mit Jahressummen sowie Art- und Jahresfilter, als CSV exportieren und importieren. Umlegbare Sonderkosten gehen in Monatssumme, Saldo, Dashboard und Jahresabrechnung ein (Zeile "Sonderkosten" in der Kostenübersicht und neue Anlage 8), nicht umlegbare trägt die Vermieter:in und sie ändern keine Abrechnung. Sichtbar mit den neuen Rechten "Sonderkosten ansehen" und "Sonderkosten erfassen", standardmäßig nur für Vermieter:in (#411, #408, #404, #405, #406, #409, #407, #410).

## Verbesserungen

- Die Zählerliste einer Ablesung zeigt den Rest von Wohnung 1 für Strom (wenn Netzbezug übrig bleibt) und Wasser.
- Löschen von Ablesung und Fixkosteneingabe fragt jetzt in einem eigenen Dialog nach statt im Browser-Fenster (#401).

## Fehlerkorrekturen

- Das Dashboard zeigt das Jahr des Abrechnungsmonats: Bei einer Ablesung vom 2. Januar für den Abrechnungsmonat Dezember zeigte es ein Jahr ohne Monat mit Daten und leere Jahreskarten (#433).

## Wartung

- Schema-Version 6: Die neue Tabelle `sonderkosten` entsteht beim Start, bestehende Daten ändern sich nicht. Die App legt beim ersten Start mit der neuen Version eine Sicherung der Datenbank an (#403).
