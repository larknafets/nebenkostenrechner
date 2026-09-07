# Nebenkostenrechner

Monatliche Nebenkostenabrechnung für ein Zweifamilienhaus mit Wärmepumpe und PV-Anlage: Strom-, Heizungs- und Wasserkosten werden aus monatlich erfassten Zählerständen berechnet und auf Wohnung 1/Wohnung 2 verteilt.

## Language

**Ablesung**:
Eine monatliche Erfassung: Zählerstände, Preise, Personenzahl je Wohnung und Heizungs-Gewichtung zu einem Ablesedatum. Im Code auch "Periode" genannt.
_Avoid_: Eintrag, Datensatz

**Teilstand**:
Eine Ablesung, bei der nicht alle Felder gefüllt sind (Zählerstände, Preise, Personenzahl, oder Abrechnungsmonat - nur Ablesedatum und Heizungs-Gewichtung sind immer zwingend). Fließt nicht in die Berechnung ein - erst nach Vervollständigung wird sie zur regulären Ablesung. Nur die neueste Ablesung darf ein Teilstand sein.
_Avoid_: unvollständige Ablesung (funktioniert, aber "Teilstand" ist der feste Begriff)

**Zählerstand**:
Der am Ablesedatum abgelesene Wert eines Zählers - ein kumulativer Punktwert, kein Verbrauch.
_Avoid_: Messwert, Reading

**Verbrauch**:
Die Differenz zwischen dem Zählerstand einer Ablesung und dem der chronologisch nächst-älteren Ablesung desselben Zählers.

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
Der pro Ablesung gewählte Split (70/30, 60/40 oder 50/50), nach dem der WP-Strom zwischen Wärmeverbrauchs-Verhältnis und Wohnungsgrößen-Verhältnis der beiden Wohnungen gewichtet wird.

**Warmwasseraufbereitung**:
Ein Wasser-Begriff (Zähler `wasser_warmwasseraufbereitung`), nicht zu verwechseln mit "WP-Strom (Heizung + Warmwasser)" oben - hier geht es um Frischwasser-Verbrauch für die Aufbereitung, nicht um Wärmepumpen-Strom.

## PV

**Einspeisung / Einspeisevergütung**:
Die ins Netz eingespeiste PV-Überschussmenge (Zähler `strom_einspeisung`) und die dafür gezahlte Vergütung. Rein informativ, haus-weit, unabhängig von der Kostenverteilung - nicht zu verwechseln mit "Nicht dem Netzbezug zugeordnet (PV)" oben, das eine Kosten-interne Zuteilungsgröße ist.

## Fixkosten

**Stammdaten** (`/stammdaten`):
Die Seite für Werte, die sich selten ändern und nicht Teil einer monatlichen Erfassung sind: Wohnungsgröße/Flurstücksgröße je Wohnung (aktuelle Einzelwerte). Änderungen wirken sofort auf alle Monate, nicht eingefroren wie ein Ablesungs- oder Fixkosten-Eingabe-Wert.

**Fixkosten-Eingabe**:
Eine monatliche Erfassung, analog zur Ablesung: Personenzahl je Wohnung (eigenständig, nicht die der Ablesung) und für jede der 14 Kostenpositionen ihre Berechnungslogik, ihr Typ und ihr Wert. Anders als die Ablesung hängt sie nicht von einer Vorperiode ab (kein Verbrauch, keine Zählerstand-Differenz); Logik/Typ/Wert sind, wie Personen und der Nebenkostenabschlag, je Eingabe unabhängig gespeichert und von der letzten Eingabe vorbelegt, nicht jahresweise zentral gepflegt.
_Avoid_: Fixkosten-Eintrag, Fixkosten-Periode

**Kostenposition**:
Eine der 14 festen Positionen (Grundsteuer, Wohngebäudeversicherung, Deichbeiträge, Abfallwirtschaft, Grundpreise Strom/Wasser/Abwasser/Internet, Wärmepumpen-Wartung) - Struktur so fix wie die Zähler, geseeded wie `meters`. Ihre Logik/Typ/Wert sind dagegen an der jeweiligen Fixkosten-Eingabe gepflegte Daten.

**Berechnungslogik**:
Die Regel, nach der eine Kostenposition auf Wohnung 1/2 aufgeteilt wird: Je Wohneinheit (50/50), Je anteiliges Flurstück, Je anteilige Wohnungsgröße, oder Je Anzahl Personen (aus derselben Fixkosten-Eingabe).
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

**Guthaben / Nachzahlung**:
Der fortlaufend seit Erfassungsbeginn kumulierte Saldo aus Nebenkostenabschlag minus Fixkosten minus Verbräuche einer Wohnung (kein Reset zum Jahreswechsel). Positiv heißt Guthaben, negativ Nachzahlung, exakt null Ausgeglichen. Im Code der Typ `AbschlagSaldo` (`internal/web/abschlag_saldo.go`).
