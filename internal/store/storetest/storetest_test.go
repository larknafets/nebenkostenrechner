package storetest_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
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

	// Werte vergleichen, nicht nur Zeilen zählen: eine Vorlage mit gleich
	// vielen, aber anderen Stammdaten fiele sonst nicht auf.
	for _, table := range []string{"apartments", "meters", "kostenpositionen", "haus"} {
		want, got := dump(t, fresh, table), dump(t, copyDB, table)
		if want == "" || want != got {
			t.Errorf("%s: frisch\n%sKopie\n%s(erwartet gleich und nicht leer)", table, want, got)
		}
	}
}

// dump gibt alle Zeilen einer Tabelle als Text zurück, nach der ersten Spalte sortiert.
func dump(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	rows, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY 1`)
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	var b strings.Builder
	for rows.Next() {
		values := make([]any, len(cols))
		targets := make([]any, len(cols))
		for i := range values {
			targets[i] = &values[i]
		}
		if err := rows.Scan(targets...); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		fmt.Fprintln(&b, values...)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("%s: %v", table, err)
	}
	return b.String()
}
