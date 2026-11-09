# Sonderkosten-Zuordnung: Ist-Stand

Research zu Ticket #465 (Map #464). Stand: `main` bei Commit dc9065f. Alle Aussagen sind am Code geprüft, nicht dem Review-Bericht entnommen. Es wurde kein Produktcode geändert.

## Antwort in Kürze

Die Regel "wer trägt den Eintrag, und ist er berechenbar" hat genau einen Ort für die Berechnung (`calc.SonderkostenAnteile`) und einen Ort für das Prädikat (`Sonderkosten.BrauchtFixkosteneingabe`). Dashboard, Anlage 8 und Liste unterscheiden sich in keiner gültig speicherbaren Kombination. Die einzige Abweichung betrifft einen Altdatensatz, den der Store heute nicht mehr zulässt (Haus, umlegbar, Logik leer). Wirklich doppelt stehen nur zwei kleine Dinge: der Ausgabeteil von `calc.SonderkostenMonate` und `abrechnung.sonderkostenAnlage` (Filter und Logik-Schlüssel, byte-gleich), und die zweite Herleitung von "berechenbar" für das Badge der Liste. Die Review-Behauptung "an sechs Stellen verstreut" stimmt nicht.

## Methode

- Code gelesen: `internal/calc/sonderkosten.go`, `internal/abrechnung/berechnung.go`, `internal/web/sonderkosten.go`, `internal/web/sonderkosten_liste.go`, `internal/web/fixkosten_hinweis.go`, `internal/store/sonderkosten.go`, `internal/store/schema.sql`.
- Tickets gelesen: Maps #416 und #421, Entscheidungen #419, #420, #428, `GLOSSARY.md` (Eintrag "Zuordnung (Sonderkosten)"), ADR 0005.
- Alle 60 Kombinationen (3 Objekte x umlegbar ja/nein x 5 Logik-Werte x Fixkosteneingabe im Monat ja/nein) wurden mit zwei Wegwerf-Tests ausgeführt (Betrag 100 EUR, QM 100/50, Flurstück 300/100, Personen 3/1) und die Tests danach gelöscht. Verglichen wurden `calc.SonderkostenMonate` (Dashboard und Abrechnungs-Monatswerte), `abrechnung.sonderkostenAnlage` (Anlage 8) und `baueSonderkostenListe` (Liste).

## Welche der sieben genannten Stellen sind Duplikation

Das Ticket nennt sieben Namen, der Review sprach von sechs Stellen.

