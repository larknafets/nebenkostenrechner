# 2. Keine gemeinsame Abstraktion für Jahreskarte/Verlauf (Wohnung vs. Wallbox/PV)

## Status

Angenommen (2026-09-09)

## Kontext

`internal/web/dashboard_fixkosten.go` enthält zwei Paare von Builder-Funktionen mit ähnlicher Form: `buildJahresCard`/`buildDashboardVerlauf` für die Wohnungen (Wohnung 1/Wohnung 2) und `buildSimpleJahresCard`/`buildSimpleVerlauf` für die haus-weiten, rein informativen Entitäten Wallboxen und PV-Anlage. Beide Paare laufen über `periodenKosten`, gruppieren nach Monat, akkumulieren Segmente, runden und skalieren prozentual gegen ein Maximum; beide nutzen dafür bereits den echten gemeinsamen generischen Helfer `gruppiereNachJahr`.

Eine Architektur-Review (zweiter Durchgang) prüfte, ob die beiden Paare zu einem gemeinsamen, generischen Builder zusammengeführt werden sollten - analog zu `gruppiereNachJahr`. Dabei zeigte sich: was tatsächlich 3-4-fach dupliziert ist, sind nur die kleinen "Maximum über alle Monate finden"-Schleifen (je 5-10 Zeilen, mit unterschiedlichem Feld-Selektor). Die größere, augenscheinlich duplizierte Form (Bucket → Akkumulieren → Runden → Prozent → Jahresgruppierung) trägt bei den Wohnungen zusätzlich Fixkosten/Kombiniert-Modus und den Nebenkostenabschlag-Saldo (`accumulateAbschlagSaldo`) - bei Wallboxen/PV-Anlage existiert das alles nicht, dort gibt es nur einen einzigen kWh-basierten Modus ohne Fixkosten- oder Saldo-Konzept.

## Entscheidung

Keine gemeinsame Abstraktion für `buildJahresCard`/`buildDashboardVerlauf` und `buildSimpleJahresCard`/`buildSimpleVerlauf`. Die Duplikation, die tatsächlich vorliegt (die Maximum-Schleifen), ist zu klein (4 Stellen à ~8 Zeilen), um eine generische Abstraktion zu rechtfertigen - anders als bei `gruppiereNachJahr`, wo die extrahierte Logik wirklich identisch war. Eine erzwungene gemeinsame Abstraktion über beide Pfade hätte genug optionale Verzweigungen gebraucht (Fixkosten ja/nein, Saldo ja/nein, variable Segmentanzahl), um selbst wieder zum flachen Modul zu werden - dazu kommt das reale Blast-Radius-Risiko, da beide Pfade sowohl vom Dashboard als auch von allen 3 Widget-Routen gelesen werden (Finanz-/Kostenberechnungscode).

## Konsequenzen

- `buildJahresCard`/`buildDashboardVerlauf` und `buildSimpleJahresCard`/`buildSimpleVerlauf` bleiben eigenständige, leicht duplizierte Funktionspaare.
- Eine künftige Architektur-Review sollte diese Zusammenführung nicht erneut vorschlagen, außer die Voraussetzungen ändern sich (siehe unten).
- Revisit-Kriterium: ein 3. numerischer Modus bei den Wohnungen oder eine 3. haus-weite "Simple"-Entität (weitere Wallbox/PV-artige Kategorie) würde die Duplikationsmasse erhöhen und eine erneute Prüfung rechtfertigen.
