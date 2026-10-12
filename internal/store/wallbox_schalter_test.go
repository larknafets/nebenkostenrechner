package store

import (
	"path/filepath"
	"testing"
)

// wallbox_schalter_test.go covers the per-Ablesung switch "Wallboxen
// vollständig Wohnung 1" (#225): persisted, editable, and off for every
// Ablesung of a database that predates it.

func TestWallboxKomplettW1_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	in := vollePeriode("2026-06-01", nil)
	in.WallboxKomplettW1 = true
	id, err := CreatePeriod(db, in)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	other, err := CreatePeriod(db, vollePeriode("2026-07-01", nil))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	got, _ := GetPeriodDetails(db, id)
	if !got.WallboxKomplettW1 {
		t.Error("WallboxKomplettW1 = false after create, want true")
	}
	if got, _ := GetPeriodDetails(db, other); got.WallboxKomplettW1 {
		t.Error("second Ablesung WallboxKomplettW1 = true, want false (default)")
	}

	in.WallboxKomplettW1 = false
	if err := UpdatePeriod(db, id, in); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	if got, _ := GetPeriodDetails(db, id); got.WallboxKomplettW1 {
		t.Error("WallboxKomplettW1 = true after switching off, want false")
	}

	in.WallboxKomplettW1 = true
	if err := UpdatePeriod(db, id, in); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	if got, _ := GetPeriodDetails(db, id); !got.WallboxKomplettW1 {
		t.Error("WallboxKomplettW1 = false after switching on, want true")
	}
}

// TestWallboxKomplettW1_BestehendeDatenbank opens a database whose periods
// table has no switch column yet: the existing Ablesungen come out with the
// switch off, nothing else changes.
func TestWallboxKomplettW1_BestehendeDatenbank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id, err := CreatePeriod(db, vollePeriode("2026-05-01", map[string]float64{"strom_gesamt": 100}))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE periods DROP COLUMN wallbox_komplett_w1`); err != nil {
		t.Fatalf("back to the pre-#225 shape: %v", err)
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
	if got.WallboxKomplettW1 || got.Readings["strom_gesamt"] != 100 || OrZero(got.Strompreis) != 0.22 {
		t.Errorf("Ablesung nach Öffnen = %+v, want Schalter aus, Rest unverändert", got)
	}
}
