package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func vollePeriode(id int64, date string, readings map[string]float64) *store.LatestPeriod {
	return &store.LatestPeriod{
		ID: id, ReadingDate: date, Monat: date,
		Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
		Readings:            baseReadings(readings),
		PersonenByApartment: map[int64]int64{1: 2, 2: 1},
	}
}

func TestVerbrauchskosten_Gruende(t *testing.T) {
	teilstand := vollePeriode(3, "2026-12-01", nil)
	teilstand.Strompreis = nil
	v := calc.New(calc.Daten{
		Periods: []*store.LatestPeriod{
			vollePeriode(1, "2026-10-01", nil),
			vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100}),
			teilstand,
		},
		Apartments: []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}},
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})

	want := map[int64]calc.Grund{1: calc.GrundKeineVorperiode, 2: calc.GrundKeiner, 3: calc.GrundTeilstand}
	for id, grund := range want {
		a, ok := v.Ablesung(id)
		if !ok || a.Grund != grund {
			t.Errorf("Ablesung(%d) = %+v, ok=%v, want Grund %v", id, a, ok, grund)
		}
	}
	if _, ok := v.Ablesung(99); ok {
		t.Error("Ablesung(99) ok = true, want false")
	}
	if got := v.Berechenbare(); len(got) != 1 || got[0].Period.ID != 2 {
		t.Errorf("Berechenbare() = %+v, want nur Ablesung 2", got)
	}
}

// TestLoad_NurBerechenbareAblesungenInReihenfolge verifies Load reads every
// Ablesung from the database and Berechenbare returns all but the first,
// oldest first.
func TestLoad_NurBerechenbareAblesungenInReihenfolge(t *testing.T) {
	db := openTestDB(t)
	var ids []int64
	for i, date := range []string{"2026-09-01", "2026-10-01", "2026-11-01", "2026-11-15"} {
		n := float64(i)
		id, err := store.CreatePeriod(db, store.PeriodInput{
			ReadingDate: date, Monat: monatVon(date),
			Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
			AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
			Readings: baseReadings(map[string]float64{"strom_gesamt": 1000 * n, "strom_einspeisung": 500 * n}),
			Personen: map[int64]int64{1: 2, 2: 1},
		})
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		ids = append(ids, id)
	}

	v, err := calc.Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := v.Berechenbare()
	if len(got) != 3 {
		t.Fatalf("len(Berechenbare()) = %d, want 3", len(got))
	}
	for i, a := range got {
		if a.Period.ID != ids[i+1] {
			t.Errorf("Berechenbare()[%d] = Ablesung %d, want %d", i, a.Period.ID, ids[i+1])
		}
	}
	if got[0].Kosten.Einspeisung.EinspeisungKWh != 500 {
		t.Errorf("EinspeisungKWh = %v, want 500", got[0].Kosten.Einspeisung.EinspeisungKWh)
	}
}
