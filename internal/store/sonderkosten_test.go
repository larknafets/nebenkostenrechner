package store

import "testing"

func TestSonderkosten_AnlegenLesenImDatenstand(t *testing.T) {
	db := openTestDB(t)
	spaet := Sonderkosten{Monat: "2026-09-01", Betreff: "Zaun", Art: ArtNeuanschaffung, Firma: "Zaunbau", Betrag: 900, Objekt: ObjektWohnung2, Umlegbar: true}
	frueh := Sonderkosten{
		Monat: "2026-03-01", Betreff: "Pumpe", Beschreibung: "Umwälzpumpe\ntauschen", Art: ArtReparatur,
		Firma: "Heizung GmbH", Rechnungsnummer: "R-17", Betrag: 412.5, Objekt: ObjektHaus, Umlegbar: true, Logik: LogikQM,
	}
	for _, s := range []Sonderkosten{spaet, frueh} {
		if _, err := CreateSonderkosten(db, s); err != nil {
			t.Fatalf("CreateSonderkosten: %v", err)
		}
	}

	ds, err := LadeDatenstand(db)
	if err != nil {
		t.Fatalf("LadeDatenstand: %v", err)
	}
	if len(ds.Sonderkosten) != 2 || ds.Sonderkosten[0].Betreff != "Pumpe" || ds.Sonderkosten[1].Betreff != "Zaun" {
		t.Fatalf("Sonderkosten = %+v, want zwei Einträge, ältester Monat zuerst", ds.Sonderkosten)
	}
	got := ds.Sonderkosten[0]
	frueh.ID = got.ID
	if got != frueh {
		t.Errorf("gelesen %+v, want %+v", got, frueh)
	}
}
