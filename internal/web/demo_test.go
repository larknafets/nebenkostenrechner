package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// demoLoginCookies logs in with the given password against mux and returns
// the resulting Set-Cookie values, ready to attach to a follow-up request.
func demoLoginCookies(t *testing.T, mux *http.ServeMux, password string) []*http.Cookie {
	t.Helper()
	form := url.Values{"password": {password}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("login status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	return w.Result().Cookies()
}

func attachCookies(req *http.Request, cookies []*http.Cookie) {
	for _, c := range cookies {
		req.AddCookie(c)
	}
}

// TestHandleLogin_DemoPassword issues the demo login regardless of whether
// LOGIN_PASSWORD (secret) is configured (Issue #118: "funktioniert immer").
func TestHandleLogin_DemoPassword(t *testing.T) {
	for _, secret := range []string{"", "geheim"} {
		t.Run("secret="+secret, func(t *testing.T) {
			cookies := demoLoginCookies(t, NewMux(openTestDB(t), openTestDB(t), "", ""), demoPassword)

			var demoCookie *http.Cookie
			for _, c := range cookies {
				if c.Name == demoSessionCookieName {
					demoCookie = c
				}
			}
			if demoCookie == nil {
				t.Fatalf("no %s cookie set, got %v", demoSessionCookieName, cookies)
			}

			req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
			req.AddCookie(demoCookie)
			if !isDemoSession(req) {
				t.Error("isDemoSession() = false for a freshly issued demo cookie")
			}
			if !isLoggedIn(req, secret) {
				t.Error("isLoggedIn() = false for a demo session")
			}
		})
	}
}

// TestDemoMode_WritesIsolatedFromRealDB is the spec's Kernanforderung
// (Issue #118 Testing Decisions): an Ablesung created in Demo-Modus must
// land exclusively in the Demo-Datenbank, never in the echte.
func TestDemoMode_WritesIsolatedFromRealDB(t *testing.T) {
	realDB := openTestDB(t)
	demoDB := openTestDB(t)
	mux := NewMux(realDB, demoDB, "", "")

	cookies := demoLoginCookies(t, mux, demoPassword)

	// Demo-Login setzt die Demo-DB bereits auf ihren 39-Monats-Ausgangs-
	// zustand zurück (Issue #121) - Baseline hier einmal einlesen, statt
	// die genaue Anzahl zu hardcoden.
	baseline, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB) baseline: %v", err)
	}

	form := periodFormValues("2026-06-15", "2026-06")
	req := httptest.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("create ablesung im Demo-Modus: status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}

	demoPeriods, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB): %v", err)
	}
	if len(demoPeriods) != len(baseline)+1 {
		t.Errorf("len(demoPeriods) = %d, want %d (baseline+1)", len(demoPeriods), len(baseline)+1)
	}

	realPeriods, err := store.AllPeriods(realDB)
	if err != nil {
		t.Fatalf("AllPeriods(realDB): %v", err)
	}
	if len(realPeriods) != 0 {
		t.Errorf("len(realPeriods) = %d, want 0 - Demo-Änderung darf nie in der echten DB landen", len(realPeriods))
	}

	// Ohne Demo-Session (nicht angemeldet, secret == "") bleibt die echte,
	// weiterhin leere Datenbank sichtbar.
	req2 := httptest.NewRequest(http.MethodGet, "/ablesungen", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if strings.Contains(w2.Body.String(), "2026-06-15") {
		t.Error("nicht angemeldete Ansicht zeigt die im Demo-Modus angelegte Ablesung - Datenbanken sind nicht isoliert")
	}
}

// TestDemoLogin_ResetsPreviousDemoSession covers Issue #121's Kernanforderung:
// eine im Demo-Modus angelegte Ablesung ist nach einem erneuten Demo-Login
// nicht mehr vorhanden, ein Reset auf die echte DB hat dabei keinerlei
// Auswirkung.
func TestDemoLogin_ResetsPreviousDemoSession(t *testing.T) {
	realDB := openTestDB(t)
	demoDB := openTestDB(t)
	mux := NewMux(realDB, demoDB, "", "")

	firstSession := demoLoginCookies(t, mux, demoPassword)
	baseline, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB) baseline: %v", err)
	}

	// 2099 liegt garantiert außerhalb des relativ zu "jetzt" generierten
	// 39-Monats-Fensters (Issue #117) - eindeutig als injizierte Änderung
	// erkennbar, im Gegensatz zu einem Monat, der ohnehin Teil der frischen
	// Baseline sein könnte.
	form := periodFormValues("2099-01-15", "2099-01")
	req := httptest.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachCookies(req, firstSession)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("create ablesung in 1. Demo-Session: status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}

	afterFirstSession, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB) nach 1. Session: %v", err)
	}
	if len(afterFirstSession) != len(baseline)+1 {
		t.Fatalf("len(afterFirstSession) = %d, want %d (baseline+1)", len(afterFirstSession), len(baseline)+1)
	}

	// Erneuter Demo-Login (2. Session) muss die Ablesung aus der 1. Session
	// verwerfen und wieder exakt die Baseline zeigen.
	demoLoginCookies(t, mux, demoPassword)

	afterReset, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB) nach Reset: %v", err)
	}
	if len(afterReset) != len(baseline) {
		t.Errorf("len(afterReset) = %d, want %d (Baseline) - erneuter Demo-Login muss vorherige Demo-Änderungen verwerfen", len(afterReset), len(baseline))
	}
	for _, p := range afterReset {
		if p.Monat == "2099-01-01" {
			t.Error("die in der 1. Demo-Session angelegte Ablesung ist nach dem Reset noch vorhanden")
		}
	}

	realPeriods, err := store.AllPeriods(realDB)
	if err != nil {
		t.Fatalf("AllPeriods(realDB): %v", err)
	}
	if len(realPeriods) != 0 {
		t.Errorf("len(realPeriods) = %d, want 0 - der Demo-Reset darf die echte DB nie berühren", len(realPeriods))
	}
}

// TestDemoMode_IndependentOfRealLogin covers the spec's letzten Punkt: ein
// regulärer Login (echtes LOGIN_PASSWORD) bleibt unverändert auf die echte
// Datenbank bezogen, unabhängig vom Demo-Modus.
func TestDemoMode_IndependentOfRealLogin(t *testing.T) {
	const secret = "geheim"
	realDB := openTestDB(t)
	demoDB := openTestDB(t)

	t.Setenv("LOGIN_PASSWORD", secret)
	mux := NewMux(realDB, demoDB, "", "")

	realCookies := demoLoginCookies(t, mux, secret)

	form := periodFormValues("2026-07-15", "2026-07")
	req := httptest.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachCookies(req, realCookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("create ablesung mit echtem Login: status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}

	realPeriods, err := store.AllPeriods(realDB)
	if err != nil {
		t.Fatalf("AllPeriods(realDB): %v", err)
	}
	if len(realPeriods) != 1 {
		t.Errorf("len(realPeriods) = %d, want 1 - regulärer Login muss die echte DB benutzen", len(realPeriods))
	}

	demoPeriods, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB): %v", err)
	}
	if len(demoPeriods) != 0 {
		t.Errorf("len(demoPeriods) = %d, want 0 - regulärer Login darf nie in der Demo-DB landen", len(demoPeriods))
	}
}
