package web

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// antwort is how a route answered, as far as the Gate is concerned.
type antwort string

const (
	durchlass       antwort = "durchlass"
	anmeldeRedirect antwort = "anmelde-redirect"
	sperrseite      antwort = "403-seite"
)

// antwortVon classifies the response of a route behind a Gate. The login
// handler itself redirects to the overlay with fehler=1, that is not the
// Gate bouncing a visitor.
func antwortVon(w *httptest.ResponseRecorder) antwort {
	loc := w.Header().Get("Location")
	switch {
	case w.Code == http.StatusForbidden:
		return sperrseite
	case w.Code == http.StatusFound && strings.Contains(loc, "login=1") && !strings.Contains(loc, "fehler=1"):
		return anmeldeRedirect
	}
	return durchlass
}

// erwartet derives the answer from the Anspruch alone: the Vermieter and an
// Offen route always pass, a Recht passes when the role has it, everything
// else is answered by role (a Mieter gets the 403 page, a visitor the login).
// teilstand only matters for AnlegenOderVermieter.
func erwartet(a Anspruch, rolle store.Rolle, rechtAn, teilstand bool) antwort {
	if a == AnlegenOderVermieter {
		a = NurVermieter
		if teilstand {
			a = Braucht(store.RechtAblesungAnlegen)
		}
	}
	switch {
	case a == Offen, rolle == RolleVermieter, a.art == anspruchBraucht && rechtAn:
		return durchlass
	case rolle == store.RolleMieter:
		return sperrseite
	}
	return anmeldeRedirect
}

// rechtVon is the Recht a row needs, "" for NurVermieter and Offen. A
// Teilstand row needs the Recht to create a reading.
func rechtVon(a Anspruch, teilstand bool) store.Recht {
	if a == AnlegenOderVermieter && teilstand {
		return store.RechtAblesungAnlegen
	}
	return a.recht
}

// routenUmgebung is a mux over fresh databases for one role and Berechtigungen,
// with a complete reading and a Teilstand (only the newest one may be a
// Teilstand, so the complete one comes first).
type routenUmgebung struct {
	mux          *http.ServeMux
	vollstaendig int64
	teilstand    int64
}

func neueRoutenUmgebung(t *testing.T, rolle store.Rolle, b store.Berechtigungen) routenUmgebung {
	t.Helper()
	db := openTestDB(t)
	vollstaendig, err := store.CreatePeriod(db, seedPeriodInputAt("2026-11-01"))
	if err != nil {
		t.Fatalf("CreatePeriod (vollständig): %v", err)
	}
	teilstand, err := store.CreatePeriod(db, store.PeriodInput{ReadingDate: "2026-12-01"})
	if err != nil {
		t.Fatalf("CreatePeriod (Teilstand): %v", err)
	}
	z := NewZugang(festeRolle{rolle: rolle}, matrixAus{b: b})
	return routenUmgebung{muxFuer(z, db, openTestDB(t), "", ""), vollstaendig, teilstand}
}

func (u routenUmgebung) anfrage(method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	w := httptest.NewRecorder()
	u.mux.ServeHTTP(w, req)
	return w
}

var alleRollen = []store.Rolle{store.RolleNichtAngemeldet, store.RolleMieter, RolleVermieter}

func testRouten(z *Zugang) []route { return routen(z, "", "") }

