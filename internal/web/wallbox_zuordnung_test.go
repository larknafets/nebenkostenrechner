package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// wallbox_zuordnung_test.go covers the assignment of the Wallbox costs to
// Wohnung 1/2 (#225) through the HTTP seam: the switch "Wallboxen
// vollständig Wohnung 1" in form, Ablesungsseite and CSV.

const wbSchalter = "wallbox_komplett_w1"

// wallboxAblesungen stores a first Ablesung (everything 0, Wallbox Wohnung 2
// at 0) and a second one with 100 kWh Netzbezug, 30 kWh Wohnung 2, 50 kWh
// Wärmepumpe and 40 kWh Wallboxen, so 20 kWh of grid draw are left for the
// Wallboxen (4,40 EUR at 0,22 EUR/kWh). It returns the id of the second.
func wallboxAblesungen(t *testing.T, db *sql.DB, einzelW1, einzelW2 *float64, komplettW1 bool) int64 {
	t.Helper()
	first := seedPeriodInputAt("2026-10-01")
	first.Readings[wbW1], first.Readings[wbW2] = 0, 0
	if _, err := store.CreatePeriod(db, first); err != nil {
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
	second.WallboxKomplettW1 = komplettW1
	id, err := store.CreatePeriod(db, second)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	return id
}

func TestWizard_SchalterUnterDenWallboxFeldern(t *testing.T) {
	body := wizardBody(t, openTestDB(t))

	iW2 := strings.Index(body, `name="`+wbW2+`"`)
	iSchalter := strings.Index(body, `name="`+wbSchalter+`"`)
	if iSchalter < 0 {
		t.Fatalf("kein Schalter %q im Formular", wbSchalter)
	}
	iPane2 := strings.Index(body, `class="c-pane" data-step="2"`)
	if iSchalter < iW2 || iSchalter > iPane2 {
		t.Errorf("Schalter steht nicht in Schritt 1 unter den Wallbox-Feldern (W2 %d, Schalter %d, Schritt 2 %d)", iW2, iSchalter, iPane2)
	}
	if !strings.Contains(body, "Wallboxen vollständig Wohnung 1") {
		t.Error("Schalter ohne Beschriftung 'Wallboxen vollständig Wohnung 1'")
	}
	if tag := inputTag(t, body, wbSchalter); !strings.Contains(tag, `type="checkbox"`) || strings.Contains(tag, "checked") {
		t.Errorf("Schalter einer neuen Ablesung ohne Vorgänger soll ein ungesetztes Kontrollkästchen sein: %s", tag)
	}
}

func TestWizard_SchalterAusLetzterAblesungVorbelegt(t *testing.T) {
	db := openTestDB(t)
	wallboxAblesungen(t, db, nil, nil, true)
	// A later Ablesung with the switch off is the chronologically last one.
	if tag := inputTag(t, wizardBody(t, db), wbSchalter); !strings.Contains(tag, "checked") {
		t.Errorf("Schalter der letzten Ablesung (an) nicht vorbelegt: %s", tag)
	}

	off := seedPeriodInputAt("2026-12-01")
	if _, err := store.CreatePeriod(db, off); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if tag := inputTag(t, wizardBody(t, db), wbSchalter); strings.Contains(tag, "checked") {
		t.Errorf("Schalter der letzten Ablesung (aus) trotzdem vorbelegt: %s", tag)
	}
}

func TestEditWizard_SchalterGiltAusDerAblesung(t *testing.T) {
	db := openTestDB(t)
	id := wallboxAblesungen(t, db, nil, nil, true)
	// The newest Ablesung has the switch off, the edited (older) one on.
	newest := seedPeriodInputAt("2026-12-01")
	if _, err := store.CreatePeriod(db, newest); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10)+"/bearbeiten", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleEditWizardForm(newAuth("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if tag := inputTag(t, w.Body.String(), wbSchalter); !strings.Contains(tag, "checked") {
		t.Errorf("Bearbeiten soll den Schalter der Ablesung zeigen (an): %s", tag)
	}
}

func TestParsePeriodInput_Schalter(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}

	in, err := parsePeriodInput(formRequest(t, vollesFormular()), apartments)
	if err != nil || in.WallboxKomplettW1 {
		t.Errorf("ohne Feld: %v, %v, want aus", in.WallboxKomplettW1, err)
	}

	form := vollesFormular()
	form.Set(wbSchalter, "1")
	in, err = parsePeriodInput(formRequest(t, form), apartments)
	if err != nil || !in.WallboxKomplettW1 {
		t.Errorf("mit Feld: %v, %v, want an", in.WallboxKomplettW1, err)
	}
}

