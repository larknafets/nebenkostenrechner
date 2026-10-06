package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
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
	periods := []*store.LatestPeriod{
		{ID: 3, ReadingDate: "2026-01-05", Monat: "2025-12-01"}, // Abrechnungsmonat wins over the reading date
		{ID: 2, ReadingDate: "2025-06-10", Monat: "2025-06-01"},
		{ID: 1, ReadingDate: "2024-12-30", Monat: ""}, // a Teilstand: the reading date counts
	}
	eingaben := []store.FixkostenEingabeSummary{{ID: 1, Monat: "2027-02-01"}}
	got := abrechnungJahre(abrechnungPruefDaten{Periods: periods, Eingaben: eingaben})
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
		`value="2" class="primary">Wohnung 2 (Vermietung)`,
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
			`betrag nachzahlung">- `,
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

	t.Run("Kontoinhaber ersetzt den Vermieter bei der Bankverbindung", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE haus SET kontoinhaber = 'Max und Erika Mustermann'`); err != nil {
			t.Fatalf("set kontoinhaber: %v", err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`UPDATE haus SET kontoinhaber = ''`) })
		_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
		mustContain(t, body, "Bankverbindung: Max und Erika Mustermann, IBAN DE00 1234 5678")
	})

	t.Run("Guthaben: keine Frist, keine IBAN", func(t *testing.T) {
		if _, err := db.Exec(`UPDATE nebenkosten_abschlaege SET wert = 100000 WHERE apartment_id = 2`); err != nil {
			t.Fatalf("raise Abschlag: %v", err)
		}
		_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
		mustContain(t, body, `betrag guthaben">`, "Aufbewahrung: Den verschickten Stand")
		mustNotContain(t, body, "Frist: Die Abrechnung muss", "Bankverbindung", `betrag nachzahlung">`)
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

// TestAbrechnungSeite_Layout checks the structure of the A4 layout (Variante
// D of the prototype): head with result box, two columns with a sidebar, the
// Anhang on its own page, and the print/mobile rules.
func TestAbrechnungSeite_Layout(t *testing.T) {
	mux := demoMux(t)
	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)

	t.Run("Aufbau", func(t *testing.T) {
		mustContain(t, body,
			`class="abr-ergebnis"`, `class="abr-body"`, `class="abr-haupt"`, `<aside class="abr-seite">`,
			"<h3>Fixkosten</h3>", "<h3>Verbrauchsabhängig</h3>", "<h3>Saldoberechnung</h3>",
			`<div class="anhang">`, "<h3>Anlage 6: Verbrauchsübersicht</h3>", "<h3>Anlage 3: Bezugsgrößen der Verteilerschlüssel</h3>", "<h3>Anlage 4: Personenzahl je Monat</h3>")
		// The sidebar carries the calculation and the notes, the notes name the legal basis.
		mustContain(t, body, "Belege können auf Verlangen eingesehen werden", "Rundungsdifferenzen")
		// The month table of the heating has no weighting and no Personen column.
		mustNotContain(t, body, "Gewichtung</th>")
	})

	t.Run("Druck: immer DIN A4, hell, ohne Bedienelemente", func(t *testing.T) {
		mustContain(t, body,
			"@page { size: A4; margin: 14mm 15mm; }",
			"break-before: page",
			".anlage { break-inside: avoid; }", "thead { display: table-header-group; }",
			"--bg: #fff !important", "--text: #000 !important",
			`class="muted no-print"`,          // the navigation
			`class="theme-toggle no-print"`,   // the theme switch
			`<h1 class="no-print">Abrechnung`) // the page title above the document
	})

	t.Run("Mobil nur am Bildschirm", func(t *testing.T) {
		// The printable width of an A4 page is below 760px too: a plain
		// max-width rule would print the card layout (8 pages instead of 3).
		mustContain(t, body, "@media screen and (max-width: 760px)", `data-l="Betrag"`)
		mustNotContain(t, body, "@media (max-width: 760px)")
	})
}

