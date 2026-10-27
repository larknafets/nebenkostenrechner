package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Smoke tests for the message mapping of store.PeriodInput.Validate in the
// two adapters: texts, status codes and CSV line numbers.

func TestWizard_EingabeFehlerTexte(t *testing.T) {
	cases := []struct {
		name     string
		feld     string
		wert     string
		wantBody string
		wantFeld string
	}{
		{"Personenzahl Nachkommastellen", "personen_1", "1.255",
			"ungültige Personenzahl für Wohnung 1: Personenzahl darf höchstens 2 Nachkommastellen haben", ""},
		{"Personenzahl negativ", "personen_2", "-1",
			"ungültige Personenzahl für Wohnung 2: Personenzahl darf nicht negativ sein", ""},
		{"Ablesedatum direkter POST", "reading_date", "gestern",
			`ungültiges Ablesedatum "gestern" (Format JJJJ-MM-TT)`, "reading_date"},
		{"Abrechnungsmonat direkter POST", "monat", "Juli",
			`ungültiger Abrechnungsmonat "Juli" (Format JJJJ-MM)`, "monat"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			form := vollesFormular()
			form.Set(c.feld, c.wert)
			w := httptest.NewRecorder()
			handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), c.wantBody) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), c.wantBody)
			}
			if got := w.Header().Get(fehlerFeldHeader); got != c.wantFeld {
				t.Errorf("Fehler-Feld = %q, want %q", got, c.wantFeld)
			}
			if all, _ := store.AllPeriods(db); len(all) != 0 {
				t.Errorf("AllPeriods = %d, want 0", len(all))
			}
		})
	}
}

func TestCSV_EingabeFehlerTexteMitZeilennummer(t *testing.T) {
	idx := func(col string) int {
		for i, h := range csvHeader {
			if h == col {
				return i
			}
		}
		t.Fatalf("Spalte %s fehlt", col)
		return -1
	}
	cases := []struct {
		name string
		col  string
		wert string
		want string
	}{
		{"Ablesedatum", "reading_date", "01.07.2026",
			`Zeile 3: ungültiges Ablesedatum "01.07.2026" (Format JJJJ-MM-TT)`},
		{"Abrechnungsmonat", "monat", "Juli",
			`Zeile 3: ungültiger Abrechnungsmonat "Juli" (Format JJJJ-MM-01)`},
		{"Abrechnungsmonat leer", "monat", "",
			`Zeile 3: ungültiger Abrechnungsmonat "" (Format JJJJ-MM-01)`},
		{"Personenzahl", "personen_2", "1,255",
			`Zeile 3: ungültige Personenzahl für Wohnung 2: "1,255" (Personenzahl darf höchstens 2 Nachkommastellen haben)`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			cells := strings.Split(csvRow("2026-07-01", "100"), ";")
			cells[idx(c.col)] = c.wert
			csvText := strings.Join(csvHeader, ";") + "\n" +
				csvRow("2026-06-01", "90") + "\n" + strings.Join(cells, ";") + "\n"
			w := httptest.NewRecorder()
			handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), c.want)
			}
			if all, _ := store.AllPeriods(db); len(all) != 0 {
				t.Errorf("AllPeriods = %d, want 0", len(all))
			}
		})
	}
}
