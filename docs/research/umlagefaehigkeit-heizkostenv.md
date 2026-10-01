# Umlagefähigkeit der Kostenpositionen und HeizkostenV bei Wärmepumpe

Stand der Recherche: 2026-10-01. Keine Rechtsberatung. Wortlaute der Normen stammen von gesetze-im-internet.de (abgerufen am Recherchetag, Gesetzesstand wie dort veröffentlicht).

Kontext: Zweifamilienhaus, nur Wohnung 2 ist vermietet, Wärmepumpe (WP) mit PV-Anlage. Positionsnamen aus `internal/store/store.go` (`KostenpositionDefaults`, 16 Stück) und `CONTEXT.md`.

## Quellenlage und Vorbehalte

- Normtexte: gesetze-im-internet.de, im Volltext gelesen (BetrKV § 1, § 2; HeizkostenV §§ 1 bis 9a, 10 bis 12; BGB § 556).
- BGH-Entscheidungen: Die Volltexte auf `juris.bundesgerichtshof.de` und `bundesgerichtshof.de/SharedDocs` waren beim Recherchelauf nicht abrufbar (Redirect bzw. 404). Aussagen zu BGH-Urteilen sind daher mit **[Primärtext nicht gelesen]** markiert; das Aktenzeichen ist gesichert, der Inhalt stammt aus Suchauszügen. Vor Verwendung in Entscheidungen bitte im Original gegenlesen.
- Wo weder Norm noch BGH-Text eine Antwort trägt, steht **offen** und die Frage ist unten gesammelt.

## 1. Einstufung der 16 Kostenpositionen

Rechtsrahmen:

- Mieter trägt Betriebskosten nur, wenn vereinbart: BGB § 556 Abs. 1 Satz 1. https://www.gesetze-im-internet.de/bgb/__556.html
- Betriebskosten sind nur Kosten, die dem Eigentümer "laufend entstehen" (BetrKV § 1 Abs. 1). Nicht dazu gehören Verwaltungskosten und Instandhaltungs-/Instandsetzungskosten (BetrKV § 1 Abs. 2 Nr. 1 und 2). https://www.gesetze-im-internet.de/betrkv/__1.html
- Katalog: BetrKV § 2 Nr. 1 bis 17. https://www.gesetze-im-internet.de/betrkv/__2.html
- Eine pauschale Vereinbarung "Mieter trägt die Betriebskosten" genügt nach BGH 10.02.2016, VIII ZR 137/15, auch formularmäßig, ohne Aufzählung der einzelnen Arten. **[Primärtext nicht gelesen]**, Nachweis über https://dejure.org/dienste/vernetzung/rechtsprechung?Gericht=BGH&Datum=10.02.2016&Aktenzeichen=VIII+ZR+137/15 . Ältere Literatur ("sonstige Betriebskosten müssen einzeln im Vertrag stehen") ist damit zumindest für die Katalognummern überholt; ob der Senat Nr. 17 gesondert behandelt, ist im Primärtext zu prüfen (**offen**).

Legende: **A** = umlagefähig (Katalogposition, bei wirksamer Betriebskostenvereinbarung). **B** = nur bei ausdrücklicher Vereinbarung bzw. nur teilweise. **C** = nie als Betriebskosten.

