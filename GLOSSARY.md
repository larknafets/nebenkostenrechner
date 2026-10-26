# Nebenkostenrechner

Monatliche Nebenkostenabrechnung für ein Zweifamilienhaus mit Wärmepumpe und PV-Anlage: Strom-, Heizungs- und Wasserkosten werden aus monatlich erfassten Zählerständen berechnet und auf Wohnung 1/Wohnung 2 verteilt.

## Language

**Ablesung**:
Eine monatliche Erfassung: Zählerstände, Preise und Personenzahl je Wohnung (Dezimalzahl mit höchstens 2 Nachkommastellen, z. B. 1,25; 0 erlaubt) zu einem Ablesedatum. Im Code auch "Periode" genannt.
_Avoid_: Eintrag, Datensatz

**Teilstand**:
Eine Ablesung, bei der nicht alle Felder gefüllt sind (Zählerstände, Preise, Personenzahl, oder Abrechnungsmonat - nur das Ablesedatum ist immer zwingend). Fließt nicht in die Berechnung ein - erst nach Vervollständigung wird sie zur regulären Ablesung. Nur die neueste Ablesung darf ein Teilstand sein.
_Avoid_: unvollständige Ablesung (funktioniert, aber "Teilstand" ist der feste Begriff)

**Ablesungsreihe**:
Alle Ablesungen des Hauses in der Reihenfolge ihres Ablesedatums. Drei Regeln gelten für jede Änderung der Reihe (Anlegen, Korrigieren, Import): das Ablesedatum steigt strikt, der Abrechnungsmonat fällt nie, und nur die neueste Ablesung darf ein Teilstand sein. Beim Korrigieren überschreibt ein leeres Feld nichts, eine vollständige Ablesung wird dort also nie zum Teilstand. Die Vorperiode einer Ablesung ist die nächst-ältere der Reihe. Löschen kann keine der Regeln verletzen. Bestehende Daten werden nicht nachgeprüft, geprüft wird nur beim Schreiben.

**Zählerstand**:
Der am Ablesedatum abgelesene Wert eines Zählers - ein kumulativer Punktwert, kein Verbrauch.
_Avoid_: Messwert, Reading

**Wallbox Wohnung 1 / Wallbox Wohnung 2**:
Die zwei optionalen Einzelzähler (kWh) hinter dem Zwischenzähler "Wallboxen", nur nachrichtlich vom Wallbox-Display abgelesen (`strom_wallbox_wohnung1`, `strom_wallbox_wohnung2`). Fehlt ein Stand, ist er "nicht erfasst" (nie 0) und die Ablesung bleibt vollständig, also kein Teilstand. Ihr Verbrauch ist nur mit Stand in der Ablesung und in der direkten Vorperiode berechenbar, sonst "nicht berechenbar". Wallbox Wohnung 2 bestimmt (samt Vorperiode) die Kosten von Wohnung 2, Wallbox Wohnung 1 dient nur der Plausibilität.
_Avoid_: Wallbox-Zähler (unklar gegenüber dem Zwischenzähler "Wallboxen")

**Wallboxen vollständig Wohnung 1**:
Der Schalter je Ablesung, der alle Wallbox-Kosten (kWh und EUR, inkl. Solarzuteilung) auf Wohnung 1 legt und die Einzelzähler überstimmt. Neue Ablesungen belegen ihn aus der letzten vor, bestehende Ablesungen haben ihn aus.
_Avoid_: Wallbox-Schalter

**Wallbox (Kostenposition)**:
Die Kosten des Zwischenzählers "Wallboxen" (Netzbezug-Anteil mal Strompreis), je Ablesung auf Wohnung 1 und Wohnung 2 verteilt: mit Schalter alles Wohnung 1, sonst zahlt Wohnung 2 `min(Verbrauch Wallbox Wohnung 2, Netzbezug-Anteil)` mal Strompreis (gerundet) und Wohnung 1 den Rest. Ohne Schalter und ohne Verbrauch von Wallbox Wohnung 2 bleibt sie informativ und keiner Wohnung zugeordnet.

