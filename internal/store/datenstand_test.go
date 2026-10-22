package store

import "testing"

func TestLadeDatenstand(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))
	mustCreatePeriod(t, db, "2026-10-01", baseReadings(nil))

	ds, err := LadeDatenstand(db)
	if err != nil {
		t.Fatalf("LadeDatenstand: %v", err)
	}
	if len(ds.Periods) != 2 || ds.Periods[0].ReadingDate != "2026-09-01" || ds.Periods[1].ReadingDate != "2026-10-01" {
		t.Errorf("Periods = %v, want die zwei Ablesungen, älteste zuerst", ds.Periods)
	}
	if len(ds.Apartments) != 2 {
		t.Errorf("Apartments = %d, want 2", len(ds.Apartments))
	}
	if len(ds.Kostenpositionen) == 0 || len(ds.Meters) == 0 {
		t.Errorf("Kostenpositionen = %d, Meters = %d, want Stammdaten geladen", len(ds.Kostenpositionen), len(ds.Meters))
	}
}
