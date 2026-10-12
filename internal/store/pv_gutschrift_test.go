package store

import (
	"path/filepath"
	"testing"
)

// pv_gutschrift_test.go covers the per-Ablesung switch "PV-Gutschrift
// Wohnung 1" (#226): persisted, editable, and off for every Ablesung of a
// database that predates it.

func TestPVGutschriftW1_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	in := vollePeriode("2026-06-01", nil)
	in.PVGutschriftW1 = true
	id, err := CreatePeriod(db, in)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	other, err := CreatePeriod(db, vollePeriode("2026-07-01", nil))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	got, _ := GetPeriodDetails(db, id)
	if !got.PVGutschriftW1 {
		t.Error("PVGutschriftW1 = false after create, want true")
	}
	if got, _ := GetPeriodDetails(db, other); got.PVGutschriftW1 {
		t.Error("second Ablesung PVGutschriftW1 = true, want false (default)")
	}
	all, err := AllPeriodDetails(db)
	if err != nil || len(all) != 2 || !all[0].PVGutschriftW1 || all[1].PVGutschriftW1 {
		t.Errorf("AllPeriodDetails = %+v, %v, want only the first with the switch", all, err)
	}

	in.PVGutschriftW1 = false
	if err := UpdatePeriod(db, id, in); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	if got, _ := GetPeriodDetails(db, id); got.PVGutschriftW1 {
		t.Error("PVGutschriftW1 = true after switching off, want false")
	}

	in.PVGutschriftW1 = true
	if err := UpdatePeriod(db, id, in); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	if got, _ := GetPeriodDetails(db, id); !got.PVGutschriftW1 {
		t.Error("PVGutschriftW1 = false after switching on, want true")
	}
}

// TestPVGutschriftW1_BestehendeDatenbank opens a database whose periods table
// has no switch column yet: the existing Ablesungen come out with the switch
// off, nothing else changes.
func TestPVGutschriftW1_BestehendeDatenbank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id, err := CreatePeriod(db, vollePeriode("2026-05-01", map[string]float64{"strom_gesamt": 100}))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE periods DROP COLUMN pv_gutschrift_w1`); err != nil {
		t.Fatalf("back to the pre-#226 shape: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open (existing database): %v", err)
	}
	t.Cleanup(func() { db.Close() })

	got, err := GetPeriodDetails(db, id)
	if err != nil || got == nil {
		t.Fatalf("GetPeriodDetails = %v, %v", got, err)
	}
	if got.PVGutschriftW1 || got.Readings["strom_gesamt"] != 100 || OrZero(got.Strompreis) != 0.22 {
		t.Errorf("Ablesung nach Öffnen = %+v, want Schalter aus, Rest unverändert", got)
	}
}
