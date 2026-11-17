# 6. Pico CSS als Frontend-Bibliothek, kein Build und kein shadcn

## Status

Angenommen (2026-11-17)

## Kontext

Das UI soll moderner und schlanker werden. Die erste Idee war shadcn/ui. Das besteht aus React, Tailwind und Radix und braucht einen Node-Build. Die App ist ein einzelnes Go-Binary mit `go:embed` (Dockerfile ohne Node, distroless), das HA-Add-on baut dasselbe Binary. Die Templates (`internal/web/templates`) sind server-gerendert, die Styles stehen bisher inline in `layout.html`.

## Entscheidung

Pico CSS (klassenloses CSS, stylt Elemente statt Klassen) wird die Basis des Looks:

- Die Version wird gepinnt und die Datei lokal per `go:embed` ausgeliefert, kein CDN, kein Build-Schritt.
- Kopfnavigation, kompakt eingestellt: Basisschrift 87,5 % ab ca. 768 px, darunter 100 %, damit Eingabefelder auf dem Handy 16 px haben.
- Unsere Farbvariablen (`--bg`, `--panel`, `--accent` und so weiter) bleiben als Aliasse auf die `--pico-*`-Variablen, der Dark Mode läuft unverändert über `data-theme`.
- Eigen bleiben: Diagramm- und Statusfarben, das Abrechnungs-CSS samt Druck und PDF, die interaktiven Bausteine (Fehler-Overlay, Login-Dialog, Tooltip im Monatsverlauf).

Die Details stehen in den geschlossenen Tickets der Map "UI modernisieren mit Pico CSS und neuem Layout".

## Betrachtete Optionen

- **shadcn/ui mit React:** braucht Node im Build, bricht das Ein-Binary-Prinzip und würde Auth, Formulare und CSV mit betreffen.
- **Tailwind (Standalone-CLI):** neuer Build-Schritt in Dockerfile, CI und Add-on, liefert keine fertigen Komponenten, lange Klassenlisten in allen Templates.
- **Basecoat (shadcn-Nachbau ohne React):** ursprüngliche Richtung. Fertige CSS-Datei (237 KB), aber Dark Mode nur über `html.dark` statt `data-theme`, ein Hauptmaintainer mit unregelmäßigen Releases. Nach dem Prototyp gefiel Pico besser.
- **Oat UI:** schlank, aber noch jung, und die Variablennamen (`--muted`, `--accent`) kollidieren mit unseren.
- **Bulma:** klassenbasiert, Klassen in allen Templates nötig, Schritt-Reiter im Dunkelmodus mit Kontrastproblem.
- **Materialize:** nur gesichtet, klassen- und JS-lastig, Material-Look.

## Konsequenzen

- Neue Elemente erben den Look von Pico, es gibt keine eigene Komponentenbibliothek. Eigene Klassen nur dort, wo Pico nichts liefert (Balken, Zeitleiste, Badges).
- Mindestversion der Browser durch `:has()` in Pico 2.1.1: Chrome und Edge ab 105, Safari ab 15.4, Firefox ab 121.
- Ein Update von Pico ist eine bewusste Handlung: Version erhöhen, Datei ersetzen, Abrechnung als PDF vor und nach dem Update vergleichen.
- Die Abrechnung behält ihr eigenes Styling, Druck und PDF bleiben unverändert (neue Elemente werden gedruckt ausgeblendet, Pico rechnet im Druck mit 100 % Basisschrift).
- Revisit-Kriterium: Wird Pico nicht mehr gepflegt, oder braucht die App Komponenten, die Pico nicht hat (Tabs, Dialoge mit Fokusfalle), sind Basecoat und Oat neu zu prüfen.
