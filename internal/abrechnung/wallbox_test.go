package abrechnung

import (
	"reflect"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// wallboxDaten builds the Übertrag fixture (Ablesungen Oktober 2026 to
// Dezember 2027, Wohnung 2 pays an Abschlag of 90 and bears a Fixkosten
// position of 100, Wohnung 1 neither) and gives every month after the
// Ausgangsstand the consumption: Zwischenzähler Wallboxen 40 kWh, Einzelzähler Wohnung 2 20 kWh,
// Einspeisung 10 kWh. At 0,22 EUR/kWh and 0,08 EUR/kWh Einspeisung that is per
// month: Wallbox Wohnung 2 4,40 EUR (20 kWh), Wallbox Wohnung 1 the rest 4,40
// EUR (20 kWh), PV-Gutschrift 0,80 EUR; per year 52,80 EUR, 52,80 EUR, 9,60 EUR.
func wallboxDaten(statusW1, statusW2 string, wallboxWeiterberechnen, pvSchalter bool) BerechnungsDaten {
	d := uebertragDaten(monat(2026, time.October), monat(2026, time.November), nil, nil)
	for k, p := range d.Pruef.Periods {
		p.Readings["strom_gesamt"] = 100 * float64(k)
		p.Readings["strom_wallbox"] = 40 * float64(k)
		p.Readings["strom_einspeisung"] = 10 * float64(k)
		p.Readings[store.MeterKeyWallboxWohnung2] = 20 * float64(k)
		p.PVGutschriftW1 = pvSchalter
	}
	d.Pruef.Apartments[0].Status, d.Pruef.Apartments[1].Status = statusW1, statusW2
	// A rented apartment needs its tenant data, or the Prüfung finds a Mangel.
	d.Pruef.Apartments[0].MieterName, d.Pruef.Apartments[0].MieterAnschrift = "Paul Beispiel", "Beispielweg 1\n12345 Musterstadt"
	d.Pruef.Haus.WallboxWeiterberechnen = wallboxWeiterberechnen
	d.Kosten = calc.New(calc.Daten{Periods: d.Pruef.Periods, Apartments: d.Pruef.Apartments, Haus: d.Pruef.Haus})
	return d
}

// TestBerechne_Statustabelle runs the table of the Wohnungsstatus (Block
// "Wallbox" and "PV-Gutschrift" per status, flag and switch) and checks that
// the Jahressaldo and the Endsaldo of the Monatsverlauf count exactly the
// shown items: Vorauszahlungen minus Betriebskosten minus Wallbox plus
// PV-Gutschrift.
func TestBerechne_Statustabelle(t *testing.T) {
	eigen, miete := store.StatusEigennutzung, store.StatusVermietet

	tests := []struct {
		name           string
		statusW1       string
		statusW2       string
		flag, schalter bool
		wohnung        int64
		wantWallbox    bool
		wantPV         bool
	}{
		{"Wohnung 2 vermietet, Flag an", eigen, miete, true, true, 2, true, false},
		{"Wohnung 2 vermietet, Flag aus", eigen, miete, false, true, 2, false, false},
		{"Wohnung 2 Eigennutzung, Flag aus gilt nicht", eigen, eigen, false, true, 2, true, false},
		{"Wohnung 1 Eigennutzung, Flag aus, Schalter an", eigen, miete, false, true, 1, true, true},
		{"Wohnung 1 Eigennutzung, Flag an, Schalter aus", eigen, miete, true, false, 1, true, false},
		{"Wohnung 1 vermietet, Flag an: Wallbox ja, Gutschrift nie", miete, miete, true, true, 1, true, false},
		{"Wohnung 1 vermietet, Flag aus: nichts", miete, miete, false, true, 1, false, false},
		{"beide vermietet, Flag aus: Wohnung 2 ohne Wallbox", miete, miete, false, true, 2, false, false},
		{"Wohnung 1 vermietet, Wohnung 2 Eigennutzung: Gutschrift folgt Wohnung 1", miete, eigen, false, true, 2, true, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := wallboxDaten(tc.statusW1, tc.statusW2, tc.flag, tc.schalter)
			erg, err := Berechne(d, 2027, GanzesJahr, tc.wohnung)
			if err != nil || erg.Abrechnung == nil {
				t.Fatalf("Berechne: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
			}
			ab := erg.Abrechnung

			wantSaldo := 0.0 // Wohnung 1: no Abschlag, no Fixkosten
			if tc.wohnung == 2 {
				wantSaldo = 90*12 - 100*12
			}
			if tc.wantWallbox {
				if ab.Wallbox == nil || ab.Wallbox.Kosten != 52.80 || ab.Wallbox.VerbrauchKWh != 240 {
					t.Fatalf("Wallbox = %+v, want 240 kWh and 52,80 EUR", ab.Wallbox)
				}
				wantSaldo -= 52.80
			} else if ab.Wallbox != nil {
				t.Fatalf("Wallbox = %+v, want none", ab.Wallbox)
			}
			if tc.wantPV {
				if ab.PVGutschrift == nil || ab.PVGutschrift.Betrag != 9.60 {
					t.Fatalf("PVGutschrift = %+v, want 9,60 EUR", ab.PVGutschrift)
				}
				wantSaldo += 9.60
			} else if ab.PVGutschrift != nil {
				t.Fatalf("PVGutschrift = %+v, want none", ab.PVGutschrift)
			}

			if got := ab.Saldo.Wert(); !gleich(got, wantSaldo) {
				t.Errorf("Jahressaldo = %v, want %v", got, wantSaldo)
			}
			v := ab.Monatsverlauf
			// Übertrag: November and Dezember 2026, same items per month.
			wantUebertrag := 0.0
			if tc.wohnung == 2 {
				wantUebertrag = 2 * (90 - 100)
			}
			if tc.wantWallbox {
				wantUebertrag -= 2 * 4.40
			}
			if tc.wantPV {
				wantUebertrag += 2 * 0.80
			}
			if !gleich(v.Uebertrag.Betrag, wantUebertrag) {
				t.Errorf("Übertrag = %v, want %v", v.Uebertrag.Betrag, wantUebertrag)
			}
			if want := v.Uebertrag.Betrag + wantSaldo; !gleich(v.Endsaldo, want) {
				t.Errorf("Endsaldo = %v, want Übertrag %v + Jahressaldo %v = %v", v.Endsaldo, v.Uebertrag.Betrag, wantSaldo, want)
			}
		})
	}
}

// einzelzaehlerMeters are the required meters of the Anlagen in these tests.
var einzelzaehlerMeters = []store.Meter{
	{Key: "strom_gesamt", Label: "Stromzähler Gesamt", Unit: "kWh"},
	{Key: "strom_wallbox", Label: "Zwischenzähler Wallboxen", Unit: "kWh"},
	{Key: "wasser_gesamt", Label: "Wasserzähler Gesamt", Unit: "m3"},
}

func spaltenNamen(z Zaehlerstaende) []string {
	var out []string
	for _, s := range z.Spalten {
		out = append(out, s.Zaehler)
	}
	return out
}

func TestBerechne_EinzelzaehlerInDenAnlagen(t *testing.T) {
	berechne := func(t *testing.T, d BerechnungsDaten) *Jahresabrechnung {
		t.Helper()
		d.Meters = einzelzaehlerMeters
		erg, err := Berechne(d, 2027, GanzesJahr, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("Berechne: err %v, Maengel %v (Einzelzähler dürfen nie ein Mangel sein)", err, maengelTexte(erg.Pruefung))
		}
		return erg.Abrechnung
	}

	t.Run("beide Einzelzähler stehen direkt unter Wallboxen", func(t *testing.T) {
		d := wallboxDaten(store.StatusEigennutzung, store.StatusVermietet, true, false)
		for _, p := range d.Pruef.Periods {
			p.Readings[store.MeterKeyWallboxWohnung1] = p.Readings[store.MeterKeyWallboxWohnung2] * 2
		}
		ab := berechne(t, d)
		want := []string{"Strom gesamt", "Wallboxen", "Wallbox Whg 1", "Wallbox Whg 2", "Wasser gesamt"}
		if got := spaltenNamen(ab.Zaehlerstaende); !reflect.DeepEqual(got, want) {
			t.Errorf("Spalten = %v, want %v", got, want)
		}
		var zeilen []string
		for _, v := range ab.Verbrauch {
			zeilen = append(zeilen, v.Zaehler)
		}
		if want := []string{"Stromzähler Gesamt", "Zwischenzähler Wallboxen", "Wallbox Wohnung 1", "Wallbox Wohnung 2", "Wasserzähler Gesamt"}; !reflect.DeepEqual(zeilen, want) {
			t.Errorf("Verbrauchsübersicht = %v, want %v", zeilen, want)
		}
		// Wohnung 2 is settled: its Einzelzähler is the own one.
		for i, s := range ab.Zaehlerstaende.Spalten {
			if want := s.Zaehler == "Wallbox Whg 2"; s.Eigen != want {
				t.Errorf("Spalte %d %s: Eigen = %v, want %v", i, s.Zaehler, s.Eigen, want)
			}
		}
	})

	t.Run("fehlender Stand wird als fehlend markiert, nicht als 0", func(t *testing.T) {
		d := wallboxDaten(store.StatusEigennutzung, store.StatusVermietet, true, false)
		delete(d.Pruef.Periods[2].Readings, store.MeterKeyWallboxWohnung2) // Dezember 2026, the Ausgangsstand of 2027
		ab := berechne(t, d)
		if got := spaltenNamen(ab.Zaehlerstaende); !reflect.DeepEqual(got, []string{"Strom gesamt", "Wallboxen", "Wallbox Whg 2", "Wasser gesamt"}) {
			t.Fatalf("Spalten = %v", got)
		}
		erste, zweite := ab.Zaehlerstaende.Zeilen[0], ab.Zaehlerstaende.Zeilen[1]
		if !erste.Staende[2].Fehlt || zweite.Staende[2].Fehlt {
			t.Errorf("Fehlt: Ausgangsstand %v, nächste Ablesung %v, want true/false", erste.Staende[2].Fehlt, zweite.Staende[2].Fehlt)
		}
		for _, v := range ab.Verbrauch {
			if v.Zaehler == "Wallbox Wohnung 2" && (!v.BeginnFehlt || v.EndeFehlt) {
				t.Errorf("Verbrauchsübersicht Wallbox Wohnung 2: BeginnFehlt %v EndeFehlt %v, want true/false", v.BeginnFehlt, v.EndeFehlt)
			}
		}
	})

	t.Run("ohne jeden Stand im Zeitraum entfallen die Zeilen", func(t *testing.T) {
		d := wallboxDaten(store.StatusEigennutzung, store.StatusVermietet, true, false)
		for _, p := range d.Pruef.Periods {
			delete(p.Readings, store.MeterKeyWallboxWohnung2)
		}
		ab := berechne(t, d)
		if got := spaltenNamen(ab.Zaehlerstaende); !reflect.DeepEqual(got, []string{"Strom gesamt", "Wallboxen", "Wasser gesamt"}) {
			t.Errorf("Spalten = %v, want without Einzelzähler", got)
		}
		if len(ab.Verbrauch) != len(einzelzaehlerMeters) {
			t.Errorf("Verbrauchsübersicht hat %d Zeilen, want %d", len(ab.Verbrauch), len(einzelzaehlerMeters))
		}
	})
}

func gleich(a, b float64) bool {
	d := a - b
	return d > -0.005 && d < 0.005
}
