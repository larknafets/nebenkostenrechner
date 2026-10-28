package web

import (
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
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("gesperrt: Status %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After fehlt")
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

	if !l.demoResetErlaubt() {
		t.Fatal("erster Demo-Reset muss erlaubt sein")
	}
	jetzt = jetzt.Add(demoResetAbstand - time.Second)
	if l.demoResetErlaubt() {
		t.Error("Demo-Reset innerhalb von 10 s muss gedrosselt werden")
	}
	jetzt = jetzt.Add(time.Second)
	if !l.demoResetErlaubt() {
		t.Error("Demo-Reset nach 10 s muss wieder erlaubt sein")
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