**PV-Gutschrift**:
Die Einspeisevergütung einer Ablesung (`Einspeisung.Ertrag`), per Schalter "PV-Gutschrift Wohnung 1" der Ablesung Wohnung 1 zugeordnet. Sie erhöht deren Guthaben im Saldo ohne Deckel, ist aber keine Kostenposition (kein Umlagefähig-Flag, keine Berechnungslogik) und mindert nicht die Verbrauchskosten. Ohne Schalter gibt es sie nicht, die Einspeisung bleibt haus-weit und informativ. Wohnung 2 hat nie eine.
_Avoid_: Einspeisevergütung (das ist der haus-weite Ertrag, nicht die Zuordnung)

**Verbrauch**:
Die Differenz zwischen dem Zählerstand einer Ablesung und dem der chronologisch nächst-älteren Ablesung desselben Zählers.

**Verbrauchskosten**:
Die aus dem Verbrauch einer Ablesung berechneten Kosten (Strom, Wasser, Heizung, Einspeisung), noch ohne Fixkosten. Nur für eine vollständige Ablesung mit Vorperiode berechenbar, ein Teilstand oder die erste Ablesung ist es nicht. Je Ablesung, nicht je Monat: mehrere Ablesungen desselben Abrechnungsmonats haben je eigene Verbrauchskosten.
_Avoid_: Monatskosten (mehrere Ablesungen pro Abrechnungsmonat möglich)

**Abrechnungsmonat**:
Der Monat, dem eine Ablesung für die Abrechnung zugeordnet ist (`periods.monat`) - unabhängig vom Ablesedatum, das nur der tatsächliche Erfassungszeitpunkt ist. Wird beim Anlegen aus dem Ablesedatum vorbelegt, ist aber manuell überschreibbar; mehrere Ablesungen desselben Abrechnungsmonats (untermonatige Ablesungen) bleiben als eigene Datensätze erhalten, ihre Kosten werden für die Monatsanzeige summiert. Muss chronologisch monoton nicht-fallend mit dem Ablesedatum bleiben.
_Avoid_: Zeitraum, Monat (zu unspezifisch - Ablesedatum trägt ebenfalls einen Monat)

**Netzbezug**:
Die vom Stromnetz bezogene Energiemenge (Zähler `strom_gesamt`) - bereits netto nach PV-Eigenverbrauch, da PV-Eigenverbrauch nie durch diesen Zähler läuft.

**Nicht dem Netzbezug zugeordnet (PV)**:
Die kWh-Differenz zwischen dem Verbrauch eines Zwischenzählers (Wohnung 2 oder Wärmepumpe) und dem Anteil, den die Zuteilungslogik ihm tatsächlich vom Netzbezug angerechnet hat. Übersteigt der Zwischenzähler-Verbrauch den verbleibenden Netzbezug, kann die Differenz nur durch PV gedeckt worden sein - eine aus der Zuteilungslogik abgeleitete Näherung, kein gemessener PV-Wert (es gibt keinen PV-Eigenverbrauchszähler).
_Avoid_: PV-Anteil, PV-Ertrag (das ist die separate Einspeisevergütung, siehe unten)

**Wohnung 1 / Wohnung 2**:
Die zwei Hausparteien, auf die Kosten verteilt werden. Wohnung 1 trägt keine eigene Strom-Kostenposition - ihr Netzbezug bleibt implizit der Rest nach Wohnung 2 und Wärmepumpe.

## Heizung & Warmwasser

**WP-Strom (Heizung + Warmwasser)**:
Die der Wärmepumpe angerechnete Netzbezug-Menge (kWh) - die Kostenbasis für die Heizungskosten. Deckt Raumheizung UND Warmwasserbereitung gemeinsam ab; die Wärmemengenzähler messen nur Raumheizung, können den Anteil also nicht trennen.
_Avoid_: Heizungsstrom (verschleiert, dass Warmwasser mit drin steckt)

**Wärmeverbrauch**:
Die von den Wärmemengenzählern (`waerme_wohnung1`/`waerme_wohnung2`) gemessene Wärmemenge in MWh - reine Raumheizung, keine Warmwasserbereitung.

