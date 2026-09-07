package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSelectDB drives the Demo/Echt-Entscheidung directly (Architecture
// Review nach dem Demo-Modus #118, Kandidat 4) - vorher nur über einen
// vollen NewMux + echte SQLite-Dateien + simulierten HTTP-Login-Roundtrip
// erreichbar (siehe demo_test.go), jetzt mit einem bloßen *http.Request.
func TestSelectDB(t *testing.T) {
	db := &sql.DB{}
	demoDB := &sql.DB{}

	t.Run("ohne Demo-Session-Cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		if got := selectDB(r, db, demoDB); got != db {
			t.Errorf("selectDB() = %p, want db (%p)", got, db)
		}
	})

	t.Run("mit gültiger Demo-Session-Cookie", func(t *testing.T) {
		w := httptest.NewRecorder()
		setDemoSessionCookie(w)
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		for _, c := range w.Result().Cookies() {
			r.AddCookie(c)
		}
		if got := selectDB(r, db, demoDB); got != demoDB {
			t.Errorf("selectDB() = %p, want demoDB (%p)", got, demoDB)
		}
	})

	t.Run("mit manipulierter/ungültig signierter Demo-Cookie", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		r.AddCookie(&http.Cookie{Name: demoSessionCookieName, Value: "manipuliert"})
		if got := selectDB(r, db, demoDB); got != db {
			t.Errorf("selectDB() = %p, want db (%p) - eine ungültig signierte Demo-Cookie darf nicht zur Demo-DB routen", got, db)
		}
	})
}
