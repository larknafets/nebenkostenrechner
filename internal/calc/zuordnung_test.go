package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

func TestFixkostenPosition_KostenFor_WaehltWohnung(t *testing.T) {
	p := calc.FixkostenPosition{KostenW1: 11.11, KostenW2: 22.22}
	cases := []struct {
		id   int64
		want float64
	}{{1, 11.11}, {2, 22.22}, {0, 11.11}, {3, 11.11}}
	for _, c := range cases {
		if got := p.KostenFor(c.id); got != c.want {
			t.Errorf("KostenFor(%d) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestFixkostenErgebnis_KostenFor_WaehltWohnung(t *testing.T) {
	e := calc.FixkostenErgebnis{KostenW1: 33.33, KostenW2: 44.44}
	cases := []struct {
		id   int64
		want float64
	}{{1, 33.33}, {2, 44.44}, {0, 33.33}, {3, 33.33}}
	for _, c := range cases {
		if got := e.KostenFor(c.id); got != c.want {
			t.Errorf("KostenFor(%d) = %v, want %v", c.id, got, c.want)
		}
	}
}

func TestStromErgebnis_WPVerbrauchKWh_Summe(t *testing.T) {
	cases := []struct {
		name   string
		wp, pv float64
		want   float64
	}{
		{"beide", 120, 30, 150},
		{"nur Netz", 120, 0, 120},
		{"nur PV", 0, 30, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := &calc.StromErgebnis{WPAnteilKWh: c.wp, PVAnteilWPKWh: c.pv}
			if got := e.WPVerbrauchKWh(); got != c.want {
				t.Errorf("WPVerbrauchKWh() = %v, want %v", got, c.want)
			}
		})
	}
}