**Heizungs-Gewichtung**:
Der Split (70/30, 60/40 oder 50/50, Startwert 70/30), nach dem der WP-Strom zwischen Wärmeverbrauchs-Verhältnis und Wohnungsgrößen-Verhältnis der beiden Wohnungen gewichtet wird. Ein einzelner Stammdaten-Wert für alle Monate, weil der Mietvertrag die Verteilung für das ganze Jahr festlegt - nicht mehr je Ablesung gewählt. Eine Änderung wirkt rückwirkend auf alle Monate. Die Heizungsverteilung nutzt keine Personenzahl.

**Warmwasseraufbereitung**:
Ein Wasser-Begriff (Zähler `wasser_warmwasseraufbereitung`), nicht zu verwechseln mit "WP-Strom (Heizung + Warmwasser)" oben - hier geht es um Frischwasser-Verbrauch für die Aufbereitung, nicht um Wärmepumpen-Strom.

## PV

**Einspeisung / Einspeisevergütung**:
Die ins Netz eingespeiste PV-Überschussmenge (Zähler `strom_einspeisung`) und die dafür gezahlte Vergütung. Rein informativ, haus-weit, unabhängig von der Kostenverteilung - nicht zu verwechseln mit "Nicht dem Netzbezug zugeordnet (PV)" oben, das eine Kosten-interne Zuteilungsgröße ist.

## Fixkosten

**Stammdaten** (`/stammdaten`):
Die Seite für Werte, die sich selten ändern und nicht Teil einer monatlichen Erfassung sind: Wohnungsgröße/Flurstücksgröße je Wohnung, Wohnungsstatus, Mieter- und Vermieter-Angaben, das Umlagefähig-Flag je Kostenposition und die Flags "Stromverbrauch weiterberechnen" und "Wallbox weiterberechnen" (aktuelle Einzelwerte). Mieter-/Bewohnername, Zustellanschrift, Vermieter, Objektanschrift und IBAN sind personenbezogen: sie sind nur für angemeldete Nutzer sichtbar und änderbar (ohne gesetztes `LOGIN_PASSWORD` ist jeder angemeldet). Änderungen wirken sofort auf alle Monate, nicht eingefroren wie ein Ablesungs- oder Fixkosten-Eingabe-Wert.

**Wohnungsstatus**:
Je Wohnung "Vermietung" oder "Eigennutzung" (Stammdaten, Startwert Wohnung 1 Eigennutzung, Wohnung 2 Vermietung). Steuert nur die Jahresabrechnung (Mieter-Block, Rechtshinweise, Pflichtfelder und welche Posten darin stehen), nie das Dashboard, den fortlaufenden Saldo, die Kostenberechnung oder die Heizungs-Gewichtung. Welche Posten in der Abrechnung einer Wohnung stehen:

| Status der Wohnung | Block "Wallbox" (ihr Anteil) | Block "PV-Gutschrift" (nur Wohnung 1, wenn der Schalter der Ablesung an ist) |
|---|---|---|
| Eigennutzung | immer (das Flag "Wallbox weiterberechnen" gilt nicht) | immer |
| Vermietung | nur mit gesetztem Flag "Wallbox weiterberechnen" | nie |

Sind beide Wohnungen vermietet und das Flag ist aus, steht die Wallbox in keiner Abrechnung und die PV-Gutschrift nirgends. Die PV-Gutschrift folgt ihrer Zuordnung zu Wohnung 1, auch wenn Wohnung 1 vermietet und Wohnung 2 Eigennutzung ist. Kein Personenbezug, deshalb auch ohne Anmeldung sichtbar (nicht änderbar).
_Avoid_: Vermietungsstatus, Nutzungsart

**Mieter seit**:
Ein Monat-Feld je Wohnung in den Stammdaten: der Monat, in dem der aktuelle Mieter eingezogen ist. Nur bei Wohnungsstatus "Vermietung" sichtbar und wirksam, leer heißt "ab Erfassungsbeginn". Ein einzelner aktueller Wert wie der Mietername, nicht historisiert; als Mieter-Angabe personenbezogen und nur für angemeldete Nutzer sichtbar und änderbar. Bestimmt den Startmonat für den **Übertrag Vorjahre** der Monatstabelle in der Jahresabrechnung. Wer die Abrechnung des Vormieters nach dem Eintragen noch braucht, sichert sie vorher als PDF.
_Avoid_: Mietbeginn, Einzugsdatum (das Feld hat nur den Monat)

