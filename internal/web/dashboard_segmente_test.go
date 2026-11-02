package web

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// All Sonderkosten of a month are one segment of their own, whatever their
// Logik: a large invoice must not inflate the bar of a Logik group.
func TestFixkostenGruppen_SonderkostenEinSegment(t *testing.T) {
	segs := fixkostenGruppen(1, nil, []calc.MonatsSonderkosten{
		{Monat: "2026-10-01", Logik: store.LogikWohneinheit, Betrag: 714.25},
		{Monat: "2026-10-01", Logik: store.LogikWohnung1, Betrag: 184.30},
	})
	for _, s := range segs {
		switch s.Farbe {
		case "logik-sonderkosten":
			if s.Kosten != 898.55 {
				t.Errorf("Sonderkosten = %v, want 898.55", s.Kosten)
			}
		default:
			if s.Kosten != 0 {
				t.Errorf("%s = %v, want 0", s.Farbe, s.Kosten)
			}
		}
	}
	if last := segs[len(segs)-1]; last.Farbe != "logik-sonderkosten" || last.Label != "Sonderkosten" {
		t.Errorf("last segment = %+v, want the Sonderkosten segment", last)
	}
}
