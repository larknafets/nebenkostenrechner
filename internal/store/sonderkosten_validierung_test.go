package store

import (
	"strings"
	"testing"
)

func gueltigeSonderkosten() Sonderkosten {
	return Sonderkosten{
		Monat: "2026-10-01", Betreff: "Pumpe", Art: ArtReparatur, Betrag: 100,
		Umsetzung: UmsetzungHaus, Umlegbar: true, Logik: LogikWohneinheit,
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
		{"Betrag 0", func(s *Sonderkosten) { s.Betrag = 0 }, false, "Betrag"},
		{"Betrag negativ", func(s *Sonderkosten) { s.Betrag = -5 }, false, "Betrag"},
		{"Betreff leer", func(s *Sonderkosten) { s.Betreff = " " }, false, "Betreff"},
		{"Monat kein Monatserster", func(s *Sonderkosten) { s.Monat = "2026-10-15" }, false, "Abrechnungsmonat"},
		{"Art unbekannt", func(s *Sonderkosten) { s.Art = "luxus" }, false, "Art"},
		{"Umsetzung unbekannt", func(s *Sonderkosten) { s.Umsetzung, s.Logik = "garage", "" }, false, "Umsetzung"},
		{"Haus umlegbar ohne Logik", func(s *Sonderkosten) { s.Logik = "" }, false, "Berechnungslogik"},
		{"Logik unbekannt", func(s *Sonderkosten) { s.Logik = "wohnung1" }, false, "Berechnungslogik"},
		{"Logik bei Wohnung 1", func(s *Sonderkosten) { s.Umsetzung = UmsetzungWohnung1 }, false, "Berechnungslogik"},
		{"Logik bei nicht umlegbar", func(s *Sonderkosten) { s.Umlegbar = false }, false, "Berechnungslogik"},
		{"Wohnung 1 ohne Logik", func(s *Sonderkosten) { s.Umsetzung, s.Logik = UmsetzungWohnung1, "" }, false, ""},
		{"Personen ohne Fixkosteneingabe", func(s *Sonderkosten) { s.Logik = LogikPersonen }, false, "Fixkosteneingabe"},
		{"Personen mit Fixkosteneingabe", func(s *Sonderkosten) { s.Logik = LogikPersonen }, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := gueltigeSonderkosten()
			c.aendere(&s)
			got := ValidiereSonderkosten(s, c.hatFixkosten)
			if c.want == "" && len(got) != 0 {
				t.Fatalf("Meldungen %v, want keine", got)
			}
			if c.want != "" && (len(got) != 1 || !strings.Contains(got[0], c.want)) {
				t.Fatalf("Meldungen %v, want eine mit %q", got, c.want)
			}
		})
	}
}
