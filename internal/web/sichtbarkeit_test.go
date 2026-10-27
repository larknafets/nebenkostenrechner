package web

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Table test for the Sichtbarkeit module (#293) at its seam: no HTTP, no
// database. Both apartments are rented, so Mieteinnahmen exist wherever the
// viewer may see them.
func TestSichtbareAnsicht_JeBetrachter(t *testing.T) {
	dd := dashboardData{
		HasAnyData:             true,
		Jahr:                   2026,
		WallboxWeiterberechnen: true,
		Apartments: []store.Apartment{
			{ID: 1, Name: "Wohnung 1", Status: store.StatusVermietet},
			{ID: 2, Name: "Wohnung 2", Status: store.StatusVermietet},
		},
	}
	miete := []mieteMonat{
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
			// The heat pump card has no condition, not even a login (#296).
			if v.WP == nil {
				t.Error("Wärmepumpe fehlt")
			}
		})
	}
}

// The Wallboxen card for a logged-in user follows the Abrechnung rule (#297):
// Eigennutzung always, Vermietung only with "Wallbox weiterberechnen".
func TestSichtbareAnsicht_WallboxKarteNachWohnungsstatus(t *testing.T) {
	eigen, miet := store.StatusEigennutzung, store.StatusVermietet
	tests := []struct {
		name     string
		w1, w2   string
		flag     bool
		sichtbar bool
	}{
		{"beide vermietet, Flag aus", miet, miet, false, false},
		{"beide vermietet, Flag an", miet, miet, true, true},
		{"Wohnung 1 Eigennutzung, Flag aus", eigen, miet, false, true},
		{"Wohnung 2 Eigennutzung, Flag aus", miet, eigen, false, true},
		{"beide Eigennutzung, Flag aus", eigen, eigen, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := dashboardData{
				HasAnyData:             true,
				Jahr:                   2026,
				WallboxWeiterberechnen: tt.flag,
				Apartments: []store.Apartment{
					{ID: 1, Name: "Wohnung 1", Status: tt.w1},
					{ID: 2, Name: "Wohnung 2", Status: tt.w2},
				},
			}
			got := sichtbareAnsicht(betrachterAngemeldet, dd, nil)
			if (got.Wallbox != nil) != tt.sichtbar {
				t.Errorf("Wallbox sichtbar = %v, want %v", got.Wallbox != nil, tt.sichtbar)
			}
			if got.WP == nil || got.PV == nil {
				t.Errorf("WP/PV present = %v/%v, want both", got.WP != nil, got.PV != nil)
			}
			if anon := sichtbareAnsicht(betrachterAnonym, dd, nil); anon.Wallbox != nil || anon.WP == nil {
				t.Errorf("anonym Wallbox/WP = %v/%v, want nil/present", anon.Wallbox != nil, anon.WP != nil)
			}
		})
	}
}

// loadDashboardData hands the Miete over as a projection of the loaded
// Datenstand (Abrechnungsmonat and Miete per apartment, #294), so the
// dashboard needs no second query for it.
func TestLoadDashboardData_LiefertMieteProjektion(t *testing.T) {
	db := openTestDB(t)
	row := strings.Replace(fixkostenCSVRow("2026-06-01", "100"), ";2;1;150;90;;800", ";2;1;150;;300;", 1)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + row + "\n"
	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("import status %d (%s)", w.Code, w.Body.String())
	}

	_, miete, err := loadDashboardData(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(miete) != 1 || miete[0].Monat != "2026-06-01" || miete[0].Miete[1] != 300 {
		t.Errorf("Miete projection = %+v, want one entry 2026-06-01 with Wohnung 1 = 300", miete)
	}
}

// The dropdown of the Monatsverlauf lists what the viewer sees, in card
// order; with one entry only the template shows no dropdown (#306).
func TestVerlaufAuswahl(t *testing.T) {
	dd := dashboardData{
		HasAnyData:             true,
		Jahr:                   2026,
		WallboxWeiterberechnen: true,
		Apartments: []store.Apartment{
			{ID: 1, Name: "Wohnung 1", Status: store.StatusVermietet},
			{ID: 2, Name: "Wohnung 2", Status: store.StatusVermietet},
		},
	}
	ids := func(b betrachter) []string {
		var out []string
		for _, o := range verlaufAuswahl(sichtbareAnsicht(b, dd, nil)) {
			out = append(out, o.ID)
		}
		return out
	}
	if got, want := ids(betrachterAngemeldet), []string{"1", "2", "waermepumpe", "wallbox", "pv"}; !reflect.DeepEqual(got, want) {
		t.Errorf("angemeldet = %v, want %v", got, want)
	}
	if got, want := ids(betrachterAnonym), []string{"2", "waermepumpe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("anonym = %v, want %v", got, want)
	}
	dd.Apartments = nil
	if got, want := ids(betrachterAnonym), []string{"waermepumpe"}; !reflect.DeepEqual(got, want) {
		t.Errorf("anonym ohne Wohnungen = %v, want %v", got, want)
	}
}
