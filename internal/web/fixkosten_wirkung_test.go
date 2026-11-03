package web

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestFixkostenWirkungVon(t *testing.T) {
	periode := func(id int64, datum, monat string, teil bool) *store.LatestPeriod {
		r := map[string]float64{}
		for _, key := range store.MeterKeys {
			r[key] = 0
		}
		if teil {
			delete(r, store.MeterKeys[0])
		}
		return &store.LatestPeriod{ID: id, ReadingDate: datum, Monat: monat, Readings: r}
	}
	eingabe := func(id int64, vollstaendig bool) *store.FixkostenEingabeDetails {
		e := &store.FixkostenEingabeDetails{ID: id, Monat: "2026-10-01", Personen: map[int64]float64{1: 1, 2: 1},
			Strompreis: store.Float64(1), FrischwasserPreis: store.Float64(1), AbwasserPreis: store.Float64(1)}
		if vollstaendig {
			e.EinspeisungPreis = store.Float64(1)
		}
		return e
	}
	personen := store.Sonderkosten{Monat: "2026-10-01", Umlegbar: true, Umsetzung: store.UmsetzungHaus, Logik: store.LogikPersonen}
	nichtUmlegbar := personen
	nichtUmlegbar.Umlegbar = false
	anderer := personen
	anderer.Monat = "2026-09-01"

	basis := []*store.LatestPeriod{periode(1, "2026-09-10", "2026-09-01", false), periode(2, "2026-10-10", "2026-10-01", false)}
	mehrere := []*store.LatestPeriod{basis[0], basis[1], periode(3, "2026-10-20", "2026-10-01", false)}
	teilstand := []*store.LatestPeriod{basis[0], periode(2, "2026-10-10", "2026-10-01", true)}
	tests := []struct {
		name         string
		periods      []*store.LatestPeriod
		eingaben     []*store.FixkostenEingabeDetails
		sonder       []store.Sonderkosten
		vollstaendig bool
		eingabeID    int64
		ablesungen   int
		sonderkosten int
		fehlt, bere  bool
	}{
		{"keine Eingabe", basis, nil, nil, false, 0, 1, 0, true, false},
		{"unvollständig", basis, []*store.FixkostenEingabeDetails{eingabe(7, false)}, nil, false, 7, 1, 0, true, false},
		{"vollständig, Ablesung wartet", basis, []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 1, 0, false, true},
		{"nur erste Ablesung", basis[1:], []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 0, 0, false, false},
		{"nur Teilstand", teilstand, nil, nil, false, 0, 0, 0, false, false},
		{"mehrere Ablesungen", mehrere, []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 2, 0, false, true},
		{"eine Sonderkosten", basis, nil, []store.Sonderkosten{personen, nichtUmlegbar, anderer}, false, 0, 1, 1, true, false},
		{"viele Sonderkosten", basis, []*store.FixkostenEingabeDetails{eingabe(7, true)}, []store.Sonderkosten{personen, personen}, true, 7, 1, 2, false, true},
	}
	for _, tt := range tests {
		ds := &store.Datenstand{Periods: tt.periods, Eingaben: tt.eingaben, Sonderkosten: tt.sonder, Apartments: []store.Apartment{{ID: 1, QM: 1}, {ID: 2, QM: 1}}}
		w := fixkostenWirkungVon(ds, calc.KostenAus(ds), "2026-10-01")
		if w.Monat != "2026-10-01" || w.Vollstaendig != tt.vollstaendig || w.EingabeID != tt.eingabeID ||
			len(w.Ablesungen) != tt.ablesungen || w.Sonderkosten != tt.sonderkosten || w.Fehlt() != tt.fehlt || w.Berechenbar() != tt.bere {
			t.Errorf("%s: got %+v (Fehlt %v, Berechenbar %v)", tt.name, w, w.Fehlt(), w.Berechenbar())
		}
	}
}
