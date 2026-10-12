package calc_test

import (
	"math"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// wallboxKosten computes the Verbrauchskosten of a second Ablesung with a
// Netzbezug of 100 kWh, Wohnung 2 30 kWh, Wärmepumpe 50 kWh and Wallboxen
// 40 kWh: the Netzbezug-Anteil Wallboxen is 20 kWh (the rest is PV). prev
// and cur add the optional Einzelzaehler readings.
func wallboxKosten(t *testing.T, preis float64, komplettW1 bool, prev, cur map[string]float64) calc.Ablesung {
	t.Helper()
	p1 := vollePeriode(1, "2026-10-01", prev)
	cur2 := map[string]float64{"strom_gesamt": 100, "strom_wohnung2": 30, "strom_waermepumpe": 50, "strom_wallbox": 40}
	for k, v := range cur {
		cur2[k] = v
	}
	p2 := vollePeriode(2, "2026-11-01", cur2)
	p2.Strompreis = store.Float64(preis)
	p2.WallboxKomplettW1 = komplettW1
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{p1, p2},
		Apartments: []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}},
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
	a, ok := v.Ablesung(2)
	if !ok || !a.Berechenbar() {
		t.Fatalf("Ablesung 2 not calculable: %+v", a)
	}
	return a
}

func gleich(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestWallbox_Verteilung(t *testing.T) {
	w2 := map[string]float64{store.MeterKeyWallboxWohnung2: 0}

	t.Run("Wohnung 2 zahlt den eigenen Verbrauch, Wohnung 1 den Rest", func(t *testing.T) {
		a := wallboxKosten(t, 0.30, false, w2, map[string]float64{store.MeterKeyWallboxWohnung2: 12})
		w1, w2 := a.Kosten.Anteil(1), a.Kosten.Anteil(2)
		if !gleich(w2.WallboxKosten, 3.6) || !gleich(w2.WallboxKWh, 12) {
			t.Errorf("Wohnung 2: %v EUR / %v kWh, want 3,60 / 12", w2.WallboxKosten, w2.WallboxKWh)
		}
		if !gleich(w1.WallboxKosten, 2.4) || !gleich(w1.WallboxKWh, 8) {
			t.Errorf("Wohnung 1: %v EUR / %v kWh, want 2,40 / 8 (Rest inkl. Solarzuteilung)", w1.WallboxKosten, w1.WallboxKWh)
		}
	})

	t.Run("Wohnung 2 ist auf den Netzbezug-Anteil gedeckelt", func(t *testing.T) {
		a := wallboxKosten(t, 0.30, false, w2, map[string]float64{store.MeterKeyWallboxWohnung2: 35})
		w1, w2 := a.Kosten.Anteil(1), a.Kosten.Anteil(2)
		if !gleich(w2.WallboxKosten, 6) || !gleich(w2.WallboxKWh, 20) || w1.WallboxKosten != 0 || w1.WallboxKWh != 0 {
			t.Errorf("W2 %v EUR / %v kWh, W1 %v EUR / %v kWh, want 6 / 20 and 0 / 0", w2.WallboxKosten, w2.WallboxKWh, w1.WallboxKosten, w1.WallboxKWh)
		}
	})

	t.Run("die Summe ist cent-genau die Kosten des Zwischenzaehlers", func(t *testing.T) {
		a := wallboxKosten(t, 0.333, false, w2, map[string]float64{store.MeterKeyWallboxWohnung2: 7})
		w1, w2 := a.Kosten.Anteil(1), a.Kosten.Anteil(2)
		if w2.WallboxKosten != 2.33 || w1.WallboxKosten != 4.33 || a.Kosten.Strom.KostenWallbox != 6.66 {
			t.Errorf("W2 %v, W1 %v, Zwischenzaehler %v, want 2,33 + 4,33 = 6,66", w2.WallboxKosten, w1.WallboxKosten, a.Kosten.Strom.KostenWallbox)
		}
		if w1.WallboxKosten+w2.WallboxKosten != a.Kosten.Strom.KostenWallbox {
			t.Errorf("Summe %v != %v", w1.WallboxKosten+w2.WallboxKosten, a.Kosten.Strom.KostenWallbox)
		}
	})

	t.Run("Wallbox Wohnung 1 darf fehlen, auch wenn sie vorhanden ist zaehlt sie nicht", func(t *testing.T) {
		both := map[string]float64{store.MeterKeyWallboxWohnung1: 0, store.MeterKeyWallboxWohnung2: 0}
		a := wallboxKosten(t, 0.30, false, both, map[string]float64{store.MeterKeyWallboxWohnung1: 999, store.MeterKeyWallboxWohnung2: 12})
		if got := a.Kosten.Anteil(2).WallboxKosten; !gleich(got, 3.6) {
			t.Errorf("Wohnung 2 = %v, want 3,60 unabhaengig vom Stand Wallbox Wohnung 1", got)
		}
	})
}

func TestWallbox_SchalterSchlaegtZaehler(t *testing.T) {
	w2 := map[string]float64{store.MeterKeyWallboxWohnung2: 0}
	a := wallboxKosten(t, 0.30, true, w2, map[string]float64{store.MeterKeyWallboxWohnung2: 12})
	w1, wo2 := a.Kosten.Anteil(1), a.Kosten.Anteil(2)
	if !gleich(w1.WallboxKosten, 6) || !gleich(w1.WallboxKWh, 20) || wo2.WallboxKosten != 0 || wo2.WallboxKWh != 0 {
		t.Errorf("W1 %v EUR / %v kWh, W2 %v EUR / %v kWh, want 6 / 20 and 0 / 0", w1.WallboxKosten, w1.WallboxKWh, wo2.WallboxKosten, wo2.WallboxKWh)
	}

	a = wallboxKosten(t, 0.30, true, nil, nil)
	if got := a.Kosten.Anteil(1).WallboxKosten; !gleich(got, 6) {
		t.Errorf("Schalter an ohne Einzelzaehler: Wohnung 1 = %v, want 6", got)
	}
}

func TestWallbox_OhneEinzelzaehlerBleibtInformativ(t *testing.T) {
	cases := map[string]struct{ prev, cur map[string]float64 }{
		"kein Einzelzaehler":          {nil, nil},
		"Stand fehlt in der Ablesung": {map[string]float64{store.MeterKeyWallboxWohnung2: 5}, nil},
		"Vorperiode fehlt":            {nil, map[string]float64{store.MeterKeyWallboxWohnung2: 12}},
		"nur Wallbox Wohnung 1":       {map[string]float64{store.MeterKeyWallboxWohnung1: 0}, map[string]float64{store.MeterKeyWallboxWohnung1: 12}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := wallboxKosten(t, 0.30, false, c.prev, c.cur)
			for _, id := range []int64{1, 2} {
				if w := a.Kosten.Anteil(id); w.WallboxKosten != 0 || w.WallboxKWh != 0 {
					t.Errorf("Wohnung %d = %v EUR / %v kWh, want nothing assigned", id, w.WallboxKosten, w.WallboxKWh)
				}
			}
			if got := a.Kosten.Strom.KostenWallbox; !gleich(got, 6) {
				t.Errorf("KostenWallbox = %v, want 6 (unchanged informational total)", got)
			}
		})
	}
}

