package web

import (
	"errors"
	"net/http"
	"net/url"
	"reflect"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Table tests of the three pure functions behind /stammdaten: no HTTP, no
// database.

func gueltigesFormular(extra url.Values) url.Values {
	f := url.Values{
		"qm_1": {"100"}, "flurstueck_groesse_1": {"600"}, "qm_2": {"50.5"}, "flurstueck_groesse_2": {"400"},
		"status_1": {"eigennutzung"}, "status_2": {"vermietet"}, "heizung_gewichtung": {"0.6"},
	}
	for k, v := range extra {
		f[k] = v
	}
	return f
}

var (
	testWohnungen  = []store.Apartment{{ID: 1}, {ID: 2}}
	testPositionen = []store.Kostenposition{{ID: 10}, {ID: 11}}
)

func TestStammdatenAusFormular_Felder(t *testing.T) {
	form := gueltigesFormular(url.Values{
		"mieter_name_2":         {"  Erika Muster \r\n"},
		"mieter_anschrift_2":    {"Weg 1\r\n12345 Ort"},
		"mieter_seit_2":         {"2026-03"},
		"vermieter_name":        {" V "},
		"vermieter_anschrift":   {"Str 2\r\nOrt"},
		"objekt_anschrift":      {"Haus 3"},
		"iban":                  {"DE00 1234"},
		"kontoinhaber":          {"K"},
		"strom_weiterberechnen": {"1"},
		"umlagefaehig_11":       {"1"},
	})
	got, err := stammdatenAusFormular(form, testWohnungen, testPositionen, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := store.StammdatenSave{
		Apartments: map[int64]store.StammdatenInput{
			1: {QM: 100, FlurstueckGroesse: 600},
			2: {QM: 50.5, FlurstueckGroesse: 400},
		},
		Flags: store.StammdatenFlags{
			Umlagefaehig:         map[int64]bool{10: false, 11: true},
			StromWeiterberechnen: true,
		},
		HeizungWaermeGewichtung: 0.6,
		Wohnungen: map[int64]store.WohnungDetails{
			1: {Status: "eigennutzung"},
			2: {MieterName: "Erika Muster", MieterAnschrift: "Weg 1\n12345 Ort", Status: "vermietet", MieterSeit: "2026-03-01"},
		},
		Haus: store.HausDetails{VermieterName: "V", VermieterAnschrift: "Str 2\nOrt", ObjektAnschrift: "Haus 3", IBAN: "DE00 1234", Kontoinhaber: "K"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("save =\n%+v\nwant\n%+v", got, want)
	}
}

func TestStammdatenAusFormular_Flags(t *testing.T) {
	cases := []struct {
		name                       string
		extra                      url.Values
		strom, wallbox, kp10, kp11 bool
	}{
		{"alle unmarkiert", nil, false, false, false, false},
		{"alle markiert", url.Values{"strom_weiterberechnen": {"1"}, "wallbox_weiterberechnen": {"1"}, "umlagefaehig_10": {"1"}, "umlagefaehig_11": {"1"}}, true, true, true, true},
		{"nur Wallbox", url.Values{"wallbox_weiterberechnen": {"1"}}, false, true, false, false},
		{"anderer Wert als 1 heisst Nein", url.Values{"strom_weiterberechnen": {"on"}, "umlagefaehig_10": {"0"}}, false, false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := stammdatenAusFormular(gueltigesFormular(c.extra), testWohnungen, testPositionen, nil)
			if err != nil {
				t.Fatal(err)
			}
			f := got.Flags
			if f.StromWeiterberechnen != c.strom || f.WallboxWeiterberechnen != c.wallbox || f.Umlagefaehig[10] != c.kp10 || f.Umlagefaehig[11] != c.kp11 {
				t.Errorf("flags = %+v", f)
			}
		})
	}
}

func TestStammdatenAusFormular_Gewichtung(t *testing.T) {
	for _, roh := range []string{"0.7", "0.6", "0.5", "0.8"} {
		// A value outside the allowed ones is store.SaveStammdaten's call,
		// the form only needs a number.
		f := gueltigesFormular(url.Values{"heizung_gewichtung": {roh}})
		got, err := stammdatenAusFormular(f, testWohnungen, testPositionen, nil)
		if err != nil {
			t.Fatalf("%s: %v", roh, err)
		}
		if want := map[string]float64{"0.7": 0.7, "0.6": 0.6, "0.5": 0.5, "0.8": 0.8}[roh]; got.HeizungWaermeGewichtung != want {
			t.Errorf("%s: Gewichtung = %v", roh, got.HeizungWaermeGewichtung)
		}
	}
}

func TestStammdatenAusFormular_Formularfehler(t *testing.T) {
	cases := []struct {
		name string
		mut  func(url.Values)
		want string
	}{
		{"qm keine Zahl", func(f url.Values) { f.Set("qm_2", "x") }, "ungültiger Wert für Wohnungsgröße bei Wohnung 2"},
		{"qm fehlt", func(f url.Values) { f.Del("qm_1") }, "ungültiger Wert für Wohnungsgröße bei Wohnung 1"},
		{"Flurstück keine Zahl", func(f url.Values) { f.Set("flurstueck_groesse_1", "") }, "ungültiger Wert für Flurstücksgröße bei Wohnung 1"},
		{"Gewichtung keine Zahl", func(f url.Values) { f.Set("heizung_gewichtung", "abc") }, "ungültige Heizungs-Gewichtung \"abc\" (muss 0.7, 0.6 oder 0.5 sein)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := gueltigesFormular(nil)
			c.mut(f)
			_, err := stammdatenAusFormular(f, testWohnungen, testPositionen, nil)
			if err == nil || err.Error() != c.want {
				t.Fatalf("err = %v, want %q", err, c.want)
			}
			if status, text := stammdatenFehler(err, f.Get("heizung_gewichtung")); status != http.StatusBadRequest || text != c.want {
				t.Errorf("stammdatenFehler = %d %q", status, text)
			}
		})
	}
}

func TestStammdatenAusFormular_Berechtigungen(t *testing.T) {
	gespeichert := func() store.Berechtigungen {
		return store.Berechtigungen{
			store.RolleNichtAngemeldet: {store.RechtAblesungenAnsehen: true, store.RechtFixkostenAnsehen: true},
			store.RolleMieter:          {store.RechtAblesungenAnsehen: true, store.RechtStammdatenAnsehen: true},
		}
	}
	cases := []struct {
		name  string
		extra url.Values
		want  func(store.Berechtigungen) bool
	}{
		{"Formular ohne Tabelle lässt nil", nil, func(b store.Berechtigungen) bool { return b == nil }},
		{
			"beide Spalten überschreiben",
			url.Values{
				"berechtigung_rolle": {"nicht_angemeldet", "mieter"},
				"recht_mieter_" + string(store.RechtFixkostenAnsehen): {"1"},
			},
			func(b store.Berechtigungen) bool {
				return !b.Erlaubt(store.RolleNichtAngemeldet, store.RechtAblesungenAnsehen) &&
					!b.Erlaubt(store.RolleNichtAngemeldet, store.RechtFixkostenAnsehen) &&
					b.Erlaubt(store.RolleMieter, store.RechtFixkostenAnsehen) &&
					!b.Erlaubt(store.RolleMieter, store.RechtAblesungenAnsehen)
			},
		},
		{
			"versteckte Mieter-Spalte behält ihre Werte",
			url.Values{
				"berechtigung_rolle": {"nicht_angemeldet"},
				"recht_nicht_angemeldet_" + string(store.RechtStammdatenAnsehen): {"1"},
			},
			func(b store.Berechtigungen) bool {
				return b.Erlaubt(store.RolleNichtAngemeldet, store.RechtStammdatenAnsehen) &&
					!b.Erlaubt(store.RolleNichtAngemeldet, store.RechtFixkostenAnsehen) &&
					b.Erlaubt(store.RolleMieter, store.RechtAblesungenAnsehen) &&
					b.Erlaubt(store.RolleMieter, store.RechtStammdatenAnsehen)
			},
		},
		{
			"unbekannte Rolle wird ignoriert",
			url.Values{"berechtigung_rolle": {"admin"}, "recht_admin_" + string(store.RechtStammdatenAnsehen): {"1"}},
			func(b store.Berechtigungen) bool { _, da := b["admin"]; return !da && len(b) == 2 },
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := gespeichert()
			got, err := stammdatenAusFormular(gueltigesFormular(c.extra), testWohnungen, testPositionen, g)
			if err != nil {
				t.Fatal(err)
			}
			if !c.want(got.Berechtigungen) {
				t.Errorf("Berechtigungen = %v", got.Berechtigungen)
			}
			if !reflect.DeepEqual(g, gespeichert()) {
				t.Errorf("the saved Berechtigungen were modified: %v", g)
			}
		})
	}
}

