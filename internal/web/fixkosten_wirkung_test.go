package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestFixkostenWirkungVon(t *testing.T) {
	periode := func(id int64, datum, monat string, teil bool) *store.LatestPeriod {
		r := map[string]float64{}
		for _, key := range store.MeterKeys {
			r[key] = 0
		}
		if teil {
			delete(r, store.MeterKeys[0])
		}
		return &store.LatestPeriod{ID: id, ReadingDate: datum, Monat: monat, Readings: r}
	}
	eingabe := func(id int64, vollstaendig bool) *store.FixkostenEingabeDetails {
		e := &store.FixkostenEingabeDetails{ID: id, Monat: "2026-10-01", Personen: map[int64]float64{1: 1, 2: 1},
			Strompreis: store.Float64(1), FrischwasserPreis: store.Float64(1), AbwasserPreis: store.Float64(1)}
		if vollstaendig {
			e.EinspeisungPreis = store.Float64(1)
		}
		return e
	}
	personen := store.Sonderkosten{Monat: "2026-10-01", Umlegbar: true, Objekt: store.ObjektHaus, Logik: store.LogikPersonen}
	nichtUmlegbar := personen
	nichtUmlegbar.Umlegbar = false
	anderer := personen
	anderer.Monat = "2026-09-01"
	altdaten := personen
	altdaten.Objekt = store.ObjektWohnung1

	basis := []*store.LatestPeriod{periode(1, "2026-09-10", "2026-09-01", false), periode(2, "2026-10-10", "2026-10-01", false)}
	mehrere := []*store.LatestPeriod{basis[0], basis[1], periode(3, "2026-10-20", "2026-10-01", false)}
	teilstand := []*store.LatestPeriod{basis[0], periode(2, "2026-10-10", "2026-10-01", true)}
	tests := []struct {
		name         string
		periods      []*store.LatestPeriod
		eingaben     []*store.FixkostenEingabeDetails
		sonder       []store.Sonderkosten
		vollstaendig bool
		eingabeID    int64
		ablesungen   int
		sonderkosten int
		fehlt, bere  bool
	}{
		{"keine Eingabe", basis, nil, nil, false, 0, 1, 0, true, false},
		{"unvollständig", basis, []*store.FixkostenEingabeDetails{eingabe(7, false)}, nil, false, 7, 1, 0, true, false},
		{"vollständig, Ablesung wartet", basis, []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 1, 0, false, true},
		{"nur erste Ablesung", basis[1:], []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 0, 0, false, false},
		{"nur Teilstand", teilstand, nil, nil, false, 0, 0, 0, false, false},
		{"mehrere Ablesungen", mehrere, []*store.FixkostenEingabeDetails{eingabe(7, true)}, nil, true, 7, 2, 0, false, true},
		{"eine Sonderkosten", basis, nil, []store.Sonderkosten{personen, nichtUmlegbar, anderer, altdaten}, false, 0, 1, 1, true, false},
		{"viele Sonderkosten", basis, []*store.FixkostenEingabeDetails{eingabe(7, true)}, []store.Sonderkosten{personen, personen}, true, 7, 1, 2, false, true},
	}
	for _, tt := range tests {
		ds := &store.Datenstand{Periods: tt.periods, Eingaben: tt.eingaben, Sonderkosten: tt.sonder, Apartments: []store.Apartment{{ID: 1, QM: 1}, {ID: 2, QM: 1}}}
		w := fixkostenWirkungVon(ds, calc.KostenAus(ds), "2026-10-01")
		if w.Vollstaendig != tt.vollstaendig || w.EingabeID != tt.eingabeID ||
			len(w.Ablesungen) != tt.ablesungen || w.Sonderkosten != tt.sonderkosten || w.Fehlt() != tt.fehlt || w.Berechenbar() != tt.bere {
			t.Errorf("%s: got %+v (Fehlt %v, Berechenbar %v)", tt.name, w, w.Fehlt(), w.Berechenbar())
		}
	}
}

// A failing Datenstand load ends as HTTP 500 on the detail page and, after a
// delete, on the list. Dropping the Sonderkosten table breaks only that load.
func TestFixkostenSeiten_DatenstandLadefehlerIstHTTP500(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreateFixkostenEingabe(db, hinweisFixkosten("2026-07-01"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE sonderkosten`); err != nil {
		t.Fatal(err)
	}
	z := newZugang("", nil)
	for name, call := range map[string]func(http.ResponseWriter, *http.Request){
		"Detail": func(w http.ResponseWriter, r *http.Request) {
			r.SetPathValue("id", strconv.FormatInt(id, 10))
			handleFixkostenDetail(z)(w, r)
		},
		"Liste nach Löschen": handleFixkostenListe(z),
	} {
		target := "/fixkosten/" + strconv.FormatInt(id, 10)
		if name != "Detail" {
			target = "/fixkosten?" + paramGeloescht + "=2026-07"
		}
		w := httptest.NewRecorder()
		call(w, requestWithDB(httptest.NewRequest(http.MethodGet, target, nil), db))
		if w.Code != http.StatusInternalServerError {
			t.Errorf("%s: status = %d, want 500", name, w.Code)
		}
	}
}
