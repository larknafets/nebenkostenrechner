package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
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
	if strings.Contains(body, `class="demo-banner no-print"`) {
		t.Error("Demo-Banner sichtbar ohne aktive Demo-Session")
	}
}

// TestNav_LoginEntry_NotLoggedIn covers acceptance criterion 5: the classic
// "not logged in" case (LOGIN_PASSWORD set, no cookie) stays unchanged -
// still exactly one entry point, now named "Anmelden" instead of "Login",
// no additional logout link.
func TestNav_LoginEntry_NotLoggedIn(t *testing.T) {
	t.Setenv("VERMIETER_PASSWORD", "geheim")
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
	if strings.Contains(body, `class="demo-banner no-print"`) {
		t.Error("Demo-Banner sichtbar ohne aktive Demo-Session")
	}
}

// TestNav_RealLogin_NoDemoEntryNoBanner covers acceptance criteria 3 and 5:
// a regularly (real LOGIN_PASSWORD) logged-in user sees neither the demo
// banner nor an additional login entry point - just "Abmelden" (logout), as
// before Issue #122.
func TestNav_RealLogin_NoDemoEntryNoBanner(t *testing.T) {
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	cookies := loginCookies(t, mux, "vermieter", "geheim")
	body := getDashboard(t, mux, cookies)

	if !strings.Contains(body, `id="logout-link">Abmelden</a>`) {
		t.Error(`Abmelden-Link fehlt oder heißt nicht mehr "Abmelden"`)
	}
	if strings.Contains(body, `id="login-open"`) {
		t.Error("zusätzlicher Login-Einstiegspunkt sichtbar für regulär eingeloggten Nutzer")
	}
	if strings.Contains(body, `class="demo-banner no-print"`) {
		t.Error("Demo-Banner sichtbar für regulär eingeloggten Nutzer (kein Demo)")
	}
}

