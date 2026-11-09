package abrechnung

import (
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Without Sonderkosten the fixtures of uebertragDaten give Wohnung 2 a
// Jahressaldo 2027 of 12*90 - 12*100 = -120 and a Übertrag Vorjahre of -20.
const (
	sonderkostenJahressaldo = -120.0
	sonderkostenUebertrag   = -20.0
)

func hausSonderkosten(monat string, betrag float64, umlegbar bool, logik string) store.Sonderkosten {
	return store.Sonderkosten{Monat: monat, Betreff: "Rechnung", Art: store.ArtReparatur, Betrag: betrag, Objekt: store.ObjektHaus, Umlegbar: umlegbar, Logik: logik}
}

func berechneSonderkosten(t *testing.T, list []store.Sonderkosten, status string, apartmentID int64) *Jahresabrechnung {
	t.Helper()
	d := uebertragDaten(monat(2026, time.October), monat(2026, time.November), nil, nil)
	d.Pruef.Apartments[1].Status = status
	d.Sonderkosten = list
	erg, err := Berechne(d, 2027, GanzesJahr, apartmentID)
	if err != nil || erg.Abrechnung == nil {
		t.Fatalf("Berechne: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
	}
	return erg.Abrechnung
}

// TestBerechne_Sonderkosten: an umlegbar entry counts as a further Fixkosten
// of its month, the same number in Betriebskosten, Jahressaldo, Monatsverlauf
// and Übertrag, independent of the Wohnungsstatus.
func TestBerechne_Sonderkosten(t *testing.T) {
	for _, status := range []string{store.StatusVermietet, store.StatusEigennutzung} {
		t.Run("Haus je Wohnungsgröße, Rest an Wohnung 2, "+status, func(t *testing.T) {
			// 88:42 of 100.01 -> W1 67.7 (rounded), W2 32.31 (rest).
			list := []store.Sonderkosten{hausSonderkosten("2027-03-01", 100.01, true, store.LogikQM)}
			ab := berechneSonderkosten(t, list, status, 2)
			w2 := 32.31
			if got, want := ab.Betriebskosten, 12*100+w2; got != want {
				t.Errorf("Betriebskosten W2 = %v, want %v", got, want)
			}
			if got, want := ab.Saldo.Wert(), sonderkostenJahressaldo-w2; got != want {
				t.Errorf("Jahressaldo W2 = %v, want %v", got, want)
			}
			v := ab.Monatsverlauf
			if got, want := v.Zeilen[2].Fixkosten, 100+w2; got != want {
				t.Errorf("Fixkosten März = %v, want %v", got, want)
			}
			if got, want := v.Endsaldo, sonderkostenUebertrag+sonderkostenJahressaldo-w2; got != want {
				t.Errorf("Endsaldo = %v, want %v", got, want)
			}
		})
	}

	t.Run("Wohnung 1 und 2 zusammen tragen genau den Rechnungsbetrag", func(t *testing.T) {
		list := []store.Sonderkosten{hausSonderkosten("2027-03-01", 100.01, true, store.LogikWohneinheit)}
		w1 := berechneSonderkosten(t, list, store.StatusEigennutzung, 1)
		w2 := berechneSonderkosten(t, list, store.StatusEigennutzung, 2)
		if w1.Betriebskosten != 50.01 || calc.Round2(w2.Betriebskosten-12*100) != 50 {
			t.Errorf("Wohnung 1 %v (want 50.01, gerundet), Wohnung 2 %v (want 1250, Rest)", w1.Betriebskosten, w2.Betriebskosten)
		}
	})

	t.Run("Wohnung 1 oder 2 trägt 100 Prozent", func(t *testing.T) {
		s := hausSonderkosten("2027-03-01", 80, true, "")
		s.Objekt = store.ObjektWohnung1
		if ab := berechneSonderkosten(t, []store.Sonderkosten{s}, store.StatusVermietet, 2); ab.Betriebskosten != 1200 {
			t.Errorf("Wohnung 2 bei Objekt Wohnung 1 = %v, want 1200", ab.Betriebskosten)
		}
		if ab := berechneSonderkosten(t, []store.Sonderkosten{s}, store.StatusEigennutzung, 1); ab.Betriebskosten != 80 {
			t.Errorf("Wohnung 1 bei Objekt Wohnung 1 = %v, want 80", ab.Betriebskosten)
		}
		s.Objekt = store.ObjektWohnung2
		if ab := berechneSonderkosten(t, []store.Sonderkosten{s}, store.StatusVermietet, 2); ab.Betriebskosten != 1280 {
			t.Errorf("Wohnung 2 bei Objekt Wohnung 2 = %v, want 1280", ab.Betriebskosten)
		}
	})

	t.Run("Personen: Anteil nach der Eingabe des Monats (2:1)", func(t *testing.T) {
		list := []store.Sonderkosten{hausSonderkosten("2027-03-01", 90, true, store.LogikPersonen)}
		ab := berechneSonderkosten(t, list, store.StatusVermietet, 2)
		if ab.Betriebskosten != 1230 {
			t.Errorf("Betriebskosten W2 = %v, want 1230", ab.Betriebskosten)
		}
	})

	t.Run("nicht umlegbar ändert nirgends etwas", func(t *testing.T) {
		list := []store.Sonderkosten{
			hausSonderkosten("2027-03-01", 100, false, ""),
			{Monat: "2026-12-01", Betrag: 55, Objekt: store.ObjektWohnung2},
		}
		ab := berechneSonderkosten(t, list, store.StatusVermietet, 2)
		if ab.Betriebskosten != 1200 || ab.Saldo.Wert() != sonderkostenJahressaldo || ab.Monatsverlauf.Endsaldo != sonderkostenUebertrag+sonderkostenJahressaldo || ab.Monatsverlauf.Uebertrag.Betrag != sonderkostenUebertrag {
			t.Errorf("Betriebskosten %v, Saldo %v, Endsaldo %v, Übertrag %v", ab.Betriebskosten, ab.Saldo.Wert(), ab.Monatsverlauf.Endsaldo, ab.Monatsverlauf.Uebertrag.Betrag)
		}
	})

	t.Run("Vorjahr zählt in den Übertrag, Monat nur mit Sonderkosten bekommt eine Zeile", func(t *testing.T) {
		// 2026-10 is the month of the first Ablesung (Ausgangsstand): no Eingabe,
		// no consumption, and still no "Fixkosten fehlen".
		list := []store.Sonderkosten{
			{Monat: "2026-10-01", Betrag: 40, Objekt: store.ObjektWohnung2, Umlegbar: true},
			hausSonderkosten("2026-10-01", 70, true, store.LogikPersonen), // no Eingabe in the month: not berechenbar
		}
		ab := berechneSonderkosten(t, list, store.StatusVermietet, 2)
		u := ab.Monatsverlauf.Uebertrag
		if !u.Vorhanden || u.Betrag != sonderkostenUebertrag-40 || u.Von != "2026-10-01" {
			t.Errorf("Übertrag = %+v, want Betrag %v ab 2026-10-01", u, sonderkostenUebertrag-40)
		}
		if got, want := ab.Monatsverlauf.Endsaldo, sonderkostenUebertrag-40+sonderkostenJahressaldo; got != want {
			t.Errorf("Endsaldo = %v, want %v", got, want)
		}
	})
}
