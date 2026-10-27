package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// labelMiete is the heading of the Miete block on a Jahreskarte.
const labelMiete = `<div class="label">Miete</div>`

// Tests for #278: the Mieteinnahmen block on the Jahreskarte of a rented
// apartment, through the HTTP seam.

func mieteEingabe(t *testing.T, db *sql.DB, monat string, miete map[int64]float64) {
	t.Helper()
	in := seedFixkostenInput(monat)
	in.Miete = miete
	if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
		t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
	}
}

func dashboardHTMLMitAuth(t *testing.T, db *sql.DB, a *Zugang) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handleDashboard("v0", "", a)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func TestDashboard_MieteinnahmenSummeDesAnzeigejahres(t *testing.T) {
	db := openTestDB(t)
	// Wohnung 2 is rented. The previous year does not count.
	mieteEingabe(t, db, "2025-12-01", map[int64]float64{2: 999})
	mieteEingabe(t, db, "2026-01-01", map[int64]float64{2: 700.10})
	mieteEingabe(t, db, "2026-02-01", map[int64]float64{2: 700.20})

	body := dashboardHTML(t, db)
	if n := strings.Count(body, labelMiete); n != 1 {
		t.Fatalf("Block Mieteinnahmen %d mal, want 1 (nur die vermietete Wohnung 2)", n)
	}
	if !strings.Contains(body, "1.400,30") {
		t.Error("Mieteinnahmen 1.400,30 EUR fehlen")
	}
	if strings.Contains(body, "2.399,30") {
		t.Error("Vorjahr zählt mit")
	}
}

func TestDashboard_MieteinnahmenAuchMitNull(t *testing.T) {
	db := openTestDB(t)
	mieteEingabe(t, db, "2026-01-01", map[int64]float64{2: 0})
	body := dashboardHTML(t, db)
	i := strings.Index(body, labelMiete)
	if i < 0 || !strings.Contains(body[i:i+200], "0,00") {
		t.Error("Mieteinnahmen mit 0,00 fehlen")
	}
	// No Miete recorded at all: the block is still there.
	db = openTestDB(t)
	mieteEingabe(t, db, "2026-01-01", nil)
	if !strings.Contains(dashboardHTML(t, db), labelMiete) {
		t.Error("ohne erfasste Miete fehlt der Block")
	}
}

func TestDashboard_MieteinnahmenNieBeiEigennutzung(t *testing.T) {
	db := openTestDB(t)
	mieteEingabe(t, db, "2026-01-01", map[int64]float64{1: 300, 2: 700})
	setStatus(t, db, 2, store.StatusEigennutzung)
	body := dashboardHTML(t, db)
	if strings.Contains(body, labelMiete) || strings.Contains(body, "700,00") {
		t.Error("Mieteinnahmen bei Eigennutzung sichtbar")
	}
}

func TestDashboard_MieteinnahmenNurMitLogin(t *testing.T) {
	db := openTestDB(t)
	mieteEingabe(t, db, "2026-01-01", map[int64]float64{2: 712.34})

	anon := dashboardHTMLMitAuth(t, db, newZugang("geheim", nil))
	if !strings.Contains(anon, "<strong>Wohnung 2</strong>") {
		t.Fatal("Wohnung 2 fehlt für Anonyme, Test prüft nichts")
	}
	if strings.Contains(anon, labelMiete) || strings.Contains(anon, "712,34") {
		t.Error("Mieteinnahmen im HTML ohne Login")
	}
	// Without LOGIN_PASSWORD everyone is logged in.
	if !strings.Contains(dashboardHTMLMitAuth(t, db, newZugang("", nil)), "712,34") {
		t.Error("ohne Passwort fehlen die Mieteinnahmen")
	}
}
