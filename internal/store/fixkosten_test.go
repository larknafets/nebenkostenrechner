package store

import (
	"database/sql"
	"errors"
	"testing"
)

func TestJahrFromMonat(t *testing.T) {
	jahr, err := JahrFromMonat("2026-09-01")
	if err != nil {
		t.Fatalf("JahrFromMonat: %v", err)
	}
	if jahr != 2026 {
		t.Errorf("JahrFromMonat(2026-09-01) = %d, want 2026", jahr)
	}

	if _, err := JahrFromMonat("garbage"); err == nil {
		t.Error("JahrFromMonat(garbage): want error, got nil")
	}
}

func TestKostenpositionen_Seed(t *testing.T) {
	db := openTestDB(t)

	kps, err := Kostenpositionen(db)
	if err != nil {
		t.Fatalf("Kostenpositionen: %v", err)
	}
	if len(kps) != 14 {
		t.Fatalf("Kostenpositionen = %d, want 14", len(kps))
	}

	wantKeys := []string{
		"grundsteuer", "gebaeudevers", "deich_grund", "deich_bau", "kreisverband",
		"abfall_haushalt", "abfall_personen", "abfall_biomuell", "abfall_restmuell",
		"strom_grundpreis", "trinkwasser", "abwasser", "internet", "wp_wartung",
	}
	for i, want := range wantKeys {
		if kps[i].Key != want {
			t.Errorf("Kostenpositionen[%d].Key = %q, want %q", i, kps[i].Key, want)
		}
		if kps[i].Label == "" {
			t.Errorf("Kostenpositionen[%d] (%s) has empty Label", i, kps[i].Key)
		}
	}
}

func mustCreateFixkostenEingabe(t *testing.T, db *sql.DB, monat string, werte map[int64]FixkostenPositionWert) int64 {
	t.Helper()
	id, err := CreateFixkostenEingabe(db, FixkostenInput{
		Monat:    monat,
		Personen: map[int64]int64{1: 2, 2: 1},
		Werte:    werte,
	})
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
	}
	return id
}

func TestFixkostenEingabe_CRUD_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]FixkostenPositionWert{
		10: {Logik: LogikWohneinheit, Typ: TypMonatlich, Wert: 12.50},
		13: {Logik: LogikWohneinheit, Typ: TypMonatlich, Wert: 39.90},
	})

	got, err := GetFixkostenEingabeDetails(db, id)
	if err != nil {
		t.Fatalf("GetFixkostenEingabeDetails: %v", err)
	}
	if got == nil {
		t.Fatal("GetFixkostenEingabeDetails: want entry, got nil")
	}
	if got.Monat != "2026-09-01" {
		t.Errorf("Monat = %q, want 2026-09-01", got.Monat)
	}
	if got.Werte[10].Wert != 12.50 || got.Werte[13].Wert != 39.90 {
		t.Errorf("Werte = %v, want {10:{...12.50}, 13:{...39.90}}", got.Werte)
	}
	if got.Werte[10].Logik != LogikWohneinheit || got.Werte[10].Typ != TypMonatlich {
		t.Errorf("Werte[10] Logik/Typ = %s/%s, want %s/%s", got.Werte[10].Logik, got.Werte[10].Typ, LogikWohneinheit, TypMonatlich)
	}
	if got.Personen[1] != 2 || got.Personen[2] != 1 {
		t.Errorf("Personen = %v, want {1:2, 2:1}", got.Personen)
	}

	// Update: neuer Monat, geaenderter Wert, neuer Personenstand.
	if err := UpdateFixkostenEingabe(db, id, FixkostenInput{
		Monat:    "2026-09-02",
		Personen: map[int64]int64{1: 3, 2: 1},
		Werte: map[int64]FixkostenPositionWert{
			10: {Logik: LogikWohneinheit, Typ: TypMonatlich, Wert: 15.00},
			13: {Logik: LogikWohneinheit, Typ: TypMonatlich, Wert: 39.90},
		},
	}); err != nil {
		t.Fatalf("UpdateFixkostenEingabe: %v", err)
	}
	got, err = GetFixkostenEingabeDetails(db, id)
	if err != nil {
		t.Fatalf("GetFixkostenEingabeDetails after update: %v", err)
	}
	if got.Monat != "2026-09-02" {
		t.Errorf("Monat after update = %q, want 2026-09-02", got.Monat)
	}
	if got.Werte[10].Wert != 15.00 {
		t.Errorf("Werte[10].Wert after update = %v, want 15.00", got.Werte[10].Wert)
	}
	if got.Personen[1] != 3 {
		t.Errorf("Personen[1] after update = %v, want 3", got.Personen[1])
	}

	latest, err := GetLatestFixkostenEingabe(db)
	if err != nil {
		t.Fatalf("GetLatestFixkostenEingabe: %v", err)
	}
	if latest == nil || latest.ID != id {
		t.Fatalf("GetLatestFixkostenEingabe = %+v, want id %d", latest, id)
	}
}

