package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestAblesung_PersonenDezimal_Formular(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}
	cases := []struct {
		raw     string
		want    float64
		wantErr bool
	}{
		{"1.25", 1.25, false},
		{"2", 2, false},
		{"0", 0, false},
		{"1.255", 0, true},
	}
	for _, c := range cases {
		t.Run(c.raw, func(t *testing.T) {
			form := vollesFormular()
			form.Set("personen_1", c.raw)
			in, err := parsePeriodInput(formRequest(t, form), apartments)
			if c.wantErr {
				if err == nil {
					t.Fatalf("personen_1=%q: want Fehler, got nil", c.raw)
				}
				return
			}
			if err != nil {
				t.Fatalf("personen_1=%q: %v", c.raw, err)
			}
			if in.Personen[1] != c.want {
				t.Errorf("Personen[1] = %v, want %v", in.Personen[1], c.want)
			}
		})
	}
}

func TestHandleCreateAblesung_PersonenZuvielNachkommastellen_400(t *testing.T) {
	db := openTestDB(t)
	form := vollesFormular()
	form.Set("personen_1", "1.255")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, form), db))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", w.Code, w.Body.String())
	}
}

// TestAblesung_PersonenDezimal_Warmwasseranteil: 1,25 zu 2 Personen teilt
// 10 m³ Warmwasser auf 3,85 m³ zu 6,15 m³ auf.
func TestAblesung_PersonenDezimal_Warmwasseranteil(t *testing.T) {
	db := openTestDB(t)

	first := vollesFormular()
	first.Set("reading_date", "2026-10-01")
	first.Set("monat", "2026-10")
	w := httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, first), db))
	if w.Code != http.StatusFound {
		t.Fatalf("first create status = %d (body: %s)", w.Code, w.Body.String())
	}

	// The Personenzahl of the month comes from its Fixkosten-Eingabe (#359).
	fk := fixkostenMitPreisen("2026-11-01")
	fk.Personen = map[int64]float64{1: 1.25, 2: 2}
	if _, err := store.CreateFixkostenEingabe(db, fk); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	second := vollesFormular()
	second.Set("personen_1", "1.25")
	second.Set("personen_2", "2")
	for _, key := range store.MeterKeys {
		second.Set(key, "11")
	}
	w = httptest.NewRecorder()
	handleCreateAblesung()(w, requestWithDB(formRequest(t, second), db))
	if w.Code != http.StatusFound {
		t.Fatalf("second create status = %d (body: %s)", w.Code, w.Body.String())
	}
	latest, err := store.GetLatestPeriod(db)
	if err != nil || latest == nil {
		t.Fatalf("GetLatestPeriod = %v, %v", latest, err)
	}
	body := detailBody(t, db, latest.ID)

	row := tabellenzeile(t, body, "Warmwasseraufbereitung-Anteil")
	for _, want := range []string{"3,85", "6,15"} {
		if !strings.Contains(row, want) {
			t.Errorf("Warmwasser-Zeile %q enthält nicht %q", row, want)
		}
	}
	pers := tabellenzeile(t, body, "Personen")
	if !strings.Contains(pers, ">1,25<") || !strings.Contains(pers, ">2<") {
		t.Errorf("Personen-Zeile %q zeigt nicht 1,25 und 2", pers)
	}
}

func TestFixkosten_PersonenDezimal(t *testing.T) {
	db := openTestDB(t)

	post := func(personen1 string) *httptest.ResponseRecorder {
		form := fixkostenForm("2026-09")
		form.Set("personen_1", personen1)
		req := httptest.NewRequest(http.MethodPost, "/fixkosten", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		handleCreateFixkosten()(w, requestWithDB(req, db))
		return w
	}

	for _, raw := range []string{"1.255"} {
		if w := post(raw); w.Code != http.StatusBadRequest {
			t.Errorf("personen_1=%q: status = %d, want 400", raw, w.Code)
		}
	}
	if w := post("1.25"); w.Code != http.StatusFound {
		t.Fatalf("personen_1=1.25: status = %d (body: %s)", w.Code, w.Body.String())
	}
	all, err := store.AllFixkostenEingabenDetails(db)
	if err != nil || len(all) != 1 {
		t.Fatalf("AllFixkostenEingabenDetails =%v, %v", all, err)
	}
	if got := all[0].Personen[1]; got != 1.25 {
		t.Errorf("Personen[1] = %v, want 1.25", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/fixkosten/"+strconv.FormatInt(all[0].ID, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(all[0].ID, 10))
	w := httptest.NewRecorder()
	handleFixkostenDetail(newZugang("", nil))(w, requestWithDB(req, db))
	if !strings.Contains(w.Body.String(), ">1,25</td>") {
		t.Errorf("Fixkosten-Detail zeigt 1,25 nicht")
	}
}

func TestPersonenzahlFeldSchritt(t *testing.T) {
	for _, f := range []string{"templates/wizard.html", "templates/fixkosten_form.html"} {
		b, err := templateFS.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, `name="personen_`) && !strings.Contains(line, `step="0.01"`) {
				t.Errorf("%s: Personen-Feld ohne step=\"0.01\": %s", f, line)
			}
		}
	}
}
