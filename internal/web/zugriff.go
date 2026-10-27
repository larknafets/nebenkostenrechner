package web

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// This file is the Zugriffs-Module (#316, ADR 0003): the one place that
// answers "may this role do that". Route gates, the navigation and the
// Dashboard ask it, none of them knows how a role is found (Sitzungen) or
// where the Berechtigungen are stored (Matrix).

// RolleVermieter is the role of the Vermieter (and of everyone when no
// Vermieter password is set, and of the demo session). Unlike the roles of
// store.Rollen it has no row in the Berechtigungen: the Vermieter may do
// everything and is never looked up.
const RolleVermieter store.Rolle = "vermieter"

// Sitzungen is the seam that turns a request into a role. Which cookie wins
// and that an app without Vermieter password makes everyone Vermieter is
// decided by the adapter (production: auth over cookies; tests: a fixed role).
type Sitzungen interface {
	// Rolle returns the role of r, whether it is a demo session and whether
	// it carries a session (cookie) at all.
	Rolle(r *http.Request) (rolle store.Rolle, demo, hatSitzung bool)
}

// Matrix is the seam that delivers the saved Berechtigungen (production: the
// store with the database of the request; tests: a map with an injectable
// error).
type Matrix interface {
	Lade(ctx context.Context) (store.Berechtigungen, error)
}

// storeMatrix is the production Matrix: the saved Berechtigungen of the
// database that withDB put into the context of the request.
type storeMatrix struct{}

func (storeMatrix) Lade(ctx context.Context) (store.Berechtigungen, error) {
	return store.GetBerechtigungen(dbFromContext(ctx))
}

// Zugang is built once when the server is built and answers every question
// about access. Logging in and out stays with auth.
type Zugang struct {
	sitzungen Sitzungen
	matrix    Matrix
	// login feeds the login entries of the navigation (which roles exist);
	// zero with the test adapters, then only the demo login exists.
	login auth
}

// NewZugang builds a Zugang from its two seams.
func NewZugang(s Sitzungen, m Matrix) *Zugang {
	return &Zugang{sitzungen: s, matrix: m}
}

// zugangFuer builds the production Zugang: sessions from the cookies of a,
// Berechtigungen from the store.
func zugangFuer(a auth) *Zugang {
	return &Zugang{sitzungen: a, matrix: storeMatrix{}, login: a}
}

// anspruchArt is what an Anspruch demands. The zero value is anspruchNurVermieter
// so that a forgotten value denies instead of opening a route.
type anspruchArt int

const (
	anspruchNurVermieter anspruchArt = iota
	anspruchBraucht
	anspruchOffen
	anspruchAnlegenOderVermieter
)

// Anspruch is what a route demands of the role: a Recht, the Vermieter or
// nothing. "Nur Vermieter" is a value of its own, not a pseudo Recht: the
// Berechtigungen hold only what is configurable (ADR 0003).
type Anspruch struct {
	art   anspruchArt
	recht store.Recht
}

// NurVermieter demands the Vermieter role.
var NurVermieter = Anspruch{art: anspruchNurVermieter}

// Offen demands nothing, everybody passes.
var Offen = Anspruch{art: anspruchOffen}

// AnlegenOderVermieter is the Anspruch of the routes that correct a reading
// (Teilstand, Ticket #130): it depends on the reading the request names. A
// Teilstand needs the Recht to create a reading (Braucht(RechtAblesungAnlegen)),
// a complete reading is for the Vermieter (NurVermieter). Gate resolves it per
// request (teilstandAnspruch); Erfuellt cannot, it has no request, and denies
// everybody but the Vermieter.
var AnlegenOderVermieter = Anspruch{art: anspruchAnlegenOderVermieter}

// Braucht demands the Recht r (the Vermieter always has it).
func Braucht(r store.Recht) Anspruch { return Anspruch{art: anspruchBraucht, recht: r} }

// matrixLauf loads the Berechtigungen at most once for one request.
type matrixLauf struct {
	once sync.Once
	m    Matrix
	ctx  context.Context
	b    store.Berechtigungen
	err  error
}

func (l *matrixLauf) get() (store.Berechtigungen, error) {
	l.once.Do(func() {
		l.b, l.err = l.m.Lade(l.ctx)
		if l.err != nil {
			log.Printf("berechtigungen: %v", l.err)
		}
	})
	return l.b, l.err
}

