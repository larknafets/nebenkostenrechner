package store

import (
	"math"
	"testing"
	"time"
)

// TestSeedDemoData_Einzelzaehler: the younger demo months carry both
// Einzelzähler, Wohnung 2 with about 40 % of the Zwischenzähler, Wohnung 1
// the rest; the older months have none.
func TestSeedDemoData_Einzelzaehler(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	if err := SeedDemoData(db, now); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	periods, err := AllPeriodDetails(db) // oldest first
	if err != nil {
		t.Fatalf("AllPeriodDetails: %v", err)
	}

	with, verteilt := 0, 0
	for i, p := range periods {
		_, has1 := p.Readings[keyWallboxW1]
		_, has2 := p.Readings[keyWallboxW2]
		if has1 != has2 {
			t.Fatalf("Monat %s: nur ein Einzelzähler gesetzt", p.Monat)
		}
		if !has1 {
			continue
		}
		with++
		if i == 0 {
			continue
		}
		prev := periods[i-1]
		d1, ok1 := Verbrauch(p.Readings, prev.Readings, keyWallboxW1)
		d2, ok2 := Verbrauch(p.Readings, prev.Readings, keyWallboxW2)
		if !ok1 || !ok2 {
			// The first month with Einzelzähler has no Vorperiode.
			continue
		}
		verteilt++
		zwischen := p.Readings["strom_wallbox"] - prev.Readings["strom_wallbox"]
		if math.Abs(d1+d2-zwischen) > 0.01 {
			t.Errorf("Monat %s: Wohnung 1 + Wohnung 2 = %v, want Zwischenzähler %v", p.Monat, d1+d2, zwischen)
		}
		if share := d2 / zwischen; math.Abs(share-0.4) > 0.01 {
			t.Errorf("Monat %s: Anteil Wohnung 2 = %v, want etwa 0,4", p.Monat, share)
		}
	}
	if with == 0 || with >= len(periods) {
		t.Errorf("%d von %d Monaten mit Einzelzählern, want nur die jüngeren", with, len(periods))
	}
	if verteilt != with-1 {
		t.Errorf("%d Monate mit berechenbarem Verbrauch, want %d (alle außer dem ersten)", verteilt, with-1)
	}
	if _, ok := periods[0].Readings[keyWallboxW1]; ok {
		t.Error("ältester Demo-Monat hat Einzelzähler")
	}
	if _, ok := periods[len(periods)-1].Readings[keyWallboxW2]; !ok {
		t.Error("jüngster Demo-Monat hat keinen Einzelzähler")
	}
}
