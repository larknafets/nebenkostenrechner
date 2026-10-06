package web

import (
	"database/sql"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func near(a, b float64) bool { return math.Abs(a-b) < 0.0051 }

// abrechnungDelta is what every Ablesung of the test scenario adds to the
// meters (so each month has the same consumption).
var abrechnungDelta = map[string]float64{
	"strom_gesamt": 1842, "strom_wohnung2": 612, "strom_waermepumpe": 984,
	"waerme_wohnung1": 0.6, "waerme_wohnung2": 0.4,
	"wasser_gesamt": 10, "wasser_wohnung2": 3, "wasser_warmwasseraufbereitung": 1,
}

// legeAblesungAn creates the k-th Ablesung (k >= 0, cumulative readings).
func legeAblesungAn(t *testing.T, db *sql.DB, k int, datum, monat string) {
	t.Helper()
	readings := make(map[string]float64, len(store.MeterKeys))
	for _, key := range store.MeterKeys {
		readings[key] = abrechnungDelta[key] * float64(k)
	}
	if _, err := store.CreatePeriod(db, store.PeriodInput{
		ReadingDate: datum, Monat: monat,
		Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
		Readings: readings, Personen: map[int64]int64{1: 2, 2: 1},
	}); err != nil {
		t.Fatalf("CreatePeriod %s: %v", datum, err)
	}
}

// abrechnungStammdaten sets complete Stammdaten: Wohnung 1 100 m², Wohnung
// 2 50 m² (a 2:1 split by size), Wohnung 2 vermietet, Strom passed on.
func abrechnungStammdaten(t *testing.T, db *sql.DB, stromWeiterberechnen bool) {
	t.Helper()
	if err := store.SaveStammdaten(db, store.StammdatenSave{
		Apartments:              map[int64]store.StammdatenInput{1: {QM: 100, FlurstueckGroesse: 600}, 2: {QM: 50, FlurstueckGroesse: 400}},
		Flags:                   store.StammdatenFlags{StromWeiterberechnen: stromWeiterberechnen},
		HeizungWaermeGewichtung: 0.7,
		Wohnungen: map[int64]store.WohnungDetails{
			1: {Status: store.StatusEigennutzung},
			2: {MieterName: "Erika Beispiel", MieterAnschrift: "Beispielweg 1", Status: store.StatusVermietet},
		},
		Haus: store.HausDetails{VermieterName: "Max Mustermann", VermieterAnschrift: "Hauptstraße 5", ObjektAnschrift: "Beispielweg 1"},
	}); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}
}

func wert(logik, typ string, w float64) store.FixkostenPositionWert {
	return store.FixkostenPositionWert{Logik: logik, Typ: typ, Wert: w}
}

// teiljahrDB is the first Erfassungsjahr: Ausgangsstand on 15 Sep 2026,
// Ablesungen and Fixkosten-Eingaben for Oct, Nov, Dec (none for Sep). The
// Nebenkostenabschlag of Wohnung 2 is 100 in Oct and Nov, missing in Dec.
func teiljahrDB(t *testing.T, stromWeiterberechnen bool) *sql.DB {
	t.Helper()
	db := openTestDB(t)
	abrechnungStammdaten(t, db, stromWeiterberechnen)

	legeAblesungAn(t, db, 0, "2026-09-15", "2026-09-01")
	legeAblesungAn(t, db, 1, "2026-10-28", "2026-10-01")
	legeAblesungAn(t, db, 2, "2026-11-28", "2026-11-01")
	legeAblesungAn(t, db, 3, "2026-12-28", "2026-12-01")

	for i, monat := range []string{"2026-10-01", "2026-11-01", "2026-12-01"} {
		werte := map[int64]store.FixkostenPositionWert{
			1:  wert(store.LogikWohneinheit, store.TypJaehrlich, 1200), // Grundsteuer: 100/month, 50 for Wohnung 2
			2:  wert(store.LogikQM, store.TypJaehrlich, 600),           // Gebäudeversicherung: 50/month, Wohnung 2 has 1/3
			3:  wert(store.LogikWohnung1, store.TypJaehrlich, 1200),    // Deichbeitrag: fully Wohnung 1
			10: wert(store.LogikWohneinheit, store.TypJaehrlich, 100),  // Grundgebühr Strom: 8.3333/month
			15: wert(store.LogikWohneinheit, store.TypMonatlich, 20),   // Streaming: not umlagefähig
		}
		if i == 2 { // December: the Gebäudeversicherung switches to Je Wohneinheit
			werte[2] = wert(store.LogikWohneinheit, store.TypJaehrlich, 600)
		}
		in := store.FixkostenInput{Monat: monat, Personen: map[int64]int64{1: 2, 2: 1}, Werte: werte, Abschlag: map[int64]float64{1: 200}}
		if i < 2 {
			in.Abschlag[2] = 100
		}
		if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
			t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
		}
	}
	return db
}

func zeile(t *testing.T, zeilen []abrechnungZeile, position string) []abrechnungZeile {
	t.Helper()
	var out []abrechnungZeile
	for _, z := range zeilen {
		if z.Position == position {
			out = append(out, z)
		}
	}
	return out
}

