package web

import (
	"database/sql"
	"net/http"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenMux is a Mux with a Vermieter and a Mieter login, the
// Sonderkosten seeded into the database it serves.
func sonderkostenMux(t *testing.T, eintraege ...store.Sonderkosten) (*http.ServeMux, *sql.DB) {
	t.Helper()
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	t.Setenv("MIETER_PASSWORD", "mieter")
	db := openTestDB(t)
	for _, s := range eintraege {
		if _, err := store.CreateSonderkosten(db, s); err != nil {
			t.Fatalf("CreateSonderkosten: %v", err)
		}
	}
	return NewMux(db, openTestDB(t), "", ""), db
}

// sonderkostenRoh inserts s with a raw INSERT, past the store validation, for
// tests that need an invalid old entry (Altlast).
func sonderkostenRoh(t *testing.T, db *sql.DB, s store.Sonderkosten) {
	t.Helper()
	if _, err := db.Exec(
		`INSERT INTO sonderkosten (monat, betreff, beschreibung, art, firma, rechnungsnummer, betrag, objekt, umlegbar, logik)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Monat, s.Betreff, s.Beschreibung, s.Art, s.Firma, s.Rechnungsnummer, s.Betrag, s.Objekt, s.Umlegbar, s.Logik,
	); err != nil {
		t.Fatalf("raw insert sonderkosten: %v", err)
	}
}

var beispielSonderkosten = []store.Sonderkosten{
	{Monat: "2026-10-01", Betreff: "Heizungspumpe", Beschreibung: "Pumpe Heizkreis 2", Art: store.ArtReparatur, Firma: "Meier GmbH", Rechnungsnummer: "RE-412", Rechnungsdatum: "2026-10-05", Betrag: 1000, Objekt: store.ObjektHaus, Umlegbar: true, Logik: store.LogikWohneinheit},
	{Monat: "2026-10-01", Betreff: "Rauchmelder", Art: store.ArtWartung, Firma: "Brandschutz Nord", Betrag: 96, Objekt: store.ObjektHaus},
	{Monat: "2026-09-01", Betreff: "Spülmaschine", Art: store.ArtNeuanschaffung, Firma: "Elektro Schmitt", Betrag: 649, Objekt: store.ObjektWohnung2, Umlegbar: true},
	{Monat: "2025-03-01", Betreff: "Wasserhahn", Art: store.ArtReparatur, Firma: "Sanitär Lehmann", Betrag: 184.3, Objekt: store.ObjektWohnung1},
}

func TestSonderkosten_SeiteZeigtKopfSummenUndKarten(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	w := getMitCookies(t, mux, "/sonderkosten", c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`<option value="2026" selected>`, // Jahr filter, the current year by default
		"1.745,00",                       // Gesamt 1000 + 96 + 649
		"1.649,00",                       // umlegbar
		"96,00",                          // trägt Vermieter:in
		"Oktober 2026", "September 2026",
		"Heizungspumpe", "Reparatur", "Meier GmbH", "RE-412", "Je Wohneinheit",
		"trägt Wohnung 2", "trägt Vermieter:in", "nicht umlegbar",
		"Pumpe Heizkreis 2", "<details",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthält %q nicht", want)
		}
	}
	if strings.Contains(body, "Wasserhahn") {
		t.Error("Eintrag aus 2025 steht im Jahr 2026")
	}
	if strings.Contains(body, "<h2>Sonderkosten") || strings.Contains(body, `class="jahr-nav"`) {
		t.Error("Überschrift und Jahrespfeile sind durch den Jahresfilter ersetzt")
	}
	if !strings.Contains(body, "<strong>Sonderkosten</strong>") || !strings.Contains(getMitCookies(t, mux, "/dashboard", c).Body.String(), `<a href="/sonderkosten">Sonderkosten</a>`) {
		t.Error("Menüpunkt Sonderkosten fehlt")
	}
}

func TestSonderkosten_JahrwahlUndArtfilter(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")

	alt := getMitCookies(t, mux, "/sonderkosten?jahr=2025", c).Body.String()
	if !strings.Contains(alt, `<option value="2025" selected>`) || !strings.Contains(alt, "Wasserhahn") || strings.Contains(alt, "Heizungspumpe") {
		t.Error("?jahr=2025 zeigt nicht das Jahr 2025")
	}

	gefiltert := getMitCookies(t, mux, "/sonderkosten?art=wartung", c).Body.String()
	if !strings.Contains(gefiltert, "Rauchmelder") || strings.Contains(gefiltert, "Heizungspumpe") {
		t.Error("Artfilter wartung zeigt nicht nur Wartung")
	}
	// The sums follow the filters (Jahr and Art): only the Wartung of 2026.
	if strings.Contains(gefiltert, "1.745,00") || !strings.Contains(gefiltert, "96,00") {
		t.Error("die Summen sollen dem Artfilter folgen")
	}
}

func TestSonderkosten_Objektfilter(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")

	w2 := getMitCookies(t, mux, "/sonderkosten?jahr=alle&objekt=wohnung2", c).Body.String()
	if !strings.Contains(w2, `<option value="wohnung2" selected>`) || !strings.Contains(w2, "Spülmaschine") || strings.Contains(w2, "Heizungspumpe") || strings.Contains(w2, "Rauchmelder") || strings.Contains(w2, "Wasserhahn") {
		t.Error("?objekt=wohnung2 zeigt nicht nur Einträge der Wohnung 2")
	}
	alle := getMitCookies(t, mux, "/sonderkosten?jahr=alle&objekt=nix", c).Body.String()
	if !strings.Contains(alle, "Rauchmelder") || !strings.Contains(alle, "Wasserhahn") || !strings.Contains(alle, "Heizungspumpe") {
		t.Error("unbekanntes Objekt soll wie Alle Objekte wirken")
	}
}

func TestSonderkosten_Rechte(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten...)
	mieter := loginCookies(t, mux, "mieter", "mieter")

	if w := getMitCookies(t, mux, "/sonderkosten", mieter); w.Code == http.StatusOK {
		t.Error("Mieter ohne Recht sieht die Seite")
	}
	if strings.Contains(getMitCookies(t, mux, "/dashboard", mieter).Body.String(), `/sonderkosten"`) {
		t.Error("Menüpunkt ohne Recht sichtbar")
	}

	b := store.DefaultBerechtigungen()
	b[store.RolleMieter][store.RechtSonderkostenAnsehen] = true
	if err := store.SaveStammdaten(db, store.StammdatenSave{HeizungWaermeGewichtung: 0.7, Berechtigungen: b}); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}

	w := getMitCookies(t, mux, "/sonderkosten", mieter)
	if w.Code != http.StatusOK {
		t.Fatalf("Mieter mit Recht: status %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"Meier GmbH", "RE-412"} {
		if !strings.Contains(body, want) {
			t.Errorf("Mieter mit Recht sieht %q nicht", want)
		}
	}
}

// The Jahr filter next to the Art filter lists every year with entries.
func TestSonderkosten_JahrFilterListetJahreMitEintraegen(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	body := getMitCookies(t, mux, "/sonderkosten", c).Body.String()
	i2026, i2025 := strings.Index(body, `<option value="2026" selected>`), strings.Index(body, `<option value="2025">`)
	if i2026 < 0 || i2025 < 0 || i2026 > i2025 {
		t.Errorf("Jahr filter: 2026 at %d (selected), 2025 at %d, want both, newest first", i2026, i2025)
	}
	// The Jahr filter comes first, then Art, then the sums; the button sits above all.
	b, j, a, g := strings.Index(body, "Neue Sonderkosten"), strings.Index(body, `id="jahr"`), strings.Index(body, `id="art"`), strings.Index(body, "Gesamt")
	if b < 0 || b >= j || j >= a || a >= g {
		t.Errorf("order button %d, Jahr %d, Art %d, sums %d: want ascending", b, j, a, g)
	}
}

// "Alle Jahre" sums and lists the entries of every year.
func TestSonderkosten_AlleJahre(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	body := getMitCookies(t, mux, "/sonderkosten?jahr=alle", c).Body.String()
	for _, want := range []string{`<option value="alle" selected>`, "Wasserhahn", "Heizungspumpe", "März 2025", "1.929,30"} { // 1745 + 184,30
		if !strings.Contains(body, want) {
			t.Errorf("Alle Jahre: Seite enthält %q nicht", want)
		}
	}
	if strings.Contains(body, `<option value="2026" selected>`) {
		t.Error("Alle Jahre: ein einzelnes Jahr ist ebenfalls gewählt")
	}
}
