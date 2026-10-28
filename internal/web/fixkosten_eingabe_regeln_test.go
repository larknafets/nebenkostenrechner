package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestFixkostenFormular_UngueltigeFelderMeldungen pins the German messages and
// the status code the form answers for fields the store's Validate rejects.
func TestFixkostenFormular_UngueltigeFelderMeldungen(t *testing.T) {
	db := openTestDB(t)
	first := strconv.FormatInt(store.KostenpositionDefaults[0].ID, 10)
	label := store.KostenpositionDefaults[0].Label

	for _, c := range []struct {
		name string
		set  func(v url.Values)
		want string
	}{
		{"Logik", func(v url.Values) { v.Set("logik_"+first, "x") }, "ungültige Berechnungslogik für " + label},
		{"Typ", func(v url.Values) { v.Set("typ_"+first, "x") }, "ungültiger Typ für " + label},
		{"Personenzahl", func(v url.Values) { v.Set("personen_1", "-1") }, "ungültige Personenzahl für "},
	} {
		t.Run(c.name, func(t *testing.T) {
			form := fixkostenForm("2026-09")
			c.set(form)
			req := httptest.NewRequest(http.MethodPost, "/fixkosten", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			handleCreateFixkosten()(w, requestWithDB(req, db))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			if !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), c.want)
			}
		})
	}
}

// TestHandleImportFixkostenCSV_UngueltigeFelderMeldungen pins status code, line
// number and German text for the fields the store's Validate rejects.
func TestHandleImportFixkostenCSV_UngueltigeFelderMeldungen(t *testing.T) {
	kd := store.KostenpositionDefaults[0]
	zeile := func(monat, logik, typ string) string {
		cells := []string{monat, logik, typ, "100"}
		for _, k := range store.KostenpositionDefaults[1:] {
			cells = append(cells, k.Logik, k.Typ, "100")
		}
		return strings.Join(append(cells, "2", "1", "150", "90", "", "800"), ";") + ";" + fixkostenCSVPreiseTail
	}
	for _, c := range []struct {
		name, row, want string
	}{
		{"Monat", zeile("2026-06", kd.Logik, kd.Typ), `Zeile 2: ungültiger Monat "2026-06" (Format JJJJ-MM-01)`},
		{"Logik", zeile("2026-06-01", "x", kd.Typ), `Zeile 2: ungültige Berechnungslogik für ` + kd.Label + `: "x"`},
		{"Typ", zeile("2026-06-01", kd.Logik, "y"), `Zeile 2: ungültiger Typ für ` + kd.Label + `: "y"`},
		{"Personenzahl keine Zahl", fixkostenCSVRowMitPersonen("2026-06-01", "abc", "1"), `Zeile 2: ungültige Personenzahl für Wohnung 1: "abc"`},
		{"Personenzahl zu viele Nachkommastellen", fixkostenCSVRowMitPersonen("2026-06-01", "1,255", "1"), `Zeile 2: ungültige Personenzahl für Wohnung 1: "1,255" (Personenzahl darf höchstens 2 Nachkommastellen haben)`},
		{"Personenzahl", fixkostenCSVRowMitPersonen("2026-06-01", "-1", "1"), `Zeile 2: ungültige Personenzahl für Wohnung 1: "-1" (Personenzahl darf nicht negativ sein)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + c.row + "\n"
			w := httptest.NewRecorder()
			handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
			}
			if !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("body = %q, want it to contain %q", w.Body.String(), c.want)
			}
		})
	}
}
