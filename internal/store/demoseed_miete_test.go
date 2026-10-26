package store

import (
	"testing"
	"time"
)

// TestSeedDemoData_Miete: the rented demo apartment (Wohnung 2) gets a rent
// in every Fixkosten-Eingabe, the owner-occupied one none.
func TestSeedDemoData_Miete(t *testing.T) {
	db := openTestDB(t)
	if err := SeedDemoData(db, time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	all, err := AllFixkostenEingabenDetails(db)
	if err != nil || len(all) == 0 {
		t.Fatalf("Eingaben = %d, %v", len(all), err)
	}
	for _, f := range all {
		if f.Miete[2] <= 0 {
			t.Fatalf("%s: Miete Wohnung 2 = %v, want > 0", f.Monat, f.Miete[2])
		}
		if _, ok := f.Miete[1]; ok {
			t.Fatalf("%s: Wohnung 1 (Eigennutzung) hat eine Miete", f.Monat)
		}
	}
}
