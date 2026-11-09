package store

import (
	"errors"
	"sync"
	"testing"
)

// The import checks "table is empty" inside its own transaction: an entry that
// lands after the handler's check and before the insert is not doubled, and the
// entry that is already there stays as it was.
func TestImportSonderkosten_NurInLeereTabelle(t *testing.T) {
	db := openTestDB(t)
	vorhanden := Sonderkosten{Monat: "2026-09-01", Betreff: "Zaun", Art: ArtNeuanschaffung, Firma: "Zaunbau", Betrag: 900, Objekt: ObjektHaus}
	id, err := CreateSonderkosten(db, vorhanden)
	if err != nil {
		t.Fatalf("CreateSonderkosten: %v", err)
	}
	neu := Sonderkosten{Monat: "2026-10-01", Betreff: "Pumpe", Art: ArtReparatur, Firma: "Meier", Betrag: 100, Objekt: ObjektWohnung1}

	_, err = ImportSonderkosten(db, []Sonderkosten{neu})
	var fehler *SonderkostenFehler
	if !errors.As(err, &fehler) || fehler.Error() != SonderkostenImportNurLeer {
		t.Fatalf("ImportSonderkosten = %v, want SonderkostenFehler %q", err, SonderkostenImportNurLeer)
	}
	alle, err := AllSonderkosten(db)
	if err != nil || len(alle) != 1 || alle[0].ID != id || alle[0].Betreff != "Zaun" || alle[0].Betrag != 900 {
		t.Errorf("Einträge = %+v, %v, want nur der vorhandene Eintrag (Zaun, 900), unverändert", alle, err)
	}
}

// Two imports at once: one takes over its entries, the other gets the
// SonderkostenFehler and not a database error.
func TestImportSonderkosten_ZweiGleichzeitigeImporte(t *testing.T) {
	db := openTestDB(t)
	eintrag := Sonderkosten{Monat: "2026-09-01", Betreff: "Zaun", Art: ArtNeuanschaffung, Firma: "Zaunbau", Betrag: 900, Objekt: ObjektHaus}

	var wg sync.WaitGroup
	fehler := make([]error, 2)
	for i := range fehler {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, fehler[i] = ImportSonderkosten(db, []Sonderkosten{eintrag})
		}()
	}
	wg.Wait()

	ok, abgelehnt := 0, 0
	for _, err := range fehler {
		var sf *SonderkostenFehler
		switch {
		case err == nil:
			ok++
		case errors.As(err, &sf) && sf.Error() == SonderkostenImportNurLeer:
			abgelehnt++
		default:
			t.Errorf("Import-Fehler = %v, want nil oder SonderkostenImportNurLeer", err)
		}
	}
	alle, err := AllSonderkosten(db)
	if ok != 1 || abgelehnt != 1 || err != nil || len(alle) != 1 {
		t.Errorf("erfolgreich %d, abgelehnt %d, Einträge %d (%v), want 1, 1, 1", ok, abgelehnt, len(alle), err)
	}
}
