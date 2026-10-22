package calc

import "testing"

// kostenW1 builds Verbrauchskosten whose Wohnung-1 share has the given parts,
// from hand-made values instead of a seeded database.
func kostenW1(heizung, frischwasser, abwasser, wallbox, pvGutschrift float64) Kosten {
	return Kosten{
		Heizung:        &HeizungErgebnis{KostenHeizungW1: heizung},
		Wasser:         &WasserErgebnis{KostenFrischwasserW1: frischwasser, KostenAbwasserW1: abwasser},
		Strom:          &StromErgebnis{KostenWallboxW1: wallbox},
		PVGutschriftW1: pvGutschrift,
	}
}

func fixErgebnis(w1 map[string]float64) *FixkostenErgebnis {
	e := &FixkostenErgebnis{}
	for key, kosten := range w1 {
		e.Positionen = append(e.Positionen, FixkostenPosition{Key: key, KostenW1: kosten})
	}
	return e
}

func TestMonatswerte(t *testing.T) {
	kosten := []MonatsKosten{
		{Monat: "2026-02-01", Kosten: kostenW1(10, 3, 2, 4, 6)},
		{Monat: "2026-01-01", Kosten: kostenW1(20, 4, 1, 0, 0)},
		{Monat: "2026-02-01", Kosten: kostenW1(5, 1, 1, 0, 0)}, // second Ablesung of the month
	}
	fix := []MonatsFixkosten{
		{Monat: "2026-02-01", Ergebnis: fixErgebnis(map[string]float64{"grundsteuer": 7, "wartung": 3}), Abschlag: map[int64]float64{1: 100}},
		{Monat: "2026-03-01", Ergebnis: fixErgebnis(map[string]float64{"grundsteuer": 9}), Abschlag: map[int64]float64{1: 100}},
		{Monat: "2026-02-01", Ergebnis: fixErgebnis(map[string]float64{"grundsteuer": 999}), Abschlag: map[int64]float64{1: 999}}, // loses to the first
	}

	t.Run("summiert Ablesungen je Monat, ältester Monat zuerst", func(t *testing.T) {
		w := Monatswerte(1, kosten, fix, Posten{})
		if len(w) != 3 || w[0].Monat != "2026-01-01" || w[1].Monat != "2026-02-01" || w[2].Monat != "2026-03-01" {
			t.Fatalf("Monate = %+v", w)
		}
		if w[1].Verbrauch != 22 || !w[1].HatVerbrauch {
			t.Errorf("Februar Verbrauch = %v (%v), want 22 (zwei Ablesungen)", w[1].Verbrauch, w[1].HatVerbrauch)
		}
		if w[2].HatVerbrauch {
			t.Errorf("März hat keine Ablesung, HatVerbrauch = true")
		}
	})
	t.Run("Posten: Wallbox und PV-Gutschrift zählen nur, wenn gewählt", func(t *testing.T) {
		ohne := Monatswerte(1, kosten, nil, Posten{})
		mit := Monatswerte(1, kosten, nil, Posten{Wallbox: true, PVGutschrift: true})
		if ohne[1].Verbrauch != 22 || ohne[1].Gutschrift != 0 {
			t.Errorf("ohne Posten: Verbrauch %v, Gutschrift %v, want 22 und 0", ohne[1].Verbrauch, ohne[1].Gutschrift)
		}
		if mit[1].Verbrauch != 26 || mit[1].Gutschrift != 6 {
			t.Errorf("mit Posten: Verbrauch %v, Gutschrift %v, want 26 und 6", mit[1].Verbrauch, mit[1].Gutschrift)
		}
	})
	t.Run("Fixkosten: alle Posten, die erste Eingabe eines Monats zählt", func(t *testing.T) {
		w := Monatswerte(1, kosten, fix, Posten{})
		if w[1].Fixkosten != 10 || w[1].Abschlag != 100 || !w[1].HatFixkosten {
			t.Errorf("Februar = %+v, want Fixkosten 10, Abschlag 100", w[1])
		}
		if w[0].HatFixkosten {
			t.Errorf("Januar hat keine Eingabe, HatFixkosten = true")
		}
	})
	t.Run("Fixkosten: nur gewählte Posten", func(t *testing.T) {
		w := Monatswerte(1, kosten, fix, Posten{Fixkosten: func(key string) bool { return key == "grundsteuer" }})
		if w[1].Fixkosten != 7 {
			t.Errorf("Februar Fixkosten = %v, want 7 (nur grundsteuer)", w[1].Fixkosten)
		}
	})
}

func TestVerlauf(t *testing.T) {
	werte := []Monatswert{
		{Monat: "2026-01-01", Verbrauch: 25, HatVerbrauch: true, Fixkosten: 10, Abschlag: 100, HatFixkosten: true},
		{Monat: "2026-02-01", Verbrauch: 22.333, HatVerbrauch: true, Fixkosten: 10, Abschlag: 100, HatFixkosten: true, Gutschrift: 5},
	}
	zeilen, end := Verlauf(50, werte)
	if len(zeilen) != 2 {
		t.Fatalf("Zeilen = %+v", zeilen)
	}
	if zeilen[0].Saldo != 115 {
		t.Errorf("Saldo Januar = %v, want 115 (50 Übertrag + 100 - 35)", zeilen[0].Saldo)
	}
	if zeilen[1].Verbrauch != 22.33 {
		t.Errorf("Verbrauch Februar = %v, want auf den Cent gerundet 22.33", zeilen[1].Verbrauch)
	}
	if zeilen[1].Saldo != 187.67 || end != 187.67 {
		t.Errorf("Saldo Februar = %v, Endsaldo = %v, want 187.67 (115 + 100 + 5 - 32.33)", zeilen[1].Saldo, end)
	}

	if z, end := Verlauf(12.5, nil); len(z) != 0 || end != 12.5 {
		t.Errorf("ohne Monate: %v, %v, want leer und der Übertrag", z, end)
	}
}

func TestMonatswerte_FixkostenJeLogikRunden(t *testing.T) {
	fix := []MonatsFixkosten{{Monat: "2026-02-01", Ergebnis: &FixkostenErgebnis{Positionen: []FixkostenPosition{
		{Key: "a", Logik: "qm", KostenW1: 0.006},
		{Key: "b", Logik: "personen", KostenW1: 0.006},
	}}}}

	if got := Monatswerte(1, nil, fix, Posten{})[0].Fixkosten; got != 0.012 {
		t.Errorf("ungerundet = %v, want 0.012", got)
	}
	// Each group rounds to 0.01 first: the Dashboard shows 0.01 + 0.01.
	if got := Monatswerte(1, nil, fix, Posten{FixkostenJeLogik: true})[0].Fixkosten; got != 0.02 {
		t.Errorf("je Logik gerundet = %v, want 0.02", got)
	}
}