func TestStammdatenFehler(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		roh    string
		status int
		text   string
	}{
		{"Formularfehler", formFehler("ungültiger Wert für %s bei Wohnung %s", "Wohnungsgröße", "1"), "", 400, "ungültiger Wert für Wohnungsgröße bei Wohnung 1"},
		{"Wohnungsstatus", &store.InvalidStatusError{ApartmentID: 2, Status: "leer"}, "0.7", 400, "ungültiger Wohnungsstatus für Wohnung 2"},
		{"Mieter seit", &store.InvalidMieterSeitError{ApartmentID: 1, MieterSeit: "x"}, "0.7", 400, "ungültiges Datum für \"Mieter seit\" bei Wohnung 1"},
		{"Gewichtung", &store.InvalidHeizungGewichtungError{Gewichtung: 0.8}, "0.8", 400, "ungültige Heizungs-Gewichtung \"0.8\" (muss 0.7, 0.6 oder 0.5 sein)"},
		{"Fehler im Store umhüllt", errors.Join(errors.New("x"), &store.InvalidStatusError{ApartmentID: 3}), "", 400, "ungültiger Wohnungsstatus für Wohnung 3"},
		{"anderer Fehler", errors.New("disk full"), "", 500, "save: disk full"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, text := stammdatenFehler(c.err, c.roh)
			if status != c.status || text != c.text {
				t.Errorf("= %d %q, want %d %q", status, text, c.status, c.text)
			}
		})
	}
}

