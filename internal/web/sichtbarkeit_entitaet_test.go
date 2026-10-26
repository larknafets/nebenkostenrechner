package web

import (
	"reflect"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sichtbareEntitaet builds only the requested entity, but decides its
// visibility by the same table as sichtbareAnsicht and yields the same
// card and Verlauf.
func TestSichtbareEntitaet_JeBetrachterUndEntitaet(t *testing.T) {
	dd := dashboardData{
		HasAnyData: true,
		Jahr:       2026,
		Apartments: []store.Apartment{
			{ID: 1, Name: "Wohnung 1", Status: store.StatusVermietet},
			{ID: 2, Name: "Wohnung 2", Status: store.StatusVermietet},
		},
	}
	miete := []mieteMonat{
		{Monat: "2026-11-01", Miete: map[int64]float64{1: 300, 2: 712.34}},
	}

	tests := []struct {
		name       string
		betrachter betrachter
		entitaet   entitaet
		sichtbar   bool
	}{
		{"Anonym Wohnung 1", betrachterAnonym, wohnungEntitaet(1), false},
		{"Anonym Wohnung 2", betrachterAnonym, wohnungEntitaet(2), true},
		{"Anonym Wallboxen", betrachterAnonym, entitaet{Art: entitaetWallbox}, false},
		{"Anonym PV", betrachterAnonym, entitaet{Art: entitaetPV}, false},
		{"Angemeldet Wohnung 1", betrachterAngemeldet, wohnungEntitaet(1), true},
		{"Angemeldet Wallboxen", betrachterAngemeldet, entitaet{Art: entitaetWallbox}, true},
		{"Widget Wohnung 2", betrachterWidget, wohnungEntitaet(2), true},
		{"Widget Wallboxen", betrachterWidget, entitaet{Art: entitaetWallbox}, true},
		{"Widget PV", betrachterWidget, entitaet{Art: entitaetPV}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full := sichtbareAnsicht(tt.betrachter, dd, miete)
			got := sichtbareEntitaet(tt.betrachter, dd, tt.entitaet, miete)

			var want, have any
			switch tt.entitaet.Art {
			case entitaetWohnung:
				w, ok := full.Wohnung(tt.entitaet.ApartmentID)
				g, gok := got.Wohnung(tt.entitaet.ApartmentID)
				if ok != tt.sichtbar || gok != tt.sichtbar {
					t.Fatalf("Wohnung visible = %v (full %v), want %v", gok, ok, tt.sichtbar)
				}
				want, have = w, g
				if len(got.Cards) > 1 || got.Wallbox != nil || got.PV != nil {
					t.Errorf("view holds more than the requested entity: %d cards, wallbox %v, pv %v", len(got.Cards), got.Wallbox != nil, got.PV != nil)
				}
			case entitaetWallbox:
				if (got.Wallbox != nil) != tt.sichtbar || got.PV != nil || len(got.Cards) != 0 {
					t.Fatalf("view = %+v, want only Wallbox, visible=%v", got, tt.sichtbar)
				}
				want, have = full.Wallbox, got.Wallbox
			case entitaetPV:
				if (got.PV != nil) != tt.sichtbar || got.Wallbox != nil || len(got.Cards) != 0 {
					t.Fatalf("view = %+v, want only PV, visible=%v", got, tt.sichtbar)
				}
				want, have = full.PV, got.PV
			}
			if tt.sichtbar && !reflect.DeepEqual(want, have) {
				t.Errorf("entity view differs from the full view:\n got %+v\nwant %+v", have, want)
			}
		})
	}
}

// A widget route for an apartment id that is not in the data yields no card
// (nil), not a card built from a zero-value apartment (#288).
func TestResolveWidgetView_UnbekannteWohnungLiefertKeineKarte(t *testing.T) {
	dd := dashboardData{
		HasAnyData: true,
		Jahr:       2026,
		Apartments: []store.Apartment{{ID: 1, Name: "Wohnung 1"}},
	}
	v := resolveWidgetView(dd, wohnungEntitaet(2))
	if v.Card != nil || v.Verlauf != nil || v.SimpleCard != nil || v.SimpleVerlauf != nil {
		t.Errorf("view = %+v, want empty", v)
	}
}
