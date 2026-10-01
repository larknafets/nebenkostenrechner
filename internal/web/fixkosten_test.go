package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestParseFixkostenMonat(t *testing.T) {
	got, err := parseFixkostenMonat("2026-09")
	if err != nil {
		t.Fatalf("parseFixkostenMonat: %v", err)
	}
	if got != "2026-09-01" {
		t.Errorf("parseFixkostenMonat(2026-09) = %q, want 2026-09-01", got)
	}

	if _, err := parseFixkostenMonat("not-a-month"); err == nil {
		t.Error("parseFixkostenMonat(not-a-month): want error, got nil")
	}
	if _, err := parseFixkostenMonat(""); err == nil {
		t.Error("parseFixkostenMonat(\"\"): want error, got nil")
	}
}

func TestMonatForInput(t *testing.T) {
	if got := monatForInput("2026-09-01"); got != "2026-09" {
		t.Errorf("monatForInput(2026-09-01) = %q, want 2026-09", got)
	}
	if got := monatForInput("garbage"); got != "" {
		t.Errorf("monatForInput(garbage) = %q, want empty string", got)
	}
}

func TestBuildFixkostenPositionRows(t *testing.T) {
	kostenpositionen := []store.Kostenposition{
		{ID: 1, Key: "grundsteuer", Label: "Grundsteuer"},
		{ID: 10, Key: "strom_grundpreis", Label: "Grundgebühr Strom"},
	}
	values := map[int64]store.FixkostenPositionWert{
		1:  {Logik: store.LogikQM, Typ: store.TypJaehrlich, Wert: 480},
		10: {Logik: store.LogikWohneinheit, Typ: store.TypMonatlich, Wert: 39.90},
	}

	rows := buildFixkostenPositionRows(kostenpositionen, values)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].ID != 1 || rows[0].Logik != store.LogikQM || rows[0].Typ != store.TypJaehrlich || rows[0].Wert != 480 {
		t.Errorf("rows[0] = %+v, want id:1 Logik:%s Typ:%s Wert:480", rows[0], store.LogikQM, store.TypJaehrlich)
	}
	if rows[1].ID != 10 || rows[1].Logik != store.LogikWohneinheit || rows[1].Typ != store.TypMonatlich || rows[1].Wert != 39.90 {
		t.Errorf("rows[1] = %+v, want id:10 Logik:%s Typ:%s Wert:39.90", rows[1], store.LogikWohneinheit, store.TypMonatlich)
	}
}

// TestBuildFixkostenPositionRows_OhneVorherigeEingabe covers #105/#108: the
// very first fixed-cost entry ever created has no values, but doesn't fall
// back to an empty Logik/Typ - instead it falls back to
// store.KostenpositionDefaults, the same starting values a freshly created
// Kostenposition year row used to get.
func TestBuildFixkostenPositionRows_OhneVorherigeEingabe(t *testing.T) {
	kostenpositionen := []store.Kostenposition{
		{ID: 1, Key: "grundsteuer", Label: "Grundsteuer"},
		{ID: 13, Key: "internet", Label: "Grundgebühr Internet"},
	}

	rows := buildFixkostenPositionRows(kostenpositionen, nil)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Logik != store.LogikQM || rows[0].Typ != store.TypJaehrlich || rows[0].Wert != 0 {
		t.Errorf("Grundsteuer ohne Vorwert = %+v, want Logik:%s Typ:%s Wert:0 (aus KostenpositionDefaults)", rows[0], store.LogikQM, store.TypJaehrlich)
	}
	if rows[1].Logik != store.LogikWohneinheit || rows[1].Typ != store.TypMonatlich || rows[1].Wert != 0 {
		t.Errorf("Internet ohne Vorwert = %+v, want Logik:%s Typ:%s Wert:0 (aus KostenpositionDefaults)", rows[1], store.LogikWohneinheit, store.TypMonatlich)
	}
}

