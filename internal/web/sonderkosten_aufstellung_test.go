package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// aufstellung returns the HTML of the table block under the sum cards, "" without one.
func aufstellung(body string) string {
	i := strings.Index(body, `class="panel sk-aufstellung"`)
	if i < 0 {
		return ""
	}
	j := strings.Index(body[i:], `</details>`)
	if j < 0 {
		return body[i:]
	}
	return body[i : i+j]
}

func TestSonderkosten_AufstellungEinJahr(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	a := aufstellung(getMitCookies(t, mux, "/sonderkosten?jahr=2026", c).Body.String())
	// 2026: Reparatur 1000 Haus, Wartung 96 Haus, Neuanschaffung 649 Wohnung 2; Summenzeile Haus 1096.
	for _, want := range []string{"Reparatur", "Wartung", "Neuanschaffung", "1.000,00", "649,00", "1.096,00", ">Summe<"} {
		if !strings.Contains(a, want) {
			t.Errorf("Aufstellung enthält %q nicht: %s", want, a)
		}
	}
	for _, not := range []string{"Modernisierung", "Sonstiges", "Zwischensumme", "184,30", "2025"} {
		if strings.Contains(a, not) {
			t.Errorf("Aufstellung enthält %q, soll nicht: %s", not, a)
		}
	}
	if strings.Count(a, "<table") != 1 {
		t.Error("ein Jahr: genau eine Tabelle")
	}
	// 3 Arten x 3 Objekte, keine Summenspalte (Smartphone).
	if n := strings.Count(a, `<td style="text-align:right">`); n != 9 {
		t.Errorf("Betragszellen = %d, want 9 ohne Summenspalte", n)
	}
}

// The Summen block (cards and table) starts closed.
func TestSonderkosten_SummenBlockStartetEingeklappt(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	body := getMitCookies(t, mux, "/sonderkosten?jahr=2026", c).Body.String()
	i := strings.Index(body, `<details id="sk-summen">`)
	if i < 0 {
		t.Fatal("kein einklappbarer Summen-Block, oder er ist offen")
	}
	block := body[i : i+strings.Index(body[i:], "</details>")]
	for _, want := range []string{"<summary><strong>Summen</strong></summary>", "Gesamt", "davon umlegbar", "trägt Vermieter:in", "sk-aufstellung"} {
		if !strings.Contains(block, want) {
			t.Errorf("Summen-Block enthält %q nicht", want)
		}
	}
}

func TestSonderkosten_AufstellungAlleJahreMitBloeckenNeuestesZuerst(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	body := getMitCookies(t, mux, "/sonderkosten?jahr=alle", c).Body.String()
	if n := strings.Count(body, "Zwischensumme"); n != 2 {
		t.Fatalf("Zwischensumme %d mal, want 2", n)
	}
	i26, i25 := strings.Index(body, "<strong>2026</strong>"), strings.Index(body, "<strong>2025</strong>")
	if i26 < 0 || i25 < 0 || i26 > i25 {
		t.Errorf("Jahreskopf 2026 at %d, 2025 at %d, want newest first", i26, i25)
	}
	if !strings.Contains(body, "184,30") {
		t.Error("Block 2025 fehlt")
	}
}

// With an Objekt filter all four columns stay, the others show 0,00 €.
func TestSonderkosten_AufstellungObjektfilterBehaeltSpalten(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	a := aufstellung(getMitCookies(t, mux, "/sonderkosten?jahr=alle&objekt=wohnung2", c).Body.String())
	for _, want := range []string{">Haus<", ">Wohnung 1<", ">Wohnung 2<", ">Zwischensumme<", "0,00", "649,00", "Neuanschaffung"} {
		if !strings.Contains(a, want) {
			t.Errorf("Aufstellung enthält %q nicht: %s", want, a)
		}
	}
	if strings.Contains(a, "Reparatur") || strings.Contains(a, "1.000,00") {
		t.Error("andere Objekte und Arten sollen fehlen")
	}
}

func TestSonderkosten_AufstellungArtfilterUndLeer(t *testing.T) {
	mux, _ := sonderkostenMux(t, beispielSonderkosten...)
	c := loginCookies(t, mux, "vermieter", "geheim")
	a := aufstellung(getMitCookies(t, mux, "/sonderkosten?art=wartung", c).Body.String())
	if !strings.Contains(a, "Wartung") || strings.Contains(a, "Reparatur") {
		t.Errorf("Artfilter: %s", a)
	}
	w := getMitCookies(t, mux, "/sonderkosten?jahr=2025&art=wartung", c)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Keine Sonderkosten für diese Auswahl.") || strings.Contains(w.Body.String(), "<table") {
		t.Error("ohne Einträge: Meldung, keine Tabelle")
	}
}

// An entry without Fixkosteneingabe (not berechenbar) counts with its invoice amount.
func TestSonderkosten_AufstellungZaehltNichtBerechenbare(t *testing.T) {
	mux, db := sonderkostenMux(t)
	sonderkostenRoh(t, db, store.Sonderkosten{Monat: "2026-02-01", Betreff: "Dach", Art: store.ArtSonstiges, Firma: "X", Betrag: 321, Objekt: store.ObjektHaus, Umlegbar: true, Logik: store.LogikPersonen})
	c := loginCookies(t, mux, "vermieter", "geheim")
	body := getMitCookies(t, mux, "/sonderkosten?jahr=2026", c).Body.String()
	if !strings.Contains(body, "nicht berechenbar") || !strings.Contains(aufstellung(body), "321,00") {
		t.Error("nicht berechenbarer Eintrag soll mit dem Rechnungsbetrag in der Tabelle stehen")
	}
}
