package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// abrechnung_wallbox_test.go covers the Wallbox block and the Einzelzähler in
// the Anlagen of the Jahresabrechnung page (#228). The Wohnungsstatus table
// itself is tested at abrechnung.Berechne.

// demoDBMux is a mux over a demo-seeded database, returned with the database
// so a test can change Stammdaten. The demo has Einzelzähler readings in
// 2025 (Wohnung 1 Eigennutzung, Wohnung 2 vermietet), none in 2023.
func demoDBMux(t *testing.T) (*http.ServeMux, func(mutate func(*store.StammdatenSave))) {
	t.Helper()
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	return NewMux(db, openTestDB(t), "", ""), func(mutate func(*store.StammdatenSave)) { saveStammdaten(t, db, mutate) }
}

func TestAbrechnungSeite_WallboxBlockJeFlag(t *testing.T) {
	mux, speichere := demoDBMux(t)

	t.Run("vermietet mit Flag: Block und Saldozeile", func(t *testing.T) {
		_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
		mustContain(t, body, "<h3>Wallbox Wohnung 2 ", "(laut Vereinbarung weiterberechnet)", `data-l="Wallbox">Wallbox</td>`)
	})

	t.Run("Eigennutzung zeigt die Wallbox, das Flag gilt nicht", func(t *testing.T) {
		speichere(func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = false })
		_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=1", nil)
		mustContain(t, body, "<h3>Wallbox Wohnung 1</h3>", `data-l="Wallbox">Wallbox</td>`)
	})

	t.Run("vermietet ohne Flag: kein Block, keine Saldozeile", func(t *testing.T) {
		speichere(func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = false })
		code, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
		if code != http.StatusOK {
			t.Fatalf("status = %d", code)
		}
		mustContain(t, body, "</html>")
		mustNotContain(t, body, "<h3>Wallbox Wohnung", `data-l="Wallbox">Wallbox</td>`)
	})

	t.Run("ohne zugeordnete Wallbox-Kosten kein Block", func(t *testing.T) {
		speichere(func(s *store.StammdatenSave) { s.Flags.WallboxWeiterberechnen = true })
		_, body := getAbrechnung(t, mux, "?jahr=2024&wohnung=2", nil)
		mustNotContain(t, body, "<h3>Wallbox Wohnung", `data-l="Wallbox">Wallbox</td>`)
	})
}

func TestAbrechnungSeite_EinzelzaehlerInDenAnlagen(t *testing.T) {
	mux, _ := demoDBMux(t)

	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=2", nil)
	mustContain(t, body, "Wallbox Whg 1", "Wallbox Whg 2", "Wallbox Wohnung 1", "Wallbox Wohnung 2")
	// "Wallboxen" comes first, the Einzelzähler directly after it.
	i, j := strings.Index(body, "Wallboxen<br>"), strings.Index(body, "Wallbox Whg 1<br>")
	if i < 0 || j < i {
		t.Errorf("Anlage 7: Wallboxen at %d, Wallbox Whg 1 at %d, want the Einzelzähler after the Zwischenzähler", i, j)
	}

	// 2024 has no Einzelzähler reading at all: the columns and rows are gone.
	_, body = getAbrechnung(t, mux, "?jahr=2024&wohnung=2", nil)
	mustNotContain(t, body, "Wallbox Whg", "Wallbox Wohnung 1", "Wallbox Wohnung 2")
}

func TestStammdaten_WallboxWeiterberechnenSpeichernUndAnzeigen(t *testing.T) {
	db := openTestDB(t)

	get := func() string {
		req := httptest.NewRequest(http.MethodGet, "/stammdaten", nil)
		w := httptest.NewRecorder()
		handleStammdatenForm(newAuth("", nil))(w, requestWithDB(req, db))
		return w.Body.String()
	}
	post := func(withFlag bool) {
		form := url.Values{
			"qm_1": {"100"}, "flurstueck_groesse_1": {"600"}, "qm_2": {"50"}, "flurstueck_groesse_2": {"400"},
			"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.7"},
		}
		if withFlag {
			form.Set("wallbox_weiterberechnen", "1")
		}
		req := httptest.NewRequest(http.MethodPost, "/stammdaten", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleUpdateStammdaten()(w, requestWithDB(req, db))
		if w.Code != http.StatusFound {
			t.Fatalf("POST status = %d (body: %s)", w.Code, w.Body.String())
		}
	}

	// New database: the flag is on by default and the label says what it means.
	body := get()
	mustContain(t, body, "Wallbox weiterberechnen", "(Haushaltsstrom)")
	if !strings.Contains(body, `name="wallbox_weiterberechnen" value="1" checked`) {
		t.Error("the flag is not checked on a new database")
	}

	post(false)
	if h, _ := store.GetHaus(db); h.WallboxWeiterberechnen {
		t.Error("WallboxWeiterberechnen = true after saving without the checkbox")
	}
	mustNotContain(t, get(), `name="wallbox_weiterberechnen" value="1" checked`)

	post(true)
	if h, _ := store.GetHaus(db); !h.WallboxWeiterberechnen {
		t.Error("WallboxWeiterberechnen = false after saving with the checkbox")
	}
}
