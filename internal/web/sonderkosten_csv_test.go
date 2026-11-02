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

// A free text that would run as a spreadsheet formula gets a leading ' in the
// export, the import takes it off again.
func TestSonderkostenCsv_FormelTextWirdEntschaerft(t *testing.T) {
	for _, text := range []string{"=HYPERLINK(\"http://x\")", "+1", "-1", "@SUM(A1)"} {
		row := formatSonderkostenCSVRow(store.Sonderkosten{Betreff: text, Art: store.ArtReparatur, Umsetzung: store.UmsetzungHaus, Betrag: 1})
		if row[1] != "'"+text {
			t.Errorf("Betreff %q wird als %q exportiert", text, row[1])
		}
		if got := csvTextLesen(row[1]); got != text {
			t.Errorf("Import von %q ergibt %q, want %q", row[1], got, text)
		}
	}
	if got := csvText("Heizungspumpe"); got != "Heizungspumpe" {
		t.Errorf("normaler Text verändert: %q", got)
	}
	if got := csvTextLesen("'normal"); got != "'normal" {
		t.Errorf("ein Apostroph am Anfang eines normalen Texts wird entfernt: %q", got)
	}
}
