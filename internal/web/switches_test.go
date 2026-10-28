package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// switches_test.go: the per-Ablesung switches belong to the Fixkosten-Eingabe
// (#359). Neither the form nor the CSV export carries them, the form ignores a
// posted value and the CSV import accepts the column and drops the value.

func TestSwitches_NichtMehrInFormularUndCSV(t *testing.T) {
	for _, s := range store.Switches {
		t.Run(s.Name, func(t *testing.T) {
			form := vollesFormular()
			form.Set(s.FormName, "1")
			in, err := parsePeriodInput(formRequest(t, form))
			if err != nil || *s.Input(&in) {
				t.Errorf("Formular mit Feld: Schalter %v, %v, want aus (Feld ignoriert)", *s.Input(&in), err)
			}

			db := openTestDB(t)
			if _, err := createPeriod(db, seedPeriodInputAt("2026-06-01")); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil)
			w := httptest.NewRecorder()
			handleExportCSV()(w, requestWithDB(req, db))
			body := strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")
			header := strings.Split(strings.TrimSpace(strings.Split(body, "\n")[0]), ";")
			for _, c := range header {
				if c == s.CSVColumn {
					t.Errorf("CSV-Export enthält die Spalte %s", s.CSVColumn)
				}
			}

			// An old file with the column is accepted, the value is dropped.
			alt := strings.Join(csvImportHeader, ";") + ";" + s.CSVColumn + "\n" + strings.TrimSuffix(csvRow("2026-06-01", "100"), ";;") + ";1\n"
			rows, err := parseImportCSV(strings.NewReader(alt))
			if err != nil || len(rows) != 1 {
				t.Fatalf("parseImportCSV (alte Datei) = %d Zeilen, %v", len(rows), err)
			}
			if *s.Input(&rows[0].input) {
				t.Errorf("Schalter %s aus der alten Datei übernommen", s.CSVColumn)
			}
		})
	}
}