**Umlagefähig**:
Ein Ja/Nein-Flag je Kostenposition in den Stammdaten: Nur umlagefähige Positionen erscheinen in der Jahresabrechnung. Ein einzelner aktueller Wert ohne Jahresbezug, wirkt rückwirkend auf alle Jahre. Dashboard, Monatsverlauf und Jahreskarten ignorieren das Flag und zeigen weiterhin alle Positionen. Es ersetzt nicht die Berechnungslogik: Eine teilweise umlagefähige Position teilt der Nutzer über die Logik der Fixkosten-Eingabe auf. Keine Abweichung von ADR 0001, der nur einen jahresweisen Stammdaten-Block für Logik/Typ/Wert verworfen hat.
_Avoid_: umlegbar, abrechenbar (das ist die Vollständigkeit eines Abrechnungsjahres)

**Stromverbrauch weiterberechnen**:
Ein haus-weites Ja/Nein-Flag in den Stammdaten, ob der Stromverbrauch von Wohnung 2 (Haushaltsstrom, Zwischenzähler, ohne Wallbox) in der Jahresabrechnung weiterberechnet wird. Der Haushaltsstrom ist keine Betriebskostenart, deshalb ein eigenes Flag statt des Umlagefähig-Flags einer Kostenposition.

**Wallbox weiterberechnen**:
Ein haus-weites Ja/Nein-Flag in den Stammdaten (Startwert Ja, auch für bestehende Datenbanken), ob der Wallbox-Anteil einer vermieteten Wohnung in deren Jahresabrechnung steht. Unabhängig von "Stromverbrauch weiterberechnen" (Haushaltsstrom). Bei Eigennutzung gilt das Flag nicht, dort steht die Wallbox immer in der Abrechnung. Die Tabelle steht unter **Wohnungsstatus**.

**Fixkosten-Eingabe**:
Eine monatliche Erfassung, analog zur Ablesung, aber **genau eine je Monat** (beim Anlegen, beim Ändern des Monats und beim CSV-Import erzwungen, per UNIQUE-Index abgesichert; anders als bei Ablesungen gibt es keine untermonatigen Zusatzeingaben): Personenzahl je Wohnung (Dezimalzahl mit höchstens 2 Nachkommastellen, 0 erlaubt; eigenständig, nicht die der Ablesung), für Wohnungen mit Status Vermietung Nebenkostenabschlag und **Miete / Mieteinnahmen** und für jede Kostenposition ihre Berechnungslogik, ihr Typ und ihr Wert. Anders als die Ablesung hängt sie nicht von einer Vorperiode ab (kein Verbrauch, keine Zählerstand-Differenz); Logik/Typ/Wert sind, wie Personen und der Nebenkostenabschlag, je Eingabe unabhängig gespeichert und von der letzten Eingabe vorbelegt, nicht jahresweise zentral gepflegt.
_Avoid_: Fixkosten-Eintrag, Fixkosten-Periode

**Kostenposition**:
Eine der festen Positionen (Grundsteuer, Wohngebäudeversicherung, Deichbeiträge, Abfallwirtschaft, Grundpreise Strom/Wasser/Abwasser/Internet, Wärmepumpen-Wartung, PV-Anlagen-Wartung, Streaming-Dienste, Sonstige Kosten) - Struktur so fix wie die Zähler, geseeded wie `meters`. Ihre Logik/Typ/Wert sind dagegen an der jeweiligen Fixkosten-Eingabe gepflegte Daten.

**Berechnungslogik**:
Die Regel, nach der eine Kostenposition auf Wohnung 1/2 aufgeteilt wird: Je Wohneinheit (50/50), Je anteiliges Flurstück, Je anteilige Wohnungsgröße, Je Anzahl Personen (aus derselben Fixkosten-Eingabe), oder vollständig Wohnung 1 bzw. Wohnung 2 (100/0).
_Avoid_: Verteilungsschlüssel, Split (das ist die Heizungs-Gewichtung, ein anderer Begriff)