func TestWallbox_Plausibilitaet(t *testing.T) {
	both := map[string]float64{store.MeterKeyWallboxWohnung1: 0, store.MeterKeyWallboxWohnung2: 0}
	// Zwischenzähler verbraucht 40 kWh: Schwelle ist max(5 % = 2, 2 kWh) = 2.
	cases := []struct {
		name         string
		w1, w2       float64
		wantAuffaell bool
	}{
		{"passt genau", 25, 15, false},
		{"knapp innerhalb der Schwelle", 25, 16.9, false},
		{"genau auf der Schwelle", 25, 17, false},
		{"knapp ausserhalb", 25, 17.1, true},
		{"zu wenig", 20, 15, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := wallboxKosten(t, 0.30, false, both, map[string]float64{store.MeterKeyWallboxWohnung1: c.w1, store.MeterKeyWallboxWohnung2: c.w2})
			s := a.Kosten.Strom
			if !s.WallboxEinzelPruefbar || s.WallboxEinzelAuffaellig != c.wantAuffaell || !gleich(s.WallboxEinzelSumme, c.w1+c.w2) {
				t.Errorf("pruefbar %v, auffaellig %v, Summe %v, want pruefbar, auffaellig %v, Summe %v", s.WallboxEinzelPruefbar, s.WallboxEinzelAuffaellig, s.WallboxEinzelSumme, c.wantAuffaell, c.w1+c.w2)
			}
		})
	}

	t.Run("Prozentschwelle bei grossem Verbrauch", func(t *testing.T) {
		// 1000 kWh Zwischenzähler: 5 % = 50 kWh > 2 kWh.
		a := wallboxKosten(t, 0.30, false, both, map[string]float64{
			"strom_wallbox": 1000, store.MeterKeyWallboxWohnung1: 600, store.MeterKeyWallboxWohnung2: 451,
		})
		if !a.Kosten.Strom.WallboxEinzelPruefbar || !a.Kosten.Strom.WallboxEinzelAuffaellig {
			t.Errorf("sum 1051 vs 1000 (> 50): %+v, want auffaellig", a.Kosten.Strom)
		}
		a = wallboxKosten(t, 0.30, false, both, map[string]float64{
			"strom_wallbox": 1000, store.MeterKeyWallboxWohnung1: 600, store.MeterKeyWallboxWohnung2: 449,
		})
		if a.Kosten.Strom.WallboxEinzelAuffaellig {
			t.Errorf("sum 1049 vs 1000 (<= 50): %+v, want not auffaellig", a.Kosten.Strom)
		}
	})

	for name, c := range map[string]struct{ prev, cur map[string]float64 }{
		"Wallbox Wohnung 1 fehlt": {map[string]float64{store.MeterKeyWallboxWohnung2: 0}, map[string]float64{store.MeterKeyWallboxWohnung2: 15}},
		"Vorperiode von W2 fehlt": {map[string]float64{store.MeterKeyWallboxWohnung1: 0}, map[string]float64{store.MeterKeyWallboxWohnung1: 25, store.MeterKeyWallboxWohnung2: 15}},
		"kein Einzelzaehler":      {nil, nil},
	} {
		t.Run("nicht pruefbar: "+name, func(t *testing.T) {
			a := wallboxKosten(t, 0.30, false, c.prev, c.cur)
			if s := a.Kosten.Strom; s.WallboxEinzelPruefbar || s.WallboxEinzelAuffaellig {
				t.Errorf("got %+v, want no Plausibilitaetspruefung", s)
			}
		})
	}
}
