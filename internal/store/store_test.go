package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func baseReadings(overrides map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(MeterKeys))
	for _, k := range MeterKeys {
		out[k] = 0
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func mustCreatePeriod(t *testing.T, db *sql.DB, date string, readings map[string]float64) int64 {
	t.Helper()
	id, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             date,
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       Float64(1.46),
		AbwasserPreis:           Float64(4.87),
		HeizungWaermeGewichtung: 0.7,
		Readings:                readings,
		Personen:                map[int64]int64{1: 2, 2: 1},
	})
	if err != nil {
		t.Fatalf("create period %s: %v", date, err)
	}
	return id
}

func TestCreatePeriod_GetLatestPeriod_AllPeriods_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	p1 := mustCreatePeriod(t, db, "2026-09-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	p2 := mustCreatePeriod(t, db, "2026-10-01", baseReadings(map[string]float64{"strom_gesamt": 200}))

	latest, err := GetLatestPeriod(db)
	if err != nil {
		t.Fatalf("GetLatestPeriod: %v", err)
	}
	if latest == nil {
		t.Fatal("GetLatestPeriod: want a period, got nil")
	}
	if latest.ID != p2 {
		t.Errorf("GetLatestPeriod.ID = %d, want %d (the newer period)", latest.ID, p2)
	}
	if latest.ReadingDate != "2026-10-01" {
		t.Errorf("GetLatestPeriod.ReadingDate = %q, want 2026-10-01", latest.ReadingDate)
	}
	if OrZero(latest.Strompreis) != 0.22 || OrZero(latest.FrischwasserPreis) != 1.46 || OrZero(latest.AbwasserPreis) != 4.87 {
		t.Errorf("GetLatestPeriod prices = %v/%v/%v, want 0.22/1.46/4.87", OrZero(latest.Strompreis), OrZero(latest.FrischwasserPreis), OrZero(latest.AbwasserPreis))
	}
	if latest.HeizungWaermeGewichtung != 0.7 {
		t.Errorf("GetLatestPeriod.HeizungWaermeGewichtung = %v, want 0.7", latest.HeizungWaermeGewichtung)
	}
	if latest.Readings["strom_gesamt"] != 200 {
		t.Errorf("GetLatestPeriod.Readings[strom_gesamt] = %v, want 200", latest.Readings["strom_gesamt"])
	}
	if latest.PersonenByApartment[1] != 2 || latest.PersonenByApartment[2] != 1 {
		t.Errorf("GetLatestPeriod.PersonenByApartment = %v, want {1:2, 2:1}", latest.PersonenByApartment)
	}

	all, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("AllPeriods: want 2 periods, got %d", len(all))
	}
	if all[0].ID != p2 || all[1].ID != p1 {
		t.Errorf("AllPeriods order = [%d, %d], want [%d, %d] (newest first)", all[0].ID, all[1].ID, p2, p1)
	}
}

// TestPeriod_Monat_Roundtrip verifies Issue #86: a period's Monat
// (Abrechnungsmonat) persists independently of ReadingDate and comes back
// through every read path.
func TestPeriod_Monat_Roundtrip(t *testing.T) {
	db := openTestDB(t)
	p1, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-08-31",
		Monat:                   "2026-08-01",
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       Float64(1.46),
		AbwasserPreis:           Float64(4.87),
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
		Personen:                map[int64]int64{1: 2, 2: 1},
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	details, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if details.Monat != "2026-08-01" {
		t.Errorf("GetPeriodDetails.Monat = %q, want 2026-08-01", details.Monat)
	}

	latest, err := GetLatestPeriod(db)
	if err != nil {
		t.Fatalf("GetLatestPeriod: %v", err)
	}
	if latest.Monat != "2026-08-01" {
		t.Errorf("GetLatestPeriod.Monat = %q, want 2026-08-01", latest.Monat)
	}

	all, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(all) != 1 || all[0].Monat != "2026-08-01" {
		t.Errorf("AllPeriods[0].Monat = %q, want 2026-08-01", all[0].Monat)
	}

	allDetails, err := AllPeriodDetails(db)
	if err != nil {
		t.Fatalf("AllPeriodDetails: %v", err)
	}
	if len(allDetails) != 1 || allDetails[0].Monat != "2026-08-01" {
		t.Errorf("AllPeriodDetails[0].Monat = %q, want 2026-08-01", allDetails[0].Monat)
	}
}

// TestUpdatePeriod_Monat_Roundtrip verifies Issue #86: UpdatePeriod persists
// a changed Monat, independent of ReadingDate.
func TestUpdatePeriod_Monat_Roundtrip(t *testing.T) {
	db := openTestDB(t)
	p1, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-08-31",
		Monat:                   "2026-08-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	if err := UpdatePeriod(db, p1, PeriodInput{
		ReadingDate:             "2026-08-31",
		Monat:                   "2026-09-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	got, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if got.Monat != "2026-09-01" {
		t.Errorf("Monat after UpdatePeriod = %q, want 2026-09-01", got.Monat)
	}
}

// TestSeed_ApartmentsQMStartsAtZero verifies Ticket #38: a fresh install
// doesn't hardcode this household's real Wohnungsgröße as an app default -
// qm starts at 0, like Strompreis/Personen have no seed default either.
func TestSeed_ApartmentsQMStartsAtZero(t *testing.T) {
	db := openTestDB(t)
	apartments, err := Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}
	if len(apartments) != 2 {
		t.Fatalf("want 2 apartments, got %d", len(apartments))
	}
	for _, a := range apartments {
		if a.QM != 0 {
			t.Errorf("apartment %q QM = %v, want 0 (unset until first Ablesung)", a.Name, a.QM)
		}
	}
}

func TestGetLatestPeriod_NoPeriods(t *testing.T) {
	db := openTestDB(t)
	latest, err := GetLatestPeriod(db)
	if err != nil {
		t.Fatalf("GetLatestPeriod: %v", err)
	}
	if latest != nil {
		t.Errorf("GetLatestPeriod on empty db = %+v, want nil", latest)
	}
}

func TestVerbrauch_Normal(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	p2 := mustCreatePeriod(t, db, "2026-10-01", baseReadings(map[string]float64{"strom_gesamt": 350}))

	v, err := Verbrauch(db, p2)
	if err != nil {
		t.Fatalf("Verbrauch: %v", err)
	}
	if v["strom_gesamt"] != 250 {
		t.Errorf("Verbrauch[strom_gesamt] = %v, want 250", v["strom_gesamt"])
	}
}

func TestVerbrauch_ErrNoPreviousPeriod(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-09-01", baseReadings(map[string]float64{"strom_gesamt": 100}))

	_, err := Verbrauch(db, p1)
	if !errors.Is(err, ErrNoPreviousPeriod) {
		t.Fatalf("Verbrauch on the oldest period: err = %v, want ErrNoPreviousPeriod", err)
	}
}

// TestVerbrauch_UeberLuecke verifies that a missing Ablesung (Ticket #9)
// doesn't break Verbrauch - it just diffs against whichever period is
// chronologically next-older, however far back that is.
func TestVerbrauch_UeberLuecke(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-06-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	// 4-month gap (July-Sept missing) instead of the usual 1.
	p2 := mustCreatePeriod(t, db, "2026-10-01", baseReadings(map[string]float64{"strom_gesamt": 500}))

	v, err := Verbrauch(db, p2)
	if err != nil {
		t.Fatalf("Verbrauch: %v", err)
	}
	if v["strom_gesamt"] != 400 {
		t.Errorf("Verbrauch ueber Luecke [strom_gesamt] = %v, want 400 (500-100, ueber den laengeren Zeitraum)", v["strom_gesamt"])
	}
}

