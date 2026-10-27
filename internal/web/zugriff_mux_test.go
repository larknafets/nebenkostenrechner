package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewMux_OffeneRoutenAntwortenWieVorher(t *testing.T) {
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}
	if w := do(http.MethodGet, "/", ""); w.Code != http.StatusFound || w.Header().Get("Location") != "/dashboard" {
		t.Errorf("GET /: %d %q", w.Code, w.Header().Get("Location"))
	}
	w := do(http.MethodPost, "/login", "rolle=vermieter&password=falsch&next=/abrechnung")
	if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "login=1") || !strings.Contains(w.Header().Get("Location"), "fehler=1") {
		t.Errorf("wrong password: %d %q", w.Code, w.Header().Get("Location"))
	}
	w = do(http.MethodPost, "/login", "rolle=vermieter&password=geheim")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/dashboard" || len(w.Result().Cookies()) == 0 {
		t.Errorf("login: %d %q, %d cookies", w.Code, w.Header().Get("Location"), len(w.Result().Cookies()))
	}
	w = do(http.MethodPost, "/logout", "")
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/dashboard" {
		t.Errorf("logout: %d %q", w.Code, w.Header().Get("Location"))
	}
	for _, c := range w.Result().Cookies() {
		if c.MaxAge >= 0 {
			t.Errorf("logout leaves cookie %s alive", c.Name)
		}
	}
	if w := do(http.MethodGet, "/update-check", ""); w.Code != http.StatusOK {
		t.Errorf("GET /update-check: %d", w.Code)
	}
}

// TestNewMux_LadefehlerDerBerechtigungen is the one deliberate change of
// behaviour (#321): a gate answers 500 instead of silent defaults, the
// navigation of a page without gate renders without rights.
func TestNewMux_LadefehlerDerBerechtigungen(t *testing.T) {
	logbuf := logAufnehmen(t)
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	db := openTestDB(t)
	if _, err := db.Exec(`DROP TABLE berechtigungen`); err != nil {
		t.Fatal(err)
	}
	mux := NewMux(db, openTestDB(t), "", "")

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fixkosten", nil))
	if w.Code != http.StatusInternalServerError {
		t.Errorf("GET /fixkosten with a load error: status %d, want 500", w.Code)
	}

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/berechnungslogik", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /berechnungslogik with a load error: status %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, link := range []string{`>Ablesungen</a>`, `>Fixkosten</a>`, `>Stammdaten</a>`, `>Abrechnung</a>`} {
		if strings.Contains(body, link) {
			t.Errorf("the navigation links %s despite the load error", link)
		}
	}
	if !strings.Contains(logbuf.String(), "berechtigungen: ") {
		t.Errorf("log %q has no line for the load error", logbuf.String())
	}
}
