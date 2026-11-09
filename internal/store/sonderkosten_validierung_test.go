package store

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func gueltigeSonderkosten() Sonderkosten {
	return Sonderkosten{
		Monat: "2026-10-01", Betreff: "Pumpe", Art: ArtReparatur, Betrag: 100,
		Objekt: ObjektHaus, Umlegbar: true, Logik: LogikWohneinheit,
	}
}

func TestValidiereSonderkosten(t *testing.T) {
	for _, c := range []struct {
		name         string
		aendere      func(*Sonderkosten)
		hatFixkosten bool
		want         string // part of the one expected message, "" = valid
	}{
		{"gültig", func(*Sonderkosten) {}, false, ""},
		{"Rechnungsdatum gültig", func(s *Sonderkosten) { s.Rechnungsdatum = "2026-03-12" }, false, ""},
		{"Rechnungsdatum nicht im Abrechnungsmonat", func(s *Sonderkosten) { s.Rechnungsdatum = "2025-12-31" }, false, ""},
		{"Rechnungsdatum kein Datum", func(s *Sonderkosten) { s.Rechnungsdatum = "2026-02-30" }, false, "Rechnungsdatum"},
		{"Rechnungsdatum falsches Format", func(s *Sonderkosten) { s.Rechnungsdatum = "12.03.2026" }, false, "Rechnungsdatum"},
		{"Betrag 0", func(s *Sonderkosten) { s.Betrag = 0 }, false, "Betrag"},
		{"Betrag negativ", func(s *Sonderkosten) { s.Betrag = -5 }, false, "Betrag"},
		{"Betrag unendlich", func(s *Sonderkosten) { s.Betrag = math.Inf(1) }, false, "Betrag"},
		{"Betrag mit Bruchteil eines Cents", func(s *Sonderkosten) { s.Betrag = 10.005 }, false, "Betrag"},
		{"Betrag in ganzen Cent", func(s *Sonderkosten) { s.Betrag = 1234.5 }, false, ""},
		{"Betreff leer", func(s *Sonderkosten) { s.Betreff = " " }, false, "Betreff"},
		{"Monat kein Monatserster", func(s *Sonderkosten) { s.Monat = "2026-10-15" }, false, "Abrechnungsmonat"},
		{"Art unbekannt", func(s *Sonderkosten) { s.Art = "luxus" }, false, "Art"},
		{"Objekt unbekannt", func(s *Sonderkosten) { s.Objekt, s.Logik = "garage", "" }, false, "Objekt"},
		{"Haus umlegbar ohne Logik", func(s *Sonderkosten) { s.Logik = "" }, false, "Berechnungslogik"},
		{"Logik unbekannt", func(s *Sonderkosten) { s.Logik = "wohnung1" }, false, "Berechnungslogik"},
		{"Logik bei Wohnung 1", func(s *Sonderkosten) { s.Objekt = ObjektWohnung1 }, false, "Berechnungslogik"},
		{"Logik bei nicht umlegbar", func(s *Sonderkosten) { s.Umlegbar = false }, false, "Berechnungslogik"},
		{"Wohnung 1 ohne Logik", func(s *Sonderkosten) { s.Objekt, s.Logik = ObjektWohnung1, "" }, false, ""},
		{"Personen ohne Fixkosteneingabe", func(s *Sonderkosten) { s.Logik = LogikPersonen }, false, "Fixkosteneingabe"},
		{"Personen mit Fixkosteneingabe", func(s *Sonderkosten) { s.Logik = LogikPersonen }, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := gueltigeSonderkosten()
			c.aendere(&s)
			got := validiereSonderkosten(s, c.hatFixkosten)
			if c.want == "" && len(got) != 0 {
				t.Fatalf("Meldungen %v, want keine", got)
			}
			if c.want != "" && (len(got) != 1 || !strings.Contains(got[0], c.want)) {
				t.Fatalf("Meldungen %v, want eine mit %q", got, c.want)
			}
		})
	}
}

func TestSonderkosten_StoreLehntUngueltigesAb(t *testing.T) {
	ungueltig := gueltigeSonderkosten()
	ungueltig.Betreff = ""
	ungueltig.Logik = LogikPersonen // no Fixkosteneingabe in 2026-10

	var fehler *SonderkostenFehler
	t.Run("PruefeSonderkosten", func(t *testing.T) {
		db := openTestDB(t)
		msgs, err := PruefeSonderkosten(db, ungueltig)
		if err != nil || len(msgs) != 2 {
			t.Fatalf("PruefeSonderkosten = %v, %v, want zwei Meldungen", msgs, err)
		}
	})
	t.Run("Create", func(t *testing.T) {
		db := openTestDB(t)
		if _, err := CreateSonderkosten(db, ungueltig); !errors.As(err, &fehler) || len(fehler.Meldungen) != 2 {
			t.Fatalf("Create: %v, want *SonderkostenFehler mit zwei Meldungen", err)
		}
		if alle, _ := AllSonderkosten(db); len(alle) != 0 {
			t.Errorf("gespeichert: %+v", alle)
		}
	})
	t.Run("Update prüft neue Werte, Altdaten bleiben lesbar", func(t *testing.T) {
		db := openTestDB(t)
		// Altlast per rohem INSERT, Personen-Logik ohne Fixkosteneingabe.
		if _, err := db.Exec(`INSERT INTO sonderkosten (id, monat, betreff, beschreibung, art, firma, rechnungsnummer, betrag, objekt, umlegbar, logik) VALUES (1, '2026-10-01', 'Alt', '', 'reparatur', '', '', 100, 'haus', 1, 'personen')`); err != nil {
			t.Fatal(err)
		}
		if alle, err := AllSonderkosten(db); err != nil || len(alle) != 1 {
			t.Fatalf("Lesen: %v, %v", alle, err)
		}
		ungueltig.ID = 1
		if err := UpdateSonderkosten(db, ungueltig); !errors.As(err, &fehler) {
			t.Fatalf("Update: %v, want *SonderkostenFehler", err)
		}
	})
	t.Run("Import atomar", func(t *testing.T) {
		db := openTestDB(t)
		if _, err := ImportSonderkosten(db, []Sonderkosten{gueltigeSonderkosten(), ungueltig}); !errors.As(err, &fehler) {
			t.Fatalf("Import: %v, want *SonderkostenFehler", err)
		}
		if alle, _ := AllSonderkosten(db); len(alle) != 0 {
			t.Errorf("nicht atomar: %+v", alle)
		}
	})
}
