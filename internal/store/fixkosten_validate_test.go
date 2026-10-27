package store

import (
	"errors"
	"math"
	"testing"
)

func gueltigeFixkosten() FixkostenInput {
	return FixkostenInput{
		Monat:    "2026-06-01",
		Personen: map[int64]float64{1: 2, 2: 1},
		Werte: map[int64]FixkostenPositionWert{
			1: {Logik: LogikWohneinheit, Typ: TypJaehrlich, Wert: 100},
			2: {Logik: LogikQM, Typ: TypMonatlich, Wert: 10},
		},
	}
}

func TestFixkostenInputValidate(t *testing.T) {
	const gueltig = Feldart(-1)
	for _, c := range []struct {
		name     string
		mutate   func(in *FixkostenInput)
		wantFeld Feldart
		wantID   int64
		wantWert string
		wantErr  bool // Err (reason) set, Personenzahl only
	}{
		{name: "gueltig", mutate: func(in *FixkostenInput) {}, wantFeld: gueltig},
		{name: "ohne Werte und Personen", mutate: func(in *FixkostenInput) { in.Werte, in.Personen = nil, nil }, wantFeld: gueltig},
		{name: "Personenzahl null", mutate: func(in *FixkostenInput) { in.Personen[1] = 0 }, wantFeld: gueltig},
		{name: "Monat leer", mutate: func(in *FixkostenInput) { in.Monat = "" }, wantFeld: FeldMonat, wantWert: ""},
		{name: "Monat kein Datum", mutate: func(in *FixkostenInput) { in.Monat = "2026-06" }, wantFeld: FeldMonat, wantWert: "2026-06"},
		{name: "Logik unbekannt", mutate: func(in *FixkostenInput) {
			in.Werte[2] = FixkostenPositionWert{Logik: "x", Typ: TypMonatlich}
		}, wantFeld: FeldLogik, wantID: 2, wantWert: "x"},
		{name: "Logik leer", mutate: func(in *FixkostenInput) {
			in.Werte[1] = FixkostenPositionWert{Typ: TypMonatlich}
		}, wantFeld: FeldLogik, wantID: 1, wantWert: ""},
		{name: "Typ unbekannt", mutate: func(in *FixkostenInput) {
			in.Werte[1] = FixkostenPositionWert{Logik: LogikQM, Typ: "woechentlich"}
		}, wantFeld: FeldTyp, wantID: 1, wantWert: "woechentlich"},
		{name: "Personenzahl negativ", mutate: func(in *FixkostenInput) { in.Personen[2] = -1 }, wantFeld: FeldPersonen, wantID: 2, wantWert: "-1", wantErr: true},
		{name: "Personenzahl NaN", mutate: func(in *FixkostenInput) { in.Personen[1] = math.NaN() }, wantFeld: FeldPersonen, wantID: 1, wantWert: "NaN", wantErr: true},
		{name: "Personenzahl drei Nachkommastellen", mutate: func(in *FixkostenInput) { in.Personen[1] = 1.255 }, wantFeld: FeldPersonen, wantID: 1, wantWert: "1.255", wantErr: true},
		// Order: Monat, then Werte by id (Logik before Typ), then Personen by id.
		{name: "Reihenfolge Monat vor Werten", mutate: func(in *FixkostenInput) {
			in.Monat = "kaputt"
			in.Werte[1] = FixkostenPositionWert{Logik: "x", Typ: "y"}
			in.Personen[1] = -1
		}, wantFeld: FeldMonat, wantWert: "kaputt"},
		{name: "Reihenfolge niedrigere Position zuerst", mutate: func(in *FixkostenInput) {
			in.Werte[2] = FixkostenPositionWert{Logik: "x", Typ: TypMonatlich}
			in.Werte[1] = FixkostenPositionWert{Logik: LogikQM, Typ: "y"}
		}, wantFeld: FeldTyp, wantID: 1, wantWert: "y"},
		{name: "Reihenfolge Logik vor Typ", mutate: func(in *FixkostenInput) {
			in.Werte[1] = FixkostenPositionWert{Logik: "x", Typ: "y"}
		}, wantFeld: FeldLogik, wantID: 1, wantWert: "x"},
		{name: "Reihenfolge Werte vor Personen", mutate: func(in *FixkostenInput) {
			in.Werte[2] = FixkostenPositionWert{Logik: "x", Typ: TypMonatlich}
			in.Personen[1] = -1
		}, wantFeld: FeldLogik, wantID: 2, wantWert: "x"},
		{name: "Reihenfolge niedrigere Wohnung zuerst", mutate: func(in *FixkostenInput) {
			in.Personen[2] = -1
			in.Personen[1] = 1.001
		}, wantFeld: FeldPersonen, wantID: 1, wantWert: "1.001", wantErr: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := gueltigeFixkosten()
			c.mutate(&in)
			err := in.Validate()
			if c.wantFeld == gueltig {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			var ef *EingabeFehler
			if !errors.As(err, &ef) {
				t.Fatalf("Validate() = %v, want *EingabeFehler", err)
			}
			if ef.Feld != c.wantFeld || ef.ID != c.wantID || ef.Wert != c.wantWert {
				t.Errorf("Validate() = {Feld %d, ID %d, Wert %q}, want {Feld %d, ID %d, Wert %q}",
					ef.Feld, ef.ID, ef.Wert, c.wantFeld, c.wantID, c.wantWert)
			}
			if (ef.Err != nil) != c.wantErr {
				t.Errorf("Err = %v, want set: %v", ef.Err, c.wantErr)
			}
		})
	}
}

