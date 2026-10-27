package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

// The PV-Anteil sits in the Wohnungsanteil of apartment 2 only (apartment 1
// has no own electricity meter), next to the PVGutschrift.
func TestAnteil_PVAnteilKWh(t *testing.T) {
	k := calc.Kosten{
		Strom:   &calc.StromErgebnis{PVAnteilW2KWh: 12.5},
		Wasser:  &calc.WasserErgebnis{},
		Heizung: &calc.HeizungErgebnis{},
	}
	if got := k.Anteil(2).PVAnteilKWh; got != 12.5 {
		t.Errorf("Wohnung 2 PVAnteilKWh = %v, want 12,5", got)
	}
	if got := k.Anteil(1).PVAnteilKWh; got != 0 {
		t.Errorf("Wohnung 1 PVAnteilKWh = %v, want 0", got)
	}
}
