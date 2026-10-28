package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func loginPost(a auth, rolle, passwort string) *httptest.ResponseRecorder {
	form := url.Values{"rolle": {rolle}, "password": {passwort}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	a.HandleLogin()(w, req)
	return w
}

func hatCookie(w *httptest.ResponseRecorder, name string) bool {
	for _, c := range w.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return true
		}
	}
	return false
}

func TestLoginLimit_FehlversucheSperren(t *testing.T) {
	jetzt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := auth{vermieter: "v", limit: newLoginLimit(func() time.Time { return jetzt })}

	for i := 0; i < maxLoginFehler; i++ {
		if w := loginPost(a, rolleFormVermieter, "falsch"); w.Code != http.StatusFound {
			t.Fatalf("Fehlversuch %d: Status %d, want 302", i+1, w.Code)
		}
	}
	// Locked: even the correct password is refused until the window passes.
	w := loginPost(a, rolleFormVermieter, "v")
	loc := w.Header().Get("Location")
	if w.Code != http.StatusFound || !strings.Contains(loc, "login=1") || !strings.Contains(loc, "gesperrt=1") || strings.Contains(loc, "fehler=1") {
		t.Fatalf("gesperrt: Status %d, Location %q, want Overlay mit gesperrt=1 ohne fehler=1", w.Code, loc)
	}
	if hatCookie(w, sessionCookieName) {
		t.Error("gesperrt: Cookie gesetzt")
	}

	jetzt = jetzt.Add(loginFensterDauer)
	w = loginPost(a, rolleFormVermieter, "v")
	if w.Code != http.StatusFound || !hatCookie(w, sessionCookieName) {
		t.Errorf("nach dem Fenster: Status %d, want Login mit Cookie", w.Code)
	}
}

func TestLoginLimit_ErfolgZaehltNicht(t *testing.T) {
	jetzt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := auth{vermieter: "v", limit: newLoginLimit(func() time.Time { return jetzt })}
	for i := 0; i < 3*maxLoginFehler; i++ {
		if w := loginPost(a, rolleFormVermieter, "v"); w.Code != http.StatusFound {
			t.Fatalf("Login %d: Status %d", i+1, w.Code)
		}
	}
}

func TestLoginLimit_DemoResetGedrosselt(t *testing.T) {
	jetzt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newLoginLimit(func() time.Time { return jetzt })
	n := 0
	reset := func() error { n++; return nil }

	for i, c := range []struct {
		nach time.Duration
		want int
	}{{0, 1}, {demoResetAbstand - time.Second, 1}, {time.Second, 2}} {
		jetzt = jetzt.Add(c.nach)
		if err := l.demoReset(reset); err != nil || n != c.want {
			t.Fatalf("Schritt %d: err %v, Resets %d, want %d", i, err, n, c.want)
		}
	}
}

func TestLoginLimit_FehlgeschlagenerDemoResetStartetKeineSperre(t *testing.T) {
	jetzt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	l := newLoginLimit(func() time.Time { return jetzt })
	if err := l.demoReset(func() error { return errors.New("kaputt") }); err == nil {
		t.Fatal("Fehler des Resets muss durchgereicht werden")
	}
	n := 0
	if err := l.demoReset(func() error { n++; return nil }); err != nil || n != 1 {
		t.Errorf("nach fehlgeschlagenem Reset: err %v, Resets %d, want 1", err, n)
	}
}

func TestLoginOverlay_ZeigtSperrmeldung(t *testing.T) {
	mux := rollenMux(t, "geheim", "")
	w := getMitCookies(t, mux, "/dashboard?login=1&gesperrt=1", nil)
	body := w.Body.String()
	if !strings.Contains(body, "Zu viele fehlgeschlagene Anmeldungen, bitte eine Minute warten.") || !strings.Contains(body, `params.get("gesperrt")`) {
		t.Error("Login-Overlay zeigt die Sperrmeldung nicht")
	}
}

func TestLoginLimit_DemoLoginBleibtErfolgreich(t *testing.T) {
	jetzt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	a := auth{demoDB: openTestDB(t), limit: newLoginLimit(func() time.Time { return jetzt })}
	for i := 0; i < 3; i++ {
		w := loginPost(a, rolleFormDemo, "")
		if w.Code != http.StatusFound || !hatCookie(w, demoSessionCookieName) {
			t.Fatalf("Demo-Login %d: Status %d, ohne Demo-Cookie", i+1, w.Code)
		}
	}
}
