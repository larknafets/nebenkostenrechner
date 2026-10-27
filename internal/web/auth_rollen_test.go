package web

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Tests for the roles Nicht angemeldet, Mieter and Vermieter and their two
// logins (ADR 0003).

func rollenMux(t *testing.T, vermieter, mieter string) *http.ServeMux {
	t.Helper()
	t.Setenv("VERMIETER_PASSWORD", vermieter)
	t.Setenv("MIETER_PASSWORD", mieter)
	return NewMux(openTestDB(t), openTestDB(t), "", "")
}

func getMitCookies(t *testing.T, mux *http.ServeMux, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestLoginOptionen(t *testing.T) {
	for _, c := range []struct {
		name              string
		vermieter, mieter string
		want              []string
	}{
		{"ohne Kennwörter: nur Demo", "", "", []string{"demo"}},
		{"nur Vermieter", "v", "", []string{"vermieter", "demo"}},
		{"beide", "v", "m", []string{"mieter", "vermieter", "demo"}},
		{"Mieter ohne Vermieter: ignoriert", "", "m", []string{"demo"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			var got []string
			for _, o := range (auth{vermieter: c.vermieter, mieter: c.mieter}).loginOptionen() {
				got = append(got, o.Wert)
			}
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Errorf("Optionen = %v, want %v", got, c.want)
			}
		})
	}
}

func TestKennwoerterLaden(t *testing.T) {
	for _, c := range []struct {
		name                      string
		env                       map[string]string
		wantVermieter, wantMieter string
	}{
		{"beide", map[string]string{"VERMIETER_PASSWORD": "v", "MIETER_PASSWORD": "m"}, "v", "m"},
		{"nur Vermieter", map[string]string{"VERMIETER_PASSWORD": "v"}, "v", ""},
		{"Mieter ohne Vermieter wird ignoriert", map[string]string{"MIETER_PASSWORD": "m"}, "", ""},
		{"altes LOGIN_PASSWORD wird nicht gelesen", map[string]string{"LOGIN_PASSWORD": "alt"}, "", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range []string{"VERMIETER_PASSWORD", "MIETER_PASSWORD", "LOGIN_PASSWORD", "SUPERVISOR_TOKEN"} {
				t.Setenv(k, c.env[k])
			}
			v, m := ladeKennwoerter()
			if v != c.wantVermieter || m != c.wantMieter {
				t.Errorf("ladeKennwoerter() = %q, %q, want %q, %q", v, m, c.wantVermieter, c.wantMieter)
			}
		})
	}
}

// The old LOGIN_PASSWORD is not read any more; a log line tells the operator
// why the app is open, but only while no Vermieter password is set.
func TestKennwoerterLaden_WarntBeiAltemLoginPassword(t *testing.T) {
	for _, c := range []struct {
		name     string
		env      map[string]string
		wantWarn bool
	}{
		{"altes Kennwort ohne neues", map[string]string{"LOGIN_PASSWORD": "alt"}, true},
		{"altes und neues Kennwort", map[string]string{"LOGIN_PASSWORD": "alt", "VERMIETER_PASSWORD": "v"}, false},
		{"kein altes Kennwort", map[string]string{}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, k := range []string{"VERMIETER_PASSWORD", "MIETER_PASSWORD", "LOGIN_PASSWORD", "SUPERVISOR_TOKEN"} {
				t.Setenv(k, c.env[k])
			}
			var buf bytes.Buffer
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			ladeKennwoerter()

			if got := strings.Contains(buf.String(), "LOGIN_PASSWORD wird nicht mehr gelesen"); got != c.wantWarn {
				t.Errorf("Warnung = %v, want %v (Log: %q)", got, c.wantWarn, buf.String())
			}
		})
	}
}

func TestLoginFormular_ZeigtAuswahlJeNachKennwoertern(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	body := getMitCookies(t, mux, "/dashboard", nil).Body.String()
	for _, s := range []string{
		`<select name="rolle" id="login-rolle">`,
		`<option value="mieter">Mieter</option>`, `<option value="vermieter">Vermieter</option>`, `<option value="demo">Demo</option>`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("Login-Formular ohne %q", s)
		}
	}

	nurVermieter := rollenMux(t, "geheim", "")
	if body := getMitCookies(t, nurVermieter, "/dashboard", nil).Body.String(); strings.Contains(body, `<option value="mieter">`) {
		t.Error("Mieter-Auswahl ohne Mieter-Kennwort")
	}
}

func TestLogin_MieterUndVermieter(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")

	mieter := loginCookies(t, mux, "mieter", "mieter")
	if c := findCookie(mieter, mieterSessionCookieName); c == nil || c.Value == "" {
		t.Fatalf("Mieter-Login: %s nicht gesetzt", mieterSessionCookieName)
	}
	if c := findCookie(mieter, sessionCookieName); c == nil || c.Value != "" {
		t.Errorf("Mieter-Login: %s nicht geräumt", sessionCookieName)
	}

	vermieter := loginCookies(t, mux, "vermieter", "geheim")
	if c := findCookie(vermieter, sessionCookieName); c == nil || c.Value == "" {
		t.Fatalf("Vermieter-Login: %s nicht gesetzt", sessionCookieName)
	}
	if c := findCookie(vermieter, mieterSessionCookieName); c == nil || c.Value != "" {
		t.Errorf("Vermieter-Login: %s nicht geräumt", mieterSessionCookieName)
	}
}

