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
	sessionCookieName = "nk_session"
	sessionTTL        = 30 * 24 * time.Hour

	// demoSessionCookieName marks a visitor's session as a demo session
	// (Issue #120) - a separate cookie from sessionCookieName, so the real
	// login password mechanism (secret) stays completely untouched. A demo
	// visitor also counts as "logged in" for UI/routing purposes (see
	// isLoggedIn); this cookie additionally tells withDB which *sql.DB a
	// request should use.
	demoSessionCookieName = "nk_demo_session"

	// demoPassword is hardcoded in the code (Issue #118 implementation
	// decision) - no ENV/config, always works regardless of whether
	// LOGIN_PASSWORD is set.
	demoPassword = "demo"

	// demoSessionSecret signs the demo session cookie. Not a real secret -
	// demoPassword itself is public/hardcoded - just reuses signSession's
	// HMAC mechanism so a visitor can't forge/tamper the marker.
	demoSessionSecret = "nebenkostenrechner-demo-session"
)

// resolveLoginPassword reads the optional login password: LOGIN_PASSWORD env
// var first (Docker/.env), falling back to the Home Assistant Supervisor API
// when unset - Supervisor does not map addon options onto container env vars
// itself, and this app's distroless image has no shell for the usual
// bashio/run.sh workaround (see larknafets/ha-addons#3). An earlier version
// of this fallback read /data/options.json directly, but that file is
// root-owned and unreadable by this image's nonroot container user
// (confirmed via addon log: "permission denied") - the Supervisor API call
// needs no filesystem access at all. Read once at startup, like
// DB_PATH/LISTEN_ADDR. Empty means the login system stays disabled -
// everything visible.
func resolveLoginPassword() string {
	if pw := os.Getenv("LOGIN_PASSWORD"); pw != "" {
		log.Printf("login: LOGIN_PASSWORD env set (length %d)", len(pw))
		return pw
	}
	token := os.Getenv("SUPERVISOR_TOKEN")
	if token == "" {
		return ""
	}
	pw, err := fetchSupervisorLoginPassword(token)
	if err != nil {
		log.Printf("login: Supervisor API call failed (%v) - login password stays empty", err)
		return ""
	}
	log.Printf("login: read login_password from Supervisor API (length %d)", len(pw))
	return pw
}

