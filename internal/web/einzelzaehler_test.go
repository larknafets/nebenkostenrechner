package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// einzelzaehler_test.go covers the optional Wallbox-Einzelzähler (#224)
// through the HTTP seam: form, Ablesungsseite, CSV, warnings.

const (
	wbW1 = store.MeterKeyWallboxWohnung1
	wbW2 = store.MeterKeyWallboxWohnung2
)

// inputTag returns the <input ...> tag with the given name attribute.
func inputTag(t *testing.T, body, name string) string {
	t.Helper()
	i := strings.Index(body, `name="`+name+`"`)
	if i < 0 {
		t.Fatalf("kein Feld %q im Formular", name)
	}
	start := strings.LastIndex(body[:i], "<input")
	end := i + strings.Index(body[i:], ">")
	return body[start : end+1]
}

func wizardBody(t *testing.T, db *sql.DB) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/neu", nil)
	w := httptest.NewRecorder()
	handleWizardForm(newZugang("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func TestWizard_ReihenfolgeWaermepumpeVorWallboxenMitEinzelzaehlern(t *testing.T) {
	body := wizardBody(t, openTestDB(t))

	order := []string{
		`name="strom_gesamt"`, `name="strom_einspeisung"`, `name="strom_wohnung2"`,
		`name="strom_waermepumpe"`, `name="strom_wallbox"`, `name="` + wbW1 + `"`, `name="` + wbW2 + `"`,
	}
	last := -1
	for _, o := range order {
		i := strings.Index(body, o)
		if i < 0 {
			t.Fatalf("Feld %s fehlt im Formular", o)
		}
		if i <= last {
			t.Errorf("Feld %s steht vor seinem Vorgänger, Reihenfolge soll Netzbezug, Einspeisung, Wohnung 2, Wärmepumpe, Wallboxen, Wallbox Wohnung 1, Wallbox Wohnung 2 sein", o)
		}
		last = i
	}
	for _, label := range []string{"Wallbox Wohnung 1 (kWh)", "Wallbox Wohnung 2 (kWh)"} {
		if !strings.Contains(body, label) {
			t.Errorf("Formular ohne Beschriftung %q", label)
		}
	}
}

func TestWizard_EinzelzaehlerWarnbasisNurMitVorstand(t *testing.T) {
	db := openTestDB(t)
	in := seedPeriodInputAt("2026-10-01")
	in.Readings[wbW2] = 40
	if _, err := store.CreatePeriod(db, in); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	body := wizardBody(t, db)

	if tag := inputTag(t, body, wbW2); !strings.Contains(tag, `data-prev="40"`) {
		t.Errorf("Wallbox Wohnung 2 ohne data-prev des Vorstands: %s", tag)
	}
	if tag := inputTag(t, body, wbW1); strings.Contains(tag, "data-prev") {
		t.Errorf("Wallbox Wohnung 1 ohne Vorstand darf kein data-prev tragen (sonst Vergleich gegen 0): %s", tag)
	}
}

func TestWizard_EinzelzaehlerZaehlenNichtZumTeilstandFortschritt(t *testing.T) {
	body := wizardBody(t, openTestDB(t))

	for _, name := range []string{wbW1, wbW2} {
		if tag := inputTag(t, body, name); !strings.Contains(tag, "data-optional") {
			t.Errorf("Feld %s nicht als optional markiert, der Teilstand-Fortschritt würde es mitzählen: %s", name, tag)
		}
	}
}

func formRequest(t *testing.T, values url.Values) *http.Request {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(values.Encode()))
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func vollesFormular() url.Values {
	form := url.Values{
		"reading_date":       {"2026-11-01"},
		"monat":              {"2026-11"},
		"strompreis":         {"0.22"},
		"frischwasser_preis": {"1.46"},
		"abwasser_preis":     {"4.87"},
		"einspeisung_preis":  {"0.08"},
		"personen_1":         {"2"},
		"personen_2":         {"1"},
	}
	for _, key := range store.MeterKeys {
		form.Set(key, "1")
	}
	return form
}

func TestParsePeriodInput_Einzelzaehler(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}

	t.Run("leer bleibt nicht erfasst", func(t *testing.T) {
		in, err := parsePeriodInput(formRequest(t, vollesFormular()), apartments)
		if err != nil {
			t.Fatalf("parsePeriodInput: %v", err)
		}
		if _, ok := in.Readings[wbW1]; ok {
			t.Error("leeres Feld Wallbox Wohnung 1 als Stand übernommen")
		}
		if _, ok := in.Readings[wbW2]; ok {
			t.Error("leeres Feld Wallbox Wohnung 2 als Stand übernommen")
		}
	})

	t.Run("nur Wohnung 2", func(t *testing.T) {
		form := vollesFormular()
		form.Set(wbW2, "12.5")
		in, err := parsePeriodInput(formRequest(t, form), apartments)
		if err != nil {
			t.Fatalf("parsePeriodInput: %v", err)
		}
		if v, ok := in.Readings[wbW2]; !ok || v != 12.5 {
			t.Errorf("Wallbox Wohnung 2 = %v, %v, want 12.5, true", v, ok)
		}
		if _, ok := in.Readings[wbW1]; ok {
			t.Error("Wallbox Wohnung 1 trotz leerem Feld übernommen")
		}
	})

	t.Run("0 ist ein Stand", func(t *testing.T) {
		form := vollesFormular()
		form.Set(wbW1, "0")
		in, _ := parsePeriodInput(formRequest(t, form), apartments)
		if v, ok := in.Readings[wbW1]; !ok || v != 0 {
			t.Errorf("Wallbox Wohnung 1 = %v, %v, want 0, true", v, ok)
		}
	})

	t.Run("Text ist ein Fehler", func(t *testing.T) {
		form := vollesFormular()
		form.Set(wbW1, "abc")
		if _, err := parsePeriodInput(formRequest(t, form), apartments); err == nil {
			t.Error("ungültiger Wert für Wallbox Wohnung 1 ohne Fehler")
		}
	})
}

