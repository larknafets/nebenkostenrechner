package store

import (
	"path/filepath"
	"testing"
)

// einzelzaehler_test.go covers the optional Wallbox-Einzelzähler (#224):
// Wallbox Wohnung 1 / Wallbox Wohnung 2 are readings a period may leave out
// without becoming a Teilstand.

const (
	keyWallboxW1 = "strom_wallbox_wohnung1"
	keyWallboxW2 = "strom_wallbox_wohnung2"
)

func vollePeriode(date string, extra map[string]float64) PeriodInput {
	r := baseReadings(nil)
	for k, v := range extra {
		r[k] = v
	}
	return PeriodInput{
		ReadingDate:       date,
		Monat:             date[:7] + "-01",
		Strompreis:        Float64(0.22),
		FrischwasserPreis: Float64(1.46),
		AbwasserPreis:     Float64(4.87),
		EinspeisungPreis:  Float64(0.08),
		Readings:          r,
		Personen:          map[int64]float64{1: 2, 2: 1},
	}
}

func TestEinzelzaehler_RoundtripAlsOptionaleStaende(t *testing.T) {
	db := openTestDB(t)

	id, err := CreatePeriod(db, vollePeriode("2026-06-01", map[string]float64{keyWallboxW2: 120.5}))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	got, err := GetPeriodDetails(db, id)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if v, ok := got.Readings[keyWallboxW2]; !ok || v != 120.5 {
		t.Errorf("Readings[%s] = %v, %v, want 120.5, true", keyWallboxW2, v, ok)
	}
	if _, ok := got.Readings[keyWallboxW1]; ok {
		t.Errorf("Readings[%s] vorhanden, want nicht erfasst (nur Wohnung 2 eingetragen)", keyWallboxW1)
	}

	// Add via update without losing the existing reading.
	in := vollePeriode("2026-06-01", map[string]float64{keyWallboxW1: 33})
	delete(in.Readings, keyWallboxW2)
	if err := UpdatePeriod(db, id, in); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}
	got, _ = GetPeriodDetails(db, id)
	if got.Readings[keyWallboxW1] != 33 || got.Readings[keyWallboxW2] != 120.5 {
		t.Errorf("Readings nach Update = W1 %v, W2 %v, want 33 und 120.5", got.Readings[keyWallboxW1], got.Readings[keyWallboxW2])
	}
}

func TestEinzelzaehler_FehlenMachtKeinenTeilstand(t *testing.T) {
	db := openTestDB(t)
	apartments, err := Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}

	id, err := CreatePeriod(db, vollePeriode("2026-06-01", nil))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	ok, err := PeriodComplete(db, id)
	if err != nil {
		t.Fatalf("PeriodComplete: %v", err)
	}
	if !ok {
		t.Error("PeriodComplete ohne Einzelzähler = false, want true")
	}
	details, _ := GetPeriodDetails(db, id)
	st := details.Teilstand(apartments)
	if st.IstTeilstand {
		t.Error("Teilstand().IstTeilstand ohne Einzelzähler = true, want false")
	}
	if st.MeterErfasst[keyWallboxW1] || st.MeterErfasst[keyWallboxW2] {
		t.Errorf("MeterErfasst für Einzelzähler = %v, want beide false", st.MeterErfasst)
	}
}

func TestEinzelzaehler_ErsetztKeinenFehlendenPflichtzaehler(t *testing.T) {
	db := openTestDB(t)
	apartments, _ := Apartments(db)

	in := vollePeriode("2026-06-01", map[string]float64{keyWallboxW1: 10, keyWallboxW2: 20})
	delete(in.Readings, "strom_gesamt")
	id, err := CreatePeriod(db, in)
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	ok, err := PeriodComplete(db, id)
	if err != nil {
		t.Fatalf("PeriodComplete: %v", err)
	}
	if ok {
		t.Error("PeriodComplete mit fehlendem Pflichtzähler und beiden Einzelzählern = true, want false")
	}
	details, _ := GetPeriodDetails(db, id)
	st := details.Teilstand(apartments)
	if !st.IstTeilstand {
		t.Error("Teilstand().IstTeilstand = false, want true")
	}
	if !st.MeterErfasst[keyWallboxW1] || !st.MeterErfasst[keyWallboxW2] {
		t.Errorf("MeterErfasst für Einzelzähler = %v, want beide true", st.MeterErfasst)
	}
}

