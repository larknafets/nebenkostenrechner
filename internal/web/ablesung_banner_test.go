package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// detailBodyMit renders the Ablesung page with query and the given Zugang.
func detailBodyMit(t *testing.T, z *Zugang, db *sql.DB, id int64, query string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10)+query, nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleAblesungDetail(z)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// TestAblesung_BannerNachDemSpeichernOhneFixkosten: saving an Ablesung whose
// Abrechnungsmonat has no Fixkosten-Eingabe leads to its page with a banner,
// the link "Fixkosten erfassen" only for the Vermieter (#359).
func TestAblesung_BannerNachDemSpeichernOhneFixkosten(t *testing.T) {
	db := openTestDB(t)
	form := vollesFormular()
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "?fixkosten_fehlen=1") {
		t.Fatalf("Location = %q, want the flag fixkosten_fehlen", loc)
	}
	neueste, err := store.GetLatestPeriod(db)
	if err != nil || neueste == nil {
		t.Fatalf("latest period = %v, %v", neueste, err)
	}

	const text = "Für November 2026 fehlen die Fixkosten"
	vermieter := detailBodyMit(t, newZugang("", nil), db, neueste.ID, "?fixkosten_fehlen=1")
	if !strings.Contains(vermieter, text) || !strings.Contains(vermieter, ">Fixkosten erfassen</a>") {
		t.Errorf("Vermieter: Banner mit Link fehlt")
	}
	mieter := detailBodyMit(t, newZugang("geheim", nil), db, neueste.ID, "?fixkosten_fehlen=1")
	if !strings.Contains(mieter, text) || strings.Contains(mieter, "Fixkosten erfassen") {
		t.Errorf("ohne Anmeldung: Banner nur mit Text erwartet")
	}
	if ohne := detailBodyMit(t, newZugang("", nil), db, neueste.ID, ""); strings.Contains(ohne, text) {
		t.Error("Banner ohne frisches Speichern")
	}

	// A month with its Fixkosten gets no banner, neither on the page nor as
	// the redirect after changing the Ablesung.
	if _, err := store.CreateFixkostenEingabe(db, fixkostenMitPreisen("2026-11-01")); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	if body := detailBodyMit(t, newZugang("", nil), db, neueste.ID, "?fixkosten_fehlen=1"); strings.Contains(body, text) {
		t.Error("Banner trotz Fixkosten-Eingabe des Monats")
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ablesungen/"+strconv.FormatInt(neueste.ID, 10), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(neueste.ID, 10))
	handleUpdateAblesung()(w, requestWithDB(req, db))
	if loc := w.Header().Get("Location"); w.Code != http.StatusFound || strings.Contains(loc, "fixkosten_fehlen") {
		t.Errorf("Ändern mit Fixkosten: %d %q, want 302 without flag", w.Code, loc)
	}
}

// TestAblesung_AendernOhneFixkostenZeigtBanner: the redirect after changing an
// Ablesung carries the flag too.
func TestAblesung_AendernOhneFixkostenZeigtBanner(t *testing.T) {
	db := openTestDB(t)
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, vollesFormular()), db))
	neueste, err := store.GetLatestPeriod(db)
	if err != nil || neueste == nil {
		t.Fatalf("latest period = %v, %v", neueste, err)
	}
	req := httptest.NewRequest(http.MethodPost, "/ablesungen/"+strconv.FormatInt(neueste.ID, 10), strings.NewReader(vollesFormular().Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(neueste.ID, 10))
	w = httptest.NewRecorder()
	handleUpdateAblesung()(w, requestWithDB(req, db))
	if loc := w.Header().Get("Location"); w.Code != http.StatusFound || !strings.Contains(loc, "?fixkosten_fehlen=1") {
		t.Errorf("Ändern ohne Fixkosten: %d %q, want 302 with flag", w.Code, loc)
	}
}