| Nr. | Position (App) | Einstufung | Begründung und Quelle |
|---|---|---|---|
| 1 | Grundsteuer | A | BetrKV § 2 Nr. 1: "die laufenden öffentlichen Lasten des Grundstücks, hierzu gehört namentlich die Grundsteuer". https://www.gesetze-im-internet.de/betrkv/__2.html |
| 2 | Wohngebäudeversicherung | A (Teil) | BetrKV § 2 Nr. 13: Versicherung des Gebäudes gegen "Feuer-, Sturm-, Wasser- sowie sonstige Elementarschäden". Nur die im Katalog genannten Sach-/Haftpflichtrisiken; Bestandteile wie Mietausfall oder Rechtsschutz stehen nicht im Katalog (**offen**, ob Nr. 17 greift). Selbstbeteiligungen und Schadenbeseitigung sind Instandsetzung (BetrKV § 1 Abs. 2 Nr. 2). |
| 3 | Deichbeitrag Grund und Boden | A, mit Vorbehalt | Öffentlich-rechtlicher Beitrag zum Grundstück: BetrKV § 2 Nr. 1 "laufende öffentliche Lasten". Muss "laufend" anfallen (BetrKV § 1 Abs. 1); einmalige Beiträge nicht. Einordnung von Deichverbandsbeiträgen konnte im Primärtext nicht belegt werden (**offen**). |
| 4 | Deichbeitrag Bauliche Anlagen | A, mit Vorbehalt | wie Nr. 3. Zusätzlich prüfen, ob der Beitrag Investitions-/Instandsetzungsanteile enthält (BetrKV § 1 Abs. 2 Nr. 2). |
| 5 | Kreisverband Wesermarsch der Wasser- und Bodenverbände | A, mit Vorbehalt | wie Nr. 3 (Verbandsbeitrag). Einordnung **offen**. |
| 6 | Abfallwirtschaft Grundgebühr Haushalt | A | BetrKV § 2 Nr. 8: "Kosten der Straßenreinigung und Müllbeseitigung", dazu "namentlich die für die Müllabfuhr zu entrichtenden Gebühren". |
| 7 | Abfallwirtschaft Grundgebühr Personen | A | wie Nr. 6. |
| 8 | Abfallwirtschaft Biomüll | A | wie Nr. 6. |
| 9 | Abfallwirtschaft Restmüll | A | wie Nr. 6. |
| 10 | Grundgebühr Strom | B (gemischt) | Der Hauptzähler-Grundpreis deckt Allgemeinstrom, WP-Strom, Haushaltsstrom W2 (und Wallbox). Umlagefähig sind nur Anteile für Katalogzwecke: Heizungsstrom (BetrKV § 2 Nr. 4 Buchst. a "Kosten des zur Wärmeerzeugung verbrauchten Stroms ... Kosten des Betriebsstroms"), Beleuchtung Gemeinschaftsflächen (Nr. 11). Haushaltsstrom von Wohnung 2 ist kein Betriebskostentyp; Weiterberechnung nur nach vertraglicher Vereinbarung (**offen**: energierechtliche Pflichten des Vermieters als Weiterverteiler nicht geprüft). Wallbox-Strom: nicht im Katalog. |
| 11 | Grundgebühr Trinkwasser | A | BetrKV § 2 Nr. 2: "Kosten der Wasserversorgung, hierzu gehören die Kosten des Wasserverbrauchs, die Grundgebühren ...". |
| 12 | Grundgebühr Abwasser | A | BetrKV § 2 Nr. 3: "Gebühren für die Haus- und Grundstücksentwässerung". |
| 13 | Grundgebühr Internet | C (als Betriebskosten) | BetrKV § 2 Nr. 15 Buchst. b: Breitband-Grundgebühren nur "bis zum 30. Juni 2024" und nur bei Betrieb einer privaten Verteilanlage; danach nur Betriebsstrom. Buchst. c: nur Betriebsstrom und Glasfaser-Bereitstellungsentgelt (§ 72 TKG), und nur bei gebäudeinterner Glasfaser-Verteilanlage mit freier Anbieterwahl. Ein Internet-Anschluss, den der Vermieter als Leistung bereitstellt, ist keine Betriebskostenart. Nur als Teil der Miete bzw. als vertragliche Nebenleistung vereinbar. https://www.gesetze-im-internet.de/betrkv/__2.html |
| 14 | Wartungskosten Wärmepumpe | A | BetrKV § 2 Nr. 4 Buchst. a: "Kosten der Bedienung, Überwachung und Pflege der Anlage, der regelmäßigen Prüfung ihrer Betriebsbereitschaft und Betriebssicherheit einschließlich der Einstellung durch eine Fachkraft". Auch HeizkostenV § 7 Abs. 2. Reparatur/Austausch von Teilen ist dagegen nie umlagefähig (BetrKV § 1 Abs. 2 Nr. 2). Gehört kostenmäßig zu den Heizkosten und unterliegt der HeizkostenV-Verteilung (siehe Abschnitt 2), nicht der Verteilung "je Wohneinheit". |
| 15 | Streaming-Dienste | C | Kein Katalogtatbestand; keine Kosten, die "durch das Eigentum am Grundstück oder durch den bestimmungsmäßigen Gebrauch des Gebäudes" entstehen (BetrKV § 1 Abs. 1, BGB § 556 Abs. 1 Satz 2). Auch Nr. 17 greift nicht, da sie nur Betriebskosten im Sinne von § 1 erfasst. |
| 16 | Sonstige Kosten | B | Nur Nr. 17: "sonstige Betriebskosten, hierzu gehören Betriebskosten im Sinne des § 1, die von den Nummern 1 bis 16 nicht erfasst sind". Je Einzelfall zu prüfen; Verwaltung und Instandhaltung nie. Empfehlung: im Mietvertrag die konkreten Arten vorsorglich nennen (BGH 137/15 macht dies für die Wirksamkeit nicht nötig, **[Primärtext nicht gelesen]**). |
| 17 | Verbrauch: Heizung | A, mit HeizkostenV-Pflicht | BetrKV § 2 Nr. 4 Buchst. a; Verteilung zwingend nach HeizkostenV (Abschnitt 2). |
| 18 | Verbrauch: Wasser (Frisch-, Abwasser, Warmwasser) | A | BetrKV § 2 Nr. 2, 3, 5 Buchst. a (Warmwasser: Wasserversorgung plus "Kosten der Wassererwärmung entsprechend Nummer 4 Buchstabe a"). |
| 19 | Verbrauch: Strom (Zwischenzähler W2) | B | Haushaltsstrom ist keine Betriebskostenart; Weiterberechnung nur nach ausdrücklicher Vereinbarung (siehe Nr. 10). |

