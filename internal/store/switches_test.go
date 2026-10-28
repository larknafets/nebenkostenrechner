package store

import (
	"path/filepath"
	"testing"
)

// switches_test.go covers every per-Ablesung switch (#225, #226) through the
// Switches descriptor, so a switch that is not wired up everywhere fails here:
// persisted, editable, loaded alone and in bulk, and off for every Ablesung of
// a database that predates it.

func TestSwitches_Roundtrip(t *testing.T) {
	for _, s := range Switches {
		t.Run(s.Name, func(t *testing.T) {
			db := openTestDB(t)

			in := vollePeriode("2026-06-01", nil)
			*s.Input(&in) = true
			id, err := CreatePeriod(db, in)
			if err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			other, err := CreatePeriod(db, vollePeriode("2026-07-01", nil))
			if err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}

			got, _ := GetPeriodDetails(db, id)
			if !*s.Detail(got) {
				t.Error("switch off after create, want on")
			}
			if got, _ := GetPeriodDetails(db, other); *s.Detail(got) != s.Default {
				t.Errorf("second Ablesung switch = %v, want default %v", *s.Detail(got), s.Default)
			}
			all, err := AllPeriodDetails(db)
			if err != nil || len(all) != 2 || !*s.Detail(all[0]) || *s.Detail(all[1]) != s.Default {
				t.Errorf("AllPeriodDetails = %+v, %v, want only the first with the switch", all, err)
			}
		})
	}
}

// TestSwitches_UpdateLaesstSchalterUnberuehrt pins that UpdatePeriod does not
// write the switches any more (#359): they belong to the Fixkosten-Eingabe, an
// Ablesung form without them must not switch them off, and a Monat left out
// keeps the stored Monat (COALESCE).
func TestSwitches_UpdateLaesstSchalterUnberuehrt(t *testing.T) {
	for _, s := range Switches {
		t.Run(s.Name, func(t *testing.T) {
			db := openTestDB(t)
			in := vollePeriode("2026-06-01", nil)
			*s.Input(&in) = true
			id, err := CreatePeriod(db, in)
			if err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			in.Monat = ""
			*s.Input(&in) = false
			if err := UpdatePeriod(db, id, in); err != nil {
				t.Fatalf("UpdatePeriod: %v", err)
			}
			got, _ := GetPeriodDetails(db, id)
			if !*s.Detail(got) || got.Monat == "" {
				t.Errorf("after update: switch = %v, Monat = %q, want switch kept on and Monat kept", *s.Detail(got), got.Monat)
			}
		})
	}
}

func TestGetPeriodDetails_UnbekannteId(t *testing.T) {
	db := openTestDB(t)
	if got, err := GetPeriodDetails(db, 4711); got != nil || err != nil {
		t.Errorf("GetPeriodDetails(unknown) = %v, %v, want nil, nil", got, err)
	}
}

// TestSwitches_BestehendeDatenbank opens a database whose periods table has
// no switch columns yet: the existing Ablesungen come out with the switch at
// its default, nothing else changes.
func TestSwitches_BestehendeDatenbank(t *testing.T) {
	for _, s := range Switches {
		t.Run(s.Name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "alt.db")
			db, err := Open(path)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			id, err := CreatePeriod(db, vollePeriode("2026-05-01", map[string]float64{"strom_gesamt": 100}))
			if err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			if _, err := db.Exec(`ALTER TABLE periods DROP COLUMN ` + s.Column); err != nil {
				t.Fatalf("back to the shape before the switch: %v", err)
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
			if *s.Detail(got) != s.Default || got.Readings["strom_gesamt"] != 100 || OrZero(got.Strompreis) != 0.22 {
				t.Errorf("Ablesung nach Öffnen = %+v, want Schalter auf Default, Rest unverändert", got)
			}
		})
	}
}
