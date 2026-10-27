# 3. Rollen und Logins: Nicht angemeldet, Mieter, Vermieter

## Status

Angenommen (2026-10-27)

## Kontext

Bisher gab es ein Kennwort (`LOGIN_PASSWORD`, im Add-on `login_password`): Wer es kannte, sah alles und durfte alles, wer es nicht kannte, sah eine feste, eingeschränkte Ansicht. Ein Mieter, der Zahlen sehen soll, aber keine Abrechnung, hatte keinen eigenen Zugang. Die Berechtigungen waren im Code fest verdrahtet.

## Entscheidung

- Drei Rollen: **Nicht angemeldet**, **Mieter**, **Vermieter**. Der Vermieter sieht und darf immer alles. Für Nicht angemeldet und Mieter stellt er in den Stammdaten je Recht ein Häkchen ein.
- Zwei Kennwörter: `VERMIETER_PASSWORD` und `MIETER_PASSWORD` (Home Assistant: `vermieter_password` und `mieter_password`). Es gibt keinen Alias für `LOGIN_PASSWORD`: Nach dem Update ist das Kennwort neu zu setzen.
- Ein Mieter-Login gilt für das ganze Haus, nicht je Wohnung.
- Das Anmeldeformular hat ein Auswahlfeld (Mieter, Vermieter, Demo) und ein Kennwortfeld. Die Auswahl bestimmt die Rolle, Demo braucht kein Kennwort.
- Je Rolle ein eigenes Sitzungs-Cookie, signiert mit dem Kennwort der Rolle. Ein geändertes Kennwort meldet nur diese Rolle ab.
- Ein Mieter-Kennwort ohne Vermieter-Kennwort wird ignoriert (Log-Warnung). Ohne Vermieter-Kennwort bleibt die App offen und jeder ist Vermieter, wie bisher.
- Die Berechtigungsmatrix (welche Rechte einstellbar oder fest sind, Defaults) steht in der Entscheidung zum Ticket "Berechtigungsmatrix festlegen", nicht hier.

## Konsequenzen

- Bruch beim Update: Wer `LOGIN_PASSWORD` oder `login_password` gesetzt hatte, muss `VERMIETER_PASSWORD` bzw. `vermieter_password` neu setzen. Bis dahin ist die App offen. Das steht im Changelog unter "Wartung" und in der Add-on-Dokumentation.
- Die Namen der Variablen und Optionen sind von außen sichtbar und nur mit einem weiteren Bruch änderbar, deshalb ein ADR.
- Der Code unterscheidet nicht mehr "angemeldet oder nicht", sondern die Rolle (Betrachter). Routen, die dem Vermieter vorbehalten sind, antworten dem Mieter mit einer Hinweisseite.
- Die Kennwörter bleiben einfache gemeinsame Geheimnisse ohne Benutzerkonten, Passwortwechsel und Mehrfaktor. Wer mehr braucht, setzt die App hinter einen Reverse Proxy.
