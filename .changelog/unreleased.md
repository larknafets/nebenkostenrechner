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

- Dashboard: Die Wärmepumpe hat einen eigenen Monatsverlauf wie die Wallboxen, auch ohne Anmeldung sichtbar (#305)

- Dashboard: Die Jahreskarte jeder Wohnung zeigt die Wallbox in einem eigenen Block mit Stromkosten in EUR und kWh, statt im Hauptbalken (#304)

- Dashboard: neue Jahreskarte "Wärmepumpe" mit Stromkosten und PV-Anteil, auch ohne Anmeldung sichtbar (#296)

## Verbesserungen

- Dashboard: Die Monatsübersicht wählst du jetzt in einem Dropdown statt über Buttons, bei nur einer Auswahl entfällt es (#306)

- Dashboard: Der Balken der Jahreskarte zeigt nur noch den Verbrauch, die Fixkosten stehen weiter als Zeile und im Gesamtbetrag (#302)

- Dashboard: Die Karte "Wallboxen" erscheint nur, wenn eine Wohnung die Wallbox in der Abrechnung trägt (Eigennutzung oder weiterberechnet) (#297)

## Fehlerkorrekturen

## Wartung

- Die Widget-Routen und der zweite Port (`WIDGET_LISTEN_ADDR`, Host-Port 8081 beim Release, 8082 beim Nightly) entfallen, Lovelace-Karten mit Webpage-Card auf diese Routen funktionieren nicht mehr (#301)
