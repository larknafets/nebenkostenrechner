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

	// demoSessionCookieName marks a visitor's session as a Demo-Session
	// (Issue #120) - a separate cookie from sessionCookieName, so the real
	// Login-Kennwort mechanism (secret) stays completely untouched. A demo
	// visitor also counts as "logged in" for UI/Routen-Zwecke (see
	// isLoggedIn); this cookie additionally tells withDB which *sql.DB a
	// request should use.
	demoSessionCookieName = "nk_demo_session"

	// demoPassword is fest im Code verankert (Issue #118 Implementation
	// Decision) - kein ENV/Config, funktioniert immer, unabhängig davon ob
	// LOGIN_PASSWORD gesetzt ist.
	demoPassword = "demo"

	// demoSessionSecret signs the Demo-Session-Cookie. Not a real secret -
	// demoPassword itself is public/hardcoded - just reuses signSession's
	// HMAC-Mechanismus so a visitor can't forge/tamper the marker.
	demoSessionSecret = "nebenkostenrechner-demo-session"
)

// resolveLoginPassword reads the optional Login-Kennwort: LOGIN_PASSWORD env
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
// Kennwort invalidates every outstanding session automatically, since the
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

// isLoggedIn reports whether r carries a valid session - always true when
// no Kennwort is configured (secret == ""), the "Login-System deaktiviert"
// case from Ticket #112, or when r carries a valid Demo-Session (Issue
// #120) - a demo visitor gets full UI/Routen-Zugriff like a regulär
// eingeloggter Nutzer, unabhängig davon ob LOGIN_PASSWORD gesetzt ist.
func isLoggedIn(r *http.Request, secret string) bool {
	if secret == "" || isDemoSession(r) {
		return true
	}
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return verifySession(secret, c.Value)
}

// setSessionCookie logs the visitor in for sessionTTL. HttpOnly (never
// readable from JS) and SameSite=Lax (the app only ever runs same-origin,
// whether direct or behind HA-Ingress - no cross-site posting scenario to
// guard against, see Ticket #112). No Secure attribute: the app has no
// notion of its own whether it's reached over TLS (HA-Ingress may terminate
// TLS in front of it), and Secure isn't required for the Same-Origin-Proxy
// setup Ingress uses.
func setSessionCookie(w http.ResponseWriter, secret string) {
	expiry := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    signSession(secret, expiry),
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie logs the visitor out.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// setDemoSessionCookie logs the visitor into den Demo-Modus (Issue #120)
// for sessionTTL - gleiche Form wie setSessionCookie, nur mit
// demoSessionSecret statt dem echten secret signiert.
func setDemoSessionCookie(w http.ResponseWriter) {
	expiry := time.Now().Add(sessionTTL)
	http.SetCookie(w, &http.Cookie{
		Name:     demoSessionCookieName,
		Value:    signSession(demoSessionSecret, expiry),
		Path:     "/",
		Expires:  expiry,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearDemoSessionCookie logs the visitor out of dem Demo-Modus.
func clearDemoSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     demoSessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// isDemoSession reports whether r carries a valid Demo-Session-Cookie -
// unabhängig von secret/LOGIN_PASSWORD, da der Demo-Login immer
// funktioniert (Issue #118 Implementation Decision).
func isDemoSession(r *http.Request) bool {
	c, err := r.Cookie(demoSessionCookieName)
	if err != nil {
		return false
	}
	return verifySession(demoSessionSecret, c.Value)
}

// demoNavFlags computes die 3 Demo-Modus-bezogenen Nav-Fakten, die jede
// Seite über die gemeinsame "nav"-Partial braucht (Issue #122): isDemo
// (steuert den Demo-Banner), showLoginEntry (Sichtbarkeit des "Anmelden"-
// Einstiegspunkts ins Login-Overlay) und showLogoutEntry (Sichtbarkeit des
// "Abmelden"-Links). showLoginEntry wird gezeigt, sobald der Besucher weder
// in einer Demo-Session ist noch mit einem echten, konfigurierten
// LOGIN_PASSWORD eingeloggt ist - deckt sowohl den klassischen "nicht
// eingeloggt"-Fall (secret gesetzt, kein gültiges Cookie) als auch den Fall
// secret == "" ab, wo isLoggedIn zwar unconditionally true ist (alles
// offen), es aber trotzdem einen sichtbaren Weg zum Demo-Login braucht.
// showLogoutEntry ist bewusst NICHT einfach isLoggedIn: bei secret == ""
// ist isLoggedIn immer true (alles offen), obwohl gar keine Session
// existiert, aus der man sich abmelden könnte - "Abmelden" ist nur
// sinnvoll bei einer echten Session (realLogin) oder einer Demo-Session.
func demoNavFlags(r *http.Request, secret string) (isDemo, showLoginEntry, showLogoutEntry bool) {
	isDemo = isDemoSession(r)
	realLogin := secret != "" && !isDemo && isLoggedIn(r, secret)
	return isDemo, !isDemo && !realLogin, isDemo || realLogin
}

// navData is every page's shared Nav-Fakten - Base plus the 3 Login/Demo-
// Facts, computed once per request instead of separately in each of the 9
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

// auth bundles the 2 facts every login-related decision needs: das
// konfigurierte Login-Kennwort (secret) und die Demo-Datenbank (gebraucht,
// um sie bei einem erfolgreichen Demo-Login zurückzusetzen). Einmal in
// NewMux gebaut (newAuth) und seitdem durchgereicht statt secret als
// bloßer String durch ~13 Routen-Registrierungen und 11 Handler-
// Konstruktoren - Architecture Review nach dem Demo-Modus (#118),
// Kandidat 2.
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
// Login-Kennwort (Ticket #112's Durchsetzungs-Matrix): with no Kennwort
// configured every route stays open (isLoggedIn always true), otherwise an
// unauthenticated request bounces to the Login-Overlay (Ticket #113).
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

// HandleLogin's demo branch resets a.demoDB to its frischen 39-Monats-
// Ausgangszustand on every erfolgreichen Demo-Login (Issue #121) - vor dem
// Setzen des Session-Cookies, damit eine gewährte Demo-Session immer den
// frischen Stand sieht, nie den einer vorherigen Session.
func (a auth) HandleLogin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}
		// demoPassword ist ein Sonderfall, geprüft vor dem regulären
		// secret-Vergleich - funktioniert immer, auch bei secret == ""
		// (Issue #118 Implementation Decision).
		if r.FormValue("password") == demoPassword {
			if err := store.ResetDemoData(a.demoDB, time.Now()); err != nil {
				http.Error(w, "demo reset: "+err.Error(), http.StatusInternalServerError)
				return
			}
			// Eine noch gültige echte Session-Cookie darf nicht liegen
			// bleiben - isDemoSession wird zwar vor der echten Session
			// geprüft (isLoggedIn/withDB), aber ein sauberer Login-Wechsel
			// räumt beide Seiten auf, statt sich allein auf diese Prüf-
			// reihenfolge zu verlassen.
			clearSessionCookie(w)
			setDemoSessionCookie(w)
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
		// Symmetrisch zum Demo-Zweig oben: eine noch gültige Demo-Session-
		// Cookie darf einen frischen echten Login nicht überstimmen.
		clearDemoSessionCookie(w)
		setSessionCookie(w, a.secret)
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}

func (a auth) HandleLogout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clearSessionCookie(w)
		clearDemoSessionCookie(w)
		http.Redirect(w, r, loginRedirectTarget(r), http.StatusFound)
	}
}
