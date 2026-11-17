package web

import (
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Wohnung cards and house cards sit in separate rows, the Monatsübersicht
// switches by tabs (buttons in a group) instead of a select box.
func TestDashboard_ReihenUndReiter(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	body := dashboardHTML(t, db)

	wohnungen, rest, ok := strings.Cut(body, `class="grid dash-haus"`)
	if !ok {
		t.Fatal("Reihe der Hauskarten fehlt")
	}
	haus, _, _ := strings.Cut(rest, `class="btn-row"`)
	if !strings.Contains(wohnungen, `class="grid dash-wohnungen"`) {
		t.Error("Reihe der Wohnungskarten fehlt vor den Hauskarten")
	}
	for _, s := range []string{"<strong>Wohnung 1</strong>", "<strong>Wohnung 2</strong>"} {
		if !strings.Contains(wohnungen, s) || strings.Contains(haus, s) {
			t.Errorf("%s gehört in die Wohnungsreihe", s)
		}
	}
	for _, s := range []string{"<strong>Wallboxen</strong>", "<strong>Wärmepumpe</strong>", "<strong>PV-Anlage</strong>"} {
		if !strings.Contains(haus, s) || strings.Contains(wohnungen, s) {
			t.Errorf("%s gehört in die Hausreihe", s)
		}
	}

	if !strings.Contains(body, `id="wohnung-select"`) || strings.Contains(body, "dash-tabs") {
		t.Error("Monatsübersicht braucht das Auswahlfeld und keine Reiter")
	}
	for _, id := range []string{"1", "2", "waermepumpe"} {
		if !strings.Contains(body, `<option value="`+id+`">`) {
			t.Errorf("Auswahl %q fehlt", id)
		}
	}
}