func TestUpdatePeriod_Roundtrip(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-09-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	p2 := mustCreatePeriod(t, db, "2026-10-01", baseReadings(map[string]float64{"strom_gesamt": 200}))

	err := UpdatePeriod(db, p2, PeriodInput{
		ReadingDate:             "2026-10-02",
		Strompreis:              Float64(0.25),
		FrischwasserPreis:       Float64(1.50),
		AbwasserPreis:           Float64(5.00),
		HeizungWaermeGewichtung: 0.6,
		Readings:                baseReadings(map[string]float64{"strom_gesamt": 210}),
		Personen:                map[int64]int64{1: 3, 2: 1},
	})
	if err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	latest, err := GetLatestPeriod(db)
	if err != nil {
		t.Fatalf("GetLatestPeriod: %v", err)
	}
	if latest.ID != p2 {
		t.Fatalf("GetLatestPeriod.ID = %d, want %d (UpdatePeriod must not create a new row)", latest.ID, p2)
	}
	if latest.ReadingDate != "2026-10-02" {
		t.Errorf("ReadingDate = %q, want 2026-10-02", latest.ReadingDate)
	}
	if OrZero(latest.Strompreis) != 0.25 || OrZero(latest.FrischwasserPreis) != 1.50 || OrZero(latest.AbwasserPreis) != 5.00 {
		t.Errorf("prices = %v/%v/%v, want 0.25/1.50/5.00", OrZero(latest.Strompreis), OrZero(latest.FrischwasserPreis), OrZero(latest.AbwasserPreis))
	}
	if latest.HeizungWaermeGewichtung != 0.6 {
		t.Errorf("HeizungWaermeGewichtung = %v, want 0.6", latest.HeizungWaermeGewichtung)
	}
	if latest.Readings["strom_gesamt"] != 210 {
		t.Errorf("Readings[strom_gesamt] = %v, want 210", latest.Readings["strom_gesamt"])
	}
	if latest.PersonenByApartment[1] != 3 {
		t.Errorf("PersonenByApartment[1] = %v, want 3", latest.PersonenByApartment[1])
	}

	v, err := Verbrauch(db, p2)
	if err != nil {
		t.Fatalf("Verbrauch: %v", err)
	}
	if v["strom_gesamt"] != 110 {
		t.Errorf("Verbrauch[strom_gesamt] after update = %v, want 110 (210-100, recomputed live)", v["strom_gesamt"])
	}
}

// TestImportPeriods_AllOrNothing verifies ImportPeriods writes every period
// in one shared transaction (Ticket #54): a failure partway through rolls
// back everything from that call, not just the failing input.
func TestImportPeriods_AllOrNothing(t *testing.T) {
	db := openTestDB(t)

	_, err := ImportPeriods(db, []PeriodInput{
		{
			ReadingDate:             "2026-06-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		},
		{
			ReadingDate:             "2026-07-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
			// Teilstand (Ticket #128) means missing Readings/prices are no
			// longer an error - but a nonexistent apartment_id still
			// violates the FK constraint on period_occupancy, so it stays
			// a genuine error trigger.
			Personen: map[int64]int64{999: 1},
		},
	})
	if err == nil {
		t.Fatal("ImportPeriods: want error for the second input's invalid apartment id, got nil")
	}

	all, allErr := AllPeriods(db)
	if allErr != nil {
		t.Fatalf("AllPeriods: %v", allErr)
	}
	if len(all) != 0 {
		t.Errorf("AllPeriods after failed ImportPeriods = %d, want 0 (all-or-nothing rollback)", len(all))
	}
}

// TestImportPeriods_Success verifies a clean multi-period import lands all
// rows and returns their ids in input order.
func TestImportPeriods_Success(t *testing.T) {
	db := openTestDB(t)

	ids, err := ImportPeriods(db, []PeriodInput{
		{
			ReadingDate:             "2026-06-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(map[string]float64{"strom_gesamt": 100}),
			Personen:                map[int64]int64{1: 2, 2: 1},
		},
		{
			ReadingDate:             "2026-07-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(map[string]float64{"strom_gesamt": 200}),
			Personen:                map[int64]int64{1: 2, 2: 1},
		},
	})
	if err != nil {
		t.Fatalf("ImportPeriods: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("ImportPeriods returned %d ids, want 2", len(ids))
	}

	all, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("AllPeriods = %d, want 2", len(all))
	}
}

// TestAllPeriodDetails_OrderAndData verifies AllPeriodDetails returns every
// period oldest-first with its full readings/occupancy attached (Ticket
// #53's CSV export data source).
func TestAllPeriodDetails_OrderAndData(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-07-01", baseReadings(map[string]float64{"strom_gesamt": 200}))
	p2 := mustCreatePeriod(t, db, "2026-06-01", baseReadings(map[string]float64{"strom_gesamt": 100}))

	details, err := AllPeriodDetails(db)
	if err != nil {
		t.Fatalf("AllPeriodDetails: %v", err)
	}
	if len(details) != 2 {
		t.Fatalf("AllPeriodDetails = %d, want 2", len(details))
	}
	if details[0].ID != p2 || details[1].ID != p1 {
		t.Errorf("AllPeriodDetails order = [%d, %d], want [%d, %d] (oldest first)", details[0].ID, details[1].ID, p2, p1)
	}
	if details[0].Readings["strom_gesamt"] != 100 {
		t.Errorf("details[0].Readings[strom_gesamt] = %v, want 100", details[0].Readings["strom_gesamt"])
	}
	if details[0].PersonenByApartment[1] != 2 || details[0].PersonenByApartment[2] != 1 {
		t.Errorf("details[0].PersonenByApartment = %v, want {1:2, 2:1}", details[0].PersonenByApartment)
	}
}

// TestUpdatePeriod_AddsMissingMeterReading verifies UpdatePeriod can set a
// meter's reading for a period that never had one - e.g. an old period that
// predates a meter (strom_einspeisung before Ticket #47). It used to be a
// blind UPDATE, which silently dropped the value when no row existed yet to
// match; it's an upsert now.
func TestUpdatePeriod_AddsMissingMeterReading(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-06-01", baseReadings(nil))

	if _, err := db.Exec(`DELETE FROM meter_readings WHERE period_id = ? AND meter_id = (SELECT id FROM meters WHERE key = 'strom_einspeisung')`, p1); err != nil {
		t.Fatalf("simulate missing reading: %v", err)
	}

	if err := UpdatePeriod(db, p1, PeriodInput{
		ReadingDate:             "2026-06-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(map[string]float64{"strom_einspeisung": 1234}),
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	details, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if details.Readings["strom_einspeisung"] != 1234 {
		t.Errorf("Readings[strom_einspeisung] = %v, want 1234", details.Readings["strom_einspeisung"])
	}
}

// TestUpdatePeriod_AddsMissingOccupancy is TestUpdatePeriod_AddsMissingMeterReading's
// counterpart for period_occupancy - e.g. an apartment added after the
// period was first created.
func TestUpdatePeriod_AddsMissingOccupancy(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-06-01", baseReadings(nil))

	if _, err := db.Exec(`DELETE FROM period_occupancy WHERE period_id = ? AND apartment_id = 2`, p1); err != nil {
		t.Fatalf("simulate missing occupancy: %v", err)
	}

	if err := UpdatePeriod(db, p1, PeriodInput{
		ReadingDate:             "2026-06-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
		Personen:                map[int64]int64{1: 2, 2: 3},
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	details, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if details.PersonenByApartment[2] != 3 {
		t.Errorf("PersonenByApartment[2] = %v, want 3", details.PersonenByApartment[2])
	}
}

// TestDateNeighborBounds verifies the "korrigieren" date-reorder guard
// (Ticket #44 review finding): a period's own date is excluded from the
// bounds computation, and only the closest neighbors on either side count.
func TestDateNeighborBounds(t *testing.T) {
	all := []PeriodSummary{
		{ID: 1, ReadingDate: "2026-06-01"},
		{ID: 2, ReadingDate: "2026-07-01"},
		{ID: 3, ReadingDate: "2026-08-01"},
	}

	prev, next, hasPrev, hasNext := dateNeighborBounds(all, 2, "2026-07-01")
	if !hasPrev || prev != "2026-06-01" {
		t.Errorf("prev = %q, %v, want 2026-06-01, true", prev, hasPrev)
	}
	if !hasNext || next != "2026-08-01" {
		t.Errorf("next = %q, %v, want 2026-08-01, true", next, hasNext)
	}

	// Oldest period: no prev bound.
	_, _, hasPrev, _ = dateNeighborBounds(all, 1, "2026-06-01")
	if hasPrev {
		t.Error("oldest period: hasPrev = true, want false")
	}

	// Newest period: no next bound.
	_, _, _, hasNext = dateNeighborBounds(all, 3, "2026-08-01")
	if hasNext {
		t.Error("newest period: hasNext = true, want false")
	}
}

// TestUpdatePeriod_DateConflict proves UpdatePeriod itself rejects a date
// moved past a chronological neighbor - not just the web wizard that used to
// be the only caller checking this (architecture review: the invariant now
// lives behind UpdatePeriod's own interface, so every caller is protected).
func TestUpdatePeriod_DateConflict(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-06-01", baseReadings(nil))
	p2 := mustCreatePeriod(t, db, "2026-07-01", baseReadings(nil))
	mustCreatePeriod(t, db, "2026-08-01", baseReadings(nil))

	t.Run("date at or before the previous neighbor", func(t *testing.T) {
		err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-06-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		})
		var tooEarly *PeriodDateTooEarlyError
		if !errors.As(err, &tooEarly) {
			t.Fatalf("UpdatePeriod err = %v, want *PeriodDateTooEarlyError", err)
		}
		if tooEarly.Neighbor != "2026-06-01" {
			t.Errorf("Neighbor = %q, want 2026-06-01", tooEarly.Neighbor)
		}
	})

	t.Run("date at or after the next neighbor", func(t *testing.T) {
		err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-08-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		})
		var tooLate *PeriodDateTooLateError
		if !errors.As(err, &tooLate) {
			t.Fatalf("UpdatePeriod err = %v, want *PeriodDateTooLateError", err)
		}
		if tooLate.Neighbor != "2026-08-01" {
			t.Errorf("Neighbor = %q, want 2026-08-01", tooLate.Neighbor)
		}
	})

	t.Run("date within the gap stays allowed", func(t *testing.T) {
		if err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-07-15",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		}); err != nil {
			t.Fatalf("UpdatePeriod: %v", err)
		}
	})
}

