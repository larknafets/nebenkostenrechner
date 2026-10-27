package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestAblesungStoreFehler: an *EingabeFehler the store returns to a caller
// that skipped parsePeriodInput gets the form text, and badRequestParse
// answers it with 400 and the field.
func TestAblesungStoreFehler(t *testing.T) {
	apartments := []store.Apartment{{ID: 1, Name: "EG"}}
	_, err := store.CreatePeriod(openTestDB(t), store.PeriodInput{ReadingDate: "kaputt"})
	mapped := ablesungStoreFehler(err, apartments)
	if mapped == nil {
		t.Fatalf("ablesungStoreFehler(%v) = nil, want Formtext", err)
	}
	w := httptest.NewRecorder()
	badRequestParse(w, mapped)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	if want := `ungültiges Ablesedatum "kaputt" (Format JJJJ-MM-TT)`; mapped.Error() != want {
		t.Errorf("msg = %q, want %q", mapped.Error(), want)
	}
	if got := ablesungStoreFehler(errors.New("db kaputt"), apartments); got != nil {
		t.Errorf("fremder Fehler: got %v, want nil", got)
	}
}

func TestFixkostenStoreFehler(t *testing.T) {
	db := openTestDB(t)
	apartments, err := store.Apartments(db)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateFixkostenEingabe(db, store.FixkostenInput{Monat: "kaputt", Personen: map[int64]float64{1: 1}})
	mapped := fixkostenStoreFehler(db, err, apartments)
	if mapped == nil {
		t.Fatalf("fixkostenStoreFehler(%v) = nil, want Formtext", err)
	}
	if want := `ungültiger Monat "kaputt"`; mapped.Error() != want {
		t.Errorf("msg = %q, want %q", mapped.Error(), want)
	}
	if got := fixkostenStoreFehler(db, errors.New("db kaputt"), apartments); got != nil {
		t.Errorf("fremder Fehler: got %v, want nil", got)
	}
}
