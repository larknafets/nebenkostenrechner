package calc_test

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func vollePeriode(id int64, date string, readings map[string]float64) *store.LatestPeriod {
	return &store.LatestPeriod{
		ID: id, ReadingDate: date, Monat: date,
		Readings: baseReadings(readings),
	}
}

// volleEingabe is the Fixkosten-Eingabe of monat with the standard prices,
// Personenzahl 2 / 1 and both switches off.
func volleEingabe(monat string) *store.FixkostenEingabeDetails {
	return &store.FixkostenEingabeDetails{
		Monat:             monat,
		Personen:          map[int64]float64{1: 2, 2: 1},
		Strompreis:        store.Float64(0.22),
		FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis:     store.Float64(4.87),
		EinspeisungPreis:  store.Float64(0.08),
	}
}

var wohnungen = []store.Apartment{{ID: 1, QM: 116.23}, {ID: 2, QM: 86}}

func TestVerbrauchskosten_Gruende(t *testing.T) {
	// Without a Fixkosten-Eingabe for its month, and without a meter reading:
	// the Teilstand is named before Fixkosten, the oldest Ablesung keeps
	// "keine Vorperiode" although its month has no Eingabe either.
	teilstand := vollePeriode(3, "2026-12-01", nil)
	delete(teilstand.Readings, "strom_gesamt")
	v := calc.New(calc.Daten{
		Periods: []*store.LatestPeriod{
			vollePeriode(1, "2026-10-01", nil),
			vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100}),
			teilstand,
		},
		Eingaben:   []*store.FixkostenEingabeDetails{volleEingabe("2026-11-01")},
		Apartments: wohnungen,
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})

	want := map[int64]calc.Grund{1: calc.GrundKeineVorperiode, 2: calc.GrundKeiner, 3: calc.GrundTeilstand}
	for id, grund := range want {
		a, ok := v.Ablesung(id)
		if !ok || a.Grund != grund {
			t.Errorf("Ablesung(%d) = %+v, ok=%v, want Grund %v", id, a, ok, grund)
		}
	}
	if _, ok := v.Ablesung(99); ok {
		t.Error("Ablesung(99) ok = true, want false")
	}
	if got := v.Berechenbare(); len(got) != 1 || got[0].Period.ID != 2 {
		t.Errorf("Berechenbare() = %+v, want nur Ablesung 2", got)
	}
}

func TestVerbrauchskosten_FixkostenFehlen(t *testing.T) {
	ohnePreis := func(setze func(*store.FixkostenEingabeDetails)) []*store.FixkostenEingabeDetails {
		e := volleEingabe("2026-11-01")
		setze(e)
		return []*store.FixkostenEingabeDetails{e}
	}
	tests := []struct {
		name     string
		eingaben []*store.FixkostenEingabeDetails
		want     calc.Grund
	}{
		{"vollständige Eingabe", []*store.FixkostenEingabeDetails{volleEingabe("2026-11-01")}, calc.GrundKeiner},
		{"keine Eingabe", nil, calc.GrundFixkostenFehlen},
		{"Eingabe nur für einen anderen Monat", []*store.FixkostenEingabeDetails{volleEingabe("2026-10-01")}, calc.GrundFixkostenFehlen},
		{"Strompreis leer", ohnePreis(func(e *store.FixkostenEingabeDetails) { e.Strompreis = nil }), calc.GrundFixkostenFehlen},
		{"Frischwasserpreis leer", ohnePreis(func(e *store.FixkostenEingabeDetails) { e.FrischwasserPreis = nil }), calc.GrundFixkostenFehlen},
		{"Abwasserpreis leer", ohnePreis(func(e *store.FixkostenEingabeDetails) { e.AbwasserPreis = nil }), calc.GrundFixkostenFehlen},
		{"Einspeisungspreis leer", ohnePreis(func(e *store.FixkostenEingabeDetails) { e.EinspeisungPreis = nil }), calc.GrundFixkostenFehlen},
		{"Preis 0 ist erfasst", ohnePreis(func(e *store.FixkostenEingabeDetails) { e.Strompreis = store.Float64(0) }), calc.GrundKeiner},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := calc.New(calc.Daten{
				Periods:    []*store.LatestPeriod{vollePeriode(1, "2026-10-01", nil), vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100})},
				Eingaben:   tc.eingaben,
				Apartments: wohnungen,
				Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
			})
			a, _ := v.Ablesung(2)
			if a.Grund != tc.want || a.Berechenbar() != (tc.want == calc.GrundKeiner) {
				t.Errorf("Grund = %v, Berechenbar = %v, want Grund %v", a.Grund, a.Berechenbar(), tc.want)
			}
			if tc.want != calc.GrundKeiner && len(v.Berechenbare()) != 0 {
				t.Errorf("Berechenbare = %d, want 0", len(v.Berechenbare()))
			}
		})
	}
}