func TestBerechneAbrechnung_Fixkosten(t *testing.T) {
	db := teiljahrDB(t, true)
	erg, err := berechneAbrechnungDB(db, 2026, 2)
	if err != nil {
		t.Fatalf("berechneAbrechnung: %v", err)
	}
	if erg.Abrechnung == nil {
		t.Fatalf("Maengel = %v, want an Abrechnung", maengelTexte(erg.Pruefung))
	}
	ab := erg.Abrechnung

	t.Run("Grundsteuer: eine Zeile, Summe der Monate", func(t *testing.T) {
		z := zeile(t, ab.Fixkosten, "Grundsteuer")
		if len(z) != 1 {
			t.Fatalf("Grundsteuer lines = %d, want 1", len(z))
		}
		if z[0].Gesamt != 300 || z[0].Betrag != 150 || z[0].Prozent != 50 || z[0].Schluessel != "Je Wohneinheit" || z[0].Von != "2026-10-01" || z[0].Bis != "2026-12-01" {
			t.Errorf("Grundsteuer = %+v, want 300/150/50 %%, Je Wohneinheit, Oktober bis Dezember", z[0])
		}
	})

	t.Run("Wechsel der Logik teilt die Zeile", func(t *testing.T) {
		z := zeile(t, ab.Fixkosten, "Wohngebäudeversicherung")
		if len(z) != 2 {
			t.Fatalf("Gebäudeversicherung lines = %d, want 2 (Je Wohnungsgröße, then Je Wohneinheit)", len(z))
		}
		// Oct+Nov by size: 50/month, Wohnung 2 has 50 of 150 m² -> 16.67 per month, 33.34 for two.
		if z[0].Schluessel != "Je anteilige Wohnungsgröße" || z[0].Gesamt != 100 || z[0].Betrag != 33.34 || z[0].Von != "2026-10-01" || z[0].Bis != "2026-11-01" {
			t.Errorf("first line = %+v, want Je anteilige Wohnungsgröße, 100 / 33.34, Oktober bis November", z[0])
		}
		if z[1].Schluessel != "Je Wohneinheit" || z[1].Gesamt != 50 || z[1].Betrag != 25 || z[1].Von != "2026-12-01" || z[1].Bis != "2026-12-01" {
			t.Errorf("second line = %+v, want Je Wohneinheit, 50 / 25, Dezember", z[1])
		}
	})

	t.Run("Rundung: Summe der je Monat gerundeten Anteile", func(t *testing.T) {
		z := zeile(t, ab.Fixkosten, "Grundgebühr Strom")
		if len(z) != 1 {
			t.Fatalf("Grundgebühr Strom lines = %d, want 1", len(z))
		}
		// 100/12 = 8.3333 per month; half is 4.1667 -> 4.17 per month, 12.51 for three. The exact value would be 12.50.
		if z[0].Gesamt != 25 || z[0].Betrag != 12.51 {
			t.Errorf("Grundgebühr Strom = Gesamt %v Betrag %v, want 25 / 12.51 (months rounded one by one)", z[0].Gesamt, z[0].Betrag)
		}
		if !near(z[0].Prozent, 50.04) {
			t.Errorf("Prozent = %v, want the effective 50.04", z[0].Prozent)
		}
	})

	t.Run("nicht umlagefaehige und voll der anderen Wohnung zugerechnete Positionen fehlen", func(t *testing.T) {
		if z := zeile(t, ab.Fixkosten, "Streaming-Dienste"); len(z) != 0 {
			t.Errorf("Streaming-Dienste (not umlagefähig) appears: %+v", z)
		}
		if z := zeile(t, ab.Fixkosten, "Deichbeitrag Grund und Boden"); len(z) != 0 {
			t.Errorf("Deichbeitrag (fully Wohnung 1) appears for Wohnung 2: %+v", z)
		}
	})

	t.Run("Wohnung 1 sieht den Deichbeitrag voll", func(t *testing.T) {
		erg1, err := berechneAbrechnungDB(db, 2026, 1)
		if err != nil || erg1.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung(Wohnung 1): err %v, Maengel %v", err, maengelTexte(erg1.Pruefung))
		}
		z := zeile(t, erg1.Abrechnung.Fixkosten, "Deichbeitrag Grund und Boden")
		if len(z) != 1 || z[0].Gesamt != 300 || z[0].Betrag != 300 || z[0].Prozent != 100 {
			t.Errorf("Deichbeitrag for Wohnung 1 = %+v, want 300 / 300 / 100 %%", z)
		}
	})

	t.Run("nur umlagefaehige Fixkosten, einschliesslich eines umgeschalteten Flags", func(t *testing.T) {
		saveStammdaten(t, db, func(s *store.StammdatenSave) {
			s.Flags = store.StammdatenFlags{Umlagefaehig: map[int64]bool{15: true, 1: false}, StromWeiterberechnen: true}
		})
		erg2, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg2.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		if z := zeile(t, erg2.Abrechnung.Fixkosten, "Streaming-Dienste"); len(z) != 1 || z[0].Betrag != 30 { // 20/month, half, three months
			t.Errorf("Streaming-Dienste after switching the flag on = %+v, want one line with Betrag 30", z)
		}
		if z := zeile(t, erg2.Abrechnung.Fixkosten, "Grundsteuer"); len(z) != 0 {
			t.Errorf("Grundsteuer after switching the flag off still appears: %+v", z)
		}
	})
}

