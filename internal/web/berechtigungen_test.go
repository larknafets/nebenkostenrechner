package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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