Hinweis zur Zählung: Die Frage nennt "16 Positionen" und zusätzlich drei verbrauchsabhängige Gruppen. Die App führt 16 feste Positionen (IDs 1 bis 16 in `KostenpositionDefaults`); die Tabelle oben führt diese als Nr. 1 bis 16 und die verbrauchsabhängigen Gruppen als Nr. 17 bis 19 (Nummerierung nur für diese Tabelle).

Zusätzlich relevant: Nur Wohnung 2 ist vermietet. Kosten, die auf Wohnung 1 (Eigennutzung) entfallen, unterliegen keiner Umlagebeschränkung; die Einstufung gilt nur für den Anteil, der Wohnung 2 belastet. Die Umlage-Ratio (Logik) ist im Mietvertrag bzw. nach § 556a BGB zu bestimmen (nicht Gegenstand dieser Recherche).

Weitere Randbedingungen für die Abrechnung gegenüber Mieter: jährliche Abrechnung, Mitteilung binnen zwölf Monaten nach Ende des Abrechnungszeitraums, sonst Ausschluss von Nachforderungen (BGB § 556 Abs. 3). https://www.gesetze-im-internet.de/bgb/__556.html

## 2. HeizkostenV bei Wärmepumpe, zwei Wohnungen

### 2.1 Anwendbarkeit

