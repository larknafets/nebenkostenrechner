package calc_test

import (
	"database/sql"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func mustCreatePeriodMitEinspeisung(t *testing.T, db *sql.DB, date string, einspeisungPreis float64, readings map[string]float64) int64 {
	t.Helper()
	id, err := store.CreatePeriod(db, store.PeriodInput{
		ReadingDate:       date,
		Monat:             monatVon(date),
		Strompreis:        store.Float64(0.22),
		FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis:     store.Float64(4.87),
		EinspeisungPreis:  store.Float64(einspeisungPreis),
		Readings:          readings,
		Personen:          map[int64]int64{1: 2, 2: 1},
	})
	if err != nil {
		t.Fatalf("create period %s: %v", date, err)
	}
	return id
}

func TestEinspeisung_ErtragBerechnung(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriodMitEinspeisung(t, db, "2026-10-01", 0.08, baseReadings(map[string]float64{
		"strom_einspeisung": 1000,
	}))
	p2 := mustCreatePeriodMitEinspeisung(t, db, "2026-11-01", 0.08, baseReadings(map[string]float64{
		"strom_einspeisung": 1350,
	}))

	got := kostenVon(t, db, p2).Einspeisung
	if got.EinspeisungKWh != 350 {
		t.Errorf("EinspeisungKWh = %v, want 350", got.EinspeisungKWh)
	}
	if got.Ertrag != 28.00 {
		t.Errorf("Ertrag = %v, want 28.00 (350 * 0.08)", got.Ertrag)
	}
}
