package web

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Table test for the Sichtbarkeit module (#293) at its seam: no HTTP, no
// database. Both apartments are rented, so Mieteinnahmen exist wherever the
// viewer may see them.
func TestSichtbareAnsicht_JeBetrachter(t *testing.T) {
	dd := dashboardData{
		HasAnyData: true,
		Jahr:       2026,
		Apartments: []store.Apartment{
			{ID: 1, Name: "Wohnung 1", Status: store.StatusVermietet},
			{ID: 2, Name: "Wohnung 2", Status: store.StatusVermietet},
		},
	}
	miete := []*store.FixkostenEingabeDetails{
		{Monat: "2026-11-01", Miete: map[int64]float64{1: 300, 2: 712.34}},
	}

	tests := []struct {
		name         string
		betrachter   betrachter
		wohnungen    []int64
		wallboxPV    bool
		mieteinnahme map[int64]float64 // apartment id -> expected sum, absent = nil
	}{
		{"Anonym", betrachterAnonym, []int64{2}, false, nil},
		{"Angemeldet", betrachterAngemeldet, []int64{1, 2}, true, map[int64]float64{1: 300, 2: 712.34}},
		{"Widget", betrachterWidget, []int64{1, 2}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := sichtbareAnsicht(tt.betrachter, dd, miete)

			if len(v.Cards) != len(tt.wohnungen) || len(v.VerlaufSpalten) != len(tt.wohnungen) {
				t.Fatalf("Cards/Verlauf = %d/%d, want %d each", len(v.Cards), len(v.VerlaufSpalten), len(tt.wohnungen))
			}
			for i, id := range tt.wohnungen {
				c := v.Cards[i]
				if c.ApartmentID != id || v.VerlaufSpalten[i].ApartmentID != id {
					t.Errorf("entry %d: card/verlauf apartment = %d/%d, want %d", i, c.ApartmentID, v.VerlaufSpalten[i].ApartmentID, id)
				}
				want, ok := tt.mieteinnahme[id]
				switch {
				case !ok && c.Mieteinnahmen != nil:
					t.Errorf("Wohnung %d: Mieteinnahmen = %v, want nil", id, *c.Mieteinnahmen)
				case ok && (c.Mieteinnahmen == nil || *c.Mieteinnahmen != want):
					t.Errorf("Wohnung %d: Mieteinnahmen = %v, want %v", id, c.Mieteinnahmen, want)
				}
			}
			if (v.Wallbox != nil) != tt.wallboxPV || (v.PV != nil) != tt.wallboxPV {
				t.Errorf("Wallbox/PV present = %v/%v, want %v", v.Wallbox != nil, v.PV != nil, tt.wallboxPV)
			}
		})
	}
}