| Stelle | Datei:Zeile | Urteil |
|---|---|---|
| `calc.SonderkostenAnteile` | `internal/calc/sonderkosten.go:27` | Der eine Ort der Wirkung, keine Duplikation. |
| `calc.SonderkostenMonate` | `internal/calc/sonderkosten.go:66` | Leser (Dashboard und Abrechnungs-Monatswerte), doppelt mit der Anlage (siehe unten). |
| `abrechnung.sonderkostenAnlage` | `internal/abrechnung/berechnung.go:51` | Leser, doppelt mit `SonderkostenMonate` (siehe unten). |
| `web.sonderkostenZuordnung` | `internal/web/sonderkosten.go:59` | Nur Anzeigetext. Dieselbe Fallunterscheidung (nicht umlegbar, Wohnung 1, Wohnung 2, sonst Logik) wie in calc, aber andere Ausgabe ("trägt ..." statt Schlüssel). Keine Regel zu "berechenbar". Teilweise. |
| `NichtBerechenbar` | `internal/web/sonderkosten_liste.go:85` | Zweite Herleitung von "berechenbar" (siehe unten). Echte, kleine Duplikation. |
| Zähler in `fixkostenWirkungVon` | `internal/web/fixkosten_hinweis.go:165-172` | Schon entschieden (#420, #428): benutzt nur das Prädikat `BrauchtFixkosteneingabe`, zählt "braucht" statt "berechenbar". Keine Duplikation. |
| `store.validiereSonderkosten` | `internal/store/sonderkosten.go:173` | Schon entschieden (#419, #420): Zeile 200 benutzt das Prädikat. Der Rest der Funktion ist eine andere Regel (Logik nur bei Haus und umlegbar, Zeilen 195-205), kein zweites "berechenbar". Keine Duplikation. |

Das Prädikat `BrauchtFixkosteneingabe` (`internal/store/sonderkosten.go:69`) hat vier Aufrufer: `store/sonderkosten.go:200`, `calc/sonderkosten.go:38`, `web/sonderkosten_liste.go:85`, `web/fixkosten_hinweis.go:168`. Das ist der in #420 entschiedene Zustand.

## Kombinationen und heutiges Ergebnis je Leser

Betrag 100 EUR. "Dashboard" ist `calc.SonderkostenMonate`, "Anlage 8" ist `sonderkostenAnlage` mit dem Monat im Bereich. Anteile als Wohnung 1 / Wohnung 2, "-" heißt: der Eintrag zählt für diese Wohnung nicht.

| Objekt | umlegbar | Logik | Fixkosteneingabe im Monat | Dashboard | Anlage 8 | Liste |
|---|---|---|---|---|---|---|
| Haus | ja | Je Wohneinheit | egal | 50 / 50 | 50 / 50 | "Je Wohneinheit", kein Badge |
| Haus | ja | Je anteiliges Flurstück | egal | 75 / 25 | 75 / 25 | "Je anteiliges Flurstück", kein Badge |
| Haus | ja | Je anteilige Wohnungsgröße | egal | 66,67 / 33,33 | 66,67 / 33,33 | "Je anteilige Wohnungsgröße", kein Badge |
| Haus | ja | Je Anzahl Personen | ja | 75 / 25 | 75 / 25 | "Je Anzahl Personen", kein Badge |
| Haus | ja | Je Anzahl Personen | nein | - / - | - / - | "Je Anzahl Personen", Badge "nicht berechenbar, Fixkosteneingabe fehlt" |
| Haus | nein | beliebig | egal | - / - | - / - | "trägt Vermieter:in", kein Badge |
| Wohnung 1 | ja | beliebig (Altdaten) | egal | 100 / - | 100 / - | "trägt Wohnung 1", kein Badge |
| Wohnung 2 | ja | beliebig (Altdaten) | egal | - / 100 | - / 100 | "trägt Wohnung 2", kein Badge |
| Wohnung 1 oder 2 | nein | beliebig | egal | - / - | - / - | "trägt Vermieter:in", kein Badge |
| Haus | ja | leer (nur Altdaten) | egal | 50 / 50, Schlüssel "Je Wohneinheit" | 50 / 50, Schlüssel "Je Wohneinheit" | Zuordnung leer (""), kein Badge |

Weitere Beobachtungen aus der Matrix:

- Dashboard und Anlage 8 sind in allen 60 Kombinationen gleich, auch im Schlüssel-Text. Beide rufen `calc.SonderkostenAnteile` mit denselben Argumenten, also ist das durch Bauart so.
- Die Liste zeigt das Badge in genau einer Kombination, und die ist dieselbe, in der Dashboard und Anlage nichts zählen (Haus, umlegbar, Personen, ohne Fixkosteneingabe). Die Liste und calc stimmen hier überein, solange die Monatsmenge dieselbe ist (siehe Abschnitt "Drei Wege, die Fixkosteneingabe zu finden").
- Die Summe "davon umlegbar" der Liste (`sonderkosten_liste.go:75-77`) zählt den Rechnungsbetrag aller umlegbaren Einträge, auch den nicht berechenbaren. Das ist Absicht (Rechnungssumme, kein Anteil) und entspricht dem Glossar. Die Aufstellung nach Art und Objekt (`baueSonderkostenAufstellung`) kennt weder umlegbar noch berechenbar und zählt nicht berechenbare mit (Test `TestSonderkosten_AufstellungZaehltNichtBerechenbare`).
- Die Löschfrage der Fixkosteneingabe (`fixkosten_hinweis.go:168`) zählt Einträge des Monats, für die `BrauchtFixkosteneingabe()` gilt. Sie prüft nicht, ob die Eingabe existiert, weil sie nur beim Löschen einer vorhandenen Eingabe läuft.
- Gleichstand bei Personen 0 zu 0: `splitRatio` fällt auf 50/50 zurück, der Eintrag bleibt berechenbar (`calc.Ratio2`).

### Die einzige Abweichung

Haus, umlegbar, Logik leer: Dashboard und Anlage 8 nehmen "Je Wohneinheit" (`calc/sonderkosten.go:81-83`, `abrechnung/berechnung.go:67-69`), die Liste zeigt `calc.LogikLabels[""]`, also einen leeren Text (`web/sonderkosten.go:68`). Dieser Zustand ist über Formular, Import und Store nicht mehr herstellbar (`validiereSonderkosten`, `store/sonderkosten.go:198`), das Schema hat aber kein CHECK dafür (`schema.sql`, Spalte `logik TEXT NOT NULL DEFAULT ''`). Er kann also nur in Altdaten oder einer von Hand bearbeiteten Datenbank vorkommen. Für verhaltensgleiches Refactoring ist er ein Randfall, den ein neues Modul bewusst nennen muss (gleich lassen oder angleichen).

## Wo die Regel wirklich doppelt steht

1. **Logik-Schlüssel und Filter in zwei Lesern.** Identischer Block in `internal/calc/sonderkosten.go:74-83` und `internal/abrechnung/berechnung.go:60-69`: Objekt Wohnung 1 oder 2 ersetzt die Logik durch `LogikWohnung1`/`LogikWohnung2`, leere Logik wird `LogikWohneinheit`. Dazu der Filter "berechenbar und Anteil ungleich 0" in `calc/sonderkosten.go:71` und `abrechnung/berechnung.go:57` (dort zusätzlich `imBereich`). Das ist die einzige Duplikation, die eine Änderung an zwei Stellen verlangt: Kommt ein neues Objekt oder eine neue Logik dazu, muss sie an beiden Stellen nachgezogen werden. Der Kommentar `berechnung.go:49-50` ("the same filter as calc.SonderkostenMonate") gibt das zu.
2. **Zweite Herleitung von "berechenbar".** `internal/web/sonderkosten_liste.go:85` rechnet `BrauchtFixkosteneingabe() && !slices.Contains(fixMonate, Monat[:7])`, `calc/sonderkosten.go:38-45` rechnet `BrauchtFixkosteneingabe()` und fragt dann die `Fixkostenreihe`. Beide benutzen das gemeinsame Prädikat, die Aussage "Eingabe im Monat vorhanden" wird aber getrennt beantwortet.
3. **Objekt auf Wohnung abbilden, vierfach.** `calc/sonderkosten.go:31-36` (Betrag), `calc/sonderkosten.go:75-80` und `abrechnung/berechnung.go:61-66` (Schlüssel), `web/sonderkosten.go:63-66` (Anzeigetext). Die vierte Stelle ist bewusst Anzeige und gibt anderen Text aus, die Fallunterscheidung ist dieselbe.

### Drei Wege, die Fixkosteneingabe zu finden

| Weg | Datei:Zeile | Quelle |
|---|---|---|
| `Fixkostenreihe.Monat(s.Monat)` | `internal/calc/sonderkosten.go:39-45` | `Datenstand.Eingaben`, Schlüssel `"YYYY-MM-01"` |
| `SELECT COUNT(*) FROM fixkosten_eingaben WHERE monat = ?` | `internal/store/sonderkosten.go:99` | Datenbank, nur beim Schreiben |
| `fixMonate` aus `AllFixkostenEingaben`, Vergleich mit `Monat[:7]` | `internal/web/sonderkosten.go:221-229`, `sonderkosten_liste.go:85` | Datenbank, `"YYYY-MM"` |

Für Daten aus derselben Datenbank liefern alle drei dasselbe. Die Liste schneidet `s.Monat[:7]` ohne Längenprüfung ab, eine Monat-Spalte kürzer als 7 Zeichen würde dort paniken. Das Schema hat dafür kein CHECK, die Schreibwege prüfen `validAbrechnungsmonat` (`store/sonderkosten.go:175`). Nicht am echten Datenbestand geprüft, nur im Code.

## Was schon entschieden und umgesetzt ist

- #419 (umgesetzt mit #440): Der Store validiert in Create und Update selbst (`insertSonderkosten`, `UpdateSonderkosten` rufen `sonderkostenGueltig`), `PruefeSonderkosten` ist der Eingang für Formular und CSV-Import, Fehlertyp `*SonderkostenFehler`. Der Kandidat "validiert nichts im Store" des Reviews ist damit überholt.
- #420: `Sonderkosten.BrauchtFixkosteneingabe()` ist der einzige Ort des Prädikats, die Liste ist als reine Funktionen in `sonderkosten_liste.go` gebaut. Beides ist im Code so.
- #428: `fixkostenWirkungVon` ist eine reine Funktion in `fixkosten_hinweis.go` und benutzt das Prädikat für den Zähler.
- Rest des Review-Kandidaten "ein Eingang für Formular und CSV-Import": Das Formular leert `Logik` für Einträge, die nicht Haus und umlegbar sind (`web/sonderkosten.go:112-114`), der Import tut das nicht, er lässt den Store melden ("Die Berechnungslogik gibt es nur beim Objekt Haus und umlegbar", `store/sonderkosten.go:204`). Beide Wege prüfen danach dieselbe Funktion `PruefeSonderkosten` (`web/sonderkosten.go:115`, `web/sonderkosten_import.go:91`). Das ist eine Entscheidung zur Eingabe, keine Doppelregel.

## Hinweis zu `GLOSSARY.md`

Der Eintrag "Zuordnung (Sonderkosten)" (Zeile 198) ist im Arbeitsverzeichnis des Hauptprojekts vorhanden, in dem Worktree dieses Researchs (Stand `main`, dc9065f) aber noch nicht eingecheckt. Der Code benutzt "Zuordnung" nur in `sonderkostenKarte.Zuordnung` und `sonderkostenZuordnung` für den Anzeigetext. Ein neues Modul sollte den Begriff aus dem Glossar tragen, nicht aus dem Anzeigetext.

## Folgerung für Map #464

- Nötig für Verhaltensgleichheit: ein Modul, das für einen Eintrag (Anteil je Wohnung, Berechenbar, Logik-Schlüssel) liefert. Dann verschwindet Duplikation 1, und Duplikation 2 und 3 lassen sich in dasselbe Modul ziehen (Berechenbar kennt dann die Monatsmenge der Fixkosteneingaben, Anzeigetext bleibt in `web`).
- Offen für die Spec: Den Altdatenfall (Haus, umlegbar, Logik leer) entweder gleich lassen (Liste leer, Rechnung 50/50) oder angleichen, dann mit Test und Entscheidung zur Changelog-Zeile.
- Nicht mehr offen: Validierung am Schreibpfad, Prädikat, Zähler der Löschfrage.
