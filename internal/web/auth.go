package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

const (
	// sessionCookieName is the cookie of the Vermieter session, mieterSessionCookieName
	// the one of the Mieter session (ADR 0003): one cookie per role, each signed
	// with the password of that role.
	sessionCookieName       = "nk_session"
	mieterSessionCookieName = "nk_session_mieter"
	sessionTTL              = 30 * 24 * time.Hour

	// demoSessionCookieName marks a visitor's session as a demo session
	// (Issue #120) - a separate cookie from the real ones, so the real
	// password mechanism stays completely untouched. A demo visitor also
	// counts as Vermieter for UI/routing purposes (see auth.istVermieter);
	// this cookie additionally tells withDB which *sql.DB a request should
	// use.
	demoSessionCookieName = "nk_demo_session"

	// demoSessionSecret signs the demo session cookie. Not a real secret -
	// the demo login needs no password - just reuses signSession's HMAC
	// mechanism so a visitor can't forge/tamper the marker.
	demoSessionSecret = "nebenkostenrechner-demo-session"

	// The values of the login form's "rolle" select (ADR 0003).
	rolleFormMieter    = "mieter"
	rolleFormVermieter = "vermieter"
	rolleFormDemo      = "demo"
)

// ladeKennwoerter reads the two optional passwords (ADR 0003):
// VERMIETER_PASSWORD and MIETER_PASSWORD from the environment (Docker/.env),
// falling back per password to the Home Assistant Supervisor API when unset -
// Supervisor does not map addon options onto container env vars itself, and
// this app's distroless image has no shell for the usual bashio/run.sh
// workaround (see larknafets/ha-addons#3). The Supervisor call needs no
// filesystem access (an earlier read of /data/options.json failed with
// "permission denied" for the nonroot container user). Read once at startup,
// like DB_PATH/LISTEN_ADDR.
//
// Empty vermieter means the login system stays disabled - everything
// visible, everyone is Vermieter. A Mieter password without a Vermieter
// password is ignored (it would make everyone Vermieter anyway).
func ladeKennwoerter() (vermieter, mieter string) {
	vermieter = os.Getenv("VERMIETER_PASSWORD")
	mieter = os.Getenv("MIETER_PASSWORD")
	if os.Getenv("LOGIN_PASSWORD") != "" && vermieter == "" {
		log.Print("login: LOGIN_PASSWORD wird nicht mehr gelesen, das Vermieter-Kennwort heisst jetzt VERMIETER_PASSWORD - die App ist ohne Vermieter-Kennwort offen")
	}
	if token := os.Getenv("SUPERVISOR_TOKEN"); token != "" && (vermieter == "" || mieter == "") {
		v, m, err := fetchSupervisorKennwoerter(token)
		if err != nil {
			log.Printf("login: Supervisor API call failed (%v) - passwords stay empty", err)
		} else {
			if vermieter == "" {
				vermieter = v
			}
			if mieter == "" {
				mieter = m
			}
		}
	}
	if mieter != "" && vermieter == "" {
		log.Print("login: MIETER_PASSWORD ohne VERMIETER_PASSWORD wird ignoriert, die App bleibt offen")
		mieter = ""
	}
	log.Printf("login: Vermieter-Kennwort (Laenge %d), Mieter-Kennwort (Laenge %d)", len(vermieter), len(mieter))
	return vermieter, mieter
}

