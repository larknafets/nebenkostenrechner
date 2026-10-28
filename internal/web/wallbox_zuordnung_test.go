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

// wallbox_zuordnung_test.go covers the assignment of the Wallbox costs to
// Wohnung 1/2 (#225) through the HTTP seam: the switch "Wallboxen
// vollständig Wohnung 1" in the form (the note), Ablesungsseite and CSV
// (switches_test.go).

const wbSchalter = "wallbox_komplett_w1"

// wallboxAblesungen stores a first Ablesung (everything 0, Wallbox Wohnung 2
// at 0) and a second one with 100 kWh Netzbezug, 30 kWh Wohnung 2, 50 kWh
// Wärmepumpe and 40 kWh Wallboxen, so 20 kWh of grid draw are left for the
// Wallboxen (4,40 EUR at 0,22 EUR/kWh). It returns the id of the second.
func wallboxAblesungen(t *testing.T, db *sql.DB, einzelW1, einzelW2 *float64, komplettW1 bool) int64 {
	t.Helper()
	first := seedPeriodInputAt("2026-10-01")
	first.Readings[wbW1], first.Readings[wbW2] = 0, 0
	if _, err := createPeriod(db, first); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	second := seedPeriodInputAt("2026-11-01")
	second.Readings["strom_gesamt"] = 100
	second.Readings["strom_wohnung2"] = 30
	second.Readings["strom_waermepumpe"] = 50
	second.Readings["strom_wallbox"] = 40
	if einzelW1 != nil {
		second.Readings[wbW1] = *einzelW1
	}
	if einzelW2 != nil {
		second.Readings[wbW2] = *einzelW2
	}
	id, err := createPeriod(db, second, mitHaekchen(komplettW1, false))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	return id
}

