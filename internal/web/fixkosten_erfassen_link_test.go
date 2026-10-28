package web

import (
	"fmt"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_erfassen_link_test.go: "Fixkosten erfassen" leads to the edit page
// of an incomplete Eingabe of the month, to the new form without one (#359,
// Story 13 and 14).

func TestFixkostenFehlenSatz(t *testing.T) {
	for _, tc := range []struct {
		monate []string
		want   string
	}{
		{[]string{"2026-07-01"}, "Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig)."},
		{[]string{"2026-06-01", "2026-08-01"}, "Für Juni 2026 und August 2026 fehlen die Fixkosten (oder sind unvollständig)."},
		{[]string{"2026-06-01", "2026-07-01", "2026-08-01"}, "Für Juni 2026, Juli 2026 und August 2026 fehlen die Fixkosten (oder sind unvollständig)."},
	} {
		if got := fixkostenFehlenSatz(tc.monate); got != tc.want {
			t.Errorf("fixkostenFehlenSatz(%v) = %q, want %q", tc.monate, got, tc.want)
		}
	}
}

func TestFixkostenErfassenPfad(t *testing.T) {
	if got := fixkostenErfassenPfad(0); got != "/fixkosten/neu" {
		t.Errorf("ohne Eingabe = %q", got)
	}
	if got := fixkostenErfassenPfad(7); got != "/fixkosten/7/bearbeiten" {
		t.Errorf("mit Eingabe = %q", got)
	}
}

func TestAblesungDetail_FixkostenErfassenLinkJeFall(t *testing.T) {
	db := openTestDB(t)
	_, nov, _ := ablesungenMitOffenemMonat(t, db)
	z := newZugang("", nil)

	// November has no Eingabe: banner and hint lead to the new form.
	body := detailBodyMit(t, z, db, nov, "?fixkosten_fehlen=1")
	if n := strings.Count(body, `href="/fixkosten/neu">Fixkosten erfassen</a>`); n != 2 {
		t.Errorf("Link zum Neuformular %d mal, want 2 (Banner und Hinweis)", n)
	}

	// An incomplete Eingabe of November: both lead to its edit page.
	id, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-11-01"))
	if err != nil {
		t.Fatal(err)
	}
	body = detailBodyMit(t, z, db, nov, "?fixkosten_fehlen=1")
	want := fmt.Sprintf(`href="/fixkosten/%d/bearbeiten">Fixkosten erfassen</a>`, id)
	if n := strings.Count(body, want); n != 2 {
		t.Errorf("Link zur Bearbeiten-Seite %d mal, want 2 (Banner und Hinweis)", n)
	}
	if strings.Contains(body, `href="/fixkosten/neu"`) {
		t.Error("Link zum Neuformular trotz vorhandener Eingabe")
	}
}

func TestOffenerHinweis_PfadJeFall(t *testing.T) {
	for name, tc := range map[string]struct {
		offen []calc.OffenerMonat
		want  string
	}{
		"keine Eingabe":            {[]calc.OffenerMonat{{Monat: "2026-10-01"}}, "/fixkosten/neu"},
		"eine unvollständige":      {[]calc.OffenerMonat{{Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten/7/bearbeiten"},
		"mehrere unvollständige":   {[]calc.OffenerMonat{{Monat: "2026-10-01", EingabeID: 6}, {Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten"},
		"eine ohne, eine mit":      {[]calc.OffenerMonat{{Monat: "2026-10-01"}, {Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten/neu"},
		"unvollständige und keine": {[]calc.OffenerMonat{{Monat: "2026-10-01", EingabeID: 6}, {Monat: "2026-11-01"}}, "/fixkosten/neu"},
	} {
		if got := offenerHinweis(tc.offen).Pfad; got != tc.want {
			t.Errorf("%s: Pfad = %q, want %q", name, got, tc.want)
		}
	}
}

func TestDashboard_BannerLinkZurUnvollstaendigenEingabe(t *testing.T) {
	db := openTestDB(t)
	id := offeneMonateSetup(t, db)
	// September and October get a complete Eingabe, so November (incomplete) is
	// the only open month.
	if _, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-09-01")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	body := dashboardHTML(t, db)
	want := fmt.Sprintf(`href="/fixkosten/%d/bearbeiten">Fixkosten erfassen</a>`, id)
	if !strings.Contains(body, want) {
		t.Errorf("Banner ohne %q", want)
	}
}