func TestCreateAblesung_SpeichertSchalter(t *testing.T) {
	db := openTestDB(t)
	form := vollesFormular()
	form.Set(wbSchalter, "1")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	latest, err := store.GetLatestPeriod(db)
	if err != nil || latest == nil || !latest.WallboxKomplettW1 {
		t.Errorf("gespeicherte Ablesung = %+v, %v, want Schalter an", latest, err)
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

func TestAblesungDetail_PlausibilitaetsHinweis(t *testing.T) {
	// Zwischenzaehler: 40 kWh, Schwelle 2 kWh.
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
			if _, err := store.CreatePeriod(db, first); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			second := seedPeriodInputAt("2026-11-01")
			second.Readings["strom_gesamt"], second.Readings["strom_wallbox"] = 100, 40
			second.Readings[wbW1], second.Readings[wbW2] = c.w1, c.w2
			id, err := store.CreatePeriod(db, second)
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

func TestCSV_SchalterAlsLetzteSpalte(t *testing.T) {
	if got := csvHeader[len(csvHeader)-1]; got != "wallbox_komplett_wohnung1" {
		t.Fatalf("letzte CSV-Spalte = %q, want wallbox_komplett_wohnung1", got)
	}

	db := openTestDB(t)
	in := seedPeriodInputAt("2026-06-01")
	in.WallboxKomplettW1 = true
	if _, err := store.CreatePeriod(db, in); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-07-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil)
	w := httptest.NewRecorder()
	handleExportCSV()(w, requestWithDB(req, db))
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")), "\n")
	if len(lines) != 3 {
		t.Fatalf("CSV hat %d Zeilen, want 3", len(lines))
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), ";;;1") || !strings.HasSuffix(strings.TrimSpace(lines[2]), ";;;0") {
		t.Errorf("Schalter-Spalte soll 1 und 0 enden: %q / %q", lines[1], lines[2])
	}
}

func TestCSV_ImportSchalter(t *testing.T) {
	row := func(schalter string) string {
		return strings.TrimSuffix(csvRow("2026-06-01", "100"), ";0") + ";" + schalter
	}
	header := strings.Join(csvHeader, ";")

	for schalter, want := range map[string]bool{"1": true, "0": false, "": false} {
		rows, err := parseImportCSV(strings.NewReader(header + "\n" + row(schalter) + "\n"))
		if err != nil {
			t.Fatalf("Schalter %q: parseImportCSV: %v", schalter, err)
		}
		if got := rows[0].input.WallboxKomplettW1; got != want {
			t.Errorf("Schalter %q = %v, want %v", schalter, got, want)
		}
	}

	t.Run("ungültiger Wert", func(t *testing.T) {
		if _, err := parseImportCSV(strings.NewReader(header + "\n" + row("vielleicht") + "\n")); err == nil {
			t.Error("ungültiger Schalter ohne Fehler")
		}
	})

	t.Run("alte Datei ohne die Spalte", func(t *testing.T) {
		oldHeader := strings.TrimSuffix(header, ";wallbox_komplett_wohnung1")
		rows, err := parseImportCSV(strings.NewReader(oldHeader + "\n" + strings.TrimSuffix(csvRow("2026-06-01", "100"), ";0") + "\n"))
		if err != nil {
			t.Fatalf("parseImportCSV (alte Datei): %v", err)
		}
		if rows[0].input.WallboxKomplettW1 {
			t.Error("alte Datei: Schalter an, want aus")
		}
	})
}
