package web

import (
	"reflect"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// The Haus list carries the whole-house cards and Verläufe in the order of the
// cards and the dropdown, per role (#325).
func TestSichtbareAnsicht_HausReihenfolge(t *testing.T) {
	dd := dashboardData{
		HasAnyData:             true,
		Jahr:                   2026,
		WallboxWeiterberechnen: true,
		Apartments: []store.Apartment{
			{ID: 1, Name: "Wohnung 1", Status: store.StatusVermietet},
			{ID: 2, Name: "Wohnung 2", Status: store.StatusVermietet},
		},
	}
	namen := func(rolle store.Rolle) []string {
		var out []string
		for _, h := range sichtbareAnsicht(zugriffMitDefaults(rolle), dd, nil).Haus {
			out = append(out, h.Card.Name)
		}
		return out
	}
	if got, want := namen(RolleVermieter), []string{"Wärmepumpe", "Wallboxen", "PV-Anlage"}; !reflect.DeepEqual(got, want) {
		t.Errorf("angemeldet = %v, want %v", got, want)
	}
	if got, want := namen(store.RolleNichtAngemeldet), []string{"Wärmepumpe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("anonym = %v, want %v", got, want)
	}
}
