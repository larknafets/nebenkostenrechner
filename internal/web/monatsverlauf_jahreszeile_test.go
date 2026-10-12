package web

import (
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

// yearRowSpans returns the per-mode spans of every Jahreszeile in body.
func yearRowSpans(body string) map[string]string {
	out := map[string]string{}
	parts := strings.Split(body, `<div class="verlauf-year-row">`)
	for _, row := range parts[1:] {
		row, _, _ = strings.Cut(row, "</div>")
		for _, modus := range []string{"verbrauch", "verbrauchswerte", "fixkosten", "kombiniert"} {
			_, rest, ok := strings.Cut(row, `<span data-modus="`+modus+`">`)
			if !ok {
				continue
			}
			span, _, _ := strings.Cut(rest, "</span>")
			out[modus] += span
		}
	}
	return out
}

func TestMonatsverlauf_JahreszeileGutschriftNurInKombiniert(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)
	spans := yearRowSpans(dashboardHTML(t, db))
	if !strings.Contains(spans["kombiniert"], "PV-Gutschrift") {
		t.Errorf("Jahreszeile Kombiniert ohne PV-Gutschrift: %q", spans["kombiniert"])
	}
	for _, modus := range []string{"verbrauch", "verbrauchswerte", "fixkosten"} {
		if spans[modus] == "" {
			t.Errorf("Jahreszeile Modus %s nicht gefunden", modus)
		}
		if strings.Contains(spans[modus], "PV-Gutschrift") {
			t.Errorf("Jahreszeile Modus %s zeigt PV-Gutschrift: %q", modus, spans[modus])
		}
	}
}

func TestJahreszeile_GesamtBruttoOhneGutschrift(t *testing.T) {
	k := calc.Kosten{
		Strom: &calc.StromErgebnis{}, Wasser: &calc.WasserErgebnis{},
		Heizung: &calc.HeizungErgebnis{KostenHeizungW1: 30}, PVGutschriftW1: 4.20,
	}
	spalte := buildDashboardVerlauf(1, "Wohnung 1", []periodKosten{{ReadingDate: "2026-11-15", Monat: "2026-11-01", K: k}}, nil)
	for _, e := range spalte.Eintraege {
		if e.Jahreszeile != nil && e.Jahreszeile.GesamtBruttoSumme != 30 {
			t.Errorf("GesamtBruttoSumme = %v, want 30 (ohne Gutschrift)", e.Jahreszeile.GesamtBruttoSumme)
		}
	}
}
