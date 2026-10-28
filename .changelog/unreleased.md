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

## Verbesserungen

- Geschlechtergerechte Sprache: Rollen und Felder heißen jetzt Mieter:in/Mieter:innen und Vermieter:in/Vermieter:innen (Anmeldung, Stammdaten, Berechtigungen, Jahresabrechnung). Die Kennwort-Variablen `VERMIETER_PASSWORD` und `MIETER_PASSWORD` bleiben unverändert.

- Dashboard: Die Badges für Fläche, Flurstück und Personen bei den Jahressummen sind kleiner und passen häufiger in eine Zeile.

- Personenzahl, Preise und die Häkchen "Wallboxen vollständig Wohnung 1" und "PV-Gutschrift Wohnung 1" werden nur noch bei der Fixkosten-Eingabe des Monats erfasst, der Ablesungs-Wizard hat nur noch drei Schritte (Strom, Wärme, Wasser). Gibt es für den Monat keine Fixkosten-Eingabe, zeigen Dashboard, Monatsverlauf, Ablesungen und Jahreskarte "Fixkosten fehlen" statt falscher Zahlen. Das Fixkosten-CSV hat sechs neue Spalten für Preise und Häkchen, das Ablesungs-CSV nimmt ältere Dateien weiter an (#359)

## Fehlerkorrekturen

## Wartung

- Update: Preise und Häkchen wandern beim ersten Start aus den Ablesungen in die Fixkosten-Eingaben desselben Monats, die Berechnung alter Monate kann sich dadurch ändern (Abweichungen stehen im Log). Monate ohne Fixkosten-Eingabe zeigen "Fixkosten fehlen", Ablesungen ohne Preise gelten als vollständig. Die Spalten für Preise, Häkchen und Personenzahl der Ablesung und die Tabelle der Ablesungs-Personenzahlen entfallen. Die Sicherung `.vor-schema-<alte Version>-<UTC-Zeit>` entsteht vor dem Start (#359)
