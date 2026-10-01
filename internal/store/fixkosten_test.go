package store

import (
	"database/sql"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	if len(kps) != 16 {
		t.Fatalf("Kostenpositionen = %d, want 16", len(kps))
	}

	wantKeys := []string{
		"grundsteuer", "gebaeudevers", "deich_grund", "deich_bau", "kreisverband",
		"abfall_haushalt", "abfall_personen", "abfall_biomuell", "abfall_restmuell",
		"strom_grundpreis", "trinkwasser", "abwasser", "internet", "wp_wartung",
		"streaming", "sonstige",
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

func countEingaben(t *testing.T, db *sql.DB, monat string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fixkosten_eingaben WHERE monat = ?`, monat).Scan(&n); err != nil {
		t.Fatalf("count eingaben %s: %v", monat, err)
	}
	return n
}

// TestFixkostenEingabe_EineJeMonat verifies Issue #161: a month has exactly
// one Fixkosten-Eingabe - creating a second one is rejected, nothing is
// written.
func TestFixkostenEingabe_EineJeMonat(t *testing.T) {
	db := openTestDB(t)
	mustCreateFixkostenEingabe(t, db, "2026-09-01", nil)

	_, err := CreateFixkostenEingabe(db, FixkostenInput{Monat: "2026-09-01", Personen: map[int64]int64{1: 2, 2: 1}})
	if !errors.Is(err, ErrFixkostenMonatBelegt) {
		t.Fatalf("second CreateFixkostenEingabe for 2026-09-01: err = %v, want ErrFixkostenMonatBelegt", err)
	}
	if n := countEingaben(t, db, "2026-09-01"); n != 1 {
		t.Errorf("eingaben for 2026-09-01 = %d, want 1", n)
	}

	// The next month is free.
	mustCreateFixkostenEingabe(t, db, "2026-10-01", nil)
}

func TestUpdateFixkostenEingabe_MonatBelegt(t *testing.T) {
	db := openTestDB(t)
	sep := mustCreateFixkostenEingabe(t, db, "2026-09-01", nil)
	okt := mustCreateFixkostenEingabe(t, db, "2026-10-01", nil)

	// Moving onto an occupied month is rejected and changes nothing.
	err := UpdateFixkostenEingabe(db, okt, FixkostenInput{Monat: "2026-09-01", Personen: map[int64]int64{1: 2, 2: 1}})
	if !errors.Is(err, ErrFixkostenMonatBelegt) {
		t.Fatalf("UpdateFixkostenEingabe onto an occupied month: err = %v, want ErrFixkostenMonatBelegt", err)
	}
	if n := countEingaben(t, db, "2026-10-01"); n != 1 {
		t.Errorf("eingaben for 2026-10-01 = %d after the rejected move, want 1", n)
	}

	// Saving an entry under its own month is fine (a plain correction).
	if err := UpdateFixkostenEingabe(db, sep, FixkostenInput{Monat: "2026-09-01", Personen: map[int64]int64{1: 3, 2: 1}}); err != nil {
		t.Errorf("UpdateFixkostenEingabe keeping its own month: %v", err)
	}
	// Moving to a free month is fine too.
	if err := UpdateFixkostenEingabe(db, okt, FixkostenInput{Monat: "2026-11-01", Personen: map[int64]int64{1: 2, 2: 1}}); err != nil {
		t.Errorf("UpdateFixkostenEingabe to a free month: %v", err)
	}
}

func TestImportFixkostenEingaben_DoppelterMonatRollback(t *testing.T) {
	db := openTestDB(t)
	in := func(monat string) FixkostenInput {
		return FixkostenInput{Monat: monat, Personen: map[int64]int64{1: 2, 2: 1}}
	}

	_, err := ImportFixkostenEingaben(db, []FixkostenInput{in("2026-09-01"), in("2026-10-01"), in("2026-09-01")})
	if !errors.Is(err, ErrFixkostenMonatBelegt) {
		t.Fatalf("ImportFixkostenEingaben with a duplicate month: err = %v, want ErrFixkostenMonatBelegt", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fixkosten_eingaben`).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 0 {
		t.Errorf("eingaben after the failed import = %d, want 0 (all or nothing)", n)
	}
}

// indexExists reports whether the UNIQUE index on fixkosten_eingaben.monat
// is there.
func indexExists(t *testing.T, db *sql.DB) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_fixkosten_eingaben_monat'`).Scan(&n); err != nil {
		t.Fatalf("check index: %v", err)
	}
	return n > 0
}

