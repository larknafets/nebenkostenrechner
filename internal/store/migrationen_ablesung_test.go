package store

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// migrationen_ablesung_test.go covers the destructive steps of #368 (spec
// #359, step 4): the six Preis/Häkchen columns of periods and the table
// period_occupancy are dropped at the end of the step list.

// ablesungsSpalten are the columns of periods that #368 drops.
var ablesungsSpalten = []string{"strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis", "wallbox_komplett_w1", "pv_gutschrift_w1"}

// mitAltenAblesungsFeldern gives a database the Ablesung fields of schema 4
// (the six columns of periods and the table period_occupancy), as they were
// before #368. Tests that need the old shape build it with this instead of
// schema.sql.
func mitAltenAblesungsFeldern(t *testing.T, db *sql.DB) {
	t.Helper()
	for _, stmt := range []string{
		`ALTER TABLE periods ADD COLUMN strompreis REAL`,
		`ALTER TABLE periods ADD COLUMN frischwasser_preis REAL`,
		`ALTER TABLE periods ADD COLUMN abwasser_preis REAL`,
		`ALTER TABLE periods ADD COLUMN einspeisung_preis REAL`,
		`ALTER TABLE periods ADD COLUMN wallbox_komplett_w1 INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE periods ADD COLUMN pv_gutschrift_w1 INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS period_occupancy (
			id           INTEGER PRIMARY KEY,
			period_id    INTEGER NOT NULL REFERENCES periods(id),
			apartment_id INTEGER NOT NULL REFERENCES apartments(id),
			personen     REAL NOT NULL,
			UNIQUE(period_id, apartment_id)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("alte Ablesungsfelder %q: %v", stmt, err)
		}
	}
}

// datenbankMitAblesungsFeldern is a fresh database in the shape of schema 4
// (see mitAltenAblesungsFeldern), for the tests of the steps that read the
// Ablesung fields.
func datenbankMitAblesungsFeldern(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	mitAltenAblesungsFeldern(t, db)
	return db
}

// datenbankImSchema4 writes a database as the last build before #368 left it:
// user_version 4, the six columns and period_occupancy with data, plus a
// Fixkosten-Eingabe that already carries the prices.
func datenbankImSchema4(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mitAltenAblesungsFeldern(t, db)
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))
	for _, stmt := range []string{
		`UPDATE periods SET strompreis = 0.3, frischwasser_preis = 1.5, abwasser_preis = 4.8, einspeisung_preis = 0.08, wallbox_komplett_w1 = 1`,
		`INSERT INTO period_occupancy (period_id, apartment_id, personen) VALUES ((SELECT id FROM periods), 1, 2)`,
		`INSERT INTO fixkosten_eingaben (monat, strompreis) VALUES ('2026-09-01', 0.3)`,
		`PRAGMA user_version = 4`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("Schema 4 %q: %v", stmt, err)
		}
	}
	db.Close()
	return path
}

func TestOpen_Schema4LoeschtSpaltenUndTabelleNachSicherung(t *testing.T) {
	path := datenbankImSchema4(t)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if v := userVersion(t, db); v != schemaVersion {
		t.Errorf("user_version = %d, want %d", v, schemaVersion)
	}
	for _, c := range ablesungsSpalten {
		if has, err := hasColumn(db, "periods", c); err != nil || has {
			t.Errorf("periods.%s vorhanden = %v, %v, want gelöscht", c, has, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'period_occupancy'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("Tabelle period_occupancy: %d, %v, want gelöscht", n, err)
	}
	// What is not dropped stays: the Ablesung and the prices of the Fixkosten-Eingabe.
	var perioden int
	if err := db.QueryRow(`SELECT COUNT(*) FROM periods`).Scan(&perioden); err != nil || perioden != 1 {
		t.Errorf("Ablesungen = %d, %v, want 1", perioden, err)
	}
	var strompreis float64
	if err := db.QueryRow(`SELECT strompreis FROM fixkosten_eingaben`).Scan(&strompreis); err != nil || strompreis != 0.3 {
		t.Errorf("Strompreis der Fixkosten-Eingabe = %v, %v, want 0.3", strompreis, err)
	}

	// The save from before the destructive steps holds the old fields.
	alle := sicherungsDateien(t, path)
	if len(alle) != 1 || !strings.Contains(alle[0], ".vor-schema-4-") {
		t.Fatalf("Sicherungen = %v, want genau eine mit der alten Version 4", alle)
	}
	kopie, err := sql.Open("sqlite", alle[0])
	if err != nil {
		t.Fatalf("open Sicherung: %v", err)
	}
	defer kopie.Close()
	var personen int
	if err := kopie.QueryRow(`SELECT COUNT(*) FROM period_occupancy`).Scan(&personen); err != nil || personen != 1 {
		t.Errorf("period_occupancy in der Sicherung = %d, %v, want 1", personen, err)
	}
	var preis float64
	if err := kopie.QueryRow(`SELECT strompreis FROM periods`).Scan(&preis); err != nil || preis != 0.3 {
		t.Errorf("strompreis in der Sicherung = %v, %v, want 0.3", preis, err)
	}
}

// TestOpen_PreiseGehenVorDemLoeschenInDieEingabe: a database that was never
// started with #361 (Eingabe without prices, the Ablesung still holds them)
// ends with the prices on the Eingabe: the backfill runs before the drop.
func TestOpen_PreiseGehenVorDemLoeschenInDieEingabe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mitAltenAblesungsFeldern(t, db)
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))
	for _, stmt := range []string{
		`UPDATE periods SET strompreis = 0.31, frischwasser_preis = 1.5, abwasser_preis = 4.8, einspeisung_preis = 0.09, pv_gutschrift_w1 = 1`,
		`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-09-01')`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%q: %v", stmt, err)
		}
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open (Migration): %v", err)
	}
	defer db.Close()
	var strom, einsp float64
	var pv int
	if err := db.QueryRow(`SELECT strompreis, einspeisung_preis, pv_gutschrift_w1 FROM fixkosten_eingaben`).Scan(&strom, &einsp, &pv); err != nil {
		t.Fatal(err)
	}
	if strom != 0.31 || einsp != 0.09 || pv != 1 {
		t.Errorf("Eingabe nach der Migration = %v / %v / %d, want 0.31 / 0.09 / 1 aus der Ablesung", strom, einsp, pv)
	}
	if alle := sicherungsDateien(t, path); len(alle) != 1 || !strings.Contains(alle[0], ".vor-schema-3-") {
		t.Errorf("Sicherungen = %v, want eine mit Version 3", alle)
	}
}

