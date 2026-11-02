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

var beispielSonderkosten = []store.Sonderkosten{
	{Monat: "2026-10-01", Betreff: "Heizungspumpe", Beschreibung: "Pumpe Heizkreis 2", Art: store.ArtReparatur, Firma: "Meier GmbH", Rechnungsnummer: "RE-412", Betrag: 1000, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikWohneinheit},
	{Monat: "2026-10-01", Betreff: "Rauchmelder", Art: store.ArtWartung, Firma: "Brandschutz Nord", Betrag: 96, Umsetzung: store.UmsetzungHaus},
	{Monat: "2026-09-01", Betreff: "Spülmaschine", Art: store.ArtNeuanschaffung, Firma: "Elektro Schmitt", Betrag: 649, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true},
	{Monat: "2025-03-01", Betreff: "Wasserhahn", Art: store.ArtReparatur, Firma: "Sanitär Lehmann", Betrag: 184.3, Umsetzung: store.UmsetzungWohnung1},
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
		"Sonderkosten 2026",
		"1.745,00", // Gesamt 1000 + 96 + 649
		"1.649,00", // umlegbar
		"96,00",    // trägt Vermieter:in
		"Oktober 2026", "September 2026",
		"Heizungspumpe", "Reparatur", "Meier GmbH", "RE-412", "Je Wohneinheit",
		"trägt Wohnung 2", "trägt Vermieter:in", "nicht umlegbar",
		"Pumpe Heizkreis 2", "<details",
		`href="?jahr=2025"`, // Pfeil zurück
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Seite enthält %q nicht", want)
		}
	}
	if strings.Contains(body, "Wasserhahn") {
		t.Error("Eintrag aus 2025 steht im Jahr 2026")
	}
	if !strings.Contains(body, "<strong>Sonderkosten</strong>") || !strings.Contains(getMitCookies(t, mux, "/dashboard", c).Body.String(), `<a href="/sonderkosten">Sonderkosten</a>`) {
		t.Error("Menüpunkt Sonderkosten fehlt")
	}
}

func TestSonderkosten_JahrwahlUndArtfilter(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")

	alt := getMitCookies(t, mux, "/sonderkosten?jahr=2025", c).Body.String()
	if !strings.Contains(alt, "Sonderkosten 2025") || !strings.Contains(alt, "Wasserhahn") || strings.Contains(alt, "Heizungspumpe") {
		t.Error("?jahr=2025 zeigt nicht das Jahr 2025")
	}

	gefiltert := getMitCookies(t, mux, "/sonderkosten?art=wartung", c).Body.String()
	if !strings.Contains(gefiltert, "Rauchmelder") || strings.Contains(gefiltert, "Heizungspumpe") {
		t.Error("Artfilter wartung zeigt nicht nur Wartung")
	}
	if !strings.Contains(gefiltert, "1.745,00") {
		t.Error("die Summen folgen dem Artfilter, sie sollen das ganze Jahr zeigen")
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
	// The button comes before the Jahr heading, like Ablesungen and Fixkosten.
	if b, h := strings.Index(body, "Neue Sonderkosten"), strings.Index(body, "<h2>Sonderkosten 2026"); b < 0 || b > h {
		t.Errorf("Neue Sonderkosten at %d, want before the heading at %d", b, h)
	}
}