func TestBerechneAbrechnung_VerbrauchVorauszahlungenUndSaldo(t *testing.T) {
	db := teiljahrDB(t, true)
	erg, err := berechneAbrechnungDB(db, 2026, 2)
	if err != nil || erg.Abrechnung == nil {
		t.Fatalf("berechneAbrechnung: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
	}
	ab := erg.Abrechnung

	// Expected consumption part: the sum over the three Ablesungen with a
	// predecessor (the Ausgangsstand of 15 Sep has no consumption).
	var heizung, wasser, strom, heizungGesamt, wasserGesamt float64
	periods, _ := store.AllPeriods(db)
	for _, p := range periods {
		k, err := berechneKosten(db, p.ID)
		if err != nil {
			t.Fatalf("berechneKosten: %v", err)
		}
		if k.KostenNote != "" {
			continue
		}
		heizung += k.Heizung.KostenHeizungW2
		heizungGesamt += k.Heizung.KostenHeizungW1 + k.Heizung.KostenHeizungW2
		wasser += k.Wasser.KostenFrischwasserW2 + k.Wasser.KostenAbwasserW2
		wasserGesamt += k.Wasser.KostenFrischwasserW1 + k.Wasser.KostenAbwasserW1 + k.Wasser.KostenFrischwasserW2 + k.Wasser.KostenAbwasserW2
		strom += k.Strom.KostenW2
	}
	if heizung == 0 || wasser == 0 || strom == 0 {
		t.Fatalf("scenario produces no consumption cost (heizung %v, wasser %v, strom %v)", heizung, wasser, strom)
	}
	if !near(ab.Heizung.Betrag, heizung) || !near(ab.Heizung.Gesamt, heizungGesamt) {
		t.Errorf("Heizung = %v of %v, want %v of %v", ab.Heizung.Betrag, ab.Heizung.Gesamt, heizung, heizungGesamt)
	}
	if !near(ab.Wasser.Betrag, wasser) || !near(ab.Wasser.Gesamt, wasserGesamt) {
		t.Errorf("Wasser = %v of %v, want %v of %v", ab.Wasser.Betrag, ab.Wasser.Gesamt, wasser, wasserGesamt)
	}
	if ab.Heizung.Schluessel != "70 % Wärmeverbrauch, 30 % Wohnfläche" {
		t.Errorf("Heizung.Schluessel = %q", ab.Heizung.Schluessel)
	}
	if ab.StromW2 == nil || !near(ab.StromW2.Kosten, strom) {
		t.Fatalf("StromW2 = %+v, want Kosten %v", ab.StromW2, strom)
	}

	// The Nebenkostenabschlag of December is missing and counts as 0.
	if ab.Vorauszahlungen != 200 {
		t.Errorf("Vorauszahlungen = %v, want 200 (100 + 100 + missing)", ab.Vorauszahlungen)
	}

	var fix float64
	for _, z := range ab.Fixkosten {
		fix += z.Betrag
	}
	if want := calc.Round2(fix + ab.Heizung.Betrag + ab.Wasser.Betrag); ab.Betriebskosten != want {
		t.Errorf("Betriebskosten = %v, want %v (Fixkosten + Heizung + Wasser, without the Strom)", ab.Betriebskosten, want)
	}
	wantSaldo := calc.Round2(ab.Vorauszahlungen - ab.Betriebskosten - ab.StromW2.Kosten)
	got := ab.Saldo.wert
	if got != wantSaldo {
		t.Errorf("Saldo = %v, want %v (Vorauszahlungen - Betriebskosten - Strom)", got, wantSaldo)
	}
	if !ab.Saldo.Nachzahlung() {
		t.Errorf("Saldo = %v, want a Nachzahlung (200 Vorauszahlungen against much higher costs)", got)
	}

	t.Run("Strom nicht weiterberechnet: Block und Saldo ohne ihn", func(t *testing.T) {
		db := teiljahrDB(t, false)
		erg, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		if erg.Abrechnung.StromW2 != nil {
			t.Errorf("StromW2 = %+v, want nil with the flag off", erg.Abrechnung.StromW2)
		}
		if want := calc.Round2(ab.Saldo.wert + ab.StromW2.Kosten); !near(erg.Abrechnung.Saldo.wert, want) {
			t.Errorf("Saldo = %v, want %v (the same without the Strom)", erg.Abrechnung.Saldo.wert, want)
		}
	})
	t.Run("Wohnung 1 hat keinen Strom-Block", func(t *testing.T) {
		erg1, err := berechneAbrechnungDB(db, 2026, 1)
		if err != nil || erg1.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung(Wohnung 1): err %v", err)
		}
		if erg1.Abrechnung.StromW2 != nil {
			t.Errorf("StromW2 for Wohnung 1 = %+v, want nil", erg1.Abrechnung.StromW2)
		}
		if erg1.Abrechnung.Vorauszahlungen != 600 {
			t.Errorf("Vorauszahlungen Wohnung 1 = %v, want 600 (3 x 200)", erg1.Abrechnung.Vorauszahlungen)
		}
	})
}