// TestAbrechnungSeite_GewaehlteWohnungFett checks that the values of the
// settled apartment are bold in the Anhang, for both apartments.
func TestAbrechnungSeite_GewaehlteWohnungFett(t *testing.T) {
	mux := demoMux(t)

	_, w2 := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
	mustContain(t, w2,
		`class="r b" data-l="Wärme Wohnung 2 (MWh)"`, `class="r" data-l="Wärme Wohnung 1 (MWh)"`,
		`class="r b" data-l="Wohnung 2"`, `class="r" data-l="Wohnung 1"`,
		`class="r b" data-l="Fixkosten Wohnung 2"`, `class="r" data-l="Fixkosten Wohnung 1"`,
		`<th class="r sel">Anteil Wohnung 2</th>`)
	// Its own meters: Strom, Wasser and Wärme of Wohnung 2, nothing of Wohnung 1.
	if got := strings.Count(w2, `<tr class="b">`); got != 3 {
		t.Errorf("bold meter rows for Wohnung 2 = %d, want 3 (Strom, Wasser, Wärme)", got)
	}

	_, w1 := getAbrechnung(t, mux, "?jahr=2025&wohnung=1", nil)
	mustContain(t, w1,
		`class="r b" data-l="Wärme Wohnung 1 (MWh)"`, `class="r" data-l="Wärme Wohnung 2 (MWh)"`,
		`class="r b" data-l="Wohnung 1"`, `<th class="r sel">Anteil Wohnung 1</th>`)
	if got := strings.Count(w1, `<tr class="b">`); got != 1 {
		t.Errorf("bold meter rows for Wohnung 1 = %d, want 1 (only its heat meter)", got)
	}
}

func TestBerechneAbrechnung_VerbrauchEigenUndBezugsgroessen(t *testing.T) {
	db := teiljahrDB(t, true)
	for _, tc := range []struct {
		apartmentID int64
		wantEigen   []string
	}{
		{2, []string{"Zwischenstromzähler Wohnung 2", "Zwischenwasserzähler Wohnung 2", "Wärmemengenzähler Wohnung 2"}},
		{1, []string{"Wärmemengenzähler Wohnung 1"}},
	} {
		erg, err := berechneAbrechnungDB(db, 2026, tc.apartmentID)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("Wohnung %d: err %v, Maengel %v", tc.apartmentID, err, maengelTexte(erg.Pruefung))
		}
		var eigen []string
		for _, v := range erg.Abrechnung.Verbrauch {
			if v.Eigen {
				eigen = append(eigen, v.Zaehler)
			}
		}
		if fmt.Sprint(eigen) != fmt.Sprint(tc.wantEigen) {
			t.Errorf("Wohnung %d: own meters = %v, want %v", tc.apartmentID, eigen, tc.wantEigen)
		}
	}

	erg, _ := berechneAbrechnungDB(db, 2026, 2)
	b := erg.Abrechnung.Bezugsgroessen
	if len(b) != 2 || b[0].Schluessel != "Wohnfläche (m²)" || b[0].W1 != 100 || b[0].W2 != 50 || b[0].Gesamt != 150 {
		t.Errorf("Wohnfläche = %+v, want 100 / 50 / 150", b)
	}
	if b[1].Schluessel != "Flurstück (m²)" || b[1].W1 != 600 || b[1].W2 != 400 || b[1].Gesamt != 1000 {
		t.Errorf("Flurstück = %+v, want 600 / 400 / 1000", b[1])
	}
	for _, v := range erg.Abrechnung.Verbrauch {
		if strings.Contains(v.Einheit, "m3") {
			t.Errorf("Einheit %q of %s, want m³", v.Einheit, v.Zaehler)
		}
	}
}

