package web

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// fehler_overlay_test.go covers the server side of the Fehler-Overlay: the
// layout carries the dialog, the five input forms opt in, and a 400 names
// the offending field. The fetch script itself runs in the browser.

func getPage(t *testing.T, h http.HandlerFunc, path string) string {
	t.Helper()
	db := openTestDB(t)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	h(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s: status %d", path, w.Code)
	}
	return w.Body.String()
}

var formMitMarkierung = regexp.MustCompile(`<form[^>]*data-fehler-overlay`)

func TestFehlerOverlay_FormulareMarkiertUndDialogImLayout(t *testing.T) {
	seiten := []struct {
		pfad   string
		h      http.HandlerFunc
		anzahl int
	}{
		{"/fixkosten/neu", handleFixkostenForm(newZugang("", nil)), 2}, // Eingabe + CSV-Import
		{"/ablesungen/neu", handleWizardForm(newZugang("", nil)), 2},   // Wizard + CSV-Import
		{"/stammdaten", handleStammdatenForm(newZugang("", nil)), 1},
	}
	for _, s := range seiten {
		body := getPage(t, s.h, s.pfad)
		if got := len(formMitMarkierung.FindAllString(body, -1)); got != s.anzahl {
			t.Errorf("%s: %d Formulare mit data-fehler-overlay, want %d", s.pfad, got, s.anzahl)
		}
		if !strings.Contains(body, `id="fehler-overlay"`) {
			t.Errorf("%s: Dialog fehlt im Layout", s.pfad)
		}
	}
}

func TestFehlerOverlay_BelegterMonatNenntFeld(t *testing.T) {
	db := openTestDB(t)
	if w := postFixkosten(t, db, fixkostenForm("2026-09")); w.Code != http.StatusFound {
		t.Fatalf("erste Eingabe: status %d", w.Code)
	}
	w := postFixkosten(t, db, fixkostenForm("2026-09"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("zweite Eingabe: status %d, want 400", w.Code)
	}
	if got := w.Header().Get(fehlerFeldHeader); got != "monat" {
		t.Errorf("%s = %q, want monat", fehlerFeldHeader, got)
	}
	if !strings.Contains(w.Body.String(), "September 2026") {
		t.Errorf("Meldung nennt den Monat nicht: %q", w.Body.String())
	}
}
