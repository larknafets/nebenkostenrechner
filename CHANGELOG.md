# Changelog

## v0.21.0 - 2026-10-28

### Verbesserungen

- Geschlechtergerechte Sprache: Rollen und Felder heißen jetzt Mieter:in/Mieter:innen und Vermieter:in/Vermieter:innen (Anmeldung, Stammdaten, Berechtigungen, Jahresabrechnung). Die Kennwort-Variablen `VERMIETER_PASSWORD` und `MIETER_PASSWORD` bleiben unverändert.

- Dashboard: Die Badges für Fläche, Flurstück und Personen bei den Jahressummen sind kleiner und passen häufiger in eine Zeile.

- Personenzahl, Preise und die Häkchen "Wallboxen vollständig Wohnung 1" und "PV-Gutschrift Wohnung 1" werden nur noch bei der Fixkosten-Eingabe des Monats erfasst, der Ablesungs-Wizard hat nur noch drei Schritte (Strom, Wärme, Wasser). Gibt es für den Monat keine Fixkosten-Eingabe, zeigen Dashboard, Monatsverlauf, Ablesungen und Jahreskarte "Fixkosten fehlen" statt falscher Zahlen. Das Fixkosten-CSV hat sechs neue Spalten für Preise und Häkchen, das Ablesungs-CSV nimmt ältere Dateien weiter an (#359)

### Fehlerkorrekturen

- Wenn zwei Personen gleichzeitig speichern, kommt nicht mehr sporadisch der Fehler "database is locked", die Datenbank wartet jetzt bis zu 5 Sekunden. Außerdem wurden Fremdschlüssel bisher nur auf einer der Datenbankverbindungen geprüft, jetzt auf allen.

- Die Abrechnungsprüfung meldet eine Fixkosten-Eingabe ohne vollständige Preise jetzt als eigenen Mangel "Fixkosten-Eingabe ist unvollständig" mit dem Link "Fixkosten ergänzen". Bisher bestand eine solche Eingabe die Prüfung, der Verbrauch dieses Monats fiel aber still aus der Jahresabrechnung (#381)

- Dashboard und Monatsverlauf zeigen den Hinweis "Fixkosten fehlen" jetzt auch für Monate mit Teilstand und für die erste Ablesung, bisher erschien er erst, sobald die Ablesung berechenbar war (#380)

- Ablesungsseite, Speichern und CSV-Import zeigen "Fixkosten fehlen" jetzt auch bei Teilstand, erster Ablesung und unvollständiger Fixkosten-Eingabe, der Link heißt dann "Fixkosten ergänzen" und führt zur Eingabe des Monats (#383)

- Die Anmeldung bremst Kennwort-Raten: Nach 10 falschen Kennwörtern innerhalb einer Minute lehnt die App weitere Kennwort-Anmeldungen bis zum Ende der Minute ab (auch das richtige Kennwort) und zeigt dazu im Anmelde-Fenster einen Hinweis. Die Demo-Anmeldung setzt die Demo-Daten höchstens alle 10 Sekunden zurück.

### Wartung

- Update: Preise und Häkchen wandern beim ersten Start aus den Ablesungen in die Fixkosten-Eingaben desselben Monats, die Berechnung alter Monate kann sich dadurch ändern (Abweichungen stehen im Log). Monate ohne Fixkosten-Eingabe zeigen "Fixkosten fehlen", Ablesungen ohne Preise gelten als vollständig. Die Spalten für Preise, Häkchen und Personenzahl der Ablesung und die Tabelle der Ablesungs-Personenzahlen entfallen. Die Sicherung `.vor-schema-<alte Version>-<UTC-Zeit>` entsteht vor dem Start (#359)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.20.0 - 2026-10-27

### Neue Funktionen

- Stammdaten: In der neuen Tabelle "Berechtigungen" stellt der Vermieter je Recht Häkchen für Nicht angemeldet und Mieter ein (Standard wie bisher, dazu die Wärmepumpe) (#310)
- Die Berechtigungen wirken: Dashboard-Karten und Monatsübersichten, Ablesungen, Fixkosten und Stammdaten sowie die Menüpunkte und Buttons zum Anlegen folgen den Häkchen. Ohne Recht landet ein nicht angemeldeter Besucher im Anmelde-Overlay, ein Mieter auf der Seite "Nicht freigegeben" (#311)
- Anmeldung mit zwei Rollen: Neben dem Vermieter gibt es einen Mieter-Login mit eigenem Kennwort, das Anmeldeformular hat ein Auswahlfeld (Mieter, Vermieter, Demo), die Demo braucht kein Kennwort mehr (#309)
- Dashboard: Die Wärmepumpe hat einen eigenen Monatsverlauf wie die Wallboxen, auch ohne Anmeldung sichtbar (#305)
- Dashboard: Die Jahreskarte jeder Wohnung zeigt die Wallbox in einem eigenen Block mit Stromkosten in EUR und kWh, statt im Hauptbalken (#304)
- Dashboard: neue Jahreskarte "Wärmepumpe" mit Stromkosten und PV-Anteil, auch ohne Anmeldung sichtbar (#296)

### Verbesserungen

- Dashboard: Die Zahlen der Jahreskarte stehen in zwei Zeilen (Strom, Heizung/Ww, Wasser, darunter Fixkosten und PV), der Wallbox-Block zeigt das Wallbox-Symbol
- Dashboard: Die Monatsübersicht wählst du jetzt in einem Dropdown statt über Buttons, bei nur einer Auswahl entfällt es (#306)
- Dashboard: Der Balken der Jahreskarte zeigt nur noch den Verbrauch, die Fixkosten stehen weiter als Zeile und im Gesamtbetrag (#302)
- Dashboard: Die Karte "Wallboxen" erscheint nur, wenn eine Wohnung die Wallbox in der Abrechnung trägt (Eigennutzung oder weiterberechnet) (#297)

### Wartung

- **Kennwort neu setzen:** `LOGIN_PASSWORD` (Home Assistant: `login_password`) heißt jetzt `VERMIETER_PASSWORD` (`vermieter_password`) und wird nicht mehr gelesen. Nach dem Update das Kennwort neu setzen, sonst ist die App offen und jeder Besucher ist Vermieter. Neu ist das optionale `MIETER_PASSWORD` (`mieter_password`) (#309)
- Die Widget-Routen und der zweite Port (`WIDGET_LISTEN_ADDR`, Host-Port 8081 beim Release, 8082 beim Nightly) entfallen, Lovelace-Karten mit Webpage-Card auf diese Routen funktionieren nicht mehr (#301)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.19.0 - 2026-10-26

### Neue Funktionen

- Miete: Je vermieteter Wohnung erfasst du die monatliche Miete in der Fixkosten-Eingabe (Pflichtfeld, 0 erlaubt). Sie erscheint als Mieteinnahmen auf der Jahreskarte (nur für angemeldete Nutzer) und auf der Detailseite der Eingabe. Sie geht in keine Berechnung, keinen Saldo und keine Abrechnung ein (#277, #278, #281)
- Die Fixkosten-Eingabe erfasst den Nebenkostenabschlag nur noch für vermietete Wohnungen (Pflichtfeld, 0 erlaubt), bei Eigennutzung gibt es kein Feld mehr. Bereits gespeicherte Abschläge zählen weiter (#277)
- Der CSV-Export der Fixkosten enthält jetzt `miete_1` und `miete_2`, ältere Dateien bleiben importierbar (#277)

### Verbesserungen

- Die Jahreskarte einer vermieteten Wohnung zeigt unter Guthaben / Nachzahlung die Mieteinnahmen des Jahres. Sie erscheinen nur für angemeldete Nutzer, die Widgets zeigen sie nicht (#278)
- Die Detailseite einer Fixkosten-Eingabe zeigt die Miete vermieteter Wohnungen neben dem Nebenkostenabschlag, auch ohne Login (#281)
- Jahreskarten: PV-Gutschrift (Wohnung 1) und PV-Anteil (Wohnung 2) stehen jetzt auch mit 0. Die Wallbox-Zeile einer vermieteten Wohnung entfällt, wenn "Wallbox weiterberechnen" aus ist und der Betrag 0 ist (#276)
- Strom von Wohnung 1: Der Rest des Netzbezugs nach Wohnung 2, Wärmepumpe und Wallboxen steht jetzt als Kategorie "Strom" in Jahreskarte, Monatsverlauf, Verbrauchswerten, Gesamt, Saldo und Widgets (#279)
- Jahresabrechnung: Der Strom einer Wohnung steht bei Eigennutzung immer in der Abrechnung, bei Vermietung nur mit "Stromverbrauch weiterberechnen" (jetzt für Wohnung 1 und 2). Der Jahressaldo zählt ihn mit und gleicht damit wieder der Änderung im Dashboard (#280)
- Die Ablesungsseite zeigt bei den Stromkosten jetzt auch die Zeile "Wohnung 1" (Anteil und Kosten) (#275)

### Wartung

- Neue Datenbanktabelle für die Miete (leer angelegt, bestehende Daten bleiben erhalten). Vor dem Start mit dem neuen Schema sichert die App die Datenbank wie gewohnt (#277)
- Die Salden von Wohnung 1 sinken rückwirkend um die bisher aufgelaufenen Stromkosten, weil der Strom von Wohnung 1 jetzt mitgerechnet wird (#279)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.18.0 - 2026-10-22

### Neue Funktionen

- Fixkosten: Neue Position "Wartungskosten PV-Anlage" direkt unter "Wartungskosten Wärmepumpe", auf der Fixkosten-Eingabe und in den Stammdaten. Vorbelegt mit Berechnungslogik "Wohnung 1", Typ "jährlich" und nicht umlagefähig. Bestehende Eingaben erhalten die Position automatisch mit dem Wert 0. Ein älterer Fixkosten-CSV-Export lässt sich nicht mehr importieren, weil die Spalten der neuen Position fehlen

### Verbesserungen

- Stammdaten: Die Reihenfolge der Kostenpositionen hat eine übersichtlichere Darstellung: nummerierte Liste mit Trennlinien und Pfeil-Symbolen statt einfacher Textpfeile, mit sichtbarem Fokus für die Tastaturbedienung (#255)

- Stammdaten: Die Reihenfolge der Kostenpositionen lässt sich mit Hoch/Runter-Buttons ändern. Sie gilt für Fixkosten-Eingabe, Fixkosten-Anzeige und Abrechnung. Neue Positionen erscheinen künftig am Ende der Liste

### Fehlerkorrekturen

- Ablesung: Eine neue Ablesung mit früherem Ablesedatum oder früherem Abrechnungsmonat als die neueste Ablesung ließ sich anlegen und verschob die Vorperioden aller anderen. Jetzt wird sie mit einer Meldung abgelehnt, auch ein zweiter offener Teilstand lässt sich nicht mehr anlegen. Das gilt auch für den CSV-Import, der die Zeile mit dem Ablesedatum nennt (#260)

- Ablesung: Die Warnung "Verbrauch über 1 Monate" erschien schon bei genau einem Monat Abstand zur letzten Ablesung (Zeitzonenfehler im Browser). Jetzt warnt sie erst bei mehr als einem Monat, nennt "1 Monat" in der Einzahl und zeigt das Datum der letzten Ablesung als 01.09.2026 (#257)

- Ablesung korrigieren: Zählerwerte und Preise erschienen in der Vorbelegung mit Gleitkommaresten (z. B. 26520,69999999999 statt 26520,7). Jetzt zeigen die Felder die eingegebenen Werte ohne diese Reste (#254)

- Dashboard: Der Update-Hinweis (Punkt neben der Version unten rechts) erschien im Nightly-Kanal nie, weil nur Release-Versionen verglichen wurden. Jetzt zeigt ein Nightly-Stand den Punkt, sobald ein neueres Nightly vorliegt, der Link führt zum zugehörigen Commit. Nightlys vom selben Tag lassen sich nicht unterscheiden und lösen keinen Hinweis aus

- Eingaben: Eine Fehlermeldung (z. B. "Für September 2026 gibt es bereits eine Fixkosten-Eingabe") führte auf eine leere Fehlerseite, man musste zurückgehen und die Eingaben waren weg. Jetzt erscheint sie als Dialog über der Seite, die Eingaben bleiben stehen, das betroffene Feld ist rot markiert und nach dem Schließen aktiv. Gilt für Fixkosten, Ablesung, Stammdaten und die beiden CSV-Importe

### Wartung

- Go auf 1.27.3 angehoben (Sicherheitsupdate für `crypto/x509`, GO-2026-6809)

- Datenbank: Vor einer Schema-Änderung sichert die App die Datenbank neben der Datei als `<Datenbank>.vor-schema-<Version>-<Zeitstempel>`, die letzten drei Sicherungen bleiben. Beim ersten Start nach diesem Update entsteht einmalig eine Sicherung der bestehenden Datenbank. Zum Zurückspielen die App stoppen und die Sicherung auf den Namen der Datenbank kopieren. Scheitert die Sicherung, startet die App nicht und die Datenbank bleibt unverändert (#266)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.17.0 - 2026-10-18

### Verbesserungen

- Ablesung: Die Auswahl "Andere Ablesung anzeigen" zeigt jetzt den zugeordneten Monat und in Klammern das Ablesedatum (z. B. "August 2026 (15.08.2026)")

### Fehlerkorrekturen

- Dashboard: Der Personen-Schnitt der Jahressumme ist jetzt das Mittel der Personenzahlen aller Fixkosten-Eingaben des Jahres (ein Wert je Monat, z. B. 1,25). Vorher kam er aus den Ablesungen und zählte jede Ablesung gleich, auch mehrere im selben Monat

## v0.16.0 - 2026-10-14

### Neue Funktionen

- Ablesung: Unter "Wallboxen" lassen sich die Zählerstände von Wallbox Wohnung 1 und Wallbox Wohnung 2 optional eintragen (nur nachrichtlich, auch nur einer von beiden). Leere Felder gelten als "nicht erfasst" und machen die Ablesung nicht unvollständig; die Ablesungsseite zeigt sie mit Verbrauch, CSV-Export und -Import führen sie als zwei neue letzte Spalten (ältere Dateien bleiben importierbar). Im Formular steht die Wärmepumpe jetzt vor den Wallboxen (#224)
- Ablesung: Neuer Schalter "Wallboxen vollständig Wohnung 1" (aus der letzten Ablesung vorbelegt). Damit liegen alle Wallbox-Kosten bei Wohnung 1; ohne Schalter zahlt Wohnung 2 ihren Verbrauch laut Einzelzähler (gedeckelt auf den Netzbezug-Anteil) und Wohnung 1 den Rest. Die Ablesungsseite zeigt die Kosten je Wohnung und einen Hinweis, wenn die Einzelzähler um mehr als 5 % oder 2 kWh vom Zwischenzähler abweichen; der CSV-Export hat dafür eine neue letzte Spalte (#225)
- Ablesung: Neuer Schalter "PV-Gutschrift Wohnung 1" (Formular Schritt 4 beim Einspeisepreis, aus der letzten Ablesung vorbelegt). Ist er an, geht die Einspeisevergütung der Ablesung als Gutschrift an Wohnung 1 und erhöht deren Guthaben im Saldo ohne Deckel; ohne Schalter bleibt die Einspeisung haus-weit und informativ. Die Ablesungsseite zeigt den Schalter als Ja/Nein, der CSV-Export hat dafür eine neue letzte Spalte (#226)
- Dashboard und Widgets: Neue Kategorie "Wallbox" bei Wohnung 1 und 2 (Jahreskarte, Monatsverlauf, Saldo, immer sichtbar), in "Verbrauchswerte" mit den zugeordneten Netzbezug-kWh. Wohnung 1 zeigt die PV-Gutschrift als negative Zeile: Sie senkt die Gesamtkosten und erhöht das Guthaben im Saldo, steht im Monatsverlauf aber nur in "Kombiniert". Die Karten "Wallboxen" und "PV-Anlage" bleiben unverändert (#227)
- Jahresabrechnung: Neuer eigener Block "Wallbox" neben dem Stromverbrauch und bei Wohnung 1 ein Block "PV-Gutschrift". Welche Posten in der Abrechnung einer Wohnung stehen, hängt vom Wohnungsstatus ab: Eigennutzung zeigt Wallbox und PV-Gutschrift immer, Vermietung die Wallbox nur mit dem neuen Stammdaten-Schalter "Wallbox weiterberechnen" und die PV-Gutschrift nie. Jahressaldo und Übertrag Vorjahre zählen genau diese Posten. Die Anlagen Verbrauchsübersicht und Zählerstände führen die Wallbox-Einzelzähler (fehlende Stände als "-", ohne jeden Stand im Zeitraum entfallen sie), fehlende Einzelzähler sind nie ein Mangel (#228)

### Verbesserungen

- Ablesung und Fixkosten-Eingabe: Die Personenzahl lässt sich mit bis zu 2 Nachkommastellen eintragen (z. B. 1,25), sie fließt in die Warmwasser-Aufteilung und den Schlüssel "Personen" ein; negative Werte und mehr als 2 Nachkommastellen lehnt das Formular ab. Angezeigt wird sie überall ohne Auffüllen ("2", "1,25"), auch im Personen-Schnitt des Dashboards. Der CSV-Import beider Dateien nimmt Personenzahlen mit Komma an (ältere ganzzahlige CSVs bleiben gültig) und bricht bei negativen Werten oder mehr als 2 Nachkommastellen mit der Zeilennummer ab, ohne etwas zu importieren (#242, #243)

### Wartung

- Beim ersten Start nach dem Update legt die Datenbank automatisch zwei neue Zähler für die Wallbox-Einzelzähler an; bestehende Ablesungen, Kosten und Salden bleiben unverändert (#224)
- Beim ersten Start nach dem Update ergänzt die Datenbank eine Spalte für den Schalter "Wallboxen vollständig Wohnung 1"; bestehende Ablesungen bekommen "Nein", ihre Kosten und Salden ändern sich nicht (#225)
- Beim ersten Start nach dem Update ergänzt die Datenbank eine Spalte für den Schalter "PV-Gutschrift Wohnung 1"; bestehende Ablesungen bekommen "Nein", ihre Kosten und Salden ändern sich nicht (#226)
- Beim ersten Start nach dem Update ergänzt die Datenbank eine Spalte für den Stammdaten-Schalter "Wallbox weiterberechnen"; bestehende Datenbanken bekommen "Ja", an bestehenden Ablesungen und am Dashboard ändert sich nichts (#228)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung

## v0.15.1 - 2026-10-10

### Wartung

- Go-Version auf 1.27.2 angehoben, behebt Sicherheitslücken in der Go-Standardbibliothek (net/http, net/textproto, crypto/tls)

## v0.15.0 - 2026-10-07

### Neue Funktionen

- Neue Anlage "Zählerstände" als letzte Seite der Abrechnung im Querformat: alle Zählerstände je Ablesung mit Ablesedatum, zugeordnetem Abrechnungsmonat und dem Ausgangsstand (#206)

- Neue Anlage "Monatsverlauf und Saldo" im Anhang der Abrechnung: je Monat Fixkosten, Verbrauchskosten und Nebenkostenabschlag mit kumuliertem Saldo, damit der Verlauf von Guthaben oder Nachzahlung nachvollziehbar ist (#205)

- Neues Feld "Mieter seit" in den Stammdaten für vermietete Wohnungen: der Einzugsmonat des aktuellen Mieters, nur für angemeldete Nutzer sichtbar (#204)

### Verbesserungen

- Die Monatstabelle der Abrechnung (Anlage 1) zeigt oben den Übertrag Vorjahre und rechnet den Saldo darauf weiter. Er zählt ab "Mieter seit" bzw. dem Beginn der Erfassung. Hat ein Vormonat einen Mangel, steht stattdessen der Hinweis "Übertrag nicht berechenbar". Beim Einzug eines neuen Mieters (Mieter seit gleich Beginn des Zeitraums) startet der Saldo bei 0 (#207)

- Der Anhang der Abrechnung ist in nummerierte Anlagen gegliedert, auf Seite 1 steht eine Anlagenliste, und keine Anlage bricht im Druck mitten in der Tabelle um. Die Verbrauchsübersicht steht jetzt als Anlage 6 am Ende des Anhangs (#203)

### Wartung

- Die Datenbank bekommt beim Start automatisch die neue Spalte "Mieter seit" für Wohnungen, bestehende Daten bleiben unverändert (#204)
- Interne Aufräumarbeiten am Code ohne sichtbare Änderung
- Aktualisierung der Abhängigkeiten

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