// TestRouten_Verdrahtung drives every row of the table through the mux for
// every role, with the Recht of the row granted and not granted, and checks
// the answer against the Anspruch of the row. A row with AnlegenOderVermieter
// is driven for a Teilstand and for a complete reading.
func TestRouten_Verdrahtung(t *testing.T) {
	for _, r := range testRouten(NewZugang(festeRolle{}, matrixAus{})) {
		zustaende := []bool{false}
		if r.Anspruch == AnlegenOderVermieter {
			zustaende = []bool{false, true}
		}
		for _, teilstand := range zustaende {
			for _, rolle := range alleRollen {
				for _, rechtAn := range []bool{false, true} {
					name := r.Methode + " " + r.Pfad + " " + string(rolle) + " recht=" + strconv.FormatBool(rechtAn)
					if r.Anspruch == AnlegenOderVermieter {
						name += " teilstand=" + strconv.FormatBool(teilstand)
					}
					t.Run(name, func(t *testing.T) {
						b := store.Berechtigungen{rolle: {}}
						if recht := rechtVon(r.Anspruch, teilstand); rechtAn && recht != "" {
							b = berechtigtMit(rolle, recht)
						}
						u := neueRoutenUmgebung(t, rolle, b)
						id := u.vollstaendig
						if teilstand {
							id = u.teilstand
						}
						pfad := strings.ReplaceAll(r.Pfad, "{id}", strconv.FormatInt(id, 10))
						got := antwortVon(u.anfrage(r.Methode, pfad))
						if want := erwartet(r.Anspruch, rolle, rechtAn, teilstand); got != want {
							t.Errorf("antwort %s, want %s", got, want)
						}
					})
				}
			}
		}
	}
}

// TestRouten_Policy is independent of what the table says for the rest: every
// writing route and every export is for the Vermieter alone. The exceptions
// are named here with the Anspruch they must carry, plus the Offen routes.
func TestRouten_Policy(t *testing.T) {
	ausnahmen := map[string]Anspruch{
		"POST /ablesungen":                Braucht(store.RechtAblesungAnlegen),
		"POST /ablesungen/{id}":           AnlegenOderVermieter,
		"POST /login":                     Offen,
		"POST /logout":                    Offen,
		"GET /ablesungen/neu":             Braucht(store.RechtAblesungAnlegen),
		"GET /ablesungen/{id}/bearbeiten": AnlegenOderVermieter,
	}
	for _, r := range testRouten(NewZugang(festeRolle{}, matrixAus{})) {
		key := r.Methode + " " + r.Pfad
		schreibend := r.Methode != http.MethodGet && r.Methode != http.MethodHead
		export := strings.HasSuffix(r.Pfad, "/export.csv")
		want, ausnahme := ausnahmen[key]
		switch {
		case ausnahme:
			if r.Anspruch != want {
				t.Errorf("%s: Anspruch %+v, want %+v", key, r.Anspruch, want)
			}
		case schreibend || export:
			if r.Anspruch != NurVermieter {
				t.Errorf("%s writes or exports and must be NurVermieter, is %+v", key, r.Anspruch)
			}
		}
	}
}

func TestRouten_Wohlgeformt(t *testing.T) {
	gesehen := map[string]bool{}
	for _, r := range testRouten(NewZugang(festeRolle{}, matrixAus{})) {
		key := r.Methode + " " + r.Pfad
		if gesehen[key] {
			t.Errorf("%s appears twice", key)
		}
		gesehen[key] = true
		if r.Handler == nil {
			t.Errorf("%s has no handler", key)
		}
		if r.Anspruch.art == anspruchBraucht && r.Anspruch.recht == "" {
			t.Errorf("%s needs a Recht but names none", key)
		}
	}
}

// TestRegistrierungNurInNewMux makes sure no route is registered past the
// table: in the non-test files of this package exactly one call registers
// on a mux, and it sits in handlers.go (muxFuer, which NewMux uses).
func TestRegistrierungNurInNewMux(t *testing.T) {
	dateien, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var stellen []string
	fset := token.NewFileSet()
	for _, datei := range dateien {
		if strings.HasSuffix(datei, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, datei, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "HandleFunc" || sel.Sel.Name == "Handle") {
				stellen = append(stellen, fset.Position(call.Pos()).String())
			}
			return true
		})
	}
	if len(stellen) != 1 || !strings.HasPrefix(stellen[0], "handlers.go:") {
		t.Errorf("routes registered at %v, want exactly one place in handlers.go", stellen)
	}
}

// teilstandZeilen are the rows of the table that correct a reading.
func teilstandZeilen(t *testing.T) []route {
	t.Helper()
	var zeilen []route
	for _, r := range testRouten(NewZugang(festeRolle{}, matrixAus{})) {
		if r.Anspruch == AnlegenOderVermieter {
			zeilen = append(zeilen, r)
		}
	}
	if len(zeilen) != 2 {
		t.Fatalf("%d rows with AnlegenOderVermieter, want 2 (form and save)", len(zeilen))
	}
	return zeilen
}

