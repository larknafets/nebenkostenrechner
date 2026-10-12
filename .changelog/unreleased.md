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

Keep every section, an empty one included. Do not change the headings.
-->

## Neue Funktionen

- Ablesung: Unter "Wallboxen" lassen sich die Zählerstände von Wallbox Wohnung 1 und Wallbox Wohnung 2 optional eintragen (nur nachrichtlich, auch nur einer von beiden). Leere Felder gelten als "nicht erfasst" und machen die Ablesung nicht unvollständig; die Ablesungsseite zeigt sie mit Verbrauch, CSV-Export und -Import führen sie als zwei neue letzte Spalten (ältere Dateien bleiben importierbar). Im Formular steht die Wärmepumpe jetzt vor den Wallboxen (#224)
- Ablesung: Neuer Schalter "Wallboxen vollständig Wohnung 1" (aus der letzten Ablesung vorbelegt). Damit liegen alle Wallbox-Kosten bei Wohnung 1; ohne Schalter zahlt Wohnung 2 ihren Verbrauch laut Einzelzähler (gedeckelt auf den Netzbezug-Anteil) und Wohnung 1 den Rest. Die Ablesungsseite zeigt die Kosten je Wohnung und einen Hinweis, wenn die Einzelzähler um mehr als 5 % oder 2 kWh vom Zwischenzähler abweichen; der CSV-Export hat dafür eine neue letzte Spalte (#225)
- Ablesung: Neuer Schalter "PV-Gutschrift Wohnung 1" (Formular Schritt 4 beim Einspeisepreis, aus der letzten Ablesung vorbelegt). Ist er an, geht die Einspeisevergütung der Ablesung als Gutschrift an Wohnung 1 und erhöht deren Guthaben im Saldo ohne Deckel; ohne Schalter bleibt die Einspeisung haus-weit und informativ. Die Ablesungsseite zeigt den Schalter als Ja/Nein, der CSV-Export hat dafür eine neue letzte Spalte (#226)
- Dashboard und Widgets: Neue Kategorie "Wallbox" bei Wohnung 1 und 2 (Jahreskarte, Monatsverlauf, Saldo, immer sichtbar), in "Verbrauchswerte" mit den zugeordneten Netzbezug-kWh. Wohnung 1 zeigt die PV-Gutschrift als negative Zeile: Sie senkt die Gesamtkosten und erhöht das Guthaben im Saldo, steht im Monatsverlauf aber nur in "Kombiniert". Die Karten "Wallboxen" und "PV-Anlage" bleiben unverändert (#227)

## Verbesserungen

## Fehlerkorrekturen

## Wartung

- Beim ersten Start nach dem Update legt die Datenbank automatisch zwei neue Zähler für die Wallbox-Einzelzähler an; bestehende Ablesungen, Kosten und Salden bleiben unverändert (#224)
- Beim ersten Start nach dem Update ergänzt die Datenbank eine Spalte für den Schalter "Wallboxen vollständig Wohnung 1"; bestehende Ablesungen bekommen "Nein", ihre Kosten und Salden ändern sich nicht (#225)
- Beim ersten Start nach dem Update ergänzt die Datenbank eine Spalte für den Schalter "PV-Gutschrift Wohnung 1"; bestehende Ablesungen bekommen "Nein", ihre Kosten und Salden ändern sich nicht (#226)
