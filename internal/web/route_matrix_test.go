package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// routeMatrix is candidate 3 from the architecture review after demo mode
// (#118): a hand-maintained enforcement matrix (Ticket #112) that checks
// NewMux's actual behavior - an accidentally missing a.RequireLogin(...) on
// a mutating route would otherwise stay invisible (the page just keeps
// working, only now for visitors who aren't logged in either - a silent
// security hole, not a crash, unlike a missing withDB, which panics
// immediately and therefore needs no test of its own).
//
// Only covers RequireLogin, not withDB. Also does NOT automatically detect
// a brand-new route forgotten in NewMux - whoever adds or renames a route
// has to update this table by hand; Go's http.ServeMux offers no public API
// to cross-check registered patterns against.
var routeMatrix = []struct {
	method string
	path   string
	gated  bool
}{
	{"GET", "/ablesungen", false},
	{"GET", "/ablesungen/export.csv", true},
	// /ablesungen/neu and POST /ablesungen are deliberately ungated -
	// creating a new meter reading is possible without being logged in.
	{"GET", "/ablesungen/neu", false},
	{"POST", "/ablesungen", false},
	{"POST", "/ablesungen/import", true},
	{"GET", "/ablesungen/1", false},
	// GET .../bearbeiten and POST /ablesungen/1 are deliberately missing
	// here - since Ticket #130 their gating depends on the target reading's
	// completeness state (requireLoginUnlessTeilstand), so it can no longer
	// be expressed as a fixed gated bool. See
	// TestBearbeitenUndUpdate_TeilstandUngatedVollstaendigGated below.
	{"POST", "/ablesungen/1/loeschen", true},
	{"GET", "/dashboard", false},
	{"GET", "/berechnungslogik", false},
	{"GET", "/stammdaten", false},
	{"POST", "/stammdaten", true},
	{"GET", "/fixkosten", false},
	{"GET", "/fixkosten/export.csv", true},
	{"GET", "/fixkosten/neu", true},
	{"POST", "/fixkosten", true},
	{"POST", "/fixkosten/import", true},
	{"GET", "/fixkosten/1", false},
	{"GET", "/fixkosten/1/bearbeiten", true},
	{"POST", "/fixkosten/1", true},
	{"POST", "/fixkosten/1/loeschen", true},
}

// TestRouteMatrix_RequireLoginCoverage drives every route in routeMatrix
// without a session cookie - RequireLogin checks isLoggedIn before the
// actual handler (form parsing, DB access) even runs, so an empty POST body
// is enough to reliably check a gated route's redirect without creating any
// real data.
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
// #130's state-dependent gating for GET .../bearbeiten and POST
// /ablesungen/{id} - the one part of routeMatrix that can no longer be
// expressed as a fixed gated bool, because it depends on the target
// reading's completeness state instead of just the path.
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

// TestBearbeitenUndUpdate_NoPassword_NichtsAendertSich verifies AC3: without
// LOGIN_PASSWORD set, everything stays open regardless of completeness
// state - requireLoginUnlessTeilstand must not introduce a restriction of
// its own here.
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
