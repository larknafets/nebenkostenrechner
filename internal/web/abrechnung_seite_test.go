package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// getAbrechnung renders /abrechnung with the given query string and returns
// status and body. headers are added to the request (e.g. X-Ingress-Path).
func getAbrechnung(t *testing.T, mux *http.ServeMux, query string, headers map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/abrechnung"+query, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func mustNotContain(t *testing.T, body string, nots ...string) {
	t.Helper()
	for _, not := range nots {
		if strings.Contains(body, not) {
			t.Errorf("body contains %q, want it absent", not)
		}
	}
}

// demoMux is a mux over a demo-seeded database (months 2023-08 to 2026-10):
// 2023 is the Teiljahr, 2024 and 2025 are complete, 2026 misses Nov and Dec.
func demoMux(t *testing.T) *http.ServeMux {
	t.Helper()
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	return NewMux(db, openTestDB(t), "", "")
}

func TestAbrechnungJahreUndStandard(t *testing.T) {
	periods := []store.PeriodSummary{
		{ID: 3, ReadingDate: "2026-01-05", Monat: "2025-12-01"}, // Abrechnungsmonat wins over the reading date
		{ID: 2, ReadingDate: "2025-06-10", Monat: "2025-06-01"},
		{ID: 1, ReadingDate: "2024-12-30", Monat: ""}, // a Teilstand: the reading date counts
	}
	eingaben := []store.FixkostenEingabeSummary{{ID: 1, Monat: "2027-02-01"}}
	got := abrechnungJahre(periods, eingaben)
	if len(got) != 3 || got[0] != 2027 || got[1] != 2025 || got[2] != 2024 {
		t.Errorf("abrechnungJahre = %v, want [2027 2025 2024] newest first, each once", got)
	}

	apartments := []store.Apartment{{ID: 1, Status: store.StatusEigennutzung}, {ID: 2, Status: store.StatusVermietet}}
	if got := standardWohnung(apartments); got != 2 {
		t.Errorf("standardWohnung = %d, want the first rented one (2)", got)
	}
	apartments[1].Status = store.StatusEigennutzung
	if got := standardWohnung(apartments); got != 1 {
		t.Errorf("standardWohnung without a rented apartment = %d, want 1", got)
	}
}

func TestAbrechnungSeite_KeineDaten(t *testing.T) {
	mux := NewMux(openTestDB(t), openTestDB(t), "", "")
	code, body := getAbrechnung(t, mux, "", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	mustContain(t, body, "Noch keine Daten für eine Abrechnung", "</html>")
	mustNotContain(t, body, `name="jahr"`)
}

func TestAbrechnungSeite_Vorbelegung(t *testing.T) {
	mux := demoMux(t)
	code, body := getAbrechnung(t, mux, "", nil)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want %d", code, http.StatusOK)
	}
	// Newest complete year (2025, 2026 is incomplete) and the rented apartment.
	mustContain(t, body,
		"Nebenkostenabrechnung 2025",
		`<option value="2025" selected>`,
		`value="2" class="primary">Wohnung 2 (vermietet)`,
		`value="1" class="secondary">Wohnung 1 (Eigennutzung)`,
		"Zeitraum: 01.01.2025 bis 31.12.2025",
		"</html>")

	t.Run("gueltige Parameter gewinnen", func(t *testing.T) {
		_, body := getAbrechnung(t, mux, "?jahr=2024&wohnung=1", nil)
		mustContain(t, body, "Nebenkostenabrechnung 2024", `value="1" class="primary">Wohnung 1`)
	})
	t.Run("ungueltige Parameter fallen auf die Vorbelegung zurueck", func(t *testing.T) {
		for _, q := range []string{"?jahr=1999&wohnung=9", "?jahr=abc&wohnung=", "?jahr=2030"} {
			code, body := getAbrechnung(t, mux, q, nil)
			if code != http.StatusOK {
				t.Errorf("%s: status = %d, want %d", q, code, http.StatusOK)
			}
			mustContain(t, body, "Nebenkostenabrechnung 2025")
		}
	})
}

