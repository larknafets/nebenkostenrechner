package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_erfassen_link_test.go: "Fixkosten erfassen" leads to the edit page
// of an incomplete Eingabe of the month, to the new form without one (#359,
// Story 13 and 14).

func TestFixkostenFehlenHinweis_TextJeKontext(t *testing.T) {
	ps := []calc.Ablesung{{Period: &store.LatestPeriod{ReadingDate: "2026-07-10"}}}
	juli := []calc.OffenerMonat{{Monat: "2026-07-01"}}
	satz := "Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig). "
	for name, tc := range map[string]struct {
		kontext fixkostenKontext
		monate  []calc.OffenerMonat
		want    string
	}{
		"Dashboard":  {kontextDashboard, juli, satz + "Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht."},
		"Ablesung":   {kontextAblesung, juli, satz + "Die Ablesung ist noch nicht berechnet, bis die Fixkosten erfasst sind."},
		"CSV-Import": {kontextCSVImport, juli, satz + "Die Ablesungen dieser Monate sind noch nicht berechnet, bis die Fixkosten erfasst sind."},
		"Löschen":    {kontextLoeschen, juli, satz + "Die Ablesung vom 10.07.2026 ist unvollständig, bis sie erfasst ist."},
		"mehrere": {kontextAblesung, []calc.OffenerMonat{{Monat: "2026-06-01"}, {Monat: "2026-08-01"}},
			"Für Juni 2026 und August 2026 fehlen die Fixkosten (oder sind unvollständig). Die Ablesung ist noch nicht berechnet, bis die Fixkosten erfasst sind."},
	} {
		if got := fixkostenFehlenHinweis(tc.kontext, tc.monate, ps).Text; got != tc.want {
			t.Errorf("%s: Text = %q, want %q", name, got, tc.want)
		}
	}
	if fixkostenFehlenHinweis(kontextDashboard, nil, nil) != nil {
		t.Error("ohne Monat: Hinweis, want nil")
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

	// November has no Eingabe: the hint leads to the new form, the banner
	// stays away (one note per page).
	body := detailBodyMit(t, z, db, nov, "?fixkosten_fehlen=1")
	if n := strings.Count(body, `href="/fixkosten/neu">Fixkosten erfassen</a>`); n != 1 {
		t.Errorf("Link zum Neuformular %d mal, want 1 (nur der Hinweis)", n)
	}

	// An incomplete Eingabe of November: both lead to its edit page.
	id, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-11-01"))
	if err != nil {
		t.Fatal(err)
	}
	body = detailBodyMit(t, z, db, nov, "?fixkosten_fehlen=1")
	want := fmt.Sprintf(`href="/fixkosten/%d/bearbeiten">Fixkosten ergänzen</a>`, id)
	if n := strings.Count(body, want); n != 1 {
		t.Errorf("Link zur Bearbeiten-Seite %d mal, want 1 (nur der Hinweis)", n)
	}
	if strings.Contains(body, `href="/fixkosten/neu"`) {
		t.Error("Link zum Neuformular trotz vorhandener Eingabe")
	}
}

func TestFixkostenFehlenHinweis_PfadUndLinktextJeFall(t *testing.T) {
	for name, tc := range map[string]struct {
		offen    []calc.OffenerMonat
		pfad     string
		linkText string
	}{
		"keine Eingabe":            {[]calc.OffenerMonat{{Monat: "2026-10-01"}}, "/fixkosten/neu", "Fixkosten erfassen"},
		"eine unvollständige":      {[]calc.OffenerMonat{{Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten/7/bearbeiten", "Fixkosten ergänzen"},
		"mehrere unvollständige":   {[]calc.OffenerMonat{{Monat: "2026-10-01", EingabeID: 6}, {Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten", "Fixkosten ergänzen"},
		"eine ohne, eine mit":      {[]calc.OffenerMonat{{Monat: "2026-10-01"}, {Monat: "2026-11-01", EingabeID: 7}}, "/fixkosten/neu", "Fixkosten erfassen"},
		"unvollständige und keine": {[]calc.OffenerMonat{{Monat: "2026-10-01", EingabeID: 6}, {Monat: "2026-11-01"}}, "/fixkosten/neu", "Fixkosten erfassen"},
	} {
		h := fixkostenFehlenHinweis(kontextDashboard, tc.offen, nil)
		if h.Pfad != tc.pfad || h.LinkText != tc.linkText {
			t.Errorf("%s: Pfad, Linktext = %q, %q, want %q, %q", name, h.Pfad, h.LinkText, tc.pfad, tc.linkText)
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
	want := fmt.Sprintf(`href="/fixkosten/%d/bearbeiten">Fixkosten ergänzen</a>`, id)
	if !strings.Contains(body, want) {
		t.Errorf("Banner ohne %q", want)
	}
}

// Teilstand and first Ablesung: the Kostenbereich shows no note, so the banner
// after saving does (once), with the link text following the state of the
// Eingabe.
func TestAblesungDetail_BannerBeiTeilstandUndErsterAblesung(t *testing.T) {
	db := openTestDB(t)
	erste, err := store.CreatePeriod(db, seedPeriodInputAt("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	teil := seedPeriodInputAt("2026-11-01")
	delete(teil.Readings, "strom_gesamt")
	teilID, err := store.CreatePeriod(db, teil)
	if err != nil {
		t.Fatal(err)
	}
	z := newZugang("", nil)
	for name, id := range map[string]int64{"erste Ablesung": erste, "Teilstand": teilID} {
		body := detailBodyMit(t, z, db, id, "?fixkosten_fehlen=1")
		if n := strings.Count(body, "fehlen die Fixkosten"); n != 1 {
			t.Errorf("%s: %d Hinweise ohne Eingabe, want 1", name, n)
		}
		if !strings.Contains(body, `href="/fixkosten/neu">Fixkosten erfassen</a>`) {
			t.Errorf("%s: Link zum Neuformular fehlt", name)
		}
		if strings.Contains(detailBodyMit(t, z, db, id, ""), "fehlen die Fixkosten") {
			t.Errorf("%s: Hinweis ohne frisches Speichern", name)
		}
	}
	// Incomplete Eingabe of November: banner leads to its edit page.
	eid, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-11-01"))
	if err != nil {
		t.Fatal(err)
	}
	body := detailBodyMit(t, z, db, teilID, "?fixkosten_fehlen=1")
	want := fmt.Sprintf(`href="/fixkosten/%d/bearbeiten">Fixkosten ergänzen</a>`, eid)
	if n := strings.Count(body, "fehlen die Fixkosten"); n != 1 || !strings.Contains(body, want) {
		t.Errorf("Teilstand mit unvollständiger Eingabe: %d Hinweise, Link %q fehlt", n, want)
	}
	// A complete Eingabe: no note.
	if _, err := store.CreateFixkostenEingabe(db, fixkostenMitPreisen("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(detailBodyMit(t, z, db, erste, "?fixkosten_fehlen=1"), "fehlen die Fixkosten") {
		t.Error("erste Ablesung: Hinweis trotz vollständiger Eingabe")
	}
}

// Saving a Teilstand (or the first Ablesung) redirects with the flag whenever
// the month has no complete Eingabe.
func TestAblesungSpeichern_FlaggeJeEingabeZustand(t *testing.T) {
	db := openTestDB(t)
	flag := func() bool {
		w := httptest.NewRecorder()
		form := vollesFormular()
		form.Del("strom_gesamt") // Teilstand
		handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
		if w.Code != http.StatusFound {
			t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
		}
		return strings.Contains(w.Header().Get("Location"), "?fixkosten_fehlen=1")
	}
	if !flag() {
		t.Error("Teilstand ohne Eingabe: Flagge fehlt")
	}
	if _, err := store.CreateFixkostenEingabe(db, fixkostenMitPreisen("2026-11-01")); err != nil {
		t.Fatal(err)
	}
	if flag() {
		t.Error("Teilstand mit vollständiger Eingabe: Flagge gesetzt")
	}
}