func TestBerechneAbrechnung_Monatsverlauf(t *testing.T) {
	nah := func(x, y float64) bool { return math.Abs(x-y) < 0.0151 }

	// pruefe checks the invariants every Monatsverlauf must hold against the
	// Abrechnung it belongs to.
	pruefe := func(t *testing.T, ab *abrechnung, stromDazu bool) {
		t.Helper()
		v := ab.Monatsverlauf
		saldo := v.Uebertrag.Betrag
		for _, z := range v.Zeilen {
			saldo = calc.Round2(saldo + z.Abschlag - z.Fixkosten - z.Verbrauch)
			if !nah(z.Saldo, saldo) {
				t.Errorf("%s: Saldo = %v, want the running balance %v", z.Monat, z.Saldo, saldo)
			}
		}
		var fix float64
		for _, z := range ab.Fixkosten {
			fix += z.Betrag
		}
		verbrauch := ab.Heizung.Betrag + ab.Wasser.Betrag
		if stromDazu {
			verbrauch += ab.StromW2.Kosten
		}
		if !nah(v.Fixkosten, fix) {
			t.Errorf("Summe Fixkosten = %v, want %v (the umlagefähige positions of the Abrechnung)", v.Fixkosten, fix)
		}
		if !nah(v.Verbrauch, verbrauch) {
			t.Errorf("Summe Verbrauch = %v, want %v", v.Verbrauch, verbrauch)
		}
		if !nah(v.Abschlag, ab.Vorauszahlungen) {
			t.Errorf("Summe Abschlag = %v, want %v", v.Abschlag, ab.Vorauszahlungen)
		}
		if !nah(v.Endsaldo-v.Uebertrag.Betrag, ab.Saldo.wert) {
			t.Errorf("Endsaldo - Übertrag = %v, want the Jahressaldo %v", v.Endsaldo-v.Uebertrag.Betrag, ab.Saldo.wert)
		}
	}

	t.Run("Wohnung 2 mit Strom", func(t *testing.T) {
		db := teiljahrDB(t, true)
		erg, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		ab := erg.Abrechnung
		var monate []string
		var abschlaege []float64
		for _, z := range ab.Monatsverlauf.Zeilen {
			monate = append(monate, z.Monat)
			abschlaege = append(abschlaege, z.Abschlag)
		}
		// The month of the Ausgangsstand (Sep) has no row, the December
		// Abschlag is missing and counts as 0.
		if want := []string{"2026-10-01", "2026-11-01", "2026-12-01"}; !reflect.DeepEqual(monate, want) {
			t.Errorf("Monate = %v, want %v", monate, want)
		}
		if want := []float64{100, 100, 0}; !reflect.DeepEqual(abschlaege, want) {
			t.Errorf("Abschlag = %v, want %v", abschlaege, want)
		}
		for _, z := range ab.Monatsverlauf.Zeilen {
			if z.Fixkosten <= 0 || z.Verbrauch <= 0 {
				t.Errorf("%s: Fixkosten %v, Verbrauch %v, want both positive", z.Monat, z.Fixkosten, z.Verbrauch)
			}
		}
		// Streaming is not umlagefähig: the monthly Fixkosten do not carry it.
		oct := ab.Monatsverlauf.Zeilen[0].Fixkosten
		if want := 50 + 50.0/3 + 100.0/12/2; !nah(oct, want) {
			t.Errorf("Fixkosten Oktober = %v, want %v (Grundsteuer 50 + Versicherung 1/3 of 50 + Strom-Grundgebühr half of 8,33)", oct, want)
		}
		pruefe(t, ab, true)
	})

	t.Run("Strom nicht weiterberechnet", func(t *testing.T) {
		db := teiljahrDB(t, false)
		erg, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		pruefe(t, erg.Abrechnung, false)
	})

	t.Run("Wohnung 1", func(t *testing.T) {
		db := teiljahrDB(t, true)
		erg, err := berechneAbrechnungDB(db, 2026, 1)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		for _, z := range erg.Abrechnung.Monatsverlauf.Zeilen {
			if z.Abschlag != 200 {
				t.Errorf("%s: Abschlag = %v, want 200", z.Monat, z.Abschlag)
			}
		}
		pruefe(t, erg.Abrechnung, false)
	})

	t.Run("Teilzeitraum zählt die Monate davor als Übertrag", func(t *testing.T) {
		db := teiljahrDB(t, true)
		d, err := ladeAbrechnungDaten(db)
		if err != nil {
			t.Fatalf("ladeAbrechnungDaten: %v", err)
		}
		ganz, err := berechneAbrechnung(db, d, 2026, ganzesJahr, 2)
		if err != nil || ganz.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung (ganz): err %v", err)
		}
		erg, err := berechneAbrechnung(db, d, 2026, monatsbereich{Von: 11, Bis: 12}, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		ab := erg.Abrechnung
		if got := len(ab.Monatsverlauf.Zeilen); got != 2 {
			t.Fatalf("rows = %d, want 2 (November, December)", got)
		}
		// October is before the period: it is the Übertrag, the same number
		// as its balance in the whole year.
		u := ab.Monatsverlauf.Uebertrag
		oktober := ganz.Abrechnung.Monatsverlauf.Zeilen[0].Saldo
		if !u.Vorhanden || !nah(u.Betrag, oktober) || u.Von != "2026-10-01" || u.Bis != "2026-10-01" {
			t.Errorf("Übertrag = %+v, want October's balance %v", u, oktober)
		}
		// Übertrag plus the two months is the balance of the whole year.
		if !nah(ab.Monatsverlauf.Endsaldo, ganz.Abrechnung.Monatsverlauf.Endsaldo) {
			t.Errorf("Endsaldo = %v, want %v (as in the whole year)", ab.Monatsverlauf.Endsaldo, ganz.Abrechnung.Monatsverlauf.Endsaldo)
		}
		// The Jahressaldo of the period alone excludes the Übertrag.
		pruefe(t, ab, true)
	})
}

