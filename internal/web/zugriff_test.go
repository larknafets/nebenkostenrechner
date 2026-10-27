package web

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func neueAnfrage(method, path string) *http.Request {
	return requestWithDB(httptest.NewRequest(method, path, nil), nil)
}

func zugriffFuer(rolle store.Rolle, m Matrix) Zugriff {
	return NewZugang(festeRolle{rolle: rolle}, m).Zugriff(neueAnfrage(http.MethodGet, "/"))
}

// logAufnehmen captures the standard logger for the rest of the test.
func logAufnehmen(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

func TestZugriff_DarfErfuelltPersonenbezug(t *testing.T) {
	mieterMit := matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtFixkostenAnsehen)}
	tests := []struct {
		name     string
		rolle    store.Rolle
		m        matrixAus
		darf     bool
		nurV     bool
		personen bool
	}{
		{"Vermieter ohne Matrix", RolleVermieter, matrixAus{}, true, true, true},
		{"Mieter mit dem Recht", store.RolleMieter, mieterMit, true, false, false},
		{"Mieter ohne das Recht", store.RolleMieter, matrixAus{b: berechtigtMit(store.RolleMieter)}, false, false, false},
		{"Nicht angemeldet, Recht nur für Mieter", store.RolleNichtAngemeldet, mieterMit, false, false, false},
		{"Nicht angemeldet mit dem Recht", store.RolleNichtAngemeldet, matrixAus{b: berechtigtMit(store.RolleNichtAngemeldet, store.RechtFixkostenAnsehen)}, true, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			z := zugriffFuer(tt.rolle, tt.m)
			if got := z.Darf(store.RechtFixkostenAnsehen); got != tt.darf {
				t.Errorf("Darf = %v, want %v", got, tt.darf)
			}
			if got := z.Erfuellt(Braucht(store.RechtFixkostenAnsehen)); got != tt.darf {
				t.Errorf("Erfuellt(Braucht) = %v, want %v", got, tt.darf)
			}
			if got := z.Erfuellt(NurVermieter); got != tt.nurV {
				t.Errorf("Erfuellt(NurVermieter) = %v, want %v", got, tt.nurV)
			}
			if !z.Erfuellt(Offen) {
				t.Error("Erfuellt(Offen) = false")
			}
			if got := z.Personenbezug(); got != tt.personen {
				t.Errorf("Personenbezug = %v, want %v", got, tt.personen)
			}
		})
	}
}

func TestZugriff_NullwertFordertNurVermieter(t *testing.T) {
	if (Zugriff{}).Erfuellt(Anspruch{}) {
		t.Error("the zero Anspruch must deny a visitor, not open the route")
	}
	if (Zugriff{}).Darf(store.RechtMiete) {
		t.Error("the zero Zugriff has no right")
	}
}

func TestZugriff_KannFelderFolgenDerMatrix(t *testing.T) {
	z := zugriffFuer(store.RolleMieter, matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtAblesungenAnsehen, store.RechtStammdatenAnsehen)})
	if !z.KannAblesungenAnsehen || z.KannAblesungAnlegen || z.KannFixkostenAnsehen || !z.KannStammdatenAnsehen {
		t.Errorf("Kann* = %v %v %v %v", z.KannAblesungenAnsehen, z.KannAblesungAnlegen, z.KannFixkostenAnsehen, z.KannStammdatenAnsehen)
	}
	v := zugriffFuer(RolleVermieter, matrixAus{})
	if !v.KannAblesungenAnsehen || !v.KannAblesungAnlegen || !v.KannFixkostenAnsehen || !v.KannStammdatenAnsehen {
		t.Error("the Vermieter has every right")
	}
}

