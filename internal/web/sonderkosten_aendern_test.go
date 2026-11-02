package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func postPfad(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, pfad string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, pfad, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestSonderkostenKorrigieren_AktualisiertUndVorbelegt(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	alle, _ := store.AllSonderkosten(db)
	id := alle[0].ID // Wasserhahn 2025, ein anderes Jahr als der Rest

	body := getMitCookies(t, mux, "/sonderkosten?jahr=2025", c).Body.String()
	for _, want := range []string{`action="/sonderkosten/` + strconv.FormatInt(id, 10) + `"`, `value="Wasserhahn"`, `value="184,30"`, `value="2025-03"`, "Korrigieren"} {
		if !strings.Contains(body, want) {
			t.Errorf("Maske nicht vorbelegt: %q fehlt", want)
		}
	}

	f := gueltigesSonderkostenForm()
	f.Set("betreff", "Wasserhahn neu")
	w := postPfad(t, mux, c, "/sonderkosten/"+strconv.FormatInt(id, 10), f)
	if w.Code != http.StatusFound {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	alle, _ = store.AllSonderkosten(db)
	if len(alle) != 4 {
		t.Fatalf("%d Einträge, want 4", len(alle))
	}
	var got *store.Sonderkosten
	for i := range alle {
		if alle[i].ID == id {
			got = &alle[i]
		}
	}
	if got == nil || got.Betreff != "Wasserhahn neu" || got.Betrag != 1234.5 || got.Monat != "2026-10-01" {
		t.Errorf("gespeichert: %+v", got)
	}
}

func TestSonderkostenKorrigieren_UngueltigGibt400UndAendertNichts(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	alle, _ := store.AllSonderkosten(db)
	f := gueltigesSonderkostenForm()
	f.Set("betrag", "0")
	w := postPfad(t, mux, c, "/sonderkosten/"+strconv.FormatInt(alle[0].ID, 10), f)
	if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) == "" {
		t.Errorf("status %d, body %q, want 400 mit Meldung", w.Code, w.Body.String())
	}
	if nach, _ := store.AllSonderkosten(db); nach[0] != alle[0] {
		t.Error("Eintrag wurde trotz Fehler geändert")
	}
	if w := postPfad(t, mux, c, "/sonderkosten/9999", gueltigesSonderkostenForm()); w.Code != http.StatusNotFound {
		t.Errorf("unbekannte id: status %d, want 404", w.Code)
	}
}

func TestSonderkostenLoeschen_MitBestaetigungUndHinweis(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	alle, _ := store.AllSonderkosten(db)
	body := getMitCookies(t, mux, "/sonderkosten", c).Body.String()
	if n := strings.Count(body, `data-bestaetigen-titel="Sonderkosten löschen?"`); n != 3 { // 2026: drei Karten
		t.Errorf("%d Lösch-Formulare, want 3", n)
	}
	if n := strings.Count(body, "data-bestaetigen-zusatz="); n != 2 { // nur die zwei umlegbaren
		t.Errorf("%d Zusatzzeilen, want 2 (nur umlegbare)", n)
	}
	w := postPfad(t, mux, c, "/sonderkosten/"+strconv.FormatInt(alle[0].ID, 10)+"/loeschen", nil)
	if w.Code != http.StatusFound {
		t.Fatalf("status %d", w.Code)
	}
	if nach, _ := store.AllSonderkosten(db); len(nach) != 3 {
		t.Errorf("%d Einträge, want 3", len(nach))
	}
}

func TestSonderkostenKorrigierenLoeschen_NurVermieter(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten...)
	m := loginCookies(t, mux, "mieter", "mieter")
	alle, _ := store.AllSonderkosten(db)
	id := strconv.FormatInt(alle[0].ID, 10)
	for _, pfad := range []string{"/sonderkosten/" + id, "/sonderkosten/" + id + "/loeschen"} {
		if w := postPfad(t, mux, m, pfad, gueltigesSonderkostenForm()); w.Code == http.StatusFound || w.Code == http.StatusOK {
			t.Errorf("%s als Mieter: status %d", pfad, w.Code)
		}
	}
	if nach, _ := store.AllSonderkosten(db); len(nach) != 4 || nach[0] != alle[0] {
		t.Error("Mieter hat Daten verändert")
	}
	if body := getMitCookies(t, mux, "/sonderkosten", m).Body.String(); strings.Contains(body, "Korrigieren") || strings.Contains(body, "/loeschen") {
		t.Error("Mieter sieht Korrigieren/Löschen")
	}
}