// TestBerechneAbrechnung_Uebertrag covers the rules of the Übertrag Vorjahre
// for a December-only period of the test scenario (months October and
// November are before it).
func TestBerechneAbrechnung_Uebertrag(t *testing.T) {
	nah := func(x, y float64) bool { return math.Abs(x-y) < 0.0151 }
	dezember := monatsbereich{Von: 12, Bis: 12}
	rechne := func(t *testing.T, db *sql.DB, apartmentID int64) *abrechnung {
		t.Helper()
		d, err := ladeAbrechnungDaten(db)
		if err != nil {
			t.Fatalf("ladeAbrechnungDaten: %v", err)
		}
		erg, err := berechneAbrechnung(db, d, 2026, dezember, apartmentID)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v, Maengel %v", err, maengelTexte(erg.Pruefung))
		}
		return erg.Abrechnung
	}
	mieterSeit := func(t *testing.T, db *sql.DB, apartmentID int64, seit string) {
		t.Helper()
		if _, err := db.Exec(`UPDATE apartments SET mieter_seit = ? WHERE id = ?`, seit, apartmentID); err != nil {
			t.Fatalf("set mieter_seit: %v", err)
		}
	}
	ohneSeit := rechne(t, teiljahrDB(t, true), 2)
	u0 := ohneSeit.Monatsverlauf.Uebertrag
	if !u0.Vorhanden || u0.Von != "2026-10-01" || u0.Bis != "2026-11-01" || u0.Hinweis != "" {
		t.Fatalf("Übertrag ohne Mieter seit = %+v, want October to November", u0)
	}

	t.Run("Mieter seit spät: nur die Monate ab dort", func(t *testing.T) {
		db := teiljahrDB(t, true)
		mieterSeit(t, db, 2, "2026-11-01")
		u := rechne(t, db, 2).Monatsverlauf.Uebertrag
		if !u.Vorhanden || u.Von != "2026-11-01" || u.Bis != "2026-11-01" {
			t.Fatalf("Übertrag = %+v, want November only", u)
		}
		if nah(u.Betrag, u0.Betrag) {
			t.Errorf("Übertrag = %v as without Mieter seit, want only November", u.Betrag)
		}
	})

	t.Run("Mieter seit vor dem Erfassungsbeginn: der Erfassungsbeginn zählt", func(t *testing.T) {
		db := teiljahrDB(t, true)
		mieterSeit(t, db, 2, "2020-01-01")
		u := rechne(t, db, 2).Monatsverlauf.Uebertrag
		if !u.Vorhanden || u.Von != "2026-10-01" || !nah(u.Betrag, u0.Betrag) {
			t.Errorf("Übertrag = %+v, want the same as without Mieter seit (%v)", u, u0.Betrag)
		}
	})

	t.Run("Mieter seit gleich Von-Monat: Übertrag 0", func(t *testing.T) {
		db := teiljahrDB(t, true)
		mieterSeit(t, db, 2, "2026-12-01")
		ab := rechne(t, db, 2)
		u := ab.Monatsverlauf.Uebertrag
		if u.Vorhanden || u.Betrag != 0 || u.Hinweis != "" {
			t.Errorf("Übertrag = %+v, want none", u)
		}
		// The balance starts at 0: the Endsaldo is the Jahressaldo.
		if !nah(ab.Monatsverlauf.Endsaldo, ab.Saldo.wert) {
			t.Errorf("Endsaldo = %v, want the Jahressaldo %v", ab.Monatsverlauf.Endsaldo, ab.Saldo.wert)
		}
	})

	t.Run("Mieter seit nach dem Von-Monat: kein Übertrag, mit Hinweis", func(t *testing.T) {
		db := teiljahrDB(t, true)
		mieterSeit(t, db, 2, "2027-02-01")
		u := rechne(t, db, 2).Monatsverlauf.Uebertrag
		if u.Vorhanden || u.Betrag != 0 || !strings.Contains(u.Hinweis, "Februar 2027") {
			t.Errorf("Übertrag = %+v, want none with a note naming Februar 2027", u)
		}
	})

	t.Run("Eigennutzung ignoriert Mieter seit", func(t *testing.T) {
		db := teiljahrDB(t, true)
		mieterSeit(t, db, 1, "2026-12-01") // Wohnung 1 is Eigennutzung
		u := rechne(t, db, 1).Monatsverlauf.Uebertrag
		if !u.Vorhanden || u.Von != "2026-10-01" {
			t.Errorf("Übertrag = %+v, want from October (Mieter seit ignored)", u)
		}
	})

	t.Run("Mangel vor dem Zeitraum: nicht berechenbar, erster Monat im Hinweis", func(t *testing.T) {
		db := teiljahrDB(t, true)
		eingaben, err := store.AllFixkostenEingabenDetails(db)
		if err != nil {
			t.Fatalf("AllFixkostenEingabenDetails: %v", err)
		}
		for _, e := range eingaben {
			if e.Monat == "2026-10-01" || e.Monat == "2026-11-01" {
				if err := store.DeleteFixkostenEingabe(db, e.ID); err != nil {
					t.Fatalf("DeleteFixkostenEingabe: %v", err)
				}
			}
		}
		ab := rechne(t, db, 2) // December alone is fine
		u := ab.Monatsverlauf.Uebertrag
		if u.Vorhanden || u.Betrag != 0 || u.Hinweis != "Übertrag nicht berechenbar: Fixkosten-Eingabe fehlt: Oktober 2026" {
			t.Errorf("Übertrag = %+v, want a note on the first affected month (October)", u)
		}
		// The running balance starts at 0.
		z := ab.Monatsverlauf.Zeilen[0]
		if want := calc.Round2(z.Abschlag - z.Fixkosten - z.Verbrauch); !nah(z.Saldo, want) {
			t.Errorf("Saldo = %v, want %v (balance starts at 0)", z.Saldo, want)
		}
	})

	t.Run("fehlender Abschlag ist kein Mangel", func(t *testing.T) {
		// The December Abschlag of Wohnung 2 is missing in the scenario and
		// December alone is settled without a finding.
		ab := ohneSeit
		if len(ab.Monatsverlauf.Zeilen) != 1 || ab.Monatsverlauf.Zeilen[0].Abschlag != 0 {
			t.Errorf("rows = %+v, want December with Abschlag 0", ab.Monatsverlauf.Zeilen)
		}
	})

	t.Run("erstes Erfassungsjahr: nichts davor", func(t *testing.T) {
		db := teiljahrDB(t, true)
		erg, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		if u := erg.Abrechnung.Monatsverlauf.Uebertrag; u.Vorhanden || u.Hinweis != "" {
			t.Errorf("Übertrag = %+v, want none in the first Erfassungsjahr", u)
		}
	})
}

