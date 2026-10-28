package abrechnung

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// TestBerechne_StromTabelle runs the Strom row of the Wohnungsstatus table
// (#280): Eigennutzung always shows the apartment's Strom (the flag does not
// apply), Vermietung only with "Stromverbrauch weiterberechnen", for Wohnung 1
// and Wohnung 2 alike. The Jahressaldo counts it only then.
func TestBerechne_StromTabelle(t *testing.T) {
	eigen, miete := store.StatusEigennutzung, store.StatusVermietet
	tests := []struct {
		name      string
		statusW1  string
		statusW2  string
		flag      bool
		wohnung   int64
		wantStrom bool
	}{
		{"Wohnung 1 Eigennutzung, Flag aus: Strom ja", eigen, miete, false, 1, true},
		{"Wohnung 1 Eigennutzung, Flag an: Strom ja", eigen, miete, true, 1, true},
		{"Wohnung 2 vermietet, Flag aus: kein Strom", eigen, miete, false, 2, false},
		{"Wohnung 2 vermietet, Flag an: Strom ja", eigen, miete, true, 2, true},
		{"Wohnung 2 Eigennutzung, Flag aus: Strom ja", eigen, eigen, false, 2, true},
		{"Wohnung 1 vermietet, Flag aus: kein Strom", miete, miete, false, 1, false},
		{"Wohnung 1 vermietet, Flag an: Strom ja", miete, miete, true, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := wallboxDaten(tc.statusW1, tc.statusW2, true, true)
			d.Pruef.Haus.StromWeiterberechnen = tc.flag
			d.Kosten = verbrauchskostenVon(d)
			erg, err := Berechne(d, 2027, GanzesJahr, tc.wohnung)
			if err != nil || erg.Abrechnung == nil {
				t.Fatalf("Berechne: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
			}
			ab := erg.Abrechnung
			if (ab.Strom != nil) != tc.wantStrom {
				t.Fatalf("Strom = %+v, want shown %v", ab.Strom, tc.wantStrom)
			}
			want := ab.Vorauszahlungen - ab.Betriebskosten
			if ab.Strom != nil {
				// Only Wohnung 1 has Strom costs in this fixture (no
				// Zwischenzähler for Wohnung 2): the rest, 13,20 EUR a month.
				if tc.wohnung == 1 && !gleich(ab.Strom.Kosten, 12*13.20) {
					t.Fatalf("Strom.Kosten = %v, want 158,40", ab.Strom.Kosten)
				}
				want -= ab.Strom.Kosten
			}
			if ab.Wallbox != nil {
				want -= ab.Wallbox.Kosten
			}
			if ab.PVGutschrift != nil {
				want += ab.PVGutschrift.Betrag
			}
			if got := ab.Saldo.Wert(); !gleich(got, want) {
				t.Errorf("Jahressaldo = %v, want %v", got, want)
			}
		})
	}
}