func TestLogikLabels_CoversAllLogikKonstanten(t *testing.T) {
	for _, logik := range []string{store.LogikWohneinheit, store.LogikFlurstueck, store.LogikQM, store.LogikPersonen} {
		if logikLabels[logik] == "" {
			t.Errorf("logikLabels missing entry for %q", logik)
		}
	}
}

// TestStammdaten_FlagsAnzeigenUndSpeichern verifies the /stammdaten flags
// (Issue #163): the page lists every Kostenposition with its current
// Umlagefähig state plus the Strom flag, and the POST handler saves
// them - an unchecked checkbox is absent from the form and means "No".
func TestStammdaten_FlagsAnzeigenUndSpeichern(t *testing.T) {
	db := openTestDB(t)
	a := newAuth("", nil)

	req := httptest.NewRequest(http.MethodGet, "/stammdaten", nil)
	w := httptest.NewRecorder()
	handleStammdatenForm(a)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want %d", w.Code, http.StatusOK)
	}
	body := w.Body.String()
	for _, want := range []string{"Streaming-Dienste", "Grundgebühr Strom", "Stromverbrauch Wohnung 2 weiterberechnen", `name="umlagefaehig_10" value="1" checked`} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /stammdaten body does not contain %q", want)
		}
	}
	if strings.Contains(body, `name="umlagefaehig_15" value="1" checked`) {
		t.Error("Streaming-Dienste is checked by default, want unchecked (Startwert Nein)")
	}

	// Streaming on, everything else (incl. Strom flag) left unchecked.
	form := url.Values{
		"qm_1": {"100"}, "flurstueck_groesse_1": {"600"},
		"qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
		"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
		"umlagefaehig_15": {"1"},
	}
	post := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	pw := httptest.NewRecorder()
	handleUpdateStammdaten()(pw, requestWithDB(post, db))
	if pw.Code != http.StatusFound {
		t.Fatalf("POST status = %d, want %d (body: %s)", pw.Code, http.StatusFound, pw.Body.String())
	}

	kps, err := store.Kostenpositionen(db)
	if err != nil {
		t.Fatalf("Kostenpositionen: %v", err)
	}
	for _, kp := range kps {
		want := kp.ID == 15
		if kp.Umlagefaehig != want {
			t.Errorf("%s: Umlagefaehig = %v, want %v", kp.Key, kp.Umlagefaehig, want)
		}
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	if haus.StromWeiterberechnen {
		t.Error("StromWeiterberechnen = true, want false (checkbox absent from the form)")
	}
}