// Personenzahl, prices and switches come from the Eingabe of the
// Abrechnungsmonat, not from the Ablesung, and every Ablesung of that month
// uses the same values.
func TestVerbrauchskosten_WerteAusDerEingabeDesMonats(t *testing.T) {
	p2 := vollePeriode(2, "2026-11-01", map[string]float64{"strom_gesamt": 100, "strom_einspeisung": 50, "wasser_gesamt": 10, "wasser_warmwasseraufbereitung": 4})
	p3 := vollePeriode(3, "2026-11-01", map[string]float64{"strom_gesamt": 200, "strom_einspeisung": 100, "wasser_gesamt": 20, "wasser_warmwasseraufbereitung": 8})
	e := volleEingabe("2026-11-01")
	e.Strompreis, e.EinspeisungPreis = store.Float64(0.5), store.Float64(0.1)
	e.FrischwasserPreis, e.AbwasserPreis = store.Float64(2), store.Float64(3)
	e.Personen = map[int64]float64{1: 3, 2: 1}
	e.PVGutschriftW1 = true
	v := calc.New(calc.Daten{
		Periods:    []*store.LatestPeriod{vollePeriode(1, "2026-10-01", nil), p2, p3},
		Eingaben:   []*store.FixkostenEingabeDetails{e},
		Apartments: wohnungen,
		Haus:       store.Haus{HeizungWaermeGewichtung: 0.7},
	})
	for _, id := range []int64{2, 3} {
		a, _ := v.Ablesung(id)
		if !a.Berechenbar() {
			t.Fatalf("Ablesung %d: Grund %v, want berechenbar", id, a.Grund)
		}
		if got := a.Kosten.Einspeisung.Ertrag; !gleich(got, 5) {
			t.Errorf("Ablesung %d: Ertrag = %v, want 5 (50 kWh * 0,10 der Eingabe)", id, got)
		}
		if got := a.Kosten.Anteil(1).PVGutschrift; !gleich(got, 5) {
			t.Errorf("Ablesung %d: PVGutschrift W1 = %v, want 5 (Häkchen der Eingabe)", id, got)
		}
		w := a.Kosten.Wasser
		if w.PersonenW1 != 3 || w.PersonenW2 != 1 {
			t.Errorf("Ablesung %d: Personen %v / %v, want 3 / 1 (Eingabe)", id, w.PersonenW1, w.PersonenW2)
		}
		if got, want := w.KostenFrischwasserW1+w.KostenFrischwasserW2, 2*(w.FrischwasserW1+w.FrischwasserW2); !gleich(got, want) || want == 0 {
			t.Errorf("Ablesung %d: Frischwasserkosten = %v, want %v (Preis 2 der Eingabe)", id, got, want)
		}
	}
}

// TestLoad_NurBerechenbareAblesungenInReihenfolge verifies Load reads every
// Ablesung from the database and Berechenbare returns all but the first,
// oldest first.
func TestLoad_NurBerechenbareAblesungenInReihenfolge(t *testing.T) {
	db := openTestDB(t)
	var ids []int64
	for i, date := range []string{"2026-09-01", "2026-10-01", "2026-11-01", "2026-11-15"} {
		n := float64(i)
		id, err := createPeriod(db, ablesungMitFixkosten{
			ReadingDate: date, Monat: monatVon(date),
			Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
			AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
			Readings: baseReadings(map[string]float64{"strom_gesamt": 1000 * n, "strom_einspeisung": 500 * n}),
			Personen: map[int64]float64{1: 2, 2: 1},
		})
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		ids = append(ids, id)
	}

	v, err := calc.Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := v.Berechenbare()
	if len(got) != 3 {
		t.Fatalf("len(Berechenbare()) = %d, want 3", len(got))
	}
	for i, a := range got {
		if a.Period.ID != ids[i+1] {
			t.Errorf("Berechenbare()[%d] = Ablesung %d, want %d", i, a.Period.ID, ids[i+1])
		}
	}
	if got[0].Kosten.Einspeisung.EinspeisungKWh != 500 {
		t.Errorf("EinspeisungKWh = %v, want 500", got[0].Kosten.Einspeisung.EinspeisungKWh)
	}
}
