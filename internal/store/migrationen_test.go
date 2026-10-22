package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestMigrationen_Reihenfolge keeps the order constraints between the steps
// out of comments only.
func TestMigrationen_Reihenfolge(t *testing.T) {
	index := map[string]int{}
	for i, m := range migrationen {
		if _, doppelt := index[m.name]; doppelt {
			t.Fatalf("Schritt %s kommt doppelt vor", m.name)
		}
		index[m.name] = i
	}
	for _, c := range []struct{ vorher, nachher, grund string }{
		{"ensurePeriodsNullablePriceColumns", "ensurePeriodSwitchColumns", "der Umbau von periods würde die Schalter-Spalten wieder entfernen"},
		{"seed", "migrateHeizungGewichtungToHaus", "die Gewichtung wird in die haus-Zeile geschrieben"},
		{"ensureFixkostenWerteLogikTypColumns", "dropKostenpositionenJahreTable", "ADR 0001: die Tabelle fällt erst weg, wenn ihre Daten umgezogen sind"},
	} {
		v, okV := index[c.vorher]
		n, okN := index[c.nachher]
		if !okV || !okN {
			t.Fatalf("Schritt %s oder %s fehlt in migrationen", c.vorher, c.nachher)
		}
		if v >= n {
			t.Errorf("%s muss vor %s laufen: %s", c.vorher, c.nachher, c.grund)
		}
	}
}

func userVersion(t *testing.T, db *sql.DB) int {
	t.Helper()
	v, err := schemaVersionOf(db)
	if err != nil {
		t.Fatalf("schemaVersionOf: %v", err)
	}
	return v
}

func sicherungsDateien(t *testing.T, path string) []string {
	t.Helper()
	alle, err := filepath.Glob(path + ".vor-schema-*")
	if err != nil {
		t.Fatalf("Glob: %v", err)
	}
	return alle
}

// bestehendeDatenbank creates a database like one from before this release:
// full schema and one Ablesung, user_version 0.
func bestehendeDatenbank(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))
	if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	db.Close()
	return path
}

func TestOpen_NeueDatenbankOhneSicherung(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if got := sicherungsDateien(t, path); len(got) != 0 {
		t.Errorf("Sicherungen einer neuen Datenbank = %v, want keine", got)
	}
	if v := userVersion(t, db); v != schemaVersion {
		t.Errorf("user_version = %d, want %d", v, schemaVersion)
	}
}

func TestOpen_SichertBestehendeDatenbankEinmal(t *testing.T) {
	path := bestehendeDatenbank(t)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if v := userVersion(t, db); v != schemaVersion {
		t.Errorf("user_version = %d, want %d", v, schemaVersion)
	}
	db.Close()

	alle := sicherungsDateien(t, path)
	if len(alle) != 1 || !strings.Contains(alle[0], ".vor-schema-0-") {
		t.Fatalf("Sicherungen = %v, want genau eine mit der alten Version 0", alle)
	}
	// The save holds the old data.
	kopie, err := sql.Open("sqlite", alle[0])
	if err != nil {
		t.Fatalf("open Sicherung: %v", err)
	}
	defer kopie.Close()
	var n int
	if err := kopie.QueryRow(`SELECT COUNT(*) FROM periods`).Scan(&n); err != nil || n != 1 {
		t.Errorf("Ablesungen in der Sicherung = %d, %v, want 1", n, err)
	}

	// A second start on the current version saves nothing more.
	db, err = Open(path)
	if err != nil {
		t.Fatalf("zweites Open: %v", err)
	}
	db.Close()
	if got := sicherungsDateien(t, path); len(got) != 1 {
		t.Errorf("Sicherungen nach zweitem Start = %v, want weiterhin eine", got)
	}
}

func TestOpen_DemoDatenbankOhneSicherung(t *testing.T) {
	path := bestehendeDatenbank(t)
	db, err := OpenDemo(path)
	if err != nil {
		t.Fatalf("OpenDemo: %v", err)
	}
	db.Close()
	if got := sicherungsDateien(t, path); len(got) != 0 {
		t.Errorf("Sicherungen der Demo-Datenbank = %v, want keine", got)
	}
}

