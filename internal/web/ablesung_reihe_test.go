package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestHandleCreateAblesung_Reihenfolge(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-08-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	post := func(readingDate, monat string) *httptest.ResponseRecorder {
		form := periodFormValues(readingDate, monat)
		req := httptest.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleCreateAblesung()(w, requestWithDB(req, db))
		return w
	}

	t.Run("Ablesedatum vor der neuesten Ablesung", func(t *testing.T) {
		w := post("2026-07-01", "2026-07")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
		if want := "Ablesedatum muss nach der Vorperiode (2026-08-01) liegen"; !strings.Contains(w.Body.String(), want) {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), want)
		}
		if got := w.Header().Get(fehlerFeldHeader); got != "reading_date" {
			t.Errorf("Fehler-Feld = %q, want reading_date", got)
		}
	})

	t.Run("Abrechnungsmonat vor dem der neuesten Ablesung", func(t *testing.T) {
		w := post("2026-09-01", "2026-07")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
		}
		if want := "Abrechnungsmonat darf nicht vor dem der Vorperiode (2026-08-01) liegen"; !strings.Contains(w.Body.String(), want) {
			t.Errorf("body = %q, want it to contain %q", w.Body.String(), want)
		}
		if got := w.Header().Get(fehlerFeldHeader); got != "monat" {
			t.Errorf("Fehler-Feld = %q, want monat", got)
		}
	})

	t.Run("nichts gespeichert", func(t *testing.T) {
		all, err := store.AllPeriods(db)
		if err != nil {
			t.Fatalf("AllPeriods: %v", err)
		}
		if len(all) != 1 {
			t.Errorf("AllPeriods = %d, want 1 (rejected POSTs leave no row)", len(all))
		}
	})

	t.Run("gültige Ablesung wird angelegt", func(t *testing.T) {
		if w := post("2026-09-01", "2026-09"); w.Code != http.StatusFound {
			t.Errorf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
		}
	})
}

// TestHandleCreateAblesung_OffenerTeilstand verifies
// the store, not a handler guard, blocks a second open reading: a direct POST
// leads to completing the open Teilstand.
func TestHandleCreateAblesung_OffenerTeilstand(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-08-01")); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	teilstand, err := store.CreatePeriod(db, store.PeriodInput{ReadingDate: "2026-09-01"})
	if err != nil {
		t.Fatalf("CreatePeriod (Teilstand): %v", err)
	}

	form := periodFormValues("2026-10-01", "2026-10")
	req := httptest.NewRequest(http.MethodPost, "/ablesungen", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(req, db))

	if w.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
	}
	if want := fmt.Sprintf("/ablesungen/%d/bearbeiten", teilstand); !strings.HasSuffix(w.Header().Get("Location"), want) {
		t.Errorf("Location = %q, want suffix %q", w.Header().Get("Location"), want)
	}
	all, _ := store.AllPeriods(db)
	if len(all) != 2 {
		t.Errorf("AllPeriods = %d, want 2 (no second open reading)", len(all))
	}
}

// TestHandleImportCSV_Reihenfolge verifies the import applies the
// Ablesungsreihe rules and answers 400 naming the offending reading date,
// instead of a 500 or a half-imported series.
func TestHandleImportCSV_Reihenfolge(t *testing.T) {
	db := openTestDB(t)
	csvText := strings.Join(csvHeader, ";") + "\n" +
		csvRow("2026-06-01", "100") + "\n" +
		csvRow("2026-06-01", "200") + "\n"

	w := httptest.NewRecorder()
	handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
	if want := "Ablesung vom 2026-06-01: Ablesedatum muss nach der Vorperiode (2026-06-01) liegen"; !strings.Contains(w.Body.String(), want) {
		t.Errorf("body = %q, want it to contain %q", w.Body.String(), want)
	}
	all, err := store.AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(all) != 0 {
		t.Errorf("AllPeriods = %d, want 0 (all-or-nothing)", len(all))
	}
}

// TestAblesungDetail_VorperiodeAusDemDatenstand verifies the detail page takes
// the Vorperiode from the one loaded Datenstand: the consumption is the
// difference to the chronologically previous Ablesung, the oldest one has
// none, and an unknown id is a 404.
func TestAblesungDetail_VorperiodeAusDemDatenstand(t *testing.T) {
	db := openTestDB(t)
	var ids []int64
	for i, datum := range []string{"2026-07-01", "2026-08-01", "2026-09-01"} {
		in := seedPeriodInputAt(datum)
		in.Readings["strom_gesamt"] = float64(100 + i*250)
		id, err := store.CreatePeriod(db, in)
		if err != nil {
			t.Fatalf("CreatePeriod %s: %v", datum, err)
		}
		ids = append(ids, id)
	}

	zeile := func(id int64) string { return tabellenzeile(t, detailBody(t, db, id), "Strom Netzbezug (1.8.0)") }
	if got := zeile(ids[1]); !strings.Contains(got, "&#43;250,00") {
		t.Errorf("mittlere Ablesung: %q, want Differenz +250,00 zur Vorperiode", got)
	}
	if got := zeile(ids[0]); strings.Contains(got, "&#43;") {
		t.Errorf("älteste Ablesung: %q, want keine Differenz", got)
	}
	for _, id := range ids {
		if body := detailBody(t, db, id); !strings.Contains(body, "2026-07-01") && !strings.Contains(body, "01.07.2026") {
			t.Errorf("Auswahlliste von Ablesung %d zeigt nicht alle Ablesungen", id)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/ablesungen/999", nil)
	req.SetPathValue("id", "999")
	w := httptest.NewRecorder()
	handleAblesungDetail(newAuth("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusNotFound {
		t.Errorf("unbekannte Id: status = %d, want 404", w.Code)
	}
}