func TestBerechneAbrechnung_Zaehlerstaende(t *testing.T) {
	t.Run("erstes Erfassungsjahr: Ausgangsstand und alle Ablesungen", func(t *testing.T) {
		db := teiljahrDB(t, true)
		erg, err := berechneAbrechnungDB(db, 2026, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		z := erg.Abrechnung.Zaehlerstaende
		if len(z.Spalten) != len(store.MeterKeys) {
			t.Fatalf("columns = %d, want %d (all meters)", len(z.Spalten), len(store.MeterKeys))
		}
		var datum, monate []string
		for _, r := range z.Zeilen {
			datum = append(datum, r.Ablesedatum)
			monate = append(monate, r.Monat)
		}
		if want := []string{"2026-09-15", "2026-10-28", "2026-11-28", "2026-12-28"}; !reflect.DeepEqual(datum, want) {
			t.Errorf("Ablesedatum = %v, want %v", datum, want)
		}
		if want := []string{"2026-09-01", "2026-10-01", "2026-11-01", "2026-12-01"}; !reflect.DeepEqual(monate, want) {
			t.Errorf("Abrechnungsmonat = %v, want %v", monate, want)
		}
		for i, r := range z.Zeilen {
			if r.Ausgangsstand != (i == 0) {
				t.Errorf("row %d: Ausgangsstand = %v", i, r.Ausgangsstand)
			}
			for j, key := range store.MeterKeys {
				if want := abrechnungDelta[key] * float64(i); r.Staende[j].Wert != want {
					t.Errorf("row %d %s = %v, want %v", i, key, r.Staende[j].Wert, want)
				}
			}
		}
		// The meters of Wohnung 2 are the settled apartment's own.
		var eigen []string
		for i, sp := range z.Spalten {
			if sp.Eigen {
				eigen = append(eigen, store.MeterKeys[i])
			}
			if sp.Eigen != z.Zeilen[1].Staende[i].Eigen {
				t.Errorf("column %d: Eigen differs between header and cell", i)
			}
		}
		if want := []string{"strom_wohnung2", "wasser_wohnung2", "waerme_wohnung2"}; !reflect.DeepEqual(eigen, want) {
			t.Errorf("own meters = %v, want %v", eigen, want)
		}
	})

	t.Run("Teilzeitraum beginnt mit der Ablesung davor", func(t *testing.T) {
		db := teiljahrDB(t, true)
		d, err := ladeAbrechnungDaten(db)
		if err != nil {
			t.Fatalf("ladeAbrechnungDaten: %v", err)
		}
		erg, err := berechneAbrechnung(db, d, 2026, monatsbereich{Von: 11, Bis: 12}, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung: err %v", err)
		}
		var monate []string
		for _, r := range erg.Abrechnung.Zaehlerstaende.Zeilen {
			monate = append(monate, r.Monat)
		}
		if want := []string{"2026-10-01", "2026-11-01", "2026-12-01"}; !reflect.DeepEqual(monate, want) {
			t.Errorf("Abrechnungsmonate = %v, want %v (October is the Ausgangsstand)", monate, want)
		}
		if !erg.Abrechnung.Zaehlerstaende.Zeilen[0].Ausgangsstand {
			t.Error("the first row is not the Ausgangsstand")
		}
	})

	t.Run("untermonatige Ablesungen eigene Zeilen, Teilstand fehlt", func(t *testing.T) {
		apartments := []store.Apartment{{ID: 1}, {ID: 2}}
		meters := []store.Meter{{Key: "strom_gesamt", Label: "Stromzähler Gesamt", Unit: "kWh"}}
		preis := store.Float64(0.3)
		ablesung := func(id int64, datum, monat string, stand float64, vollstaendig bool) *store.LatestPeriod {
			p := &store.LatestPeriod{ID: id, ReadingDate: datum, Monat: monat, Readings: map[string]float64{}, PersonenByApartment: map[int64]int64{1: 1, 2: 1}}
			if vollstaendig {
				p.Strompreis, p.FrischwasserPreis, p.AbwasserPreis, p.EinspeisungPreis = preis, preis, preis, preis
			}
			for _, key := range store.MeterKeys {
				p.Readings[key] = stand
			}
			return p
		}
		periods := []*store.LatestPeriod{
			ablesung(4, "2026-03-01", "2026-03-01", 40, false), // Teilstand
			ablesung(3, "2026-02-20", "2026-02-01", 30, true),
			ablesung(2, "2026-02-10", "2026-02-01", 20, true),
			ablesung(1, "2026-01-10", "2026-01-01", 10, true),
		}
		imZeitraum := map[string]bool{"2026-02-01": true, "2026-03-01": true}
		z := zaehlerstaende(periods, apartments, meters, imZeitraum, 2)
		var datum []string
		for _, r := range z.Zeilen {
			datum = append(datum, r.Ablesedatum)
		}
		// January is the Ausgangsstand, both February Ablesungen are rows, the
		// Teilstand of March is not.
		if want := []string{"2026-01-10", "2026-02-10", "2026-02-20"}; !reflect.DeepEqual(datum, want) {
			t.Errorf("Ablesedatum = %v, want %v", datum, want)
		}
	})
}

func TestBerechneAbrechnung_Anhang(t *testing.T) {
	db := teiljahrDB(t, true)
	erg, err := berechneAbrechnungDB(db, 2026, 2)
	if err != nil || erg.Abrechnung == nil {
		t.Fatalf("berechneAbrechnung: err %v", err)
	}
	ab := erg.Abrechnung

	byZaehler := map[string]abrechnungVerbrauchZeile{}
	for _, v := range ab.Verbrauch {
		byZaehler[v.Zaehler] = v
	}
	if len(ab.Verbrauch) != len(store.MeterKeys) {
		t.Fatalf("Verbrauch lines = %d, want one per meter (%d)", len(ab.Verbrauch), len(store.MeterKeys))
	}
	// First year: Beginn is the Ausgangsstand (k=0, 0), Ende the December reading (k=3).
	if v := byZaehler["Stromzähler Gesamt"]; v.Beginn != 0 || v.Ende != 1842*3 || v.Verbrauch != 1842*3 || v.Einheit != "kWh" {
		t.Errorf("Stromzähler Gesamt = %+v, want 0 -> 5526 kWh", v)
	}

	if len(ab.HeizungMonate) != 3 || ab.HeizungMonate[0].Monat != "2026-10-01" {
		t.Fatalf("HeizungMonate = %+v, want Oktober, November, Dezember", ab.HeizungMonate)
	}
	var anteil float64
	for _, m := range ab.HeizungMonate {
		anteil += m.Anteil
		if m.WPStromKWh <= 0 || !near(m.WaermeW1MWh, 0.6) || !near(m.WaermeW2MWh, 0.4) {
			t.Errorf("month %s = %+v, want WP-Strom > 0 and heat 0.6 / 0.4 MWh", m.Monat, m)
		}
	}
	if !near(anteil, ab.Heizung.Betrag) {
		t.Errorf("Heizung table sums to %v, want %v (the line's Betrag)", anteil, ab.Heizung.Betrag)
	}

	// Personen per month from both sources: no Eingabe in September.
	if len(ab.PersonenMonate) != 4 || ab.PersonenMonate[0].Monat != "2026-09-01" {
		t.Fatalf("PersonenMonate = %+v, want September to December", ab.PersonenMonate)
	}
	if ab.PersonenMonate[0].Fixkosten != nil || ab.PersonenMonate[0].Ablesung[2] != 1 {
		t.Errorf("September = %+v, want no Fixkosten-Personen but the Ablesung's", ab.PersonenMonate[0])
	}
	if p := ab.PersonenMonate[1]; p.Fixkosten[1] != 2 || p.Fixkosten[2] != 1 || p.Ablesung[1] != 2 {
		t.Errorf("Oktober = %+v, want both sources", p)
	}
	if ab.Gewichtung != 0.7 || len(ab.Apartments) != 2 {
		t.Errorf("Gewichtung %v, Apartments %d, want 0.7 and 2", ab.Gewichtung, len(ab.Apartments))
	}
}

func TestBerechneAbrechnung_NichtAbrechenbar(t *testing.T) {
	db := teiljahrDB(t, true)
	// Remove the Fixkosten-Eingabe of November.
	eingaben, _ := store.AllFixkostenEingaben(db)
	for _, e := range eingaben {
		if e.Monat == "2026-11-01" {
			if err := store.DeleteFixkostenEingabe(db, e.ID); err != nil {
				t.Fatalf("DeleteFixkostenEingabe: %v", err)
			}
		}
	}
	erg, err := berechneAbrechnungDB(db, 2026, 2)
	if err != nil {
		t.Fatalf("berechneAbrechnung: %v", err)
	}
	if erg.Abrechnung != nil {
		t.Error("Abrechnung computed although a Fixkosten-Eingabe is missing")
	}
	if fmt.Sprint(maengelTexte(erg.Pruefung)) != "[Fixkosten-Eingabe fehlt: November 2026]" {
		t.Errorf("Maengel = %v, want the missing November Eingabe", maengelTexte(erg.Pruefung))
	}
}

// TestBerechneAbrechnung_StimmtMitDemDashboardUeberein is the consistency
// check of Issue #166: with every position umlagefähig and the Strom passed
// on, the Jahressaldo of a year is the change of the Dashboard's cumulative
// balance over that year (the Dashboard's year-end balance is continuous,
// no reset), for both apartments.
func TestBerechneAbrechnung_StimmtMitDemDashboardUeberein(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	all := map[int64]bool{}
	kps, _ := store.Kostenpositionen(db)
	for _, kp := range kps {
		all[kp.ID] = true
	}
	saveStammdaten(t, db, func(s *store.StammdatenSave) {
		s.Flags = store.StammdatenFlags{Umlagefaehig: all, StromWeiterberechnen: true}
	})

	dd, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	for _, apt := range dd.Apartments {
		verlauf := buildDashboardVerlauf(apt.ID, apt.Name, dd.PeriodenKosten, dd.FixkostenListe)
		endstand := map[int]float64{}
		for _, e := range verlauf.Eintraege {
			if e.Jahreszeile != nil && e.Jahreszeile.Endstand != nil {
				endstand[e.Jahreszeile.Jahr] = e.Jahreszeile.Endstand.wert
			}
		}

		for _, jahr := range []int{2023, 2024, 2025} {
			erg, err := berechneAbrechnungDB(db, jahr, apt.ID)
			if err != nil || erg.Abrechnung == nil {
				t.Fatalf("%s %d: err %v, Maengel %v", apt.Name, jahr, err, maengelTexte(erg.Pruefung))
			}
			want := endstand[jahr] - endstand[jahr-1] // 2022 has none: the balance starts at 0
			if got := erg.Abrechnung.Saldo.wert; !near(got, want) {
				t.Errorf("%s %d: Jahressaldo = %v, want the Dashboard's change over the year %v", apt.Name, jahr, got, want)
			}
		}
	}
}

// saveStammdaten applies mutate to a SaveStammdaten prefilled with the
// database's current house values, so a test changes only what it names
// (SaveStammdaten always writes the Heizungs-Gewichtung, the Strom flag and
// the house details).
func saveStammdaten(t *testing.T, db *sql.DB, mutate func(s *store.StammdatenSave)) {
	t.Helper()
	haus, err := store.GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	s := store.StammdatenSave{
		Flags:                   store.StammdatenFlags{StromWeiterberechnen: haus.StromWeiterberechnen},
		HeizungWaermeGewichtung: haus.HeizungWaermeGewichtung,
		Haus: store.HausDetails{
			VermieterName: haus.VermieterName, VermieterAnschrift: haus.VermieterAnschrift,
			ObjektAnschrift: haus.ObjektAnschrift, IBAN: haus.IBAN, Kontoinhaber: haus.Kontoinhaber,
		},
	}
	mutate(&s)
	if err := store.SaveStammdaten(db, s); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}
}

