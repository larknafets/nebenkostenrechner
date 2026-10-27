package web

import (
	"errors"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestInsertFixkostenRows_SicherheitsnetzNenntZeilennummer: a row that got
// past parseImportFixkostenRow but breaks a store rule is answered with the
// CSV line number, not with a 500.
func TestInsertFixkostenRows_SicherheitsnetzNenntZeilennummer(t *testing.T) {
	db := openTestDB(t)
	gut := store.FixkostenInput{Monat: "2026-06-01", Personen: map[int64]float64{1: 2, 2: 1}}
	schlecht := store.FixkostenInput{Monat: "2026-07-01", Personen: map[int64]float64{1: -1, 2: 1}}
	_, err := insertFixkostenRows(db, []fixkostenImportRow{{input: gut, line: 2}, {input: schlecht, line: 9}})
	var ie *importEingabeFehler
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v, want *importEingabeFehler", err)
	}
	if want := `Zeile 9: ungültige Personenzahl für Wohnung 1: "-1" (Personenzahl darf nicht negativ sein)`; ie.msg != want {
		t.Errorf("msg = %q, want %q", ie.msg, want)
	}
}
