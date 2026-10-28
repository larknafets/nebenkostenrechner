package store

import "testing"

// TestNeueDB_PersonenSpaltenSindReal: fresh databases get REAL columns
// (Entscheidung #237).
func TestNeueDB_PersonenSpaltenSindReal(t *testing.T) {
	db := openTestDB(t)
	var typ string
	err := db.QueryRow(`SELECT type FROM pragma_table_info('fixkosten_personen') WHERE name = 'personen'`).Scan(&typ)
	if err != nil {
		t.Fatalf("table_info fixkosten_personen: %v", err)
	}
	if typ != "REAL" {
		t.Errorf("fixkosten_personen.personen = %q, want REAL", typ)
	}
}

// TestBestehendeDB_IntegerSpalteSpeichertDezimalwert: an existing database
// keeps its INTEGER column, whose affinity still stores 1.5 as REAL.
func TestBestehendeDB_IntegerSpalteSpeichertDezimalwert(t *testing.T) {
	db := openTestDB(t)
	for _, stmt := range []string{
		`DROP TABLE fixkosten_personen`,
		`CREATE TABLE fixkosten_personen (id INTEGER PRIMARY KEY, fixkosten_eingabe_id INTEGER NOT NULL REFERENCES fixkosten_eingaben(id), apartment_id INTEGER NOT NULL REFERENCES apartments(id), personen INTEGER NOT NULL, UNIQUE(fixkosten_eingabe_id, apartment_id))`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
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