// TestUpdatePeriod_MonatConflict verifies Issue #86: UpdatePeriod rejects a
// monat moved before the chronologically previous period's monat or after
// the next one's - but allows it to equal a neighbor's monat (multiple
// Ablesungen may share an Abrechnungsmonat).
func TestUpdatePeriod_MonatConflict(t *testing.T) {
	db := openTestDB(t)
	create := func(date string) int64 {
		id, err := CreatePeriod(db, PeriodInput{ReadingDate: date, Monat: date, HeizungWaermeGewichtung: 0.7, Readings: baseReadings(nil)})
		if err != nil {
			t.Fatalf("CreatePeriod %s: %v", date, err)
		}
		return id
	}
	create("2026-06-01")
	p2 := create("2026-07-01")
	create("2026-08-01")

	t.Run("monat vor dem der vorherigen Ablesung", func(t *testing.T) {
		err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-07-01",
			Monat:                   "2026-05-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		})
		var tooEarly *PeriodMonatTooEarlyError
		if !errors.As(err, &tooEarly) {
			t.Fatalf("UpdatePeriod err = %v, want *PeriodMonatTooEarlyError", err)
		}
		if tooEarly.Neighbor != "2026-06-01" {
			t.Errorf("Neighbor = %q, want 2026-06-01", tooEarly.Neighbor)
		}
	})

	t.Run("monat nach dem der naechsten Ablesung", func(t *testing.T) {
		err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-07-01",
			Monat:                   "2026-09-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		})
		var tooLate *PeriodMonatTooLateError
		if !errors.As(err, &tooLate) {
			t.Fatalf("UpdatePeriod err = %v, want *PeriodMonatTooLateError", err)
		}
		if tooLate.Neighbor != "2026-08-01" {
			t.Errorf("Neighbor = %q, want 2026-08-01", tooLate.Neighbor)
		}
	})

	t.Run("monat gleich dem eines Nachbarn ist erlaubt", func(t *testing.T) {
		if err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-07-01",
			Monat:                   "2026-06-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		}); err != nil {
			t.Errorf("UpdatePeriod mit monat gleich dem Vorgaenger: %v", err)
		}
		if err := UpdatePeriod(db, p2, PeriodInput{
			ReadingDate:             "2026-07-01",
			Monat:                   "2026-08-01",
			HeizungWaermeGewichtung: 0.7,
			Readings:                baseReadings(nil),
		}); err != nil {
			t.Errorf("UpdatePeriod mit monat gleich dem Nachfolger: %v", err)
		}
	})
}

// TestCreatePeriod_MonatUnvalidated verifies Issue #86: unlike UpdatePeriod,
// CreatePeriod does not enforce the monat-monotonicity invariant - matching
// the existing behavior for reading_date, which also isn't checked against
// neighbors on creation, only on correction.
func TestCreatePeriod_MonatUnvalidated(t *testing.T) {
	db := openTestDB(t)
	if _, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-08-01",
		Monat:                   "2026-08-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	}); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	// A chronologically later period with an earlier monat - would be
	// rejected by UpdatePeriod, but CreatePeriod lets it through.
	if _, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-09-01",
		Monat:                   "2026-07-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	}); err != nil {
		t.Errorf("CreatePeriod with out-of-order monat: %v, want no error", err)
	}
}