// TestBerechneAbrechnung_Teilzeitraum: a Mieterwechsel splits the year into
// two periods that together equal the whole period (per-month rounding makes
// the sums exact up to the cent of each line).
func TestBerechneAbrechnung_Teilzeitraum(t *testing.T) {
	db := teiljahrDB(t, true)
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		t.Fatalf("ladeAbrechnungDaten: %v", err)
	}
	rechne := func(b monatsbereich) *abrechnung {
		t.Helper()
		erg, err := berechneAbrechnung(db, d, 2026, b, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung(%+v): err %v, Maengel %v", b, err, maengelTexte(erg.Pruefung))
		}
		return erg.Abrechnung
	}
	ganz := rechne(ganzesJahr)
	a := rechne(monatsbereich{Von: 10, Bis: 10})
	b := rechne(monatsbereich{Von: 11, Bis: 12})

	if a.Zeitraum.Titel() != "Nebenkostenabrechnung Oktober 2026 bis Oktober 2026" || !a.Zeitraum.Teilzeitraum {
		t.Errorf("Titel = %q, Teilzeitraum %v", a.Zeitraum.Titel(), a.Zeitraum.Teilzeitraum)
	}
	if got := b.Zeitraum.Von.Format("2006-01-02") + " " + b.Zeitraum.Bis.Format("2006-01-02"); got != "2026-11-01 2026-12-31" {
		t.Errorf("Zeitraum B = %s, want 2026-11-01 2026-12-31", got)
	}
	if b.Zeitraum.TeilJahr || b.Zeitraum.Zusatz != "" {
		t.Errorf("a period after the first month is no Teiljahr: %+v", b.Zeitraum)
	}
	if ganz.Zeitraum.Teilzeitraum || ganz.Zeitraum.Titel() != "Nebenkostenabrechnung 2026" {
		t.Errorf("ganzes Jahr: Titel %q Teilzeitraum %v", ganz.Zeitraum.Titel(), ganz.Zeitraum.Teilzeitraum)
	}

	nah := func(x, y float64) bool { return math.Abs(x-y) < 0.0151 }
	if a.Vorauszahlungen != 100 || b.Vorauszahlungen != 100 {
		t.Errorf("Vorauszahlungen = %v + %v, want 100 + 100 (Dec missing)", a.Vorauszahlungen, b.Vorauszahlungen)
	}
	if !nah(a.Betriebskosten+b.Betriebskosten, ganz.Betriebskosten) {
		t.Errorf("Betriebskosten %v + %v, want %v", a.Betriebskosten, b.Betriebskosten, ganz.Betriebskosten)
	}
	if !nah(a.Heizung.Betrag+b.Heizung.Betrag, ganz.Heizung.Betrag) || !nah(a.Wasser.Betrag+b.Wasser.Betrag, ganz.Wasser.Betrag) {
		t.Errorf("Heizung/Wasser of the parts do not add up to the whole")
	}
	if !nah(a.Saldo.wert+b.Saldo.wert, ganz.Saldo.wert) {
		t.Errorf("Saldo %v + %v, want %v", a.Saldo.wert, b.Saldo.wert, ganz.Saldo.wert)
	}
}

