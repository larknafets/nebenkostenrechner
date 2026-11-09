package store

import (
	"errors"
	"testing"
)

// The import checks "table is empty" inside its own transaction: an entry that
// lands after the handler's check and before the insert is not doubled.
func TestImportSonderkosten_NurInLeereTabelle(t *testing.T) {
	db := openTestDB(t)
	e := Sonderkosten{Monat: "2026-09-01", Betreff: "Zaun", Art: ArtNeuanschaffung, Firma: "Zaunbau", Betrag: 900, Objekt: ObjektHaus}
	if _, err := CreateSonderkosten(db, e); err != nil {
		t.Fatalf("CreateSonderkosten: %v", err)
	}

	_, err := ImportSonderkosten(db, []Sonderkosten{e})
	var fehler *SonderkostenFehler
	if !errors.As(err, &fehler) || fehler.Error() != SonderkostenImportNurLeer {
		t.Fatalf("ImportSonderkosten = %v, want SonderkostenFehler %q", err, SonderkostenImportNurLeer)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sonderkosten`).Scan(&n); err != nil || n != 1 {
		t.Errorf("Einträge = %d, %v, want 1 (nichts doppelt)", n, err)
	}
}
