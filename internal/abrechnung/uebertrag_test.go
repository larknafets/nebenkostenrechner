package abrechnung

import (
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Übertrag fixtures: Wohnung 2 pays an Abschlag of 90 per month and bears a
// Fixkosten position of 100 per month, all meters stand still (no consumption
// cost), so every month with data has the balance -10.
const (
	uebertragAbschlag = 90.0
	uebertragKosten   = 100.0
)

// uebertragDaten builds the BerechnungsDaten by hand: complete Ablesungen from
// ablesungVon to Dezember 2027 and one Fixkosten-Eingabe per month from
// eingabeVon to Dezember 2027, except the months in ohneAblesung/ohneEingabe.
func uebertragDaten(ablesungVon, eingabeVon time.Time, ohneAblesung, ohneEingabe map[string]bool) BerechnungsDaten {
	apartments, haus := stammdatenOK()
	ende := time.Date(2027, time.December, 1, 0, 0, 0, 0, time.UTC)

	var periods []*store.LatestPeriod
	var id int64 = 1
	for m := ablesungVon; !m.After(ende); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		if ohneAblesung[key] {
			continue
		}
		periods = append(periods, vollePeriode(id, m.Format("2006-01")+"-28", key))
		id++
	}

	kostenpositionen := []store.Kostenposition{{ID: 1, Key: "grundsteuer", Label: "Grundsteuer", Umlagefaehig: true}}
	var details []*store.FixkostenEingabeDetails
	var summaries []store.FixkostenEingabeSummary
	id = 1
	for m := eingabeVon; !m.After(ende); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		if ohneEingabe[key] {
			continue
		}
		details = append(details, &store.FixkostenEingabeDetails{
			ID: id, Monat: key,
			Personen: map[int64]int64{1: 2, 2: 1},
			Werte:    map[int64]store.FixkostenPositionWert{1: {Logik: store.LogikWohnung2, Typ: store.TypMonatlich, Wert: uebertragKosten}},
			Abschlag: map[int64]float64{1: 0, 2: uebertragAbschlag},
		})
		summaries = append(summaries, store.FixkostenEingabeSummary{ID: id, Monat: key})
		id++
	}
	return BerechnungsDaten{
		Pruef:            Daten{Periods: periods, Eingaben: summaries, Apartments: apartments, Haus: haus},
		Kostenpositionen: kostenpositionen,
		Fixkosten:        calc.NewFixkostenreihe(calc.FixkostenDaten{Eingaben: details, Kostenpositionen: kostenpositionen, Apartments: apartments}),
		Kosten:           calc.New(calc.Daten{Periods: periods, Apartments: apartments, Haus: haus}),
	}
}

func monat(jahr int, m time.Month) time.Time {
	return time.Date(jahr, m, 1, 0, 0, 0, 0, time.UTC)
}

