package store

import (
	"errors"
	"testing"
)

func TestSonderkosten_AendernUndLoeschen(t *testing.T) {
	db := openTestDB(t)
	id, err := CreateSonderkosten(db, Sonderkosten{Monat: "2026-03-01", Betreff: "Pumpe", Art: ArtReparatur, Betrag: 100, Objekt: ObjektHaus, Umlegbar: true, Logik: LogikQM})
	if err != nil {
		t.Fatal(err)
	}
	neu := Sonderkosten{ID: id, Monat: "2026-04-01", Betreff: "Zaun", Beschreibung: "neu", Art: ArtNeuanschaffung, Firma: "F", Rechnungsnummer: "R1", Betrag: 55.5, Objekt: ObjektWohnung2}
	if err := UpdateSonderkosten(db, neu); err != nil {
		t.Fatalf("UpdateSonderkosten: %v", err)
	}
	alle, _ := AllSonderkosten(db)
	if len(alle) != 1 || alle[0] != neu {
		t.Fatalf("nach Update %+v, want %+v", alle, neu)
	}
	if err := UpdateSonderkosten(db, Sonderkosten{ID: id + 99, Monat: "2026-04-01", Betreff: "Zaun", Art: ArtNeuanschaffung, Betrag: 55.5, Objekt: ObjektWohnung2}); !errors.Is(err, ErrSonderkostenNotFound) {
		t.Errorf("Update unbekannt: %v, want ErrSonderkostenNotFound", err)
	}
	if err := DeleteSonderkosten(db, id); err != nil {
		t.Fatalf("DeleteSonderkosten: %v", err)
	}
	if alle, _ := AllSonderkosten(db); len(alle) != 0 {
		t.Errorf("nach Delete %+v", alle)
	}
	if err := DeleteSonderkosten(db, id); !errors.Is(err, ErrSonderkostenNotFound) {
		t.Errorf("Delete unbekannt: %v", err)
	}
}
