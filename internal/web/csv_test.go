package web

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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

func csvRowMitPersonen(readingDate, p1, p2 string) string {
	return strings.Replace(csvRow(readingDate, "100"), ";0,08;2;1;", ";0,08;"+p1+";"+p2+";", 1)
}

func TestHandleImportCSV_PersonenzahlDezimal(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(csvHeader, ";") + "\n" + csvRowMitPersonen("2026-06-01", "1,25", "0,5") + "\n"

	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}

	// Export and re-import into a fresh database: values stay unchanged.
	ex := httptest.NewRecorder()
	handleExportCSV()(ex, requestWithDB(httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil), db))
	db2 := openTestDB(t)
	w2 := httptest.NewRecorder()
	handleImportCSV()(w2, requestWithDB(csvUploadRequest(t, ex.Body.String()), db2))
	if w2.Code != http.StatusFound {
		t.Fatalf("re-import status = %d, want %d (body: %s)", w2.Code, http.StatusFound, w2.Body.String())
	}
	periods, err := store.AllPeriods(db2)
	if err != nil || len(periods) != 1 {
		t.Fatalf("AllPeriods = %v, %v, want 1 period", periods, err)
	}
	got, err := store.PersonenByApartment(db2, periods[0].ID)
	if err != nil {
		t.Fatalf("PersonenByApartment: %v", err)
	}
	if got[1] != 1.25 || got[2] != 0.5 {
		t.Errorf("Personen = %v, want 1: 1.25, 2: 0.5", got)
	}
}

func TestHandleImportCSV_PersonenzahlUngueltigBrichtAb(t *testing.T) {
	for _, bad := range []string{"-1", "1,255"} {
		db := openTestDB(t)
		csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-05-01", "50") + "\n" +
			csvRowMitPersonen("2026-06-01", bad, "1") + "\n"

		w := httptest.NewRecorder()
		handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want %d", bad, w.Code, http.StatusBadRequest)
		}
		if !strings.Contains(w.Body.String(), "Zeile 3") {
			t.Errorf("%q: body = %q, want line number", bad, w.Body.String())
		}
		periods, _ := store.AllPeriods(db)
		if len(periods) != 0 {
			t.Errorf("%q: %d periods imported, want 0", bad, len(periods))
		}
	}
}