// fetchSupervisorKennwoerter calls the Supervisor's internal
// "/addons/self/info" API (only reachable from inside the addon container,
// requires "hassio_api: true" in config.yaml) and extracts the currently
// configured "vermieter_password" and "mieter_password" options. The options
// object in this response is the addon's own, unredacted configuration -
// unlike the equivalent "/addons/<slug>/info" call from outside, which
// redacts it.
func fetchSupervisorKennwoerter(token string) (vermieter, mieter string, err error) {
	req, err := http.NewRequest(http.MethodGet, "http://supervisor/addons/self/info", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Options struct {
				VermieterPassword string `json:"vermieter_password"`
				MieterPassword    string `json:"mieter_password"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", "", err
	}
	return body.Data.Options.VermieterPassword, body.Data.Options.MieterPassword, nil
}

// signSession returns a session cookie value valid until expiry:
// "<unix-expiry>.<hex-hmac>". Stateless (no server-side session store, so it
// survives restarts) and keyed by secret itself - changing the configured
// password invalidates every outstanding session automatically, since the
// HMAC key changes with it.
func signSession(secret string, expiry time.Time) string {
	exp := strconv.FormatInt(expiry.Unix(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(exp))
	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

// verifySession checks a session cookie value against secret: well-formed,
// not expired, and HMAC-valid (constant-time comparison).
func verifySession(secret, value string) bool {
	exp, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	expUnix, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expUnix {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(exp))
	wantSig := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(wantSig))
}

// sessionKind is one of the 3 cookie-based session mechanisms this app
// knows (Vermieter, Mieter, demo login) - same shape (stateless HMAC-signed
// cookie, sessionTTL), differing only in cookie name and signing secret.
// Extracting this (architecture review after demo mode #118, candidate 3)
// replaces what used to be 4 hand-written, near-identical set/clear
// functions with one set of methods.
type sessionKind struct {
	cookieName string
	secret     string
}

// demoKind is the demo session's cookie/secret pair - fixed regardless of
// the passwords, since the demo login always works (Issue #118
// implementation decision).
var demoKind = sessionKind{cookieName: demoSessionCookieName, secret: demoSessionSecret}

// vermieterKind and mieterKind are the real logins' cookie/secret pairs for
// this auth instance - the secret is the password of the role, so an empty
// password's "login disabled" meaning is still entirely the caller's concern,
// not sessionKind's.
func (a auth) vermieterKind() sessionKind {
	return sessionKind{cookieName: sessionCookieName, secret: a.vermieter}
}

func (a auth) mieterKind() sessionKind {
	return sessionKind{cookieName: mieterSessionCookieName, secret: a.mieter}
}

// active reports whether r carries a valid, unexpired cookie for this kind.
func (k sessionKind) active(r *http.Request) bool {
	c, err := r.Cookie(k.cookieName)
	if err != nil {
		return false
	}
	return verifySession(k.secret, c.Value)
}

// set logs the visitor into this kind for sessionTTL. HttpOnly (never
// readable from JS) and SameSite=Lax (the app only ever runs same-origin,
// whether direct or behind HA-Ingress - no cross-site posting scenario to
// guard against, see Ticket #112). No Secure attribute: the app has no
// notion of its own whether it's reached over TLS (HA-Ingress may terminate
// TLS in front of it), and Secure isn't required for the Same-Origin-Proxy
// setup Ingress uses.
func (k sessionKind) set(w http.ResponseWriter) {
	expiry := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     k.cookieName,
		Value:    signSession(k.secret, expiry),
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clear logs the visitor out of this kind.
func (k sessionKind) clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     k.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// isDemoSession reports whether r carries a valid demo session cookie -
// independent of the passwords. Kept as its own entry point (rather than
// folded into resolveSession) since selectDB (dbcontext.go) needs exactly
// this, with no passwords in scope and no interest in real-session precedence.
func isDemoSession(r *http.Request) bool {
	return demoKind.active(r)
}

// Session kinds as resolveSession reports them.
const (
	sitzungDemo      = "demo"
	sitzungVermieter = "vermieter"
	sitzungMieter    = "mieter"
	sitzungKeine     = "none"
)

// mieterAktiv reports whether the Mieter login exists: it needs its own
// password and a Vermieter password (without one everyone is Vermieter).
func (a auth) mieterAktiv() bool {
	return a.vermieter != "" && a.mieter != ""
}

// resolveSession is the one place session precedence is decided: demo wins
// when several cookies happen to be valid at once, then Vermieter, then Mieter.
func (a auth) resolveSession(r *http.Request) string {
	switch {
	case demoKind.active(r):
		return sitzungDemo
	case a.vermieter != "" && a.vermieterKind().active(r):
		return sitzungVermieter
	case a.mieterAktiv() && a.mieterKind().active(r):
		return sitzungMieter
	}
	return sitzungKeine
}

// istVermieter reports whether r counts as Vermieter - always true when no
// Vermieter password is configured (the "login system disabled" case from
// Ticket #112), or when resolveSession finds a demo or Vermieter session.
func (a auth) istVermieter(r *http.Request) bool {
	if a.vermieter == "" {
		return true
	}
	kind := a.resolveSession(r)
	return kind == sitzungVermieter || kind == sitzungDemo
}

// loginOption is one entry of the login form's "Anmelden als" select.
type loginOption struct {
	Wert  string
	Label string
}

// loginOptionen lists the roles one can log in as: Vermieter and Mieter only
// when their password is set, Demo always. The first entry is preselected.
func (a auth) loginOptionen() []loginOption {
	var out []loginOption
	if a.mieterAktiv() {
		out = append(out, loginOption{rolleFormMieter, "Mieter"})
	}
	if a.vermieter != "" {
		out = append(out, loginOption{rolleFormVermieter, "Vermieter"})
	}
	return append(out, loginOption{rolleFormDemo, "Demo"})
}

// navData is every page's shared nav facts - Base plus the 3 login/demo
// facts, computed once per request instead of separately in each of the 9
// page-handlers. Embedded (anonymous field) in every page's template-data
// struct: text/template promotes embedded-struct fields for dot-access, so
// layout.html keeps reading .Base/.IsLoggedIn/.IsDemoSession/.ShowLoginEntry
// unchanged, whether they come directly from a struct or via this embed.
// Aktuell (the active nav tab) deliberately stays out - each handler names
// its own page, a constructor here can't derive that for it.
type navData struct {
	Base string
	// IsLoggedIn is true for the Vermieter (and the demo session, which
	// shows the Vermieter's view); the templates gate everything reserved for
	// the Vermieter on it.
	IsLoggedIn      bool
	IsDemoSession   bool
	ShowLoginEntry  bool
	ShowLogoutEntry bool
	LoginOptionen   []loginOption
	// HatMieter is true when the Mieter login exists (see mieterAktiv).
	HatMieter bool
	// The rights that decide which pages and buttons the nav and the pages
	// show (#311). The Vermieter has all of them.
	KannAblesungenAnsehen bool
	KannAblesungAnlegen   bool
	KannFixkostenAnsehen  bool
	KannStammdatenAnsehen bool
}

// auth bundles what every login-related decision needs: the two configured
// passwords and the demo database (needed to reset it on a successful demo
// login). Built once in NewMux and passed along ever since, instead of the
// password as a bare string through ~13 route registrations and 11 handler
// constructors - architecture review after demo mode (#118), candidate 2.
type auth struct {
	vermieter string
	mieter    string
	demoDB    *sql.DB
}

// newAuth builds an auth with a Vermieter password only (no Mieter login).
func newAuth(vermieter string, demoDB *sql.DB) auth {
	return auth{vermieter: vermieter, demoDB: demoDB}
}

// NavData builds navData for the current request - the single place
// requestBase/session resolution are called together, replacing what used to
// be duplicated across all 9 page-handlers.
//
// The login entry shows for everybody but the Vermieter and the demo
// session, so a Mieter can switch to the Vermieter and the (otherwise open)
// app keeps a visible path to the demo login (Issue #122). The logout entry
// needs a real session to end: with no Vermieter password everyone counts as
// Vermieter, but there is no session to log out of.
func (a auth) NavData(r *http.Request) navData {
	kind := a.resolveSession(r)
	rechte := a.rechte(r)
	return navData{
		Base:            requestBase(r),
		IsLoggedIn:      a.istVermieter(r),
		IsDemoSession:   kind == sitzungDemo,
		ShowLoginEntry:  kind != sitzungDemo && kind != sitzungVermieter,
		ShowLogoutEntry: kind != sitzungKeine,
		LoginOptionen:   a.loginOptionen(),
		HatMieter:       a.mieterAktiv(),

		KannAblesungenAnsehen: rechte.darf(store.RechtAblesungenAnsehen),
		KannAblesungAnlegen:   rechte.darf(store.RechtAblesungAnlegen),
		KannFixkostenAnsehen:  rechte.darf(store.RechtFixkostenAnsehen),
		KannStammdatenAnsehen: rechte.darf(store.RechtStammdatenAnsehen),
	}
}

// RequireLogin gates a mutating or create-only route behind the Vermieter
// (Ticket #112's enforcement matrix): with no Vermieter password configured
// every route stays open (istVermieter always true). Otherwise an
// unauthenticated request bounces to the login overlay (Ticket #113), and a
// logged-in Mieter gets the page "Nur für den Vermieter" (ADR 0003).
//
// The bounce always lands on the Dashboard, never back on the gated URL
// itself - a fully gated GET page (e.g. /ablesungen/neu) is itself wrapped
// in RequireLogin, so redirecting to itself with ?login=1 would just hit
// RequireLogin again and loop forever, since a query param alone never
// grants access. The Dashboard is the one page never behind RequireLogin,
// so it's always safe to land on; the originally attempted URL travels
// along as "next" and is where a *successful* login lands instead (see
// loginRedirectTarget).
func (a auth) RequireLogin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.istVermieter(r) {
			next(w, r)
			return
		}
		wantedPath := r.URL.Path
		if r.URL.RawQuery != "" {
			wantedPath += "?" + r.URL.RawQuery
		}
		if r.Method != http.MethodGet {
			// A gated POST (e.g. .../loeschen) is never itself worth
			// landing on after login - fall back to the page the form was
			// submitted from, if any.
			wantedPath = refererPath(r)
		}
		if a.resolveSession(r) == sitzungMieter {
			a.keineBerechtigungSeite(w, r, wantedPath, "Nur für den Vermieter", "Diese Seite ist dem Vermieter vorbehalten. Als Mieter hast du hier keinen Zugriff.")
			return
		}
		redirectToLoginOverlay(w, r, wantedPath, "", false)
	}
}

// rechteSicht is what the role of one request may do: the Vermieter everything,
// the others what the saved Berechtigungen grant.
type rechteSicht struct {
	vermieter bool
	erlaubt   map[store.Recht]bool
}

func (s rechteSicht) darf(recht store.Recht) bool {
	return s.vermieter || s.erlaubt[recht]
}

// betrachter maps the session of r to the role whose view applies.
func (a auth) betrachter(r *http.Request) betrachter {
	switch {
	case a.istVermieter(r):
		return betrachterVermieter
	case a.resolveSession(r) == sitzungMieter:
		return betrachterMieter
	}
	return betrachterNichtAngemeldet
}

// rechte reads the rights of the role of r. Without a database in the context
// or on a read error the defaults apply, a Berechtigung read must not take a
// page down.
func (a auth) rechte(r *http.Request) rechteSicht {
	b := a.betrachter(r)
	if b == betrachterVermieter {
		return rechteSicht{vermieter: true}
	}
	berechtigungen := store.DefaultBerechtigungen()
	if db := dbFromContextOptional(r.Context()); db != nil {
		if gelesen, err := store.GetBerechtigungen(db); err != nil {
			log.Printf("berechtigungen: %v", err)
		} else {
			berechtigungen = gelesen
		}
	}
	return rechteSicht{erlaubt: berechtigungen[b.rolle()]}
}

// RequireRecht gates a route behind a right (#311): the Vermieter and whoever
// has the right pass. Without the right a Mieter gets the page "Nicht
// freigegeben" and a visitor who is not logged in is bounced to the login
// overlay, as for the pages reserved for the Vermieter. Only for routes
// wrapped in withDB (the rights come from its database).
func (a auth) RequireRecht(recht store.Recht, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a.rechte(r).darf(recht) {
			next(w, r)
			return
		}
		wantedPath := r.URL.Path
		if r.URL.RawQuery != "" {
			wantedPath += "?" + r.URL.RawQuery
		}
		if a.resolveSession(r) == sitzungMieter {
			a.keineBerechtigungSeite(w, r, wantedPath, "Nicht freigegeben", "Der Vermieter hat diese Seite für dich nicht freigegeben.")
			return
		}
		redirectToLoginOverlay(w, r, wantedPath, "", false)
	}
}

// refererPath extracts just the path(+query) off the Referer header - never
// its scheme/host, so an unexpected cross-origin Referer can't smuggle
// anything past loginRedirectTarget's same-origin-only check downstream.
func refererPath(r *http.Request) string {
	u, err := url.Parse(r.Referer())
	if err != nil || u.Path == "" {
		return ""
	}
	if u.RawQuery != "" {
		return u.Path + "?" + u.RawQuery
	}
	return u.Path
}

// redirectToLoginOverlay bounces to the Dashboard with the Login-Overlay
// flagged open (?login=1[&fehler=1]) and next carrying where a successful
// login should actually land - see requireLogin for why it's always the
// Dashboard, never the originally attempted URL.
func redirectToLoginOverlay(w http.ResponseWriter, r *http.Request, next, rolle string, fehler bool) {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/dashboard"
	}
	target := requestBase(r) + "/dashboard?login=1&next=" + url.QueryEscape(next)
	if rolle != "" {
		target += "&rolle=" + url.QueryEscape(rolle)
	}
	if fehler {
		target += "&fehler=1"
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// keineBerechtigungSeite answers a logged-in Mieter who asked for a page
// reserved for the Vermieter or not granted to him: 403 with a link that opens
// the login as Vermieter and leads back to the wanted page.
func (a auth) keineBerechtigungSeite(w http.ResponseWriter, r *http.Request, wanted, titel, text string) {
	if wanted == "" || !strings.HasPrefix(wanted, "/") || strings.HasPrefix(wanted, "//") {
		wanted = "/dashboard"
	}
	data := struct {
		navData
		Aktuell string
		Next    string
		Titel   string
		Text    string
	}{navData: a.NavData(r), Next: wanted, Titel: titel, Text: text}
	w.WriteHeader(http.StatusForbidden)
	if err := keineBerechtigungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("keine berechtigung: %v", err)
	}
}

// loginRedirectTarget resolves where to send the browser after a
// *successful* login (or a logout): the "next" hidden field the Overlay
// fills in (Ticket #113), falling back to the Dashboard when absent
// (logout - see nav's Logout-Formular) or malformed. Only ever a
// same-origin path, never a full URL, to rule out open-redirect abuse via a
// crafted "next" value.
func loginRedirectTarget(r *http.Request) string {
	next := r.FormValue("next")
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return requestBase(r) + "/dashboard"
	}
	return requestBase(r) + next
}

// HandleLogin logs in as the role the form's "rolle" select names (ADR 0003):
// Mieter and Vermieter check the password of that role, Demo needs none.
func (a auth) HandleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}
		rolle := r.FormValue("rolle")
		fehler := func() { redirectToLoginOverlay(w, r, r.FormValue("next"), rolle, true) }
		passt := func(want string) bool {
			return subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte(want)) == 1
		}
		switch rolle {
		case rolleFormDemo:
			// The demo login needs no password and always works, even when
			// no password is configured (Issue #118). It resets a.demoDB to
			// its fresh 39-month initial state on every login (Issue #121) -
			// before setting the session cookie, so a granted demo session
			// always sees the fresh state, never that of a previous session.
			if err := store.ResetDemoData(a.demoDB, time.Now()); err != nil {
				http.Error(w, "demo reset: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// A still-valid real session cookie must not be left lying
			// around: a clean login switch cleans up all sides instead of
			// relying solely on the precedence in resolveSession.
			a.vermieterKind().clear(w)
			a.mieterKind().clear(w)
			demoKind.set(w)
		case rolleFormVermieter:
			if a.vermieter == "" {
				// Login system disabled, everyone is Vermieter already.
				break
			}
			if !passt(a.vermieter) {
				fehler()
				return
			}
			// Symmetric to the demo branch: no other session may override a
			// fresh login.
			demoKind.clear(w)
			a.mieterKind().clear(w)
			a.vermieterKind().set(w)
		case rolleFormMieter:
			if !a.mieterAktiv() || !passt(a.mieter) {
				fehler()
				return
			}
			demoKind.clear(w)
			a.vermieterKind().clear(w)
			a.mieterKind().set(w)
		default:
			fehler()
			return
		}
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}

func (a auth) HandleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.vermieterKind().clear(w)
		a.mieterKind().clear(w)
		demoKind.clear(w)
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}
