# Nebenkostenrechner

Web-App zur monatlichen Nebenkostenabrechnung für ein Zweifamilienhaus mit Wärmepumpe und PV-Anlage. Berechnet Strom-, Heizung/Warmwasser- und Wasserkosten je Wohnung aus monatlich erfassten Zählerständen, sowie Fixkosten/Grundgebühren (Grundsteuer, Versicherung, Deichbeiträge, Abfallwirtschaft, Grundpreise, Wärmepumpenwartung, PV-Anlagenwartung). Zusätzlich können Nebenkostenabschläge eingetragen und gegengerechnet werden, so daß eine erfassungsaktuelle Guthaben/Nachzahlung Übersicht entsteht. Angenommen wird, dass es Hauptzähler für die Gemeinschaft und separate Zwischenzähler für die Einliegerwohnung (Wohnung 2) und Wallboxen gibt.

## Stammdaten

Auf der `/stammdaten`-Seite gepflegt werden folgende Daten gepflegt. und gelten sofort auf alle Berechnungen:

| Wohnung | Wohnungsgröße | Flurstücksgröße |
|---|---|---|
| Wohnung 1 | 116,23 m² | - |
| Wohnung 2 | 86 m² | - |

Je Wohnung gibt es außerdem einen **Wohnungsstatus** ("Vermietung" oder "Eigennutzung", vorbelegt Wohnung 1 Eigennutzung, Wohnung 2 Vermietung), den Namen des Mieters bzw. Bewohners, die Zustellanschrift und bei Vermietung den Einzugsmonat ("Mieter seit", leer = ab Erfassungsbeginn). Für das Haus gibt es Vermieter-Name, Vermieter-Anschrift, Objektanschrift und eine optionale IBAN mit optionalem Kontoinhaber (leer = Vermieter). Diese Angaben füllen die künftige Jahresabrechnung. Die personenbezogenen Felder (Namen, Anschriften, "Mieter seit", IBAN) sind nur für angemeldete Nutzer sichtbar und änderbar, der Wohnungsstatus ist für alle sichtbar. Ohne gesetztes `LOGIN_PASSWORD` ist jeder angemeldet.

