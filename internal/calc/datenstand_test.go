package calc

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestKostenAus_HandgebauterDatenstand builds the Datenstand by hand, without
// a database or seed data.
func TestKostenAus_HandgebauterDatenstand(t *testing.T) {
	readings := func(strom float64) map[string]float64 {
		r := map[string]float64{}
		for _, key := range store.MeterKeys {
			r[key] = 0
		}
		r["strom_gesamt"] = strom
		return r
	}
	periode := func(id int64, datum string, strom float64) *store.LatestPeriod {
		return &store.LatestPeriod{
			ID: id, ReadingDate: datum, Monat: datum,
			Readings: readings(strom),
		}
	}
	eingabe := &store.FixkostenEingabeDetails{
		Monat: "2026-10-01", Personen: map[int64]float64{1: 2, 2: 1},
		Strompreis: store.Float64(0.3), FrischwasserPreis: store.Float64(1), AbwasserPreis: store.Float64(1), EinspeisungPreis: store.Float64(0.1),
	}
	ds := &store.Datenstand{
		Periods:    []*store.LatestPeriod{periode(1, "2026-09-01", 100), periode(2, "2026-10-01", 200)},
		Eingaben:   []*store.FixkostenEingabeDetails{eingabe},
		Apartments: []store.Apartment{{ID: 1, Name: "Wohnung 1", QM: 80}, {ID: 2, Name: "Wohnung 2", QM: 60}},
	}

	kosten := KostenAus(ds)
	if a, ok := kosten.Ablesung(1); !ok || a.Grund != GrundKeineVorperiode {
		t.Errorf("erste Ablesung = %+v, %v, want GrundKeineVorperiode", a, ok)
	}
	if a, ok := kosten.Ablesung(2); !ok || !a.Berechenbar() {
		t.Errorf("zweite Ablesung = %+v, %v, want berechenbar", a, ok)
	}
	if len(kosten.Berechenbare()) != 1 {
		t.Errorf("Berechenbare = %d, want 1", len(kosten.Berechenbare()))
	}

	ohne := &store.Datenstand{Periods: ds.Periods, Apartments: ds.Apartments}
	if a, _ := KostenAus(ohne).Ablesung(2); a.Grund != GrundFixkostenFehlen {
		t.Errorf("zweite Ablesung ohne Eingaben: Grund = %v, want GrundFixkostenFehlen", a.Grund)
	}
	if reihe := FixkostenreiheAus(ohne); len(reihe.Neueste()) != 0 {
		t.Errorf("Fixkostenreihe ohne Eingaben = %d, want 0", len(reihe.Neueste()))
	}
}
