# Fixkosten-Regel: Auswirkung auf Demo- und Testdaten (#377, Karte #371)

Stand: `main` bei `ec09f9c`. Methode: `internal/store/demoseed.go` gelesen, auf der Demodatenbank (`SeedDemoData` mit now = 2026-10-15, wie im Golden-Test) `abr.Pruefe`, `fixkostenFehlenMonate` und `store.BetroffeneAblesungen` per Wegwerf-Test ausgeführt (nicht eingecheckt, Ausgabe unten).

## Befund in Kürze

Die Demodaten enthalten keinen Fall, in dem die neue Regel anders meldet als heute. `kosten_demo.golden` bleibt unverändert, solange die Texte des bestehenden Mangels "Fixkosten fehlen" gleich bleiben.

## Frage 1: Gibt es abweichende Fälle in den Demodaten?

Nein.

- 39 Ablesungen (2023-08 bis 2026-10, `demoseed.go:123` `seedDemoPeriods`, `monthCount = 39` Zeile 137) und 39 Fixkosten-Eingaben für dieselben Monate (`seedDemoFixkosten` ab Zeile 269, `monthCount` Zeile 279). Jede Eingabe setzt alle vier Preise (`demoseed.go:289-292`).
- Keine unvollständige Eingabe, kein Monat mit mehr als einer Eingabe, kein Teilstand (alle Zähler und der Abrechnungsmonat sind in jeder Ablesung gesetzt, `demoseed.go:124-135, 181-185`).
- Die erste Ablesung (2023-08) hat eine vollständige Eingabe, ist also weder "fehlt" noch "unvollständig".
- Monate ohne Eingabe: nur 2026-11 und 2026-12, dort fehlt auch die Ablesung. Beide sind heute schon `fixkosten_fehlt` (`pruefung.go:229`) und bleiben es (keine Eingabe = nicht vollständig).

Ausgabe des Wegwerf-Tests:

```
periods 39 2023-08-01 2026-10-01 eingaben 39
unvollstaendige Eingaben: 0
teilstaende: 0
fixkostenFehlenMonate(alle Ablesungsmonate): []
BetroffeneAblesungen: 2023-08-01 -> 0 (erste Ablesung), jeder andere Monat -> genau 1
Pruefe 2023, 2024, 2025: keine Maengel
Pruefe 2026: ablesung_fehlt 2026-11, 2026-12; fixkosten_fehlt 2026-11, 2026-12
fixkostenFehlenMonate(2026-11, 2026-12) = [2026-11-01 2026-12-01]
BetroffeneAblesungen(2026-11) = 0
```

Zur Illustration (nicht in den Demodaten; Wegwerf-Variante mit `strompreis = NULL` in 2025-03): `fixkostenFehlenMonate` meldet 2025-03, `BetroffeneAblesungen(2025-03)` liefert 0, `Pruefe(2025)` meldet 0 Mängel. Das ist die heutige Lücke, die "Fixkosten unvollständig" schließt. Ursache: `eingabenJeMonat` zählt nur Eingaben (`pruefung.go:213-229`), und `abr.Daten.Eingaben` ist `[]store.FixkostenEingabeSummary` mit nur `ID` und `Monat` (`store/fixkosten.go:473-476`). Die Prüfung kennt die Preise nicht, `Daten` braucht also ein Vollständig-Flag (Hinweis für die Spec).

## Frage 2: Welche Teile von `kosten_demo.golden` ändern sich?

Keine. Berührt wäre nur der Prüfungsblock `Maengel`: 2026 Wohnung 1 und 2 (Zeilen 6251-6324) haben je 4 Mängel (zweimal `ablesung_fehlt`, zweimal `fixkosten_fehlt`, 2026-11 und 2026-12). 2023-2025 haben `Maengel: [0]`, 2022 hat `kein_zeitraum` (Zeilen 1-22).

