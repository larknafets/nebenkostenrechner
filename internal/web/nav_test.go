package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// getDashboard renders /dashboard, optionally carrying cookies, and returns
// the body - the shared "nav" Partial (layout.html) is exercised by every
// route, so /dashboard is enough to cover Issue #122's nav/banner criteria.
func getDashboard(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /dashboard: status = %d, want %d", w.Code, http.StatusOK)
	}
	return w.Body.String()
}

// TestNav_LoginEntry_NoLoginPassword covers Issue #122's core requirement:
// if LOGIN_PASSWORD is not set, there is still a visible, clickable path to
// the login overlay (id="login-open"), even though isLoggedIn is
// unconditionally true in this state.
func TestNav_LoginEntry_NoLoginPassword(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	body := getDashboard(t, mux, nil)

	if !strings.Contains(body, `id="login-open"`) {
		t.Error(`kein Login-Overlay-Einstiegspunkt (id="login-open") sichtbar, obwohl LOGIN_PASSWORD nicht gesetzt ist`)
	}
	if !strings.Contains(body, `id="login-overlay"`) {
		t.Error("das Login-Overlay selbst wird nicht gerendert, obwohl ein Einstiegspunkt dafür sichtbar sein soll")
	}
	// Without an active session (no real login possible since secret == "",
	// and no demo session either) there's nothing to log out of - the
	// logout link stays hidden (otherwise logout and login would show up
	// side by side contradictorily, see Issue #132 bugfix).
	if strings.Contains(body, `id="logout-link"`) {
		t.Error("Abmelden-Link sichtbar ohne aktive Session (secret == \"\", keine Demo-Session)")
	}
	if strings.Contains(body, `class="demo-banner"`) {
		t.Error("Demo-Banner sichtbar ohne aktive Demo-Session")
	}
}

// TestNav_LoginEntry_NotLoggedIn covers acceptance criterion 5: the classic
// "not logged in" case (LOGIN_PASSWORD set, no cookie) stays unchanged -
// still exactly one entry point, now named "Anmelden" instead of "Login",
// no additional logout link.
func TestNav_LoginEntry_NotLoggedIn(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	body := getDashboard(t, mux, nil)

	if !strings.Contains(body, `id="login-open">Anmelden</a>`) {
		t.Error(`Einstiegspunkt fehlt oder heißt nicht mehr "Anmelden"`)
	}
	if strings.Contains(body, "Login<") {
		t.Error(`Nav-Link heißt noch "Login" statt "Anmelden"`)
	}
	if strings.Contains(body, `id="logout-link"`) {
		t.Error("Abmelden-Link sichtbar, obwohl nicht eingeloggt")
	}
	if strings.Contains(body, `class="demo-banner"`) {
		t.Error("Demo-Banner sichtbar ohne aktive Demo-Session")
	}
}

// TestNav_RealLogin_NoDemoEntryNoBanner covers acceptance criteria 3 and 5:
// a regularly (real LOGIN_PASSWORD) logged-in user sees neither the demo
// banner nor an additional login entry point - just "Abmelden" (logout), as
// before Issue #122.
func TestNav_RealLogin_NoDemoEntryNoBanner(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	cookies := demoLoginCookies(t, mux, "geheim")
	body := getDashboard(t, mux, cookies)

	if !strings.Contains(body, `id="logout-link">Abmelden</a>`) {
		t.Error(`Abmelden-Link fehlt oder heißt nicht mehr "Abmelden"`)
	}
	if strings.Contains(body, `id="login-open"`) {
		t.Error("zusätzlicher Login-Einstiegspunkt sichtbar für regulär eingeloggten Nutzer")
	}
	if strings.Contains(body, `class="demo-banner"`) {
		t.Error("Demo-Banner sichtbar für regulär eingeloggten Nutzer (kein Demo)")
	}
}

// TestNav_DemoSession_ShowsBannerNotEntry covers acceptance criteria 2 and
// 3: during an active demo session the banner is visible, no additional
// login entry point (logout is enough to leave the demo).
func TestNav_DemoSession_ShowsBannerNotEntry(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	cookies := demoLoginCookies(t, mux, demoPassword)
	body := getDashboard(t, mux, cookies)

	if !strings.Contains(body, `class="demo-banner"`) {
		t.Error("Demo-Banner fehlt während einer laufenden Demo-Session")
	}
	if !strings.Contains(body, "geteilte Vorführumgebung") {
		t.Error("Demo-Banner-Text macht nicht klar, dass es sich um eine geteilte Demo-Umgebung handelt")
	}
	if !strings.Contains(body, `id="logout-link">Abmelden</a>`) {
		t.Error(`Abmelden-Link fehlt oder heißt nicht mehr "Abmelden"`)
	}
	if strings.Contains(body, `id="login-open"`) {
		t.Error("zusätzlicher Login-Einstiegspunkt sichtbar während einer laufenden Demo-Session")
	}
}