Dazu kommt die **Heizung/Warmwasser-Gewichtung** (siehe [Heizung](#heizungwarmwasser-konfigurierbarer-split-default-7030)), die im Gegensatz zu den beiden folgenden Schaltern auch Dashboard und Verlauf beeinflusst. Außerdem stehen hier drei Schalter für die Jahresabrechnung. Sie wirken auf alle Monate und Jahre, Dashboard und Monatsverlauf zeigen weiterhin alle Positionen und ändern sich nicht:

- **Reihenfolge** der Kostenpositionen: Mit den Hoch/Runter-Buttons unter "Reihenfolge der Kostenpositionen" änderst du die Reihenfolge. Sie gilt für Fixkosten-Eingabe, Fixkosten-Anzeige und Abrechnung. Der Fixkosten-CSV-Export behält seine feste Spaltenreihenfolge.
- **Umlagefähig** je Kostenposition (Ja/Nein): Nur umlagefähige Positionen erscheinen in der Jahresabrechnung. Vorbelegt mit Ja für Grundsteuer, Wohngebäudeversicherung, Deichbeitrag Grund und Boden, Deichbeitrag Bauliche Anlagen, Kreisverband Wesermarsch, die vier Abfallwirtschafts-Positionen, Grundgebühr Strom, Trinkwasser und Abwasser sowie Wärmepumpenwartung. Vorbelegt mit Nein für Grundgebühr Internet, PV-Anlagenwartung, Streaming-Dienste und Sonstige Kosten. Teilweise umlagefähige Positionen teilst du über die Berechnungslogik der Fixkosteneingabe auf (z. B. "Wohnung 1" für den nicht umlagefähigen Teil).
- **Stromverbrauch Wohnung 2 weiterberechnen** (Ja/Nein, vorbelegt Ja): Der Haushaltsstrom ist keine Betriebskostenart und wird nur laut Vereinbarung weiterberechnet.
- **Wallbox weiterberechnen** (Ja/Nein, vorbelegt Ja, auch nach dem Update einer bestehenden Datenbank): Ob der Wallbox-Anteil einer vermieteten Wohnung in deren Jahresabrechnung steht, unabhängig vom Haushaltsstrom. Bei Eigennutzung gilt der Schalter nicht, dort steht die Wallbox immer in der Abrechnung (Tabelle unter "Jahresabrechnung").

Die Fixkostenkostenpositionen (Berechnungslogik, Typ jährlich/monatlich, Wert) sowie der Nebenkostenabschlag werden dagegen nicht hier, sondern direkt an der jeweiligen Fixkosteneingabe gepflegt - siehe [Fixkosten/Grundgebühren](#fixkostengrundgebühren) und [Nebenkostenabschlag](#nebenkostenabschlag-guthabennachzahlung) unten.

Personenzahl ist variabel und wird separat pro Ablesung *und* pro Fixkosteneingabe erfasst (zwei unabhängige Werte, nicht gemeinsam versioniert).

Preise (aktuell, werden pro Monat neu erfasst statt zentral versioniert):

| Kostenart | Preis |
|---|---|
| Strom | 0,22 EUR/kWh |
| Frischwasser | 1,46 EUR/m³ |
| Abwasser | 4,87 EUR/m³ |
| Einspeisung (PV) | 0,08 EUR/kWh |

## Zähler

| Key | Beschreibung | Einheit |
|---|---|---|
| `strom_gesamt` | Stromzähler Netzbezug (1.8.0) | kWh |
| `strom_einspeisung` | Einspeisezähler (2.8.0, PV) | kWh |
| `strom_wohnung2` | Zwischenstromzähler Wohnung 2 | kWh |
| `strom_waermepumpe` | Zwischenstromzähler Wärmepumpe | kWh |
| `strom_wallbox` | Zwischenzähler Wallboxen | kWh |
| `strom_wallbox_wohnung1` | Wallbox Wohnung 1 (optional) | kWh |
| `strom_wallbox_wohnung2` | Wallbox Wohnung 2 (optional) | kWh |
| `waerme_wohnung1` | Wärmemengenzähler Wohnung 1 | **MWh** |
| `waerme_wohnung2` | Wärmemengenzähler Wohnung 2 | **MWh** |
| `wasser_gesamt` | Wasserzähler Gesamt | m³ |
| `wasser_wohnung2` | Zwischenwasserzähler Wohnung 2 | m³ |
| `wasser_warmwasseraufbereitung` | Zwischenwasserzähler Warmwasseraufbereitung | m³ |

Ableserhythmus: 1x/Monat. Verbrauch = aktueller Zählerstand minus Stand der chronologisch nächstälteren Ablesung (einfache Differenz, funktioniert automatisch auch über Lücken hinweg).

Eine Ablesung darf auch nur teilweise ausgefüllt gespeichert werden (**Teilstand**) - einzelne Zählerstände, Preise, Personenzahl oder der Abrechnungsmonat können fehlen und später nachgetragen werden (nur das Ablesedatum ist immer zwingend). Ein Teilstand fließt nicht in die Berechnung ein (Dashboard, Verbrauch) und ist in der Ablesungen-Liste/-Detailansicht als "unvollständig" markiert. Nur die jeweils neueste Ablesung darf ein Teilstand sein - solange sie es ist, führt "Neue Ablesung" stattdessen zu ihrer Vervollständigung. Anlegen und Vervollständigen sind beide auch ohne Login möglich, siehe `LOGIN_PASSWORD` unten.

Unter den Wallboxen lassen sich zwei **optionale Einzelzähler** erfassen: `strom_wallbox_wohnung1` (Wallbox Wohnung 1) und `strom_wallbox_wohnung2` (Wallbox Wohnung 2), jeweils in kWh und nur nachrichtlich vom Wallbox-Display abzulesen. Beide Felder dürfen leer bleiben, auch nur eines von beiden: Die Ablesung bleibt vollständig (kein Teilstand) und der Zähler gilt als "nicht erfasst". Der Verbrauch eines Einzelzählers entsteht nur, wenn der Stand in der Ablesung und in der direkten Vorperiode vorliegt, sonst ist er "nicht berechenbar" (er wird nie gegen 0 gerechnet, und eine Lücke wird nicht über mehrere Monate überbrückt). Die Warnungen "negativer Verbrauch" und "Ausreißer" gelten auch für die Einzelzähler und entfallen, sobald ein beteiligter Stand fehlt. Wofür die Einzelzähler in die Kosten eingehen, steht unter "Strom".

Je Ablesung gibt es außerdem den Schalter **"Wallboxen vollständig Wohnung 1"** (Formular, Schritt 1, direkt unter den Wallbox-Feldern; die Ablesungsseite zeigt ihn als Ja/Nein). Eine neue Ablesung belegt ihn aus der chronologisch letzten Ablesung vor, beim Korrigieren gilt der Wert der Ablesung. Bestehende Ablesungen bekommen beim Update den Wert "Nein", ihre Kosten ändern sich nicht.

Ebenso gibt es je Ablesung den Schalter **"PV-Gutschrift Wohnung 1"** (Formular, Schritt 4, beim Einspeisepreis; die Ablesungsseite zeigt ihn als Ja/Nein), gleich vorbelegt und für bestehende Ablesungen auf "Nein". Was er bewirkt, steht unter "Einspeisevergütung (PV)".

Im CSV-Export stehen die zwei Einzelzähler und die zwei Schalter als letzte vier Spalten (`strom_wallbox_wohnung1`, `strom_wallbox_wohnung2`, leer = nicht erfasst, sowie `wallbox_komplett_wohnung1` und `pv_gutschrift_wohnung1`, je 1 oder 0). Der Import akzeptiert sie als optional (fehlende oder leere Schalter-Zelle = 0), ältere Dateien ohne diese Spalten lassen sich weiter importieren.

Personenzahlen (`personen_1`, `personen_2`) stehen in beiden CSV-Dateien (Ablesungen und Fixkosten) mit Komma, z. B. `1,25`; ein Punkt wird als Tausenderpunkt gelesen (`1.25` ergibt 125). Erlaubt sind 0 oder mehr mit höchstens 2 Nachkommastellen. Ein negativer Wert oder mehr Nachkommastellen brechen den Import mit der Zeilennummer ab, dabei wird nichts importiert. Alte CSV-Dateien mit ganzen Zahlen bleiben gültig.

`strom_wallbox` fließt als dritte Zuteilungsstufe in die Strom-Netzbezug-Zuteilung ein (siehe unten) - sie wird vom nach Wohnung 2/Wärmepumpe verbleibenden Netzbezug abgezogen, bevor der Rest implizit Wohnung 1 zufällt. Kein eigener Wallbox-Tarif. Wie die Wallbox-Kosten auf die Wohnungen verteilt werden, steht unter "Strom" (Verteilung der Wallbox-Kosten).

Visuelle Übersicht der Zählerverschachtelung (und der PV-Verrechnungskaskade): [docs/pv-verrechnung.md](docs/pv-verrechnung.md).

## Berechnungslogik

### Strom (PV-Netzbezug-Zuteilung)

Netzbezug wird sequenziell zugeteilt: zuerst Wohnung 2 (gedeckelt auf ihren eigenen Verbrauch), dann die Wärmepumpe auf den verbleibenden Netzbezug, dann die Wallboxen auf den danach noch verbleibenden Netzbezug (keine eigene Wohnungs-Zuteilung, kein eigener Wallbox-Tarif, aber dieselbe Deckelung wie Wohnung 2/Wärmepumpe). Der Rest zählt implizit zu Wohnung 1 (keine eigene Kostenposition).

```
Netzbezug_Gesamt = Verbrauch(strom_gesamt)

W2_Anteil_kWh      = min(Netzbezug_Gesamt, Verbrauch(strom_wohnung2))
Rest1              = Netzbezug_Gesamt - W2_Anteil_kWh
WP_Anteil_kWh      = min(Rest1, Verbrauch(strom_waermepumpe))
Rest2              = Rest1 - WP_Anteil_kWh
Wallbox_Anteil_kWh = min(Rest2, Verbrauch(strom_wallbox))

Kosten_Strom_W2  = W2_Anteil_kWh * Strompreis
Kosten_WP_gesamt = WP_Anteil_kWh * Strompreis        (Basis für Heizungskosten, siehe unten)
Kosten_Wallbox   = Wallbox_Anteil_kWh * Strompreis
```

Bei PV-Überschuss (`Netzbezug_Gesamt = 0`) sind alle 3 Kosten 0. Alle Anteile über `min()` gedeckelt, nie negativ.

**Verteilung der Wallbox-Kosten auf die Wohnungen.** `Kosten_Wallbox` sind die Kosten des Zwischenzählers (auf den Cent gerundet). Auf der Ablesungsseite werden sie so auf Wohnung 1 und Wohnung 2 verteilt:

```
Schalter "Wallboxen vollständig Wohnung 1" an:
    Wohnung 1 = alle Wallbox-Kosten und -kWh (inkl. Solarzuteilung), Wohnung 2 = 0, die Einzelzähler sind egal

Schalter aus, Wallbox Wohnung 2 hat einen Verbrauch (Stand in der Ablesung und der Vorperiode):
    kWh_W2    = min(Verbrauch(strom_wallbox_wohnung2), Wallbox_Anteil_kWh)
    Kosten_W2 = Round2(kWh_W2 * Strompreis)
    Kosten_W1 = Kosten_Wallbox - Kosten_W2      (Rest, inkl. Solarzuteilung)

sonst (Schalter aus, Wallbox Wohnung 2 oder deren Vorperiode fehlt):
    keine Zuordnung, die Wallbox-Kosten bleiben informativ
```

Der Schalter schlägt die Einzelzähler. Wallbox Wohnung 1 darf fehlen, die Summe von Wohnung 1 und 2 ist immer cent-genau `Kosten_Wallbox`. Liegen beide Einzelzähler samt Vorperiode vor, vergleicht die Ablesungsseite deren Summe mit dem Verbrauch des Zwischenzählers und zeigt einen nicht blockierenden Plausibilitätshinweis, wenn sie um mehr als 5 % des Zwischenzähler-Verbrauchs oder 2 kWh (je nachdem, was größer ist) abweichen. Die Hinweis- und Kostenanzeige erscheint auch direkt nach dem Speichern, weil das Speichern auf die Ablesungsseite weiterleitet. Dashboard und Jahresabrechnung übernehmen die Zuordnung in späteren Schritten.

Visuelle Übersicht der Zählerverschachtelung und der PV-Verrechnungskaskade: [docs/pv-verrechnung.md](docs/pv-verrechnung.md).

### Heizung/Warmwasser (konfigurierbarer Split, Default 70/30)

Die Wärmepumpenstromkosten (siehe oben) werden nach Wärmemengenzählerverhältnis und Wohnungsgrößenverhältnis auf die beiden Wohnungen verteilt. Die Gewichtung (70/30, 60/40 oder 50/50, vorbelegt 70/30) ist ein einzelner Wert auf der `/stammdaten`-Seite, weil der Mietvertrag die Heizkostenverteilung für das ganze Jahr festlegt. Sie gilt für alle Monate, auch rückwirkend, und wird nicht mehr bei der Ablesung gewählt. Beim Update einer bestehenden Datenbank übernimmt die Migration einmalig den Wert der neuesten Ablesung. `qm_W1`/`qm_W2` kommen ebenfalls live von den Stammdaten, nicht von der Periode. Die Heizungsverteilung verwendet keine Personenzahl.

```
Ratio_Waerme_W1  = Verbrauch(waerme_wohnung1) / (Verbrauch(waerme_wohnung1) + Verbrauch(waerme_wohnung2))
Ratio_Waerme_W2  = Verbrauch(waerme_wohnung2) / (Verbrauch(waerme_wohnung1) + Verbrauch(waerme_wohnung2))
Ratio_Flaeche_W1 = qm_W1 / (qm_W1 + qm_W2)
Ratio_Flaeche_W2 = qm_W2 / (qm_W1 + qm_W2)

Kosten_Heizung_W1 = Kosten_WP_gesamt * (Gewichtung_Waerme * Ratio_Waerme_W1 + Gewichtung_Flaeche * Ratio_Flaeche_W1)
Kosten_Heizung_W2 = Kosten_WP_gesamt * (Gewichtung_Waerme * Ratio_Waerme_W2 + Gewichtung_Flaeche * Ratio_Flaeche_W2)
```

### Wasser (Frischwasser + Abwasser)

Der Verbrauch für die Warmwasseraufbereitung wird nach dem Personenverhältnis der jeweiligen Ablesung auf beide Wohnungen umgelegt, bevor die Frischwasserkosten berechnet werden. Abwasser wird in gleicher Menge wie Frischwasser angenommen (kein separater Abwasserzähler).

```
WW_Anteil_W1 = Verbrauch(wasser_warmwasseraufbereitung) * Personen_W1 / (Personen_W1 + Personen_W2)
WW_Anteil_W2 = Verbrauch(wasser_warmwasseraufbereitung) * Personen_W2 / (Personen_W1 + Personen_W2)

Frischwasser_W2 = Verbrauch(wasser_wohnung2) + WW_Anteil_W2
Frischwasser_W1 = (Verbrauch(wasser_gesamt) - Verbrauch(wasser_wohnung2) - Verbrauch(wasser_warmwasseraufbereitung)) + WW_Anteil_W1

Abwasser_W1 = Frischwasser_W1
Abwasser_W2 = Frischwasser_W2

Kosten_Frischwasser_X = Frischwasser_X * Frischwasserpreis
Kosten_Abwasser_X     = Abwasser_X * Abwasserpreis
```

### Einspeisevergütung (PV)

Haus-weit und informativ, unabhängig von der Kostenverteilung oben, da der Einspeisezähler hausweit misst. Nur mit dem Schalter **"PV-Gutschrift Wohnung 1"** der Ablesung wird der Ertrag dieser Ablesung als **PV-Gutschrift** Wohnung 1 gutgeschrieben: Sie erhöht deren Guthaben im fortlaufenden Saldo ohne Deckel, mindert aber nicht die Verbrauchskosten. Wohnung 2 bekommt nie eine Gutschrift, die haus-weite Tabelle bleibt unverändert. Mehrere Ablesungen im selben Abrechnungsmonat haben je ihren eigenen Schalter.

```
Einspeisevergütung = Verbrauch(strom_einspeisung) * Einspeisung_Preis
```

### Fixkosten/Grundgebühren

Die festen Kostenpositionen (Grundsteuer, Wohngebäudeversicherung, Deichbeitrag Grund und Boden, Deichbeitrag Bauliche Anlagen, Kreisverband Wesermarsch, Abfallwirtschaft Grundgebühr Haushalt/Personen/Biomüll/Restmüll, Grundgebühr Strom, Grundgebühr Trinkwasser/Abwasser, Grundgebühr Internet, Wärmepumpenwartung, PV-Anlagenwartung, Streaming-Dienste, Sonstige Kosten) werden unabhängig von Strom/Heizung/Wasser auf einer eigenen monatlichen Fixkosteneingabe erfasst und berechnet. Jede Position trägt in dieser Eingabe ihre eigene Berechnungslogik, ihren Typ und ihren Wert (vorbelegt von der letzten Eingabe, frei überschreibbar):

Je Monat gibt es genau eine Fixkosten-Eingabe. Eine zweite für denselben Monat sowie das Verschieben auf einen belegten Monat werden abgelehnt, ebenso ein CSV-Import mit einem doppelten Monat. Enthält eine bestehende Datenbank schon doppelte Monate, startet die App trotzdem und nennt die betroffenen Monate im Log. Die doppelten Eingaben lassen sich unter `/fixkosten` löschen oder in einen freien Monat verschieben, eine Eingabe kann dabei auch unter ihrem eigenen Monat weiter korrigiert werden. Der Schutz per UNIQUE-Index wird beim nächsten Start angelegt, sobald keine Doppelten mehr existieren.

```
Monatswert("jährlich")  = Jahreswert / 12          (Jahreswert = erfasster Wert dieser Eingabe)
Monatswert("monatlich") = erfasster Wert dieser Eingabe
```

Aufteilung auf Wohnung 1/2 je nach Berechnungslogik (ebenfalls an der Fixkosten-Eingabe gewählt):

```
Je Wohneinheit             : 50 / 50
Je anteiliges Flurstück    : flurstueck_W1 / (flurstueck_W1 + flurstueck_W2)   (Stammdaten)
Je anteilige Wohnungsgröße : qm_W1 / (qm_W1 + qm_W2)                            (Stammdaten)
Je Anzahl Personen         : Personen_W1 / (Personen_W1 + Personen_W2)         (Fixkosten-Eingabe)
Wohnung 1                  : 100 / 0
Wohnung 2                  : 0 / 100
```

Sind bei "Je anteiliges Flurstück", "Je anteilige Wohnungsgröße" oder "Je Anzahl Personen" beide Werte 0, fällt die Aufteilung auf hälftig zurück (gleiche Regel wie bei der Heizungswärme-Ratio). "Wohnung 1"/"Wohnung 2" rechnen die Position immer vollständig einer Seite zu, ohne Ratio.

### Nebenkostenabschlag (Guthaben/Nachzahlung)

Die monatliche Vorauszahlung je Wohnung, an der jeweiligen Fixkosteneingabe erfasst wie ein monatlich typisierter Wert (vorbelegt von der letzten Eingabe, frei überschreibbar) - deckt Fixkosten UND Verbräuche gemeinsam ab, ist selbst aber keine Kostenposition (keine Berechnungslogik, kein Split, direkter Wert je Wohnung).

```
Saldo(Monat) = Saldo(Vormonat) + Abschlag(Monat) - (Fixkosten(Monat) + Verbrauch(Monat))
```

Der Verbrauch enthält die **Wallbox**-Kosten der Wohnung (siehe Strom). Bei Wohnung 1 kommt die **PV-Gutschrift** des Monats als Gutschrift dazu: `Saldo(Monat) = Saldo(Vormonat) + Abschlag + PV-Gutschrift - (Fixkosten + Verbrauch)`, ohne Deckel.

**Dashboard und Widgets:** Die Jahreskarte und der Monatsverlauf beider Wohnungen zeigen die Kategorie **Wallbox** immer (auch mit 0 EUR); "Strom" bleibt der Haushaltsstrom von Wohnung 2, "Verbrauchswerte" zeigen die zugeordneten Netzbezug-kWh. Wohnung 1 bekommt in der Jahreskarte die negative Zeile **PV-Gutschrift**: Sie senkt die Gesamtkosten, aber nicht die Verbrauchskosten und ist kein Balkensegment. Im Monatsverlauf steht sie nur in "Kombiniert" (Gesamt abzüglich Gutschrift) und fließt in den Saldo ein, nicht in "Verbrauch" und "Verbrauchswerte". Die haus-weiten Karten "Wallboxen" und "PV-Anlage" bleiben unverändert. Die Widget-Routen zeigen dieselben Posten, ohne neue Routen.

Fortlaufend seit Erfassungsbeginn kumuliert, kein Reset zum Jahreswechsel. Positiv heißt Guthaben, negativ Nachzahlung, exakt 0 Ausgeglichen. Ein Monat ohne Fixkosteneingabe lässt den Saldo unverändert, statt ihn verschwinden zu lassen.

### Jahresabrechnung (`/abrechnung`)

Die Seite `/abrechnung?jahr=2027&wohnung=2` zeigt die Betriebskostenabrechnung einer Wohnung für ein Jahr, live aus den vorhandenen Daten berechnet (nichts wird gespeichert). Sie liegt hinter dem Login (mit gesetztem `LOGIN_PASSWORD` erscheint der Menüpunkt "Abrechnung" nur angemeldet), weil sie Namen, Anschriften und die IBAN enthält. Ohne gesetztes `LOGIN_PASSWORD` ist sie offen wie alles andere.

- **Auswahl:** Die Jahresliste enthält alle Jahre mit Ablesungen oder Fixkosten-Eingaben, die Wohnung wählst du über zwei Schaltflächen mit Name und Wohnungsstatus. Vorbelegt sind das neueste Jahr, das vollständig ist (sonst das neueste), und die erste vermietete Wohnung. Ein ungültiges Jahr oder eine ungültige Wohnung in der URL fällt auf diese Vorbelegung zurück.
- **Nicht abrechenbar:** Fehlen Daten, zeigt die Seite statt der Abrechnung die Liste aller Mängel mit je einem Link zum Beheben (fehlende oder unvollständige Ablesung, fehlende oder doppelte Fixkosten-Eingabe, fehlende Pflicht-Stammdaten). Im ersten Erfassungsjahr beginnt der Zeitraum mit dem Datum der ersten Ablesung (Ausgangsstand), Fixkosten und Vorauszahlungen gelten dort ab dem Folgemonat.
- **Mieterwechsel unterjährig:** Das Häkchen "Abweichender Zeitraum" blendet Von- und Bis-Monat ein (`?teil=1&von=3&bis=7`), die Abrechnung gilt dann für diese vollen Monate innerhalb des Jahres, Standard ist das Kalenderjahr. Ein Monat gehört ganz zu dem Zeitraum, in den die Ablesung seines Abrechnungsmonats fällt. Mieter und Anschrift kommen aus den Stammdaten, die App speichert keine Abrechnung: den Stand als PDF sichern, bevor die Stammdaten für den neuen Mieter umgeschrieben werden. Beim Einzug eines neuen Mieters trägst du in den Stammdaten den Einzugsmonat unter "Mieter seit" ein: Der Übertrag in der Monatstabelle (Anlage 1) zählt dann erst ab diesem Monat, auch in den Folgejahren.
- **Inhalt:** Kopf mit Vermieter, Mieter und Zeitraum, Ergebnis (Guthaben, Nachzahlung mit vorangestelltem Minus), Kostenübersicht je Position (Gesamtkosten, Verteilerschlüssel, Anteil, Betrag), bei Bedarf der weiterberechnete Strom von Wohnung 2, Vorauszahlungen, Saldo, Hinweise, eine Anlagenliste und der Anhang mit sieben nummerierten Anlagen: 1 Monatsverlauf und Saldo, 2 Heizung und Warmwasser je Monat, 3 Bezugsgrößen, 4 Personenzahl je Monat, 5 Verteilerschlüssel, 6 Verbrauchsübersicht, 7 Zählerstände. Bei Eigennutzung steht oben "Interne Übersicht, keine Betriebskostenabrechnung", ohne Mieter-Block und Rechtshinweise.
- **Wallbox und PV-Gutschrift:** Der Wohnungsstatus bestimmt, welche Posten in der Abrechnung einer Wohnung stehen:

  | Status der Wohnung | Block "Wallbox" (ihr Anteil) | Block "PV-Gutschrift" (nur Wohnung 1, wenn der Schalter der Ablesung an ist) |
  |---|---|---|
  | Eigennutzung | immer (der Schalter "Wallbox weiterberechnen" gilt nicht) | immer |
  | Vermietung | nur mit gesetztem "Wallbox weiterberechnen" | nie |

  Sind beide Wohnungen vermietet und "Wallbox weiterberechnen" ist aus, steht die Wallbox in keiner Abrechnung und die PV-Gutschrift nirgends. Die Gutschrift folgt ihrer Zuordnung zu Wohnung 1, auch wenn Wohnung 1 vermietet und Wohnung 2 Eigennutzung ist. Der Block "Wallbox" erscheint neben dem Stromverbrauch (Haushaltsstrom), sobald die Wallbox-Kosten den Wohnungen zugeordnet sind (Schalter oder Einzelzähler von Wohnung 2). Der Jahressaldo und der Übertrag Vorjahre zählen genau die Posten, die in der Abrechnung stehen: Vorauszahlungen minus Betriebskosten minus Wallbox plus PV-Gutschrift. Das Dashboard bleibt vom Wohnungsstatus unberührt und zeigt immer alles.
- **Monatsverlauf und Saldo (Anlage 1):** Je Abrechnungsmonat Fixkosten (nur umlagefähig), Verbrauchskosten (Heizung, Wasser/Abwasser, bei Wohnung 2 der weiterberechnete Strom, die Wallbox, wenn sie in der Abrechnung steht), bei Wohnung 1 in Eigennutzung eine Spalte PV-Gutschrift, Nebenkostenabschlag und der kumulierte Saldo, darunter die Summe und der Jahressaldo der Saldoberechnung. Oben steht, wenn es etwas zu übertragen gibt, der **Übertrag Vorjahre**: der Saldo (Abschlag minus Kosten) aller Monate vom Startmonat bis zum Monat vor dem Zeitraum. Startmonat ist der spätere von "Mieter seit" (nur bei Vermietung) und dem Beginn der Erfassung. Ein eingegrenzter Zeitraum zählt die Monate davor mit. Hat einer dieser Monate einen Mangel, entfällt der Übertrag mit dem Hinweis "Übertrag nicht berechenbar" und dem ersten betroffenen Monat. Liegt "Mieter seit" nach dem Beginn des Zeitraums, gibt es keinen Übertrag. Der Übertrag ist nicht der Dashboard-Saldo, weil das Dashboard auch nicht umlagefähige Positionen zählt.
- **Zählerstände (Anlage 7):** Eine Zeile je Ablesung mit Ablesedatum, zugeordnetem Abrechnungsmonat und allen Zählerständen, die erste Zeile ist der Ausgangsstand. Die Einzelzähler Wallbox Wohnung 1 und 2 stehen direkt hinter dem Zwischenzähler "Wallboxen", ein fehlender Stand als "-". Liegt im Zeitraum für einen Einzelzähler kein Stand vor, entfällt seine Spalte (so auch seine Zeile in Anlage 6, Verbrauchsübersicht). Fehlende Einzelzähler sind nie ein Mangel. Am Bildschirm scrollt die Tabelle waagerecht.
- **Layout:** Der Kopf zeigt Titel, Objekt, Zeitraum, Vermieter und Mieter und das Ergebnis in einer Box. Darunter stehen links die Kostenblöcke "Fixkosten" und "Verbrauchsabhängig" (mit dem Strom von Wohnung 2) und rechts eine Seitenleiste mit der Saldoberechnung und den Hinweisen. Der Anhang beginnt auf einer eigenen Seite und läuft über die volle Breite, die Werte der abgerechneten Wohnung stehen dort fett. Jede Anlage bricht im Druck nicht um und rückt als Ganzes auf die nächste Seite, wenn sie nicht mehr passt. Die Zählerstände (Anlage 7) stehen allein auf der letzten Seite im Querformat. Gedruckt oder als PDF wird immer hell auf DIN A4 ausgegeben (Ränder 14 bis 15 mm), am Bildschirm folgt die Seite dem Theme der App. Auf einem schmalen Bildschirm stehen die Tabellen als gestapelte Karten, die Seitenleiste folgt unter den Tabellen.
- **Nur am Bildschirm:** Auswahl, Drucken-Knopf und eine Hinweisbox (Frist bis 31.12. des Folgejahres bei einer Nachzahlung, PDF aufbewahren, Warnung bei einem Wechsel des Verteilerschlüssels im Jahr) erscheinen nicht im Druck. Zum Sichern als PDF: Drucken, Ziel PDF. Die App speichert den verschickten Stand nicht.

### Rundung

Jede Kostenposition (Strom, Heizung/Warmwasser, Frischwasser, Abwasser, jede der 16 Fixkostenpositionen) wird einzeln je Wohnung **kaufmännisch auf Cent gerundet** (0,5 Cent immer aufgerundet), erst nach der vollständigen Berechnung mit float-Genauigkeit. Die angezeigte Gesamtsumme je Wohnung kann dadurch um 1-2 Cent von der rechnerisch exakten Summe abweichen - das ist akzeptiert, es gibt keinen Korrekturmechanismus.

### Preise & Personenzahl

Es gibt kein zentrales Preishistoriekonzept: Strompreis, Frischwasser- und Abwasserpreis sowie die Personenzahl je Wohnung werden direkt bei jeder monatlichen Ablesung mit erfasst (nicht separat versioniert). Ein einmal berechneter Monat bleibt dadurch automatisch "eingefroren", auch wenn sich Preise oder Personenzahl später ändern.

Die Fixkosteneingabe hat ihre eigene, unabhängige Personenzahl je Wohnung (ebenfalls pro Monat eingefroren) - muss nicht mit der Ablesung übereinstimmen. 

### Fehlerbehandlung bei Zähleranomalien

Reine Warnhinweise, kein hartes Blockieren (Single-User-App ohne Vier-Augen-Prinzip):

- **Negativer Verbrauch** (neuer Stand < Vorstand): Warnung, Speichern bleibt möglich.
- **Ausreißer**: Warnung, wenn der Verbrauch eines Zählers um mehr als ±50% vom Durchschnitt seiner letzten 3 Ablesungen abweicht (bei <3 Vorwerten kein Check).
- **Fehlende Ablesung** (Lücke >1 Monat): Hinweis "Verbrauch über X Monate", Berechnung läuft automatisch über den längeren Zeitraum.

## Datenmodell

SQLite, kein ORM (`modernc.org/sqlite` + `database/sql`):

```
apartments(id, name, qm, flurstueck_groesse)
meters(id, key UNIQUE, type, unit, apartment_id NULLABLE -> apartments.id, label)
periods(id, reading_date DATE, strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis)
meter_readings(id, period_id -> periods.id, meter_id -> meters.id, zaehlerstand, UNIQUE(period_id, meter_id))
period_occupancy(id, period_id -> periods.id, apartment_id -> apartments.id, personen, UNIQUE(period_id, apartment_id))

kostenpositionen(id, key UNIQUE, label)
fixkosten_eingaben(id, monat DATE)
fixkosten_werte(id, fixkosten_eingabe_id -> fixkosten_eingaben.id, kostenposition_id -> kostenpositionen.id, wert, logik, typ, UNIQUE(fixkosten_eingabe_id, kostenposition_id))
fixkosten_personen(id, fixkosten_eingabe_id -> fixkosten_eingaben.id, apartment_id -> apartments.id, personen, UNIQUE(fixkosten_eingabe_id, apartment_id))
nebenkosten_abschlaege(id, fixkosten_eingabe_id -> fixkosten_eingaben.id, apartment_id -> apartments.id, wert, UNIQUE(fixkosten_eingabe_id, apartment_id))
```

Logik/Typ/Wert je Kostenposition liegen direkt an `fixkosten_werte` (nicht mehr jahresweise) - jede Fixkosten-Eingabe trägt ihren eigenen, unabhängigen Stand, wie Personen und der Nebenkostenabschlag.

Berechnete Kosten werden nicht persistiert, sondern bei jedem Aufruf live aus den Rohdaten berechnet.

## Tech-Stack

Go, `modernc.org/sqlite` (kein ORM, `database/sql`), server-rendered `html/template` mit Vanilla-JS für die Wizard-Interaktivität (kein htmx - ursprünglich in der Spec vorgesehen, aber nicht gebraucht). SQLite-Datei unter `/data` im Container. Multi-Arch-Docker-Image (`linux/amd64`, `linux/arm64`, `linux/arm/v7`) auf Basis `gcr.io/distroless/static-debian12`, läuft als nicht-root User (UID/GID 65532). Version und Build-Datum werden per `-ldflags` eingebrannt und im Dashboard angezeigt.

## Installation

### Docker

```bash
docker run -d \
  --name nebenkostenrechner \
  -p 8080:8080 \
  -v nebenkosten-data:/data \
  ghcr.io/larknafets/nebenkostenrechner:latest
```

Danach erreichbar unter `http://localhost:8080`, Health-Check unter `/healthz`. Die Image-Tags folgen den Release-Versionen - siehe [Releases](https://github.com/larknafets/nebenkostenrechner/releases).

> **Breaking Change (ab dieser Version):** Der Container läuft nicht mehr als root, sondern als UID/GID 65532. Ein neues, leeres Volume bekommt die passenden Rechte automatisch. Ein **bestehendes** Volume aus einer älteren Version muss einmalig manuell angepasst werden:
> ```bash
> docker run --rm -v nebenkosten-data:/data busybox chown -R 65532:65532 /data
> ```



| Umgebungsvariable | Default | Beschreibung |
|---|---|---|
| `DB_PATH` | `/data/nebenkosten.db` | Pfad zur SQLite-Datenbankdatei |
| `LISTEN_ADDR` | `:8080` | Listen-Adresse des HTTP-Servers |
| `WIDGET_LISTEN_ADDR` | `:8081` | Listen-Adresse der Widget-Routen (siehe unten) |
| `LOGIN_PASSWORD` | *(leer)* | Optionales Kennwort. Leer = alles offen wie bisher. Gesetzt: nicht angemeldete Besucher sehen nur eingeschränkte, lesende Ansichten (z. B. nur Wohnung 2 auf dem Dashboard), alle Eingabe-/Änderungsaktionen sind gesperrt - mit Ausnahme einer neuen Ablesung bzw. dem Vervollständigen eines bestehenden Teilstands, siehe oben - "Login"-Link in der Navigation öffnet ein Overlay zur Anmeldung. |

### Demo-Modus

Login mit dem Kennwort `demo` funktioniert immer, unabhängig davon ob `LOGIN_PASSWORD` gesetzt ist - er meldet in einer komplett separaten, eigenen Datenbank (`demo.db`, im selben Verzeichnis wie `DB_PATH`) an, gefüllt mit 39 Monaten realitätsnahen Testdaten. Änderungen im Demo-Modus (Ablesungen, Fixkosten-Eingaben, Stammdaten) landen ausschließlich in dieser Demo-Datenbank, nie in den echten Daten. Jeder erneute Demo-Login setzt die Demo-Datenbank vollständig auf ihren frischen Ausgangszustand zurück - keine Isolation zwischen gleichzeitigen Demo-Besuchern.

Ein manueller Reset (z. B. nach einem Bug) ist ohne eigene Route möglich: `demo.db` löschen und den Server neu starten - die Datei entsteht beim nächsten Start automatisch neu, wieder mit einem frischen 39-Monats-Datensatz.

> **Beim lokalen Testen:** das Browser-Cookie ist an den Host gebunden, nicht an den Port - ein Browser, der bereits als `demo` angemeldet ist, schickt dieses Cookie automatisch auch an eine andere lokale Instanz unter `localhost` (z. B. `go run` auf einem Port, ein Docker-Container auf einem anderen). Da beide Instanzen mit demselben fest verdrahteten Cookie-Secret signieren, wird das Cookie dort ebenfalls als gültige Demo-Session akzeptiert. Für einen sauberen, nicht angemeldeten Zustand des Cookies für `localhost` löschen oder ein privates Fenster verwenden.

### Home Assistant Add-on

Für den Betrieb als Home Assistant Add-on (inkl. Ingress-Integration) siehe das Add-on-Repository [`larknafets/ha-addons`](https://github.com/larknafets/ha-addons).

Das Repository enthält zwei Add-ons zur Wahl: **Nebenkosten** (Release, aktualisiert sich bei jedem Tag `v*`) und **Nebenkosten (Nightly)** (täglicher Stand von `main`, `stage: experimental`). Der Nightly hat eigene Daten und einen eigenen Port (8082 statt 8081 für die Widget-Routen), beide Add-ons können gleichzeitig installiert sein. Ein Wechsel nimmt die Daten nicht mit, und der Nightly kann Datenbank-Migrationen enthalten, die der Release nicht kennt - nie die Datenbank vom Nightly zurück in den Release kopieren. Der Workflow `nightly.yml` baut jede Nacht (nur wenn es in den letzten 24 Stunden einen Commit gab), behält die letzten 7 Images und lässt sich manuell für einen beliebigen Branch starten.

### Dashboard-Widgets (Home Assistant Lovelace)

Für einzelne, kleine Karten im eigenen Dashboard - statt der ganzen App über Ingress - stellt der Server auf einem eigenen, 2. Port (`WIDGET_LISTEN_ADDR`) 3 zusätzliche, read-only Routen bereit, gedacht für ein Lovelace "Webpage card"/Iframe. HA-Ingress-URLs lassen sich nicht zuverlässig in ein beliebiges Iframe einbetten (Ingress-Session muss erst durch Besuch des Addon-Panels geprimt werden), deshalb ein eigener, Ingress-freier Port - bewusst ohne Login und ohne jede mutierende Route, kleinstmögliche Angriffsfläche.

| Route | Inhalt |
|---|---|
| `GET /widget/jahressumme/{entity}` | nur die Jahressummen-Karte |
| `GET /widget/verbrauchswerte/{entity}` | nur das Verbrauchswerte-Panel (Verbrauch/Verbrauchswerte/Fixkosten/Kombiniert bleibt umschaltbar) |
| `GET /widget/uebersicht/{entity}` | beide Ansichten untereinander |

`{entity}` ist eine von: `wohnung-1`, `wohnung-2`, `wallboxen`, `pv-anlage`. Jede Route zeigt ohne Nav/Footer/Theme-Toggle immer das laufende Jahr, folgt nur `prefers-color-scheme` (kein Toggle-Button).