- Die HeizkostenV gilt für Kosten zentraler Heizungs- und Warmwasseranlagen, verteilt durch den Gebäudeeigentümer auf die Nutzer (§ 1 Abs. 1). https://www.gesetze-im-internet.de/heizkostenv/__1.html
- **Kleinhaus-Ausnahme, § 2:** "Außer bei Gebäuden mit nicht mehr als zwei Wohnungen, von denen eine der Vermieter selbst bewohnt, gehen die Vorschriften dieser Verordnung rechtsgeschäftlichen Bestimmungen vor." https://www.gesetze-im-internet.de/heizkostenv/__2.html
  - Bewohnt der Vermieter Wohnung 1 selbst (Annahme, aus dem Repo nicht belegt), geht die Vereinbarung im Mietvertrag der Verordnung vor. Dann sind Abrechnungsmaßstäbe außerhalb 50 bis 70 Prozent oder eine Pauschale vertraglich möglich, solange der Vertrag das regelt. Ob und wie das Kürzungsrecht nach § 12 dann wirkt, ist durch Norm nicht beantwortet (**offen**; naheliegend: § 12 Abs. 1 setzt Abweichung "entgegen den Vorschriften dieser Verordnung" voraus, die bei vertraglichem Vorrang fehlt).
  - Ohne Selbstbewohnung durch den Vermieter (etwa beide Wohnungen vermietet) gilt die HeizkostenV zwingend.
  - Wichtig: Enthält der Mietvertrag keine eigene Regel zur Heizkostenverteilung, ist unklar, was stattdessen gilt (**offen**, Auslegungsfrage ohne belegte BGH-Aussage).
- Die Gewichtung ist Wahl des Gebäudeeigentümers (§ 6 Abs. 4 Satz 1). https://www.gesetze-im-internet.de/heizkostenv/__6.html

### 2.2 Heizung: Verbrauchsanteil 50 bis 70 Prozent

- § 7 Abs. 1 Satz 1: "mindestens 50 vom Hundert, höchstens 70 vom Hundert nach dem erfassten Wärmeverbrauch der Nutzer", der Rest "nach der Wohn- oder Nutzfläche oder nach dem umbauten Raum". https://www.gesetze-im-internet.de/heizkostenv/__7.html
- Die 70-Prozent-Pflicht in § 7 Abs. 1 Satz 2 gilt nur bei Öl- oder Gasheizung in nicht WSVO-1995-konformen Gebäuden; für eine WP nicht einschlägig.
- Rechtsgeschäftliche Höchstsätze über 70 Prozent bleiben unberührt (§ 10). https://www.gesetze-im-internet.de/heizkostenv/__10.html
- Verbrauchserfassung: "Wärmezähler oder Heizkostenverteiler" (§ 5 Abs. 1 Satz 1). https://www.gesetze-im-internet.de/heizkostenv/__5.html

**Bezug zur App-Gewichtung (70/30, 60/40, 50/50):** Alle drei Stufen liegen innerhalb 50 bis 70 Prozent Verbrauchsanteil. Der Flächenanteil (30/40/50 Prozent) entspricht der Rest-Verteilung nach Wohnfläche. Die Struktur ist damit konform, sofern "Wärmeverbrauch" die erfasste Wärmemenge ist. Zwei Punkte weichen ab:

1. Maßstabswahl und Änderung (§ 6 Abs. 4): Die Wahl der Maßstäbe bleibt beim Eigentümer. Eine Änderung für künftige Zeiträume ist nur aus den Gründen Nr. 1 bis 3 möglich, "durch Erklärung gegenüber den Nutzern", und "nur mit Wirkung zum Beginn eines Abrechnungszeitraumes zulässig" (§ 6 Abs. 4 Satz 3). Die App lässt die Gewichtung pro Ablesung (monatlich) wählen. Gegenüber dem Mieter muss der Maßstab je Abrechnungszeitraum feststehen; eine monatliche Änderung ist bei Anwendbarkeit der HeizkostenV nicht zulässig (Folgerung aus dem Wortlaut; kein BGH-Beleg gelesen).
2. Kostenbasis: Der App-Ansatz nimmt den gesamten WP-Strom als Heizkostenbasis (siehe 2.4).

### 2.3 Wärmepumpen-Strom als "Brennstoff"

