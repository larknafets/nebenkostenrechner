package web

import (
	"database/sql"
	"reflect"
	"testing"
	"time"

	abr "github.com/larknafets/nebenkostenrechner/internal/abrechnung"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// pruefeAbrechnungDB loads the data and checks it, the way the page does.
func pruefeAbrechnungDB(db *sql.DB, jahr int, apartmentID int64) (abr.Pruefung, error) {
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		return abr.Pruefung{}, err
	}
	return abr.Pruefe(d.Pruef, jahr, abr.GanzesJahr, apartmentID)
}

// berechneAbrechnungDB loads the data and computes the Abrechnung, the way
// the page does.
func berechneAbrechnungDB(db *sql.DB, jahr int, apartmentID int64) (abrechnungErgebnis, error) {
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	return berechneAbrechnung(db, d, jahr, abr.GanzesJahr, apartmentID)
}

// pruefeAbrechnungAlteLadung is the loading of the Prüfung from before
// abrechnungDaten (per-table store calls, Eingaben as newest-first
// summaries), kept as the reference the new data path must agree with.
func pruefeAbrechnungAlteLadung(db *sql.DB, jahr int, apartmentID int64) (abr.Pruefung, error) {
	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abr.Pruefung{}, err
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return abr.Pruefung{}, err
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abr.Pruefung{}, err
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abr.Pruefung{}, err
	}
	return abr.Pruefe(abr.Daten{Periods: periods, Eingaben: eingaben, Apartments: apartments, Haus: haus}, jahr, abr.GanzesJahr, apartmentID)
}

func maengelTexte(p abr.Pruefung) []string {
	out := make([]string, len(p.Maengel))
	for i, m := range p.Maengel {
		out[i] = m.Text
	}
	return out
}

// TestPruefeAbrechnung_Demodaten runs the loading function against the demo
// database: months 2023-08 to 2026-10, a complete Ablesung and Fixkosten-
// Eingabe for each, made-up Stammdaten.
func TestPruefeAbrechnung_Demodaten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	for _, tc := range []struct {
		jahr     int
		teilJahr bool
	}{{2023, true}, {2024, false}, {2025, false}} {
		got, err := pruefeAbrechnungDB(db, tc.jahr, 2)
		if err != nil {
			t.Fatalf("pruefeAbrechnung(%d): %v", tc.jahr, err)
		}
		if !got.Abrechenbar() {
			t.Errorf("%d: Maengel = %v, want none", tc.jahr, maengelTexte(got))
		}
		if got.Zeitraum == nil || got.Zeitraum.TeilJahr != tc.teilJahr {
			t.Errorf("%d: Zeitraum = %+v, want TeilJahr %v", tc.jahr, got.Zeitraum, tc.teilJahr)
		}
	}

	if got, _ := pruefeAbrechnungDB(db, 2026, 2); got.Abrechenbar() || got.Maengel[0].Text != "Ablesung fehlt: November 2026" {
		t.Errorf("2026: Maengel = %v, want findings starting with the missing November Ablesung", maengelTexte(got))
	}
	if got, _ := pruefeAbrechnungDB(db, 2022, 2); got.Zeitraum != nil || got.Abrechenbar() {
		t.Errorf("2022: Zeitraum %+v, Maengel %v, want no Zeitraum", got.Zeitraum, maengelTexte(got))
	}
}

// TestStandardJahrUndPruefungAufGeladenenDaten: the preselection and the
// check work on the same loaded data and agree with the check per year: the
// preselected year is the newest one whose check passes, else the newest year
// (Demodaten: 2026 has Mängel, 2025 is the newest complete year).
func TestStandardJahrUndPruefungAufGeladenenDaten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		t.Fatalf("ladeAbrechnungDaten: %v", err)
	}

	jahre := abrechnungJahre(d.Pruef)
	if len(jahre) == 0 || jahre[0] != 2026 {
		t.Fatalf("abrechnungJahre = %v, want newest first starting with 2026", jahre)
	}
	for _, apartmentID := range []int64{1, 2} {
		got, err := standardJahr(d.Pruef, jahre, apartmentID)
		if err != nil {
			t.Fatalf("standardJahr(%d): %v", apartmentID, err)
		}
		if got != 2025 {
			t.Errorf("standardJahr(Wohnung %d) = %d, want 2025", apartmentID, got)
		}
		for _, y := range jahre {
			fromDB, err := pruefeAbrechnungAlteLadung(db, y, apartmentID)
			if err != nil {
				t.Fatalf("pruefeAbrechnungAlteLadung(%d): %v", y, err)
			}
			onData, err := abr.Pruefe(d.Pruef, y, abr.GanzesJahr, apartmentID)
			if err != nil {
				t.Fatalf("abr.Pruefe(%d): %v", y, err)
			}
			if !reflect.DeepEqual(fromDB, onData) {
				t.Errorf("Wohnung %d, %d: Prüfung differs: %+v vs %+v", apartmentID, y, fromDB, onData)
			}
			if y >= got && onData.Abrechenbar() != (y == got) {
				t.Errorf("Wohnung %d, %d: Abrechenbar = %v, preselection %d", apartmentID, y, onData.Abrechenbar(), got)
			}
		}
	}
}