func TestPruefeAbrechnung_Teilzeitraum(t *testing.T) {
	d := ladeTeiljahrPruefDaten(t)
	t.Run("nur Monate des Zeitraums pruefen", func(t *testing.T) {
		d := d
		d.Eingaben = d.Eingaben[:0] // no Fixkosten at all
		got, err := pruefeAbrechnungDaten(d, 2026, monatsbereich{Von: 11, Bis: 11}, 2)
		if err != nil || len(got.Maengel) != 1 || got.Maengel[0].Art != mangelFixkostenFehlt {
			t.Errorf("Maengel = %+v, err %v, want only the missing Fixkosten of November", got.Maengel, err)
		}
	})
	t.Run("Zeitraum vor der ersten Ablesung", func(t *testing.T) {
		got, _ := pruefeAbrechnungDaten(d, 2026, monatsbereich{Von: 1, Bis: 8}, 2)
		if got.Zeitraum != nil || len(got.Maengel) != 1 || got.Maengel[0].Art != mangelKeinZeitraum {
			t.Errorf("got %+v, want kein_zeitraum", got)
		}
	})
	t.Run("Zeitraum mit dem ersten Monat bleibt Teiljahr", func(t *testing.T) {
		got, _ := pruefeAbrechnungDaten(d, 2026, monatsbereich{Von: 1, Bis: 10}, 2)
		if got.Zeitraum == nil || !got.Zeitraum.TeilJahr || got.Zeitraum.Von.Format("2006-01-02") != "2026-09-15" || got.Zeitraum.Bis.Format("2006-01-02") != "2026-10-31" {
			t.Errorf("Zeitraum = %+v", got.Zeitraum)
		}
	})
}

func ladeTeiljahrPruefDaten(t *testing.T) abrechnungPruefDaten {
	t.Helper()
	d, err := ladeAbrechnungDaten(teiljahrDB(t, true))
	if err != nil {
		t.Fatalf("ladeAbrechnungDaten: %v", err)
	}
	return d.Pruef
}

// TestBerechneAbrechnung_UebertragStimmtMitDenVorjahrenUeberein checks the
// Übertrag against the demo data: with gap-free data and only umlagefähige
// positions it is the sum of the Jahressaldi of the earlier years, and the
// Endsaldo of the Monatsverlauf is that sum plus the Jahressaldo.
func TestBerechneAbrechnung_UebertragStimmtMitDenVorjahrenUeberein(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	nah := func(x, y float64) bool { return math.Abs(x-y) < 0.0351 }

	var vorher float64
	for _, jahr := range []int{2023, 2024, 2025} {
		erg, err := berechneAbrechnungDB(db, jahr, 2)
		if err != nil || erg.Abrechnung == nil {
			t.Fatalf("berechneAbrechnung(%d): err %v, Maengel %v", jahr, err, maengelTexte(erg.Pruefung))
		}
		v := erg.Abrechnung.Monatsverlauf
		if !nah(v.Uebertrag.Betrag, vorher) {
			t.Errorf("%d: Übertrag = %v, want %v (sum of the Jahressaldi before)", jahr, v.Uebertrag.Betrag, vorher)
		}
		if !nah(v.Endsaldo, vorher+erg.Abrechnung.Saldo.wert) {
			t.Errorf("%d: Endsaldo = %v, want Übertrag + Jahressaldo = %v", jahr, v.Endsaldo, vorher+erg.Abrechnung.Saldo.wert)
		}
		vorher += erg.Abrechnung.Saldo.wert
	}
}