// Zugriff is the answer for one request: role and rights as flat facts the
// templates read directly ({{if .Vermieter}}). It has no error to handle:
// when the Berechtigungen cannot be loaded, every right of a non-Vermieter is
// false (fail closed) and Fehler names the cause for the callers that answer
// with an error page. The Berechtigungen are loaded lazily, at most once per
// request, and never for the Vermieter.
type Zugriff struct {
	Rolle store.Rolle
	// Vermieter is true for the Vermieter role, the demo session and every
	// visitor of an app without Vermieter password. It replaces IsLoggedIn.
	Vermieter bool
	// Demo is true during a demo session, HatSitzung when a session cookie is
	// valid (also without Vermieter password, where everyone is Vermieter).
	Demo       bool
	HatSitzung bool
	// The rights that decide which pages and buttons the navigation and the
	// pages show (#311). The Vermieter has all of them.
	KannAblesungenAnsehen bool
	KannAblesungAnlegen   bool
	KannFixkostenAnsehen  bool
	KannStammdatenAnsehen bool

	lade *matrixLauf
}

// Darf reports whether the role may use recht. The Vermieter rule lives here
// and nowhere else: the Vermieter may do everything and triggers no query.
func (z Zugriff) Darf(recht store.Recht) bool {
	if z.Vermieter {
		return true
	}
	if z.lade == nil {
		return false
	}
	b, err := z.lade.get()
	if err != nil {
		return false
	}
	return b.Erlaubt(z.Rolle, recht)
}

// Fehler returns why the Berechtigungen could not be loaded, nil otherwise
// (and always nil for the Vermieter).
func (z Zugriff) Fehler() error {
	if z.lade == nil {
		return nil
	}
	_, err := z.lade.get()
	return err
}

// Erfuellt reports whether the role meets a.
func (z Zugriff) Erfuellt(a Anspruch) bool {
	switch a.art {
	case anspruchOffen:
		return true
	case anspruchBraucht:
		return z.Darf(a.recht)
	}
	return z.Vermieter
}

// Personenbezug reports whether names, addresses and bank details may be
// shown: only for the Vermieter (Issue #164).
func (z Zugriff) Personenbezug() bool { return z.Vermieter }

// Sichtbar reports whether the role may see e on the Dashboard: the
// Vermieter everything, the others what the right for e grants. Whether e
// exists at all (the Wallboxen card) is up to the Dashboard.
func (z Zugriff) Sichtbar(e entitaet) bool {
	if z.Vermieter {
		return true
	}
	recht, ok := e.rechtFuer()
	return ok && z.Darf(recht)
}

// zugriffCacheKey carries a zugriffCache in the request context (set by
// requestWithDB, so by withDB).
type zugriffCacheKey struct{}

// zugriffCache keeps the Zugriff of one request, so the navigation, the gate
// and the Dashboard share one Matrix load.
type zugriffCache struct {
	mu      sync.Mutex
	von     *Zugang
	zugriff Zugriff
}

