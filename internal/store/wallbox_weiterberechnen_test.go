package store

import (
	"path/filepath"
	"testing"
)

// wallbox_weiterberechnen_test.go covers the house-wide flag "Wallbox
// weiterberechnen" (#228): default on, savable, and on for a database that
// predates it.

func TestWallboxWeiterberechnen_StandardAnUndSpeicherbar(t *testing.T) {
	db := openTestDB(t)

	h, err := GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	if !h.WallboxWeiterberechnen {
		t.Error("WallboxWeiterberechnen = false on a new database, want true (default)")
	}

	for _, want := range []bool{false, true} {
		if err := SaveStammdaten(db, StammdatenSave{
			Flags:                   StammdatenFlags{WallboxWeiterberechnen: want},
			HeizungWaermeGewichtung: 0.7,
		}); err != nil {
			t.Fatalf("SaveStammdaten: %v", err)
		}
		if h, _ := GetHaus(db); h.WallboxWeiterberechnen != want {
			t.Errorf("WallboxWeiterberechnen = %v after saving %v", h.WallboxWeiterberechnen, want)
		}
	}
}

func TestWallboxWeiterberechnen_BestehendeDatenbank(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := db.Exec(`UPDATE haus SET strom_weiterberechnen = 0 WHERE id = 1`); err != nil {
		t.Fatalf("set the existing flag: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE haus DROP COLUMN wallbox_weiterberechnen`); err != nil {
		t.Fatalf("back to the pre-#228 shape: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open (existing database): %v", err)
	}
	t.Cleanup(func() { db.Close() })

	h, err := GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	if !h.WallboxWeiterberechnen || h.StromWeiterberechnen {
		t.Errorf("Haus = %+v, want Wallbox weiterberechnen on (default), the existing Strom flag off kept", h)
	}
}
