package store

import (
	"testing"
	"time"
)

func TestSeedDemoData(t *testing.T) {
	db := openTestDB(t)
	now := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)

	if err := SeedDemoData(db, now); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	periods, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(periods) != 39 {
		t.Fatalf("got %d periods, want 39", len(periods))
	}

	eingaben, err := AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(eingaben) != 39 {
		t.Fatalf("got %d fixkosten eingaben, want 39", len(eingaben))
	}

	// Neuester Monat = now's Monat, aeltester = 38 Monate zuvor.
	newest, oldest := periods[0], periods[len(periods)-1]
	if want := "2026-03-01"; newest.Monat != want {
		t.Errorf("newest period Monat = %q, want %q", newest.Monat, want)
	}
	if want := "2023-01-01"; oldest.Monat != want {
		t.Errorf("oldest period Monat = %q, want %q", oldest.Monat, want)
	}

	// Stammdaten wurden gesetzt (nicht der 0-Default eines frischen Installs).
	apartments, err := Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}
	for _, a := range apartments {
		if a.QM == 0 || a.FlurstueckGroesse == 0 {
			t.Errorf("apartment %d QM/FlurstueckGroesse still 0", a.ID)
		}
	}

	// Saisonalitaet: waerme_wohnung1-Zuwachs im Sommer (Juni) faktisch 0,
	// im Winter (Januar) deutlich groesser als 0.
	var summerDelta, winterDelta float64
	for i := 1; i < len(periods); i++ {
		older, err := GetPeriodDetails(db, periods[i].ID)
		if err != nil {
			t.Fatalf("GetPeriodDetails: %v", err)
		}
		newer, err := GetPeriodDetails(db, periods[i-1].ID)
		if err != nil {
			t.Fatalf("GetPeriodDetails: %v", err)
		}
		delta := newer.Readings["waerme_wohnung1"] - older.Readings["waerme_wohnung1"]
		month, err := time.Parse("2006-01-02", newer.Monat)
		if err != nil {
			t.Fatalf("parse Monat %q: %v", newer.Monat, err)
		}
		switch month.Month() {
		case time.June, time.July, time.August:
			summerDelta = delta
		case time.January:
			winterDelta = delta
		}
	}
	if summerDelta != 0 {
		t.Errorf("waerme_wohnung1 Sommer-Delta = %v, want 0 (keine Heizung im Sommer)", summerDelta)
	}
	if winterDelta <= 1.0 {
		t.Errorf("waerme_wohnung1 Winter-Delta = %v, want > 1.0 (Heizsaison)", winterDelta)
	}

	// Personenzahl Wohnung 2 variiert zwischen 1 und 2, Wohnung 1 immer 2.
	sawOne, sawTwo := false, false
	for _, p := range periods {
		details, err := GetPeriodDetails(db, p.ID)
		if err != nil {
			t.Fatalf("GetPeriodDetails: %v", err)
		}
		if details.PersonenByApartment[1] != 2 {
			t.Errorf("period %s: Personen Wohnung 1 = %d, want 2", p.Monat, details.PersonenByApartment[1])
		}
		switch details.PersonenByApartment[2] {
		case 1:
			sawOne = true
		case 2:
			sawTwo = true
		}
	}
	if !sawOne || !sawTwo {
		t.Errorf("Personen Wohnung 2 varies: sawOne=%v sawTwo=%v, want both true", sawOne, sawTwo)
	}
}

// TestResetDemoData covers Issue #121: ein erneuter Demo-Login muss
// Änderungen aus einer vorherigen Demo-Session vollständig verwerfen und
// wieder exakt den frischen 39-Monats-Ausgangszustand zeigen, mit dem
// neuesten Monat am aktuellen Kalendermonat, nicht an einem eingefrorenen
// historischen Datum.
func TestResetDemoData(t *testing.T) {
	db := openTestDB(t)
	firstRun := time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC)
	if err := SeedDemoData(db, firstRun); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	// Simuliert eine im Demo-Modus vorgenommene Änderung, die der Reset
	// verwerfen muss.
	readings := make(map[string]float64, len(MeterKeys))
	for _, key := range MeterKeys {
		readings[key] = 0
	}
	if _, err := CreatePeriod(db, PeriodInput{
		ReadingDate: "2099-01-01",
		Monat:       "2099-01-01",
		Personen:    map[int64]int64{1: 2, 2: 2},
		Readings:    readings,
	}); err != nil {
		t.Fatalf("CreatePeriod (simulierte Demo-Änderung): %v", err)
	}
	if _, err := CreateFixkostenEingabe(db, FixkostenInput{
		Monat:    "2099-01-01",
		Personen: map[int64]int64{1: 2, 2: 2},
		Werte:    map[int64]FixkostenPositionWert{},
		Abschlag: map[int64]float64{1: 0, 2: 0},
	}); err != nil {
		t.Fatalf("CreateFixkostenEingabe (simulierte Demo-Änderung): %v", err)
	}

	secondRun := time.Date(2026, time.April, 20, 0, 0, 0, 0, time.UTC)
	if err := ResetDemoData(db, secondRun); err != nil {
		t.Fatalf("ResetDemoData: %v", err)
	}

	periods, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(periods) != 39 {
		t.Fatalf("got %d periods after reset, want 39", len(periods))
	}
	for _, p := range periods {
		if p.Monat == "2099-01-01" {
			t.Error("Reset hat die simulierte Demo-Änderung nicht verworfen (2099-01-01 noch vorhanden)")
		}
	}
	if want := "2026-04-01"; periods[0].Monat != want {
		t.Errorf("neuester Monat nach Reset = %q, want %q (aktueller Kalendermonat von secondRun)", periods[0].Monat, want)
	}

	eingaben, err := AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(eingaben) != 39 {
		t.Fatalf("got %d fixkosten eingaben after reset, want 39", len(eingaben))
	}
	for _, e := range eingaben {
		if e.Monat == "2099-01-01" {
			t.Error("Reset hat die simulierte Fixkosten-Demo-Änderung nicht verworfen (2099-01-01 noch vorhanden)")
		}
	}
}