- Zu den Kosten des Betriebs der zentralen Heizungsanlage gehören "die Kosten des zur Wärmeerzeugung verbrauchten Stroms und der verbrauchten Brennstoffe und ihrer Lieferung, die Kosten des Betriebsstromes, ... Bedienung, Überwachung und Pflege ... regelmäßigen Prüfung ... Messungen nach dem BImSchG, ... Ausstattung zur Verbrauchserfassung ..." (HeizkostenV § 7 Abs. 2; wortgleich BetrKV § 2 Nr. 4 Buchst. a). https://www.gesetze-im-internet.de/heizkostenv/__7.html
- WP-Strom ist damit Heizkostenbestandteil, ebenso Wartung (App-Position 14 "Wartungskosten Wärmepumpe") und Kosten der Zähler/Abrechnung. Die App verteilt Position 14 derzeit "je Wohneinheit" als Fixkosten; im HeizkostenV-Regime gehört sie in den Verteiltopf nach § 7 Abs. 1 (50 bis 70 Prozent nach Verbrauch, Rest nach Fläche).
- Es gibt "Kosten des zur Wärmeerzeugung verbrauchten Stroms": nur tatsächlich angefallene Stromkosten. PV-Eigenverbrauch erzeugt keine Bezugskosten (siehe Abschnitt 3).

### 2.4 Getrennte Warmwasserkosten, Wärmemengenzähler misst nur Raumheizung

- Warmwasser: mindestens 50, höchstens 70 Prozent nach erfasstem Warmwasserverbrauch, Rest nach Wohn- oder Nutzfläche (§ 8 Abs. 1). Kosten der Wasserversorgung gehören dazu, "soweit sie nicht gesondert abgerechnet werden" (§ 8 Abs. 2). https://www.gesetze-im-internet.de/heizkostenv/__8.html
- Verbundene Anlage (Heizung und Warmwasser aus einem Erzeuger, hier die WP): Die "einheitlich entstandenen Kosten des Betriebs" sind aufzuteilen. Bei Wärmepumpen "nach den Anteilen am Wärmeverbrauch" (§ 9 Abs. 1 Satz 2). Der Heizungsanteil ergibt sich aus dem Gesamtverbrauch "nach Abzug des Verbrauchs der zentralen Warmwasserversorgungsanlage" (§ 9 Abs. 1 Satz 4). https://www.gesetze-im-internet.de/heizkostenv/__9.html
- Die auf die Warmwasseranlage entfallende Wärmemenge "ist mit einem Wärmezähler zu messen" (§ 9 Abs. 2 Satz 1). Nur bei unzumutbar hohem Aufwand Ersatz durch Formel Q = 2,5 x V x (tw-10) (Satz 2), im Ausnahmefall Q = 32 x AWohn; bei "Betrieb einer monovalenten Wärmepumpe" ist die so bestimmte Wärmemenge "mit 0,30 zu multiplizieren" (Satz 6 Nr. 3). Die Einheit des Ergebnisses (Wärme vs. Strom) lässt der Wortlaut nach Lesart offen (**offen**: Auslegung des Faktors 0,30 nicht geprüft).
- Konsequenz für die App: Die Wärmemengenzähler `waerme_wohnung1`/`waerme_wohnung2` messen laut `CONTEXT.md` nur Raumheizung, der WP-Strom deckt Heizung und Warmwasser gemeinsam. Die App behandelt den gesamten WP-Strom als Heizungskosten und verteilt ihn nach Raumwärmeverhältnis und Fläche. Das entspricht nicht § 9: Es fehlt (a) die Messung oder Ermittlung der Warmwasser-Wärmemenge, (b) die Abtrennung des Warmwasseranteils vom WP-Strom, (c) die Verteilung des Warmwasseranteils nach § 8 (erfasster Warmwasserverbrauch je Nutzer, mindestens 50 Prozent). Die App-Zähler `wasser_warmwasseraufbereitung` (Haus gesamt) und `wasser_wohnung2` (Gesamtwasser W2) liefern keinen Warmwasserverbrauch je Wohnung; ein Warmwasserzähler je Nutzer im Sinne § 5 Abs. 1 Satz 1 ist nicht erkennbar.
- Wärmepumpen-spezifische Übergangsregel: Wurde der WP-Verbrauch am 1. Oktober 2024 nicht erfasst, musste der Eigentümer "bis zum Ablauf des 30. September 2025" Erfassungsausstattung installieren; die Verordnung gilt erstmals für den Abrechnungszeitraum nach der Installation (§ 12 Abs. 3 Satz 1, 2). Die Frist ist zum Recherchedatum (2026-10-01) abgelaufen. Bruttowarmmiete-Regel (Satz 3) betrifft nur Mieter mit Bruttowarmmiete; bei Nebenkostenabschlag nicht einschlägig. https://www.gesetze-im-internet.de/heizkostenv/__12.html
- Ausnahmen (§ 11 Abs. 1): Heizwärmebedarf unter 15 kWh/(m2 a) (Nr. 1 Buchst. a), unverhältnismäßig hohe Kosten (Nr. 1 Buchst. b), Gebäude, die "überwiegend" mit Wärme aus Rückgewinnungsanlagen oder Solaranlagen versorgt werden (Nr. 3 Buchst. a). Ob eine Luft- oder Erd-WP unter "Anlagen zur Rückgewinnung von Wärme" fällt, ist aus der Norm nicht zu entscheiden (**offen**; gelesen wurde nur der Wortlaut). https://www.gesetze-im-internet.de/heizkostenv/__11.html
- Sonderfall Geräteausfall: Schätzung nach § 9a, bei mehr als 25 Prozent betroffener Fläche reine Flächenverteilung (§ 9a Abs. 2). https://www.gesetze-im-internet.de/heizkostenv/__9a.html