func TestOpen_BehaeltNurDieLetztenDreiSicherungen(t *testing.T) {
	path := bestehendeDatenbank(t)
	t.Cleanup(func() { jetzt = time.Now })

	start := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		jetzt = func() time.Time { return start.Add(time.Duration(i) * time.Minute) }
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		// Back to the old version: the next start has to save again.
		if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
			t.Fatalf("set user_version: %v", err)
		}
		db.Close()
	}

	alle := sicherungsDateien(t, path)
	if len(alle) != sicherungenBehalten {
		t.Fatalf("Sicherungen = %v, want %d", alle, sicherungenBehalten)
	}
	var zeiten []string
	for _, a := range alle {
		zeiten = append(zeiten, a[strings.LastIndex(a, "-")+1:])
	}
	want := []string{"20261022T100200.000Z", "20261022T100300.000Z", "20261022T100400.000Z"}
	if !reflect.DeepEqual(zeiten, want) {
		t.Errorf("verbliebene Sicherungen = %v, want die letzten drei %v", zeiten, want)
	}
}

// altesSchema writes a database from before the Abrechnungsmonat (#86) and
// the Heizungs-Gewichtung in haus (#162): the schema steps have work to do.
func altesSchema(t *testing.T, path string) {
	t.Helper()
	alt, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer alt.Close()
	for _, stmt := range []string{
		`CREATE TABLE periods (
			id                        INTEGER PRIMARY KEY,
			reading_date              TEXT NOT NULL,
			strompreis                REAL NOT NULL,
			frischwasser_preis        REAL NOT NULL,
			abwasser_preis            REAL NOT NULL,
			heizung_waerme_gewichtung REAL NOT NULL DEFAULT 0.7,
			einspeisung_preis         REAL NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE haus (id INTEGER PRIMARY KEY CHECK (id = 1), strom_weiterberechnen INTEGER NOT NULL DEFAULT 1)`,
		`INSERT INTO periods (reading_date, strompreis, frischwasser_preis, abwasser_preis, heizung_waerme_gewichtung) VALUES ('2026-08-31', 0.22, 1.46, 4.87, 0.6)`,
	} {
		if _, err := alt.Exec(stmt); err != nil {
			t.Fatalf("old schema %q: %v", stmt, err)
		}
	}
}