func TestOpen_Schema4WiederholungIstIdempotent(t *testing.T) {
	path := datenbankImSchema4(t)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	vorher := schemaVon(t, db)
	db.Close()

	for i := 0; i < 2; i++ {
		db, err = Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", i+2, err)
		}
		if got := schemaVon(t, db); !reflect.DeepEqual(got, vorher) {
			t.Errorf("Schema nach erneutem Start weicht ab:\n got %v\nwant %v", got, vorher)
		}
		// The steps themselves are idempotent, also when called directly.
		if err := migrate(db); err != nil {
			t.Errorf("migrate auf migrierter Datenbank: %v", err)
		}
		db.Close()
	}
	if got := sicherungsDateien(t, path); len(got) != 1 {
		t.Errorf("Sicherungen nach Wiederholung = %v, want weiterhin eine", got)
	}
}

func TestOpen_Schema4HatDasSchemaEinerFrischenDatenbank(t *testing.T) {
	neu := openTestDB(t)
	db, err := Open(datenbankImSchema4(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if got, want := schemaVon(t, db), schemaVon(t, neu); !reflect.DeepEqual(got, want) {
		t.Errorf("Schema nach der Migration weicht von einer frischen Datenbank ab:\n got %v\nwant %v", got, want)
	}
}

func TestOpen_FrischeDatenbankHatKeineAblesungsfelder(t *testing.T) {
	db := openTestDB(t)
	for _, c := range ablesungsSpalten {
		if has, err := hasColumn(db, "periods", c); err != nil || has {
			t.Errorf("frische Datenbank: periods.%s = %v, %v, want nicht vorhanden", c, has, err)
		}
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'period_occupancy'`).Scan(&n); err != nil || n != 0 {
		t.Errorf("frische Datenbank: period_occupancy %d, %v, want nicht vorhanden", n, err)
	}
	// Running every step again on the current schema does not fail.
	if err := migrate(db); err != nil {
		t.Errorf("migrate auf frischer Datenbank: %v", err)
	}
}

// TestMigrationen_ZerstoerendeSchritteStehenAmEnde pins the end of the list:
// everything that deletes data comes last, after the backfill that reads the
// columns.
func TestMigrationen_ZerstoerendeSchritteStehenAmEnde(t *testing.T) {
	var namen []string
	for _, m := range migrationen {
		namen = append(namen, m.name)
	}
	n := len(namen)
	if n < 3 || namen[n-2] != "dropPeriodsAblesungsFelder" || namen[n-1] != "dropPeriodOccupancyTable" {
		t.Fatalf("die letzten Schritte = %v, want dropPeriodsAblesungsFelder, dropPeriodOccupancyTable", namen[max(0, n-2):])
	}
	if namen[n-3] != "backfillFixkostenEingabenAusAblesungen" {
		t.Errorf("vor den zerstörenden Schritten steht %s, want backfillFixkostenEingabenAusAblesungen", namen[n-3])
	}
}
