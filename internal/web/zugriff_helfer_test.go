package web

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// newZugang builds the production Zugang for a Vermieter password only (no
// Mieter login), the way the tests of the pages need it.
func newZugang(vermieter string, demoDB *sql.DB) *Zugang {
	return zugangFuer(auth{vermieter: vermieter, demoDB: demoDB})
}

// festeRolle is the test adapter of Sitzungen: every request has this role.
type festeRolle struct {
	rolle store.Rolle
	demo  bool
	sitz  bool
}

func (f festeRolle) Rolle(*http.Request) (store.Rolle, bool, bool) { return f.rolle, f.demo, f.sitz }

// matrixAus is the test adapter of Matrix: a fixed map or an injected error,
// and a count of the loads.
type matrixAus struct {
	b       store.Berechtigungen
	err     error
	geladen *int
}

func (m matrixAus) Lade(context.Context) (store.Berechtigungen, error) {
	if m.geladen != nil {
		*m.geladen++
	}
	return m.b, m.err
}

// berechtigtMit grants rechte to rolle and nothing else.
func berechtigtMit(rolle store.Rolle, rechte ...store.Recht) store.Berechtigungen {
	b := store.Berechtigungen{rolle: {}}
	for _, r := range rechte {
		b[rolle][r] = true
	}
	return b
}

func farFuture() time.Time { return time.Now().Add(time.Hour) }
