package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// switches_test.go runs every per-Ablesung switch (store.Switches) through
// the form parse and the CSV export and import, so a switch missing in one of
// them fails here.

func TestSwitches_FormularCSVRoundtrip(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}

	for _, s := range store.Switches {
		t.Run(s.Name, func(t *testing.T) {
			// Form: absent field is off, a checked one is on.
			in, err := parsePeriodInput(formRequest(t, vollesFormular()), apartments)
			if err != nil || *s.Input(&in) {
				t.Fatalf("ohne Feld: %v, %v, want aus", *s.Input(&in), err)
			}
			form := vollesFormular()
			form.Set(s.FormName, "1")
			in, err = parsePeriodInput(formRequest(t, form), apartments)
			if err != nil || !*s.Input(&in) {
				t.Fatalf("mit Feld: %v, %v, want an", *s.Input(&in), err)
			}

			// Export: the column is in the header, 1 for the first, 0 for the
			// second Ablesung.
			db := openTestDB(t)
			in.ReadingDate = "2026-06-01"
			if _, err := store.CreatePeriod(db, in); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			second := seedPeriodInputAt("2026-07-01")
			*s.Input(&second) = false
			if _, err := store.CreatePeriod(db, second); err != nil {
				t.Fatalf("CreatePeriod: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/ablesungen/export.csv", nil)
			w := httptest.NewRecorder()
			handleExportCSV()(w, requestWithDB(req, db))
			body := strings.TrimPrefix(w.Body.String(), "\xef\xbb\xbf")
			lines := strings.Split(strings.TrimSpace(body), "\n")
			if len(lines) != 3 {
				t.Fatalf("CSV hat %d Zeilen, want 3", len(lines))
			}
			cols := strings.Split(strings.TrimSpace(lines[0]), ";")
			idx := -1
			for i, c := range cols {
				if c == s.CSVColumn {
					idx = i
				}
			}
			if idx < 0 {
				t.Fatalf("CSV-Spalte %s fehlt im Header %v", s.CSVColumn, cols)
			}

			// Import of the exported file gives the switches back.
			rows, err := parseImportCSV(strings.NewReader(body))
			if err != nil || len(rows) != 2 {
				t.Fatalf("parseImportCSV = %d Zeilen, %v", len(rows), err)
			}
			if !*s.Input(&rows[0].input) || *s.Input(&rows[1].input) {
				t.Errorf("Schalter nach Import = %v / %v, want an / aus", *s.Input(&rows[0].input), *s.Input(&rows[1].input))
			}
		})
	}
}
