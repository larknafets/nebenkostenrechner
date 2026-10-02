# Changelog

## v0.14.2 - 2026-10-02

### Fehlerkorrekturen

- Ablesungen und Fixkosten-Details passen jetzt aufs Handy: der Zeitraum entfällt, die Logik steht unter der Position, das Badge "unvollständig" bricht um.

## v0.14.1 - 2026-10-02

### Verbesserungen

- Die App fragt beim Start einmal nach dem neuesten Release. Der Update-Punkt zeigt ein neues Release dadurch schon beim ersten Aufruf der Seite an

## v0.14.0 - 2026-10-02

### Neue Funktionen

- Die Abrechnung lässt sich auf volle Monate eines Jahres eingrenzen, zum Beispiel für einen Mieterwechsel mitten im Jahr. Ein Häkchen "Abweichender Zeitraum" blendet Von- und Bis-Monat ein, der Standard bleibt das Kalenderjahr. Die Frist nach § 556 Abs. 3 BGB richtet sich nach dem Ende des gewählten Zeitraums (#189)

### Verbesserungen

- Von- und Bis-Monat stehen in der Abrechnung nebeneinander

## v0.13.1 - 2026-10-01

### Verbesserungen

- In der Navigation steht "Stammdaten" jetzt vor "Abrechnung"
- Der Hinweis "– keine Ablesung –" bzw. "– keine Fixkosten-Eingabe –" im Monatsverlauf steht mittig und endet mit einem Strich

### Fehlerkorrekturen

- Der Update-Punkt in der Fußzeile war immer zu sehen, auch ohne neues Release. Jetzt erscheint er nur noch, wenn ein Update bekannt ist
- Das Add-on in Home Assistant zeigte nach einem Release teils die Version "main". Es zeigt wieder die richtige Versionsnummer

## v0.13.0 - 2026-10-01

### Neue Funktionen

- Neue Seite "Abrechnung": Jahresabrechnung je Wohnung mit Auswahl von Jahr und Wohnung, Ergebnis als Guthaben oder Nachzahlung, Hinweisen zur Frist, Anhang (Verbrauch, Heizung je Monat, Bezugsgrößen, Personen), A4-Druck und Mobilansicht (#175, #176, #177)
- Vor der Abrechnung prüft die App, ob alle Daten da sind, und nennt bei Lücken jeden Mangel mit Link zum Beheben (#174)
- Stammdaten: Wohnungsstatus ("Vermietung" oder "Eigennutzung"), Mieter- und Vermieter-Angaben, IBAN und Kontoinhaber (#170, #178)
- Stammdaten: je Kostenposition das Flag "Umlagefähig", dazu "Stromverbrauch weiterberechnen" für Wohnung 2 (#169)
- Die Heizungs-Gewichtung ist ein zentraler Wert in den Stammdaten statt je Ablesung (#172)

### Verbesserungen

- Der Ablesungs-Assistent hat eine neue Reihenfolge: Strom, Wärme, Wasser, bei Strom Netzbezug (1.8.0), Einspeisung (2.8.0), Wohnung 2, Wallboxen, Wärmepumpe. Alle Einheiten stehen in Klammern (#178)
- Die Boxen der Stammdaten haben gleiche Abstände, das IBAN-Feld ist größer
- Es gibt je Monat genau eine Fixkosten-Eingabe, ein doppelter Monat wird beim Speichern abgelehnt (#173)

### Fehlerkorrekturen

- Die Zählernamen sind überall einheitlich (#179)

### Wartung

- Beim ersten Start nach dem Update erweitert die App die Datenbank um die neuen Felder für Stammdaten und Abrechnung. Vorher eine Sicherung der Datenbank anlegen
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.12.0 - 2026-09-18

### Neue Funktionen

- Neue Kostenpositionen "Streaming-Dienste" und "Sonstige Kosten", dazu die Berechnungslogik "Wohnung 1" und "Wohnung 2" (Kosten vollständig einer Wohnung zugerechnet)

### Verbesserungen

- Der Update-Hinweis wird auch bei Bedienung des Dashboards geprüft, nicht nur beim Laden der Seite
- Der Umschalter für das Farbschema ist ein einzelnes Symbol, das bei jedem Klick durch System, Hell und Dunkel wechselt

## v0.11.2 - 2026-09-11

### Fehlerkorrekturen

- Die Abschlag-Balken im Monatsverlauf brechen auf dem Smartphone jetzt um, statt über den Rand zu laufen

## v0.11.1 - 2026-09-10

### Wartung

- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.11.0 - 2026-09-09

### Neue Funktionen

- CSV-Export und CSV-Import für die Fixkosten-Eingaben (#132, #133)

## v0.10.2 - 2026-09-09

### Fehlerkorrekturen

- Das Login-Kennwort aus den Add-on-Optionen wird jetzt über die Supervisor-Schnittstelle gelesen. Vorher wurde es im Add-on nicht erkannt

## v0.10.1 - 2026-09-09

### Verbesserungen

- Zählerstände werden mit drei Nachkommastellen angezeigt

## v0.10.0 - 2026-09-07

### Neue Funktionen

- Teilstand: Eine Ablesung lässt sich unvollständig speichern und später vervollständigen, auch ohne Anmeldung. Ein geführter Assistent zeigt, was noch fehlt
- Optionales Login-Kennwort (`LOGIN_PASSWORD`): Ohne Anmeldung bleiben Ansehen und Erfassen von Ablesungen möglich, geschützt sind persönliche Angaben und Änderungen
- Demo-Modus: Ein Login ohne Kennwort führt auf eine eigene Demo-Datenbank mit 39 Monaten Testdaten, mit Banner. Sie wird bei jedem Demo-Login zurückgesetzt

### Fehlerkorrekturen

- Der Link "Abmelden" erschien bei leerem Kennwort neben "Anmelden". Er erscheint nur noch, wenn man angemeldet ist
- Rechte im Dashboard stimmen wieder: Eine Ablesung lässt sich auch ohne Anmeldung erfassen
- Das leere Dashboard zeigt den Update-Hinweis wieder
- Beim Anmelden wird der jeweils andere Sitzungs-Cookie sauber entfernt

### Wartung

- Die Preise einer Ablesung dürfen in der Datenbank leer sein, damit ein Teilstand gespeichert werden kann
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.9.0 - 2026-09-06

### Neue Funktionen

- Fixkosten-Formular: alle Positionen sind bearbeitbar mit Logik und Typ, die Werte kommen vom Vormonat
- Nebenkostenabschlag mit Guthaben- und Nachzahlung-Anzeige im Dashboard und im Reiter "Abschlag" der Home-Assistant-Widgets
- Auf der Karte von Wohnung 2 steht die Zeile "Nicht dem Netzbezug zugeordnet" (PV-kWh)

### Verbesserungen

- Die Speichern-Buttons der Stammdaten sind erst nach einer Änderung aktiv

### Fehlerkorrekturen

- Zwischen Wert und Einheit (kWh, m², m³, MWh) sowie vor dem Euro-Zeichen entsteht kein Zeilenumbruch mehr
- Der Nebenkostenabschlag erscheint auf der Detailseite einer Fixkosten-Eingabe
- Das Guthaben ignoriert Monate ohne Fixkosten-Eingabe und rechnet bei Lücken korrekt

### Wartung

- Die Fixkosten-Werte tragen jetzt Logik und Typ in der Datenbank. Bisherige Jahreswerte werden beim Start übernommen. Vorher eine Sicherung anlegen
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.8.1 - 2026-09-05

### Wartung

- Das Home-Assistant-Add-on legt die Datenbank jetzt unter `addon_configs` ab. Eine vorhandene Datenbank wird beim Start dorthin übernommen

## v0.8.0 - 2026-09-05

### Neue Funktionen

- Ein Hinweis in der Fußzeile des Dashboards zeigt ein neues Release auf GitHub
- Jede Ablesung lässt sich einem Abrechnungsmonat zuordnen (#86)
- Kombinierte Widget-Übersicht für Home Assistant mit Jahressumme und Verbrauchswerten

### Fehlerkorrekturen

- Die Balken im Monatsverlauf überschreiten nie 100 %
- Der Rücklink erscheint nur noch auf Detailseiten
- Ein Tausenderpunkt wird beim CSV-Import richtig gelesen (#87)

### Wartung

- Der Container läuft als nonroot-Benutzer statt als root
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.7.0 - 2026-09-04

### Neue Funktionen

- Widget-Routen ohne Ingress, zum Einbinden in Home-Assistant-Dashboards
- In der Zählertabelle steht die absolute Differenz zur vorherigen Ablesung

### Verbesserungen

- Icon-Badges für Wohnungsgröße, Flurstück und Personen sowie das deutsche Tausendertrennzeichen
- Verbrauchswerte stehen immer mit genau zwei Nachkommastellen
- Die Jahressummen-Karte zeigt Flurstücksgröße und Personen-Schnitt
- Die Legende folgt dem aktiven Modus des Monatsverlaufs, die Verbrauchswerte sind farbig

### Fehlerkorrekturen

- Die Verbrauchswerte im Dashboard zeigen die tatsächlichen kWh statt des durch PV gedeckelten, abgerechneten Anteils

## v0.6.0 - 2026-09-03

### Verbesserungen

- Der PV-Anteil steht in der Jahressummen-Karte der Wallboxen
- Verbrauchswerte und Wallbox zeigen die tatsächlichen kWh, nicht nur die abgerechneten Werte
- Doppelte Kennzahlen-Boxen in den Tab-Ansichten des Dashboards sind entfernt

### Wartung

- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.5.1 - 2026-09-03

### Fehlerkorrekturen

- Jährliche Fixkosten-Werte blieben leer, wenn das Zahlenfeld einen ungültigen Wert enthielt. Die Bezeichnungen der Kostenpositionen sind abgeglichen

## v0.5.0 - 2026-09-03

### Neue Funktionen

- Wallbox und PV-Anlage erscheinen im Dashboard, die Seiten-Navigation ist vereinheitlicht (#67)

### Wartung

- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.4.0 - 2026-09-03

### Neue Funktionen

- Neues Dashboard mit Jahressummen und einem Monatsverlauf in vier Modi (#60)
- Fixkosten: Seite `/fixkosten` zum Anlegen, Ändern und Löschen, mit Aufteilung der Kosten auf beide Wohnungen (#60)
- Neue Stammdaten-Seite für Wohnungsgröße und Flurstücksgröße (#61)
- Einheitliche obere Navigation auf jeder Seite

### Verbesserungen

- Der Strom der Wärmepumpe wird in der Heizungskosten-Tabelle je Wohnung aufgeteilt

### Wartung

- Neue Tabellen in der Datenbank für die Fixkosten
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.3.0 - 2026-09-01

### Wartung

- Das Projekt heißt jetzt "nebenkostenrechner" (vorher "nebenkosten-energierechner"). Das Home-Assistant-Add-on bekam dabei einen neuen Slug (#51, #52)

## v0.2.2 - 2026-09-01

### Neue Funktionen

- CSV-Export (#53) und CSV-Import zum Start (#54) für Ablesungen

### Verbesserungen

- Der Zeitraum steht neben dem Ablesedatum in der Übersicht und in der Auswahl der Detailseite
- Die Heizungskosten zeigen den Wärmepumpen-Strom in kWh, die Stromkosten den PV-Anteil (#50)

## v0.2.1 - 2026-09-01

### Fehlerkorrekturen

- Beim Korrigieren einer Ablesung gingen bei Lücken stillschweigend Zählerstände und Belegung verloren

## v0.2.0 - 2026-09-01

### Neue Funktionen

- Der Umschalter für das Farbschema bietet System, Hell und Dunkel, im Dashboard steht ein Versions-Badge (#48, #49)

### Verbesserungen

- Die Seite "Wie wird gerechnet?" erklärt die PV-Einspeisung

## v0.1.5 - 2026-09-01

### Neue Funktionen

- Die Einspeisung ins Netz wird erfasst (#47)

### Fehlerkorrekturen

- Beim Korrigieren der ältesten Ablesung fehlten Preise und Personenzahl in der Vorbelegung (#47)

## v0.1.4 - 2026-09-01

### Neue Funktionen

- Jede Ablesung lässt sich bearbeiten und löschen, eine Übersicht listet alle Ablesungen (#41, #43, #44, #45)

### Verbesserungen

- "Neue Ablesung erfassen" und "Korrigieren" sind Buttons, "Wie wird gerechnet?" steht nur noch im Dashboard (#46)
- Die Detailansicht zeigt den Zeitraum, die Navigations-Links sind einheitlich groß

## v0.1.3 - 2026-09-01

### Verbesserungen

- Die Formularfelder der Ablesung erlauben beliebig viele Nachkommastellen

## v0.1.2 - 2026-09-01

### Neue Funktionen

- Der Verlauf lässt sich zwischen Euro und Verbrauch umschalten (#39)
- Ablesungen lassen sich korrigieren, das Dashboard hat einen Link, Zahlen erscheinen im deutschen Format (#34, #35, #36)

### Verbesserungen

- Der Verbrauch wird auf höchstens zwei Nachkommastellen gerundet, Euro-Beträge haben immer zwei (#37, #38, #40)
- Die Wohnungsgröße ist nicht mehr fest einprogrammiert
- Ein fester Tooltip erscheint bei Balkenstücken, die zu schmal für den Euro-Text sind

### Wartung

- Der Container läuft als root statt als nonroot-Benutzer der Distroless-Basis

## v0.1.1 - 2026-08-29

### Neue Funktionen

- Erste veröffentlichte Version: monatliche Ablesungen erfassen und die Kosten für Strom, Heizung und Wasser auf zwei Wohnungen verteilen, als Home-Assistant-Add-on

### Wartung

- Das Add-on-Repository `larknafets/ha-addons` wird bei jedem Release automatisch aktualisiert
