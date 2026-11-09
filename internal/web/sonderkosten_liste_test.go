package web

import (
	"slices"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestSonderkostenFilterAus(t *testing.T) {
	jahre := []int{2026, 2025}
	tests := []struct {
		name, jahr, art, objekt string
		want                    sonderkostenFilter
	}{
		{"leer", "", "", "", sonderkostenFilter{Jahr: 2026}},
		{"gueltiges Jahr", "2025", "", "", sonderkostenFilter{Jahr: 2025}},
		{"unbekanntes Jahr", "2020", "", "", sonderkostenFilter{Jahr: 2026}},
		{"kein Jahr", "x", "", "", sonderkostenFilter{Jahr: 2026}},
		{"alle Jahre", "alle", "", "", sonderkostenFilter{Jahr: 2026, AlleJahre: true}},
		{"gueltige Art", "", store.ArtReparatur, "", sonderkostenFilter{Jahr: 2026, Art: store.ArtReparatur}},
		{"ungueltige Art", "", "nix", "", sonderkostenFilter{Jahr: 2026}},
		{"gueltiges Objekt", "", "", store.ObjektWohnung1, sonderkostenFilter{Jahr: 2026, Objekt: store.ObjektWohnung1}},
		{"unbekanntes Objekt", "", "", "nix", sonderkostenFilter{Jahr: 2026}},
	}
	for _, tt := range tests {
		if got := sonderkostenFilterAus(tt.jahr, tt.art, tt.objekt, jahre, 2026); got != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestSonderkostenJahre(t *testing.T) {
	alle := []store.Sonderkosten{{Monat: "2024-05-01"}, {Monat: "2028-01-01"}, {Monat: "2024-06-01"}}
	if got, want := sonderkostenJahre(alle, 2026), []int{2028, 2026, 2024}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := sonderkostenJahre(nil, 2026), []int{2026}; !slices.Equal(got, want) {
		t.Errorf("leer: got %v, want %v", got, want)
	}
}

func TestBaueSonderkostenListe(t *testing.T) {
	alle := []store.Sonderkosten{
		{ID: 1, Monat: "2025-12-01", Betrag: 10, Art: store.ArtReparatur, Umlegbar: true},
		{ID: 2, Monat: "2026-01-01", Betrag: 20, Art: store.ArtReparatur},
		{ID: 3, Monat: "2026-01-01", Betrag: 5, Art: store.ArtReparatur, Umlegbar: true},
		{ID: 4, Monat: "2026-02-01", Betrag: 7, Art: store.ArtReparatur, Umlegbar: true, Objekt: store.ObjektHaus, Logik: store.LogikPersonen},
	}
	l := baueSonderkostenListe(alle, nil, sonderkostenFilter{Jahr: 2026})
	if l.Gesamt != 32 || l.Umlegbar != 12 || l.VermieterTraegt() != 20 {
		t.Errorf("sums: %v %v %v", l.Gesamt, l.Umlegbar, l.VermieterTraegt())
	}
	if len(l.Monate) != 2 || l.Monate[0].Karten[0].ID != 4 || len(l.Monate[1].Karten) != 2 {
		t.Fatalf("groups/order: %+v", l.Monate)
	}
	if !l.Monate[0].Karten[0].NichtBerechenbar {
		t.Error("badge expected without Fixkosteneingabe")
	}
	if l.Monate[1].Umlegbar != 5 || l.Monate[1].Karten[0].LoeschenZusatz != "" || l.Monate[1].Karten[1].LoeschenZusatz == "" {
		t.Errorf("month group: %+v", l.Monate[1])
	}
	if l := baueSonderkostenListe(alle, []string{"2026-02"}, sonderkostenFilter{Jahr: 2026}); l.Monate[0].Karten[0].NichtBerechenbar {
		t.Error("no badge with Fixkosteneingabe")
	}
	if l := baueSonderkostenListe(alle, nil, sonderkostenFilter{AlleJahre: true}); l.Gesamt != 42 || len(l.Monate) != 3 {
		t.Errorf("alle Jahre: %+v", l)
	}
	if l := baueSonderkostenListe(alle, nil, sonderkostenFilter{AlleJahre: true, Art: store.ArtWartung}); len(l.Monate) != 0 || l.Gesamt != 0 {
		t.Errorf("Art filter: %+v", l)
	}
	if l := baueSonderkostenListe(alle, nil, sonderkostenFilter{Jahr: 2026, Art: store.ArtReparatur}); l.Gesamt != 32 {
		t.Errorf("Art filter match: %+v", l)
	}
	if l := baueSonderkostenListe(alle, nil, sonderkostenFilter{AlleJahre: true, Objekt: store.ObjektHaus}); l.Gesamt != 7 || len(l.Monate) != 1 {
		t.Errorf("Objekt filter: %+v", l)
	}
}
