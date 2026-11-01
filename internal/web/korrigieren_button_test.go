package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// The correct button sits in the row of the "Andere ... anzeigen" select,
// above the panel, and only the Vermieter sees it on a complete Ablesung.
func TestAblesungDetail_KorrigierenButtonOben(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01"))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	get := func(z *Zugang) string {
		req := httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10), nil)
		req.SetPathValue("id", strconv.FormatInt(id, 10))
		w := httptest.NewRecorder()
		handleAblesungDetail(z)(w, requestWithDB(req, db))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d", w.Code)
		}
		return w.Body.String()
	}

	body := get(newZugang("", nil))
	knopf, panel := strings.Index(body, "Eingabe korrigieren"), strings.Index(body, `<div class="panel">`)
	if knopf < 0 || knopf > panel {
		t.Errorf("Vermieter: Eingabe korrigieren at %d, want before the panel at %d", knopf, panel)
	}
	if strings.Contains(body, "Ablesung korrigieren") {
		t.Error("alte Beschriftung Ablesung korrigieren noch da")
	}
	if strings.Contains(get(newZugang("geheim", nil)), "Eingabe korrigieren") {
		t.Error("ohne Login steht Eingabe korrigieren bei einer vollständigen Ablesung")
	}
}

func TestFixkostenDetail_KorrigierenObenLoeschenUnten(t *testing.T) {
	db := openTestDB(t)
	id := mieteDetailEingabe(t, db, "2026-01-01", map[int64]float64{2: 712.34})
	body := fixkostenDetailHTML(t, db, newZugang("", nil), id)
	knopf, panel := strings.Index(body, "Eingabe korrigieren"), strings.Index(body, `<div class="panel">`)
	if knopf < 0 || knopf > panel {
		t.Errorf("Eingabe korrigieren at %d, want before the panel at %d", knopf, panel)
	}
	if !strings.Contains(body, "Fixkosteneingabe löschen") {
		t.Error("Fixkosteneingabe löschen fehlt")
	}
}
