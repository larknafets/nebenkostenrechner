package web

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

// dashboard_strom_wohnung1_test.go covers Wohnung 1's Strom position (#279):
// the rest of the grid draw after Wohnung 2, heat pump and Wallboxen, priced
// per Ablesung. Data seam for the numbers, HTTP seam (Dashboard)
// for the visibility.

const titelStrom = `title="Strom"`

// stromRestAblesungen stores two Ablesungen whose second has the given grid
// draw against 30 kWh Wohnung 2, 50 kWh heat pump and 40 kWh Wallboxen, at
// 0,22 EUR/kWh.
func stromRestAblesungen(t *testing.T, db *sql.DB, netzbezug float64) {
	t.Helper()
	if _, err := createPeriod(db, seedPeriodInputAt("2026-10-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	second := seedPeriodInputAt("2026-11-01")
	second.Readings["strom_gesamt"] = netzbezug
	second.Readings["strom_wohnung2"] = 30
	second.Readings["strom_waermepumpe"] = 50
	second.Readings["strom_wallbox"] = 40
	if _, err := createPeriod(db, second); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
}

func TestStromWohnung1_RestInKarteVerlaufUndGesamt(t *testing.T) {
	db := openTestDB(t)
	// 200 - 30 - 50 - 40 = 80 kWh rest, 17,60 EUR.
	stromRestAblesungen(t, db, 200)
	dd := mustLoadDashboard(t, db)

	card, verlauf := buildEntityView(dd, 1)
	if card.StromEUR != 17.60 {
		t.Errorf("StromEUR = %v, want 17.60", card.StromEUR)
	}
	seg := segmentByLabel(card.Segmente, "Strom")
	if seg == nil || seg.Kosten != 17.60 {
		t.Errorf("Segment Strom = %+v, want 17.60 EUR", seg)
	}
	if want := calc.Round2(card.StromEUR + card.WallboxEUR + card.HeizungEUR + card.WasserEUR); card.VerbrauchEUR != want {
		t.Errorf("VerbrauchEUR = %v, want %v (including Strom)", card.VerbrauchEUR, want)
	}

	m := verlauf.Eintraege[0].Monat
	vs := segmentByLabel(m.VerbrauchSegmente, "Strom")
	if vs == nil || vs.Kosten != 17.60 || vs.Verbrauch != 80 || vs.Einheit != "kWh" {
		t.Errorf("Verbrauchswert Strom = %+v, want 17.60 EUR / 80 kWh", vs)
	}

	// The Wallbox position is untouched: no Einzelzähler, no switch, so 0.
	if card.WallboxEUR != 0 {
		t.Errorf("WallboxEUR = %v, want 0", card.WallboxEUR)
	}
	// Wohnung 2 is unchanged: 30 kWh * 0,22.
	card2, _ := buildEntityView(dd, 2)
	if card2.StromEUR != 6.60 {
		t.Errorf("Wohnung 2 StromEUR = %v, want 6.60", card2.StromEUR)
	}
}

func TestStromWohnung1_SaldoEnthaeltDenStrom(t *testing.T) {
	db := openTestDB(t)
	stromRestAblesungen(t, db, 200)
	dd := mustLoadDashboard(t, db)
	_, verlauf := buildEntityView(dd, 1)
	m := verlauf.Eintraege[0].Monat
	if m.VerbrauchGesamt < 17.60 {
		t.Errorf("VerbrauchGesamt = %v, want at least the Strom 17.60", m.VerbrauchGesamt)
	}
}

func TestStromWohnung1_ImmerSichtbarAuchMitNull(t *testing.T) {
	cases := map[string]float64{
		"Rest gleich 0":                        120, // 30 + 50 + 40
		"Netzbezug kleiner als Zwischenzähler": 60,
		"sinkender Zählerstand":                -50,
	}
	for name, netz := range cases {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			stromRestAblesungen(t, db, netz)

			card, verlauf := buildEntityView(mustLoadDashboard(t, db), 1)
			if seg := segmentByLabel(card.Segmente, "Strom"); seg == nil || seg.Kosten != 0 {
				t.Errorf("Segment Strom = %+v, want 0 EUR", seg)
			}
			if card.StromEUR < 0 || card.GesamtEUR < 0 {
				t.Errorf("StromEUR = %v, GesamtEUR = %v, want nothing negative", card.StromEUR, card.GesamtEUR)
			}
			if segmentByLabel(verlauf.Eintraege[0].Monat.VerbrauchSegmente, "Strom") == nil {
				t.Error("Monatsverlauf zeigt Strom nicht")
			}

			if n := strings.Count(dashboardHTML(t, db), titelStrom); n < 2 {
				t.Errorf("Dashboard: Strom %d mal in den Jahreskarten, want mindestens 2 (Wohnung 1 und 2)", n)
			}
		})
	}
}
