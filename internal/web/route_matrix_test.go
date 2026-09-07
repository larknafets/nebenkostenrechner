package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
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
	// /ablesungen/neu und POST /ablesungen bewusst ungated - eine neue
	// Ablesung anlegen ist auch nicht eingeloggt möglich.
	{"GET", "/ablesungen/neu", false},
	{"POST", "/ablesungen", false},
	{"POST", "/ablesungen/import", true},
	{"GET", "/ablesungen/1", false},
	// GET .../bearbeiten und POST /ablesungen/1 fehlen hier bewusst - ihr
	// Gating hängt seit Ticket #130 vom Vollständigkeits-Zustand der
	// Ziel-Ablesung ab (requireLoginUnlessTeilstand), lässt sich also nicht
	// mehr als fixer gated-bool ausdrücken. Siehe
	// TestBearbeitenUndUpdate_TeilstandUngatedVollstaendigGated unten.
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

// bouncedToLogin mirrors TestRouteMatrix_RequireLoginCoverage's own check -
// factored out so the Teilstand-Gating-Tests below can reuse it.
func bouncedToLogin(w *httptest.ResponseRecorder) bool {
	return w.Code == http.StatusFound && strings.Contains(w.Header().Get("Location"), "login=1")
}

// TestBearbeitenUndUpdate_TeilstandUngatedVollstaendigGated covers Ticket
// #130's zustandsabhängiges Gating für GET .../bearbeiten und POST
// /ablesungen/{id} - der einzige Teil von routeMatrix, der sich nicht mehr
// als fixer gated-bool ausdrücken lässt, weil er vom Vollständigkeits-
// Zustand der jeweiligen Ziel-Ablesung abhängt statt nur vom Pfad.
func TestBearbeitenUndUpdate_TeilstandUngatedVollstaendigGated(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	db := openTestDB(t)
	mux := NewMux(db, openTestDB(t), "", "")

	teilstandID, err := store.CreatePeriod(db, store.PeriodInput{
		ReadingDate:             "2026-11-01",
		HeizungWaermeGewichtung: 0.7,
	})
	if err != nil {
		t.Fatalf("CreatePeriod (Teilstand): %v", err)
	}
	vollstaendigID, err := store.CreatePeriod(db, seedPeriodInputAt("2026-12-01"))
	if err != nil {
		t.Fatalf("CreatePeriod (vollständig): %v", err)
	}

	t.Run("GET bearbeiten Teilstand ungated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ablesungen/"+strconv.FormatInt(teilstandID, 10)+"/bearbeiten", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if bouncedToLogin(w) {
			t.Errorf("unerwarteter Login-Redirect für einen Teilstand, location=%q", w.Header().Get("Location"))
		}
	})

	t.Run("POST update Teilstand ungated", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/ablesungen/"+strconv.FormatInt(teilstandID, 10), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if bouncedToLogin(w) {
			t.Errorf("unerwarteter Login-Redirect für einen Teilstand, location=%q", w.Header().Get("Location"))
		}
	})

	t.Run("GET bearbeiten vollständige Ablesung gated", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ablesungen/"+strconv.FormatInt(vollstaendigID, 10)+"/bearbeiten", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if !bouncedToLogin(w) {
			t.Errorf("erwartet Login-Redirect für eine vollständige Ablesung, bekam status=%d location=%q", w.Code, w.Header().Get("Location"))
		}
	})

	t.Run("POST update vollständige Ablesung gated", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/ablesungen/"+strconv.FormatInt(vollstaendigID, 10), nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if !bouncedToLogin(w) {
			t.Errorf("erwartet Login-Redirect für eine vollständige Ablesung, bekam status=%d location=%q", w.Code, w.Header().Get("Location"))
		}
	})

	t.Run("GET bearbeiten nicht existierende Ablesung 404 statt Login-Redirect", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ablesungen/999999/bearbeiten", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d (kein Login-Redirect für nicht existierende ID)", w.Code, http.StatusNotFound)
		}
	})

	t.Run("POST update nicht existierende Ablesung 404 statt Login-Redirect", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/ablesungen/999999", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d (kein Login-Redirect für nicht existierende ID)", w.Code, http.StatusNotFound)
		}
	})
}

// TestBearbeitenUndUpdate_NoPassword_NichtsAendertSich verifiziert AC3:
// ohne gesetztes LOGIN_PASSWORD bleibt alles offen, unabhängig vom
// Vollständigkeits-Zustand - requireLoginUnlessTeilstand darf hier keine
// eigene Einschränkung einführen.
func TestBearbeitenUndUpdate_NoPassword_NichtsAendertSich(t *testing.T) {
	db := openTestDB(t)
	mux := NewMux(db, openTestDB(t), "", "")

	vollstaendigID, err := store.CreatePeriod(db, seedPeriodInputAt("2026-12-01"))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	req := httptest.NewRequest("GET", "/ablesungen/"+strconv.FormatInt(vollstaendigID, 10)+"/bearbeiten", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d (kein LOGIN_PASSWORD gesetzt, bleibt offen)", w.Code, http.StatusOK)
	}
}
