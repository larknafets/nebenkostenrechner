package calc_test

import (
	"database/sql"
	"math"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func mustCreateFixkostenEingabe(t *testing.T, db *sql.DB, monat string, personen map[int64]int64, werte map[int64]store.FixkostenPositionWert) int64 {
	t.Helper()
	id, err := store.CreateFixkostenEingabe(db, store.FixkostenInput{Monat: monat, Personen: personen, Werte: werte})
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
	}
	return id
}

func findPosition(t *testing.T, erg *calc.FixkostenErgebnis, key string) calc.FixkostenPosition {
	t.Helper()
	for _, p := range erg.Positionen {
		if p.Key == key {
			return p
		}
	}
	t.Fatalf("Position %q not found in %+v", key, erg.Positionen)
	return calc.FixkostenPosition{}
}

func TestFixkosten_LogikWohneinheit_5050(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 2, 2: 1}, map[int64]store.FixkostenPositionWert{
		6: {Logik: store.LogikWohneinheit, Typ: store.TypJaehrlich, Wert: 240}, // abfall_haushalt
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "abfall_haushalt")
	if pos.Monatswert != 20 {
		t.Errorf("Monatswert = %v, want 20 (240/12)", pos.Monatswert)
	}
	if pos.KostenW1 != 10 || pos.KostenW2 != 10 {
		t.Errorf("KostenW1/W2 = %v/%v, want 10/10 (wohneinheit ist immer 50/50, unabhaengig von Personen)", pos.KostenW1, pos.KostenW2)
	}
}

func TestFixkosten_LogikFlurstueck_Ratio(t *testing.T) {
	db := openTestDB(t)
	if err := store.UpdateStammdaten(db, map[int64]store.StammdatenInput{
		1: {QM: 100, FlurstueckGroesse: 600},
		2: {QM: 100, FlurstueckGroesse: 400},
	}); err != nil {
		t.Fatalf("UpdateStammdaten: %v", err)
	}
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 1, 2: 1}, map[int64]store.FixkostenPositionWert{
		3: {Logik: store.LogikFlurstueck, Typ: store.TypJaehrlich, Wert: 1200}, // deich_grund
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "deich_grund")
	if pos.Monatswert != 100 {
		t.Errorf("Monatswert = %v, want 100 (1200/12)", pos.Monatswert)
	}
	if pos.KostenW1 != 60 || pos.KostenW2 != 40 {
		t.Errorf("KostenW1/W2 = %v/%v, want 60/40 (600:400 Flurstueck-Verhaeltnis)", pos.KostenW1, pos.KostenW2)
	}
}

func TestFixkosten_LogikQM_Ratio(t *testing.T) {
	db := openTestDB(t)
	if err := store.UpdateStammdaten(db, map[int64]store.StammdatenInput{
		1: {QM: 150, FlurstueckGroesse: 0},
		2: {QM: 50, FlurstueckGroesse: 0},
	}); err != nil {
		t.Fatalf("UpdateStammdaten: %v", err)
	}
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 1, 2: 1}, map[int64]store.FixkostenPositionWert{
		1: {Logik: store.LogikQM, Typ: store.TypJaehrlich, Wert: 480}, // grundsteuer
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "grundsteuer")
	if pos.Monatswert != 40 {
		t.Errorf("Monatswert = %v, want 40 (480/12)", pos.Monatswert)
	}
	if pos.KostenW1 != 30 || pos.KostenW2 != 10 {
		t.Errorf("KostenW1/W2 = %v/%v, want 30/10 (150:50 QM-Verhaeltnis)", pos.KostenW1, pos.KostenW2)
	}
}

func TestFixkosten_LogikPersonen_Ratio(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 3, 2: 1}, map[int64]store.FixkostenPositionWert{
		7: {Logik: store.LogikPersonen, Typ: store.TypJaehrlich, Wert: 360}, // abfall_personen
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "abfall_personen")
	if pos.Monatswert != 30 {
		t.Errorf("Monatswert = %v, want 30 (360/12)", pos.Monatswert)
	}
	if pos.KostenW1 != 22.5 || pos.KostenW2 != 7.5 {
		t.Errorf("KostenW1/W2 = %v/%v, want 22.5/7.5 (3:1 Personen-Verhaeltnis)", pos.KostenW1, pos.KostenW2)
	}
}

