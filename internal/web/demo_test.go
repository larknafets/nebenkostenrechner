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
// LOGIN_PASSWORD (secret) is configured (Issue #118: "always works").
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

// TestDemoMode_WritesIsolatedFromRealDB is the spec's core requirement
// (Issue #118 Testing Decisions): a meter reading (Ablesung) created in
// demo mode must land exclusively in the demo database, never in the real
// one.
func TestDemoMode_WritesIsolatedFromRealDB(t *testing.T) {
	realDB := openTestDB(t)
	demoDB := openTestDB(t)
	mux := NewMux(realDB, demoDB, "", "")

	cookies := demoLoginCookies(t, mux, demoPassword)

	// Demo login already resets the demo DB to its 39-month starting state
	// (Issue #121) - read the baseline once here instead of hardcoding the
	// exact count.
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

	// Without a demo session (not logged in, secret == "") the real,
	// still-empty database stays visible.
	req2 := httptest.NewRequest(http.MethodGet, "/ablesungen", nil)
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)
	if strings.Contains(w2.Body.String(), "2026-06-15") {
		t.Error("nicht angemeldete Ansicht zeigt die im Demo-Modus angelegte Ablesung - Datenbanken sind nicht isoliert")
	}
}

// TestDemoLogin_ResetsPreviousDemoSession covers Issue #121's core
// requirement: a meter reading created in demo mode is no longer present
// after a renewed demo login, and this reset has no effect whatsoever on
// the real DB.
func TestDemoLogin_ResetsPreviousDemoSession(t *testing.T) {
	realDB := openTestDB(t)
	demoDB := openTestDB(t)
	mux := NewMux(realDB, demoDB, "", "")

	firstSession := demoLoginCookies(t, mux, demoPassword)
	baseline, err := store.AllPeriods(demoDB)
	if err != nil {
		t.Fatalf("AllPeriods(demoDB) baseline: %v", err)
	}

	// 2099 is guaranteed to lie outside the 39-month window generated
	// relative to "now" (Issue #117) - clearly identifiable as an injected
	// change, unlike a month that could already be part of the fresh
	// baseline.
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

	// A renewed demo login (2nd session) must discard the reading from the
	// 1st session and show exactly the baseline again.
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

// findCookie returns the cookie with the given name among cookies, or nil.
func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestLogin_ClearsOtherSessionCookie covers a gap surfaced by the whole-
// feature review of #118: handleLogin only ever set the cookie for the kind
// of session it just granted, never clearing the other one - a stale
// nk_demo_session cookie (30 day TTL) would outlive a subsequent real login
// and (since isDemoSession is checked before the real cookie) keep serving
// demo data despite a fresh real login. Every successful login now
// symmetrically clears whichever other session cookie exists, instead of
// relying solely on the check order.
func TestLogin_ClearsOtherSessionCookie(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")

	demoCookies := demoLoginCookies(t, mux, demoPassword)
	if c := findCookie(demoCookies, sessionCookieName); c == nil || c.Value != "" {
		t.Errorf("Demo-Login: %s nicht geräumt (Value=%q), want geräumt (Value == \"\")", sessionCookieName, valueOrMissing(c))
	}
	if c := findCookie(demoCookies, demoSessionCookieName); c == nil || c.Value == "" {
		t.Errorf("Demo-Login: %s nicht gesetzt", demoSessionCookieName)
	}

	realCookies := demoLoginCookies(t, mux, "geheim")
	if c := findCookie(realCookies, demoSessionCookieName); c == nil || c.Value != "" {
		t.Errorf("echter Login: %s nicht geräumt (Value=%q), want geräumt (Value == \"\")", demoSessionCookieName, valueOrMissing(c))
	}
	if c := findCookie(realCookies, sessionCookieName); c == nil || c.Value == "" {
		t.Errorf("echter Login: %s nicht gesetzt", sessionCookieName)
	}
}

func valueOrMissing(c *http.Cookie) string {
	if c == nil {
		return "<cookie fehlt ganz>"
	}
	return c.Value
}

// TestDemoMode_IndependentOfRealLogin covers the spec's last point: a
// regular login (real LOGIN_PASSWORD) stays tied to the real database
// unchanged, independent of demo mode.
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
