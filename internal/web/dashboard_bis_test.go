package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func dashboardBody(t *testing.T, db *sql.DB, query string) string {
	t.Helper()
	w := httptest.NewRecorder()
	handleDashboard("v0", "", newZugang("", nil))(w, requestWithDB(httptest.NewRequest(http.MethodGet, "/"+query, nil), db))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /%s: status %d", query, w.Code)
	}
	return w.Body.String()
}

func demoDB(t *testing.T) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	return db
}

func TestDashboard_MieteBisMonat(t *testing.T) {
	db := openTestDB(t)
	mieteEingabe(t, db, "2026-01-01", map[int64]float64{2: 700.10})
	mieteEingabe(t, db, "2026-02-01", map[int64]float64{2: 700.20})

	if body := dashboardBody(t, db, ""); !strings.Contains(body, "1.400,30") {
		t.Error("ohne Parameter fehlt die Miete 1.400,30")
	}
	if body := dashboardBody(t, db, "?bis=2026-01-01"); !strings.Contains(body, "700,10") || strings.Contains(body, "1.400,30") {
		t.Error("bis Januar: Miete soll 700,10 sein")
	}
}

func TestDashboard_JahressummenBisMonat(t *testing.T) {
	db := demoDB(t)
	standard := dashboardBody(t, db, "")
	if strings.Contains(standard, "bis März</span>") || !strings.Contains(standard, `class="zeit"`) {
		t.Error("ohne Parameter: Zeitleiste fehlt oder Überschrift hat schon ein Bis")
	}

	bis := dashboardBody(t, db, "?bis=2026-03-01")
	for _, want := range []string{`<span class="badge">bis März</span>`, `href="?bis=2026-03-01" title="bis März 2026" aria-label="bis März 2026" aria-current="true"`, `href="?"`} {
		if !strings.Contains(bis, want) {
			t.Errorf("?bis=2026-03-01 ohne %q", want)
		}
	}
	if bis == standard {
		t.Error("?bis=2026-03-01 ändert die Seite nicht")
	}

	// A month that is not offered counts as no limit.
	for _, q := range []string{"?bis=quatsch", "?bis=2020-01-01"} {
		if got := dashboardBody(t, db, q); got != standard {
			t.Errorf("%s weicht von der Standardseite ab", q)
		}
	}
}

func TestDashboard_SaldoImVorjahr(t *testing.T) {
	dd, _, offen, err := loadDashboardData(demoDB(t), wunsch{}, 2026)
	if err != nil {
		t.Fatal(err)
	}
	neu, _ := buildEntityView(dd, offen, 2)
	dd, _, offen, err = loadDashboardData(demoDB(t), wunsch{jahr: 2025}, 2026)
	if err != nil {
		t.Fatal(err)
	}
	alt, _ := buildEntityView(dd, offen, 2)
	if neu.Saldo == nil || alt.Saldo == nil {
		t.Fatalf("Saldo fehlt: neu=%v alt=%v", neu.Saldo, alt.Saldo)
	}
	if neu.Saldo.Betrag() == alt.Saldo.Betrag() {
		t.Errorf("Saldo 2025 = Saldo 2026 = %v, want der Stand Ende 2025", alt.Saldo.Betrag())
	}
	// Same as an explicit Bis-Monat on the last month of 2025.
	dd.Bis = "2025-12-01"
	explizit, _ := buildEntityView(dd, offen, 2)
	if explizit.Saldo.Betrag() != alt.Saldo.Betrag() {
		t.Errorf("Saldo 2025 = %v, mit bis=Dezember %v", alt.Saldo.Betrag(), explizit.Saldo.Betrag())
	}
}

func TestBisMonat(t *testing.T) {
	monat := func(s string) string { return s }
	in := []string{"2026-01-01", "2026-02-01", "2026-03-01"}
	if got := bisMonat(in, "", monat); len(got) != 3 {
		t.Errorf("ohne Grenze = %v, want alle", got)
	}
	if got := bisMonat(in, "2026-02-01", monat); len(got) != 2 || got[1] != "2026-02-01" {
		t.Errorf("bis Februar = %v, want Januar und Februar", got)
	}
}

func TestDashboard_Jahreswahl(t *testing.T) {
	db := demoDB(t)
	standard := dashboardBody(t, db, "")
	if !strings.Contains(standard, `title="Vorjahr"`) || strings.Contains(standard, `title="Folgejahr"`) {
		t.Error("neuestes Jahr: Pfeil zurück erwartet, Pfeil vor nicht")
	}

	vorjahr := dashboardBody(t, db, "?jahr=2025")
	for _, want := range []string{"Jahressummen 2025", `title="Folgejahr"`, `href="?"`, `href="?jahr=2025&amp;bis=2025-03-01"`} {
		if !strings.Contains(vorjahr, want) {
			t.Errorf("?jahr=2025 ohne %q", want)
		}
	}

	// A year without data keeps the newest year.
	if got := dashboardBody(t, db, "?jahr=1999"); got != standard {
		t.Error("?jahr=1999 weicht von der Standardseite ab")
	}
}

// An Ablesung of 2 January for the Abrechnungsmonat December: the Anzeigejahr
// follows the Abrechnungsmonat (GLOSSARY.md), so it is the year of December,
// not the year of the Ablesedatum (#433).
func TestDashboard_AnzeigejahrFolgtDemAbrechnungsmonat(t *testing.T) {
	db := openTestDB(t)
	for i, p := range []struct{ datum, monat string }{{"2026-11-01", "2026-11-01"}, {"2027-01-02", "2026-12-01"}} {
		in := seedPeriodInputAt(p.datum)
		in.Monat = p.monat
		in.Readings["strom_gesamt"] = float64(100 * i)
		if _, err := createPeriod(db, in); err != nil {
			t.Fatalf("createPeriod %s: %v", p.monat, err)
		}
	}
	if dd := mustLoadDashboard(t, db); dd.Jahr != 2026 || dd.NeuesJahr != 2026 {
		t.Errorf("Jahr %d, NeuesJahr %d, want 2026", dd.Jahr, dd.NeuesJahr)
	}
	if body := dashboardBody(t, db, ""); !strings.Contains(body, "Jahressummen 2026") {
		t.Error("Dashboard ohne Jahressummen 2026")
	}
}
