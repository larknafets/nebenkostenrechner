package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// ablesung_fixkosten_ansicht_test.go: Ablesungsliste, -Detail und Abrechnung
// folgen der Fixkosten-Eingabe als einziger Quelle (#365, Spec #359).

// ablesungenMitOffenemMonat legt drei Ablesungen an: Oktober mit Eingabe,
// November ohne Eingabe (Fixkosten fehlen), Dezember mit Eingabe (Häkchen
// "Wallboxen vollständig Wohnung 1" gesetzt, Strompreis 0,31). Zurück kommen
// die ids in dieser Reihenfolge.
func ablesungenMitOffenemMonat(t *testing.T, db *sql.DB) (okt, nov, dez int64) {
	t.Helper()
	ids := make([]int64, 0, 3)
	for i, datum := range []string{"2026-10-01", "2026-11-01", "2026-12-01"} {
		in := seedPeriodInputAt(datum)
		in.Readings["strom_gesamt"] = float64(100 * i)
		id, err := store.CreatePeriod(db, in)
		if err != nil {
			t.Fatalf("CreatePeriod %s: %v", datum, err)
		}
		ids = append(ids, id)
	}
	for _, monat := range []string{"2026-10-01", "2026-12-01"} {
		in := seedFixkostenInput(monat)
		in.Strompreis, in.FrischwasserPreis = store.Float64(0.31), store.Float64(1.9)
		in.AbwasserPreis, in.EinspeisungPreis = store.Float64(2.4), store.Float64(0.0786)
		in.WallboxKomplettW1 = monat == "2026-12-01"
		if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
			t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
		}
	}
	return ids[0], ids[1], ids[2]
}

// absatz returns the paragraph of the Ablesungsseite that starts with label.
func absatz(body, label string) string {
	i := strings.Index(body, "<strong>"+label+":</strong>")
	if i < 0 {
		return ""
	}
	if j := strings.Index(body[i:], "</p>"); j >= 0 {
		return body[i : i+j]
	}
	return body[i:]
}

func listeBody(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie) string {
	t.Helper()
	w := getMitCookies(t, mux, "/ablesungen", cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /ablesungen: status %d", w.Code)
	}
	return w.Body.String()
}

func TestAblesungsliste_BadgeFixkostenFehlen(t *testing.T) {
	db := openTestDB(t)
	ablesungenMitOffenemMonat(t, db)
	body := listeBody(t, NewMux(db, openTestDB(t), "", ""), nil)

	// Only the November row (no Eingabe) carries the badge.
	if n := strings.Count(body, "unvollständig · Fixkosten fehlen"); n != 1 {
		t.Errorf("Badge \"unvollständig · Fixkosten fehlen\" %d-mal, want 1", n)
	}
	if n := strings.Count(body, `class="badge badge-warning`); n != 1 {
		t.Errorf("%d Badges, want 1", n)
	}
}