func TestStammdatenSicht(t *testing.T) {
	wohnungen := []store.Apartment{
		{ID: 1, Name: "EG", QM: 100, FlurstueckGroesse: 600, Status: store.StatusVermietet, MieterName: "M", MieterAnschrift: "A", MieterSeit: "2026-01-01"},
	}
	haus := store.Haus{
		StromWeiterberechnen: true, HeizungWaermeGewichtung: 0.6,
		VermieterName: "V", VermieterAnschrift: "VA", ObjektAnschrift: "OA", IBAN: "DE00", Kontoinhaber: "K",
	}
	cases := []struct {
		name  string
		rolle store.Rolle
		sehen bool
	}{
		{"Vermieter", RolleVermieter, true},
		{"Mieter", store.RolleMieter, false},
		{"nicht angemeldet", store.RolleNichtAngemeldet, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			z := NewZugang(festeRolle{rolle: c.rolle, sitz: true}, matrixAus{b: berechtigtMit(c.rolle, store.RechtStammdatenAnsehen)}).
				Zugriff(neueAnfrage(http.MethodGet, "/stammdaten"))
			w, h := stammdatenSicht(z, wohnungen, haus)
			if c.sehen {
				if !reflect.DeepEqual(w, wohnungen) || h != haus {
					t.Errorf("Vermieter sees a changed view: %+v %+v", w, h)
				}
				return
			}
			wantW := store.Apartment{ID: 1, Name: "EG", QM: 100, FlurstueckGroesse: 600, Status: store.StatusVermietet}
			if len(w) != 1 || w[0] != wantW {
				t.Errorf("Wohnung = %+v, want %+v", w[0], wantW)
			}
			wantH := store.Haus{StromWeiterberechnen: true, HeizungWaermeGewichtung: 0.6}
			if h != wantH {
				t.Errorf("Haus = %+v, want %+v", h, wantH)
			}
			if wohnungen[0].MieterName != "M" || haus.IBAN != "DE00" {
				t.Error("the arguments were modified")
			}
		})
	}
}
