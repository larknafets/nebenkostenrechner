package store

import (
	"path/filepath"
	"testing"
)

func mieteTestWerte() map[int64]FixkostenPositionWert {
	werte := map[int64]FixkostenPositionWert{}
	for _, kd := range KostenpositionDefaults {
		werte[kd.ID] = FixkostenPositionWert{Logik: kd.Logik, Typ: kd.Typ, Wert: 1}
	}
	return werte
}

// TestFixkostenEingabe_MieteRoundtrip: Miete is stored per Eingabe and
// apartment, an update leaves an apartment without a map entry untouched.
func TestFixkostenEingabe_MieteRoundtrip(t *testing.T) {
	db := openTestDB(t)
	in := FixkostenInput{Monat: "2026-09-01", Personen: map[int64]float64{1: 1, 2: 1}, Werte: mieteTestWerte(),
		Abschlag: map[int64]float64{2: 100}, Miete: map[int64]float64{2: 750}}
	id, err := CreateFixkostenEingabe(db, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	in.Miete = map[int64]float64{1: 300}
	if err := UpdateFixkostenEingabe(db, id, in); err != nil {
		t.Fatalf("Update: %v", err)
	}
	d, err := GetFixkostenEingabeDetails(db, id)
	if err != nil || d == nil {
		t.Fatalf("Details: %v %v", d, err)
	}
	if d.Miete[2] != 750 || d.Miete[1] != 300 {
		t.Errorf("Miete = %v, want 1:300 2:750", d.Miete)
	}
	if err := DeleteFixkostenEingabe(db, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestOpen_MieteTabelleNeuUndLeer: a database from before the Miete (#277)
// gets the new table empty, its Abschläge stay, and it is saved first.
func TestOpen_MieteTabelleNeuUndLeer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := CreateFixkostenEingabe(db, FixkostenInput{
		Monat: "2026-09-01", Personen: map[int64]float64{1: 1, 2: 1}, Werte: mieteTestWerte(),
		Abschlag: map[int64]float64{2: 120},
	}); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	// Back to a database from before the release: no table, old version.
	for _, stmt := range []string{`DROP TABLE mieten`, `PRAGMA user_version = 1`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open nach Update: %v", err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM mieten`).Scan(&n); err != nil || n != 0 {
		t.Errorf("mieten = %d, %v, want neue leere Tabelle", n, err)
	}
	all, err := AllFixkostenEingabenDetails(db)
	if err != nil || len(all) != 1 || all[0].Abschlag[2] != 120 {
		t.Fatalf("Eingaben nach Update = %+v, %v, want der Abschlag 120 bleibt", all, err)
	}
	if _, ok := all[0].Miete[2]; ok {
		t.Error("Miete nach Update vorhanden, want nicht erfasst")
	}
	if got := sicherungsDateien(t, path); len(got) != 1 {
		t.Errorf("Sicherungen = %v, want eine vor dem Update", got)
	}
}