// fetchSupervisorLoginPassword calls the Supervisor's internal
// "/addons/self/info" API (only reachable from inside the addon container,
// requires "hassio_api: true" in config.yaml) and extracts the currently
// configured "login_password" option. The options object in this response
// is the addon's own, unredacted configuration - unlike the equivalent
// "/addons/<slug>/info" call from outside, which redacts it.
func fetchSupervisorLoginPassword(token string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "http://supervisor/addons/self/info", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var body struct {
		Data struct {
			Options struct {
				LoginPassword string `json:"login_password"`
			} `json:"options"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.Data.Options.LoginPassword, nil
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

// sessionKind is one of the 2 cookie-based session mechanisms this app
// knows (real login vs demo login) - same shape (stateless HMAC-signed
// cookie, sessionTTL), differing only in cookie name and signing secret.
// Extracting this (architecture review after demo mode #118, candidate 3)
// replaces what used to be 4 hand-written, near-identical set/clear
// functions with one set of methods.
type sessionKind struct {
	cookieName string
	secret     string
}

// demoKind is the demo session's cookie/secret pair - fixed regardless of
// LOGIN_PASSWORD, since the demo login always works (Issue #118
// implementation decision).
var demoKind = sessionKind{cookieName: demoSessionCookieName, secret: demoSessionSecret}

// realKind is the real login's cookie/secret pair for this auth instance -
// secret is a.secret, so an empty secret's "login disabled" meaning is
// still entirely a's caller's concern, not sessionKind's.
func (a auth) realKind() sessionKind {
	return sessionKind{cookieName: sessionCookieName, secret: a.secret}
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
// independent of secret/LOGIN_PASSWORD. Kept as its own entry point (rather
// than folded into resolveSession) since selectDB (dbcontext.go) and
// HandleLogin's demo-password branch need exactly this, with no secret in
// scope and no interest in real-session precedence.
func isDemoSession(r *http.Request) bool {
	return demoKind.active(r)
}

// resolveSession is the one place session precedence is decided: "demo",
// "real", or "none" - demo wins when both cookies happen to be valid at
// once. isLoggedIn and demoNavFlags both read from this instead of each
// re-deriving the same precedence.
func resolveSession(r *http.Request, secret string) string {
	if demoKind.active(r) {
		return "demo"
	}
	if secret != "" && (sessionKind{cookieName: sessionCookieName, secret: secret}).active(r) {
		return "real"
	}
	return "none"
}

// isLoggedIn reports whether r carries a valid session - always true when
// no password is configured (secret == ""), the "login system disabled"
// case from Ticket #112, or when resolveSession finds a valid demo or real
// session.
func isLoggedIn(r *http.Request, secret string) bool {
	return secret == "" || resolveSession(r, secret) != "none"
}

// demoNavFlags computes the 3 demo-mode-related nav facts that every
// page needs via the shared "nav" partial (Issue #122): isDemo
// (controls the demo banner), showLoginEntry (visibility of the "Login"
// entry point into the login overlay) and showLogoutEntry (visibility of
// the "Logout" link). showLoginEntry is shown as soon as the visitor is
// neither in a demo session nor logged in with a real, configured
// LOGIN_PASSWORD - covers both the classic "not logged in" case (secret
// set, no valid cookie) and the secret == "" case, where isLoggedIn is
// unconditionally true (everything open) but there's still a need for a
// visible path to the demo login. showLogoutEntry is deliberately NOT
// simply isLoggedIn: with secret == "", isLoggedIn is always true
// (everything open) even though no session exists to log out of -
// "Logout" only makes sense with a real session (realLogin) or a demo
// session.
func demoNavFlags(r *http.Request, secret string) (isDemo, showLoginEntry, showLogoutEntry bool) {
	kind := resolveSession(r, secret)
	isDemo = kind == "demo"
	realLogin := kind == "real"
	return isDemo, !isDemo && !realLogin, isDemo || realLogin
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
	Base            string
	IsLoggedIn      bool
	IsDemoSession   bool
	ShowLoginEntry  bool
	ShowLogoutEntry bool
}

// auth bundles the 2 facts every login-related decision needs: the
// configured login password (secret) and the demo database (needed to
// reset it on a successful demo login). Built once in NewMux (newAuth)
// and passed along ever since, instead of secret as a bare string through
// ~13 route registrations and 11 handler constructors - architecture
// review after demo mode (#118), candidate 2.
type auth struct {
	secret string
	demoDB *sql.DB
}

func newAuth(secret string, demoDB *sql.DB) auth {
	return auth{secret: secret, demoDB: demoDB}
}

// NavData builds navData for the current request - the single place
// requestBase/isLoggedIn/demoNavFlags are called together, replacing what
// used to be duplicated across all 9 page-handlers.
func (a auth) NavData(r *http.Request) navData {
	isDemo, showLoginEntry, showLogoutEntry := demoNavFlags(r, a.secret)
	return navData{
		Base:            requestBase(r),
		IsLoggedIn:      isLoggedIn(r, a.secret),
		IsDemoSession:   isDemo,
		ShowLoginEntry:  showLoginEntry,
		ShowLogoutEntry: showLogoutEntry,
	}
}

// RequireLogin gates a mutating or create-only route behind the
// login password (Ticket #112's enforcement matrix): with no password
// configured every route stays open (isLoggedIn always true), otherwise an
// unauthenticated request bounces to the login overlay (Ticket #113).
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
		if isLoggedIn(r, a.secret) {
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
		redirectToLoginOverlay(w, r, wantedPath, false)
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
func redirectToLoginOverlay(w http.ResponseWriter, r *http.Request, next string, fehler bool) {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		next = "/dashboard"
	}
	target := requestBase(r) + "/dashboard?login=1&next=" + url.QueryEscape(next)
	if fehler {
		target += "&fehler=1"
	}
	http.Redirect(w, r, target, http.StatusFound)
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

// HandleLogin's demo branch resets a.demoDB to its fresh 39-month
// initial state on every successful demo login (Issue #121) - before
// setting the session cookie, so a granted demo session always sees the
// fresh state, never that of a previous session.
func (a auth) HandleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}
		// demoPassword is a special case, checked before the regular
		// secret comparison - always works, even when secret == ""
		// (Issue #118 implementation decision).
		if r.FormValue("password") == demoPassword {
			if err := store.ResetDemoData(a.demoDB, time.Now()); err != nil {
				http.Error(w, "demo reset: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// A still-valid real session cookie must not be left lying
			// around - isDemoSession is checked before the real session
			// (isLoggedIn/withDB), but a clean login switch cleans up
			// both sides instead of relying solely on that check
			// ordering.
			a.realKind().clear(w)
			demoKind.set(w)
			http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
			return
		}
		if a.secret == "" {
			http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.FormValue("password")), []byte(a.secret)) != 1 {
			redirectToLoginOverlay(w, r, r.FormValue("next"), true)
			return
		}
		// Symmetric to the demo branch above: a still-valid demo session
		// cookie must not override a fresh real login.
		demoKind.clear(w)
		a.realKind().set(w)
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}

func (a auth) HandleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a.realKind().clear(w)
		demoKind.clear(w)
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}