**Typ (jährlich/monatlich)**:
Ob eine Kostenposition in dieser Eingabe einen Jahreswert hat, der für die Monatsanzeige automatisch durch 12 geteilt wird ("jährlich"), oder ob ihr Wert direkt der Monatsbetrag ist ("monatlich").

**Jahreswert**:
Der Jahresgesamtbetrag einer in dieser Eingabe "jährlich"-typisierten Kostenposition.

**Monatswert**:
Der einer Kostenposition für einen konkreten Monat tatsächlich zugerechnete Betrag - bei "jährlich" `Jahreswert / 12`, bei "monatlich" direkt der erfasste Wert.

**Flurstück / Flurstücksgröße**:
Die Grundstücksgröße je Wohnung (`apartments.flurstueck_groesse`), Grundlage der Berechnungslogik "Je anteiliges Flurstück" (z.B. für Deichbeiträge). Wie die Wohnungsgröße ein aktueller Einzelwert, nicht historisiert - auf der Stammdaten-Seite gepflegt.

**Nebenkostenabschlag**:
Die monatliche Vorauszahlung je Wohnung (Tabelle `nebenkosten_abschlaege`), an der jeweiligen Fixkosten-Eingabe erfasst wie ein monatlich-typisierter Wert, aber keine Kostenposition - deckt Fixkosten UND Verbräuche gemeinsam ab, zählt nicht in die Fixkosten-Summen und hat keine Berechnungslogik (direkter Wert je Wohnung, kein Split).
_Avoid_: Abschlag ohne "Nebenkosten"-Präfix (zu unspezifisch), Vorauszahlung

**Miete / Mieteinnahmen**:
Die monatliche Miete je Wohnung (Tabelle `mieten`), an der Fixkosten-Eingabe erfasst wie der Nebenkostenabschlag, aber reine Einnahmen-Information: keine Kostenposition, keine Berechnungslogik, geht in keinen Saldo, Übertrag und keine Abrechnung ein. Das Formularfeld heißt "Miete", die Anzeige (Jahressumme) "Mieteinnahmen". Wie der Abschlag nur für Wohnungen mit Status Vermietung erfasst (Pflicht, 0 erlaubt, vorbelegt aus der letzten Eingabe), bei Eigennutzung gibt es kein Feld. Ein früher gespeicherter Abschlag einer inzwischen eigengenutzten Wohnung zählt weiter und bleibt beim Korrigieren unverändert. Im CSV immer `abschlag_1`, `abschlag_2`, `miete_1`, `miete_2`, eine leere Zelle heißt nicht erfasst, der Import übernimmt Werte unabhängig vom Status.
_Avoid_: Kaltmiete, Mietzahlung

**Abrechnungszeitraum**:
Der Zeitraum einer Jahresabrechnung: das Kalenderjahr, im ersten Erfassungsjahr ab dem Datum der ersten Ablesung (dem Ausgangsstand, der selbst keinen Verbrauch liefert) bis 31.12. Fixkosten-Eingaben und Nebenkostenabschläge sind dort erst ab dem Folgemonat des Ausgangsstands nötig. Der Verbrauch jeder späteren Ablesung ist die Differenz zur vorherigen, deshalb ist er im Zeitraum immer berechenbar.

Auf der Abrechnungsseite lässt sich der Zeitraum per Häkchen auf volle Monate eines Jahres eingrenzen (Mieterwechsel unterjährig, Von-/Bis-Monat); der Standard bleibt das Kalenderjahr. Ein Monat gehört ganz in den Zeitraum, in den die Ablesung seines Abrechnungsmonats fällt, auch bei einem Wechsel mitten im Monat. Mieter und Anschrift kommen immer aus den aktuellen Stammdaten, die App speichert keine Abrechnung: den Stand vor dem Umschreiben als PDF sichern. Die Frist nach § 556 Abs. 3 BGB endet am Ende des zwölften Monats nach dem Zeitraumende. Ein Zeitraum liegt immer innerhalb eines Kalenderjahres.