func TestAblesungsDetail_FixkostenFehlen(t *testing.T) {
	db := openTestDB(t)
	_, nov, _ := ablesungenMitOffenemMonat(t, db)
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	t.Setenv("MIETER_PASSWORD", "mieter")
	mux := NewMux(db, openTestDB(t), "", "")
	vermieter := loginCookies(t, mux, "vermieter", "geheim")
	mieter := loginCookies(t, mux, "mieter", "mieter")
	pfad := fmt.Sprintf("/ablesungen/%d", nov)

	voll := getMitCookies(t, mux, pfad, vermieter)
	if voll.Code != http.StatusOK {
		t.Fatalf("Vermieter: status %d", voll.Code)
	}
	body := voll.Body.String()

	if !strings.Contains(body, "unvollständig · Fixkosten fehlen") {
		t.Error("Badge fehlt im Detail")
	}
	// The hint replaces the Euro blocks, the Zählerstände stay.
	for _, weg := range []string{"Stromkosten (PV-Netzbezug-Zuteilung)", "Wasserkosten", "Heizungskosten", "Einspeisevergütung (PV)", "Kosten Wohnung 1"} {
		if strings.Contains(body, weg) {
			t.Errorf("Detail zeigt trotz fehlender Fixkosten %q", weg)
		}
	}
	if !strings.Contains(body, "Zähler") || !strings.Contains(body, "(&#43;100,00&nbsp;kWh)") {
		t.Errorf("Zählerstände und Differenzen fehlen: %s", tabellenzeile(t, body, "Strom Netzbezug (1.8.0)"))
	}
	if !strings.Contains(body, `href="/fixkosten/neu"`) || !strings.Contains(body, "Fixkosten erfassen") {
		t.Error("Vermieter sieht den Link \"Fixkosten erfassen\" nicht")
	}
	// Preise and the switch read the Eingabe, here there is none.
	for _, label := range []string{"Preise", "Wallboxen vollständig Wohnung 1"} {
		if !strings.Contains(absatz(body, label), "Fixkosten fehlen") {
			t.Errorf("Zeile %q zeigt nicht \"Fixkosten fehlen\"", label)
		}
	}
	// Wallbox: Netzbezug-Anteil in kWh, the hint instead of the split.
	if !strings.Contains(body, "Wallbox Anteil") {
		t.Error("Wallbox-Block ohne Netzbezug-Anteil")
	}
	if strings.Contains(body, "keiner Wohnung zugeordnet") {
		t.Error("Wallbox-Hinweis \"keiner Wohnung zugeordnet\" ohne Eingabe")
	}
	// The Personenzahl column is gone.
	if strings.Contains(body, "<th>Personen</th>") {
		t.Error("Wohnungstabelle hat noch die Spalte Personen")
	}

	for name, cookies := range map[string][]*http.Cookie{"Mieter": mieter, "nicht angemeldet": nil} {
		w := getMitCookies(t, mux, pfad, cookies)
		if w.Code != http.StatusOK {
			continue // no right to see Ablesungen, nothing to check
		}
		b := w.Body.String()
		if !strings.Contains(b, "Fixkosten fehlen") {
			t.Errorf("%s: Hinweis fehlt", name)
		}
		if strings.Contains(b, `href="/fixkosten/neu"`) {
			t.Errorf("%s sieht den Link \"Fixkosten erfassen\"", name)
		}
	}
}

func TestAblesungsDetail_ZeilenAusDerEingabe(t *testing.T) {
	db := openTestDB(t)
	okt, _, dez := ablesungenMitOffenemMonat(t, db)

	body := detailBody(t, db, dez)
	if strings.Contains(body, "Fixkosten fehlen") {
		t.Error("Detail mit Eingabe zeigt \"Fixkosten fehlen\"")
	}
	if row := absatz(body, "Wallboxen vollständig Wohnung 1"); !strings.Contains(row, "Ja") {
		t.Errorf("Wallbox-Zeile %q ohne Ja (Häkchen der Eingabe)", row)
	}
	for _, want := range []string{"0,31", "1,90", "2,40", "0,0786"} {
		if !strings.Contains(body, want) {
			t.Errorf("Preise-Zeile ohne %q", want)
		}
	}
	if !strings.Contains(body, `href="/fixkosten/`) {
		t.Error("kein Link zur Fixkosten-Eingabe")
	}
	if row := absatz(detailBody(t, db, okt), "Wallboxen vollständig Wohnung 1"); !strings.Contains(row, "Nein") {
		t.Errorf("Wallbox-Zeile Oktober %q ohne Nein", row)
	}
	// The Personenzahl stays in the Wasserblock only.
	if strings.Contains(body, "<th>Personen</th>") || !strings.Contains(body, "<td>Personen</td>") {
		t.Error("Personenzahl nicht nur im Wasserblock")
	}
}

func TestAbrechnung_PersonenMonateEineSpalte(t *testing.T) {
	db := teiljahrDB(t, true)
	_, body := getAbrechnung(t, NewMux(db, openTestDB(t), "", ""), "?jahr=2026&wohnung=2", nil)
	if strings.Contains(body, "Ablesung (Warmwasser)") {
		t.Error("Anlage Personenzahl hat noch die Spalte der Ablesung")
	}
	if !strings.Contains(body, "Fixkosten-Eingabe (Schlüssel Personen)") {
		t.Error("Anlage Personenzahl ohne Spalte der Fixkosten-Eingabe")
	}
}