func TestAbrechnungSeite_Maengelliste(t *testing.T) {
	mux := demoMux(t)
	_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", map[string]string{"X-Ingress-Path": "/api/hassio_ingress/abc"})
	mustContain(t, body,
		"Abrechnung 2026 nicht möglich",
		"Ablesung fehlt: November 2026",
		"Fixkosten-Eingabe fehlt: November 2026",
		// Links to fix a finding carry the ingress base.
		`href="/api/hassio_ingress/abc/ablesungen/neu"`,
		`href="/api/hassio_ingress/abc/fixkosten/neu"`)
	mustNotContain(t, body, "Nebenkostenabrechnung 2026", "Drucken")
}

func TestAbrechnungSeite_Eigennutzung(t *testing.T) {
	mux := demoMux(t)
	_, body := getAbrechnung(t, mux, "?jahr=2024&wohnung=1", nil)
	mustContain(t, body, "Interne Übersicht, keine Betriebskostenabrechnung", "(Eigennutzung)", "Rundungsdifferenzen")
	// No tenant block, no legal notes, no screen notes for the own use.
	mustNotContain(t, body, "<strong>Mieter</strong>", "Belege können auf Verlangen", "Einwendungen", "Hinweise zur Abrechnung", "Bankverbindung")
}

func TestAbrechnungSeite_Hinweise(t *testing.T) {
	db := teiljahrDB(t, true)
	if _, err := db.Exec(`UPDATE haus SET iban = 'DE00 1234 5678'`); err != nil {
		t.Fatalf("set iban: %v", err)
	}
	mux := NewMux(db, openTestDB(t), "", "")

	t.Run("Nachzahlung: Frist, Aufbewahrung, Schluesselwechsel, IBAN", func(t *testing.T) {
		code, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		mustContain(t, body,
			"Nachzahlung: - ",
			"Frist: Die Abrechnung muss dem Mieter bis zum 31.12.2027 zugehen",
			"Aufbewahrung: Den verschickten Stand als PDF aufbewahren",
			"Wechsel des Verteilerschlüssels im Jahr bei: Wohngebäudeversicherung",
			"§ 556a Abs. 2 BGB",
			"Bankverbindung: Max Mustermann, IBAN DE00 1234 5678",
			"Belege können auf Verlangen eingesehen werden",
			"Fixkosten und Vorauszahlungen ab Oktober 2026",
			// A split position names its months.
			"(Okt 2026 bis Nov 2026)")
		// The notes and controls are screen-only.
		mustContain(t, body, `class="panel no-print"`, `class="no-print"`, "Zum Sichern als PDF: Drucken, Ziel PDF", `onclick="window.print()"`, "@media print { .no-print")
	})

	t.Run("Guthaben: keine Frist, keine IBAN", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE nebenkosten_abschlaege SET wert = 100000 WHERE apartment_id = 2`); err != nil {
			t.Fatalf("raise Abschlag: %v", err)
		}
		_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
		mustContain(t, body, "Guthaben: ", "Aufbewahrung: Den verschickten Stand")
		mustNotContain(t, body, "Frist: Die Abrechnung muss", "Bankverbindung", "Nachzahlung: - ")
	})
}

func TestAbrechnungSeite_Navigation(t *testing.T) {
	t.Run("angemeldet: Menuepunkt vorhanden", func(t *testing.T) {
		body := getDashboard(t, NewMux(openTestDB(t), openTestDB(t), "", ""), nil)
		mustContain(t, body, `href="/abrechnung">Abrechnung</a>`)
	})
	t.Run("nicht angemeldet: kein Menuepunkt", func(t *testing.T) {
		t.Setenv("LOGIN_PASSWORD", "geheim")
		body := getDashboard(t, NewMux(openTestDB(t), openTestDB(t), "", ""), nil)
		mustNotContain(t, body, `href="/abrechnung"`)
	})
}

// A visitor who is not logged in gets no Abrechnung (it carries names,
// addresses and the IBAN) - TestRouteMatrix_RequireLoginCoverage checks the
// redirect, this checks that no data leaks with it.
func TestAbrechnungSeite_NichtAngemeldetKeineDaten(t *testing.T) {
	t.Setenv("LOGIN_PASSWORD", "geheim")
	db := teiljahrDB(t, true)
	mux := NewMux(db, openTestDB(t), "", "")
	code, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
	if code != http.StatusFound {
		t.Fatalf("status = %d, want a redirect to the login", code)
	}
	mustNotContain(t, body, "Erika Beispiel", "Max Mustermann")
}