**Mangel** (Abrechnungsprüfung):
Ein Grund, warum für ein Jahr und eine Wohnung noch keine Jahresabrechnung entstehen kann: fehlende oder unvollständige Ablesung (Teilstand) eines Abrechnungsmonats, fehlende oder mehrfache Fixkosten-Eingabe eines Monats, fehlende Pflicht-Stammdaten. Die Prüfung liefert alle Mängel auf einmal, jeweils mit dem Ort zum Beheben, statt einer Abrechnung. Ein fehlender Nebenkostenabschlag ist kein Mangel (zählt als 0).
_Avoid_: Fehler, Warnung

**Jahresabrechnung**:
Die Betriebskostenabrechnung einer Wohnung für einen Abrechnungszeitraum, live aus den vorhandenen Daten berechnet und nichts festgeschrieben. Sie zeigt nur umlagefähige Kostenpositionen, Heizung/Warmwasser und Wasser/Abwasser (Verbrauch) sowie, wenn das Stammdaten-Flag es erlaubt, den Stromverbrauch von Wohnung 2 als eigenen Block. Je Position steht der Anteil als Summe der je Monat auf den Cent gerundeten Monatsanteile, wie im Dashboard (daher sind wenige Cent Abweichung zu "Gesamtkosten mal Prozentsatz" möglich). Wechselt die Berechnungslogik einer Position im Zeitraum, gibt es je zusammenhängendem Zeitraum gleicher Logik eine Zeile. Eine Position, für die die abgerechnete Wohnung nichts zahlt (z. B. voll der anderen Wohnung zugerechnet), erscheint nicht.

**Übertrag Vorjahre**:
Der Saldo (Nebenkostenabschlag minus Kosten nach Abrechnungs-Logik) aller Monate vom Startmonat bis zum Monat vor dem Zeitraum der Jahresabrechnung, Anfangswert des kumulierten Saldos der Monatstabelle. Startmonat ist der spätere von "Mieter seit" (nur bei Vermietung) und dem Monat der ersten Ablesung. Hat ein Monat dieses Bereichs einen Mangel, ist er nicht berechenbar und entfällt mit Hinweis. Nicht der Dashboard-Saldo: Der zählt auch nicht umlagefähige Positionen. Im Code `abrechnung.Berechne` (`internal/abrechnung/berechnung.go`): Übertrag und Jahresabrechnung falten ihre Monate gleich (`calc.Monatswerte`) und summieren mit `calc.Verlauf`, der Übertrag ist dessen Startwert.
_Avoid_: Vortrag, Saldovortrag

**Anlage**:
Ein nummerierter Teil des Anhangs der Jahresabrechnung (1 Monatsverlauf und Saldo bis 7 Zählerstände), aufgelistet auf Seite 1 und im Druck nicht umbrechend. Anlage 7 steht allein im Querformat auf der letzten Seite.

**Jahressaldo**:
Vorauszahlungen (Nebenkostenabschläge des Abrechnungszeitraums, ein fehlender zählt als 0) minus die Betriebskosten der Jahresabrechnung minus den weiterberechneten Strom minus die Wallbox plus die PV-Gutschrift, jeweils nur, wenn der Posten laut Tabelle unter **Wohnungsstatus** in der Abrechnung steht. Der Übertrag Vorjahre zählt dieselben Posten. Positiv Guthaben, negativ Nachzahlung, null Ausgeglichen. Nicht der fortlaufend kumulierte Saldo des Dashboards, aber bei lückenlosen Daten und lauter umlagefähigen Positionen gleich dessen Änderung über das Jahr.
_Avoid_: Jahresergebnis

**Guthaben / Nachzahlung**:
Der fortlaufend seit Erfassungsbeginn kumulierte Saldo aus Nebenkostenabschlag minus Fixkosten minus Verbräuche einer Wohnung (kein Reset zum Jahreswechsel). Positiv heißt Guthaben, negativ Nachzahlung, exakt null Ausgeglichen. Im Code der Typ `calc.AbschlagSaldo` (`internal/calc/abschlagsaldo.go`), die Monatswerte und die Monat-für-Monat-Summierung (`calc.Monatswerte`, `calc.Verlauf`, darunter `calc.Saldoverlauf`; `internal/calc/monatsverlauf.go`) gelten für Dashboard und Monatsverlauf gleich, sie unterscheiden sich nur in den Posten.