func TestZugriff_Sichtbar(t *testing.T) {
	m := matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtWohnung1, store.RechtPVAnlage)}
	tests := []struct {
		name  string
		rolle store.Rolle
		e     entitaet
		want  bool
	}{
		{"Vermieter sieht die Wärmepumpe", RolleVermieter, entitaet{Art: entitaetWP}, true},
		{"Vermieter sieht eine Wohnung ohne Recht", RolleVermieter, wohnungEntitaet(7), true},
		{"Mieter sieht Wohnung 1", store.RolleMieter, wohnungEntitaet(1), true},
		{"Mieter sieht Wohnung 2 nicht", store.RolleMieter, wohnungEntitaet(2), false},
		{"Mieter sieht PV", store.RolleMieter, entitaet{Art: entitaetPV}, true},
		{"Mieter sieht die Wärmepumpe nicht", store.RolleMieter, entitaet{Art: entitaetWP}, false},
		{"Mieter sieht Wohnung ohne Recht nicht", store.RolleMieter, wohnungEntitaet(7), false},
		{"Nicht angemeldet sieht PV nicht", store.RolleNichtAngemeldet, entitaet{Art: entitaetPV}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := zugriffFuer(tt.rolle, m).Sichtbar(tt.e); got != tt.want {
				t.Errorf("Sichtbar = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestZugriff_VermieterLoestKeineAbfrageAus(t *testing.T) {
	var geladen int
	z := zugriffFuer(RolleVermieter, matrixAus{geladen: &geladen})
	z.Darf(store.RechtMiete)
	z.Sichtbar(entitaet{Art: entitaetPV})
	if z.Fehler() != nil || geladen != 0 {
		t.Errorf("Vermieter: Fehler %v, %d loads, want none", z.Fehler(), geladen)
	}
}

func TestZugriff_MatrixWirdProRequestEinmalGeladen(t *testing.T) {
	var geladen int
	zugang := NewZugang(festeRolle{rolle: store.RolleMieter}, matrixAus{b: berechtigtMit(store.RolleMieter), geladen: &geladen})
	req := neueAnfrage(http.MethodGet, "/")
	for i := 0; i < 3; i++ {
		z := zugang.Zugriff(req)
		z.Darf(store.RechtMiete)
		zugang.NavData(req)
	}
	if geladen != 1 {
		t.Errorf("%d loads in one request, want 1", geladen)
	}
	zugang.Zugriff(neueAnfrage(http.MethodGet, "/"))
	if geladen != 2 {
		t.Errorf("%d loads after a second request, want 2", geladen)
	}
}

func TestZugriff_LadefehlerSperrtAlleRechte(t *testing.T) {
	logbuf := logAufnehmen(t)
	z := zugriffFuer(store.RolleMieter, matrixAus{err: errors.New("db weg")})
	if z.Darf(store.RechtFixkostenAnsehen) || z.KannAblesungenAnsehen || z.KannFixkostenAnsehen || z.KannStammdatenAnsehen {
		t.Error("a load error must leave no right")
	}
	if z.Fehler() == nil {
		t.Error("Fehler = nil after a load error")
	}
	if !strings.Contains(logbuf.String(), "berechtigungen: db weg") {
		t.Errorf("log %q misses the load error", logbuf.String())
	}
	if n := strings.Count(logbuf.String(), "db weg"); n != 1 {
		t.Errorf("load error logged %d times, want once", n)
	}
}

func gateAnfrage(zugang *Zugang, a Anspruch) (*httptest.ResponseRecorder, bool) {
	var erreicht bool
	h := zugang.Gate(a, func(http.ResponseWriter, *http.Request) { erreicht = true })
	w := httptest.NewRecorder()
	h(w, neueAnfrage(http.MethodGet, "/fixkosten"))
	return w, erreicht
}

func TestGate_AntwortenJeRolle(t *testing.T) {
	granted := matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtFixkostenAnsehen)}
	braucht := Braucht(store.RechtFixkostenAnsehen)
	tests := []struct {
		name     string
		rolle    store.Rolle
		m        matrixAus
		a        Anspruch
		status   int
		erreicht bool
		ort      string
	}{
		{"Vermieter, NurVermieter", RolleVermieter, matrixAus{}, NurVermieter, 200, true, ""},
		{"Vermieter, Recht", RolleVermieter, matrixAus{}, braucht, 200, true, ""},
		{"Mieter mit Recht", store.RolleMieter, granted, braucht, 200, true, ""},
		{"Mieter ohne Recht", store.RolleMieter, matrixAus{b: berechtigtMit(store.RolleMieter)}, braucht, 403, false, ""},
		{"Mieter, NurVermieter", store.RolleMieter, granted, NurVermieter, 403, false, ""},
		{"Besucher ohne Recht", store.RolleNichtAngemeldet, granted, braucht, 302, false, "/dashboard?login=1"},
		{"Besucher, NurVermieter", store.RolleNichtAngemeldet, granted, NurVermieter, 302, false, "/dashboard?login=1"},
		{"Besucher, Offen", store.RolleNichtAngemeldet, matrixAus{}, Offen, 200, true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, erreicht := gateAnfrage(NewZugang(festeRolle{rolle: tt.rolle}, tt.m), tt.a)
			if w.Code != tt.status || erreicht != tt.erreicht {
				t.Errorf("status %d, erreicht %v, want %d, %v", w.Code, erreicht, tt.status, tt.erreicht)
			}
			if !strings.HasPrefix(w.Header().Get("Location"), tt.ort) {
				t.Errorf("Location %q, want prefix %q", w.Header().Get("Location"), tt.ort)
			}
		})
	}
}

