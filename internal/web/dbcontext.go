package web

import (
	"context"
	"database/sql"
	"net/http"
)

// dbContextKey is the request-context key withDB stores the per-request
// *sql.DB under (Ticket #119, Prefactoring für den Demo-Modus #118): NewMux
// used to hand every handler a single *sql.DB fixed at startup via closure -
// too rigid once a request must be able to pick real vs. Demo-Datenbank
// depending on its session (next ticket). Handlers now read it via
// dbFromContext instead.
type dbContextKey struct{}

// selectDB picks demoDB when r carries a valid Demo-Session-Cookie (Issue
// #120), db otherwise - the actual Demo/Echt-Entscheidung, pulled out of
// withDB (Architecture Review nach dem Demo-Modus #118, Kandidat 4) so
// sie sich direkt mit einem bloßen *http.Request testen lässt, ohne einen
// Mux, echten Handler oder echte SQLite-Dateien zu brauchen.
func selectDB(r *http.Request, db, demoDB *sql.DB) *sql.DB {
	if isDemoSession(r) {
		return demoDB
	}
	return db
}

// withDB wraps next so it (and any helper it calls with r in scope) can read
// the request's *sql.DB via dbFromContext(r.Context()) - the only place in
// the app that branches Demo/Echt-Datenbank (via selectDB); every
// downstream handler just reads whichever one it got.
func withDB(db, demoDB *sql.DB, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, requestWithDB(r, selectDB(r, db, demoDB)))
	}
}

// requestWithDB returns a copy of r carrying db in its context - the same
// mechanism withDB uses, exposed directly for tests that call a handler
// without going through NewMux/withDB.
func requestWithDB(r *http.Request, db *sql.DB) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), dbContextKey{}, db))
}

// dbFromContext returns the request's *sql.DB, set by withDB. Panics if
// missing - every route registered in NewMux must be wrapped in withDB, so a
// missing value is a wiring bug, not a normal runtime condition.
func dbFromContext(ctx context.Context) *sql.DB {
	db, ok := ctx.Value(dbContextKey{}).(*sql.DB)
	if !ok {
		panic("web: no *sql.DB in request context - handler not wrapped in withDB")
	}
	return db
}
