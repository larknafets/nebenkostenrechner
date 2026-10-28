package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// monatsregelDaten: Ablesungen in 2026-10 (first one), 2026-11 (two, one of
// them a Teilstand) and 2026-12, plus the given Eingaben.
func monatsregelDaten(eingaben ...*store.FixkostenEingabeDetails) *calc.Verbrauchskosten {
	teilstand := vollePeriode(4, "2026-11-20", nil)
	teilstand.Monat = "2026-11-01"
	delete(teilstand.Readings, "strom_gesamt")
	return calc.New(calc.Daten{
		Periods: []*store.LatestPeriod{
			vollePeriode(1, "2026-10-01", nil),
			vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100}),
			teilstand,
			vollePeriode(3, "2026-12-01", map[string]float64{"strom_gesamt": 200}),
		},
		Eingaben:   eingaben,
		Apartments: wohnungen,
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
}

func TestVerbrauchskosten_MonatVollstaendigUndEingabe(t *testing.T) {
	unvollstaendig := volleEingabe("2026-12-01")
	unvollstaendig.AbwasserPreis = nil
	v := monatsregelDaten(volleEingabe("2026-11-01"), unvollstaendig)
	for monat, want := range map[string]bool{"2026-10-01": false, "2026-11-01": true, "2026-12-01": false} {
		if got := v.MonatVollstaendig(monat); got != want {
			t.Errorf("MonatVollstaendig(%s) = %v, want %v", monat, got, want)
		}
	}
	if v.Eingabe("2026-10-01") != nil || v.Eingabe("2026-12-01") != unvollstaendig {
		t.Error("Eingabe(monat): nil ohne Eingabe, sonst die des Monats")
	}
}

func TestVerbrauchskosten_OffeneMonate(t *testing.T) {
	unvollstaendig := volleEingabe("2026-12-01")
	unvollstaendig.ID, unvollstaendig.Strompreis = 7, nil
	// 2026-10: first Ablesung, no Eingabe. 2026-11: complete. 2026-12: incomplete.
	got := monatsregelDaten(volleEingabe("2026-11-01"), unvollstaendig).OffeneMonate()
	want := []calc.OffenerMonat{{Monat: "2026-10-01"}, {Monat: "2026-12-01", EingabeID: 7}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("OffeneMonate = %+v, want %+v (EingabeID 0 = keine Eingabe)", got, want)
	}
	// The Teilstand of November counts too, each month once.
	if got := monatsregelDaten().OffeneMonate(); len(got) != 3 {
		t.Errorf("OffeneMonate ohne Eingaben = %+v, want 3 Monate", got)
	}
}

// A Teilstand without Abrechnungsmonat is valid input and has no month to miss.
func TestVerbrauchskosten_OffeneMonateOhneAbrechnungsmonat(t *testing.T) {
	ohneMonat := vollePeriode(5, "2026-12-15", nil)
	ohneMonat.Monat = ""
	delete(ohneMonat.Readings, "strom_gesamt")
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{vollePeriode(1, "2026-10-01", nil), ohneMonat},
		Apartments: wohnungen,
	})
	got := v.OffeneMonate()
	if len(got) != 1 || got[0].Monat != "2026-10-01" {
		t.Errorf("OffeneMonate = %+v, want only 2026-10-01", got)
	}
}

func TestVerbrauchskosten_BetroffeneAblesungen(t *testing.T) {
	ids := func(as []calc.Ablesung) (out []int64) {
		for _, a := range as {
			out = append(out, a.Period.ID)
		}
		return out
	}
	// Whether the Eingabe is complete does not matter.
	for name, v := range map[string]*calc.Verbrauchskosten{"ohne Eingabe": monatsregelDaten(), "vollständig": monatsregelDaten(volleEingabe("2026-11-01"))} {
		if got := ids(v.BetroffeneAblesungen("2026-11-01")); len(got) != 1 || got[0] != 2 {
			t.Errorf("%s: BetroffeneAblesungen(2026-11) = %v, want [2] (ohne Teilstand)", name, got)
		}
	}
	v := monatsregelDaten()
	if got := v.BetroffeneAblesungen("2026-10-01"); len(got) != 0 {
		t.Errorf("BetroffeneAblesungen der ersten Ablesung = %v, want leer", ids(got))
	}
	if got := ids(v.BetroffeneAblesungen("2026-12-01")); len(got) != 1 || got[0] != 3 {
		t.Errorf("BetroffeneAblesungen(2026-12) = %v, want [3]", got)
	}
}

func TestVerbrauchskosten_DoppelteEingabeLetzteGilt(t *testing.T) {
	leer := volleEingabe("2026-11-01")
	leer.Strompreis = nil
	if v := monatsregelDaten(leer, volleEingabe("2026-11-01")); !v.MonatVollstaendig("2026-11-01") {
		t.Error("letzte (vollständige) Eingabe soll gelten")
	}
	if v := monatsregelDaten(volleEingabe("2026-11-01"), leer); v.MonatVollstaendig("2026-11-01") {
		t.Error("letzte (unvollständige) Eingabe soll gelten")
	}
}