// Zugriff returns what the role of r may do.
func (z *Zugang) Zugriff(r *http.Request) Zugriff {
	c, ok := r.Context().Value(zugriffCacheKey{}).(*zugriffCache)
	if !ok {
		return z.bauen(r)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.von != z {
		c.zugriff, c.von = z.bauen(r), z
	}
	return c.zugriff
}

func (z *Zugang) bauen(r *http.Request) Zugriff {
	rolle, demo, hatSitzung := z.sitzungen.Rolle(r)
	zg := Zugriff{Rolle: rolle, Vermieter: rolle == RolleVermieter, Demo: demo, HatSitzung: hatSitzung}
	if !zg.Vermieter {
		zg.lade = &matrixLauf{m: z.matrix, ctx: r.Context()}
	}
	zg.KannAblesungenAnsehen = zg.Darf(store.RechtAblesungenAnsehen)
	zg.KannAblesungAnlegen = zg.Darf(store.RechtAblesungAnlegen)
	zg.KannFixkostenAnsehen = zg.Darf(store.RechtFixkostenAnsehen)
	zg.KannStammdatenAnsehen = zg.Darf(store.RechtStammdatenAnsehen)
	return zg
}

// Rolle is the cookie adapter of Sitzungen. Demo wins over Vermieter over
// Mieter (resolveSession); without a Vermieter password everyone is Vermieter
// (Ticket #112).
func (a auth) Rolle(r *http.Request) (rolle store.Rolle, demo, hatSitzung bool) {
	kind := a.resolveSession(r)
	demo, hatSitzung = kind == sitzungDemo, kind != sitzungKeine
	switch {
	case kind == sitzungDemo, kind == sitzungVermieter, a.vermieter == "":
		return RolleVermieter, demo, hatSitzung
	case kind == sitzungMieter:
		return store.RolleMieter, demo, hatSitzung
	}
	return store.RolleNichtAngemeldet, demo, hatSitzung
}

// NurVermieter gates next behind the Vermieter role.
func (z *Zugang) NurVermieter(next http.HandlerFunc) http.HandlerFunc {
	return z.Gate(NurVermieter, next)
}

// Recht gates next behind the right r.
func (z *Zugang) Recht(r store.Recht, next http.HandlerFunc) http.HandlerFunc {
	return z.Gate(Braucht(r), next)
}

// Gate gates next behind a. Without it a Mieter gets the page "Nur für den
// Vermieter" or "Nicht freigegeben", a visitor who is not logged in is
// bounced to the login overlay. When the Berechtigungen cannot be loaded and
// a needs them, the answer is 500 (fail closed). Only for routes wrapped in
// withDB (the Berechtigungen come from its database).
//
// The bounce always lands on the Dashboard, never back on the gated URL
// itself: a fully gated GET page (e.g. /ablesungen/neu) would loop forever,
// since a query param alone never grants access. The wanted URL travels
// along as "next" and is where a successful login lands (see
// loginRedirectTarget).
func (z *Zugang) Gate(a Anspruch, next http.HandlerFunc) http.HandlerFunc {
	if a.art == anspruchOffen {
		// Nothing to check, so no Berechtigungen are loaded either.
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		a := a // resolved per request, never written back to the shared value
		if a.art == anspruchAnlegenOderVermieter {
			// The id is looked up first: a missing reading answers 404 and an
			// invalid id 400 before any right is checked.
			resolved, ok := teilstandAnspruch(w, r)
			if !ok {
				return
			}
			a = resolved
		}
		zg := z.Zugriff(r)
		if zg.Erfuellt(a) {
			next(w, r)
			return
		}
		if a.art == anspruchBraucht {
			if err := zg.Fehler(); err != nil {
				http.Error(w, fmt.Sprintf("berechtigungen: %v", err), http.StatusInternalServerError)
				return
			}
		}
		wantedPath := r.URL.Path
		if r.URL.RawQuery != "" {
			wantedPath += "?" + r.URL.RawQuery
		}
		// A gated POST for the Vermieter (e.g. .../loeschen) is never itself
		// worth landing on after login: fall back to the page the form was
		// submitted from, if any.
		if a.art == anspruchNurVermieter && r.Method != http.MethodGet {
			wantedPath = refererPath(r)
		}
		if zg.Rolle == store.RolleMieter {
			if a.art == anspruchBraucht {
				z.keineBerechtigungSeite(w, r, wantedPath, "Nicht freigegeben", "Der Vermieter hat diese Seite für dich nicht freigegeben.")
			} else {
				z.keineBerechtigungSeite(w, r, wantedPath, "Nur für den Vermieter", "Diese Seite ist dem Vermieter vorbehalten. Als Mieter hast du hier keinen Zugriff.")
			}
			return
		}
		redirectToLoginOverlay(w, r, wantedPath, "", false)
	}
}

// keineBerechtigungSeite answers a logged-in Mieter who asked for a page
// reserved for the Vermieter or not granted to him: 403 with a link that opens
// the login as Vermieter and leads back to the wanted page.
func (z *Zugang) keineBerechtigungSeite(w http.ResponseWriter, r *http.Request, wanted, titel, text string) {
	if wanted == "" || !strings.HasPrefix(wanted, "/") || strings.HasPrefix(wanted, "//") {
		wanted = "/dashboard"
	}
	data := struct {
		navData
		Aktuell string
		Next    string
		Titel   string
		Text    string
	}{navData: z.NavData(r), Next: wanted, Titel: titel, Text: text}
	w.WriteHeader(http.StatusForbidden)
	if err := keineBerechtigungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("keine berechtigung: %v", err)
	}
}

// NavData builds navData for the current request - the single place the
// Zugriff, requestBase and the login entries are put together.
//
// The login entry shows for everybody but the Vermieter session and the demo
// session, so a Mieter can switch to the Vermieter and the (otherwise open)
// app keeps a visible path to the demo login (Issue #122). The logout entry
// needs a real session to end: with no Vermieter password everyone counts as
// Vermieter, but there is no session to log out of.
func (z *Zugang) NavData(r *http.Request) navData {
	zg := z.Zugriff(r)
	return navData{
		Zugriff:         zg,
		Base:            requestBase(r),
		IsDemoSession:   zg.Demo,
		ShowLoginEntry:  !zg.HatSitzung || !zg.Vermieter,
		ShowLogoutEntry: zg.HatSitzung,
		LoginOptionen:   z.login.loginOptionen(),
		HatMieter:       z.login.mieterAktiv(),
	}
}
