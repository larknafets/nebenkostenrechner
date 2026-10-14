package store

import "testing"

// TestNeueDB_PersonenSpaltenSindReal: fresh databases get REAL columns
// (Entscheidung #237).
func TestNeueDB_PersonenSpaltenSindReal(t *testing.T) {
	db := openTestDB(t)
	for _, tabelle := range []string{"period_occupancy", "fixkosten_personen"} {
		var typ string
		err := db.QueryRow(`SELECT type FROM pragma_table_info(?) WHERE name = 'personen'`, tabelle).Scan(&typ)
		if err != nil {
			t.Fatalf("table_info %s: %v", tabelle, err)
		}
		if typ != "REAL" {
			t.Errorf("%s.personen = %q, want REAL", tabelle, typ)
		}
	}
}

// TestBestehendeDB_IntegerSpalteSpeichertDezimalwert: an existing database
// keeps its INTEGER column, whose affinity still stores 1.5 as REAL.
func TestBestehendeDB_IntegerSpalteSpeichertDezimalwert(t *testing.T) {
	db := openTestDB(t)
	for _, stmt := range []string{
		`DROP TABLE period_occupancy`,
		`CREATE TABLE period_occupancy (id INTEGER PRIMARY KEY, period_id INTEGER NOT NULL REFERENCES periods(id), apartment_id INTEGER NOT NULL REFERENCES apartments(id), personen INTEGER NOT NULL, UNIQUE(period_id, apartment_id))`,
		`DROP TABLE fixkosten_personen`,
		`CREATE TABLE fixkosten_personen (id INTEGER PRIMARY KEY, fixkosten_eingabe_id INTEGER NOT NULL REFERENCES fixkosten_eingaben(id), apartment_id INTEGER NOT NULL REFERENCES apartments(id), personen INTEGER NOT NULL, UNIQUE(fixkosten_eingabe_id, apartment_id))`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}

	periodID, err := CreatePeriod(db, PeriodInput{
		ReadingDate:       "2026-09-01",
		Strompreis:        Float64(0.22),
		FrischwasserPreis: Float64(1.46),
		AbwasserPreis:     Float64(4.87),
		Readings:          baseReadings(nil),
		Personen:          map[int64]float64{1: 1.5, 2: 2},
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	got, err := PersonenByApartment(db, periodID)
	if err != nil {
		t.Fatalf("PersonenByApartment: %v", err)
	}
	if got[1] != 1.5 || got[2] != 2 {
		t.Errorf("Ablesung Personen = %v, want 1:1.5 2:2", got)
	}

	eingabeID, err := CreateFixkostenEingabe(db, FixkostenInput{Monat: "2026-09-01", Personen: map[int64]float64{1: 2.5, 2: 1}})
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	d, err := GetFixkostenEingabeDetails(db, eingabeID)
	if err != nil || d == nil {
		t.Fatalf("details: %v %v", d, err)
	}
	if d.Personen[1] != 2.5 || d.Personen[2] != 1 {
		t.Errorf("Fixkosten Personen = %v, want 1:2.5 2:1", d.Personen)
	}
}