func TestVerbrauchOptional(t *testing.T) {
	cases := []struct {
		name      string
		cur, prev map[string]float64
		want      float64
		wantOK    bool
	}{
		{"beide Stände", map[string]float64{"k": 150}, map[string]float64{"k": 100}, 50, true},
		{"Stand 0 ist ein Stand", map[string]float64{"k": 5}, map[string]float64{"k": 0}, 5, true},
		{"Vorperiode fehlt", map[string]float64{"k": 150}, map[string]float64{}, 0, false},
		{"aktueller Stand fehlt", map[string]float64{}, map[string]float64{"k": 100}, 0, false},
		{"negativ bleibt negativ", map[string]float64{"k": 90}, map[string]float64{"k": 100}, -10, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := VerbrauchOptional(c.cur, c.prev, "k")
			if ok != c.wantOK || got != c.want {
				t.Errorf("VerbrauchOptional = %v, %v, want %v, %v", got, ok, c.want, c.wantOK)
			}
		})
	}
}

// TestEinzelzaehler_BestehendeDatenbankBleibtUnveraendert opens a database
// that predates the Einzelzähler (meters 1-10 only) and checks that nothing
// that existed changes: readings, Verbrauch, completeness and the meter list
// used by the Anlage stay as they were.
func TestEinzelzaehler_BestehendeDatenbankBleibtUnveraendert(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alt.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	p1, err := CreatePeriod(db, vollePeriode("2026-05-01", map[string]float64{"strom_gesamt": 100}))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	p2, err := CreatePeriod(db, vollePeriode("2026-06-01", map[string]float64{"strom_gesamt": 350}))
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	// Back to the pre-#224 shape: no Einzelzähler meters.
	if _, err := db.Exec(`DELETE FROM meters WHERE key IN (?, ?)`, keyWallboxW1, keyWallboxW2); err != nil {
		t.Fatalf("remove Einzelzähler meters: %v", err)
	}
	db.Close()

	db, err = Open(path)
	if err != nil {
		t.Fatalf("Open (existing database): %v", err)
	}
	t.Cleanup(func() { db.Close() })

	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM meters WHERE key IN (?, ?)`, keyWallboxW1, keyWallboxW2).Scan(&n); err != nil {
		t.Fatalf("count meters: %v", err)
	}
	if n != 2 {
		t.Errorf("Einzelzähler-Meter nach Öffnen = %d, want 2", n)
	}

	for _, id := range []int64{p1, p2} {
		ok, err := PeriodComplete(db, id)
		if err != nil || !ok {
			t.Errorf("PeriodComplete(%d) = %v, %v, want true, nil", id, ok, err)
		}
	}
	got, _ := GetPeriodDetails(db, p2)
	if _, ok := got.Readings[keyWallboxW1]; ok || len(got.Readings) != len(MeterKeys) {
		t.Errorf("Readings nach Öffnen = %v, want nur die %d Pflichtzähler", got.Readings, len(MeterKeys))
	}

	meters, err := Meters(db)
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	required := 0
	for _, m := range meters {
		if !m.Optional {
			required++
		}
	}
	if required != len(MeterKeys) || len(meters) != len(MeterKeys)+len(OptionalMeterKeys) {
		t.Errorf("Meters = %d, davon Pflicht %d, want %d und %d (alle Zähler, Anlage filtert selbst)", len(meters), required, len(MeterKeys)+len(OptionalMeterKeys), len(MeterKeys))
	}
}
