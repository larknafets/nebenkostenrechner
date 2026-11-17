package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Lists, Stammdaten and text pages sit in the narrow column (Ticket #489).
func TestListenseiten_SchmaleSpalte(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	cookies := demoLoginCookies(t, mux)
	for _, pfad := range []string{"/ablesungen", "/fixkosten", "/sonderkosten", "/stammdaten", "/berechnungslogik", "/ablesungen/neu", "/fixkosten/neu"} {
		req := httptest.NewRequest(http.MethodGet, pfad, nil)
		attachCookies(req, cookies)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s: status %d", pfad, w.Code)
		}
		if !strings.Contains(w.Body.String(), `class="liste-schmal"`) {
			t.Errorf("GET %s: no liste-schmal wrapper", pfad)
		}
	}
}
