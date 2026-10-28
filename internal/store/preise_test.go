package store

import "testing"

// Every Preis names its own Feldart, so a message points at the right price.
func TestPreise_JederPreisHatEigeneFeldart(t *testing.T) {
	gesehen := map[Feldart]string{}
	for _, p := range Preise {
		if p.Feld == FeldReadingDate { // the zero value
			t.Errorf("%s: Feld ist nicht gesetzt", p.Spalte)
		}
		if andere, doppelt := gesehen[p.Feld]; doppelt {
			t.Errorf("%s und %s teilen die Feldart %v", andere, p.Spalte, p.Feld)
		}
		gesehen[p.Feld] = p.Spalte
	}
}

// PreiseVollstaendig reads the table: a nil in any price makes it incomplete.
func TestPreise_VollstaendigWertetJedesNilAlsUnvollstaendig(t *testing.T) {
	for _, fehlt := range Preise {
		f := &FixkostenEingabeDetails{}
		for _, p := range Preise {
			if p.Spalte != fehlt.Spalte {
				*p.FixkostenDetail(f) = Float64(1)
			}
		}
		if f.PreiseVollstaendig() {
			t.Errorf("%s fehlt: want nicht vollständig", fehlt.Spalte)
		}
	}
}
