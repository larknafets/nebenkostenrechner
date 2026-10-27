package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// TestHandleUpdateStammdaten_UngueltigeEingabe verifies that invalid input is
// answered with 400 and the German message, decided by store.SaveStammdaten
// (the one place for the rules), never with a 500.
func TestHandleUpdateStammdaten_UngueltigeEingabe(t *testing.T) {
	gueltig := func() url.Values {
		return url.Values{
			"qm_1": {"100"}, "flurstueck_groesse_1": {"600"}, "qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
			"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
		}
	}
	cases := []struct {
		name   string
		mutate func(url.Values)
		want   string
	}{
		{"Wohnungsstatus", func(f url.Values) { f.Set("status_2", "leer") }, "ungültiger Wohnungsstatus für Wohnung 2"},
		{"Mieter seit", func(f url.Values) { f.Set("mieter_seit_1", "2026-13") }, "ungültiges Datum für \"Mieter:innen seit\" bei Wohnung 1"},
		{"Heizungs-Gewichtung", func(f url.Values) { f.Set("heizung_gewichtung", "0.8") }, "ungültige Heizungs-Gewichtung \"0.8\" (muss 0.7, 0.6 oder 0.5 sein)"},
		{"Heizungs-Gewichtung keine Zahl", func(f url.Values) { f.Set("heizung_gewichtung", "abc") }, "ungültige Heizungs-Gewichtung \"abc\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			form := gueltig()
			c.mutate(form)
			req := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			handleUpdateStammdaten()(w, requestWithDB(req, db))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), c.want)
			}
		})
	}
}
