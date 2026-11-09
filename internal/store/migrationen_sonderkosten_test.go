package store

import (
	"path/filepath"
	"strings"
	"testing"
)

// datenbankImSchema6 writes a database as the build before #453 left it:
// user_version 6 and sonderkosten with the column umsetzung and data.
func datenbankImSchema6(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nk.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	for _, stmt := range []string{
		`DROP TABLE sonderkosten`,
		`CREATE TABLE sonderkosten (
			id INTEGER PRIMARY KEY, monat TEXT NOT NULL, betreff TEXT NOT NULL,
			beschreibung TEXT NOT NULL DEFAULT '', art TEXT NOT NULL, firma TEXT NOT NULL,
			rechnungsnummer TEXT NOT NULL DEFAULT '', betrag REAL NOT NULL CHECK (betrag > 0),
			umsetzung TEXT NOT NULL CHECK (umsetzung IN ('haus', 'wohnung1', 'wohnung2')),
			umlegbar INTEGER NOT NULL CHECK (umlegbar IN (0, 1)), logik TEXT NOT NULL DEFAULT ''
		)`,
		`INSERT INTO sonderkosten (monat, betreff, art, firma, betrag, umsetzung, umlegbar, logik) VALUES
			('2026-10-01', 'Pumpe', 'reparatur', 'Meier', 100, 'haus', 1, 'qm'),
			('2026-09-01', 'Spüle', 'neuanschaffung', 'Schmitt', 40, 'wohnung2', 0, '')`,
		`PRAGMA user_version = 6`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("Schema 6 %q: %v", stmt, err)
		}
	}
	db.Close()
	return path
}

func TestOpen_Schema6BenenntUmsetzungInObjektUm(t *testing.T) {
	path := datenbankImSchema6(t)

	for lauf := 1; lauf <= 2; lauf++ { // the second start changes nothing
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", lauf, err)
		}
		if v := userVersion(t, db); v != schemaVersion || schemaVersion < 7 {
			t.Errorf("Lauf %d: user_version = %d, want %d (>= 7)", lauf, v, schemaVersion)
		}
		if has, err := hasColumn(db, "sonderkosten", "umsetzung"); err != nil || has {
			t.Errorf("Lauf %d: umsetzung vorhanden = %v, %v, want umbenannt", lauf, has, err)
		}
		alle, err := AllSonderkosten(db)
		if err != nil || len(alle) != 2 {
			t.Fatalf("Lauf %d: %d Einträge, %v, want 2", lauf, len(alle), err)
		}
		if alle[0].Betreff != "Spüle" || alle[0].Objekt != ObjektWohnung2 || alle[1].Betreff != "Pumpe" || alle[1].Objekt != ObjektHaus {
			t.Errorf("Lauf %d: Einträge = %+v", lauf, alle)
		}
		// The CHECK constraint came along with the column.
		if _, err := db.Exec(`INSERT INTO sonderkosten (monat, betreff, art, firma, betrag, objekt, umlegbar) VALUES ('2026-10-01', 'X', 'reparatur', '', 1, 'garage', 0)`); err == nil {
			t.Errorf("Lauf %d: objekt 'garage' wurde angenommen, want CHECK-Fehler", lauf)
		}
		db.Close()
	}

	alle := sicherungsDateien(t, path)
	if len(alle) != 1 || !strings.Contains(alle[0], ".vor-schema-6-") {
		t.Errorf("Sicherungen = %v, want genau eine mit der alten Version 6", alle)
	}
}

func TestOpen_FrischeDatenbankHatObjektSpalte(t *testing.T) {
	db := openTestDB(t)
	for spalte, want := range map[string]bool{"objekt": true, "umsetzung": false} {
		if has, err := hasColumn(db, "sonderkosten", spalte); err != nil || has != want {
			t.Errorf("sonderkosten.%s vorhanden = %v, %v, want %v", spalte, has, err, want)
		}
	}
}

// A second process that opens the same version 6 database at the same time may
// find the column already renamed after its own check: that is no failed start.
func TestMigration_UmbenennenToleriertDenParallelenStart(t *testing.T) {
	db := openTestDB(t) // has objekt, like the database after the other process's rename
	if _, err := db.Exec(`ALTER TABLE sonderkosten ADD COLUMN umsetzung TEXT`); err != nil {
		t.Fatal(err) // makes this process still see the old column, so its RENAME fails
	}
	if err := renameSonderkostenUmsetzungToObjekt(db); err != nil {
		t.Errorf("renameSonderkostenUmsetzungToObjekt = %v, want nil: objekt exists, the other process was first", err)
	}
}

func TestOpen_Schema7LegtRechnungsdatumAn(t *testing.T) {
	path := datenbankImSchema6(t)
	db, err := Open(path) // 6 -> 8 in one start
	if err != nil {
		t.Fatal(err)
	}
	// Back to the state of version 7: objekt exists, rechnungsdatum does not.
	for _, stmt := range []string{`ALTER TABLE sonderkosten DROP COLUMN rechnungsdatum`, `PRAGMA user_version = 7`} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	db.Close()

	for lauf := 1; lauf <= 2; lauf++ { // the second start changes nothing
		db, err := Open(path)
		if err != nil {
			t.Fatalf("Open %d: %v", lauf, err)
		}
		if v := userVersion(t, db); v != schemaVersion || schemaVersion != 8 {
			t.Errorf("Lauf %d: user_version = %d, want %d (8)", lauf, v, schemaVersion)
		}
		alle, err := AllSonderkosten(db)
		if err != nil || len(alle) != 2 {
			t.Fatalf("Lauf %d: %d Einträge, %v, want 2", lauf, len(alle), err)
		}
		for _, s := range alle {
			if s.Rechnungsdatum != "" {
				t.Errorf("Lauf %d: %s hat Rechnungsdatum %q, want leer", lauf, s.Betreff, s.Rechnungsdatum)
			}
		}
		db.Close()
	}
}

func TestOpen_FrischeDatenbankHatRechnungsdatumSpalte(t *testing.T) {
	db := openTestDB(t)
	if has, err := hasColumn(db, "sonderkosten", "rechnungsdatum"); err != nil || !has {
		t.Errorf("sonderkosten.rechnungsdatum vorhanden = %v, %v, want true", has, err)
	}
}