func TestFixkostenSchreiber_LehnenUngueltigesAb(t *testing.T) {
	db := openTestDB(t)
	schlecht := gueltigeFixkosten()
	schlecht.Werte[1] = FixkostenPositionWert{Logik: "x", Typ: TypMonatlich}

	var ef *EingabeFehler
	if _, err := CreateFixkostenEingabe(db, schlecht); !errors.As(err, &ef) || ef.Feld != FeldLogik {
		t.Errorf("Create: err = %v, want *EingabeFehler FeldLogik", err)
	}

	id, err := CreateFixkostenEingabe(db, gueltigeFixkosten())
	if err != nil {
		t.Fatalf("Create (gueltig): %v", err)
	}
	schlecht.Monat = "2026-07-01"
	if err := UpdateFixkostenEingabe(db, id, schlecht); !errors.As(err, &ef) || ef.Feld != FeldLogik {
		t.Errorf("Update: err = %v, want *EingabeFehler FeldLogik", err)
	}
	details, err := GetFixkostenEingabeDetails(db, id)
	if err != nil || details == nil || details.Monat != "2026-06-01" || details.Werte[1].Logik != LogikWohneinheit {
		t.Errorf("Eingabe nach abgelehntem Update = %+v, %v, want unveraendert", details, err)
	}

	// Import: the error names the position in the list, nothing is stored.
	db2 := openTestDB(t)
	zweite := gueltigeFixkosten()
	zweite.Monat = "2026-07-01"
	zweite.Personen[1] = -1
	_, err = ImportFixkostenEingaben(db2, []FixkostenInput{gueltigeFixkosten(), zweite})
	var ie *FixkostenImportError
	if !errors.As(err, &ie) || ie.Index != 1 {
		t.Fatalf("Import: err = %v, want *FixkostenImportError mit Index 1", err)
	}
	if !errors.As(err, &ef) || ef.Feld != FeldPersonen || ef.ID != 1 {
		t.Errorf("Import: err = %v, want *EingabeFehler FeldPersonen id 1", err)
	}
	if alle, _ := AllFixkostenEingaben(db2); len(alle) != 0 {
		t.Errorf("nach abgelehntem Import %d Eingaben, want 0", len(alle))
	}
}

// TestImportFixkostenEingaben_MonatBelegtNenntPosition: like ImportPeriods the
// import wraps a rule violation with the input's position, and errors.Is
// still finds the sentinel.
func TestImportFixkostenEingaben_MonatBelegtNenntPosition(t *testing.T) {
	db := openTestDB(t)
	_, err := ImportFixkostenEingaben(db, []FixkostenInput{gueltigeFixkosten(), gueltigeFixkosten()})
	var ie *FixkostenImportError
	if !errors.As(err, &ie) || ie.Index != 1 {
		t.Fatalf("err = %v, want *FixkostenImportError mit Index 1", err)
	}
	if !errors.Is(err, ErrFixkostenMonatBelegt) {
		t.Errorf("err = %v, want ErrFixkostenMonatBelegt", err)
	}
}
