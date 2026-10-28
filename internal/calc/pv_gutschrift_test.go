package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// pvKosten computes the Verbrauchskosten of a second Ablesung that feeds in
// 52.5 kWh at 0.08 EUR/kWh (Ertrag 4,20 EUR), with the PV-Gutschrift switch
// set as given (#226).
func pvKosten(t *testing.T, gutschriftW1 bool) calc.Kosten {
	t.Helper()
	p1 := vollePeriode(1, "2026-10-01", nil)
	p2 := vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100, "strom_einspeisung": 52.5})
	e := volleEingabe("2026-11-01")
	e.PVGutschriftW1 = gutschriftW1
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{p1, p2},
		Eingaben:   []*store.FixkostenEingabeDetails{e},
		Apartments: []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}},
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
	a, ok := v.Ablesung(2)
	if !ok || !a.Berechenbar() {
		t.Fatalf("Ablesung 2 not calculable: %+v", a)
	}
	return a.Kosten
}

func TestPVGutschrift_SchalterAn(t *testing.T) {
	k := pvKosten(t, true)
	if got := k.Anteil(1).PVGutschrift; !gleich(got, 4.2) {
		t.Errorf("Wohnung 1 PVGutschrift = %v, want 4,20 (Ertrag der Ablesung)", got)
	}
	if got := k.Anteil(2).PVGutschrift; got != 0 {
		t.Errorf("Wohnung 2 PVGutschrift = %v, want 0 (nie eine Gutschrift)", got)
	}
	if got := k.Einspeisung.Ertrag; !gleich(got, 4.2) {
		t.Errorf("haus-weiter Ertrag = %v, want 4,20 unveraendert", got)
	}
}

func TestPVGutschrift_SchalterAusBleibtInformativ(t *testing.T) {
	k := pvKosten(t, false)
	for _, id := range []int64{1, 2} {
		if got := k.Anteil(id).PVGutschrift; got != 0 {
			t.Errorf("Wohnung %d PVGutschrift = %v, want 0", id, got)
		}
	}
	if got := k.Einspeisung.Ertrag; !gleich(got, 4.2) {
		t.Errorf("haus-weiter Ertrag = %v, want 4,20", got)
	}
}

func TestPVGutschrift_AblesungenImMonatTeilenDenSchalter(t *testing.T) {
	// Two Ablesungen in the same Abrechnungsmonat: the switch belongs to the
	// Fixkosten-Eingabe of the month, so both credit their Ertrag (#359).
	p1 := vollePeriode(1, "2026-10-01", nil)
	p2 := vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100, "strom_einspeisung": 50})
	p3 := vollePeriode(3, "2026-11-01", map[string]float64{"strom_gesamt": 200, "strom_einspeisung": 100})
	e := volleEingabe("2026-11-01")
	e.PVGutschriftW1 = true
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{p1, p2, p3},
		Eingaben:   []*store.FixkostenEingabeDetails{e},
		Apartments: []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}},
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
	a2, _ := v.Ablesung(2)
	a3, _ := v.Ablesung(3)
	if got := a2.Kosten.Anteil(1).PVGutschrift; !gleich(got, 4) {
		t.Errorf("Ablesung 2 = %v, want 4 (50 kWh * 0,08)", got)
	}
	if got := a3.Kosten.Anteil(1).PVGutschrift; !gleich(got, 4) {
		t.Errorf("Ablesung 3 = %v, want 4 (50 kWh * 0,08)", got)
	}
}

func TestSaldoverlauf_Gutschrift(t *testing.T) {
	t.Run("erhoeht den Saldo ohne Deckel", func(t *testing.T) {
		// Abschlag 100, Kosten 30: +70; Gutschrift 500 pushes the balance far
		// beyond what was paid in.
		saldi, end := calc.Saldoverlauf(0, []calc.SaldoMonat{
			{Abschlag: 100, Kosten: 30, Gutschrift: 500},
			{Abschlag: 100, Kosten: 130},
		})
		if saldi[0] != 570 || saldi[1] != 540 || end != 540 {
			t.Fatalf("saldi=%v end=%v, want 570, 540", saldi, end)
		}
	})
	t.Run("ohne Gutschrift unveraendert", func(t *testing.T) {
		saldi, _ := calc.Saldoverlauf(0, []calc.SaldoMonat{{Abschlag: 50, Kosten: 33.333}})
		if saldi[0] != 16.67 {
			t.Fatalf("saldi=%v", saldi)
		}
	})
}