// editWizardBody renders the "korrigieren" form of the Ablesung id.
func editWizardBody(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10)+"/bearbeiten", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleEditWizardForm(newZugang("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// hinweisText returns the text of the Wallbox note of a form.
func hinweisText(t *testing.T, body string) string {
	t.Helper()
	i := strings.Index(body, `id="wallbox-hinweis"`)
	if i < 0 {
		t.Fatal("kein Wallbox-Hinweis im Formular")
	}
	rest := body[i:]
	start := strings.Index(rest, ">") + 1
	return rest[start : start+strings.Index(rest[start:], "</div>")]
}

// TestWizard_WallboxHinweisMitStand: the switch is no field of the form any
// more (#359), the note below the Wallbox-Einzelzähler shows its state in the
// Fixkosten-Eingabe of the Abrechnungsmonat, or that the Fixkosten are missing.
func TestWizard_WallboxHinweisMitStand(t *testing.T) {
	t.Run("Haekchen gesetzt", func(t *testing.T) {
		db := openTestDB(t)
		id := wallboxAblesungen(t, db, nil, nil, true)
		body := editWizardBody(t, db, id)
		if got, want := hinweisText(t, body), "Häkchen gesetzt: die Einzelzähler bestimmen die Kosten nicht"; got != want {
			t.Errorf("Hinweis = %q, want %q", got, want)
		}
	})
	t.Run("Haekchen nicht gesetzt", func(t *testing.T) {
		db := openTestDB(t)
		id := wallboxAblesungen(t, db, nil, nil, false)
		body := editWizardBody(t, db, id)
		if got, want := hinweisText(t, body), "nicht gesetzt: Wohnung 2 bestimmt ihren Anteil"; got != want {
			t.Errorf("Hinweis = %q, want %q", got, want)
		}
	})
	t.Run("Fixkosten fehlen", func(t *testing.T) {
		db := openTestDB(t)
		id, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01"))
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		body := editWizardBody(t, db, id)
		if got, want := hinweisText(t, body), "Fixkosten für November 2026 fehlen"; got != want {
			t.Errorf("Hinweis = %q, want %q", got, want)
		}
	})
	t.Run("neue Ablesung: der Monat von heute", func(t *testing.T) {
		db := openTestDB(t)
		heute := time.Now().Format("2006-01") + "-01"
		fk := fixkostenMitPreisen(heute)
		fk.WallboxKomplettW1 = true
		if _, err := store.CreateFixkostenEingabe(db, fk); err != nil {
			t.Fatalf("CreateFixkostenEingabe: %v", err)
		}
		if got, want := hinweisText(t, wizardBody(t, db)), "Häkchen gesetzt: die Einzelzähler bestimmen die Kosten nicht"; got != want {
			t.Errorf("Hinweis = %q, want %q", got, want)
		}
	})
	t.Run("das Skript kennt jeden Monat mit Eingabe", func(t *testing.T) {
		db := openTestDB(t)
		wallboxAblesungen(t, db, nil, nil, true)
		body := wizardBody(t, db)
		if !strings.Contains(body, `&#34;2026-11&#34;:true`) {
			t.Errorf("Formular ohne den Stand von 2026-11 für das Skript")
		}
	})
}

// TestWizard_OhneHaekchenPreiseUndPersonen: three steps, no field for
// Personenzahl, prices and the two switches (#359).
func TestWizard_OhneHaekchenPreiseUndPersonen(t *testing.T) {
	db := openTestDB(t)
	id := wallboxAblesungen(t, db, nil, nil, true)
	for name, body := range map[string]string{"erfassen": wizardBody(t, db), "korrigieren": editWizardBody(t, db, id)} {
		for _, feld := range []string{"wallbox_komplett_w1", "pv_gutschrift_w1", "strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis", "personen_1", "personen_2"} {
			if strings.Contains(body, `name="`+feld+`"`) {
				t.Errorf("%s: Feld %q im Formular", name, feld)
			}
		}
		for _, schritt := range []string{"1", "2", "3"} {
			if !strings.Contains(body, `class="c-step-dot`+"") || !strings.Contains(body, `data-step="`+schritt+`"`) {
				t.Errorf("%s: Schritt %s fehlt", name, schritt)
			}
		}
		if strings.Contains(body, `data-step="4"`) {
			t.Errorf("%s: es gibt einen vierten Schritt", name)
		}
	}
}

// TestAblesung_OhnePreiseAnlegenAendernVervollstaendigen: an Ablesung needs
// none of the dropped fields, to create, change or complete it (#359). Posted
// prices, Personenzahl and switches are ignored.
func TestAblesung_OhnePreiseAnlegenAendernVervollstaendigen(t *testing.T) {
	db := openTestDB(t)
	form := vollesFormular()
	form.Set(wbSchalter, "1")
	form.Set("strompreis", "9")
	form.Set("personen_1", "7")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
	if w.Code != http.StatusFound {
		t.Fatalf("anlegen: status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	latest, err := store.GetLatestPeriod(db)
	if err != nil || latest == nil {
		t.Fatalf("GetLatestPeriod = %v, %v", latest, err)
	}
	// The posted values reach no Fixkosten-Eingabe.
	if eingaben, err := store.AllFixkostenEingabenDetails(db); err != nil || len(eingaben) != 0 {
		t.Errorf("Fixkosten-Eingaben nach dem Anlegen = %v, %v, want keine", eingaben, err)
	}

	// Change: new date and a meter reading, nothing else posted.
	change := vollesFormular()
	change.Set("strom_gesamt", "5")
	req := httptest.NewRequest(http.MethodPost, "/ablesungen/"+strconv.FormatInt(latest.ID, 10), strings.NewReader(change.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(latest.ID, 10))
	w = httptest.NewRecorder()
	handleUpdateAblesung()(w, requestWithDB(req, db))
	if w.Code != http.StatusFound {
		t.Fatalf("ändern: status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}

	// Complete a Teilstand: a form with only date and month, then the rest.
	teil := url.Values{"reading_date": {"2026-12-01"}, "monat": {"2026-12"}}
	w = httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, teil), db))
	if w.Code != http.StatusFound {
		t.Fatalf("Teilstand anlegen: status = %d (body: %s)", w.Code, w.Body.String())
	}
	open, err := store.GetLatestPeriod(db)
	if err != nil || open == nil || !open.Teilstand(nil).IstTeilstand {
		t.Fatalf("Teilstand = %+v, %v", open, err)
	}
	full := vollesFormular()
	full.Set("reading_date", "2026-12-01")
	full.Set("monat", "2026-12")
	req = httptest.NewRequest(http.MethodPost, "/ablesungen/"+strconv.FormatInt(open.ID, 10), strings.NewReader(full.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", strconv.FormatInt(open.ID, 10))
	w = httptest.NewRecorder()
	handleUpdateAblesung()(w, requestWithDB(req, db))
	if w.Code != http.StatusFound {
		t.Fatalf("vervollständigen: status = %d (body: %s)", w.Code, w.Body.String())
	}
	done, err := store.GetPeriodDetails(db, open.ID)
	if err != nil || done == nil || done.Teilstand(nil).IstTeilstand {
		t.Errorf("Ablesung nach dem Vervollständigen = %+v, %v, want vollständig", done, err)
	}
}

func TestAblesungDetail_SchalterAlsJaNein(t *testing.T) {
	for _, c := range []struct {
		komplett bool
		want     string
	}{{true, "Ja"}, {false, "Nein"}} {
		db := openTestDB(t)
		id := wallboxAblesungen(t, db, nil, nil, c.komplett)
		body := detailBody(t, db, id)
		if !strings.Contains(body, "Wallboxen vollständig Wohnung 1:</strong> "+c.want+"<") {
			t.Errorf("Schalter %v: Ablesungsseite ohne %q", c.komplett, c.want)
		}
	}
}

func TestAblesungDetail_WallboxKostenJeWohnung(t *testing.T) {
	zwoelf, null := 12.0, 0.0

	t.Run("Wohnung 2 mit Einzelzaehler, Wohnung 1 den Rest", func(t *testing.T) {
		db := openTestDB(t)
		body := detailBody(t, db, wallboxAblesungen(t, db, &null, &zwoelf, false))
		// 12 kWh * 0,22 = 2,64 EUR, Rest 4,40 - 2,64 = 1,76 EUR.
		for _, want := range []string{
			"Kosten Wallbox Wohnung 1</strong></td><td><strong>1,76&nbsp;EUR",
			"Kosten Wallbox Wohnung 2</strong></td><td><strong>2,64&nbsp;EUR",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("Ablesungsseite ohne %q", want)
			}
		}
		if strings.Contains(body, "keiner Wohnung zugeordnet") {
			t.Error("verteilte Wallbox-Kosten als 'keiner Wohnung zugeordnet' markiert")
		}
	})

	t.Run("Schalter an: alles bei Wohnung 1", func(t *testing.T) {
		db := openTestDB(t)
		body := detailBody(t, db, wallboxAblesungen(t, db, nil, &zwoelf, true))
		for _, want := range []string{
			"Kosten Wallbox Wohnung 1</strong></td><td><strong>4,40&nbsp;EUR",
			"Kosten Wallbox Wohnung 2</strong></td><td><strong>0,00&nbsp;EUR",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("Ablesungsseite ohne %q", want)
			}
		}
	})

	t.Run("ohne Einzelzaehler und Schalter informativ wie heute", func(t *testing.T) {
		db := openTestDB(t)
		body := detailBody(t, db, wallboxAblesungen(t, db, nil, nil, false))
		if !strings.Contains(body, "Kosten Wallbox</strong></td><td><strong>4,40&nbsp;EUR") {
			t.Error("Kosten Wallbox des Zwischenzaehlers fehlen")
		}
		if strings.Contains(body, "Kosten Wallbox Wohnung") || !strings.Contains(body, "keiner Wohnung zugeordnet") {
			t.Error("ohne Zuordnung soll nur der Hinweis 'keiner Wohnung zugeordnet' erscheinen")
		}
	})
}

func TestAblesungDetail_StromWohnung1(t *testing.T) {
	db := openTestDB(t)
	if _, err := createPeriod(db, seedPeriodInputAt("2026-10-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	second := seedPeriodInputAt("2026-11-01")
	second.Readings["strom_gesamt"] = 100
	second.Readings["strom_wohnung2"] = 20
	second.Readings["strom_waermepumpe"] = 30
	second.Readings["strom_wallbox"] = 10
	id, err := createPeriod(db, second)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	body := detailBody(t, db, id)
	// Rest 100 - 20 - 30 - 10 = 40 kWh, 40 * 0,22 = 8,80 EUR.
	for _, want := range []string{
		"Wohnung 1 Anteil</td><td>40,00&nbsp;kWh",
		"Kosten Wohnung 1</strong></td><td><strong>8,80&nbsp;EUR",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Ablesungsseite ohne %q", want)
		}
	}
}

func TestAblesungDetail_PlausibilitaetsHinweis(t *testing.T) {
	// Zwischenzähler: 40 kWh, Schwelle 2 kWh.
	cases := []struct {
		name    string
		w1, w2  float64
		hinweis bool
	}{
		{"passt", 25, 15, false},
		{"weicht ab", 10, 15, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			first := seedPeriodInputAt("2026-10-01")
			first.Readings[wbW1], first.Readings[wbW2] = 0, 0
			if _, err := createPeriod(db, first); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			second := seedPeriodInputAt("2026-11-01")
			second.Readings["strom_gesamt"], second.Readings["strom_wallbox"] = 100, 40
			second.Readings[wbW1], second.Readings[wbW2] = c.w1, c.w2
			id, err := createPeriod(db, second)
			if err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			body := detailBody(t, db, id)
			if got := strings.Contains(body, "Plausibilität Wallbox-Einzelzähler"); got != c.hinweis {
				t.Errorf("Hinweis sichtbar = %v, want %v", got, c.hinweis)
			}
		})
	}

	t.Run("nur ein Einzelzaehler: kein Hinweis", func(t *testing.T) {
		db := openTestDB(t)
		w2 := 1.0
		body := detailBody(t, db, wallboxAblesungen(t, db, nil, &w2, false))
		if strings.Contains(body, "Plausibilität Wallbox-Einzelzähler") {
			t.Error("Hinweis trotz nur einem Einzelzaehler")
		}
	})
}
