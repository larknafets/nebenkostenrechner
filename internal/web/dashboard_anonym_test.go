package web

import (
	"strings"
	"testing"
)

// Tests for #290: pin the anonymous Dashboard (LOGIN_PASSWORD set, no login).
// Only Wohnung 2 is visible; Wohnung 1, Wallboxen, PV-Anlage and
// Mieteinnahmen are reserved for logged-in users. The tests document the
// behaviour as it is today and are the safety net for the status rework.

func anonymesDashboardSetup(t *testing.T) (anon, eingeloggt string) {
	t.Helper()
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)
	mieteEingabe(t, db, "2026-11-01", map[int64]float64{1: 300, 2: 712.34})

	anon = dashboardHTMLMitAuth(t, db, newZugang("geheim", nil))
	// Without LOGIN_PASSWORD everyone counts as logged in.
	eingeloggt = dashboardHTMLMitAuth(t, db, newZugang("", nil))
	return anon, eingeloggt
}

func TestDashboard_AnonymNurWohnung2(t *testing.T) {
	anon, _ := anonymesDashboardSetup(t)

	if !strings.Contains(anon, "<strong>Wohnung 2</strong>") {
		t.Fatal("Wohnung 2 fehlt für Anonyme")
	}
	if !strings.Contains(anon, "<strong>Wärmepumpe</strong>") {
		t.Error("Wärmepumpe fehlt für Anonyme")
	}
	// The Wärmepumpe has a tab and a Monatsverlauf panel, too (#305).
	for _, s := range []string{`<option value="waermepumpe">Wärmepumpe</option>`, `<div class="wohnung-panel" data-wohnung="waermepumpe">`} {
		if !strings.Contains(anon, s) {
			t.Errorf("%q fehlt für Anonyme", s)
		}
	}
	// The bare text "Wohnung 1" also occurs in the static Fixkosten legend,
	// so the check targets the card and the tab of Wohnung 1.
	for _, s := range []string{
		"<strong>Wohnung 1</strong>", `data-wohnung="1"`, `<option value="1">`,
		"Wallboxen", "PV-Anlage", labelMiete, "712,34",
	} {
		if strings.Contains(anon, s) {
			t.Errorf("%q im HTML ohne Login", s)
		}
	}
}

func TestDashboard_EingeloggtZeigtAlles(t *testing.T) {
	_, voll := anonymesDashboardSetup(t)

	for _, s := range []string{
		"<strong>Wohnung 1</strong>", "<strong>Wohnung 2</strong>",
		`data-wohnung="1"`, `<option value="1">Wohnung 1</option>`,
		"<strong>Wallboxen</strong>", "<strong>Wärmepumpe</strong>", "<strong>PV-Anlage</strong>",
		`<option value="waermepumpe">Wärmepumpe</option>`, `data-wohnung="waermepumpe"`,
		labelMiete, "712,34",
	} {
		if !strings.Contains(voll, s) {
			t.Errorf("%q fehlt im HTML mit Login", s)
		}
	}
}