func TestCreateAblesung_OhneEinzelzaehlerBleibtVollstaendig(t *testing.T) {
	db := openTestDB(t)
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, vollesFormular()), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	latest, err := store.GetLatestPeriod(db)
	if err != nil || latest == nil {
		t.Fatalf("GetLatestPeriod = %v, %v", latest, err)
	}
	ok, err := store.PeriodComplete(db, latest.ID)
	if err != nil || !ok {
		t.Errorf("PeriodComplete = %v, %v, want true (Einzelzähler leer = kein Teilstand)", ok, err)
	}
}

func detailBody(t *testing.T, db *sql.DB, id int64) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleAblesungDetail(newZugang("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", w.Code, w.Body.String())
	}
	return w.Body.String()
}

// tabellenzeile returns the table row of the Ablesungsseite whose label cell is label.
func tabellenzeile(t *testing.T, body, label string) string {
	t.Helper()
	i := strings.Index(body, ">"+label+"</td>")
	if i < 0 {
		t.Fatalf("keine Zeile %q auf der Ablesungsseite", label)
	}
	end := strings.Index(body[i:], "</tr>")
	return body[i : i+end]
}

func TestAblesungDetail_Einzelzaehler(t *testing.T) {
	db := openTestDB(t)
	vor := seedPeriodInputAt("2026-10-01")
	vor.Readings[wbW1] = 100
	if _, err := store.CreatePeriod(db, vor); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	cur := seedPeriodInputAt("2026-11-01")
	cur.Readings[wbW1] = 160.5
	cur.Readings[wbW2] = 30
	id, err := store.CreatePeriod(db, cur)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	body := detailBody(t, db, id)

	w1 := tabellenzeile(t, body, "Wallbox Wohnung 1")
	if !strings.Contains(w1, "160,500") || !strings.Contains(w1, "60,50") {
		t.Errorf("Wallbox Wohnung 1 ohne Stand und Verbrauch in Klammern: %s", w1)
	}
	w2 := tabellenzeile(t, body, "Wallbox Wohnung 2")
	if !strings.Contains(w2, "30,000") || !strings.Contains(w2, "nicht berechenbar") {
		t.Errorf("Wallbox Wohnung 2 (Vorstand fehlt) soll den Stand mit 'nicht berechenbar' zeigen: %s", w2)
	}
	if strings.Contains(body, "noch offen") || strings.Contains(body, "unvollständig") {
		t.Error("vollständige Ablesung zeigt 'noch offen' oder 'unvollständig'")
	}
}

