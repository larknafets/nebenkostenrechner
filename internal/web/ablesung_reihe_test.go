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
