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

- Miete: Je vermieteter Wohnung erfasst du die monatliche Miete in der Fixkosten-Eingabe (Pflichtfeld, 0 erlaubt). Sie erscheint als Mieteinnahmen auf der Jahreskarte (nur für angemeldete Nutzer) und auf der Detailseite der Eingabe. Sie geht in keine Berechnung, keinen Saldo und keine Abrechnung ein (#277, #278, #281)
- Die Fixkosten-Eingabe erfasst den Nebenkostenabschlag nur noch für vermietete Wohnungen (Pflichtfeld, 0 erlaubt), bei Eigennutzung gibt es kein Feld mehr. Bereits gespeicherte Abschläge zählen weiter (#277)
- Der CSV-Export der Fixkosten enthält jetzt `miete_1` und `miete_2`, ältere Dateien bleiben importierbar (#277)

## Verbesserungen

- Die Jahreskarte einer vermieteten Wohnung zeigt unter Guthaben / Nachzahlung die Mieteinnahmen des Jahres. Sie erscheinen nur für angemeldete Nutzer, die Widgets zeigen sie nicht (#278)
- Die Detailseite einer Fixkosten-Eingabe zeigt die Miete vermieteter Wohnungen neben dem Nebenkostenabschlag, auch ohne Login (#281)
- Jahreskarten: PV-Gutschrift (Wohnung 1) und PV-Anteil (Wohnung 2) stehen jetzt auch mit 0. Die Wallbox-Zeile einer vermieteten Wohnung entfällt, wenn "Wallbox weiterberechnen" aus ist und der Betrag 0 ist (#276)
- Strom von Wohnung 1: Der Rest des Netzbezugs nach Wohnung 2, Wärmepumpe und Wallboxen steht jetzt als Kategorie "Strom" in Jahreskarte, Monatsverlauf, Verbrauchswerten, Gesamt, Saldo und Widgets (#279)
- Jahresabrechnung: Der Strom einer Wohnung steht bei Eigennutzung immer in der Abrechnung, bei Vermietung nur mit "Stromverbrauch weiterberechnen" (jetzt für Wohnung 1 und 2). Der Jahressaldo zählt ihn mit und gleicht damit wieder der Änderung im Dashboard (#280)
- Die Ablesungsseite zeigt bei den Stromkosten jetzt auch die Zeile "Wohnung 1" (Anteil und Kosten) (#275)

## Fehlerkorrekturen

## Wartung

- Neue Datenbanktabelle für die Miete (leer angelegt, bestehende Daten bleiben erhalten). Vor dem Start mit dem neuen Schema sichert die App die Datenbank wie gewohnt (#277)
- Die Salden von Wohnung 1 sinken rückwirkend um die bisher aufgelaufenen Stromkosten, weil der Strom von Wohnung 1 jetzt mitgerechnet wird (#279)