func TestUpdatePeriod_UnknownID(t *testing.T) {
	db := openTestDB(t)
	err := UpdatePeriod(db, 999, PeriodInput{
		ReadingDate:             "2026-10-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	})
	if err == nil {
		t.Fatal("UpdatePeriod on unknown id: want error, got nil")
	}
}

// TestGetPeriodDetails_ArbitraryPeriod verifies GetPeriodDetails (Ticket
// #43/#44's generalization of GetLatestPeriod) returns the right period's
// data by id, not just the latest one.
func TestGetPeriodDetails_ArbitraryPeriod(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-09-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	mustCreatePeriod(t, db, "2026-10-01", baseReadings(map[string]float64{"strom_gesamt": 200}))

	got, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if got == nil {
		t.Fatal("GetPeriodDetails: want the older period, got nil")
	}
	if got.ID != p1 || got.ReadingDate != "2026-09-01" {
		t.Errorf("GetPeriodDetails = {ID:%d, ReadingDate:%q}, want {%d, 2026-09-01}", got.ID, got.ReadingDate, p1)
	}
	if got.Readings["strom_gesamt"] != 100 {
		t.Errorf("GetPeriodDetails.Readings[strom_gesamt] = %v, want 100", got.Readings["strom_gesamt"])
	}
}

func TestGetPeriodDetails_UnknownID(t *testing.T) {
	db := openTestDB(t)
	got, err := GetPeriodDetails(db, 999)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if got != nil {
		t.Errorf("GetPeriodDetails(999) = %+v, want nil", got)
	}
}

// TestPeriodReadingsBefore_ArbitraryTarget verifies the Ausreißer-Baseline
// lookup works against any target period, not just the latest one (Ticket
// #44: "korrigieren" is no longer limited to the latest Ablesung).
func TestPeriodReadingsBefore_ArbitraryTarget(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-06-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	p2 := mustCreatePeriod(t, db, "2026-07-01", baseReadings(map[string]float64{"strom_gesamt": 200}))
	mustCreatePeriod(t, db, "2026-08-01", baseReadings(map[string]float64{"strom_gesamt": 300}))

	before, err := PeriodReadingsBefore(db, p2, 4)
	if err != nil {
		t.Fatalf("PeriodReadingsBefore: %v", err)
	}
	if len(before) != 1 || before[0].ID != p1 {
		t.Fatalf("PeriodReadingsBefore(p2) = %+v, want just [p1]", before)
	}
}

func TestDeletePeriod_MiddlePeriod_Roundtrip(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-06-01", baseReadings(map[string]float64{"strom_gesamt": 100}))
	p2 := mustCreatePeriod(t, db, "2026-07-01", baseReadings(map[string]float64{"strom_gesamt": 250}))
	p3 := mustCreatePeriod(t, db, "2026-08-01", baseReadings(map[string]float64{"strom_gesamt": 500}))

	if err := DeletePeriod(db, p2); err != nil {
		t.Fatalf("DeletePeriod: %v", err)
	}

	all, err := AllPeriods(db)
	if err != nil {
		t.Fatalf("AllPeriods: %v", err)
	}
	if len(all) != 2 || all[0].ID != p3 || all[1].ID != p1 {
		t.Fatalf("AllPeriods after delete = %+v, want [p3, p1]", all)
	}

	if got, err := GetPeriodDetails(db, p2); err != nil || got != nil {
		t.Fatalf("GetPeriodDetails(deleted p2) = %+v, %v, want nil, nil", got, err)
	}

	// p3's Verbrauch must now diff against p1 (the new previous period),
	// recomputed live since nothing caches consumption.
	v, err := Verbrauch(db, p3)
	if err != nil {
		t.Fatalf("Verbrauch: %v", err)
	}
	if v["strom_gesamt"] != 400 {
		t.Errorf("Verbrauch[strom_gesamt] after deleting p2 = %v, want 400 (500-100)", v["strom_gesamt"])
	}
}

func TestDeletePeriod_UnknownID(t *testing.T) {
	db := openTestDB(t)
	if err := DeletePeriod(db, 999); err == nil {
		t.Fatal("DeletePeriod on unknown id: want error, got nil")
	}
}

func TestEnsurePeriodsHeizungGewichtungColumn(t *testing.T) {
	t.Run("fuegt Spalte zu einer alten Tabelle ohne sie hinzu, mit Default 0.7", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		// Pre-#27 schema: periods without heizung_waerme_gewichtung.
		if _, err := db.Exec(`CREATE TABLE periods (
			id                 INTEGER PRIMARY KEY,
			reading_date       TEXT NOT NULL,
			strompreis         REAL NOT NULL,
			frischwasser_preis REAL NOT NULL,
			abwasser_preis     REAL NOT NULL
		)`); err != nil {
			t.Fatalf("create old-shape periods table: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO periods (reading_date, strompreis, frischwasser_preis, abwasser_preis) VALUES (?, ?, ?, ?)`,
			"2026-01-01", 0.22, 1.46, 4.87,
		); err != nil {
			t.Fatalf("insert pre-existing row: %v", err)
		}

		if err := ensurePeriodsHeizungGewichtungColumn(db); err != nil {
			t.Fatalf("ensurePeriodsHeizungGewichtungColumn: %v", err)
		}

		var gewichtung float64
		if err := db.QueryRow(`SELECT heizung_waerme_gewichtung FROM periods WHERE reading_date = '2026-01-01'`).Scan(&gewichtung); err != nil {
			t.Fatalf("query migrated column: %v", err)
		}
		if gewichtung != 0.7 {
			t.Errorf("pre-existing row's heizung_waerme_gewichtung = %v, want 0.7 (backfilled default)", gewichtung)
		}

		// Idempotent: a second call must not fail with "duplicate column".
		if err := ensurePeriodsHeizungGewichtungColumn(db); err != nil {
			t.Fatalf("second ensurePeriodsHeizungGewichtungColumn call: %v", err)
		}
	})

	t.Run("neue Tabelle hat die Spalte bereits - no-op", func(t *testing.T) {
		db := openTestDB(t)
		if err := ensurePeriodsHeizungGewichtungColumn(db); err != nil {
			t.Fatalf("ensurePeriodsHeizungGewichtungColumn on an already-current schema: %v", err)
		}
	})
}

func TestEnsurePeriodsEinspeisungPreisColumn(t *testing.T) {
	t.Run("fuegt Spalte zu einer alten Tabelle ohne sie hinzu, mit Default 0", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		// Pre-#47 schema: periods without einspeisung_preis.
		if _, err := db.Exec(`CREATE TABLE periods (
			id                        INTEGER PRIMARY KEY,
			reading_date              TEXT NOT NULL,
			strompreis                REAL NOT NULL,
			frischwasser_preis        REAL NOT NULL,
			abwasser_preis            REAL NOT NULL,
			heizung_waerme_gewichtung REAL NOT NULL DEFAULT 0.7
		)`); err != nil {
			t.Fatalf("create old-shape periods table: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO periods (reading_date, strompreis, frischwasser_preis, abwasser_preis) VALUES (?, ?, ?, ?)`,
			"2026-01-01", 0.22, 1.46, 4.87,
		); err != nil {
			t.Fatalf("insert pre-existing row: %v", err)
		}

		if err := ensurePeriodsEinspeisungPreisColumn(db); err != nil {
			t.Fatalf("ensurePeriodsEinspeisungPreisColumn: %v", err)
		}

		var preis float64
		if err := db.QueryRow(`SELECT einspeisung_preis FROM periods WHERE reading_date = '2026-01-01'`).Scan(&preis); err != nil {
			t.Fatalf("query migrated column: %v", err)
		}
		if preis != 0 {
			t.Errorf("pre-existing row's einspeisung_preis = %v, want 0 (backfilled default)", preis)
		}

		// Idempotent: a second call must not fail with "duplicate column".
		if err := ensurePeriodsEinspeisungPreisColumn(db); err != nil {
			t.Fatalf("second ensurePeriodsEinspeisungPreisColumn call: %v", err)
		}
	})

	t.Run("neue Tabelle hat die Spalte bereits - no-op", func(t *testing.T) {
		db := openTestDB(t)
		if err := ensurePeriodsEinspeisungPreisColumn(db); err != nil {
			t.Fatalf("ensurePeriodsEinspeisungPreisColumn on an already-current schema: %v", err)
		}
	})
}

// TestEnsurePeriodsMonatColumn verifies Issue #86's migration: an existing
// installation's periods get monat backfilled from their own reading_date
// (YYYY-MM-01), and a second call doesn't clobber an already-set value.
func TestEnsurePeriodsMonatColumn(t *testing.T) {
	t.Run("fuegt Spalte zu einer alten Tabelle ohne sie hinzu, backfillt aus reading_date", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		// Pre-#86 schema: periods without monat.
		if _, err := db.Exec(`CREATE TABLE periods (
			id                 INTEGER PRIMARY KEY,
			reading_date       TEXT NOT NULL,
			strompreis         REAL NOT NULL,
			frischwasser_preis REAL NOT NULL,
			abwasser_preis     REAL NOT NULL
		)`); err != nil {
			t.Fatalf("create old-shape periods table: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO periods (reading_date, strompreis, frischwasser_preis, abwasser_preis) VALUES (?, ?, ?, ?)`,
			"2026-08-31", 0.22, 1.46, 4.87,
		); err != nil {
			t.Fatalf("insert pre-existing row: %v", err)
		}

		if err := ensurePeriodsMonatColumn(db); err != nil {
			t.Fatalf("ensurePeriodsMonatColumn: %v", err)
		}

		var monat string
		if err := db.QueryRow(`SELECT monat FROM periods WHERE reading_date = '2026-08-31'`).Scan(&monat); err != nil {
			t.Fatalf("query migrated column: %v", err)
		}
		if monat != "2026-08-01" {
			t.Errorf("pre-existing row's monat = %q, want 2026-08-01 (backfilled from reading_date)", monat)
		}

		// Idempotent: a second call must not fail with "duplicate column".
		if err := ensurePeriodsMonatColumn(db); err != nil {
			t.Fatalf("second ensurePeriodsMonatColumn call: %v", err)
		}
	})

	t.Run("zweiter Aufruf ueberschreibt einen bereits gesetzten Wert nicht", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		if _, err := db.Exec(`CREATE TABLE periods (
			id                 INTEGER PRIMARY KEY,
			reading_date       TEXT NOT NULL,
			strompreis         REAL NOT NULL,
			frischwasser_preis REAL NOT NULL,
			abwasser_preis     REAL NOT NULL
		)`); err != nil {
			t.Fatalf("create old-shape periods table: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO periods (reading_date, strompreis, frischwasser_preis, abwasser_preis) VALUES (?, ?, ?, ?)`,
			"2026-08-31", 0.22, 1.46, 4.87,
		); err != nil {
			t.Fatalf("insert pre-existing row: %v", err)
		}
		if err := ensurePeriodsMonatColumn(db); err != nil {
			t.Fatalf("first ensurePeriodsMonatColumn: %v", err)
		}
		if _, err := db.Exec(`UPDATE periods SET monat = '2026-09-01' WHERE reading_date = '2026-08-31'`); err != nil {
			t.Fatalf("simulate manual override: %v", err)
		}

		if err := ensurePeriodsMonatColumn(db); err != nil {
			t.Fatalf("second ensurePeriodsMonatColumn: %v", err)
		}

		var monat string
		if err := db.QueryRow(`SELECT monat FROM periods WHERE reading_date = '2026-08-31'`).Scan(&monat); err != nil {
			t.Fatalf("query column: %v", err)
		}
		if monat != "2026-09-01" {
			t.Errorf("monat = %q, want 2026-09-01 (manual override must survive a re-run)", monat)
		}
	})

	t.Run("neue Tabelle hat die Spalte bereits - no-op", func(t *testing.T) {
		db := openTestDB(t)
		if err := ensurePeriodsMonatColumn(db); err != nil {
			t.Fatalf("ensurePeriodsMonatColumn on an already-current schema: %v", err)
		}
	})
}

// TestSeed_AddsNewMeterToExistingDB verifies Ticket #47: seed() runs on
// every Open() and is additive via ON CONFLICT DO NOTHING, so a meter added
// after a database was first created (like strom_einspeisung) still gets
// backfilled into an existing installation without a dedicated migration.
func TestSeed_AddsNewMeterToExistingDB(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.Exec(`DELETE FROM meters WHERE key = 'strom_einspeisung'`); err != nil {
		t.Fatalf("simulate pre-Ticket-#47 db: %v", err)
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM meters WHERE key = 'strom_einspeisung'`).Scan(&count); err != nil {
		t.Fatalf("count meters: %v", err)
	}
	if count != 0 {
		t.Fatalf("setup: strom_einspeisung still present after DELETE")
	}

	if err := seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := db.QueryRow(`SELECT count(*) FROM meters WHERE key = 'strom_einspeisung'`).Scan(&count); err != nil {
		t.Fatalf("count meters after seed: %v", err)
	}
	if count != 1 {
		t.Errorf("strom_einspeisung meters count after seed = %d, want 1 (backfilled)", count)
	}
}

func TestEinspeisungPreis_Roundtrip(t *testing.T) {
	db := openTestDB(t)
	p1 := mustCreatePeriod(t, db, "2026-09-01", baseReadings(nil))

	if err := UpdatePeriod(db, p1, PeriodInput{
		ReadingDate:             "2026-09-01",
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       Float64(1.46),
		AbwasserPreis:           Float64(4.87),
		HeizungWaermeGewichtung: 0.7,
		EinspeisungPreis:        Float64(0.082),
		Readings:                baseReadings(nil),
		Personen:                map[int64]int64{1: 2, 2: 1},
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	got, err := GetPeriodDetails(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if OrZero(got.EinspeisungPreis) != 0.082 {
		t.Errorf("EinspeisungPreis = %v, want 0.082", OrZero(got.EinspeisungPreis))
	}

	byID, err := GetPeriodByID(db, p1)
	if err != nil {
		t.Fatalf("GetPeriodByID: %v", err)
	}
	if byID.EinspeisungPreis != 0.082 {
		t.Errorf("GetPeriodByID.EinspeisungPreis = %v, want 0.082", byID.EinspeisungPreis)
	}
}

// TestEnsureApartmentsFlurstueckGroesseColumn verifies Issue #61's
// migration is additive: an existing installation's apartments.qm value
// (Ticket #38) survives, and flurstueck_groesse backfills to 0.
func TestEnsureApartmentsFlurstueckGroesseColumn(t *testing.T) {
	t.Run("fuegt Spalte zu einer alten Tabelle ohne sie hinzu, mit Default 0, Bestandsdaten bleiben erhalten", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
		if err != nil {
			t.Fatalf("open sqlite: %v", err)
		}
		t.Cleanup(func() { db.Close() })

		// Pre-#61 schema: apartments without flurstueck_groesse.
		if _, err := db.Exec(`CREATE TABLE apartments (
			id   INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			qm   REAL NOT NULL
		)`); err != nil {
			t.Fatalf("create old-shape apartments table: %v", err)
		}
		if _, err := db.Exec(
			`INSERT INTO apartments (id, name, qm) VALUES (1, 'Wohnung 1', 116.23)`,
		); err != nil {
			t.Fatalf("insert pre-existing row: %v", err)
		}

		if err := ensureApartmentsFlurstueckGroesseColumn(db); err != nil {
			t.Fatalf("ensureApartmentsFlurstueckGroesseColumn: %v", err)
		}

		var qm, flurstueckGroesse float64
		if err := db.QueryRow(`SELECT qm, flurstueck_groesse FROM apartments WHERE id = 1`).Scan(&qm, &flurstueckGroesse); err != nil {
			t.Fatalf("query migrated column: %v", err)
		}
		if qm != 116.23 {
			t.Errorf("pre-existing row's qm = %v, want 116.23 (unchanged by the migration)", qm)
		}
		if flurstueckGroesse != 0 {
			t.Errorf("pre-existing row's flurstueck_groesse = %v, want 0 (backfilled default)", flurstueckGroesse)
		}

		// Idempotent: a second call must not fail with "duplicate column".
		if err := ensureApartmentsFlurstueckGroesseColumn(db); err != nil {
			t.Fatalf("second ensureApartmentsFlurstueckGroesseColumn call: %v", err)
		}
	})

	t.Run("neue Tabelle hat die Spalte bereits - no-op", func(t *testing.T) {
		db := openTestDB(t)
		if err := ensureApartmentsFlurstueckGroesseColumn(db); err != nil {
			t.Fatalf("ensureApartmentsFlurstueckGroesseColumn on an already-current schema: %v", err)
		}
	})
}

// TestUpdateStammdaten_Roundtrip verifies the /stammdaten page's write path
// (Issue #61): both apartments' Wohnungsgröße/Flurstücksgröße persist and
// read back via Apartments(), and a partial input only touches the
// apartments it names.
func TestUpdateStammdaten_Roundtrip(t *testing.T) {
	db := openTestDB(t)

	if err := UpdateStammdaten(db, map[int64]StammdatenInput{
		1: {QM: 116.23, FlurstueckGroesse: 450.5},
		2: {QM: 86, FlurstueckGroesse: 300},
	}); err != nil {
		t.Fatalf("UpdateStammdaten: %v", err)
	}

	apartments, err := Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}
	var w1, w2 Apartment
	for _, a := range apartments {
		switch a.ID {
		case 1:
			w1 = a
		case 2:
			w2 = a
		}
	}
	if w1.QM != 116.23 || w1.FlurstueckGroesse != 450.5 {
		t.Errorf("Wohnung 1 = QM:%v FlurstueckGroesse:%v, want QM:116.23 FlurstueckGroesse:450.5", w1.QM, w1.FlurstueckGroesse)
	}
	if w2.QM != 86 || w2.FlurstueckGroesse != 300 {
		t.Errorf("Wohnung 2 = QM:%v FlurstueckGroesse:%v, want QM:86 FlurstueckGroesse:300", w2.QM, w2.FlurstueckGroesse)
	}

	// Partial update: only Wohnung 1 given, Wohnung 2 must stay untouched.
	if err := UpdateStammdaten(db, map[int64]StammdatenInput{
		1: {QM: 120, FlurstueckGroesse: 500},
	}); err != nil {
		t.Fatalf("UpdateStammdaten (partial): %v", err)
	}
	apartments, err = Apartments(db)
	if err != nil {
		t.Fatalf("Apartments: %v", err)
	}
	for _, a := range apartments {
		if a.ID == 1 && (a.QM != 120 || a.FlurstueckGroesse != 500) {
			t.Errorf("Wohnung 1 after partial update = QM:%v FlurstueckGroesse:%v, want QM:120 FlurstueckGroesse:500", a.QM, a.FlurstueckGroesse)
		}
		if a.ID == 2 && (a.QM != 86 || a.FlurstueckGroesse != 300) {
			t.Errorf("Wohnung 2 after partial update = QM:%v FlurstueckGroesse:%v, want unchanged QM:86 FlurstueckGroesse:300", a.QM, a.FlurstueckGroesse)
		}
	}
}

// TestEnsureFixkostenWerteLogikTypColumns covers Issue #106: an existing
// installation whose fixkosten_werte doesn't yet have logik/typ columns
// gets them added, and every existing Fixkosten-Eingabe gets backfilled
// retroactively from kostenpositionen_jahre - annual value for Typ
// "jaehrlich", explicit value or last-known annual-value/12 fallback for
// "monatlich" (identical to calc.Fixkosten.monatswertFuer, but here just
// once at migration time instead of on every Berechnung).
func TestEnsureFixkostenWerteLogikTypColumns(t *testing.T) {
	// kostenpositionen_jahre no longer exists in the regular schema
	// (openTestDB) since #109 - this test therefore simulates directly via
	// SQL an installation from before #106/#109, where it still existed.
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	for _, stmt := range []string{
		`CREATE TABLE kostenpositionen (id INTEGER PRIMARY KEY, key TEXT NOT NULL UNIQUE, label TEXT NOT NULL)`,
		`CREATE TABLE kostenpositionen_jahre (
			id                INTEGER PRIMARY KEY,
			kostenposition_id INTEGER NOT NULL,
			jahr              INTEGER NOT NULL,
			logik             TEXT NOT NULL,
			typ               TEXT NOT NULL,
			jahreswert        REAL NOT NULL DEFAULT 0,
			UNIQUE(kostenposition_id, jahr)
		)`,
		`CREATE TABLE fixkosten_eingaben (id INTEGER PRIMARY KEY, monat TEXT NOT NULL)`,
		`CREATE TABLE fixkosten_werte (
			id                   INTEGER PRIMARY KEY,
			fixkosten_eingabe_id INTEGER NOT NULL,
			kostenposition_id    INTEGER NOT NULL,
			wert                 REAL NOT NULL,
			logik                TEXT NOT NULL DEFAULT '',
			typ                  TEXT NOT NULL DEFAULT '',
			UNIQUE(fixkosten_eingabe_id, kostenposition_id)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("create old-shape schema: %v", err)
		}
	}

	if _, err := db.Exec(`INSERT INTO kostenpositionen (id, key, label) VALUES (1, 'grundsteuer', 'Grundsteuer'), (13, 'internet', 'Grundgebühr Internet')`); err != nil {
		t.Fatalf("seed kostenpositionen: %v", err)
	}

	// Jahr 2025: Grundsteuer (ID 1) jaehrlich with annual value 1200,
	// Internet (ID 13) monatlich. Jahr 2024: Internet was jaehrlich with
	// annual value 480 - fallback source for a monthly Eingabe without
	// its own value.
	if _, err := db.Exec(
		`INSERT INTO kostenpositionen_jahre (kostenposition_id, jahr, logik, typ, jahreswert) VALUES
		 (1, 2025, ?, ?, 1200), (13, 2025, ?, ?, 0), (13, 2024, ?, ?, 480)`,
		LogikQM, TypJaehrlich, LogikWohneinheit, TypMonatlich, LogikWohneinheit, TypJaehrlich,
	); err != nil {
		t.Fatalf("seed kostenpositionen_jahre: %v", err)
	}

	// Eingabe with an explicit Internet value (old schema: logik/typ
	// empty, only wert set - exactly the state from before this
	// migration).
	res, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2025-01-01')`)
	if err != nil {
		t.Fatalf("insert fixkosten_eingabe (mit Wert): %v", err)
	}
	mitWert, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("mitWert id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO fixkosten_werte (fixkosten_eingabe_id, kostenposition_id, wert) VALUES (?, 13, 42)`, mitWert); err != nil {
		t.Fatalf("insert fixkosten_werte (mit Wert): %v", err)
	}

	// Eingabe without its own Internet value - must fall back to the 2024
	// annual value / 12.
	res, err = db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2025-02-01')`)
	if err != nil {
		t.Fatalf("insert fixkosten_eingabe (ohne Wert): %v", err)
	}
	ohneWert, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("ohneWert id: %v", err)
	}

	if err := backfillFixkostenWerteLogikTyp(db); err != nil {
		t.Fatalf("backfillFixkostenWerteLogikTyp: %v", err)
	}

	type row struct {
		logik, typ string
		wert       float64
	}
	get := func(eingabeID, kostenpositionID int64) row {
		t.Helper()
		var r row
		if err := db.QueryRow(
			`SELECT logik, typ, wert FROM fixkosten_werte WHERE fixkosten_eingabe_id = ? AND kostenposition_id = ?`,
			eingabeID, kostenpositionID,
		).Scan(&r.logik, &r.typ, &r.wert); err != nil {
			t.Fatalf("query fixkosten_werte eingabe=%d position=%d: %v", eingabeID, kostenpositionID, err)
		}
		return r
	}

	if got := get(mitWert, 1); got.logik != LogikQM || got.typ != TypJaehrlich || got.wert != 1200 {
		t.Errorf("Grundsteuer (mitWert) = %+v, want {qm jaehrlich 1200} (Jahreswert, nicht /12 - das teilt calc.Fixkosten)", got)
	}
	if got := get(mitWert, 13); got.logik != LogikWohneinheit || got.typ != TypMonatlich || got.wert != 42 {
		t.Errorf("Internet (mitWert) = %+v, want {wohneinheit monatlich 42} (expliziter Wert bleibt erhalten)", got)
	}
	if got := get(ohneWert, 13); got.logik != LogikWohneinheit || got.typ != TypMonatlich || got.wert != 40 {
		t.Errorf("Internet (ohneWert) = %+v, want {wohneinheit monatlich 40} (Fallback 480/12 aus 2024)", got)
	}

	// Idempotent: a second call must not fail and must not change
	// anything.
	if err := backfillFixkostenWerteLogikTyp(db); err != nil {
		t.Fatalf("second backfillFixkostenWerteLogikTyp call: %v", err)
	}
	if got := get(mitWert, 13); got.wert != 42 {
		t.Errorf("Internet (mitWert) nach 2. Lauf = %v, want unveraendert 42", got.wert)
	}
}

// TestEnsureFixkostenWerteLogikTypColumns_AlteInstallation covers the
// actual column migration (Issue #106): a table in the pre-#106 schema
// (without logik/typ) gets the columns added, and a second call doesn't
// fail with "duplicate column".
func TestEnsureFixkostenWerteLogikTypColumns_AlteInstallation(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "old.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec(`CREATE TABLE fixkosten_werte (
		id                   INTEGER PRIMARY KEY,
		fixkosten_eingabe_id INTEGER NOT NULL,
		kostenposition_id    INTEGER NOT NULL,
		wert                 REAL NOT NULL,
		UNIQUE(fixkosten_eingabe_id, kostenposition_id)
	)`); err != nil {
		t.Fatalf("create old-shape fixkosten_werte table: %v", err)
	}
	// backfillFixkostenWerteLogikTyp reads fixkosten_eingaben, even if
	// there are none (as here) - the table still has to exist.
	if _, err := db.Exec(`CREATE TABLE fixkosten_eingaben (id INTEGER PRIMARY KEY, monat TEXT NOT NULL)`); err != nil {
		t.Fatalf("create fixkosten_eingaben table: %v", err)
	}

	if err := ensureFixkostenWerteLogikTypColumns(db); err != nil {
		t.Fatalf("ensureFixkostenWerteLogikTypColumns: %v", err)
	}
	if err := ensureFixkostenWerteLogikTypColumns(db); err != nil {
		t.Fatalf("second ensureFixkostenWerteLogikTypColumns call: %v", err)
	}
}

// --- Ticket #128 (Teilstand): Store-Ebene ---

// columnNotNull reports whether table.column is currently declared NOT
// NULL, via PRAGMA table_info - the same inspection
// periodsStrompreisNullable does internally, duplicated here so the test
// doesn't need a *sql.Conn just to assert on schema state.
func columnNotNull(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("PRAGMA table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == column {
			return notnull != 0
		}
	}
	t.Fatalf("%s.%s column not found", table, column)
	return false
}

// TestEnsurePeriodsNullablePriceColumns verifies the table-rebuild
// migration (Ticket #128): it makes periods' 4 price columns nullable,
// preserves existing data and FK-linked meter_readings/period_occupancy
// rows, and is idempotent. "Works on an empty DB" is covered pervasively
// by every other test in this file - openTestDB always runs this migration
// against a brand-new, empty database via Open().
func TestEnsurePeriodsNullablePriceColumns(t *testing.T) {
	t.Run("macht Preis-Spalten einer alten NOT-NULL-Tabelle nullable, Bestandsdaten und FK-Bezüge bleiben erhalten", func(t *testing.T) {
		db := openTestDB(t)

		p1, err := CreatePeriod(db, PeriodInput{
			ReadingDate:             "2026-01-01",
			Monat:                   "2026-01-01",
			Strompreis:              Float64(0.22),
			FrischwasserPreis:       Float64(1.46),
			AbwasserPreis:           Float64(4.87),
			HeizungWaermeGewichtung: 0.7,
			EinspeisungPreis:        Float64(0.08),
			Readings:                baseReadings(map[string]float64{"strom_gesamt": 123}),
			Personen:                map[int64]int64{1: 2, 2: 1},
		})
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}

		// Simulates an existing DB from before Ticket #128: price columns
		// back to NOT NULL - the one existing row has real values
		// everywhere, so the rebuild doesn't fail.
		for _, stmt := range []string{
			`CREATE TABLE periods_old_shape (
			    id                         INTEGER PRIMARY KEY,
			    reading_date               TEXT NOT NULL,
			    strompreis                 REAL NOT NULL,
			    frischwasser_preis         REAL NOT NULL,
			    abwasser_preis             REAL NOT NULL,
			    heizung_waerme_gewichtung  REAL NOT NULL DEFAULT 0.7,
			    einspeisung_preis          REAL NOT NULL DEFAULT 0,
			    monat                      TEXT NOT NULL DEFAULT ''
			)`,
			`INSERT INTO periods_old_shape SELECT * FROM periods`,
			`PRAGMA foreign_keys = OFF`,
			`DROP TABLE periods`,
			`ALTER TABLE periods_old_shape RENAME TO periods`,
			`PRAGMA foreign_keys = ON`,
		} {
			if _, err := db.Exec(stmt); err != nil {
				t.Fatalf("setup old-shape periods (%s): %v", stmt, err)
			}
		}
		if !columnNotNull(t, db, "periods", "strompreis") {
			t.Fatal("setup: expected periods.strompreis to be NOT NULL before migration")
		}

		if err := ensurePeriodsNullablePriceColumns(db); err != nil {
			t.Fatalf("ensurePeriodsNullablePriceColumns: %v", err)
		}

		if columnNotNull(t, db, "periods", "strompreis") {
			t.Error("periods.strompreis still NOT NULL after migration")
		}

		got, err := GetPeriodDetails(db, p1)
		if err != nil {
			t.Fatalf("GetPeriodDetails after migration: %v", err)
		}
		if got.ReadingDate != "2026-01-01" || OrZero(got.Strompreis) != 0.22 || OrZero(got.EinspeisungPreis) != 0.08 {
			t.Errorf("period data after migration = %+v, want ReadingDate=2026-01-01 Strompreis=0.22 EinspeisungPreis=0.08", got)
		}
		if got.Readings["strom_gesamt"] != 123 {
			t.Errorf("Readings[strom_gesamt] after migration = %v, want 123 (FK-verknüpfte meter_readings-Row erhalten)", got.Readings["strom_gesamt"])
		}
		if got.PersonenByApartment[1] != 2 || got.PersonenByApartment[2] != 1 {
			t.Errorf("PersonenByApartment after migration = %v, want {1:2, 2:1} (FK-verknüpfte period_occupancy-Rows erhalten)", got.PersonenByApartment)
		}

		// Idempotent: a second call must not fail or rebuild again.
		if err := ensurePeriodsNullablePriceColumns(db); err != nil {
			t.Fatalf("second ensurePeriodsNullablePriceColumns call: %v", err)
		}
	})

	t.Run("neue Tabelle hat die Spalten schon nullable - no-op", func(t *testing.T) {
		db := openTestDB(t)
		if err := ensurePeriodsNullablePriceColumns(db); err != nil {
			t.Fatalf("ensurePeriodsNullablePriceColumns on an already-current schema: %v", err)
		}
	})
}

// TestCreatePeriod_Teilstand verifies AC2: CreatePeriod accepts nil
// prices, an empty Monat, and Readings/Personen maps that don't cover
// every MeterKeys entry/apartment - no error, the missing values simply
// aren't persisted.
func TestCreatePeriod_Teilstand(t *testing.T) {
	db := openTestDB(t)

	id, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-05-01",
		Monat:                   "", // Teilstand: Abrechnungsmonat fehlt
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       nil, // Teilstand: Preis fehlt
		AbwasserPreis:           nil,
		HeizungWaermeGewichtung: 0.7,
		EinspeisungPreis:        nil,
		Readings:                map[string]float64{"strom_gesamt": 100}, // nur 1 von 10 Metern
		Personen:                map[int64]int64{1: 2},                   // nur 1 von 2 Wohnungen
	})
	if err != nil {
		t.Fatalf("CreatePeriod mit Teilstand-Daten: %v", err)
	}

	got, err := GetPeriodDetails(db, id)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if got.Monat != "" {
		t.Errorf("Monat = %q, want \"\" (nicht erfasst)", got.Monat)
	}
	if got.Strompreis == nil || *got.Strompreis != 0.22 {
		t.Errorf("Strompreis = %v, want 0.22", got.Strompreis)
	}
	if got.FrischwasserPreis != nil {
		t.Errorf("FrischwasserPreis = %v, want nil (nicht erfasst)", *got.FrischwasserPreis)
	}
	if got.AbwasserPreis != nil {
		t.Errorf("AbwasserPreis = %v, want nil (nicht erfasst)", *got.AbwasserPreis)
	}
	if got.EinspeisungPreis != nil {
		t.Errorf("EinspeisungPreis = %v, want nil (nicht erfasst)", *got.EinspeisungPreis)
	}
	if len(got.Readings) != 1 || got.Readings["strom_gesamt"] != 100 {
		t.Errorf("Readings = %v, want nur {strom_gesamt: 100}", got.Readings)
	}
	if len(got.PersonenByApartment) != 1 || got.PersonenByApartment[1] != 2 {
		t.Errorf("PersonenByApartment = %v, want nur {1: 2}", got.PersonenByApartment)
	}

	complete, err := PeriodComplete(db, id)
	if err != nil {
		t.Fatalf("PeriodComplete: %v", err)
	}
	if complete {
		t.Error("PeriodComplete = true, want false (Teilstand)")
	}
}

// TestUpdatePeriod_Teilstand_PreservesUnspecifiedFields verifies AC2 for
// UpdatePeriod (Vervollständigen): a field left nil/empty/absent in this
// call's PeriodInput must not erase what was already stored - only the
// fields actually given in this round get written, the same "don't touch
// what wasn't given" rule Readings/Personen already followed before
// Ticket #128.
func TestUpdatePeriod_Teilstand_PreservesUnspecifiedFields(t *testing.T) {
	db := openTestDB(t)

	id, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-05-01",
		Monat:                   "2026-05-01",
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       Float64(1.46),
		AbwasserPreis:           Float64(4.87),
		HeizungWaermeGewichtung: 0.7,
		EinspeisungPreis:        Float64(0.08),
		Readings:                baseReadings(map[string]float64{"strom_gesamt": 100}),
		Personen:                map[int64]int64{1: 2, 2: 1},
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	// Vervollstaendigen/Korrigieren round: only Strompreis is given this
	// time, everything else stays empty/absent in this call - must not
	// delete the values already stored.
	if err := UpdatePeriod(db, id, PeriodInput{
		ReadingDate:             "2026-05-01",
		Monat:                   "",
		Strompreis:              Float64(0.25),
		FrischwasserPreis:       nil,
		AbwasserPreis:           nil,
		HeizungWaermeGewichtung: 0.7,
		EinspeisungPreis:        nil,
		Readings:                map[string]float64{},
		Personen:                map[int64]int64{},
	}); err != nil {
		t.Fatalf("UpdatePeriod: %v", err)
	}

	got, err := GetPeriodDetails(db, id)
	if err != nil {
		t.Fatalf("GetPeriodDetails: %v", err)
	}
	if got.Monat != "2026-05-01" {
		t.Errorf("Monat = %q, want 2026-05-01 (unverändert, da diese Runde leer war)", got.Monat)
	}
	if OrZero(got.Strompreis) != 0.25 {
		t.Errorf("Strompreis = %v, want 0.25 (diese Runde geändert)", OrZero(got.Strompreis))
	}
	if OrZero(got.FrischwasserPreis) != 1.46 {
		t.Errorf("FrischwasserPreis = %v, want 1.46 (unverändert)", OrZero(got.FrischwasserPreis))
	}
	if OrZero(got.AbwasserPreis) != 4.87 {
		t.Errorf("AbwasserPreis = %v, want 4.87 (unverändert)", OrZero(got.AbwasserPreis))
	}
	if OrZero(got.EinspeisungPreis) != 0.08 {
		t.Errorf("EinspeisungPreis = %v, want 0.08 (unverändert)", OrZero(got.EinspeisungPreis))
	}
	if got.Readings["strom_gesamt"] != 100 {
		t.Errorf("Readings[strom_gesamt] = %v, want 100 (unverändert, da diese Runde keinen neuen Wert mitgab)", got.Readings["strom_gesamt"])
	}
	if got.PersonenByApartment[1] != 2 || got.PersonenByApartment[2] != 1 {
		t.Errorf("PersonenByApartment = %v, want {1:2, 2:1} (unverändert)", got.PersonenByApartment)
	}
}

// TestUpdatePeriod_EmptyMonat_NoNeighborConflict verifies AC4:
// checkMonatNeighbors must not reject an empty Monat as a chronological
// conflict, even when a real (non-empty) value at that position would
// conflict with a neighbor.
func TestUpdatePeriod_EmptyMonat_NoNeighborConflict(t *testing.T) {
	db := openTestDB(t)

	mustCreatePeriod(t, db, "2026-01-01", baseReadings(nil))
	p2 := mustCreatePeriod(t, db, "2026-02-01", baseReadings(nil))
	mustCreatePeriod(t, db, "2026-03-01", baseReadings(nil))

	if err := UpdatePeriod(db, p2, PeriodInput{
		ReadingDate:             "2026-02-01",
		Monat:                   "", // partial reading: would collide with a neighbor as "2026-01-01" or "2026-04-01", but gets skipped
		Strompreis:              Float64(0.22),
		FrischwasserPreis:       Float64(1.46),
		AbwasserPreis:           Float64(4.87),
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
		Personen:                map[int64]int64{1: 2, 2: 1},
	}); err != nil {
		t.Fatalf("UpdatePeriod mit leerem Monat: %v, want kein Fehler (Nachbar-Prüfung übersprungen)", err)
	}
}

// TestPeriodComplete covers AC3: every combination of a missing field
// (Preis, Zählerstand, Personenzahl, Monat) must make PeriodComplete
// report false; only a fully filled period is true.
func TestPeriodComplete(t *testing.T) {
	db := openTestDB(t)

	complete := func(t *testing.T, in PeriodInput) bool {
		t.Helper()
		id, err := CreatePeriod(db, in)
		if err != nil {
			t.Fatalf("CreatePeriod: %v", err)
		}
		ok, err := PeriodComplete(db, id)
		if err != nil {
			t.Fatalf("PeriodComplete: %v", err)
		}
		return ok
	}

	full := func() PeriodInput {
		return PeriodInput{
			ReadingDate:             "2026-05-01",
			Monat:                   "2026-05-01",
			Strompreis:              Float64(0.22),
			FrischwasserPreis:       Float64(1.46),
			AbwasserPreis:           Float64(4.87),
			HeizungWaermeGewichtung: 0.7,
			EinspeisungPreis:        Float64(0.08),
			Readings:                baseReadings(nil),
			Personen:                map[int64]int64{1: 2, 2: 1},
		}
	}

	t.Run("alle Felder gesetzt", func(t *testing.T) {
		if !complete(t, full()) {
			t.Error("PeriodComplete = false, want true")
		}
	})
	t.Run("Preis fehlt", func(t *testing.T) {
		in := full()
		in.EinspeisungPreis = nil
		if complete(t, in) {
			t.Error("PeriodComplete = true, want false (Preis fehlt)")
		}
	})
	t.Run("Zählerstand fehlt", func(t *testing.T) {
		in := full()
		delete(in.Readings, "strom_gesamt")
		if complete(t, in) {
			t.Error("PeriodComplete = true, want false (Zählerstand fehlt)")
		}
	})
	t.Run("Personenzahl fehlt", func(t *testing.T) {
		in := full()
		delete(in.Personen, 2)
		if complete(t, in) {
			t.Error("PeriodComplete = true, want false (Personenzahl fehlt)")
		}
	})
	t.Run("Monat fehlt", func(t *testing.T) {
		in := full()
		in.Monat = ""
		if complete(t, in) {
			t.Error("PeriodComplete = true, want false (Monat fehlt)")
		}
	})

	t.Run("nicht existierende Periode", func(t *testing.T) {
		_, err := PeriodComplete(db, 999999)
		if !errors.Is(err, ErrPeriodNotFound) {
			t.Errorf("PeriodComplete(999999) err = %v, want ErrPeriodNotFound", err)
		}
	})
}

// TestGetPeriodByID_TeilstandPricesReadAsZero verifies GetPeriodByID
// doesn't crash on a Teilstand's NULL prices - it coalesces them to 0,
// since Period only ever feeds calc's Kosten-Berechnung, which never runs
// against an incomplete period by design (see PeriodComplete).
func TestGetPeriodByID_TeilstandPricesReadAsZero(t *testing.T) {
	db := openTestDB(t)

	id, err := CreatePeriod(db, PeriodInput{
		ReadingDate:             "2026-05-01",
		HeizungWaermeGewichtung: 0.7,
		Readings:                baseReadings(nil),
	})
	if err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}

	got, err := GetPeriodByID(db, id)
	if err != nil {
		t.Fatalf("GetPeriodByID: %v", err)
	}
	if got.Strompreis != 0 || got.FrischwasserPreis != 0 || got.AbwasserPreis != 0 || got.EinspeisungPreis != 0 {
		t.Errorf("prices = %v/%v/%v/%v, want alle 0", got.Strompreis, got.FrischwasserPreis, got.AbwasserPreis, got.EinspeisungPreis)
	}
}