// TestRouten_Teilstand covers Ticket #130 through the rows of the table: a
// Teilstand needs the Recht to create a reading, a complete reading is for
// the Vermieter, a missing id answers 404 and an invalid id 400 before any
// right is checked.
func TestRouten_Teilstand(t *testing.T) {
	for _, r := range teilstandZeilen(t) {
		pfad := func(id string) string { return strings.ReplaceAll(r.Pfad, "{id}", id) }
		t.Run(r.Methode+" "+r.Pfad, func(t *testing.T) {
			ohne := neueRoutenUmgebung(t, store.RolleNichtAngemeldet, store.Berechtigungen{})
			mit := neueRoutenUmgebung(t, store.RolleNichtAngemeldet, berechtigtMit(store.RolleNichtAngemeldet, store.RechtAblesungAnlegen))
			mieter := neueRoutenUmgebung(t, store.RolleMieter, berechtigtMit(store.RolleMieter, store.RechtAblesungAnlegen))
			vermieter := neueRoutenUmgebung(t, RolleVermieter, store.Berechtigungen{})

			id := func(u routenUmgebung, teilstand bool) string {
				if teilstand {
					return strconv.FormatInt(u.teilstand, 10)
				}
				return strconv.FormatInt(u.vollstaendig, 10)
			}
			tests := []struct {
				name string
				u    routenUmgebung
				id   string
				want antwort
			}{
				{"Teilstand, Besucher mit Recht", mit, id(mit, true), durchlass},
				{"Teilstand, Besucher ohne Recht", ohne, id(ohne, true), anmeldeRedirect},
				{"Teilstand, Mieter mit Recht", mieter, id(mieter, true), durchlass},
				{"Teilstand, Vermieter", vermieter, id(vermieter, true), durchlass},
				{"vollständig, Besucher mit Recht", mit, id(mit, false), anmeldeRedirect},
				{"vollständig, Mieter mit Recht", mieter, id(mieter, false), sperrseite},
				{"vollständig, Vermieter", vermieter, id(vermieter, false), durchlass},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					if got := antwortVon(tt.u.anfrage(r.Methode, pfad(tt.id))); got != tt.want {
						t.Errorf("antwort %s, want %s", got, tt.want)
					}
				})
			}

			// The id is looked up before the right is checked, so even a
			// visitor without any right learns 404 and 400.
			if w := ohne.anfrage(r.Methode, pfad("999999")); w.Code != http.StatusNotFound {
				t.Errorf("missing id: status %d, want 404 (no login redirect)", w.Code)
			}
			if w := ohne.anfrage(r.Methode, pfad("abc")); w.Code != http.StatusBadRequest {
				t.Errorf("invalid id: status %d, want 400", w.Code)
			}
		})
	}
}

// TestNewMux_ProduktionsZugang runs the real NewMux, with the cookie adapter
// and the passwords of the environment: a visitor is bounced from a Vermieter
// route, and without a Vermieter password everything is open, a complete
// reading included.
func TestNewMux_ProduktionsZugang(t *testing.T) {
	t.Run("Besucher mit Kennwort", func(t *testing.T) {
		t.Setenv("VERMIETER_PASSWORD", "geheim")
		w := httptest.NewRecorder()
		NewMux(openTestDB(t), openTestDB(t), "", "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/abrechnung", nil))
		if got := antwortVon(w); got != anmeldeRedirect {
			t.Errorf("antwort %s, want %s", got, anmeldeRedirect)
		}
	})
	t.Run("ohne Kennwort bleibt alles offen", func(t *testing.T) {
		db := openTestDB(t)
		id, err := store.CreatePeriod(db, seedPeriodInputAt("2026-12-01"))
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		w := httptest.NewRecorder()
		NewMux(db, openTestDB(t), "", "").ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ablesungen/"+strconv.FormatInt(id, 10)+"/bearbeiten", nil))
		if w.Code != http.StatusOK {
			t.Errorf("status %d, want 200", w.Code)
		}
	})
}