func TestGate_LadefehlerAntwortetMit500(t *testing.T) {
	logAufnehmen(t)
	fehler := matrixAus{err: errors.New("db weg")}
	for _, rolle := range []store.Rolle{store.RolleMieter, store.RolleNichtAngemeldet} {
		w, erreicht := gateAnfrage(NewZugang(festeRolle{rolle: rolle}, fehler), Braucht(store.RechtFixkostenAnsehen))
		if w.Code != http.StatusInternalServerError || erreicht {
			t.Errorf("%s: status %d, erreicht %v, want 500 and no handler", rolle, w.Code, erreicht)
		}
	}
	// The Vermieter never loads, so he never fails.
	w, erreicht := gateAnfrage(NewZugang(festeRolle{rolle: RolleVermieter}, fehler), Braucht(store.RechtFixkostenAnsehen))
	if w.Code != http.StatusOK || !erreicht {
		t.Errorf("Vermieter: status %d, erreicht %v", w.Code, erreicht)
	}
	// A route for the Vermieter alone needs no right, the usual answer stays.
	w, _ = gateAnfrage(NewZugang(festeRolle{rolle: store.RolleMieter}, fehler), NurVermieter)
	if w.Code != http.StatusForbidden {
		t.Errorf("Mieter on NurVermieter with a load error: status %d, want 403", w.Code)
	}
}

func TestNavData_LadefehlerRendertOhneRechte(t *testing.T) {
	logbuf := logAufnehmen(t)
	zugang := NewZugang(festeRolle{rolle: store.RolleMieter, sitz: true}, matrixAus{err: errors.New("db weg")})
	nav := zugang.NavData(neueAnfrage(http.MethodGet, "/"))
	if nav.Vermieter || nav.KannAblesungenAnsehen || nav.KannAblesungAnlegen || nav.KannFixkostenAnsehen || nav.KannStammdatenAnsehen {
		t.Errorf("nav carries rights despite the load error: %+v", nav.Zugriff)
	}
	if !nav.ShowLoginEntry || !nav.ShowLogoutEntry {
		t.Error("the login and logout entries must not depend on the Berechtigungen")
	}
	if !strings.Contains(logbuf.String(), "berechtigungen: db weg") {
		t.Errorf("log %q misses the load error", logbuf.String())
	}
}

func TestNavData_Einstiegspunkte(t *testing.T) {
	tests := []struct {
		name   string
		s      festeRolle
		login  bool
		logout bool
	}{
		{"offene App, keine Sitzung", festeRolle{rolle: RolleVermieter}, true, false},
		{"Vermieter-Sitzung", festeRolle{rolle: RolleVermieter, sitz: true}, false, true},
		{"Demo", festeRolle{rolle: RolleVermieter, demo: true, sitz: true}, false, true},
		{"Mieter", festeRolle{rolle: store.RolleMieter, sitz: true}, true, true},
		{"Besucher", festeRolle{rolle: store.RolleNichtAngemeldet}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nav := NewZugang(tt.s, matrixAus{b: berechtigtMit(store.RolleMieter)}).NavData(neueAnfrage(http.MethodGet, "/"))
			if nav.ShowLoginEntry != tt.login || nav.ShowLogoutEntry != tt.logout || nav.IsDemoSession != tt.s.demo {
				t.Errorf("login %v logout %v demo %v", nav.ShowLoginEntry, nav.ShowLogoutEntry, nav.IsDemoSession)
			}
		})
	}
}

func TestSitzungenCookieAdapter_Rolle(t *testing.T) {
	vermieter := &http.Cookie{Name: sessionCookieName, Value: signSession("v", farFuture())}
	mieter := &http.Cookie{Name: mieterSessionCookieName, Value: signSession("m", farFuture())}
	demo := &http.Cookie{Name: demoSessionCookieName, Value: signSession(demoSessionSecret, farFuture())}
	tests := []struct {
		name    string
		a       auth
		cookies []*http.Cookie
		rolle   store.Rolle
		demo    bool
		sitzung bool
	}{
		{"ohne Kennwort, ohne Cookie", auth{}, nil, RolleVermieter, false, false},
		{"mit Kennwort, ohne Cookie", auth{vermieter: "v"}, nil, store.RolleNichtAngemeldet, false, false},
		{"Vermieter-Cookie", auth{vermieter: "v"}, []*http.Cookie{vermieter}, RolleVermieter, false, true},
		{"Mieter-Cookie", auth{vermieter: "v", mieter: "m"}, []*http.Cookie{mieter}, store.RolleMieter, false, true},
		{"Demo-Cookie", auth{vermieter: "v"}, []*http.Cookie{demo}, RolleVermieter, true, true},
		{"Demo vor Mieter", auth{vermieter: "v", mieter: "m"}, []*http.Cookie{mieter, demo}, RolleVermieter, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			for _, c := range tt.cookies {
				req.AddCookie(c)
			}
			rolle, demo, sitzung := tt.a.Rolle(req)
			if rolle != tt.rolle || demo != tt.demo || sitzung != tt.sitzung {
				t.Errorf("Rolle = %q %v %v, want %q %v %v", rolle, demo, sitzung, tt.rolle, tt.demo, tt.sitzung)
			}
		})
	}
}