// TestEnsureFixkostenEingabenMonatUnique verifies the migration (Issue
// #161): with duplicates it warns naming exactly the affected months, keeps
// every entry and skips the index; once they are cleaned up it creates the
// UNIQUE index (idempotent), which then also blocks a raw duplicate insert
// as a backstop.
func TestEnsureFixkostenEingabenMonatUnique(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`DROP INDEX idx_fixkosten_eingaben_monat`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-03-01'), ('2026-03-01'), ('2026-04-01'), ('2026-05-01'), ('2026-05-01')`); err != nil {
		t.Fatalf("insert duplicates: %v", err)
	}

	var logged strings.Builder
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if err := ensureFixkostenEingabenMonatUnique(db); err != nil {
		t.Fatalf("ensureFixkostenEingabenMonatUnique with duplicates: %v, want it to warn and carry on", err)
	}
	for _, want := range []string{"2026-03-01", "2026-05-01"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("warning %q does not name the month %s", logged.String(), want)
		}
	}
	if strings.Contains(logged.String(), "2026-04-01") {
		t.Errorf("warning %q names 2026-04-01, which has no duplicate", logged.String())
	}
	if indexExists(t, db) {
		t.Error("index created although duplicates exist")
	}
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fixkosten_eingaben`).Scan(&total); err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 5 {
		t.Errorf("eingaben = %d after the migration, want all 5 untouched", total)
	}

	// Clean up the duplicates, then the migration creates the index - twice.
	if _, err := db.Exec(`DELETE FROM fixkosten_eingaben WHERE id IN (SELECT MAX(id) FROM fixkosten_eingaben GROUP BY monat HAVING COUNT(*) > 1)`); err != nil {
		t.Fatalf("remove duplicates: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := ensureFixkostenEingabenMonatUnique(db); err != nil {
			t.Fatalf("ensureFixkostenEingabenMonatUnique call %d: %v", i+1, err)
		}
	}
	if !indexExists(t, db) {
		t.Fatal("index missing after the duplicates are gone")
	}
	if _, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-03-01')`); err == nil {
		t.Error("raw duplicate insert succeeded, want the UNIQUE index to reject it")
	}
}

// TestOpen_MitDoppeltenFixkostenMonaten verifies the whole start: a
// database with duplicate months still starts (so the user can fix them in
// the UI), new duplicates are refused, a duplicate can still be corrected
// under its own month and deleted, and the next start creates the index.
func TestOpen_MitDoppeltenFixkostenMonaten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dup.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := db.Exec(`DROP INDEX idx_fixkosten_eingaben_monat`); err != nil {
		t.Fatalf("drop index: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-06-01'), ('2026-06-01')`); err != nil {
		t.Fatalf("insert duplicates: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open on a database with duplicate months: %v, want it to start", err)
	}
	t.Cleanup(func() { db.Close() })
	if indexExists(t, db) {
		t.Fatal("index created although duplicates exist")
	}

	// A third entry for the month is refused.
	if _, err := CreateFixkostenEingabe(db, FixkostenInput{Monat: "2026-06-01", Personen: map[int64]int64{1: 2, 2: 1}}); !errors.Is(err, ErrFixkostenMonatBelegt) {
		t.Errorf("CreateFixkostenEingabe on a duplicated month: err = %v, want ErrFixkostenMonatBelegt", err)
	}

	// A duplicate can be corrected under its own month ...
	eingaben, err := AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if err := UpdateFixkostenEingabe(db, eingaben[0].ID, FixkostenInput{Monat: "2026-06-01", Personen: map[int64]int64{1: 3, 2: 1}}); err != nil {
		t.Errorf("UpdateFixkostenEingabe keeping the month of a duplicate: %v", err)
	}
	// ... and fixed by deleting it.
	if err := DeleteFixkostenEingabe(db, eingaben[1].ID); err != nil {
		t.Fatalf("DeleteFixkostenEingabe: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if !indexExists(t, db) {
		t.Error("index missing after a restart without duplicates")
	}
}

// TestMonatUniqueErr verifies the safety net for a write that gets past the
// preflight check (PR #173 review): a real violation of the UNIQUE index is
// mapped to ErrFixkostenMonatBelegt, every other error passes through.
func TestMonatUniqueErr(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-03-01')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	_, rawErr := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-03-01')`)
	if rawErr == nil {
		t.Fatal("raw duplicate insert succeeded, want the UNIQUE index to reject it")
	}

	mapped := monatUniqueErr(rawErr, "2026-03-01")
	if !errors.Is(mapped, ErrFixkostenMonatBelegt) {
		t.Errorf("monatUniqueErr(%v) = %v, want ErrFixkostenMonatBelegt", rawErr, mapped)
	}

	other := errors.New("database is locked")
	if got := monatUniqueErr(other, "2026-03-01"); got != other {
		t.Errorf("monatUniqueErr(other error) = %v, want it returned unchanged", got)
	}
	if got := monatUniqueErr(nil, "2026-03-01"); got != nil {
		t.Errorf("monatUniqueErr(nil) = %v, want nil", got)
	}
}