// TestStammdaten_MieterUndVermieterNurAngemeldet verifies Issue #164: the
// tenant, landlord, address and IBAN fields are shown to logged-in users
// only (the server does not even send the values otherwise), the
// Wohnungsstatus is visible to everyone but only editable when logged in,
// and the POST handler saves everything.
func TestStammdaten_MieterUndVermieterNurAngemeldet(t *testing.T) {
	db := openTestDB(t)

	form := url.Values{
		"qm_1": {"100"}, "flurstueck_groesse_1": {"600"},
		"qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
		"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
		"mieter_name_2":         {"  Erika Beispiel \n"},
		"mieter_anschrift_2":    {"Beispielweg 1\r\nWohnung 2\r\n12345 Musterstadt"},
		"vermieter_name":        {"Max Mustermann"},
		"vermieter_anschrift":   {"Hauptstraße 5"},
		"objekt_anschrift":      {"Beispielweg 1, 12345 Musterstadt"},
		"iban":                  {"DE00 1234 5678"},
		"strom_weiterberechnen": {"1"},
	}
	post := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	pw := httptest.NewRecorder()
	handleUpdateStammdaten()(pw, requestWithDB(post, db))
	if pw.Code != http.StatusFound {
		t.Fatalf("POST status = %d, want %d (body: %s)", pw.Code, http.StatusFound, pw.Body.String())
	}

	apartments, err := store.Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}
	if got := apartments[1].MieterName; got != "Erika Beispiel" {
		t.Errorf("MieterName = %q, want trimmed %q", got, "Erika Beispiel")
	}
	if got := apartments[1].MieterAnschrift; got != "Beispielweg 1\nWohnung 2\n12345 Musterstadt" {
		t.Errorf("MieterAnschrift = %q, want line breaks normalized to \\n", got)
	}

	secrets := []string{"Erika Beispiel", "Max Mustermann", "Hauptstraße 5", "DE00 1234 5678", "12345 Musterstadt"}

	// Logged in (no LOGIN_PASSWORD configured): everything is visible.
	req := httptest.NewRequest(http.MethodGet, "/stammdaten", nil)
	w := httptest.NewRecorder()
	handleStammdatenForm(newAuth("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("GET (logged in) status = %d", w.Code)
	}
	for _, want := range secrets {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("GET /stammdaten (logged in) does not contain %q", want)
		}
	}

	// Not logged in (LOGIN_PASSWORD set, no cookie): personal data is gone,
	// the status is visible but disabled.
	req = httptest.NewRequest(http.MethodGet, "/stammdaten", nil)
	w = httptest.NewRecorder()
	handleStammdatenForm(newAuth("geheim", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("GET (not logged in) status = %d", w.Code)
	}
	body := w.Body.String()
	for _, secret := range secrets {
		if strings.Contains(body, secret) {
			t.Errorf("GET /stammdaten (not logged in) leaks %q", secret)
		}
	}
	for _, field := range []string{`name="mieter_name_`, `name="vermieter_name"`, `name="iban"`} {
		if strings.Contains(body, field) {
			t.Errorf("GET /stammdaten (not logged in) renders the field %s", field)
		}
	}
	if !strings.Contains(body, `<select name="status_2" disabled>`) || !strings.Contains(body, `value="vermietet" selected`) {
		t.Error("GET /stammdaten (not logged in) does not show the Wohnungsstatus as a disabled select")
	}
}

func TestStammdaten_UngueltigerStatusWird400(t *testing.T) {
	db := openTestDB(t)

	form := url.Values{
		"qm_1": {"100"}, "flurstueck_groesse_1": {"600"},
		"qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
		"status_1": {"leerstand"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
	}
	post := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(form.Encode()))
	post.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	pw := httptest.NewRecorder()
	handleUpdateStammdaten()(pw, requestWithDB(post, db))
	if pw.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want %d", pw.Code, http.StatusBadRequest)
	}
}

