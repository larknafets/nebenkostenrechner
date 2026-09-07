package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// routeMatrix is Kandidat 3 aus dem Architecture Review nach dem Demo-Modus
// (#118): eine von Hand gepflegte Durchsetzungs-Matrix (Ticket #112), die
// NewMux's tatsächliches Verhalten gegenprüft - ein versehentlich fehlendes
// a.RequireLogin(...) auf einer mutierenden Route bleibt sonst unsichtbar
// (die Seite funktioniert einfach weiter, nur eben auch für nicht
// eingeloggte Besucher - eine stille Sicherheitslücke, kein Crash, anders
// als ein fehlendes withDB, das sofort panict und deshalb keinen eigenen
// Test braucht).
//
// Deckt nur RequireLogin ab, nicht withDB. Erkennt auch NICHT automatisch
// eine komplett neue, in NewMux vergessene Route - wer eine Route
// hinzufügt oder umbenennt, muss diese Tabelle von Hand mitziehen; Go's
// http.ServeMux bietet keine öffentliche API, um registrierte Patterns
// gegenzuprüfen.
var routeMatrix = []struct {
	method string
	path   string
	gated  bool
}{
	{"GET", "/ablesungen", false},
	{"GET", "/ablesungen/export.csv", true},
	{"GET", "/ablesungen/neu", true},
	{"POST", "/ablesungen", true},
	{"POST", "/ablesungen/import", true},
	{"GET", "/ablesungen/1", false},
	{"GET", "/ablesungen/1/bearbeiten", true},
	{"POST", "/ablesungen/1", true},
	{"POST", "/ablesungen/1/loeschen", true},
	{"GET", "/dashboard", false},
	{"GET", "/berechnungslogik", false},
	{"GET", "/stammdaten", false},
	{"POST", "/stammdaten", true},
	{"GET", "/fixkosten", false},
	{"GET", "/fixkosten/neu", true},
	{"POST", "/fixkosten", true},
	{"GET", "/fixkosten/1", false},
	{"GET", "/fixkosten/1/bearbeiten", true},
	{"POST", "/fixkosten/1", true},
	{"POST", "/fixkosten/1/loeschen", true},
}

// TestRouteMatrix_RequireLoginCoverage treibt jede Route in routeMatrix ohne
// Session-Cookie an - RequireLogin prüft isLoggedIn, bevor der eigentliche
// Handler (Formular-Parsing, DB-Zugriff) überhaupt läuft, ein leerer POST-
// Body reicht also aus, um eine gated Route zuverlässig auf ihren Redirect
// zu prüfen, ohne echte Daten anzulegen.
func TestRouteMatrix_RequireLoginCoverage(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")

	for _, rc := range routeMatrix {
		t.Run(rc.method+" "+rc.path, func(t *testing.T) {
			req := httptest.NewRequest(rc.method, rc.path, nil)
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			bouncedToLogin := w.Code == http.StatusFound && strings.Contains(w.Header().Get("Location"), "login=1")

			if rc.gated && !bouncedToLogin {
				t.Errorf("erwartet Login-Redirect (gated), bekam status=%d location=%q", w.Code, w.Header().Get("Location"))
			}
			if !rc.gated && bouncedToLogin {
				t.Errorf("unerwarteter Login-Redirect (sollte ungated sein), location=%q", w.Header().Get("Location"))
			}
		})
	}
}
