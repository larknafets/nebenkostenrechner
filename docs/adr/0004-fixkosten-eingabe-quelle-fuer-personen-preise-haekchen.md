# 4. Die Fixkosten-Eingabe ist die Quelle für Personenzahl, Preise und Häkchen

## Status

Angenommen (2026-10-28)

## Kontext

Personenzahl je Wohnung, die vier Preise (Strom, Frischwasser, Abwasser, Einspeisung) und die zwei Häkchen ("Wallboxen vollständig Wohnung 1", "PV-Gutschrift Wohnung 1") wurden bei jeder Ablesung eingetragen, obwohl sie je Monat gelten. Die Fixkosten-Eingabe trug schon eine eigene Personenzahl. Damit gab es zwei Wahrheiten für die Personenzahl, und Preise und Häkchen mussten bei jeder Ablesung neu gepflegt werden. Ein Teilstand entstand schon, weil ein Preis oder eine Personenzahl fehlte, nicht weil ein Zählerstand fehlte.

## Entscheidung

- Die Fixkosten-Eingabe des Abrechnungsmonats ist die einzige Quelle für Personenzahl, vier Preise und beide Häkchen. Bei der Ablesung entfallen alle sieben Felder samt Wizard-Schritt "Personen & Preise".
- Die Kostenrechnung löst die Werte über den Abrechnungsmonat der Ablesung auf. Mehrere Ablesungen im selben Monat nutzen dieselben Werte (genau eine Eingabe je Monat).
- Gibt es für den Monat keine Fixkosten-Eingabe oder fehlen in ihr Preise, ist die Ablesung nicht berechenbar ("Fixkosten für <Monat> fehlen"), ohne Rückgriff auf einen anderen Monat. Der Grund steht neben dem Teilstand. Eine Ablesung ist vollständig mit Ablesedatum, Abrechnungsmonat und Pflicht-Zählerständen.
- Beim Update übernimmt die Migration Preise und Häkchen aus den Ablesungen desselben Monats in die vorhandenen Eingaben (jeder Preis vom jüngsten nicht leeren Wert, die Häkchen von der jüngsten Ablesung), loggt Abweichungen und löscht danach die sechs Spalten der Ablesung und die Tabelle der Ablesungs-Personenzahlen. Vor allem Zerstörenden legt die App eine Sicherung `.vor-schema-<alte Version>-<UTC-Zeit>` an.

## Alternativen

- **Beide Quellen behalten** (Ablesung als Vorgabe, Fixkosten als Überschreibung): verworfen, weil es die zwei Wahrheiten nur verlagert und bei jeder Ablesung weiter Pflege kostet.
- **Rückgriff auf die letzte Fixkosten-Eingabe** bei fehlendem Monat: verworfen, weil eine Zahl später springt, sobald die echte Eingabe nachgetragen wird. "Nicht berechenbar" ist ehrlicher als eine falsche Zahl.
- **Spalten der Ablesung behalten, nur nicht mehr lesen**: verworfen, der Eigentümer wollte keine ungenutzten Daten behalten.
- **Nur das Wasser offen lassen**, wenn die Personenzahl fehlt: verworfen, weil auch Preise und Häkchen fehlen können und ein einziger Zustand einfacher zu erklären ist.

## Konsequenzen

- Schwer umkehrbar: Die Preise, Häkchen und Personenzahlen der Ablesungen werden gelöscht. Das Zurück geht nur über die Sicherung vor dem Start, ein Tippfehler im neuen Fixkosten-Formular wirkt auf alle Ablesungen des Monats.
- Überraschend: Die Kosten einer Ablesung hängen an der Fixkosten-Eingabe, nicht an der Ablesung selbst. Wer zuerst abliest, sieht bis zur Fixkosten-Eingabe "Fixkosten fehlen". Dashboard-Banner, Zeile im Monatsverlauf und "vorläufig" an der Jahreskarte weisen darauf hin.
- Trade-off: Der übliche Ablauf (erst ablesen, dann Fixkosten) bleibt möglich, dafür sind Ablesungen ohne Fixkosten nicht berechenbar statt mit alten Werten gerechnet.
- Die Berechnung alter Monate kann sich bei der Übernahme ändern, wenn mehrere Ablesungen eines Monats voneinander abwichen.
- Eine künftige Session sollte Personenzahl, Preise oder Häkchen nicht wieder an der Ablesung ablegen und keinen Rückgriff auf andere Monate einbauen.
