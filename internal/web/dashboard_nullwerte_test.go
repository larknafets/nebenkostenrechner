package web

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboard_nullwerte_test.go covers the display rules of the Jahreskarten
// (#276, #304): zero values of PV-Gutschrift and PV-Anteil stay visible, the
// Wallbox block of an apartment disappears when it is not passed on or not
// assigned. Everything goes through the HTTP seam (Dashboard), except the
// unchanged totals, which read the card data.

const (
	titelWallbox      = `title="Wallbox"`
	blockWallbox      = `class="stat-wallbox"`
	titelPVGutschrift = `title="PV-Gutschrift"`
	titelPVAnteil     = `title="Nicht dem Netzbezug zugeordnet (PV)"`
)

func TestJahreskarte_PVZeilenAuchMitNull(t *testing.T) {
	db := openTestDB(t)
	// No PV switch, no feed-in: both PV values are 0.
	wallboxAblesungen(t, db, nil, nil, false)

	body := dashboardHTML(t, db)
	if !strings.Contains(body, titelPVGutschrift) || !strings.Contains(body, titelPVAnteil) {
		t.Error("Dashboard: PV-Gutschrift oder PV-Anteil mit 0 fehlt")
	}
	// A zero credit is no deduction: no minus sign in front of it.
	if strings.Contains(body, "&minus;0,00") || strings.Contains(body, "−0,00") {
		t.Error("PV-Gutschrift 0 steht mit Minuszeichen")
	}
}

// The Wallbox block of a Jahreskarte (#304) shows only when the Wohnung
// carries the Wallbox in its Abrechnung and the costs are assigned to the
// apartments. Wohnung 1 is in Eigennutzung, so it has the block whenever the
// costs are assigned.
func TestJahreskarte_WallboxBlockJeStatusFlagUndZuordnung(t *testing.T) {
	einzelW2 := 15.0
	for _, c := range []struct {
		name       string
		einzelW2   *float64
		weiter     bool
		w2Status   string
		wantBloeke int
	}{
		{"nicht zugeordnet: kein Block", nil, true, store.StatusEigennutzung, 0},
		{"vermietet, Flag aus: nur Wohnung 1", &einzelW2, false, store.StatusVermietet, 1},
		{"vermietet, Flag an: beide", &einzelW2, true, store.StatusVermietet, 2},
		{"Eigennutzung, Flag aus: beide", &einzelW2, false, store.StatusEigennutzung, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			wallboxAblesungen(t, db, nil, c.einzelW2, false)
			saveStammdaten(t, db, func(s *store.StammdatenSave) {
				s.Flags.WallboxWeiterberechnen = c.weiter
				s.Wohnungen = map[int64]store.WohnungDetails{2: {Status: c.w2Status}}
			})

			body := dashboardHTML(t, db)
			if n := strings.Count(body, blockWallbox); n != c.wantBloeke {
				t.Errorf("Dashboard: %d Wallbox-Blöcke, want %d", n, c.wantBloeke)
			}
			// Wallbox is no chip and no segment of the main bar any more.
			if strings.Contains(body, titelWallbox) || strings.Contains(body, "stat-bar-seg cat-wallbox") {
				t.Error("Wallbox steht noch im Hauptbalken oder bei den Chips der Jahreskarte")
			}
			// The house-wide card stays.
			if !strings.Contains(body, "<strong>Wallboxen</strong>") {
				t.Error("Karte Wallboxen fehlt")
			}
		})
	}
}

// Hiding the block changes no number: the Wallbox stays in Verbrauch and Gesamt.
func TestJahreskarte_WallboxBlockAusblendenAendertGesamtNicht(t *testing.T) {
	db := openTestDB(t)
	einzelW2 := 15.0
	wallboxAblesungen(t, db, nil, &einzelW2, false)
	saveStammdaten(t, db, func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = true })
	vorher, _ := buildEntityView(mustLoadDashboard(t, db), 2)

	saveStammdaten(t, db, func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = false })
	nachher, _ := buildEntityView(mustLoadDashboard(t, db), 2)

	if vorher.Wallbox == nil || nachher.Wallbox != nil {
		t.Fatalf("Block vorher %v, nachher %v, want vorhanden und dann weg", vorher.Wallbox, nachher.Wallbox)
	}
	if vorher.GesamtEUR != nachher.GesamtEUR || vorher.VerbrauchEUR != nachher.VerbrauchEUR || vorher.WallboxEUR != nachher.WallboxEUR {
		t.Errorf("Zahlen ändern sich: vorher %+v, nachher %+v", vorher, nachher)
	}
}

func mustLoadDashboard(t *testing.T, db *sql.DB) dashboardData {
	t.Helper()
	dd, _, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	return dd
}