### 2.5 Kürzungsrecht

- § 12 Abs. 1 Satz 1: Werden die Kosten "entgegen den Vorschriften dieser Verordnung nicht verbrauchsabhängig abgerechnet", darf der Nutzer "den auf ihn entfallenden Anteil um 15 vom Hundert" kürzen. Satz 2 und 3: zusätzlich 3 Prozent bei fehlender fernablesbarer Ausstattung (nach § 5 Abs. 2 oder 3) bzw. bei fehlenden Informationen nach § 6a. https://www.gesetze-im-internet.de/heizkostenv/__12.html
- Das Recht greift nur "soweit" nicht verbrauchsabhängig abgerechnet wird. Auf den Warmwasser- und Heizungsanteil bezogen: Eine fehlende Warmwasser-Wärmemengenmessung lässt nach der Rechtsprechung zu verbundenen Anlagen die Heiz- und Warmwasserkosten nicht verbrauchsabhängig sein; BGH 12.01.2022, VIII ZR 151/20 bestätigte das 15-Prozent-Kürzungsrecht bei fehlendem Wärmezähler für Warmwasser einer verbundenen Anlage (Fernwärme-Fall). **[Primärtext nicht gelesen]**, übertragbar auf WP nur per Analogie zu § 9 Abs. 2 (**offen**).
- Fernablesbarkeit und § 6a: Bei installierten fernablesbaren Ausstattungen sind monatlich Verbrauchsinformationen mitzuteilen (§ 6a Abs. 1 Nr. 2); nicht fernablesbare Ausstattungen müssen bis 31. Dezember 2026 nachgerüstet sein (§ 5 Abs. 3). https://www.gesetze-im-internet.de/heizkostenv/__6a.html

## 3. PV-Eigenverbrauch und Grundpreis Strom

Normbefund (ohne PV-spezifische Vorschrift in BetrKV oder HeizkostenV; PV-Anlagen kommen in BetrKV § 2 nicht vor):

