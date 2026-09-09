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

// fixkosten_csv_test.go covers the handler seam (status code, headers,
// redirect) - parseImportFixkostenCSV/parseImportFixkostenRow's row-level
// logic stays leaf-tested below.

func seedFixkostenInput(monat string) store.FixkostenInput {
	werte := make(map[int64]store.FixkostenPositionWert, len(store.KostenpositionDefaults))
	for _, kd := range store.KostenpositionDefaults {
		werte[kd.ID] = store.FixkostenPositionWert{Logik: kd.Logik, Typ: kd.Typ, Wert: 100}
	}
	return store.FixkostenInput{
		Monat:    monat,
		Personen: map[int64]int64{1: 2, 2: 1},
		Werte:    werte,
		Abschlag: map[int64]float64{1: 150, 2: 90},
	}
}

func TestHandleExportFixkostenCSV(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-06-01")); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/fixkosten/export.csv", nil)
	w := httptest.NewRecorder()
	handleExportFixkostenCSV()(w, requestWithDB(req, db))

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
		t.Error("body missing seeded Monat")
	}
}

func fixkostenCSVRow(monat string, wert string) string {
	cells := []string{monat}
	for _, kd := range store.KostenpositionDefaults {
		cells = append(cells, kd.Logik, kd.Typ, wert)
	}
	cells = append(cells, "2", "1", "150", "90")
	return strings.Join(cells, ";")
}

func fixkostenCSVUploadRequest(t *testing.T, csvText string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("csv", "fixkosten.csv")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	part.Write([]byte(csvText))
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/fixkosten/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestHandleImportFixkostenCSV_Success(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" +
		fixkostenCSVRow("2026-06-01", "100") + "\n" +
		fixkostenCSVRow("2026-07-01", "210") + "\n"

	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "imported=2") {
		t.Errorf("Location = %q, want imported=2", loc)
	}

	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(eingaben) != 2 {
		t.Errorf("len(eingaben) = %d, want 2", len(eingaben))
	}
}

func TestHandleImportFixkostenCSV_RejectsWhenDataExists(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-06-01")); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}

	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + fixkostenCSVRow("2026-06-01", "100") + "\n"
	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleImportFixkostenCSV_BadRowRejectedWithoutPersisting(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + fixkostenCSVRow("2026-06-01", "nicht-numerisch") + "\n"

	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		t.Fatalf("AllFixkostenEingaben: %v", err)
	}
	if len(eingaben) != 0 {
		t.Errorf("len(eingaben) = %d, want 0 (bad row must abort whole import)", len(eingaben))
	}
}