func TestAbrechnungSeite_Teilzeitraum(t *testing.T) {
	db := teiljahrDB(t, true)
	mux := NewMux(db, openTestDB(t), "", "")

	t.Run("Standard: Jahr, Felder verborgen", func(t *testing.T) {
		_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2", nil)
		mustContain(t, body, "Nebenkostenabrechnung 2026", `id="zeitraum-felder" hidden`, ".zeitraum-felder[hidden] { display: none; }")
		mustNotContain(t, body, "Mieterwechsel: Ein Monat gehört")
	})
	t.Run("Haken: Zeitraum, Titel, Frist, Hinweis", func(t *testing.T) {
		_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2&teil=1&von=10&bis=11", nil)
		mustContain(t, body,
			"Nebenkostenabrechnung Oktober 2026 bis November 2026",
			"Zeitraum: 01.10.2026 bis 30.11.2026",
			"Frist: Die Abrechnung muss dem Mieter bis zum 30.11.2027 zugehen",
			"Mieterwechsel: Ein Monat gehört ganz zu dem Zeitraum",
			`name="teil" value="1" checked`,
			`<option value="10" selected>Oktober`, `<option value="11" selected>November`)
		mustNotContain(t, body, `id="zeitraum-felder" hidden`)
	})
	t.Run("ungueltiger Bereich oder ohne Haken: ganzes Jahr", func(t *testing.T) {
		for _, q := range []string{"&teil=1&von=11&bis=10", "&teil=1&von=0&bis=13", "&teil=1&von=x", "&von=10&bis=11"} {
			_, body := getAbrechnung(t, mux, "?jahr=2026&wohnung=2"+q, nil)
			mustContain(t, body, "Nebenkostenabrechnung 2026")
		}
	})
}

// TestAbrechnungSeite_Anlagen checks the numbered Anlagen of the Anhang: the
// list on page 1 and the headings carry the same numbers and titles in the
// same order, and each Anlage is its own block that does not break.
func TestAbrechnungSeite_Anlagen(t *testing.T) {
	mux := demoMux(t)
	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)

	var want []string
	for _, a := range abrechnungAnlagen {
		want = append(want, fmt.Sprintf("Anlage %d: %s", a.Nr, a.Titel))
	}

	headings := regexp.MustCompile(`<h3>(Anlage \d+: [^<]*)</h3>`).FindAllStringSubmatch(body, -1)
	var got []string
	for _, m := range headings {
		got = append(got, m[1])
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("headings = %v, want %v", got, want)
	}

	liste := regexp.MustCompile(`<li value="(\d+)">([^<]*)</li>`).FindAllStringSubmatch(body, -1)
	var items []string
	for _, m := range liste {
		items = append(items, "Anlage "+m[1]+": "+m[2])
	}
	if !reflect.DeepEqual(items, want) {
		t.Errorf("Anlagenliste = %v, want %v", items, want)
	}

	// The list sits in the sidebar of page 1, before the Anhang.
	if strings.Index(body, `class="klein anlagen-liste"`) > strings.Index(body, `<div class="anhang">`) {
		t.Error("Anlagenliste steht nicht vor dem Anhang")
	}
	if got := strings.Count(body, `<section class="anlage"`); got != len(want) {
		t.Errorf("Anlage blocks = %d, want %d", got, len(want))
	}
	// The Verbrauchsübersicht follows the Verteilerschlüssel as Anlage 6.
	if strings.Index(body, "Anlage 5: Verteilerschlüssel</h3>") > strings.Index(body, "Anlage 6: Verbrauchsübersicht</h3>") {
		t.Error("Anlage 6 steht vor Anlage 5")
	}
}

// TestAbrechnungSeite_Monatsverlauf checks that the Monatsverlauf (Anlage 1)
// is rendered with its rows, the sum row and the Jahressaldo.
func TestAbrechnungSeite_Monatsverlauf(t *testing.T) {
	mux := demoMux(t)
	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)

	mustContain(t, body,
		"<h3>Anlage 1: Monatsverlauf und Saldo</h3>",
		`data-l="Saldo kumuliert"`, "davon Jahressaldo", "Januar 2025", "Dezember 2025",
		"weiterberechnete Strom Wohnung 2")
	// Anlage 1 comes first in the Anhang.
	if strings.Index(body, "Anlage 1: Monatsverlauf") > strings.Index(body, "Anlage 2: Heizung") {
		t.Error("Anlage 1 steht nicht vor Anlage 2")
	}
	// The last cumulative balance is the Jahressaldo of the Saldoberechnung
	// (no carry-over yet), so the page shows it twice in the table.
	if got := strings.Count(body, "- 308,65"); got < 2 {
		t.Errorf("Jahressaldo - 308,65 € erscheint %d Mal, want at least 2 (Saldoberechnung and Monatsverlauf)", got)
	}
}
