package abrechnung

import (
	"reflect"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestBerechne_SonderkostenZeileUndAnlage: the Kostenübersicht line and the
// Anlage list the umlegbar entries with a share above 0 for the apartment;
// the line's Betrag is the Sonderkosten part of the Betriebskosten (#407).
func TestBerechne_SonderkostenZeileUndAnlage(t *testing.T) {
	list := []store.Sonderkosten{
		{Monat: "2027-03-01", Betreff: "Pumpe", Art: store.ArtReparatur, Firma: "Fa. Müller", Rechnungsnummer: "R-1", Betrag: 100.01, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikQM, Beschreibung: "geheim"},
		{Monat: "2027-04-01", Betreff: "Spüle", Art: store.ArtNeuanschaffung, Betrag: 40, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true},
		{Monat: "2027-05-01", Betreff: "Nur W1", Betrag: 500, Umsetzung: store.UmsetzungWohnung1, Umlegbar: true},
		{Monat: "2027-06-01", Betreff: "Nicht umlegbar", Betrag: 999, Umsetzung: store.UmsetzungHaus, Logik: store.LogikQM},
		{Monat: "2026-12-01", Betreff: "Vorjahr", Betrag: 77, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true},
	}
	ab := berechneSonderkosten(t, list, store.StatusVermietet, 2)

	z := ab.Sonderkosten
	if z == nil {
		t.Fatal("Sonderkosten-Zeile fehlt")
	}
	// 88:42 of 100.01 -> W2 32.31 (rest), plus 40.
	if z.Gesamt != 140.01 || z.Betrag != 72.31 || z.Schluessel != "Nach Eintrag" || z.Position != "Sonderkosten" {
		t.Errorf("Zeile = %+v, want Gesamt 140.01, Betrag 72.31, Schlüssel Nach Eintrag", *z)
	}
	if got, want := ab.Betriebskosten, 1200+z.Betrag; got != want {
		t.Errorf("Betriebskosten = %v, want %v (Zeile zählt einmal)", got, want)
	}

	want := []SonderkostenEintrag{
		{Monat: "2027-03-01", Betreff: "Pumpe", Art: store.ArtReparatur, Firma: "Fa. Müller", Rechnungsnummer: "R-1", Umsetzung: store.UmsetzungHaus, Rechnungsbetrag: 100.01, Schluessel: "Je anteilige Wohnungsgröße", Anteil: 32.31},
		{Monat: "2027-04-01", Betreff: "Spüle", Art: store.ArtNeuanschaffung, Umsetzung: store.UmsetzungWohnung2, Rechnungsbetrag: 40, Schluessel: "Wohnung 2", Anteil: 40},
	}
	if !reflect.DeepEqual(ab.SonderkostenAnlage, want) {
		t.Errorf("Anlage = %+v, want %+v", ab.SonderkostenAnlage, want)
	}

	t.Run("ohne Einträge im Zeitraum weder Zeile noch Anlage", func(t *testing.T) {
		ab := berechneSonderkosten(t, list[2:4], store.StatusVermietet, 2)
		if ab.Sonderkosten != nil || ab.SonderkostenAnlage != nil {
			t.Errorf("Zeile %v, Anlage %v, want nil", ab.Sonderkosten, ab.SonderkostenAnlage)
		}
	})
}