func TestLogin_FalschesKennwortOderRolle(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	for _, c := range []struct{ name, rolle, password string }{
		{"Mieter mit Vermieter-Kennwort", "mieter", "geheim"},
		{"Vermieter mit Mieter-Kennwort", "vermieter", "mieter"},
		{"falsches Kennwort", "vermieter", "falsch"},
		{"ohne Rolle", "", "geheim"},
		{"unbekannte Rolle", "admin", "geheim"},
	} {
		t.Run(c.name, func(t *testing.T) {
			form := strings.NewReader("rolle=" + c.rolle + "&password=" + c.password)
			req := httptest.NewRequest(http.MethodPost, "/login", form)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)
			loc := w.Header().Get("Location")
			if w.Code != http.StatusFound || !strings.Contains(loc, "login=1") || !strings.Contains(loc, "fehler=1") {
				t.Errorf("status %d, Location %q, want Weiterleitung zum Overlay mit fehler=1", w.Code, loc)
			}
			if len(w.Result().Cookies()) != 0 {
				t.Errorf("Cookies bei falschem Login: %v", w.Result().Cookies())
			}
		})
	}
}

func TestLogin_MieterOhneMieterKennwort(t *testing.T) {
	mux := rollenMux(t, "geheim", "")
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("rolle=mieter&password="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "fehler=1") {
		t.Errorf("Mieter-Login ohne Mieter-Kennwort: Location %q, want fehler=1", loc)
	}
}

// A Mieter reaches a page reserved for the Vermieter as a 403 with the link to
// log in as Vermieter; a visitor who is not logged in is bounced to the login
// overlay, the Vermieter gets the page.
func TestGate_JeRolle(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	mieter := loginCookies(t, mux, "mieter", "mieter")
	vermieter := loginCookies(t, mux, "vermieter", "geheim")

	w := getMitCookies(t, mux, "/abrechnung", mieter)
	if w.Code != http.StatusForbidden {
		t.Fatalf("Mieter auf /abrechnung: status %d, want 403", w.Code)
	}
	for _, s := range []string{"Nur für den Vermieter", "login=1&amp;rolle=vermieter&amp;next=%2fabrechnung"} {
		if !strings.Contains(strings.ToLower(w.Body.String()), strings.ToLower(s)) {
			t.Errorf("403-Seite ohne %q", s)
		}
	}

	w = getMitCookies(t, mux, "/abrechnung", nil)
	if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "login=1") {
		t.Errorf("nicht angemeldet auf /abrechnung: status %d, Location %q, want Weiterleitung zum Overlay", w.Code, w.Header().Get("Location"))
	}

	if w := getMitCookies(t, mux, "/abrechnung", vermieter); w.Code != http.StatusOK {
		t.Errorf("Vermieter auf /abrechnung: status %d, want 200", w.Code)
	}
}

// The Mieter sees the same as a visitor who is not logged in until the
// Berechtigungen exist, but is logged in: Abmelden and the way to the
// Vermieter login are in the nav, no Abrechnung link.
func TestMieter_SichtUndNavigation(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	mieter := loginCookies(t, mux, "mieter", "mieter")
	body := getMitCookies(t, mux, "/dashboard", mieter).Body.String()

	if !strings.Contains(body, `id="logout-link"`) || !strings.Contains(body, `id="login-open"`) {
		t.Error("Mieter sieht nicht Abmelden und Anmelden (als Vermieter)")
	}
	if strings.Contains(body, `>Abrechnung</a>`) || strings.Contains(body, labelMiete) || strings.Contains(body, "<strong>Wohnung 1</strong>") {
		t.Error("Mieter sieht Vermieter-Inhalte")
	}
}

func TestLogout_RaeumtAlleSitzungen(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	for _, name := range []string{sessionCookieName, mieterSessionCookieName, demoSessionCookieName} {
		if c := findCookie(w.Result().Cookies(), name); c == nil || c.Value != "" {
			t.Errorf("Abmelden räumt %s nicht", name)
		}
	}
}

// Changing a password logs out only the role it belongs to.
func TestSitzung_KennwortwechselMeldetNurDieseRolleAb(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	mieter := loginCookies(t, mux, "mieter", "mieter")

	neu := rollenMux(t, "geheim", "anders")
	if w := getMitCookies(t, neu, "/abrechnung", mieter); w.Code == http.StatusForbidden {
		t.Error("altes Mieter-Cookie gilt nach dem Kennwortwechsel noch als Mieter")
	}
}
