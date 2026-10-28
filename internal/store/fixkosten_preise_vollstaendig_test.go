package store

import "testing"

// PreiseVollstaendig is the one place that says whether a Fixkosten-Eingabe
// has all four prices: calc (Ablesung nicht berechenbar) and the web hints ask
// it instead of each repeating the nil checks.
func TestFixkostenEingabeDetails_PreiseVollstaendig(t *testing.T) {
	voll := func() *FixkostenEingabeDetails {
		return &FixkostenEingabeDetails{
			Strompreis: Float64(0.3), FrischwasserPreis: Float64(1.5),
			AbwasserPreis: Float64(4.8), EinspeisungPreis: Float64(0.08),
		}
	}
	if !voll().PreiseVollstaendig() {
		t.Error("alle vier Preise gesetzt: want vollständig")
	}
	if p := voll(); true {
		p.EinspeisungPreis = nil
		if p.PreiseVollstaendig() {
			t.Error("Einspeisung fehlt: want nicht vollständig")
		}
	}
	if p := voll(); true {
		p.Strompreis = nil
		if p.PreiseVollstaendig() {
			t.Error("Strompreis fehlt: want nicht vollständig")
		}
	}
	// 0 is an entered price.
	z := voll()
	z.AbwasserPreis = Float64(0)
	if !z.PreiseVollstaendig() {
		t.Error("Preis 0 ist erfasst: want vollständig")
	}
}
