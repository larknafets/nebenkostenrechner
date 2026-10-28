package web

import (
	"strings"
	"testing"
)

// pv_gutschrift_test.go covers the switch "PV-Gutschrift Wohnung 1" (#226)
// through the HTTP seam: the Ablesungsseite. Form and CSV no longer carry it
// (#359), see wallbox_zuordnung_test.go and csv_test.go.

func TestAblesungDetail_PVSchalterAlsJaNein(t *testing.T) {
	for _, c := range []struct {
		an   bool
		want string
	}{{true, "Ja"}, {false, "Nein"}} {
		db := openTestDB(t)
		first := seedPeriodInputAt("2026-10-01")
		if _, err := createPeriod(db, first); err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		second := seedPeriodInputAt("2026-11-01")
		second.Readings["strom_einspeisung"] = 52.5 // * 0,08 EUR/kWh = 4,20 EUR
		second.PVGutschriftW1 = c.an
		id, err := createPeriod(db, second)
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		body := detailBody(t, db, id)
		if !strings.Contains(body, "PV-Gutschrift Wohnung 1:</strong> "+c.want+"<") {
			t.Errorf("Schalter %v: Ablesungsseite ohne %q", c.an, c.want)
		}
		// The house-wide table "Einspeisevergütung" stays either way.
		if !strings.Contains(body, "Einspeisevergütung (PV)") || !strings.Contains(body, "4,20") {
			t.Errorf("Schalter %v: haus-weite Einspeisevergütung fehlt", c.an)
		}
		if got := strings.Contains(body, "Gutschrift Wohnung 1</td>"); got != c.an {
			t.Errorf("Schalter %v: Zeile Gutschrift Wohnung 1 sichtbar = %v", c.an, got)
		}
	}
}