- Der neue Mangel tritt in den Demodaten nicht auf, die 4 Mängel bleiben bytegleich.
- Abrechnungsbeträge, Dashboard und Fixkosten-Liste hängen an `calc`, dessen Regel (`calc/verbrauchskosten.go:173-176`, `!e.PreiseVollstaendig()`) schon die Zielregel ist.
- Vorbehalt: Ändert die Spec Text oder Aktion des bestehenden `fixkosten_fehlt`, ändern sich genau die Zeilen 6276-6286 und 6313-6323. Der Beschluss der Karte betrifft nur den neuen Mangel.

## Frage 3: Welche Tests prüfen das heutige Verhalten?

`MangelFixkostenFehlt`:
- `internal/abrechnung/pruefung_test.go:153` (Arten und Text "Fixkosten-Eingabe fehlt: November 2027", Pfad `/fixkosten/neu`). Die Eingaben dort sind `FixkostenEingabeSummary{ID, Monat}` ohne Preise: Bekommt die Prüfung ein Vollständig-Flag, muss dieser Test es setzen.
- `internal/web/abrechnung_test.go:819` (`TestPruefeAbrechnung_Teilzeitraum`, erwartet genau einen `MangelFixkostenFehlt`).
- Golden: `internal/web/golden_kosten_test.go` (`TestKostenDemodatenGolden`).

`fixkostenFehlenMonate` (kein eigener Test, nur über die Aufrufer `ablesung_gespeichert.go:26,36` und `csv.go:323`):
- `internal/web/ablesung_fixkosten_ansicht_test.go`: `TestAblesungsliste_BadgeFixkostenFehlen`, `TestAblesungsDetail_FixkostenFehlen`.
- `internal/web/fixkosten_fehlen_test.go`: `TestAnsichten_AblesungenOhneFixkostenEingabe`.
- Dieselbe Regel über `calc.FixkostenOffen`: `internal/web/dashboard_fixkosten_fehlen_test.go` (`TestDashboard_BannerBeiOffenenMonaten`, `..._OhneOffeneMonateUnveraendert`, `..._VerlaufZeileOffenerMonat`, `..._JahreskarteVorlaeufigBeiOffenemMonat`, `..._JahreskarteNurOffeneMonateDesAnzeigejahres`).

`BetroffeneAblesungen`:
- `internal/store/fixkosten_preise_test.go`: `TestBetroffeneAblesungen` (246), `TestBetroffeneAblesungen_NurMitVollstaendigerEingabe` (284). Sie legen die heutige Semantik fest: unvollständige Eingabe liefert leere Liste (`store/fixkosten.go:690-703`), erste Ablesung und Teilstand zählen nicht (`fixkosten.go:709`).
- `internal/web/fixkosten_hinweis_test.go`: `TestFixkostenDetail_LoeschenBestaetigungNenntBetroffeneAblesungen`, `..._OhneBetroffeneAblesungenKeineZusatzBestaetigung`, `TestFixkostenLoeschen_UnvollstaendigeEingabeKeineBestaetigungKeinBanner`, `..._BannerMitLinkNurFuerVermieter`, `..._OhneBetroffeneKeinBanner`, `..._BannerEntfaelltWennMonatWiederErfasst`, `TestFixkostenAendern_MonatswechselHinweisAltInfoNeu`, `..._OhneMonatswechselKeinHinweis`.

Zielregel schon in `calc`: `internal/calc/verbrauchskosten_test.go:76-81` (je ein fehlender Preis), `internal/calc/datenstand_test.go:48`.

## Unsicherheiten

- Die Demo-Ablesungen sind alle vollständig und haben Eingaben. Teilstand und erste Ablesung unter der neuen Regel belegen nur die Unit-Tests, nicht die Demo.
- Nicht geprüft: was `UPDATE_GOLDEN=1` nach der Implementierung wirklich ändert. Die Aussage gilt für die Regel, nicht für den fertigen Code.
- Der CSV-Import-Hinweis (`csv.go:323`) ist keinem Test namentlich zugeordnet, die Zuordnung der oben genannten Tests zu `fixkostenFehlenMonate` beruht auf Aufrufern und Namen, nicht auf einer Ausführung.
- Zeilennummern gelten für `ec09f9c`.
