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

**Vor dem Update:** Beim ersten Start baut die App die Datenbank um (bei den Sonderkosten heißt die Spalte `umsetzung` danach `objekt`, dazu kommt `rechnungsdatum`). Eine ältere Version der App kann diese Datenbank danach nicht mehr lesen, der Weg zurück führt nur über die Sicherung, die die App vor dem Umbau anlegt.

## Neue Funktionen

- Die Seite Sonderkosten lässt sich jetzt nach Objekt (Haus, Wohnung 1, Wohnung 2) filtern, zeigt die Summenkarten und eine Tabelle der Rechnungsbeträge nach Art und Objekt in einem Block "Summen", der zuerst eingeklappt ist, zeigt auf jeder Karte oben Objekt und Art und nimmt ein optionales Rechnungsdatum auf (#453, #454, #455, #456, #457). Der Filter wirkt auf Karten, Monatsüberschriften, Summenkarten und Tabelle. Die Jahresabrechnung bleibt unberührt, nur die Spalte "Objekt" in der Anlage 8 heißt jetzt so (vorher "Umsetzung")
- Auf der Seite Sonderkosten gibt es neben Jahr und Art einen Filter "Objekt" (Alle Objekte, Haus, Wohnung 1, Wohnung 2). Er wirkt auf die Karten, die Monatsüberschriften und die drei Summenkarten (#455)
- Sonderkosten haben ein optionales Rechnungsdatum: Es lässt sich beim Anlegen und Korrigieren eintragen oder löschen, steht auf der Karte in der Zeile mit Firma und Rechnungsnummer (TT.MM.JJJJ) und im CSV-Export als letzte Spalte `rechnungsdatum`. Beim Import ist die Spalte optional. Abrechnungsmonat, Reihenfolge, Summen und die Anlage der Jahresabrechnung bleiben unverändert (#454)
- Auf der Seite Sonderkosten steht unter den drei Summenkarten eine Tabelle der Rechnungsbeträge nach Art und Objekt (Haus, Wohnung 1, Wohnung 2), bei "Alle Jahre" ein Block je Jahr mit Zwischensumme. Sie folgt dem Filter und zeigt nur Arten mit Einträgen. Summenkarten und Tabelle stehen in einem Block "Summen", der zuerst eingeklappt ist (#456)

## Verbesserungen

- Auf den Sonderkosten-Karten stehen über dem Betreff Objekt und Art als Markierungen, in der Betreff-Zeile bleiben nur Betreff und Betrag (#457)

## Fehlerkorrekturen

## Wartung

- Bei den Sonderkosten heißt "Umsetzung" jetzt "Objekt" (Haus, Wohnung 1, Wohnung 2), auch in der Anlage der Jahresabrechnung. Das Update benennt die Spalte `umsetzung` der Datenbank in `objekt` um (Schema-Version 7, vorher entsteht eine Sicherung) und der CSV-Export schreibt die Kopfzeile `objekt`. Der Import nimmt weiter auch alte Dateien mit `umsetzung` an, eigene Auswertungen auf die Datenbank oder den Export brauchen den neuen Namen (#453)
