# 5. Sonderkosten als eigener Posten statt Pseudo-Kostenposition

## Status

Angenommen (2026-11-02)

## Kontext

Eine Rechnung, die in keine der 14 Kostenpositionen passt (defekte Heizungspumpe, neue Spülmaschine), soll in Saldo und Jahresabrechnung eingehen können. Naheliegend wäre eine 15. Kostenposition "Sonderkosten" mit einem Wert in der Fixkosteneingabe des Monats. Dagegen spricht, dass die Struktur einer Kostenposition (Typ jährlich/monatlich, Jahreswert durch 12, ein Wert je Fixkosteneingabe, Umlagefähig-Flag in den Stammdaten) zu einzelnen Rechnungen nicht passt: Es gibt mehrere Rechnungen je Monat, jede mit eigenem Betreff, eigener Firma und eigener Umsetzung, und der Betrag fällt voll in einen Monat.

## Entscheidung

Sonderkosten sind ein eigener Posten mit eigener Tabelle `sonderkosten` (Schema-Version 6), keine Kostenposition. Jeder Eintrag trägt sein Umlegbar-Häkchen, seine Umsetzung (Haus, Wohnung 1, Wohnung 2) und bei Haus seine Berechnungslogik selbst. Berechnung und Anzeige behandeln umlegbare Einträge wie eine weitere Zeile der Fixkosten ihres Abrechnungsmonats (`calc.SonderkostenMonate`, `calc.Monatswerte`), die Jahresabrechnung zeigt sie als Zeile "Sonderkosten" mit Anlage 8. Die Kostenpositionen bleiben die feste, geseedete Liste.

## Konsequenzen

- Eine neue Auswertung oder Anzeige von Fixkosten muss Sonderkosten gesondert einbeziehen, sie stehen nicht in der Liste der Kostenpositionen und nicht in der Fixkosteneingabe.
- Das Umlagefähig-Flag und die Typ-/Jahreswert-Logik der Kostenpositionen gelten für Sonderkosten nicht. Das Häkchen "umlegbar" gilt je Eintrag und entscheidet allein, der Wohnungsstatus nicht (wie bei Umlagefähig).
- Neue Berechnungslogiken, die für Kostenpositionen entstehen, erreichen Sonderkosten nur, wenn sie in `logikenHaus` (`internal/store/sonderkosten.go`) aufgenommen werden.
- Revisit-Kriterium: Sollen Sonderkosten über mehrere Monate verteilt oder jährlich wiederkehrend werden, passt eine Kostenposition besser, und die Entscheidung ist neu zu prüfen.