- Betriebskosten sind nur Kosten, die dem Eigentümer tatsächlich "laufend entstehen" (BetrKV § 1 Abs. 1). PV-Eigenverbrauch verursacht keine Strombezugskosten; WP-Strom aus PV ist damit kostenlos im Sinne der Kostenverteilung. HeizkostenV § 7 Abs. 2 ("Kosten des zur Wärmeerzeugung verbrauchten Stroms") verteilt nur angefallene Kosten. Die App berechnet WP-Kosten nur aus dem Netzbezug-Anteil ("Nicht dem Netzbezug zugeordnet (PV)" bleibt kostenfrei); das ist mit diesem Wortlaut vereinbar. https://www.gesetze-im-internet.de/betrkv/__1.html
- Kosten der PV-Anlage selbst (Anschaffung, Instandsetzung) sind keine Betriebskosten (BetrKV § 1 Abs. 2 Nr. 2 für Instandsetzung; Anschaffung ohnehin nicht "laufend"). Ob laufende PV-Kosten (Wartung, Versicherung, Zähler) über Nr. 13 bzw. Nr. 17 umlagefähig sind, ist aus dem Normtext nicht entschieden (**offen**).
- Fiktive Berechnung von PV-Strom (z. B. zu Netzpreis) gegenüber dem Mieter ist durch BetrKV/HeizkostenV nicht gedeckt, da keine "entstandenen" Kosten (Folgerung). Als Stromlieferung an den Mieter bräuchte sie eine vertragliche Vereinbarung und eigene energierechtliche Prüfung (EEG/EnWG nicht geprüft, **offen**).
- Wirtschaftlichkeitsgebot bei der Abrechnung: BGB § 556 Abs. 3 Satz 1. https://www.gesetze-im-internet.de/bgb/__556.html
- Einspeisevergütung (`strom_einspeisung`) ist Einnahme des Eigentümers; in BetrKV § 1 ist keine Gutschrift an Mieter vorgesehen (nicht ausdrücklich geregelt, informative Größe in der App, siehe `CONTEXT.md`).
- Zuteilungskaskade der App (W2, dann WP, dann Wallbox, Rest W1; `internal/calc/strom.go`): Die Reihenfolge ist eine Verteilentscheidung des Vermieters, keine normierte; bei Wohnung 2 als einzige Mieterin begünstigt/benachteiligt die Reihenfolge den Mieter, je nach Netzbezug und PV-Anteil (Bewertung nicht Gegenstand dieser Recherche).
- Grundpreis Strom (Position 10): fällt unabhängig von PV an. Ein einziger Hauptzähler-Grundpreis ist auf Zwecke aufzuteilen (Heizungsstrom Nr. 4 Buchst. a, Allgemeinstrom Nr. 11, Haushaltsstrom W2/W1 nicht umlagefähig). Wie die Aufteilung rechtssicher zu erfolgen hat, regelt weder BetrKV noch HeizkostenV (**offen**). Ein separater Grundpreis für einen eigenen WP-Zähler wäre Betriebsstrom der Heizung (BetrKV § 2 Nr. 4 Buchst. a "Kosten des Betriebsstroms"); ob Grundpreise darunter fallen, ist im Wortlaut nicht ausdrücklich geregelt (**offen**).

## Offene Fragen

1. Ist der Vermieter Selbstbewohner von Wohnung 1? Dann § 2 HeizkostenV (Vertrag vor Verordnung) und Auswirkungen auf Kürzungsrecht.
2. Was regelt der Mietvertrag zu Betriebskosten, Heizkosten, Abschlägen, Strom-Weiterberechnung?
3. BGH-Volltexte zu VIII ZR 137/15 und VIII ZR 151/20 gegenlesen.
4. Einordnung Deichverbandsbeiträge und Wasser- und Bodenverbandsbeiträge als "öffentliche Lasten" (Nr. 1).
5. Warmwasseranteil WP-Strom: Messung per Wärmezähler oder Formel nach § 9 Abs. 2 inkl. Faktor 0,30; Warmwasserzähler je Wohnung nötig?
6. Fällt eine WP unter § 11 Abs. 1 Nr. 3 Buchst. a?
7. Energierechtliche Folgen der Weiterberechnung von Haushalts- und PV-Strom an den Mieter.
