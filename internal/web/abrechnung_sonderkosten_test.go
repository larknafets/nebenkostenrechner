package web

import (
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestAbrechnungSeite_Sonderkosten: the line in the Kostenübersicht and the
// Anlage 8 behind the other Anlagen, only with umlegbar entries in the period
// (#407). Without them the page has no trace of Sonderkosten.
func TestAbrechnungSeite_Sonderkosten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	mux := NewMux(db, openTestDB(t), "", "")
	_, ohne := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
	mustNotContain(t, ohne, "Nach Eintrag", "Anlage 8")

	for _, s := range []store.Sonderkosten{
		{Monat: "2025-06-01", Betreff: "Pumpe", Beschreibung: "nur intern", Art: store.ArtReparatur, Firma: "Fa. Müller", Rechnungsnummer: "R-17", Betrag: 100, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikWohneinheit},
		{Monat: "2025-07-01", Betreff: "Privat", Betrag: 55, Umsetzung: store.UmsetzungWohnung2},
	} {
		if _, err := store.CreateSonderkosten(db, s); err != nil {
			t.Fatal(err)
		}
	}
	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
	mustContain(t, body,
		"Nach Eintrag", "<h3>Anlage 8: Sonderkosten</h3>", `<li value="8">Sonderkosten</li>`,
		"Pumpe", "Reparatur", "Fa. Müller", "R-17", "Haus", "Je Wohneinheit", `data-l="Rechnungsbetrag"`, `data-l="Anteil"`)
	mustNotContain(t, body, "nur intern", "Privat")
	if strings.Index(body, "Anlage 7: Zählerstände") > strings.Index(body, "Anlage 8: Sonderkosten") {
		t.Error("Anlage 8 steht nicht hinter Anlage 7")
	}
	_, w1 := getAbrechnung(t, mux, "?jahr=2025&wohnung=1", nil)
	mustContain(t, w1, "Anlage 8: Sonderkosten")
}
