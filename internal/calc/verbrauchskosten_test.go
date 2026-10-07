package calc_test

import (
	"reflect"
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

// TestLoad_MatchesPerPeriodCalc pins the in-memory model to the per-period
// calc functions it replaces: same numbers for every calculable Ablesung.
func TestLoad_MatchesPerPeriodCalc(t *testing.T) {
	db := openTestDB(t)
	var ids []int64
	for i, date := range []string{"2026-09-01", "2026-10-01", "2026-11-01", "2026-11-15"} {
		n := float64(i)
		id, err := store.CreatePeriod(db, store.PeriodInput{
			ReadingDate: date, Monat: date[:8] + "01",
			Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
			AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
			Readings: baseReadings(map[string]float64{
				"strom_gesamt": 1000 * n * n, "strom_wohnung2": 300 * n, "strom_waermepumpe": 700 * n,
				"strom_einspeisung": 500 * n, "waerme_wohnung1": 3 * n, "waerme_wohnung2": 2 * n,
				"wasser_gesamt": 18 * n, "wasser_wohnung2": 7 * n, "wasser_warmwasseraufbereitung": 5 * n,
			}),
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
	if got := len(v.Berechenbare()); got != 3 {
		t.Fatalf("len(Berechenbare()) = %d, want 3", got)
	}
	for _, id := range ids[1:] {
		a, _ := v.Ablesung(id)
		strom, _ := calc.Strom(db, id)
		wasser, _ := calc.Wasser(db, id)
		heizung, _ := calc.Heizung(db, id)
		einspeisung, _ := calc.Einspeisung(db, id)
		want := calc.Kosten{Strom: strom, Wasser: wasser, Heizung: heizung, Einspeisung: einspeisung}
		if !reflect.DeepEqual(a.Kosten, want) {
			t.Errorf("Ablesung %d Kosten = %+v, want %+v", id, a.Kosten, want)
		}
	}
}