// TestFixkosten_LogikPersonen_KeinePersonen_FaelltAufHaelftigeVerteilungZurueck
// mirrors Issue #26's Ratio2 zero-guard (see calc.Ratio2 doc comment): 0/0
// occupants must not silently make the position vanish from both bills.
func TestFixkosten_LogikPersonen_KeinePersonen_FaelltAufHaelftigeVerteilungZurueck(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 0, 2: 0}, map[int64]store.FixkostenPositionWert{
		7: {Logik: store.LogikPersonen, Typ: store.TypJaehrlich, Wert: 240},
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "abfall_personen")
	if pos.KostenW1 != 10 || pos.KostenW2 != 10 {
		t.Errorf("KostenW1/W2 bei 0 Personen = %v/%v, want 10/10 (haelftiger Fallback statt 0/0)", pos.KostenW1, pos.KostenW2)
	}
}

func TestFixkosten_TypMonatlich_ExpliziterWert(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 1, 2: 1}, map[int64]store.FixkostenPositionWert{
		13: {Logik: store.LogikWohneinheit, Typ: store.TypMonatlich, Wert: 39.90}, // internet
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	pos := findPosition(t, got, "internet")
	if pos.Monatswert != 39.90 {
		t.Errorf("Monatswert = %v, want 39.90 (monatlich, keine /12-Teilung)", pos.Monatswert)
	}
	if pos.KostenW1 != 19.95 || pos.KostenW2 != 19.95 {
		t.Errorf("KostenW1/W2 = %v/%v, want 19.95/19.95", pos.KostenW1, pos.KostenW2)
	}
}

// TestFixkosten_PositionOhneWert_Uebersprungen covers #105/#107: since
// Logik/Typ/Wert hang directly off the entry (instead of per-year in
// kostenpositionen_jahre), there's no fallback lookup anymore - a position
// without its own value in this entry is simply skipped, instead of
// guessing a Logik/Typ.
func TestFixkosten_PositionOhneWert_Uebersprungen(t *testing.T) {
	db := openTestDB(t)
	id := mustCreateFixkostenEingabe(t, db, "2026-03-01", map[int64]int64{1: 1, 2: 1}, nil)

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	if len(got.Positionen) != 0 {
		t.Errorf("len(Positionen) = %d, want 0 (keine Werte erfasst)", len(got.Positionen))
	}
	if got.KostenW1 != 0 || got.KostenW2 != 0 {
		t.Errorf("KostenW1/W2 = %v/%v, want 0/0", got.KostenW1, got.KostenW2)
	}
}

func TestFixkosten_SummenKonsistenz(t *testing.T) {
	db := openTestDB(t)
	if err := store.UpdateStammdaten(db, map[int64]store.StammdatenInput{
		1: {QM: 120, FlurstueckGroesse: 700},
		2: {QM: 80, FlurstueckGroesse: 300},
	}); err != nil {
		t.Fatalf("UpdateStammdaten: %v", err)
	}
	id := mustCreateFixkostenEingabe(t, db, "2026-09-01", map[int64]int64{1: 3, 2: 1}, map[int64]store.FixkostenPositionWert{
		1:  {Logik: store.LogikQM, Typ: store.TypJaehrlich, Wert: 480},
		3:  {Logik: store.LogikFlurstueck, Typ: store.TypJaehrlich, Wert: 1200},
		6:  {Logik: store.LogikWohneinheit, Typ: store.TypJaehrlich, Wert: 240},
		7:  {Logik: store.LogikPersonen, Typ: store.TypJaehrlich, Wert: 360},
		13: {Logik: store.LogikWohneinheit, Typ: store.TypMonatlich, Wert: 39.90},
	})

	got, err := calc.Fixkosten(db, id)
	if err != nil {
		t.Fatalf("calc.Fixkosten: %v", err)
	}
	if len(got.Positionen) != 5 {
		t.Fatalf("len(Positionen) = %d, want 5 (nur die erfassten)", len(got.Positionen))
	}
	var sumMonatswerte float64
	for _, p := range got.Positionen {
		sumMonatswerte += p.Monatswert
	}
	if diff := math.Abs((got.KostenW1 + got.KostenW2) - sumMonatswerte); diff > 0.05 {
		t.Errorf("KostenW1+KostenW2 = %v, sumMonatswerte = %v, diff %v > 0.05 (Rundungsdrift zu gross)", got.KostenW1+got.KostenW2, sumMonatswerte, diff)
	}
}
