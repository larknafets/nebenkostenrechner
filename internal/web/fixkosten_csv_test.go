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
		Personen: map[int64]float64{1: 2, 2: 1},
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
	cells = append(cells, "2", "1", "150", "90", "", "800")
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
	if _, err := part.Write([]byte(csvText)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

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

func fixkostenCSVRowMitPersonen(monat, p1, p2 string) string {
	return strings.TrimSuffix(fixkostenCSVRow(monat, "100"), "2;1;150;90;;800") + p1 + ";" + p2 + ";150;90;;800"
}

func TestHandleImportFixkostenCSV_PersonenzahlDezimal(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + fixkostenCSVRowMitPersonen("2026-06-01", "1,25", "0,5") + "\n"

	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d (body: %s)", w.Code, http.StatusFound, w.Body.String())
	}

	ex := httptest.NewRecorder()
	handleExportFixkostenCSV()(ex, requestWithDB(httptest.NewRequest(http.MethodGet, "/fixkosten/export.csv", nil), db))
	db2 := openTestDB(t)
	w2 := httptest.NewRecorder()
	handleImportFixkostenCSV()(w2, requestWithDB(fixkostenCSVUploadRequest(t, ex.Body.String()), db2))
	if w2.Code != http.StatusFound {
		t.Fatalf("re-import status = %d, want %d (body: %s)", w2.Code, http.StatusFound, w2.Body.String())
	}
	eingaben, err := store.AllFixkostenEingabenDetails(db2)
	if err != nil || len(eingaben) != 1 {
		t.Fatalf("AllFixkostenEingaben = %v, %v, want 1", eingaben, err)
	}
	if got := eingaben[0].Personen; got[1] != 1.25 || got[2] != 0.5 {
		t.Errorf("Personen = %v, want 1: 1.25, 2: 0.5", got)
	}
}

func TestHandleImportFixkostenCSV_PersonenzahlUngueltigBrichtAb(t *testing.T) {
	for _, bad := range []string{"1,255"} {
		db := openTestDB(t)
		csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + fixkostenCSVRow("2026-05-01", "100") + "\n" +
			fixkostenCSVRowMitPersonen("2026-06-01", bad, "1") + "\n"

		w := httptest.NewRecorder()
		handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%q: status = %d, want %d", bad, w.Code, http.StatusBadRequest)
		}
		if !strings.Contains(w.Body.String(), "Zeile 3") {
			t.Errorf("%q: body = %q, want line number", bad, w.Body.String())
		}
		eingaben, _ := store.AllFixkostenEingaben(db)
		if len(eingaben) != 0 {
			t.Errorf("%q: %d imported, want 0", bad, len(eingaben))
		}
	}
}