func TestGetFixkostenEingabeDetails_NotFound(t *testing.T) {
	db := openTestDB(t)
	got, err := GetFixkostenEingabeDetails(db, 999)
	if err != nil {
		t.Fatalf("GetFixkostenEingabeDetails: %v", err)
	}
	if got != nil {
		t.Errorf("GetFixkostenEingabeDetails(999) = %+v, want nil", got)
	}
}

func TestGetLatestFixkostenEingabe_NoEingaben(t *testing.T) {
	db := openTestDB(t)
	got, err := GetLatestFixkostenEingabe(db)
	if err != nil {
		t.Fatalf("GetLatestFixkostenEingabe: %v", err)
	}
	if got != nil {
		t.Errorf("GetLatestFixkostenEingabe on empty db = %+v, want nil", got)
	}
}

func TestUpdateFixkostenEingabe_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := UpdateFixkostenEingabe(db, 999, FixkostenInput{Monat: "2026-09-01"})
	if !errors.Is(err, ErrFixkostenEingabeNotFound) {
		t.Fatalf("UpdateFixkostenEingabe on unknown id: err = %v, want ErrFixkostenEingabeNotFound", err)
	}
}

func TestDeleteFixkostenEingabe(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]FixkostenPositionWert{
		10: {Logik: LogikWohneinheit, Typ: TypMonatlich, Wert: 12.50},
	})

	if err := DeleteFixkostenEingabe(db, id); err != nil {
		t.Fatalf("DeleteFixkostenEingabe: %v", err)
	}

	got, err := GetFixkostenEingabeDetails(db, id)
	if err != nil {
		t.Fatalf("GetFixkostenEingabeDetails after delete: %v", err)
	}
	if got != nil {
		t.Errorf("GetFixkostenEingabeDetails after delete = %+v, want nil", got)
	}

	all, err := AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("AllFixkostenEingaben after delete = %d, want 0", len(all))
	}
}

func TestDeleteFixkostenEingabe_NotFound(t *testing.T) {
	db := openTestDB(t)
	err := DeleteFixkostenEingabe(db, 999)
	if !errors.Is(err, ErrFixkostenEingabeNotFound) {
		t.Fatalf("DeleteFixkostenEingabe on unknown id: err = %v, want ErrFixkostenEingabeNotFound", err)
	}
}

func TestAllFixkostenEingaben_NewestFirst(t *testing.T) {
	db := openTestDB(t)
	mustCreateFixkostenEingabe(t, db, "2026-07-01", nil)
	newest := mustCreateFixkostenEingabe(t, db, "2026-09-01", nil)
	mustCreateFixkostenEingabe(t, db, "2026-08-01", nil)

	all, err := AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("AllFixkostenEingaben = %d, want 3", len(all))
	}
	if all[0].ID != newest || all[0].Monat != "2026-09-01" {
		t.Errorf("AllFixkostenEingaben[0] = %+v, want newest (2026-09-01)", all[0])
	}
}
