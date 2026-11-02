package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestSonderkostenExport_AlleEintraegeNachMonat(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	w := getMitCookies(t, mux, "/sonderkosten/export.csv", c)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, `"sonderkosten.csv"`) {
		t.Errorf("Content-Disposition = %q", cd)
	}
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(w.Body.String(), "\uFEFF")), "\n")
	want := []string{
		"monat;betreff;beschreibung;art;firma;rechnungsnummer;betrag;umsetzung;umlegbar;logik",
		"2025-03-01;Wasserhahn;;Reparatur;Sanitär Lehmann;;184,3;Wohnung 1;0;",
		"2026-09-01;Spülmaschine;;Neuanschaffung;Elektro Schmitt;;649;Wohnung 2;1;",
		"2026-10-01;Heizungspumpe;Pumpe Heizkreis 2;Reparatur;Meier GmbH;RE-412;1.000;Haus;1;wohneinheit",
		"2026-10-01;Rauchmelder;;Wartung;Brandschutz Nord;;96;Haus;0;",
	}
	if len(lines) != len(want) {
		t.Fatalf("%d Zeilen, want %d:\n%s", len(lines), len(want), w.Body.String())
	}
	for i := range want {
		if strings.TrimSpace(lines[i]) != want[i] {
			t.Errorf("Zeile %d = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestSonderkostenExport_NurVermieter(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	m := loginCookies(t, mux, "mieter", "mieter")
	if w := getMitCookies(t, mux, "/sonderkosten/export.csv", m); w.Code == http.StatusOK {
		t.Errorf("Mieter:in erhält den Export (status %d)", w.Code)
	}
	if body := getMitCookies(t, mux, "/sonderkosten", m).Body.String(); strings.Contains(body, "export.csv") {
		t.Error("Mieter:in sieht den Export-Button")
	}
	v := loginCookies(t, mux, "vermieter", "geheim")
	if body := getMitCookies(t, mux, "/sonderkosten", v).Body.String(); !strings.Contains(body, "/sonderkosten/export.csv") {
		t.Error("Vermieter:in sieht den Export-Button nicht")
	}
}

func TestSonderkostenCsv_BezeichnungenRoundtrip(t *testing.T) {
	for _, a := range store.SonderkostenArten {
		if got, ok := sonderkostenArtSchluessel(sonderkostenArtLabel(a.Wert)); !ok || got != a.Wert {
			t.Errorf("Art %q: %q, %v", a.Wert, got, ok)
		}
	}
	for _, u := range store.SonderkostenUmsetzungen {
		if got, ok := sonderkostenUmsetzungSchluessel(sonderkostenUmsetzungLabel(u.Wert)); !ok || got != u.Wert {
			t.Errorf("Umsetzung %q: %q, %v", u.Wert, got, ok)
		}
	}
	if _, ok := sonderkostenArtSchluessel("Unbekannt"); ok {
		t.Error("unbekannte Art wird akzeptiert")
	}
}
