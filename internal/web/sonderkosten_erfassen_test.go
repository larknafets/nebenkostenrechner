package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func postSonderkosten(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/sonderkosten", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func gueltigesSonderkostenForm() url.Values {
	return url.Values{
		"monat": {"2026-10"}, "art": {"reparatur"}, "betreff": {"Heizungspumpe"},
		"beschreibung": {"Pumpe\nHeizkreis 2"}, "firma": {"Meier GmbH"}, "rechnungsnummer": {"RE-412"},
		"betrag": {"1.234,50"}, "umsetzung": {"haus"}, "umlegbar": {"1"}, "logik": {"wohneinheit"},
	}
}

func TestSonderkostenErfassen_SpeichertUndZeigtInDerListe(t *testing.T) {
	mux, db := sonderkostenMux(t)
	c := loginCookies(t, mux, "vermieter", "geheim")
	w := postSonderkosten(t, mux, c, gueltigesSonderkostenForm())
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/sonderkosten" {
		t.Fatalf("status %d, Location %q, want 302 /sonderkosten", w.Code, w.Header().Get("Location"))
	}
	alle, err := store.AllSonderkosten(db)
	if err != nil || len(alle) != 1 {
		t.Fatalf("AllSonderkosten: %v (%d)", err, len(alle))
	}
	s := alle[0]
	if s.Monat != "2026-10-01" || s.Betrag != 1234.5 || !s.Umlegbar || s.Logik != store.LogikWohneinheit || s.Beschreibung != "Pumpe\nHeizkreis 2" {
		t.Errorf("gespeichert: %+v", s)
	}
	if body := getMitCookies(t, mux, "/sonderkosten", c).Body.String(); !strings.Contains(body, "Heizungspumpe") {
		t.Error("Liste zeigt den neuen Eintrag nicht")
	}
}

func TestSonderkostenErfassen_LogikVerschwindetBeiWohnungOderNichtUmlegbar(t *testing.T) {
	mux, db := sonderkostenMux(t)
	c := loginCookies(t, mux, "vermieter", "geheim")
	f := gueltigesSonderkostenForm()
	f.Set("umsetzung", "wohnung2") // das versteckte Logik-Feld wird mitgeschickt
	if w := postSonderkosten(t, mux, c, f); w.Code != http.StatusFound {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	alle, _ := store.AllSonderkosten(db)
	if len(alle) != 1 || alle[0].Logik != "" {
		t.Errorf("Logik bei Wohnung 2 soll leer sein: %+v", alle)
	}
}

func TestSonderkostenErfassen_UngueltigGibt400MitMeldung(t *testing.T) {
	mux, db := sonderkostenMux(t)
	c := loginCookies(t, mux, "vermieter", "geheim")
	for name, aendere := range map[string]func(url.Values){
		"Betrag 0":                       func(f url.Values) { f.Set("betrag", "0") },
		"Betrag keine Zahl":              func(f url.Values) { f.Set("betrag", "abc") },
		"Art unbekannt":                  func(f url.Values) { f.Set("art", "luxus") },
		"Haus umlegbar ohne Logik":       func(f url.Values) { f.Del("logik") },
		"Personen ohne Fixkosteneingabe": func(f url.Values) { f.Set("logik", "personen") },
		"Monat fehlt":                    func(f url.Values) { f.Del("monat") },
	} {
		t.Run(name, func(t *testing.T) {
			f := gueltigesSonderkostenForm()
			aendere(f)
			w := postSonderkosten(t, mux, c, f)
			if w.Code != http.StatusBadRequest || strings.TrimSpace(w.Body.String()) == "" {
				t.Errorf("status %d, body %q, want 400 mit Meldung", w.Code, w.Body.String())
			}
		})
	}
	if alle, _ := store.AllSonderkosten(db); len(alle) != 0 {
		t.Errorf("%d Einträge gespeichert, want 0", len(alle))
	}
}

func TestSonderkostenErfassen_PersonenMitFixkosteneingabe(t *testing.T) {
	mux, db := sonderkostenMux(t)
	if _, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	c := loginCookies(t, mux, "vermieter", "geheim")
	f := gueltigesSonderkostenForm()
	f.Set("logik", "personen")
	if w := postSonderkosten(t, mux, c, f); w.Code != http.StatusFound {
		t.Errorf("status %d: %s", w.Code, w.Body.String())
	}
	// Monat ohne Eingabe bleibt erlaubt (andere Logik).
	f = gueltigesSonderkostenForm()
	f.Set("monat", "2026-03")
	if w := postSonderkosten(t, mux, c, f); w.Code != http.StatusFound {
		t.Errorf("Monat ohne Fixkosteneingabe: status %d: %s", w.Code, w.Body.String())
	}
}

func TestSonderkostenErfassen_MaskeUndHinweisNurMitRecht(t *testing.T) {
	mux, db := sonderkostenMux(t)
	vermieter := loginCookies(t, mux, "vermieter", "geheim")
	mieter := loginCookies(t, mux, "mieter", "mieter")

	body := getMitCookies(t, mux, "/sonderkosten", vermieter).Body.String()
	for _, want := range []string{
		"Neue Sonderkosten", `<form method="post" action="/sonderkosten"`, "data-fehler-overlay",
		`name="betreff"`, `name="beschreibung"`, `name="rechnungsnummer"`, `name="umlegbar"`, `name="logik"`,
		"Hinweis: Reparaturen sind in der Regel keine umlegbaren Betriebskosten. Bitte vor dem Umlegen prüfen.",
		"Hinweis: Neuanschaffungen sind in der Regel keine umlegbaren Betriebskosten. Bitte vor dem Umlegen prüfen.",
		"Hinweis: Modernisierungen sind in der Regel keine umlegbaren Betriebskosten. Bitte vor dem Umlegen prüfen.",
		"Hinweis: Nicht jede sonstige Rechnung ist umlegbar. Bitte vor dem Umlegen prüfen.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Vermieter-Seite enthält %q nicht", want)
		}
	}
	if strings.Count(body, "Hinweis: ") != 4 {
		t.Error("Wartung bekommt keinen Hinweis, es sind genau vier Hinweistexte erwartet")
	}

	// Mieter ohne das Recht "erfassen": Seite ohne Button und Maske, POST abgelehnt.
	setzeRecht(t, db, store.RolleMieter, store.RechtSonderkostenAnsehen, true)
	if b := getMitCookies(t, mux, "/sonderkosten", mieter).Body.String(); strings.Contains(b, "Neue Sonderkosten") || strings.Contains(b, `name="betreff"`) {
		t.Error("Mieter ohne Recht sieht Button oder Maske")
	}
	if w := postSonderkosten(t, mux, mieter, gueltigesSonderkostenForm()); w.Code != http.StatusForbidden {
		t.Errorf("POST ohne Recht: status %d, want 403", w.Code)
	}
	if alle, _ := store.AllSonderkosten(db); len(alle) != 0 {
		t.Error("POST ohne Recht hat gespeichert")
	}

	// Mit dem Recht sieht der Mieter die Maske und darf speichern.
	setzeRecht(t, db, store.RolleMieter, store.RechtSonderkostenErfassen, true)
	if b := getMitCookies(t, mux, "/sonderkosten", mieter).Body.String(); !strings.Contains(b, "Neue Sonderkosten") {
		t.Error("Mieter mit Recht sieht den Button nicht")
	}
	if w := postSonderkosten(t, mux, mieter, gueltigesSonderkostenForm()); w.Code != http.StatusFound {
		t.Errorf("POST mit Recht: status %d", w.Code)
	}
}
