package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Tests for the Berechtigungen table on /stammdaten (#310): the Vermieter sets
// the rights of Nicht angemeldet and Mieter with checkboxes.

func stammdatenSeite(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie) string {
	t.Helper()
	w := getMitCookies(t, mux, "/stammdaten", cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /stammdaten: status %d", w.Code)
	}
	return w.Body.String()
}

func TestStammdaten_BerechtigungenNurFuerDenVermieter(t *testing.T) {
	mux := rollenMux(t, "geheim", "mieter")
	vermieter := loginCookies(t, mux, "vermieter", "geheim")
	mieter := loginCookies(t, mux, "mieter", "mieter")

	voll := stammdatenSeite(t, mux, vermieter)
	if !strings.Contains(voll, `id="berechtigungen"`) {
		t.Fatal("Vermieter sieht die Berechtigungen nicht")
	}
	// One row per right and role, with the defaults checked.
	if n := strings.Count(voll, `name="recht_nicht_angemeldet_`); n != len(store.Rechte) {
		t.Errorf("%d Zeilen Nicht angemeldet, want %d", n, len(store.Rechte))
	}
	if n := strings.Count(voll, `name="recht_mieter_`); n != len(store.Rechte) {
		t.Errorf("%d Zeilen Mieter, want %d", n, len(store.Rechte))
	}
	if !strings.Contains(voll, `name="recht_mieter_wohnung2" value="1" checked`) || strings.Contains(voll, `name="recht_mieter_wallboxen" value="1" checked`) {
		t.Error("Defaults nicht gesetzt: Mieter Wohnung 2 an, Wallboxen aus")
	}
	if !strings.Contains(voll, "Miete (Block in der Jahreskarte)") {
		t.Error("Recht Miete fehlt")
	}

	for name, cookies := range map[string][]*http.Cookie{"Mieter": mieter, "nicht angemeldet": nil} {
		if strings.Contains(stammdatenSeite(t, mux, cookies), `id="berechtigungen"`) {
			t.Errorf("%s sieht die Berechtigungen", name)
		}
	}
}

func TestStammdaten_MieterSpalteNurMitMieterLogin(t *testing.T) {
	mux := rollenMux(t, "geheim", "")
	vermieter := loginCookies(t, mux, "vermieter", "geheim")
	body := stammdatenSeite(t, mux, vermieter)
	if strings.Contains(body, `name="recht_mieter_`) || strings.Contains(body, `value="mieter"`) {
		t.Error("Mieter-Spalte ohne Mieter-Kennwort")
	}
	if !strings.Contains(body, `name="recht_nicht_angemeldet_wohnung2"`) {
		t.Error("Spalte Nicht angemeldet fehlt")
	}
}

func stammdatenForm(extra url.Values) url.Values {
	form := url.Values{
		"qm_1": {"100"}, "flurstueck_groesse_1": {"600"}, "qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
		"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
	}
	for k, v := range extra {
		form[k] = v
	}
	return form
}

func TestStammdaten_BerechtigungenSpeichern(t *testing.T) {
	db := openTestDB(t)
	post := func(extra url.Values) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(stammdatenForm(extra).Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleUpdateStammdaten()(w, requestWithDB(req, db))
		if w.Code != http.StatusFound {
			t.Fatalf("POST status = %d (body: %s)", w.Code, w.Body.String())
		}
	}

	// A form without the table leaves the rights alone (and writes none).
	post(nil)
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM berechtigungen`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("Formular ohne Berechtigungen schrieb %d Zeilen (err %v), want 0", n, err)
	}

	// Both columns shown: unchecked boxes are absent and mean "No".
	post(url.Values{
		"berechtigung_rolle":              {"nicht_angemeldet", "mieter"},
		"recht_nicht_angemeldet_wohnung2": {"1"},
		"recht_mieter_wohnung2":           {"1"},
		"recht_mieter_wallboxen":          {"1"},
		"recht_mieter_miete":              {"1"},
	})
	b, err := store.GetBerechtigungen(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		rolle store.Rolle
		recht store.Recht
		want  bool
	}{
		{store.RolleNichtAngemeldet, store.RechtWohnung2, true},
		{store.RolleNichtAngemeldet, store.RechtWaermepumpe, false}, // default an, abgehakt entfernt
		{store.RolleMieter, store.RechtWallboxen, true},
		{store.RolleMieter, store.RechtMiete, true},
		{store.RolleMieter, store.RechtWaermepumpe, false},
	} {
		if got := b.Erlaubt(c.rolle, c.recht); got != c.want {
			t.Errorf("%s/%s = %v, want %v", c.rolle, c.recht, got, c.want)
		}
	}

	// Only the Nicht-angemeldet column shown (no Mieter login): the Mieter
	// values stay as saved.
	post(url.Values{
		"berechtigung_rolle":              {"nicht_angemeldet"},
		"recht_nicht_angemeldet_wohnung1": {"1"},
	})
	b, _ = store.GetBerechtigungen(db)
	if !b.Erlaubt(store.RolleNichtAngemeldet, store.RechtWohnung1) || b.Erlaubt(store.RolleNichtAngemeldet, store.RechtWohnung2) {
		t.Errorf("Spalte Nicht angemeldet nicht übernommen: %+v", b[store.RolleNichtAngemeldet])
	}
	if !b.Erlaubt(store.RolleMieter, store.RechtWallboxen) || !b.Erlaubt(store.RolleMieter, store.RechtMiete) {
		t.Errorf("ausgeblendete Mieter-Spalte wurde verändert: %+v", b[store.RolleMieter])
	}
}

// A form without the Berechtigungen table must not read them: saving the
// Stammdaten works even when they cannot be loaded, and only a form that
// carries the table fails then.
func TestStammdaten_OhneTabelleLiestKeineBerechtigungen(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`DROP TABLE berechtigungen`); err != nil {
		t.Fatal(err)
	}
	post := func(extra url.Values) int {
		req := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(stammdatenForm(extra).Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleUpdateStammdaten()(w, requestWithDB(req, db))
		return w.Code
	}
	if got := post(nil); got != http.StatusFound {
		t.Errorf("Formular ohne Tabelle: status = %d, want %d", got, http.StatusFound)
	}
	if got := post(url.Values{"berechtigung_rolle": {"mieter"}}); got != http.StatusInternalServerError {
		t.Errorf("Formular mit Tabelle: status = %d, want %d", got, http.StatusInternalServerError)
	}
}

// --- Berechtigungen apply (#311) ---

// rechteMux builds a mux over a database with demo data and both passwords and
// returns it with the database, so a test can save rights.
func rechteMux(t *testing.T) (*http.ServeMux, *sql.DB) {
	t.Helper()
	t.Setenv("VERMIETER_PASSWORD", "geheim")
	t.Setenv("MIETER_PASSWORD", "mieter")
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	return NewMux(db, openTestDB(t), "", ""), db
}

// setzeRecht saves one right for one role.
func setzeRecht(t *testing.T, db *sql.DB, rolle store.Rolle, recht store.Recht, erlaubt bool) {
	t.Helper()
	b, err := store.GetBerechtigungen(db)
	if err != nil {
		t.Fatal(err)
	}
	b[rolle][recht] = erlaubt
	saveStammdaten(t, db, func(s *store.StammdatenSave) { s.Berechtigungen = b })
}

func rollenCookies(t *testing.T, mux *http.ServeMux) map[string][]*http.Cookie {
	t.Helper()
	return map[string][]*http.Cookie{
		"nicht angemeldet": nil,
		"mieter":           loginCookies(t, mux, "mieter", "mieter"),
		"vermieter":        loginCookies(t, mux, "vermieter", "geheim"),
	}
}

// With a right off, a visitor who is not logged in is bounced to the login
// and a Mieter gets the page "Nicht freigegeben"; with the right on both pass.
// The Vermieter always passes.
func TestRechte_RoutenJeRolleUndRecht(t *testing.T) {
	mux, db := rechteMux(t)
	cookies := rollenCookies(t, mux)
	periods, err := store.AllPeriods(db)
	if err != nil || len(periods) == 0 {
		t.Fatalf("AllPeriods: %v (%d)", err, len(periods))
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil || len(eingaben) == 0 {
		t.Fatalf("AllFixkostenEingaben: %v (%d)", err, len(eingaben))
	}
	pid, fid := strconv.FormatInt(periods[0].ID, 10), strconv.FormatInt(eingaben[0].ID, 10)

	for _, c := range []struct {
		recht store.Recht
		pfade []string
	}{
		{store.RechtAblesungenAnsehen, []string{"/ablesungen", "/ablesungen/" + pid}},
		{store.RechtAblesungAnlegen, []string{"/ablesungen/neu"}},
		{store.RechtStammdatenAnsehen, []string{"/stammdaten"}},
		{store.RechtFixkostenAnsehen, []string{"/fixkosten", "/fixkosten/" + fid}},
	} {
		for _, rolle := range []store.Rolle{store.RolleNichtAngemeldet, store.RolleMieter} {
			name := "nicht angemeldet"
			if rolle == store.RolleMieter {
				name = "mieter"
			}
			for _, pfad := range c.pfade {
				t.Run(string(c.recht)+" "+name+" "+pfad, func(t *testing.T) {
					setzeRecht(t, db, rolle, c.recht, false)
					w := getMitCookies(t, mux, pfad, cookies[name])
					switch rolle {
					case store.RolleMieter:
						if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "Nicht freigegeben") {
							t.Errorf("ohne Recht: status %d, want 403 Nicht freigegeben", w.Code)
						}
					default:
						if w.Code != http.StatusFound || !strings.Contains(w.Header().Get("Location"), "login=1") {
							t.Errorf("ohne Recht: status %d, Location %q, want Weiterleitung zum Login", w.Code, w.Header().Get("Location"))
						}
					}
					if w := getMitCookies(t, mux, pfad, cookies["vermieter"]); w.Code != http.StatusOK {
						t.Errorf("Vermieter ohne Haken: status %d, want 200", w.Code)
					}
					setzeRecht(t, db, rolle, c.recht, true)
					if w := getMitCookies(t, mux, pfad, cookies[name]); w.Code != http.StatusOK {
						t.Errorf("mit Recht: status %d, want 200", w.Code)
					}
				})
			}
		}
	}
}

// What the Dashboard shows follows the rights of the role.
func TestRechte_DashboardJeRolle(t *testing.T) {
	mux, db := rechteMux(t)
	cookies := rollenCookies(t, mux)
	zeige := func(name string) string {
		return getMitCookies(t, mux, "/dashboard", cookies[name]).Body.String()
	}

	for _, rolle := range []store.Rolle{store.RolleNichtAngemeldet, store.RolleMieter} {
		name := "nicht angemeldet"
		if rolle == store.RolleMieter {
			name = "mieter"
		}
		t.Run(name, func(t *testing.T) {
			body := zeige(name)
			// Defaults: Wohnung 2 and Wärmepumpe only.
			if !strings.Contains(body, "<strong>Wohnung 2</strong>") || !strings.Contains(body, "<strong>Wärmepumpe</strong>") {
				t.Error("Default: Wohnung 2 und Wärmepumpe fehlen")
			}
			for _, s := range []string{"<strong>Wohnung 1</strong>", "<strong>Wallboxen</strong>", "<strong>PV-Anlage</strong>", labelMiete, "712,34"} {
				if strings.Contains(body, s) {
					t.Errorf("Default: %q sichtbar", s)
				}
			}

			for _, c := range []struct {
				recht store.Recht
				want  string
			}{
				{store.RechtWohnung1, "<strong>Wohnung 1</strong>"},
				{store.RechtWallboxen, "<strong>Wallboxen</strong>"},
				{store.RechtPVAnlage, "<strong>PV-Anlage</strong>"},
				{store.RechtMiete, labelMiete},
			} {
				setzeRecht(t, db, rolle, c.recht, true)
				if !strings.Contains(zeige(name), c.want) {
					t.Errorf("Recht %s an: %q fehlt", c.recht, c.want)
				}
				setzeRecht(t, db, rolle, c.recht, false)
			}

			// Nothing granted: the hint instead of empty space.
			setzeRecht(t, db, rolle, store.RechtWohnung2, false)
			setzeRecht(t, db, rolle, store.RechtWaermepumpe, false)
			if !strings.Contains(zeige(name), "noch nichts freigegeben") {
				t.Error("ohne Rechte fehlt der Hinweis")
			}
			setzeRecht(t, db, rolle, store.RechtWohnung2, true)
			setzeRecht(t, db, rolle, store.RechtWaermepumpe, true)
		})
	}

	// The Vermieter ignores the checkboxes.
	body := zeige("vermieter")
	for _, s := range []string{"<strong>Wohnung 1</strong>", "<strong>Wallboxen</strong>", "<strong>PV-Anlage</strong>", labelMiete} {
		if !strings.Contains(body, s) {
			t.Errorf("Vermieter sieht %q nicht", s)
		}
	}
}

// The nav shows only the pages the role may open, and the buttons to create
// follow the right to create.
func TestRechte_NavigationUndButtons(t *testing.T) {
	mux, db := rechteMux(t)
	cookies := rollenCookies(t, mux)
	for _, recht := range []store.Recht{store.RechtAblesungenAnsehen, store.RechtFixkostenAnsehen, store.RechtStammdatenAnsehen, store.RechtAblesungAnlegen} {
		setzeRecht(t, db, store.RolleMieter, recht, false)
	}
	body := getMitCookies(t, mux, "/dashboard", cookies["mieter"]).Body.String()
	for _, s := range []string{`href="/ablesungen"`, `href="/fixkosten"`, `href="/stammdaten"`, "Neue Ablesung"} {
		if strings.Contains(body, s) {
			t.Errorf("Mieter ohne Rechte sieht %q", s)
		}
	}
	if !strings.Contains(body, `href="/berechnungslogik"`) {
		t.Error("Wie wird gerechnet? fehlt, sie sehen alle")
	}

	voll := getMitCookies(t, mux, "/dashboard", cookies["vermieter"]).Body.String()
	for _, s := range []string{`href="/ablesungen"`, `href="/fixkosten"`, `href="/stammdaten"`, "Neue Ablesung", `href="/abrechnung"`} {
		if !strings.Contains(voll, s) {
			t.Errorf("Vermieter sieht %q nicht", s)
		}
	}
}

// Completing a Teilstand needs the right to create, nothing else.
func TestRechte_TeilstandVervollstaendigenBrauchtAnlegen(t *testing.T) {
	mux, db := rechteMux(t)
	cookies := rollenCookies(t, mux)
	id, err := store.CreatePeriod(db, store.PeriodInput{ReadingDate: "2026-12-01"})
	if err != nil {
		t.Fatalf("CreatePeriod (Teilstand): %v", err)
	}
	pfad := "/ablesungen/" + strconv.FormatInt(id, 10) + "/bearbeiten"

	if w := getMitCookies(t, mux, pfad, cookies["nicht angemeldet"]); w.Code != http.StatusOK {
		t.Errorf("Default: status %d, want 200", w.Code)
	}
	setzeRecht(t, db, store.RolleNichtAngemeldet, store.RechtAblesungAnlegen, false)
	if w := getMitCookies(t, mux, pfad, cookies["nicht angemeldet"]); w.Code != http.StatusFound {
		t.Errorf("ohne Recht: status %d, want Weiterleitung zum Login", w.Code)
	}
	setzeRecht(t, db, store.RolleMieter, store.RechtAblesungAnlegen, false)
	if w := getMitCookies(t, mux, pfad, cookies["mieter"]); w.Code != http.StatusForbidden {
		t.Errorf("Mieter ohne Recht: status %d, want 403", w.Code)
	}
}
