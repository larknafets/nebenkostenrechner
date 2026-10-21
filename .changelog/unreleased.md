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

- Fixkosten: Neue Position "Wartungskosten PV-Anlage" direkt unter "Wartungskosten Wärmepumpe", auf der Fixkosten-Eingabe und in den Stammdaten. Vorbelegt mit Berechnungslogik "Wohnung 1", Typ "jährlich" und nicht umlagefähig. Bestehende Eingaben erhalten die Position automatisch mit dem Wert 0. Ein älterer Fixkosten-CSV-Export lässt sich nicht mehr importieren, weil die Spalten der neuen Position fehlen

## Verbesserungen

- Stammdaten: Die Reihenfolge der Kostenpositionen hat eine übersichtlichere Darstellung: nummerierte Liste mit Trennlinien und Pfeil-Symbolen statt einfacher Textpfeile, mit sichtbarem Fokus für die Tastaturbedienung (#255)

- Stammdaten: Die Reihenfolge der Kostenpositionen lässt sich mit Hoch/Runter-Buttons ändern. Sie gilt für Fixkosten-Eingabe, Fixkosten-Anzeige und Abrechnung. Neue Positionen erscheinen künftig am Ende der Liste

## Fehlerkorrekturen

- Ablesung: Die Warnung "Verbrauch über 1 Monate" erschien schon bei genau einem Monat Abstand zur letzten Ablesung (Zeitzonenfehler im Browser). Jetzt warnt sie erst bei mehr als einem Monat, nennt "1 Monat" in der Einzahl und zeigt das Datum der letzten Ablesung als 01.09.2026 (#257)

- Ablesung korrigieren: Zählerwerte und Preise erschienen in der Vorbelegung mit Gleitkommaresten (z. B. 26520,69999999999 statt 26520,7). Jetzt zeigen die Felder die eingegebenen Werte ohne diese Reste (#254)

- Dashboard: Der Update-Hinweis (Punkt neben der Version unten rechts) erschien im Nightly-Kanal nie, weil nur Release-Versionen verglichen wurden. Jetzt zeigt ein Nightly-Stand den Punkt, sobald ein neueres Nightly vorliegt, der Link führt zum zugehörigen Commit. Nightlys vom selben Tag lassen sich nicht unterscheiden und lösen keinen Hinweis aus

- Eingaben: Eine Fehlermeldung (z. B. "Für September 2026 gibt es bereits eine Fixkosten-Eingabe") führte auf eine leere Fehlerseite, man musste zurückgehen und die Eingaben waren weg. Jetzt erscheint sie als Dialog über der Seite, die Eingaben bleiben stehen, das betroffene Feld ist rot markiert und nach dem Schließen aktiv. Gilt für Fixkosten, Ablesung, Stammdaten und die beiden CSV-Importe

## Wartung
