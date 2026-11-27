package storetest_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
	"github.com/larknafets/nebenkostenrechner/internal/store/storetest"
)

func count(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestOpener_MigrationLaeuftEinmalProTestBinary(t *testing.T) {
	var templates, copies int
	open := func(path string) (*sql.DB, error) {
		if filepath.Base(path) == "template.db" {
			templates++
		} else {
			copies++
		}
		return store.Open(path)
	}
	openFast := storetest.Opener(open)

	for i := 0; i < 3; i++ {
		openFast(t)
	}
	if templates != 1 || copies != 3 {
		t.Errorf("Vorlage %d-mal angelegt, %d Kopien geöffnet, erwartet 1 und 3", templates, copies)
	}
}

func TestOpener_JederAufrufBekommtEineUnabhaengigeDatenbank(t *testing.T) {
	openFast := storetest.Opener(store.Open)
	a, b := openFast(t), openFast(t)

	if _, err := a.Exec(`CREATE TABLE nur_in_a (x INTEGER)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	if n := count(t, a, `SELECT count(*) FROM sqlite_master WHERE name = 'nur_in_a'`); n != 1 {
		t.Fatalf("Tabelle fehlt in a")
	}
	if n := count(t, b, `SELECT count(*) FROM sqlite_master WHERE name = 'nur_in_a'`); n != 0 {
		t.Errorf("Änderung in a ist in b sichtbar")
	}
}

func TestOpener_KopieEnthaeltDieselbenStammdatenWieEinFrischesOpen(t *testing.T) {
	fresh, err := store.Open(filepath.Join(t.TempDir(), "frisch.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer fresh.Close()
	copyDB := storetest.Opener(store.Open)(t)

	for _, table := range []string{"apartments", "meters"} {
		q := `SELECT count(*) FROM ` + table
		if want, got := count(t, fresh, q), count(t, copyDB, q); want == 0 || want != got {
			t.Errorf("%s: frisch %d Zeilen, Kopie %d (erwartet gleich und nicht leer)", table, want, got)
		}
	}
}
