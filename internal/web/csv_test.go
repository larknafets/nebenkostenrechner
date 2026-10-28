package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// csv_test.go covers the handler seam (status code, headers, redirect) -
// parseImportCSV/parseImportRow's row-level logic stays leaf-tested above.

func seedPeriodInput() store.PeriodInput {
	readings := make(map[string]float64, len(store.MeterKeys))
	for _, key := range store.MeterKeys {
		readings[key] = 0
	}
	return store.PeriodInput{
		ReadingDate:       "2026-06-01",
		Monat:             "2026-06-01",
		Strompreis:        store.Float64(0.22),
		FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis:     store.Float64(4.87),
		EinspeisungPreis:  store.Float64(0.08),
		Readings:          readings,
		Personen:          map[int64]float64{1: 2, 2: 1},
	}
}

func TestHandleExportCSV(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInput()); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil)
	w := httptest.NewRecorder()
	handleExportCSV()(w, requestWithDB(req, db))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv prefix", ct)
	}
	body := w.Body.Bytes()
	if len(body) < 3 || body[0] != 0xEF || body[1] != 0xBB || body[2] != 0xBF {
		t.Error("body missing UTF-8 BOM")
	}
	if !strings.Contains(string(body), "2026-06-01") {
		t.Error("body missing seeded ReadingDate")
	}
}

func csvUploadRequest(t *testing.T, csvText string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("csv", "ablesungen.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write([]byte(csvText)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/ablesungen/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestHandleImportCSV_Success(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(csvHeader, ";") + "\n" +
		csvRow("2026-06-01", "100") + "\n" +
		csvRow("2026-07-01", "210") + "\n"

	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "imported=2") {
		t.Errorf("Location = %q, want imported=2", loc)
	}

	periods, err := store.AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(periods) != 2 {
		t.Errorf("len(periods) = %d, want 2", len(periods))
	}
}

func TestHandleImportCSV_RejectsWhenDataExists(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInput()); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-06-01", "100") + "\n"
	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleImportCSV_BadRowRejectedWithoutPersisting(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-06-01", "nicht-numerisch") + "\n"

	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	periods, err := store.AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(periods) != 0 {
		t.Errorf("len(periods) = %d, want 0 (bad row must abort whole import)", len(periods))
	}
}

// TestHandleImportCSV_AlteDateiVerwirftWerte: a file in the old format (prices,
// Personenzahl and switches as columns) imports, the values are dropped and a
// note says so (#359).
func TestHandleImportCSV_AlteDateiVerwirftWerte(t *testing.T) {
	db := openTestDB(t)
	csvText := csvKopfAlt + "\n" + csvRowAlt("2026-06-01", "100") + "\n"

	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	hinweise := strings.Join(loc.Query()["warning"], "\n")
	if !strings.Contains(hinweise, "Preise, Personenzahlen oder Häkchen") || !strings.Contains(hinweise, "nicht übernommen") {
		t.Errorf("Hinweise = %q, want the note about dropped values", hinweise)
	}

	periods, err := store.AllPeriods(db)
	if err != nil || len(periods) != 1 {
		t.Fatalf("AllPeriods = %v, %v, want 1 period", periods, err)
	}
	got, err := store.GetPeriodDetails(db, periods[0].ID)
	if err != nil || got == nil {
		t.Fatalf("GetPeriodDetails = %v, %v", got, err)
	}
	if got.Strompreis != nil || len(got.PersonenByApartment) != 0 {
		t.Errorf("Ablesung trägt Preise oder Personen: %+v", got)
	}
}

// TestHandleImportCSV_NeueDateiOhneVerworfeneWerte: a file in the new format
// (no price, Personenzahl and switch columns) imports without the note about
// dropped values, and the export gives the new format back.
func TestHandleImportCSV_NeueDateiOhneVerworfeneWerte(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-06-01", "100") + "\n"
	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	if loc := w.Header().Get("Location"); strings.Contains(loc, "Personenzahlen") {
		t.Errorf("Location = %q, want no note about dropped values", loc)
	}

	ex := httptest.NewRecorder()
	handleExportCSV()(ex, requestWithDB(httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil), db))
	body := strings.TrimPrefix(ex.Body.String(), "\xef\xbb\xbf")
	if head := strings.TrimSpace(strings.Split(body, "\n")[0]); head != strings.Join(csvHeader, ";") {
		t.Errorf("Export-Kopfzeile = %q, want %q", head, strings.Join(csvHeader, ";"))
	}
	for _, spalte := range csvVerworfeneSpalten {
		if strings.Contains(body, spalte) {
			t.Errorf("Export enthält die Spalte %s", spalte)
		}
	}
	// Export and re-import into a fresh database works.
	db2 := openTestDB(t)
	w2 := httptest.NewRecorder()
	handleImportCSV()(w2, requestWithDB(csvUploadRequest(t, ex.Body.String()), db2))
	if w2.Code != http.StatusFound {
		t.Fatalf("re-import status = %d, want %d (body: %s)", w2.Code, http.StatusFound, w2.Body.String())
	}
}

// TestHandleImportCSV_SammelmeldungNenntMonateOhneFixkosten: the note after an
// import names the Abrechnungsmonate without a complete Fixkosten-Eingabe, once
// each, and leaves out a month that has one.
func TestHandleImportCSV_SammelmeldungNenntMonateOhneFixkosten(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreateFixkostenEingabe(db, fixkostenMitPreisen("2026-07-01")); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	csvText := strings.Join(csvHeader, ";") + "\n" +
		csvRow("2026-06-01", "100") + "\n" + csvRow("2026-06-15", "110") + "\n" +
		csvRow("2026-07-01", "120") + "\n" + csvRow("2026-08-01", "130") + "\n"
	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatalf("Location: %v", err)
	}
	var sammel string
	for _, h := range loc.Query()["warning"] {
		if strings.Contains(h, "fehlen die Fixkosten") {
			sammel = h
		}
	}
	if !strings.HasPrefix(sammel, "Für Juni 2026, August 2026 fehlen die Fixkosten") || strings.Contains(sammel, "Juli") {
		t.Errorf("Sammelmeldung = %q, want Juni 2026 and August 2026 once, not Juli", sammel)
	}
}

// TestHandleImportCSV_UnvollstaendigeEingabeZaehltAlsFehlend: an Eingabe
// without a price does not count as complete.
func TestHandleImportCSV_UnvollstaendigeEingabeZaehltAlsFehlend(t *testing.T) {
	db := openTestDB(t)
	fk := fixkostenMitPreisen("2026-06-01")
	fk.Strompreis = nil
	if _, err := store.CreateFixkostenEingabe(db, fk); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-06-01", "100") + "\n"
	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "Juni+2026+fehlen+die+Fixkosten") {
		t.Errorf("Location = %q, want the note for Juni 2026", loc)
	}
}
