package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// pv_gutschrift_test.go covers the switch "PV-Gutschrift Wohnung 1" (#226)
// through the HTTP seam: form, Ablesungsseite and CSV.

const pvSchalter = "pv_gutschrift_w1"

func TestWizard_PVSchalterInSchritt4BeimEinspeisepreis(t *testing.T) {
	body := wizardBody(t, openTestDB(t))

	iPreis := strings.Index(body, `name="einspeisung_preis"`)
	iSchalter := strings.Index(body, `name="`+pvSchalter+`"`)
	if iSchalter < 0 {
		t.Fatalf("kein Schalter %q im Formular", pvSchalter)
	}
	iActions := strings.Index(body, `class="wizard-actions"`)
	if iSchalter < iPreis || iSchalter > iActions {
		t.Errorf("Schalter steht nicht in Schritt 4 beim Einspeisepreis (Preis %d, Schalter %d, Aktionen %d)", iPreis, iSchalter, iActions)
	}
	if !strings.Contains(body, "PV-Gutschrift Wohnung 1") {
		t.Error("Schalter ohne Beschriftung 'PV-Gutschrift Wohnung 1'")
	}
	if tag := inputTag(t, body, pvSchalter); !strings.Contains(tag, `type="checkbox"`) || strings.Contains(tag, "checked") {
		t.Errorf("Schalter einer neuen Ablesung ohne Vorgänger soll ein ungesetztes Kontrollkästchen sein: %s", tag)
	}
}

func TestWizard_PVSchalterAusLetzterAblesungVorbelegt(t *testing.T) {
	db := openTestDB(t)
	on := seedPeriodInputAt("2026-10-01")
	on.PVGutschriftW1 = true
	if _, err := store.CreatePeriod(db, on); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if tag := inputTag(t, wizardBody(t, db), pvSchalter); !strings.Contains(tag, "checked") {
		t.Errorf("Schalter der letzten Ablesung (an) nicht vorbelegt: %s", tag)
	}

	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if tag := inputTag(t, wizardBody(t, db), pvSchalter); strings.Contains(tag, "checked") {
		t.Errorf("Schalter der letzten Ablesung (aus) trotzdem vorbelegt: %s", tag)
	}
}

func TestEditWizard_PVSchalterGiltAusDerAblesung(t *testing.T) {
	db := openTestDB(t)
	in := seedPeriodInputAt("2026-10-01")
	in.PVGutschriftW1 = true
	id, err := store.CreatePeriod(db, in)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10)+"/bearbeiten", nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleEditWizardForm(newAuth("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if tag := inputTag(t, w.Body.String(), pvSchalter); !strings.Contains(tag, "checked") {
		t.Errorf("Bearbeiten soll den Schalter der Ablesung zeigen (an): %s", tag)
	}
}

func TestCreateAblesung_SpeichertPVSchalter(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}
	in, err := parsePeriodInput(formRequest(t, vollesFormular()), apartments)
	if err != nil || in.PVGutschriftW1 {
		t.Errorf("ohne Feld: %v, %v, want aus", in.PVGutschriftW1, err)
	}

	db := openTestDB(t)
	form := vollesFormular()
	form.Set(pvSchalter, "1")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	latest, err := store.GetLatestPeriod(db)
	if err != nil || latest == nil || !latest.PVGutschriftW1 {
		t.Errorf("gespeicherte Ablesung = %+v, %v, want Schalter an", latest, err)
	}
}

func TestAblesungDetail_PVSchalterAlsJaNein(t *testing.T) {
	for _, c := range []struct {
		an   bool
		want string
	}{{true, "Ja"}, {false, "Nein"}} {
		db := openTestDB(t)
		first := seedPeriodInputAt("2026-10-01")
		if _, err := store.CreatePeriod(db, first); err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		second := seedPeriodInputAt("2026-11-01")
		second.Readings["strom_einspeisung"] = 52.5 // * 0,08 EUR/kWh = 4,20 EUR
		second.PVGutschriftW1 = c.an
		id, err := store.CreatePeriod(db, second)
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		body := detailBody(t, db, id)
		if !strings.Contains(body, "PV-Gutschrift Wohnung 1:</strong> "+c.want+"<") {
			t.Errorf("Schalter %v: Ablesungsseite ohne %q", c.an, c.want)
		}
		// The house-wide table "Einspeisevergütung" stays either way.
		if !strings.Contains(body, "Einspeisevergütung (PV)") || !strings.Contains(body, "4,20") {
			t.Errorf("Schalter %v: haus-weite Einspeisevergütung fehlt", c.an)
		}
		if got := strings.Contains(body, "Gutschrift Wohnung 1</td>"); got != c.an {
			t.Errorf("Schalter %v: Zeile Gutschrift Wohnung 1 sichtbar = %v", c.an, got)
		}
	}
}

func TestCSV_PVSchalterAlsLetzteSpalte(t *testing.T) {
	if got := csvHeader[len(csvHeader)-1]; got != "pv_gutschrift_wohnung1" {
		t.Fatalf("letzte CSV-Spalte = %q, want pv_gutschrift_wohnung1", got)
	}

	db := openTestDB(t)
	in := seedPeriodInputAt("2026-06-01")
	in.PVGutschriftW1 = true
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
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), ";0;1") || !strings.HasSuffix(strings.TrimSpace(lines[2]), ";0;0") {
		t.Errorf("PV-Spalte soll 1 und 0 enden (davor der Wallbox-Schalter): %q / %q", lines[1], lines[2])
	}
}

func TestCSV_ImportPVSchalter(t *testing.T) {
	row := func(schalter string) string {
		return strings.TrimSuffix(csvRow("2026-06-01", "100"), ";0") + ";" + schalter
	}
	header := strings.Join(csvHeader, ";")

	for schalter, want := range map[string]bool{"1": true, "0": false, "": false} {
		rows, err := parseImportCSV(strings.NewReader(header + "\n" + row(schalter) + "\n"))
		if err != nil {
			t.Fatalf("Schalter %q: parseImportCSV: %v", schalter, err)
		}
		if got := rows[0].input.PVGutschriftW1; got != want {
			t.Errorf("Schalter %q = %v, want %v", schalter, got, want)
		}
	}

	t.Run("ungültiger Wert", func(t *testing.T) {
		if _, err := parseImportCSV(strings.NewReader(header + "\n" + row("vielleicht") + "\n")); err == nil {
			t.Error("ungültiger Schalter ohne Fehler")
		}
	})

	t.Run("Datei aus #225 ohne die Spalte", func(t *testing.T) {
		oldHeader := strings.TrimSuffix(header, ";pv_gutschrift_wohnung1")
		rows, err := parseImportCSV(strings.NewReader(oldHeader + "\n" + strings.TrimSuffix(csvRow("2026-06-01", "100"), ";0") + "\n"))
		if err != nil {
			t.Fatalf("parseImportCSV (alte Datei): %v", err)
		}
		if rows[0].input.PVGutschriftW1 {
			t.Error("alte Datei: Schalter an, want aus")
		}
	})
}