func TestAblesungDetail_EinzelzaehlerLeerIstNichtErfasstNichtOffen(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01"))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	body := detailBody(t, db, id)

	for _, label := range []string{"Wallbox Wohnung 1", "Wallbox Wohnung 2"} {
		z := tabellenzeile(t, body, label)
		if !strings.Contains(z, "nicht erfasst") {
			t.Errorf("%s ohne Stand soll 'nicht erfasst' zeigen: %s", label, z)
		}
		if strings.Contains(z, "noch offen") || strings.Contains(z, "text-warning") {
			t.Errorf("%s ohne Stand zeigt die gelbe Warnung: %s", label, z)
		}
	}
	if strings.Contains(body, "noch offen") {
		t.Error("vollständige Ablesung zeigt 'noch offen'")
	}
}

func TestAblesungDetail_ZaehlerReihenfolge(t *testing.T) {
	db := openTestDB(t)
	id, _ := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01"))
	body := detailBody(t, db, id)

	last := -1
	for _, label := range []string{"Strom Wärmepumpe", "Strom Wallboxen", "Wallbox Wohnung 1", "Wallbox Wohnung 2"} {
		i := strings.Index(body, ">"+label+"</td>")
		if i < 0 || i <= last {
			t.Errorf("Zeile %q fehlt oder steht an falscher Stelle", label)
		}
		last = i
	}
}

func TestCSV_ExportHatEinzelzaehlerAmEnde(t *testing.T) {
	if n := len(csvHeader); n < 4 || csvHeader[n-4] != wbW1 || csvHeader[n-3] != wbW2 {
		t.Fatalf("csvHeader endet mit %v, want %s und %s vor den Schaltern", csvHeader[len(csvHeader)-4:], wbW1, wbW2)
	}

	db := openTestDB(t)
	in := seedPeriodInputAt("2026-06-01")
	in.Readings[wbW2] = 12.5
	if _, err := store.CreatePeriod(db, in); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil)
	w := httptest.NewRecorder()
	handleExportCSV()(w, requestWithDB(req, db))

	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")), "\n")
	if len(lines) != 2 {
		t.Fatalf("CSV hat %d Zeilen, want 2", len(lines))
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), ";"+wbW1+";"+wbW2+";wallbox_komplett_wohnung1;pv_gutschrift_wohnung1") {
		t.Errorf("Kopfzeile endet nicht mit den Einzelzählern und dem Schalter: %s", lines[0])
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), ";2;1;;12,5;0;0") {
		t.Errorf("Datenzeile soll leer (nicht erfasst) für Wohnung 1 und 12,5 für Wohnung 2 enden: %s", lines[1])
	}
}