// TestNav_DemoSession_ShowsBannerNotEntry covers acceptance criteria 2 and
// 3: during an active demo session the banner is visible, no additional
// login entry point (logout is enough to leave the demo).
func TestNav_DemoSession_ShowsBannerNotEntry(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	cookies := demoLoginCookies(t, mux)
	body := getDashboard(t, mux, cookies)

	if !strings.Contains(body, `class="demo-banner no-print"`) {
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

// TestFooterVersion_UpdateDotHidden: the update dot is hidden without a
// known update, and the stylesheet must not override the hidden attribute
// (".footer-version a { display: flex }" did, so the dot always showed).
func TestFooterVersion_UpdateDotHidden(t *testing.T) {
	// Unknown state: no latest version known yet. Sets the shared cache itself
	// (fresh, so nothing asks GitHub) because a filled cache would make v99 "aktuell".
	setReleaseCache(t, "")
	mux := NewMux(openTestDB(t), openTestDB(t), "v99.0.0", "2026-10-01T18:01:25Z")
	body := getDashboard(t, mux, nil)

	i := strings.Index(body, `id="update-dot"`)
	if i < 0 {
		t.Fatal("Update-Punkt (id=\"update-dot\") fehlt, obwohl eine Version gesetzt ist")
	}
	tag := body[i : i+strings.Index(body[i:], ">")]
	if !strings.Contains(tag, "hidden") {
		t.Errorf("Update-Punkt ohne hidden, obwohl kein Update bekannt ist: %s", tag)
	}
	// Pico's [hidden] rule hides the dot, as long as none of our rules sets display on the footer links.
	if strings.Contains(body, ".footer-version a { display") || strings.Contains(body, ".footer-version a { color: var(--muted); display") {
		t.Error("das Stylesheet setzt display auf Links im Seitenfuß, hidden würde überschrieben")
	}
	if strings.Contains(body, ".footer-version { position: fixed") {
		t.Error("der Seitenfuß ist wieder fixiert")
	}
	if !strings.Contains(body, `<footer class="footer-version no-print">`) {
		t.Error("Seitenfuß fehlt oder trägt kein no-print")
	}
}

// TestFooterVersion_UpdateDotStates: the dot is visible in "Update" (red class,
// tooltip with version) and "aktuell" (no red class), hidden for other channels.
func TestFooterVersion_UpdateDotStates(t *testing.T) {
	cases := []struct {
		name, latest, version string
		hidden, update        bool
		title                 string
	}{
		{"update", "v100.0.0", "v99.0.0", false, true, "Update verfügbar: v100.0.0"},
		{"aktuell", "v99.0.0", "v99.0.0", false, false, "Version ist aktuell"},
		{"unbekannt, anderer Kanal", "nightly.20261019-aaaaaaa", "v99.0.0", true, false, ""},
		{"unbekannt, gleicher Nightly-Tag", "nightly.20261019-aaaaaaa", "nightly.20261019-bbbbbbb", true, false, ""},
		{"aktuell, gleicher Nightly", "nightly.20261019-aaaaaaa", "nightly.20261019-aaaaaaa", false, false, "Version ist aktuell"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			setReleaseCache(t, c.latest)
			mux := NewMux(openTestDB(t), openTestDB(t), c.version, "2026-10-01T18:01:25Z")
			body := getDashboard(t, mux, nil)
			i := strings.Index(body, `id="update-dot"`)
			if i < 0 {
				t.Fatal("Update-Punkt fehlt")
			}
			tag := body[i : i+strings.Index(body[i:], ">")]
			if got := strings.Contains(tag, "hidden"); got != c.hidden {
				t.Errorf("hidden = %v, want %v: %s", got, c.hidden, tag)
			}
			if got := strings.Contains(tag, "is-update"); got != c.update {
				t.Errorf("is-update = %v, want %v: %s", got, c.update, tag)
			}
			if c.title != "" && !strings.Contains(tag, `title="`+c.title+`"`) {
				t.Errorf("Tooltip %q fehlt: %s", c.title, tag)
			}
		})
	}
}

// TestNav_Kopf: the header carries the brand once, the menu for wide and
// narrow screens, marks the current page and prints nothing.
func TestNav_Kopf(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	for _, p := range []struct{ pfad, h1 string }{
		{"/dashboard", "<h1>Dashboard</h1>"},
		{"/ablesungen", "<h1>Ablesungen</h1>"},
		{"/berechnungslogik", "<h1>Berechnungslogik</h1>"},
	} {
		body := getMitCookies(t, mux, p.pfad, nil).Body.String()
		for _, s := range []string{`<header class="no-print">`, `<details class="dropdown">`, `<a href="` + p.pfad + `" aria-current="page">`, p.h1} {
			if !strings.Contains(body, s) {
				t.Errorf("%s: %q fehlt", p.pfad, s)
			}
		}
		if n := strings.Count(body, ` aria-current="page"`); n != 2 { // wide list and dropdown
			t.Errorf("%s: aria-current %d-mal, want 2", p.pfad, n)
		}
		if n := strings.Count(body, "<strong>Nebenkostenrechner</strong>"); n != 1 {
			t.Errorf("%s: Marke %d-mal, want 1", p.pfad, n)
		}
		if strings.Contains(body, "<h1>Nebenkostenrechner</h1>") {
			t.Errorf("%s: Überschrift trägt die Marke", p.pfad)
		}
	}
}

// The theme switch stays one button, last in the nav.
func TestNav_UmschalterAmEnde(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	body := getDashboard(t, mux, nil)
	if n := strings.Count(body, `id="theme-toggle-btn"`); n != 1 {
		t.Fatalf("Umschalter %d-mal, want 1", n)
	}
	if strings.Index(body, `id="theme-toggle-btn"`) < strings.Index(body, `id="login-open"`) {
		t.Error("Umschalter steht vor dem Anmelden-Eintrag, nicht am Ende")
	}
	if strings.Contains(body, ".theme-toggle { position: fixed") {
		t.Error("Umschalter ist noch fixiert")
	}
}

// TestDashboardVerlauf_KeineAblesungZentriert: a month without Ablesung
// shows the note framed by dashes, and the stylesheet centers it in the bar.
func TestDashboardVerlauf_KeineAblesungZentriert(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	body := getDashboard(t, NewMux(db, openTestDB(t), "", ""), nil)

	if !strings.Contains(body, `data-modus="verbrauch">– keine Ablesung –</span>`) {
		t.Error(`Monat ohne Ablesung zeigt nicht "– keine Ablesung –"`)
	}
	if !strings.Contains(body, ".verlauf-none { display: flex; align-items: center; justify-content: center;") {
		t.Error("der Hinweis ist im Stylesheet nicht vertikal und horizontal zentriert")
	}
}

// TestNav_StammdatenVorAbrechnung: the navigation lists Stammdaten before
// Abrechnung (the Abrechnung entry exists for logged-in users only).
func TestNav_StammdatenVorAbrechnung(t *testing.T) {
	body := getDashboard(t, NewMux(openTestDB(t), openTestDB(t), "", ""), nil)

	stammdaten := strings.Index(body, `/stammdaten">Stammdaten</a>`)
	abrechnung := strings.Index(body, `/abrechnung">Abrechnung</a>`)
	if stammdaten < 0 || abrechnung < 0 {
		t.Fatalf("Navigation ohne Stammdaten (%d) oder Abrechnung (%d)", stammdaten, abrechnung)
	}
	if stammdaten > abrechnung {
		t.Error("Abrechnung steht vor Stammdaten, erwartet: Stammdaten zuerst")
	}
}
