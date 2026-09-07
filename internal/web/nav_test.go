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

// TestNav_LoginEntry_NoLoginPassword covers Issue #122's Kernanforderung:
// ist LOGIN_PASSWORD nicht gesetzt, gibt es trotzdem einen sichtbaren,
// klickbaren Weg zum Login-Overlay (id="login-open"), obwohl isLoggedIn in
// diesem Zustand unconditionally true ist.
func TestNav_LoginEntry_NoLoginPassword(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	body := getDashboard(t, mux, nil)

	if !strings.Contains(body, `id="login-open"`) {
		t.Error(`kein Login-Overlay-Einstiegspunkt (id="login-open") sichtbar, obwohl LOGIN_PASSWORD nicht gesetzt ist`)
	}
	if !strings.Contains(body, `id="login-overlay"`) {
		t.Error("das Login-Overlay selbst wird nicht gerendert, obwohl ein Einstiegspunkt dafür sichtbar sein soll")
	}
	// Bestehendes Verhalten (Ticket #112): secret == "" gilt weiterhin als
	// eingeloggt, der Abmelden-Link bleibt sichtbar.
	if !strings.Contains(body, `id="logout-link"`) {
		t.Error("Abmelden-Link fehlt - bestehendes Verhalten für secret == \"\" darf sich nicht ändern")
	}
	if strings.Contains(body, `class="demo-banner"`) {
		t.Error("Demo-Banner sichtbar ohne aktive Demo-Session")
	}
}

// TestNav_LoginEntry_NotLoggedIn covers acceptance criterion 5: der
// klassische "nicht eingeloggt"-Fall (LOGIN_PASSWORD gesetzt, kein Cookie)
// bleibt unverändert - weiterhin genau ein Einstiegspunkt, jetzt "Anmelden"
// statt "Login" benannt, kein zusätzlicher Abmelden-Link.
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

// TestNav_RealLogin_NoDemoEntryNoBanner covers acceptance criteria 3 und 5:
// ein regulär (mit echtem LOGIN_PASSWORD) eingeloggter Nutzer sieht weder
// den Demo-Banner noch einen zusätzlichen Login-Einstiegspunkt - nur
// "Abmelden", wie vor Issue #122.
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

// TestNav_DemoSession_ShowsBannerNotEntry covers acceptance criteria 2 und
// 3: während einer laufenden Demo-Session ist der Banner sichtbar, kein
// zusätzlicher Login-Einstiegspunkt (Abmelden reicht, um die Demo zu
// verlassen).
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
