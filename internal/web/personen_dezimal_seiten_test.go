package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// personen_dezimal_seiten_test.go covers the rendered pages for a decimal
// occupant count (spec #240): Kostenanteil, Dashboard, Anhang, prefill and
// the whole-number/0 inputs of both forms.

func postFixkosten(t *testing.T, db *sql.DB, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/fixkosten", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	handleCreateFixkosten()(w, requestWithDB(req, db))
	return w
}

// TestFixkosten_PersonenDezimal_Kostenanteil: Schlüssel Personen with 1,25
// to 2 splits a monthly 100 EUR into 38,46 and 61,54 EUR (1,25 / 3,25).
func TestFixkosten_PersonenDezimal_Kostenanteil(t *testing.T) {
	db := openTestDB(t)
	form := fixkostenForm("2026-09")
	form.Set("personen_1", "1.25")
	form.Set("personen_2", "2")
	for _, kd := range store.KostenpositionDefaults {
		id := strconv.FormatInt(kd.ID, 10)
		form.Set("logik_"+id, store.LogikPersonen)
		form.Set("wert_"+id, "100")
	}
	if w := postFixkosten(t, db, form); w.Code != http.StatusFound {
		t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
	}
	all, err := store.AllFixkostenEingabenDetails(db)
	if err != nil || len(all) != 1 {
		t.Fatalf("AllFixkostenEingabenDetails = %v, %v", all, err)
	}

	id := strconv.FormatInt(all[0].ID, 10)
	req := httptest.NewRequest(http.MethodGet, "/fixkosten/"+id, nil)
	req.SetPathValue("id", id)
	w := httptest.NewRecorder()
	handleFixkostenDetail(newAuth("", nil))(w, requestWithDB(req, db))
	mustContain(t, w.Body.String(), "38,46&nbsp;€", "61,54&nbsp;€")
}

func TestDashboard_PersonenSchnittDezimal(t *testing.T) {
	db := openTestDB(t)
	jahr := strconv.Itoa(time.Now().Year())
	// The Personen-Schnitt is the mean over the year's Fixkosten-Eingaben.
	form := fixkostenForm(jahr + "-01")
	form.Set("personen_1", "1.25")
	form.Set("personen_2", "2")
	if w := postFixkosten(t, db, form); w.Code != http.StatusFound {
		t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
	}

	badge := func(body string) []string {
		var out []string
		for _, m := range regexp.MustCompile(`title="Personen-Schnitt">.*?</svg>([^<]*)</span>`).FindAllStringSubmatch(body, -1) {
			out = append(out, m[1])
		}
		return out
	}
	if got := badge(dashboardHTML(t, db)); !reflect.DeepEqual(got, []string{"1,25", "2"}) {
		t.Errorf("Dashboard-Badges = %q, want [1,25 2]", got)
	}
	if got := badge(widgetHTML(t, db, "jahressumme", "wohnung-1")); !reflect.DeepEqual(got, []string{"1,25"}) {
		t.Errorf("Widget-Badges = %q, want [1,25]", got)
	}
}

func TestAbrechnungSeite_Anlage4_PersonenDezimal(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	for _, q := range []string{
		`UPDATE period_occupancy SET personen = 1.25 WHERE apartment_id = 1`,
		`UPDATE fixkosten_personen SET personen = 1.25 WHERE apartment_id = 1`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mux := NewMux(db, openTestDB(t), "", "")
	_, body := getAbrechnung(t, mux, "?jahr=2025&wohnung=1", nil)

	i := strings.Index(body, `id="anlage-personen"`)
	if i < 0 {
		t.Fatal("keine Anlage Personenzahl")
	}
	section := body[i : i+strings.Index(body[i:], "</section>")]
	mustContain(t, section,
		`data-l="Fixkosten Wohnung 1">1,25</td>`,
		`data-l="Ablesung Wohnung 1">1,25</td>`)
}

func TestPersonenDezimal_VorbelegungAusVorperiode(t *testing.T) {
	t.Run("Wizard", func(t *testing.T) {
		db := openTestDB(t)
		in := seedPeriodInputAt("2026-10-01")
		in.Personen = map[int64]float64{1: 1.25, 2: 2}
		if _, err := store.CreatePeriod(db, in); err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		body := wizardBody(t, db)
		mustContain(t, inputTag(t, body, "personen_1"), `value="1.25"`)
		mustContain(t, inputTag(t, body, "personen_2"), `value="2"`)
	})

	t.Run("Fixkosten", func(t *testing.T) {
		db := openTestDB(t)
		form := fixkostenForm("2026-09")
		form.Set("personen_1", "1.25")
		if w := postFixkosten(t, db, form); w.Code != http.StatusFound {
			t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
		}
		req := httptest.NewRequest(http.MethodGet, "/fixkosten/neu", nil)
		w := httptest.NewRecorder()
		handleFixkostenForm(newAuth("", nil))(w, requestWithDB(req, db))
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d (body: %s)", w.Code, w.Body.String())
		}
		mustContain(t, inputTag(t, w.Body.String(), "personen_1"), `value="1.25"`)
	})
}

func TestPersonenDezimal_GanzzahlUndNullInBeidenFormularen(t *testing.T) {
	for _, raw := range []string{"2", "0"} {
		t.Run("Ablesung "+raw, func(t *testing.T) {
			db := openTestDB(t)
			form := vollesFormular()
			form.Set("personen_1", raw)
			w := httptest.NewRecorder()
			handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
			if w.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
			}
			latest, err := store.GetLatestPeriod(db)
			if err != nil || latest == nil {
				t.Fatalf("GetLatestPeriod = %v, %v", latest, err)
			}
			want, _ := strconv.ParseFloat(raw, 64)
			if got := latest.PersonenByApartment[1]; got != want {
				t.Errorf("Personen[1] = %v, want %v", got, want)
			}
		})
		t.Run("Fixkosten "+raw, func(t *testing.T) {
			db := openTestDB(t)
			form := fixkostenForm("2026-09")
			form.Set("personen_1", raw)
			if w := postFixkosten(t, db, form); w.Code != http.StatusFound {
				t.Fatalf("status = %d, want 302 (body: %s)", w.Code, w.Body.String())
			}
			all, err := store.AllFixkostenEingabenDetails(db)
			if err != nil || len(all) != 1 {
				t.Fatalf("AllFixkostenEingabenDetails = %v, %v", all, err)
			}
			want, _ := strconv.ParseFloat(raw, 64)
			if got := all[0].Personen[1]; got != want {
				t.Errorf("Personen[1] = %v, want %v", got, want)
			}
		})
	}
}
