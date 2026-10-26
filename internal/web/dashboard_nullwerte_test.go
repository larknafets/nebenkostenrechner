package web

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboard_nullwerte_test.go covers the display rules of the Jahreskarten
// (#276): zero values of PV-Gutschrift and PV-Anteil stay visible, the
// Wallbox line of a rented apartment disappears when it is not passed on and
// 0. Everything goes through the HTTP seam (Dashboard and widget), except
// the unchanged totals, which read the card data.

const (
	titelWallbox      = `title="Wallbox"`
	titelPVGutschrift = `title="PV-Gutschrift"`
	titelPVAnteil     = `title="Nicht dem Netzbezug zugeordnet (PV)"`
)

func TestJahreskarte_PVZeilenAuchMitNull(t *testing.T) {
	db := openTestDB(t)
	// No PV switch, no feed-in: both PV values are 0.
	wallboxAblesungen(t, db, nil, nil, false)

	for _, route := range []string{"jahressumme", "uebersicht"} {
		w1 := widgetHTML(t, db, route, "wohnung-1")
		if !strings.Contains(w1, titelPVGutschrift) {
			t.Errorf("Widget %s Wohnung 1: PV-Gutschrift mit 0 fehlt", route)
		}
		if strings.Contains(w1, titelPVAnteil) {
			t.Errorf("Widget %s Wohnung 1: PV-Anteil gehört nur zu Wohnung 2", route)
		}
		// The widgets never showed the PV-Anteil, that stays.
		w2 := widgetHTML(t, db, route, "wohnung-2")
		if strings.Contains(w2, titelPVGutschrift) {
			t.Errorf("Widget %s Wohnung 2: PV-Gutschrift gehört nur zu Wohnung 1", route)
		}
	}

	body := dashboardHTML(t, db)
	if !strings.Contains(body, titelPVGutschrift) || !strings.Contains(body, titelPVAnteil) {
		t.Error("Dashboard: PV-Gutschrift oder PV-Anteil mit 0 fehlt")
	}
	// A zero credit is no deduction: no minus sign in front of it.
	if strings.Contains(body, "&minus;0,00") || strings.Contains(body, "−0,00") {
		t.Error("PV-Gutschrift 0 steht mit Minuszeichen")
	}
}

func TestJahreskarte_WallboxZeileJeStatusFlagUndBetrag(t *testing.T) {
	einzelW2 := 15.0
	for _, c := range []struct {
		name         string
		einzelW2     *float64
		weiter       bool
		w2Status     string
		wantWallbox2 bool
	}{
		{"vermietet, Flag aus, 0: ausgeblendet", nil, false, store.StatusVermietet, false},
		{"vermietet, Flag aus, Betrag: sichtbar", &einzelW2, false, store.StatusVermietet, true},
		{"vermietet, Flag an, 0: sichtbar", nil, true, store.StatusVermietet, true},
		{"Eigennutzung, Flag aus, 0: sichtbar", nil, false, store.StatusEigennutzung, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			wallboxAblesungen(t, db, nil, c.einzelW2, false)
			saveStammdaten(t, db, func(s *store.StammdatenSave) {
				s.Flags.WallboxWeiterberechnen = c.weiter
				s.Wohnungen = map[int64]store.WohnungDetails{2: {Status: c.w2Status}}
			})

			for _, route := range []string{"jahressumme", "uebersicht"} {
				if got := strings.Contains(widgetHTML(t, db, route, "wohnung-2"), titelWallbox); got != c.wantWallbox2 {
					t.Errorf("Widget %s Wohnung 2: Wallbox sichtbar = %v, want %v", route, got, c.wantWallbox2)
				}
				// Wohnung 1 is in Eigennutzung: always visible.
				if !strings.Contains(widgetHTML(t, db, route, "wohnung-1"), titelWallbox) {
					t.Errorf("Widget %s Wohnung 1: Wallbox fehlt", route)
				}
			}

			// Dashboard: the house-wide card stays, only the line of
			// Wohnung 2 may go.
			body := dashboardHTML(t, db)
			want := 1
			if c.wantWallbox2 {
				want = 2
			}
			if n := strings.Count(body, titelWallbox); n != want {
				t.Errorf("Dashboard: %d Wallbox-Zeilen, want %d", n, want)
			}
			if !strings.Contains(body, "<strong>Wallboxen</strong>") {
				t.Error("Karte Wallboxen fehlt")
			}
		})
	}
}

func TestJahreskarte_WallboxAusblendenAendertGesamtNicht(t *testing.T) {
	db := openTestDB(t)
	wallboxAblesungen(t, db, nil, nil, false)
	vorher, _ := buildEntityView(mustLoadDashboard(t, db), 2)

	saveStammdaten(t, db, func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = false })
	nachher, _ := buildEntityView(mustLoadDashboard(t, db), 2)

	if vorher.GesamtEUR != nachher.GesamtEUR || vorher.VerbrauchEUR != nachher.VerbrauchEUR || vorher.WallboxEUR != nachher.WallboxEUR {
		t.Errorf("Zahlen ändern sich: vorher %+v, nachher %+v", vorher, nachher)
	}
	if segmentByLabel(nachher.Segmente, "Wallbox") != nil {
		t.Error("Segment Wallbox bleibt trotz vermietet, Flag aus, 0")
	}
}

func mustLoadDashboard(t *testing.T, db *sql.DB) dashboardData {
	t.Helper()
	dd, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	return dd
}
