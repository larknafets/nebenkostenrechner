package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// stromW1 computes the Kosten of one Ablesung whose consumption is the given
// meter differences against an all-zero Vorperiode (Strompreis 0.22).
// wallboxKomplettW1 is the period's switch "Wallboxen vollständig Wohnung 1".
func stromW1(t *testing.T, verbrauch map[string]float64, wallboxKomplettW1 bool) calc.Kosten {
	t.Helper()
	p := vollePeriode(2, "2026-11-01", verbrauch)
	p.WallboxKomplettW1 = wallboxKomplettW1
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{vollePeriode(1, "2026-10-01", nil), p},
		Apartments: []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}},
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
	a, ok := v.Ablesung(2)
	if !ok || !a.Berechenbar() {
		t.Fatalf("Ablesung 2 not calculable: %+v", a)
	}
	return a.Kosten
}

func TestAnteil_Wohnung1TraegtRestDesNetzbezugs(t *testing.T) {
	a := stromW1(t, map[string]float64{
		"strom_gesamt": 1000, "strom_wohnung2": 300, "strom_waermepumpe": 200, "strom_wallbox": 100,
	}, false).Anteil(1)
	if !a.HatStrom {
		t.Fatal("Wohnung 1 HatStrom = false, want true")
	}
	if a.StromKWh != 400 {
		t.Errorf("StromKWh = %v, want 400 (1000 - 300 - 200 - 100)", a.StromKWh)
	}
	if a.StromKosten != 88 {
		t.Errorf("StromKosten = %v, want 88 (400 kWh * 0.22)", a.StromKosten)
	}
}

func TestAnteil_Wohnung1RundetJeAblesungAufDenCent(t *testing.T) {
	a := stromW1(t, map[string]float64{"strom_gesamt": 10.05}, false).Anteil(1)
	if a.StromKosten != 2.21 {
		t.Errorf("StromKosten = %v, want 2.21 (10.05 * 0.22 = 2.211)", a.StromKosten)
	}
}

func TestAnteil_Wohnung1WallboxAnteilWirdNichtDoppeltGezaehlt(t *testing.T) {
	a := stromW1(t, map[string]float64{"strom_gesamt": 500, "strom_wallbox": 100}, true).Anteil(1)
	if a.WallboxKWh != 100 {
		t.Errorf("WallboxKWh = %v, want 100", a.WallboxKWh)
	}
	if a.StromKWh != 400 {
		t.Errorf("StromKWh = %v, want 400 (the 100 kWh Wallbox stay in the Wallbox position)", a.StromKWh)
	}
}

func TestAnteil_Wohnung1RestWirdNieNegativ(t *testing.T) {
	cases := map[string]map[string]float64{
		"Netzbezug kleiner als die Zwischenzähler": {
			"strom_gesamt": 100, "strom_wohnung2": 300, "strom_waermepumpe": 200, "strom_wallbox": 100,
		},
		"Netzbezug gleich Summe der Zwischenzähler": {
			"strom_gesamt": 600, "strom_wohnung2": 300, "strom_waermepumpe": 200, "strom_wallbox": 100,
		},
		"sinkender Zählerstand Netzbezug": {
			"strom_gesamt": -50, "strom_wohnung2": 300, "strom_waermepumpe": 200, "strom_wallbox": 100,
		},
		"sinkender Zählerstand Netzbezug ohne Zwischenzähler": {
			"strom_gesamt": -50,
		},
		"sinkender Zählerstand Zwischenzähler": {
			"strom_gesamt": 100, "strom_wohnung2": -300, "strom_waermepumpe": -200, "strom_wallbox": -100,
		},
	}
	for name, verbrauch := range cases {
		t.Run(name, func(t *testing.T) {
			a := stromW1(t, verbrauch, false).Anteil(1)
			if a.StromKWh < 0 || a.StromKosten < 0 {
				t.Errorf("StromKWh = %v, StromKosten = %v, want both >= 0", a.StromKWh, a.StromKosten)
			}
		})
	}
}

func TestAnteil_Wohnung2StromBleibtUnveraendert(t *testing.T) {
	a := stromW1(t, map[string]float64{"strom_gesamt": 1000, "strom_wohnung2": 300}, false).Anteil(2)
	if !a.HatStrom || a.StromKWh != 300 || a.StromKosten != 66 {
		t.Errorf("Wohnung 2 Strom = %+v, want 300 kWh / 66 EUR", a)
	}
}
