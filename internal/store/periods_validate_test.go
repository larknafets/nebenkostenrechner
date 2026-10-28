package store

import (
	"errors"
	"testing"
)

func TestPeriodInput_Validate(t *testing.T) {
	cases := []struct {
		name     string
		mod      func(in *PeriodInput)
		wantFeld Feldart
		wantID   int64
		wantWert string
		wantErr  bool
	}{
		{"gültig", func(in *PeriodInput) {}, 0, 0, "", false},
		{"Teilstand: Monat leer", func(in *PeriodInput) { in.Monat = "" }, 0, 0, "", false},
		{"Ablesedatum leer", func(in *PeriodInput) { in.ReadingDate = "" }, FeldReadingDate, 0, "", true},
		{"Ablesedatum falsches Format", func(in *PeriodInput) { in.ReadingDate = "01.07.2026" }, FeldReadingDate, 0, "01.07.2026", true},
		{"Ablesedatum nur Monat", func(in *PeriodInput) { in.ReadingDate = "2026-07" }, FeldReadingDate, 0, "2026-07", true},
		{"Ablesedatum 30. Februar", func(in *PeriodInput) { in.ReadingDate = "2026-02-30" }, FeldReadingDate, 0, "2026-02-30", true},
		{"Monat falsches Format", func(in *PeriodInput) { in.Monat = "2026-07" }, FeldMonat, 0, "2026-07", true},
		{"Monat Text", func(in *PeriodInput) { in.Monat = "abc" }, FeldMonat, 0, "abc", true},
		{"Prüfreihenfolge: Datum vor Monat", func(in *PeriodInput) {
			in.ReadingDate, in.Monat = "x", "y"
		}, FeldReadingDate, 0, "x", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := completePeriodInput("2026-07-01", baseReadings(nil))
			c.mod(&in)
			err := in.Validate()
			if !c.wantErr {
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
				t.Errorf("EingabeFehler = {Feld %d, ID %d, Wert %q}, want {%d, %d, %q}", ef.Feld, ef.ID, ef.Wert, c.wantFeld, c.wantID, c.wantWert)
			}
		})
	}
}

func TestPeriods_SchreiberLehnenUngueltigesAb(t *testing.T) {
	ungueltig := func(mod func(in *PeriodInput)) PeriodInput {
		in := completePeriodInput("2026-07-01", baseReadings(nil))
		mod(&in)
		return in
	}
	faelle := []struct {
		name string
		mod  func(in *PeriodInput)
		feld Feldart
	}{
		{"Ablesedatum", func(in *PeriodInput) { in.ReadingDate = "01.07.2026" }, FeldReadingDate},
		{"Abrechnungsmonat", func(in *PeriodInput) { in.Monat = "2026-07" }, FeldMonat},
	}
	for _, c := range faelle {
		t.Run("Anlegen "+c.name, func(t *testing.T) {
			db := openTestDB(t)
			_, err := CreatePeriod(db, ungueltig(c.mod))
			var ef *EingabeFehler
			if !errors.As(err, &ef) || ef.Feld != c.feld {
				t.Fatalf("CreatePeriod err = %v, want *EingabeFehler Feld %d", err, c.feld)
			}
			if all, _ := AllPeriods(db); len(all) != 0 {
				t.Errorf("AllPeriods = %d, want 0", len(all))
			}
		})
		t.Run("Ändern "+c.name, func(t *testing.T) {
			db := openTestDB(t)
			id := mustCreatePeriod(t, db, "2026-06-01", baseReadings(nil))
			err := UpdatePeriod(db, id, ungueltig(c.mod))
			var ef *EingabeFehler
			if !errors.As(err, &ef) || ef.Feld != c.feld {
				t.Fatalf("UpdatePeriod err = %v, want *EingabeFehler Feld %d", err, c.feld)
			}
			got, _ := GetPeriodDetails(db, id)
			if got.ReadingDate != "2026-06-01" {
				t.Errorf("ReadingDate = %q, want unchanged 2026-06-01", got.ReadingDate)
			}
		})
		t.Run("Import "+c.name, func(t *testing.T) {
			db := openTestDB(t)
			erste := completePeriodInput("2026-06-01", baseReadings(nil))
			_, err := ImportPeriods(db, []PeriodInput{erste, ungueltig(c.mod)})
			var ef *EingabeFehler
			var row *ImportRowError
			if !errors.As(err, &ef) || ef.Feld != c.feld || !errors.As(err, &row) {
				t.Fatalf("ImportPeriods err = %v, want *ImportRowError mit *EingabeFehler Feld %d", err, c.feld)
			}
			if row.Index != 1 {
				t.Errorf("Index = %d, want 1 (Position in der Liste)", row.Index)
			}
			if all, _ := AllPeriods(db); len(all) != 0 {
				t.Errorf("AllPeriods = %d, want 0 (alles oder nichts)", len(all))
			}
		})
	}
}

func TestImportPeriods_ReiheFehlerTraegtIndex(t *testing.T) {
	db := openTestDB(t)
	_, err := ImportPeriods(db, []PeriodInput{
		completePeriodInput("2026-06-01", baseReadings(nil)),
		completePeriodInput("2026-06-01", baseReadings(nil)),
	})
	var row *ImportRowError
	if !errors.As(err, &row) || row.Index != 1 {
		t.Fatalf("err = %v, want *ImportRowError mit Index 1", err)
	}
}
