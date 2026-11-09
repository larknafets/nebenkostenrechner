package calc

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenReihe builds a Fixkostenreihe with one Eingabe in 2026-03 whose
// Personen are 3 and 1; the other months have none.
func sonderkostenReihe() *Fixkostenreihe {
	return NewFixkostenreihe(FixkostenDaten{Eingaben: []*store.FixkostenEingabeDetails{
		{ID: 1, Monat: "2026-03-01", Personen: map[int64]float64{1: 3, 2: 1}},
	}})
}

func sonderkostenWohnungen() []store.Apartment {
	return []store.Apartment{{ID: 1, QM: 100, FlurstueckGroesse: 300}, {ID: 2, QM: 50, FlurstueckGroesse: 100}}
}

func TestSonderkostenAnteile(t *testing.T) {
	haus := func(logik string, betrag float64, umlegbar bool) store.Sonderkosten {
		return store.Sonderkosten{Monat: "2026-03-01", Betrag: betrag, Objekt: store.ObjektHaus, Umlegbar: umlegbar, Logik: logik}
	}
	tests := []struct {
		name   string
		s      store.Sonderkosten
		w1, w2 float64
		ok     bool
	}{
		{"Wohneinheit, Rest-Cent an Wohnung 2", haus(store.LogikWohneinheit, 100.01, true), 50.01, 50, true},
		{"Wohnungsgröße 2:1", haus(store.LogikQM, 100, true), 66.67, 33.33, true},
		{"Flurstück 3:1", haus(store.LogikFlurstueck, 10, true), 7.5, 2.5, true},
		{"Personen 3:1 mit Eingabe", haus(store.LogikPersonen, 80, true), 60, 20, true},
		{"Personen ohne Eingabe im Monat nicht berechenbar", store.Sonderkosten{Monat: "2026-04-01", Betrag: 80, Objekt: store.ObjektHaus, Umlegbar: true, Logik: store.LogikPersonen}, 0, 0, false},
		{"Wohnung 1 trägt alles", store.Sonderkosten{Monat: "2026-03-01", Betrag: 184.3, Objekt: store.ObjektWohnung1, Umlegbar: true}, 184.3, 0, true},
		{"Wohnung 2 trägt alles", store.Sonderkosten{Monat: "2026-03-01", Betrag: 649, Objekt: store.ObjektWohnung2, Umlegbar: true}, 0, 649, true},
		{"nicht umlegbar wirkt nirgends", haus(store.LogikQM, 100, false), 0, 0, true},
		{"nicht umlegbar, Wohnung 2", store.Sonderkosten{Monat: "2026-03-01", Betrag: 5, Objekt: store.ObjektWohnung2}, 0, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := SonderkostenAnteile(tt.s, sonderkostenWohnungen(), sonderkostenReihe())
			if a.W1 != tt.w1 || a.W2 != tt.w2 || a.Berechenbar != tt.ok {
				t.Errorf("got W1 %v, W2 %v, berechenbar %v, want %v, %v, %v", a.W1, a.W2, a.Berechenbar, tt.w1, tt.w2, tt.ok)
			}
			if tt.ok && a.W1+a.W2 != 0 && Round2(a.W1+a.W2) != tt.s.Betrag {
				t.Errorf("Summe %v != Betrag %v", a.W1+a.W2, tt.s.Betrag)
			}
		})
	}
	t.Run("Wohnungsgröße 0/0 fällt auf 50/50", func(t *testing.T) {
		a := SonderkostenAnteile(haus(store.LogikQM, 9.99, true), []store.Apartment{{ID: 1}, {ID: 2}}, nil)
		if a.W1 != 5 || a.W2 != 4.99 {
			t.Errorf("W1 %v, W2 %v, want 5 und 4.99", a.W1, a.W2)
		}
	})
}

func TestSonderkostenMonate(t *testing.T) {
	list := []store.Sonderkosten{
		{Monat: "2026-03-01", Betrag: 100, Objekt: store.ObjektHaus, Umlegbar: true, Logik: store.LogikQM},
		{Monat: "2026-03-01", Betrag: 50, Objekt: store.ObjektWohnung2, Umlegbar: true},
		{Monat: "2026-03-01", Betrag: 70, Objekt: store.ObjektHaus, Umlegbar: false},
		{Monat: "2026-04-01", Betrag: 80, Objekt: store.ObjektHaus, Umlegbar: true, Logik: store.LogikPersonen},
	}
	got := SonderkostenMonate(list, sonderkostenWohnungen(), sonderkostenReihe(), 1)
	want := MonatsSonderkosten{Monat: "2026-03-01", Logik: store.LogikQM, Betrag: 66.67}
	if len(got) != 1 || got[0] != want {
		t.Errorf("Wohnung 1 = %+v, want [%+v]", got, want)
	}
	got = SonderkostenMonate(list, sonderkostenWohnungen(), sonderkostenReihe(), 2)
	if len(got) != 2 || got[1].Logik != store.LogikWohnung2 || got[1].Betrag != 50 {
		t.Errorf("Wohnung 2 = %+v, want Qm-Anteil und 50 mit Logik wohnung2", got)
	}
}

func TestMonatswerte_Sonderkosten(t *testing.T) {
	p := Posten{Sonderkosten: []MonatsSonderkosten{{Monat: "2026-02-01", Logik: store.LogikQM, Betrag: 12.5}, {Monat: "2026-05-01", Logik: store.LogikWohnung1, Betrag: 4}}}
	fix := []MonatsFixkosten{{Monat: "2026-02-01", Ergebnis: fixErgebnis(map[string]float64{"grundsteuer": 7}), Abschlag: map[int64]float64{1: 100}}}

	w := Monatswerte(1, nil, fix, p)
	if len(w) != 2 || w[0].Fixkosten != 19.5 || !w[0].HatSonderkosten {
		t.Fatalf("Februar = %+v, want Fixkosten 19.5 mit Sonderkosten", w)
	}
	if w[1].Monat != "2026-05-01" || w[1].Fixkosten != 4 || w[1].HatFixkosten || w[1].HatVerbrauch {
		t.Errorf("Mai nur mit Sonderkosten = %+v", w[1])
	}
	p.FixkostenJeLogik = true
	if w := Monatswerte(1, nil, fix, p); w[0].Fixkosten != 19.5 {
		t.Errorf("FixkostenJeLogik: Februar Fixkosten = %v, want 19.5", w[0].Fixkosten)
	}
	zeilen, end := Verlauf(0, Monatswerte(1, nil, nil, p))
	if len(zeilen) != 2 || zeilen[1].Fixkosten != 4 || end != -16.5 {
		t.Errorf("Verlauf = %+v, Endsaldo %v, want zwei Zeilen und -16.5", zeilen, end)
	}
}