// TestBerechne_Uebertrag checks the Übertrag Vorjahre of the Jahresabrechnung
// 2027 of Wohnung 2 for different starts, without a database.
func TestBerechne_Uebertrag(t *testing.T) {
	oktober2026, dezember2026 := monat(2026, time.October), monat(2026, time.December)

	tests := []struct {
		name string
		// ablesungVon is the month of the first Ablesung, the Eingaben start
		// one month later (the Ausgangsstand month needs none).
		ablesungVon  time.Time
		ohneAblesung map[string]bool
		ohneEingabe  map[string]bool
		status       string
		mieterSeit   string
		want         Uebertrag
	}{
		{
			name:        "ab Erfassungsbeginn, Ausgangsstand-Monat ohne Zeile",
			ablesungVon: oktober2026, status: store.StatusEigennutzung,
			want: Uebertrag{Vorhanden: true, Betrag: -20, Von: "2026-11-01", Bis: "2026-12-01"},
		},
		{
			name:        "Mieter seit vor dem Erfassungsbeginn zaehlt nicht",
			ablesungVon: oktober2026, status: store.StatusVermietet, mieterSeit: "2026-01-01",
			want: Uebertrag{Vorhanden: true, Betrag: -20, Von: "2026-11-01", Bis: "2026-12-01"},
		},
		{
			name:        "Mieter seit im Vorjahr verkuerzt den Übertrag",
			ablesungVon: oktober2026, status: store.StatusVermietet, mieterSeit: "2026-12-01",
			want: Uebertrag{Vorhanden: true, Betrag: -10, Von: "2026-12-01", Bis: "2026-12-01"},
		},
		{
			name:        "Mieter seit bei Eigennutzung wird ignoriert",
			ablesungVon: oktober2026, status: store.StatusEigennutzung, mieterSeit: "2027-03-01",
			want: Uebertrag{Vorhanden: true, Betrag: -20, Von: "2026-11-01", Bis: "2026-12-01"},
		},
		{
			name:        "Mieter seit nach Beginn des Zeitraums: Hinweis statt Übertrag",
			ablesungVon: oktober2026, status: store.StatusVermietet, mieterSeit: "2027-03-01",
			want: Uebertrag{Hinweis: "Kein Übertrag: Mieter seit März 2027, also nach dem Beginn dieses Zeitraums."},
		},
		{
			name:        "Mieter seit am Beginn des Zeitraums: nichts davor",
			ablesungVon: oktober2026, status: store.StatusVermietet, mieterSeit: "2027-01-01",
			want: Uebertrag{},
		},
		{
			name:        "erste Ablesung im Dezember: nur der Ausgangsstand-Monat davor",
			ablesungVon: dezember2026, status: store.StatusEigennutzung,
			want: Uebertrag{},
		},
		{
			name:        "erste Ablesung im Januar: nichts davor",
			ablesungVon: monat(2027, time.January), status: store.StatusEigennutzung,
			want: Uebertrag{},
		},
		{
			name:        "Fixkosten-Eingabe fehlt im Vorjahr",
			ablesungVon: oktober2026, ohneEingabe: map[string]bool{"2026-12-01": true}, status: store.StatusEigennutzung,
			want: Uebertrag{Hinweis: "Übertrag nicht berechenbar: Fixkosten-Eingabe fehlt: Dezember 2026"},
		},
		{
			name:        "frueheste Mangel gewinnt",
			ablesungVon: oktober2026, ohneAblesung: map[string]bool{"2026-11-01": true}, ohneEingabe: map[string]bool{"2026-12-01": true}, status: store.StatusEigennutzung,
			want: Uebertrag{Hinweis: "Übertrag nicht berechenbar: Ablesung fehlt: November 2026"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			eingabeVon := tc.ablesungVon.AddDate(0, 1, 0)
			if tc.ablesungVon.Year() == 2027 {
				eingabeVon = tc.ablesungVon
			}
			d := uebertragDaten(tc.ablesungVon, eingabeVon, tc.ohneAblesung, tc.ohneEingabe)
			d.Pruef.Apartments[1].Status, d.Pruef.Apartments[1].MieterSeit = tc.status, tc.mieterSeit

			erg, err := Berechne(d, 2027, GanzesJahr, 2)
			if err != nil || erg.Abrechnung == nil {
				t.Fatalf("Berechne: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
			}
			got := erg.Abrechnung.Monatsverlauf.Uebertrag
			if got != tc.want {
				t.Fatalf("Uebertrag = %+v, want %+v", got, tc.want)
			}

			// The Monatsverlauf starts with the Übertrag and adds -10 per month
			// of 2027, so the Endsaldo is the Übertrag plus the Jahressaldo.
			v := erg.Abrechnung.Monatsverlauf
			jahressaldo := uebertragAbschlag*12 - uebertragKosten*12
			if want := got.Betrag + jahressaldo; v.Endsaldo != want {
				t.Errorf("Endsaldo = %v, want Übertrag %v + Jahressaldo %v = %v", v.Endsaldo, got.Betrag, jahressaldo, want)
			}
			if erg.Abrechnung.Saldo.Wert() != jahressaldo {
				t.Errorf("Jahressaldo = %v, want %v", erg.Abrechnung.Saldo.Wert(), jahressaldo)
			}
			if len(v.Zeilen) != 12 {
				t.Fatalf("Zeilen = %d, want 12", len(v.Zeilen))
			}
			if want := got.Betrag - 10; v.Zeilen[0].Saldo != want {
				t.Errorf("Saldo of January = %v, want Übertrag %v - 10 = %v", v.Zeilen[0].Saldo, got.Betrag, want)
			}
		})
	}
}