// TestStammdaten_HeizungGewichtung verifies Issue #162: the page offers the
// three allowed weightings with the saved one selected, the POST handler
// saves a valid choice and rejects an invalid one with HTTP 400.
func TestStammdaten_HeizungGewichtung(t *testing.T) {
	db := openTestDB(t)

	form := func(gewichtung string) url.Values {
		return url.Values{
			"qm_1": {"100"}, "flurstueck_groesse_1": {"600"},
			"qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
			"status_1": {"eigennutzung"}, "status_2": {"vermietet"},
			"heizung_gewichtung": {gewichtung},
		}
	}
	post := func(v url.Values) int {
		req := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(v.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleUpdateStammdaten()(w, requestWithDB(req, db))
		return w.Code
	}

	if code := post(form("0.55")); code != http.StatusBadRequest {
		t.Fatalf("POST with 0.55: status = %d, want %d", code, http.StatusBadRequest)
	}
	if code := post(form("0.6")); code != http.StatusFound {
		t.Fatalf("POST with 0.6: status = %d, want %d", code, http.StatusFound)
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	if haus.HeizungWaermeGewichtung != 0.6 {
		t.Errorf("HeizungWaermeGewichtung = %v, want 0.6", haus.HeizungWaermeGewichtung)
	}

	req := httptest.NewRequest(http.MethodGet, "/stammdaten", nil)
	w := httptest.NewRecorder()
	handleStammdatenForm(newAuth("", nil))(w, requestWithDB(req, db))
	body := w.Body.String()
	for _, want := range []string{`name="heizung_gewichtung" value="0.7" `, `name="heizung_gewichtung" value="0.6" checked`, `name="heizung_gewichtung" value="0.5" `, "60 % / 40 %"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /stammdaten does not contain %q", want)
		}
	}
	if strings.Contains(body, `value="0.7" checked`) {
		t.Error("70/30 is checked although 60/40 was saved")
	}
}

// fixkostenForm builds a complete /fixkosten form for the given month
// ("YYYY-MM") - every Kostenposition on Wohneinheit/monatlich with value 10.
func fixkostenForm(monat string) url.Values {
	v := url.Values{
		"monat":      {monat},
		"personen_1": {"2"}, "personen_2": {"1"},
		"abschlag_1": {"0"}, "abschlag_2": {"100"},
	}
	for _, kd := range store.KostenpositionDefaults {
		id := strconv.FormatInt(kd.ID, 10)
		v.Set("logik_"+id, store.LogikWohneinheit)
		v.Set("typ_"+id, store.TypMonatlich)
		v.Set("wert_"+id, "10")
	}
	return v
}

// TestFixkosten_EineEingabeJeMonat verifies Issue #161 at the form level:
// a second Eingabe for a month, and moving one onto an occupied month, are
// answered with HTTP 400 and a readable German message, nothing is saved.
func TestFixkosten_EineEingabeJeMonat(t *testing.T) {
	db := openTestDB(t)

	post := func(path string, form url.Values, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		if id == "" {
			handleCreateFixkosten()(w, requestWithDB(req, db))
		} else {
			req.SetPathValue("id", id)
			handleUpdateFixkosten()(w, requestWithDB(req, db))
		}
		return w
	}

	if w := post("/fixkosten", fixkostenForm("2026-09"), ""); w.Code != http.StatusFound {
		t.Fatalf("first create status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	w := post("/fixkosten", fixkostenForm("2026-09"), "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("second create for the same month: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	for _, want := range []string{"September 2026", "bereits eine Fixkosten-Eingabe"} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("message %q does not contain %q", w.Body.String(), want)
		}
	}

	if w := post("/fixkosten", fixkostenForm("2026-10"), ""); w.Code != http.StatusFound {
		t.Fatalf("create for 2026-10 status = %d, want %d", w.Code, http.StatusFound)
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(eingaben) != 2 {
		t.Fatalf("eingaben = %d, want 2 (September and Oktober)", len(eingaben))
	}

	// eingaben are newest first: [0] = Oktober. Moving it onto September fails.
	okt := strconv.FormatInt(eingaben[0].ID, 10)
	if w := post("/fixkosten/"+okt, fixkostenForm("2026-09"), okt); w.Code != http.StatusBadRequest {
		t.Errorf("update onto an occupied month: status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	// Correcting it under its own month works.
	if w := post("/fixkosten/"+okt, fixkostenForm("2026-10"), okt); w.Code != http.StatusFound {
		t.Errorf("update keeping its own month: status = %d, want %d", w.Code, http.StatusFound)
	}
}

func TestValidateFixkostenMonateEindeutig(t *testing.T) {
	in := func(monat string) store.FixkostenInput { return store.FixkostenInput{Monat: monat} }

	if err := validateFixkostenMonateEindeutig([]store.FixkostenInput{in("2026-01-01"), in("2026-02-01")}); err != nil {
		t.Errorf("distinct months: unexpected error %v", err)
	}
	err := validateFixkostenMonateEindeutig([]store.FixkostenInput{in("2026-01-01"), in("2026-02-01"), in("2026-01-01")})
	if err == nil || !strings.Contains(err.Error(), "Januar 2026") {
		t.Errorf("duplicate month: err = %v, want an error naming Januar 2026", err)
	}
}
