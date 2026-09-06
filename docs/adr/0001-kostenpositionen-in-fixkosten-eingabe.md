# 1. Kostenpositionen-Verwaltung in der Fixkosten-Eingabe statt in Stammdaten

## Status

Angenommen (2026-09-06)

## Kontext

Bis Issue #105 lag Logik/Typ/Jahreswert jeder der 14 Kostenpositionen jahresweise in einer eigenen Stammdaten-Struktur (Tabelle `kostenpositionen_jahre`, Seite `/stammdaten`, Block "Kostenpositionen je Jahr" mit "+ Jahr anlegen"/"Jahr löschen"). Eine Fixkosten-Eingabe verwies nur auf das jeweilige Kalenderjahr; nur monatlich-typisierte Positionen hatten dort einen eigenen, editierbaren Wert. Jährlich-typisierte Positionen waren im Fixkosten-Formular als deaktiviertes Feld sichtbar ("Jahreswert · Stammdaten"), Änderungen wirkten sofort rückwirkend auf alle bereits erfassten Monate des betroffenen Jahres.

Das widersprach der Historisierung, die jedes andere Feld einer Fixkosten-Eingabe (Personen, Nebenkostenabschlag, monatlich-typisierte Werte) längst hatte: dort trägt jede Eingabe ihren eigenen, unabhängigen Stand, vom Vormonat vorbelegt, frei überschreibbar, ohne Rückwirkung auf andere Monate. Für Logik/Typ/Jahreswert existierte dagegen ein zweiter, abweichender Pflegeweg mit anderer Semantik (sofortige Rückwirkung statt Historisierung) und einer eigenen Seite.

## Entscheidung

Logik/Typ/Wert jeder Kostenposition werden vollständig in die Fixkosten-Eingabe verlagert - genau wie Personen und Nebenkostenabschlag: pro Eingabe unabhängig gespeichert (Tabelle `fixkosten_werte`, um die Spalten `logik`/`typ` erweitert), vom Vormonat vorbelegt, frei überschreibbar, keine automatische Rückwirkung auf andere Monate. Korrekturen für vergangene Monate laufen wie bei jedem anderen Feld über "Eingabe korrigieren".

Die Typ-Unterscheidung (jährlich/monatlich) bleibt bestehen: bei "jährlich" ist der erfasste Wert ein Jahresgesamtbetrag (für die Berechnung durch 12 geteilt), bei "monatlich" ein direkter Monatsbetrag - nur der Pflegeort ändert sich.

Der Stammdaten-Jahresblock (Tabelle `kostenpositionen_jahre`, Seite, Routen, Store-Funktionen) entfällt ersatzlos. Bestehende Fixkosten-Eingaben wurden einmalig automatisch beim App-Start mit den zu ihrem jeweiligen Monat gültigen Werten aus `kostenpositionen_jahre` befüllt, bevor die Tabelle gedroppt wurde (siehe `ensureFixkostenWerteLogikTypColumns`/`dropKostenpositionenJahreTable` in `internal/store/store.go`).

Stammdaten (`/stammdaten`) bleibt als Seite bestehen, aber nur noch für Wohnungsgröße/Flurstücksgröße je Wohnung - Werte, die sich selten ändern, nicht Teil einer monatlichen Erfassung sind und bewusst sofort für alle Monate wirken sollen (kein Historisierungsbedarf).

## Konsequenzen

- Ein einziger Pflegeweg und ein einziges Historisierungsmodell für alle Felder einer Fixkosten-Eingabe, keine Sonderbehandlung für jährlich-typisierte Kostenpositionen mehr.
- Eine rückwirkende Korrektur, die mehrere Monate betrifft (z.B. ein systemisch falscher Jahreswert), erfordert jetzt das einzelne Bearbeiten jeder betroffenen Eingabe statt einer einzigen zentralen Korrektur in Stammdaten. Das ist eine bewusst in Kauf genommene Abwägung zugunsten von Konsistenz mit dem restlichen Datenmodell.
- `store.LatestJaehrlichWert` (Fallback auf den letzten bekannten Jahreswert bei fehlendem Monatswert) entfällt - jede Eingabe hat durch die Vorbelegung vom Vormonat immer einen Wert, ein Fallback-Mechanismus ist nicht mehr nötig.
- Eine künftige Session sollte keinen neuen jahresweisen Stammdaten-Block für Kostenpositionen vorschlagen - das war der vorherige Zustand und wurde bewusst verworfen.
