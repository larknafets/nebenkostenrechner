package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_hinweis_test.go: the notes when deleting or changing a
// Fixkosten-Eingabe (#367, Spec #359 step 3).

// hinweisFixkosten is a complete Eingabe (all four prices): only then are the
// Ablesungen of its month berechenbar.
func hinweisFixkosten(monat string) store.FixkostenInput {
	in := seedFixkostenInput(monat)
	in.Strompreis = store.Float64(0.22)
	in.FrischwasserPreis = store.Float64(1.46)
	in.AbwasserPreis = store.Float64(4.87)
	in.EinspeisungPreis = store.Float64(0.08)
	return in
}

// hinweisSeed: Ausgangsstand in June, two Ablesungen in July (affected) and a
// Fixkosten-Eingabe for July. Returns the Eingabe id.
func hinweisSeed(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	for _, datum := range []string{"2026-06-30", "2026-07-10", "2026-07-20"} {
		in := seedPeriodInputAt(datum)
		if datum != "2026-06-30" {
			in.Monat = "2026-07-01"
		}
		if _, err := store.CreatePeriod(db, in); err != nil {
			t.Fatalf("CreatePeriod %s: %v", datum, err)
		}
	}
	id, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-07-01"))
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	return id
}

func fixkostenListeHTML(t *testing.T, db *sql.DB, z *Zugang, query string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/fixkosten"+query, nil)
	w := httptest.NewRecorder()
	handleFixkostenListe(z)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func fixkostenPost(t *testing.T, db *sql.DB, h http.HandlerFunc, pfad string, id int64, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, pfad, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	h(w, requestWithDB(req, db))
	return w
}

func TestFixkostenDetail_LoeschenBestaetigungNenntBetroffeneAblesungen(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)
	body := fixkostenDetailHTML(t, db, newZugang("", nil), id)

	for _, want := range []string{"10.07.2026", "20.07.2026", "Juli 2026 neu erfasst"} {
		if !strings.Contains(body, want) {
			t.Errorf("Löschen-Bestätigung ohne %q", want)
		}
	}
	if strings.Contains(body, "30.06.2026") {
		t.Error("Ausgangsstand ist nicht betroffen, steht aber in der Bestätigung")
	}
}

func TestFixkostenDetail_OhneBetroffeneAblesungenKeineZusatzBestaetigung(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-07-01"))
	if err != nil {
		t.Fatal(err)
	}
	body := fixkostenDetailHTML(t, db, newZugang("", nil), id)
	if strings.Contains(body, "unvollständig, bis") {
		t.Error("Zusatz-Bestätigung ohne betroffene Ablesungen")
	}
	if n := strings.Count(body, "confirm("); n != 1 {
		t.Errorf("confirm( kommt %d mal vor, want 1", n)
	}
}

func TestFixkostenLoeschen_UnvollstaendigeEingabeKeineBestaetigungAberBanner(t *testing.T) {
	db := openTestDB(t)
	hinweisSeed(t, db)
	// Replace the complete Eingabe by an incomplete one: the Ablesungen were
	// not berechenbar, so there is no extra question before deleting it.
	alle, _ := store.AllFixkostenEingaben(db)
	if err := store.DeleteFixkostenEingabe(db, alle[0].ID); err != nil {
		t.Fatal(err)
	}
	unvollstaendig := hinweisFixkosten("2026-07-01")
	unvollstaendig.Strompreis = nil
	id, err := store.CreateFixkostenEingabe(db, unvollstaendig)
	if err != nil {
		t.Fatal(err)
	}
	if body := fixkostenDetailHTML(t, db, newZugang("", nil), id); strings.Contains(body, "unvollständig, bis") {
		t.Error("Zusatz-Bestätigung beim Löschen einer unvollständigen Eingabe")
	}
	// The target page derives the note from the data: with the incomplete
	// Eingabe still there it links to its edit page.
	body := fixkostenListeHTML(t, db, newZugang("", nil), "?geloescht=2026-07-01")
	for _, want := range []string{"Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig).", "10.07.2026", fmt.Sprintf(`href="/fixkosten/%d/bearbeiten"`, id)} {
		if !strings.Contains(body, want) {
			t.Errorf("Hinweis bei unvollständiger Eingabe ohne %q", want)
		}
	}
	w := fixkostenPost(t, db, handleDeleteFixkosten(), "/fixkosten/x/loeschen", id, nil)
	if loc := w.Header().Get("Location"); loc != "/fixkosten?geloescht=2026-07-01" {
		t.Errorf("Location = %q, want nur den Monat", loc)
	}
}

