package calc

import (
	"math"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// The pure cores need no database: edge cases are checked here directly, the
// tests in the other files cover the same results through the public
// functions with their loaders.

func nah(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestBerechneStrom_Zuteilung(t *testing.T) {
	t.Run("sequenziell: Wohnung 2, Waermepumpe, Wallbox", func(t *testing.T) {
		got := berechneStrom(stromEingabe{Strompreis: 0.30, Verbrauch: map[string]float64{
			"strom_gesamt": 100, "strom_wohnung2": 30, "strom_waermepumpe": 50, "strom_wallbox": 40,
		}})
		if got.W2AnteilKWh != 30 || got.WPAnteilKWh != 50 || got.WallboxAnteilKWh != 20 {
			t.Errorf("Anteile = %v / %v / %v, want 30 / 50 / 20", got.W2AnteilKWh, got.WPAnteilKWh, got.WallboxAnteilKWh)
		}
		if got.PVAnteilWallboxKWh != 20 || got.KostenW2 != 9 || got.KostenWallbox != 6 || !nah(got.KostenWPGesamtUnrounded, 15) {
			t.Errorf("PV Wallbox %v, Kosten W2 %v / Wallbox %v / WP %v, want 20, 9, 6, 15", got.PVAnteilWallboxKWh, got.KostenW2, got.KostenWallbox, got.KostenWPGesamtUnrounded)
		}
	})
	t.Run("Netzbezug kleiner als der Zwischenzaehler: gedeckelt, Rest ist PV", func(t *testing.T) {
		got := berechneStrom(stromEingabe{Strompreis: 1, Verbrauch: map[string]float64{
			"strom_gesamt": 20, "strom_wohnung2": 30, "strom_waermepumpe": 10,
		}})
		if got.W2AnteilKWh != 20 || got.PVAnteilW2KWh != 10 || got.WPAnteilKWh != 0 || got.PVAnteilWPKWh != 10 || got.W2VerbrauchKWh != 30 {
			t.Errorf("got %+v, want W2 capped at 20 (10 PV), nothing left for the heat pump", got)
		}
	})
}

func TestBerechneHeizung_Gewichtung(t *testing.T) {
	strom := &StromErgebnis{KostenWPGesamtUnrounded: 100, WPAnteilKWh: 200, PVAnteilWPKWh: 50}
	in := func(gewichtung float64) heizungEingabe {
		return heizungEingabe{
			Strom: strom, WaermeGewichtung: gewichtung,
			Verbrauch: map[string]float64{"waerme_wohnung1": 3, "waerme_wohnung2": 1},
			QMW1:      100, QMW2: 100,
		}
	}

	got := berechneHeizung(in(0.7)) // Waerme 75/25, Flaeche 50/50 -> Wohnung 1: .7*.75 + .3*.5 = .675
	if got.KostenHeizungW1 != 67.5 || got.KostenHeizungW2 != 32.5 || !nah(got.WPVerbrauchW1KWh, 250*0.675) {
		t.Errorf("0.7: Kosten %v / %v, WPVerbrauch W1 %v, want 67.5 / 32.5 / 168.75", got.KostenHeizungW1, got.KostenHeizungW2, got.WPVerbrauchW1KWh)
	}
	got = berechneHeizung(in(0.5)) // .5*.75 + .5*.5 = .625
	if got.KostenHeizungW1 != 62.5 || got.KostenHeizungW2 != 37.5 {
		t.Errorf("0.5: Kosten %v / %v, want 62.5 / 37.5", got.KostenHeizungW1, got.KostenHeizungW2)
	}
}

func TestBerechneHeizung_OhneWaermeverbrauch(t *testing.T) {
	got := berechneHeizung(heizungEingabe{
		Strom: &StromErgebnis{KostenWPGesamtUnrounded: 80}, WaermeGewichtung: 0.7,
		Verbrauch: map[string]float64{}, QMW1: 100, QMW2: 100,
	})
	if got.KostenHeizungW1 != 40 || got.KostenHeizungW2 != 40 {
		t.Errorf("Kosten %v / %v, want the 50/50 fallback", got.KostenHeizungW1, got.KostenHeizungW2)
	}
}

func TestBerechneWasser(t *testing.T) {
	verbrauch := map[string]float64{"wasser_gesamt": 100, "wasser_wohnung2": 30, "wasser_warmwasseraufbereitung": 20}

	got := berechneWasser(wasserEingabe{FrischwasserPreis: 2, AbwasserPreis: 4, Verbrauch: verbrauch, PersonenW1: 3, PersonenW2: 1})
	if got.WWAnteilW1 != 15 || got.WWAnteilW2 != 5 || got.FrischwasserW1 != 65 || got.FrischwasserW2 != 35 {
		t.Errorf("Warmwasser %v / %v, Frischwasser %v / %v, want 15 / 5 / 65 / 35", got.WWAnteilW1, got.WWAnteilW2, got.FrischwasserW1, got.FrischwasserW2)
	}
	if got.KostenFrischwasserW1 != 130 || got.KostenFrischwasserW2 != 70 || got.KostenAbwasserW1 != 260 || got.KostenAbwasserW2 != 140 {
		t.Errorf("Kosten = %+v, want Frischwasser 130 / 70, Abwasser 260 / 140", got)
	}

	got = berechneWasser(wasserEingabe{FrischwasserPreis: 1, AbwasserPreis: 1, Verbrauch: verbrauch})
	if got.WWAnteilW1 != 10 || got.WWAnteilW2 != 10 || got.FrischwasserW1 != 60 || got.FrischwasserW2 != 40 {
		t.Errorf("ohne Personen: Warmwasser %v / %v, Frischwasser %v / %v, want the 50/50 split 10 / 10 / 60 / 40", got.WWAnteilW1, got.WWAnteilW2, got.FrischwasserW1, got.FrischwasserW2)
	}
}

func TestBerechneEinspeisung(t *testing.T) {
	got := berechneEinspeisung(einspeisungEingabe{EinspeisungPreis: 0.0812, Verbrauch: map[string]float64{"strom_einspeisung": 100}})
	if got.EinspeisungKWh != 100 || got.Ertrag != 8.12 {
		t.Errorf("got %+v, want 100 kWh and 8.12 EUR", got)
	}
}

func TestBerechneFixkosten(t *testing.T) {
	got := berechneFixkosten(fixkostenEingabe{
		Eingabe: &store.FixkostenEingabeDetails{
			Personen: map[int64]int64{1: 3, 2: 1},
			Werte: map[int64]store.FixkostenPositionWert{
				1: {Logik: store.LogikWohnung1, Typ: store.TypJaehrlich, Wert: 1200},
				2: {Logik: store.LogikWohneinheit, Typ: store.TypMonatlich, Wert: 60},
				4: {Logik: store.LogikPersonen, Typ: store.TypMonatlich, Wert: 40},
			},
		},
		Kostenpositionen: []store.Kostenposition{{ID: 1, Key: "a"}, {ID: 2, Key: "b"}, {ID: 3, Key: "ohne_wert"}, {ID: 4, Key: "c"}},
		QMW1:             100, QMW2: 100,
	})
	if len(got.Positionen) != 3 {
		t.Fatalf("Positionen = %d, want 3 (the one without a value is skipped)", len(got.Positionen))
	}
	if p := got.Positionen[0]; p.Monatswert != 100 || p.KostenW1 != 100 || p.KostenW2 != 0 {
		t.Errorf("jaehrlich %+v, want 100 per month, all Wohnung 1", p)
	}
	if p := got.Positionen[1]; p.KostenW1 != 30 || p.KostenW2 != 30 {
		t.Errorf("Wohneinheit %+v, want 30 / 30", p)
	}
	if p := got.Positionen[2]; p.KostenW1 != 30 || p.KostenW2 != 10 {
		t.Errorf("Personen %+v, want 30 / 10 (3:1)", p)
	}
	if got.KostenW1 != 160 || got.KostenW2 != 40 {
		t.Errorf("Summen %v / %v, want 160 / 40", got.KostenW1, got.KostenW2)
	}
}
