package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestSelectDB drives the demo/real DB decision directly (architecture
// review after demo mode #118, candidate 4) - previously only reachable via
// a full NewMux + real SQLite files + simulated HTTP login roundtrip (see
// demo_test.go), now with a bare *http.Request.
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
		demoKind.set(w)
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
