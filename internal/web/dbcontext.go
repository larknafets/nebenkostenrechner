package web

import (
	"context"
	"database/sql"
	"net/http"
)

// dbContextKey is the request-context key withDB stores the per-request
// *sql.DB under (Ticket #119, prefactoring for demo mode #118): NewMux
// used to hand every handler a single *sql.DB fixed at startup via closure -
// too rigid once a request must be able to pick the real vs. demo database
// depending on its session (next ticket). Handlers now read it via
// dbFromContext instead.
type dbContextKey struct{}

// selectDB picks demoDB when r carries a valid demo session cookie (Issue
// #120), db otherwise - the actual demo/real decision, pulled out of
// withDB (architecture review after demo mode #118, candidate 4) so it
// can be tested directly with a bare *http.Request, without needing a
// mux, a real handler, or real SQLite files.
func selectDB(r *http.Request, db, demoDB *sql.DB) *sql.DB {
	if isDemoSession(r) {
		return demoDB
	}
	return db
}

// withDB wraps next so it (and any helper it calls with r in scope) can read
// the request's *sql.DB via dbFromContext(r.Context()) - the only place in
// the app that branches demo/real database (via selectDB); every
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