// schemaVon lists every table with its columns and every index, to compare
// two databases.
func schemaVon(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var out []string
	rows, err := db.Query(`SELECT m.name || '.' || p.name || ' ' || p.type || ' notnull=' || p."notnull" || ' dflt=' || COALESCE(p.dflt_value, '') || ' pk=' || p.pk
		FROM sqlite_master m, pragma_table_info(m.name) p WHERE m.type = 'table' AND m.name NOT LIKE 'sqlite_%' ORDER BY m.name, p.name`)
	if err != nil {
		t.Fatalf("columns: %v", err)
	}
	for rows.Next() {
		var spalte string
		if err := rows.Scan(&spalte); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, spalte)
	}
	rows.Close()
	rows, err = db.Query(`SELECT 'index ' || name FROM sqlite_master WHERE type = 'index' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("indexes: %v", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out = append(out, name)
	}
	rows.Close()
	return out
}

func TestOpen_BrichtOhneSicherungAb(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	altesSchema(t, path)
	t.Cleanup(func() { jetzt = time.Now })
	fest := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	jetzt = func() time.Time { return fest }
	// The target exists already, so VACUUM INTO fails.
	belegt := path + ".vor-schema-0-" + fest.Format("20060102T150405.000Z")
	if err := os.WriteFile(belegt, []byte("belegt"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if db, err := Open(path); err == nil {
		db.Close()
		t.Fatal("Open: want error when the save fails")
	} else if !strings.Contains(err.Error(), "save before migration failed") {
		t.Errorf("Open err = %v, want the failed save named", err)
	}
	if b, _ := os.ReadFile(belegt); string(b) != "belegt" {
		t.Errorf("the existing file was removed or changed: %q", b)
	}

	// Unchanged: version 0, no schema step and no schema.sql ran.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()
	if v := userVersion(t, db); v != 0 {
		t.Errorf("user_version = %d, want 0", v)
	}
	var tabellen int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tabellen); err != nil || tabellen != 2 {
		t.Errorf("Tabellen = %d, %v, want 2 (periods, haus), schema.sql must not have run", tabellen, err)
	}
	if ok, err := hasColumn(db, "periods", "monat"); err != nil || ok {
		t.Errorf("periods hat monat = %v, %v, want false (kein Schritt gelaufen)", ok, err)
	}
}

// TestOpen_AltesSchemaWirdZumAktuellen opens an old database and expects
// the same tables, columns and indexes as a new one, with the old data kept.
func TestOpen_AltesSchemaWirdZumAktuellen(t *testing.T) {
	neu := openTestDB(t)

	path := filepath.Join(t.TempDir(), "alt.db")
	altesSchema(t, path)

	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open (altes Schema): %v", err)
	}
	defer db.Close()

	if got, want := schemaVon(t, db), schemaVon(t, neu); !reflect.DeepEqual(got, want) {
		t.Errorf("Schema nach der Migration weicht von einer neuen Datenbank ab:\n got %v\nwant %v", got, want)
	}
	var monat string
	if err := db.QueryRow(`SELECT monat FROM periods`).Scan(&monat); err != nil || monat != "2026-08-01" {
		t.Errorf("Bestandszeile: monat = %q, %v, want 2026-08-01", monat, err)
	}
	var gewichtung float64
	if err := db.QueryRow(`SELECT heizung_waerme_gewichtung FROM haus WHERE id = 1`).Scan(&gewichtung); err != nil || gewichtung != 0.6 {
		t.Errorf("Gewichtung in haus = %v, %v, want 0.6 (aus der jüngsten Ablesung)", gewichtung, err)
	}
	if got := sicherungsDateien(t, path); len(got) != 1 {
		t.Errorf("Sicherungen = %v, want eine vor der Migration", got)
	}
}

// TestOpen_FehlgeschlagenerStartBehaeltDieErsteSicherung: a failing step must
// not rotate the clean save out by writing a copy of the half-migrated state
// on every retry.
func TestOpen_FehlgeschlagenerStartBehaeltDieErsteSicherung(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	altesSchema(t, path)
	t.Cleanup(func() { jetzt = time.Now })
	vorher := migrationen
	t.Cleanup(func() { migrationen = vorher })
	migrationen = append(append([]migration{}, vorher...), migration{"kaputt", func(*sql.DB) error { return os.ErrInvalid }})

	start := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		jetzt = func() time.Time { return start.Add(time.Duration(i) * time.Minute) }
		if db, err := Open(path); err == nil {
			db.Close()
			t.Fatalf("Open %d: want error from the failing step", i)
		} else if !strings.Contains(err.Error(), "migrate kaputt") {
			t.Fatalf("Open %d err = %v, want the failing step named", i, err)
		}
	}

	alle := sicherungsDateien(t, path)
	if len(alle) != 1 || !strings.HasSuffix(alle[0], "20261022T100000.000Z") {
		t.Errorf("Sicherungen = %v, want nur die erste (vor dem ersten Versuch)", alle)
	}
}

func TestOpen_NeuereDatenbankWirdGesichertUndBehaeltIhreVersion(t *testing.T) {
	path := bestehendeDatenbank(t)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion+5)); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	if v := userVersion(t, db); v != schemaVersion+5 {
		t.Errorf("user_version = %d, want %d (never lowered)", v, schemaVersion+5)
	}
	alle := sicherungsDateien(t, path)
	if len(alle) != 1 || !strings.Contains(alle[0], fmt.Sprintf(".vor-schema-%d-", schemaVersion+5)) {
		t.Errorf("Sicherungen = %v, want eine mit der neueren Version", alle)
	}
}

// TestOpen_SonderzeichenImPfadUndFremdeDateien: glob characters in the path
// and files that only look like saves must neither be counted nor removed.
func TestOpen_SonderzeichenImPfadUndFremdeDateien(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nk[1]")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, "nk[1].db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))
	if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
	db.Close()

	fremd := []string{path + ".vor-schema-0-20260101T000000.000Z-wal", path + ".vor-schema-x"}
	for _, f := range fremd {
		if err := os.WriteFile(f, []byte("fremd"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	t.Cleanup(func() { jetzt = time.Now })
	start := time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		jetzt = func() time.Time { return start.Add(time.Duration(i) * time.Minute) }
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", i, err)
		}
		if _, err := db.Exec("PRAGMA user_version = 0"); err != nil {
			t.Fatalf("set user_version: %v", err)
		}
		db.Close()
	}

	echte, err := sicherungen(path)
	if err != nil || len(echte) != sicherungenBehalten {
		t.Errorf("echte Sicherungen = %v, %v, want %d", echte, err, sicherungenBehalten)
	}
	for _, f := range fremd {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("fremde Datei %s wurde entfernt: %v", f, err)
		}
	}
}

func TestOpen_SicherungHatDieRechteDerDatenbank(t *testing.T) {
	path := bestehendeDatenbank(t)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	db.Close()

	alle := sicherungsDateien(t, path)
	if len(alle) != 1 {
		t.Fatalf("Sicherungen = %v, want eine", alle)
	}
	fi, err := os.Stat(alle[0])
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("Rechte der Sicherung = %v, want 0600 wie die Datenbank", fi.Mode().Perm())
	}
}