func TestFixkostenListe_VollstaendigerMonatKeinHinweis(t *testing.T) {
	db := openTestDB(t)
	hinweisSeed(t, db)
	if body := fixkostenListeHTML(t, db, newZugang("", nil), "?geloescht=2026-07-01"); strings.Contains(body, "fehlen die Fixkosten") {
		t.Error("Hinweis trotz vollständiger Eingabe")
	}
}

func TestFixkostenLoeschen_BannerMitLinkNurFuerVermieter(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)

	w := fixkostenPost(t, db, handleDeleteFixkosten(), "/fixkosten/x/loeschen", id, nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "/fixkosten?geloescht=2026-07-01" {
		t.Fatalf("Location = %q, want nur den Monat", loc)
	}
	query := strings.TrimPrefix(loc, "/fixkosten")

	vermieter := fixkostenListeHTML(t, db, newZugang("", nil), query)
	for _, want := range []string{
		"Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig).",
		"10.07.2026", "20.07.2026", "unvollständig, bis sie erfasst ist",
		`href="/fixkosten/neu"`, "Fixkosten erfassen",
	} {
		if !strings.Contains(vermieter, want) {
			t.Errorf("Banner (Vermieter) ohne %q", want)
		}
	}
	andere := fixkostenListeHTML(t, db, newZugang("pw", nil), query)
	if !strings.Contains(andere, "Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig).") {
		t.Error("Banner fehlt für andere Rollen")
	}
	if strings.Contains(andere, "Fixkosten erfassen") {
		t.Error("Link im Banner für andere Rollen")
	}
}

func TestFixkostenLoeschen_OhneBetroffeneKeinBanner(t *testing.T) {
	db := openTestDB(t)
	id, _ := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-07-01"))
	w := fixkostenPost(t, db, handleDeleteFixkosten(), "/fixkosten/x/loeschen", id, nil)
	if loc := w.Header().Get("Location"); loc != "/fixkosten?geloescht=2026-07-01" {
		t.Errorf("Location = %q, want den Monat auch ohne Ablesungen", loc)
	}
	query := strings.TrimPrefix(w.Header().Get("Location"), "/fixkosten")
	if body := fixkostenListeHTML(t, db, newZugang("", nil), query); strings.Contains(body, "fehlen die Fixkosten") {
		t.Error("Banner ohne betroffene Ablesungen")
	}
}

func TestFixkostenLoeschen_BannerEntfaelltWennMonatWiederErfasst(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)
	w := fixkostenPost(t, db, handleDeleteFixkosten(), "/fixkosten/x/loeschen", id, nil)
	query := strings.TrimPrefix(w.Header().Get("Location"), "/fixkosten")
	if _, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-07-01")); err != nil {
		t.Fatal(err)
	}
	if body := fixkostenListeHTML(t, db, newZugang("", nil), query); strings.Contains(body, "fehlen die Fixkosten") {
		t.Error("Banner trotz neu erfasster Eingabe")
	}
}

func TestFixkostenAendern_MonatswechselHinweisAltInfoNeu(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)
	// A third Ablesung in August (no Eingabe yet) becomes berechenbar.
	aug := seedPeriodInputAt("2026-08-05")
	aug.Monat = "2026-08-01"
	if _, err := store.CreatePeriod(db, aug); err != nil {
		t.Fatal(err)
	}

	form := fixkostenForm("2026-08")
	w := fixkostenPost(t, db, handleUpdateFixkosten(), "/fixkosten/x", id, form)
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if want := fmt.Sprintf("/fixkosten/%d?alt=2026-07-01", id); loc != want {
		t.Fatalf("Location = %q, want %q", loc, want)
	}
	body := fixkostenDetailHTMLQuery(t, db, newZugang("", nil), id, loc)

	for _, want := range []string{
		"Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig).", "10.07.2026", "20.07.2026",
		"Für August 2026 ist die Fixkosten-Eingabe erfasst.", "05.08.2026", "berechenbar",
		"Fixkosten erfassen",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Detail nach Monatswechsel ohne %q", want)
		}
	}
	andere := fixkostenDetailHTMLQuery(t, db, newZugang("pw", nil), id, loc)
	if strings.Contains(andere, "Fixkosten erfassen") {
		t.Error("Link im Hinweis für andere Rollen")
	}
}

func TestFixkostenAendern_OhneMonatswechselKeinHinweis(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)
	w := fixkostenPost(t, db, handleUpdateFixkosten(), "/fixkosten/x", id, fixkostenForm("2026-07"))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d (%s)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); strings.Contains(loc, "?") {
		t.Errorf("Location = %q, want ohne Query", loc)
	}
}

func fixkostenDetailHTMLQuery(t *testing.T, db *sql.DB, z *Zugang, id int64, ziel string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, ziel, nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleFixkostenDetail(z)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}
