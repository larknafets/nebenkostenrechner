package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestCreateAblesung_PlausibilitaetsHinweisNachDemSpeichern checks that the
// Wallbox-Einzelzähler plausibility note (#225) is visible right after
// saving: the POST redirects to the Ablesungsseite, which renders it.
func TestCreateAblesung_PlausibilitaetsHinweisNachDemSpeichern(t *testing.T) {
	cases := []struct {
		name    string
		w1, w2  string
		hinweis bool
	}{
		{"passt", "25", "15", false},
		{"weicht ab", "10", "15", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			first := seedPeriodInputAt("2026-10-01")
			for k := range first.Readings {
				first.Readings[k] = 0
			}
			first.Readings[wbW1], first.Readings[wbW2] = 0, 0
			if _, err := store.CreatePeriod(db, first); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}

			form := vollesFormular()
			for _, key := range store.MeterKeys {
				form.Set(key, "0")
			}
			form.Set("strom_gesamt", "100")
			form.Set("strom_wallbox", "40")
			form.Set(wbW1, c.w1)
			form.Set(wbW2, c.w2)
			w := httptest.NewRecorder()
			handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
			if w.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
			}

			req := httptest.NewRequest(http.MethodGet, w.Header().Get("Location"), nil)
			id := req.URL.Path[strings.LastIndex(req.URL.Path, "/")+1:]
			req.SetPathValue("id", id)
			rec := httptest.NewRecorder()
			handleAblesungDetail(newZugang("", nil))(rec, requestWithDB(req, db))
			if rec.Code != http.StatusOK {
				t.Fatalf("Weiterleitungsziel status = %d, want 200", rec.Code)
			}
			if got := strings.Contains(rec.Body.String(), "Plausibilität Wallbox-Einzelzähler"); got != c.hinweis {
				t.Errorf("Hinweis nach dem Speichern sichtbar = %v, want %v", got, c.hinweis)
			}
		})
	}
}