func TestCSV_ImportMitUndOhneEinzelzaehlerSpalten(t *testing.T) {
	t.Run("neue Datei mit Wert und leerer Zelle", func(t *testing.T) {
		row := strings.TrimSuffix(csvRow("2026-06-01", "100"), ";;;0;0") + ";7,5;;0;0"
		rows, err := parseImportCSV(strings.NewReader(strings.Join(csvHeader, ";") + "\n" + row + "\n"))
		if err != nil {
			t.Fatalf("parseImportCSV: %v", err)
		}
		r := rows[0].input.Readings
		if v, ok := r[wbW1]; !ok || v != 7.5 {
			t.Errorf("Wallbox Wohnung 1 = %v, %v, want 7,5", v, ok)
		}
		if _, ok := r[wbW2]; ok {
			t.Error("leere Zelle Wallbox Wohnung 2 als Stand übernommen")
		}
	})

	t.Run("alte Datei ohne die Spalten", func(t *testing.T) {
		header := strings.TrimSuffix(strings.Join(csvHeader, ";"), ";"+wbW1+";"+wbW2+";wallbox_komplett_wohnung1;pv_gutschrift_wohnung1")
		row := strings.TrimSuffix(csvRow("2026-06-01", "100"), ";;;0;0")
		rows, err := parseImportCSV(strings.NewReader(header + "\n" + row + "\n"))
		if err != nil {
			t.Fatalf("parseImportCSV (alte Datei): %v", err)
		}
		r := rows[0].input.Readings
		if _, ok := r[wbW1]; ok {
			t.Error("alte Datei: Wallbox Wohnung 1 trotz fehlender Spalte gesetzt")
		}
		if _, ok := r[wbW2]; ok {
			t.Error("alte Datei: Wallbox Wohnung 2 trotz fehlender Spalte gesetzt")
		}
		if len(r) != len(store.MeterKeys) {
			t.Errorf("alte Datei: %d Stände, want %d Pflichtzähler", len(r), len(store.MeterKeys))
		}
	})

	t.Run("ungültiger Wert", func(t *testing.T) {
		row := strings.TrimSuffix(csvRow("2026-06-01", "100"), ";;;0;0") + ";abc;;0;0"
		if _, err := parseImportCSV(strings.NewReader(strings.Join(csvHeader, ";") + "\n" + row + "\n")); err == nil {
			t.Error("ungültiger Wert in Einzelzähler-Spalte ohne Fehler")
		}
	})
}

func TestImportWarnings_Einzelzaehler(t *testing.T) {
	mk := func(line int, date string, w1, w2 *float64) importRow {
		r := make(map[string]float64)
		for _, k := range store.MeterKeys {
			r[k] = 1000
		}
		if w1 != nil {
			r[wbW1] = *w1
		}
		if w2 != nil {
			r[wbW2] = *w2
		}
		return importRow{input: store.PeriodInput{ReadingDate: date, Readings: r}, line: line}
	}
	f := store.Float64

	t.Run("negativer Verbrauch bei beiden Ständen", func(t *testing.T) {
		w := importWarnings([]importRow{mk(2, "2026-05-01", f(100), nil), mk(3, "2026-06-01", f(90), nil)}, []int64{1, 2})
		if len(w) != 1 || !strings.Contains(w[0], wbW1) || !strings.Contains(w[0], "negativer Verbrauch") {
			t.Errorf("warnings = %v, want genau einen negativen Verbrauch bei %s", w, wbW1)
		}
	})

	t.Run("fehlender Stand wird übersprungen", func(t *testing.T) {
		w := importWarnings([]importRow{mk(2, "2026-05-01", nil, nil), mk(3, "2026-06-01", f(90), f(5))}, []int64{1, 2})
		if len(w) != 0 {
			t.Errorf("warnings = %v, want keine (Vorperiode ohne Stand, nie gegen 0 vergleichen)", w)
		}
		w = importWarnings([]importRow{mk(2, "2026-05-01", f(100), nil), mk(3, "2026-06-01", nil, nil)}, []int64{1, 2})
		if len(w) != 0 {
			t.Errorf("warnings = %v, want keine (aktueller Stand fehlt)", w)
		}
	})

	t.Run("Ausreißer nur bei vier vollständigen Ständen", func(t *testing.T) {
		rows := []importRow{
			mk(2, "2026-01-01", f(0), nil), mk(3, "2026-02-01", f(10), nil), mk(4, "2026-03-01", f(20), nil),
			mk(5, "2026-04-01", f(30), nil), mk(6, "2026-05-01", f(130), nil),
		}
		w := importWarnings(rows, []int64{1, 2, 3, 4, 5})
		if len(w) != 1 || !strings.Contains(w[0], "Ausreißer") || !strings.Contains(w[0], wbW1) {
			t.Errorf("warnings = %v, want genau einen Ausreißer bei %s", w, wbW1)
		}

		// A gap in the history disables the baseline for that meter.
		rows[2] = mk(4, "2026-03-01", nil, nil)
		w = importWarnings(rows, []int64{1, 2, 3, 4, 5})
		if len(w) != 0 {
			t.Errorf("warnings = %v, want keine (Lücke in der Historie)", w)
		}
	})
}
