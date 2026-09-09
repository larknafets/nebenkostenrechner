package store

import (
	"database/sql"
	"fmt"
	"time"
)

// SeedDemoData fills an otherwise-empty DB with 39 months of realistic
// synthetic meter readings/fixed-cost entries, ending at now's month (Issue
// #117, part of the demo mode map #115). Dates are always relative to now
// rather than a fixed calendar range, so a demo reset always looks current
// instead of slowly going stale as real time passes.
//
// Called on every demo-session reset (Issue #116's "demo DB fully reset on
// every login") against the separate demo DB only - never against the real
// one. Not idempotent: callers must start from a fresh, empty-of-user-data
// DB (e.g. by deleting and recreating the demo DB file, which re-runs
// Open's schema/seed step) rather than calling this twice on the same DB.
func SeedDemoData(db *sql.DB, now time.Time) error {
	if err := seedDemoStammdaten(db); err != nil {
		return fmt.Errorf("stammdaten: %w", err)
	}
	if err := seedDemoPeriods(db, now); err != nil {
		return fmt.Errorf("periods: %w", err)
	}
	if err := seedDemoFixkosten(db, now); err != nil {
		return fmt.Errorf("fixkosten: %w", err)
	}
	return nil
}

// seedDemoStammdaten sets plausible, non-zero apartment size/parcel size
// - a fresh real install seeds these at 0 (nobody's filled in /stammdaten
// yet), which would degrade the heating-split/parcel-fixed-cost logic to
// their 50/50 fallback and make the demo look broken.
func seedDemoStammdaten(db *sql.DB) error {
	return UpdateStammdaten(db, map[int64]StammdatenInput{
		1: {QM: 116.23, FlurstueckGroesse: 460},
		2: {QM: 86, FlurstueckGroesse: 340},
	})
}

// demoMonth is month i of 39 (0 = oldest, 38 = current), first-of-month,
// relative to now.
func demoMonth(now time.Time, i int) time.Time {
	anchor := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	return anchor.AddDate(0, -(38 - i), 0)
}

// heizfaktor is how much of peak space-heating load a calendar month
// carries: 0 in summer (Jun-Aug, hot water only - see README
// "Heizung/Warmwasser"), 1 at the winter peak (Dec/Jan/Feb), ramping
// slowly up/down through autumn/spring in between.
func heizfaktor(m time.Month) float64 {
	switch m {
	case time.December, time.January, time.February:
		return 1.0
	case time.November, time.March:
		return 0.6
	case time.October, time.April:
		return 0.35
	case time.September, time.May:
		return 0.15
	default: // June, July, August
		return 0
	}
}

// pvFaktor is the inverse shape for PV feed-in - peak in midsummer, minimal
// in winter (short, low-sun days).
func pvFaktor(m time.Month) float64 {
	switch m {
	case time.June, time.July:
		return 1.0
	case time.May, time.August:
		return 0.85
	case time.April, time.September:
		return 0.55
	case time.March, time.October:
		return 0.25
	case time.February, time.November:
		return 0.08
	default: // December, January
		return 0.03
	}
}

// personenWohnung2 varies 1/2 across the 39 months (autumn/winter months
// 10-14 and 27-29 have only 1 person - e.g. a temporary single occupancy) -
// Wohnung 1 always stays at 2, as fixed by the user.
func personenWohnung2(i int) int64 {
	if (i >= 10 && i <= 14) || (i >= 27 && i <= 29) {
		return 1
	}
	return 2
}

// seedDemoPeriods creates 39 monthly meter readings, oldest first
// (CreatePeriod expects chronological order - see store.UpdatePeriod's
// previous-period checks elsewhere), with cumulative meter readings that
// only ever go up and seasonal deltas per heizfaktor/pvFaktor.
func seedDemoPeriods(db *sql.DB, now time.Time) error {
	readings := map[string]float64{
		"strom_gesamt":                  38400,
		"strom_wohnung2":                9200,
		"strom_waermepumpe":             14600,
		"strom_wallbox":                 5100,
		"wasser_gesamt":                 612,
		"wasser_wohnung2":               168,
		"wasser_warmwasseraufbereitung": 94,
		"waerme_wohnung1":               52.4,
		"waerme_wohnung2":               38.1,
		"strom_einspeisung":             11200,
	}

	const monthCount = 39
	for i := 0; i < monthCount; i++ {
		date := demoMonth(now, i)
		hf := heizfaktor(date.Month())
		pf := pvFaktor(date.Month())

		wohnung2Delta := 165.0 + 25*hf
		wallboxDelta := 115.0 + 20*hf
		wpDelta := 250 + 900*hf // 250 kWh/month hot-water baseline + heating share
		wohnung1EigenerAnteil := 370.0 + 60*hf

		readings["strom_wohnung2"] += wohnung2Delta
		readings["strom_waermepumpe"] += wpDelta
		readings["strom_wallbox"] += wallboxDelta
		readings["strom_gesamt"] += wohnung2Delta + wpDelta + wallboxDelta + wohnung1EigenerAnteil
		readings["strom_einspeisung"] += 80 + 670*pf

		readings["wasser_gesamt"] += 14 + 3*pf // a bit more in summer (garden)
		readings["wasser_wohnung2"] += 4.2
		readings["wasser_warmwasseraufbereitung"] += 2.5 // constant, independent of heating season

		readings["waerme_wohnung1"] += 1.25 * hf
		readings["waerme_wohnung2"] += 0.9 * hf

		// Slight price increase over the 39 months (electricity/water/
		// wastewater - realistic), whereas the feed-in tariff stays constant
		// for the PV system's runtime, fixed by law (EEG).
		progress := float64(i) / float64(monthCount-1)
		strompreis := 0.19 + 0.03*progress
		frischwasserPreis := 1.35 + 0.11*progress
		abwasserPreis := 4.60 + 0.27*progress

		readingsCopy := make(map[string]float64, len(readings))
		for k, v := range readings {
			readingsCopy[k] = v
		}

		monat := date.Format("2006-01") + "-01"
		in := PeriodInput{
			ReadingDate:             date.Format("2006-01-02"),
			Monat:                   monat,
			Strompreis:              Float64(strompreis),
			FrischwasserPreis:       Float64(frischwasserPreis),
			AbwasserPreis:           Float64(abwasserPreis),
			HeizungWaermeGewichtung: 0.7,
			EinspeisungPreis:        Float64(0.08),
			Readings:                readingsCopy,
			Personen:                map[int64]int64{1: 2, 2: personenWohnung2(i)},
		}
		if _, err := CreatePeriod(db, in); err != nil {
			return fmt.Errorf("monat %s: %w", monat, err)
		}
	}
	return nil
}

// ResetDemoData wipes every meter reading/fixed-cost entry/occupancy/
// installment row and calls SeedDemoData again (Issue #121: every new demo
// login fully resets the demo database back to its fresh 39-month starting
// state, with the current month as the newest). Master data (apartments,
// meters, kostenpositionen) stays untouched - SeedDemoData overwrites
// apartments' qm/flurstueck_groesse itself anyway.
func ResetDemoData(db *sql.DB, now time.Time) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Child tables first - PRAGMA foreign_keys=ON (store.Open) otherwise
	// forbids deleting a still-referenced periods/fixkosten_eingaben row.
	for _, table := range []string{
		"meter_readings", "period_occupancy",
		"fixkosten_werte", "fixkosten_personen", "nebenkosten_abschlaege",
		"periods", "fixkosten_eingaben",
	} {
		if _, err := tx.Exec("DELETE FROM " + table); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}

	return SeedDemoData(db, now)
}

// demoKostenpositionWerte are plausible EUR values per cost item (Issue
// #117) - constant across all 39 months, the way property tax, insurance
// & base fees mostly stay unchanged for years in real life. Order/logic/
// type come from KostenpositionDefaults, this only supplies the missing
// value per cost-item key.
var demoKostenpositionWerte = map[string]float64{
	"grundsteuer":      620,   // annual
	"gebaeudevers":     455,   // annual
	"deich_grund":      58,    // annual
	"deich_bau":        92,    // annual
	"kreisverband":     41,    // annual
	"abfall_haushalt":  118,   // annual
	"abfall_personen":  176,   // annual
	"abfall_biomuell":  52,    // annual
	"abfall_restmuell": 184,   // annual
	"strom_grundpreis": 12.40, // monthly
	"trinkwasser":      8.20,  // monthly
	"abwasser":         9.80,  // monthly
	"internet":         39.90, // monthly
	"wp_wartung":       14.50, // monthly
}

// demoAbschlag is the monthly Nebenkostenabschlag (utility installment)
// per apartment - constant, the way it's only adjusted after an annual
// statement in real life.
var demoAbschlag = map[int64]float64{1: 260, 2: 190}

// seedDemoFixkosten creates 39 monthly fixed-cost entries covering the same
// range as seedDemoPeriods, so Dashboard/history show a complete record
// instead of meter readings without a matching fixed-cost entry.
func seedDemoFixkosten(db *sql.DB, now time.Time) error {
	werte := make(map[int64]FixkostenPositionWert, len(KostenpositionDefaults))
	for _, kd := range KostenpositionDefaults {
		werte[kd.ID] = FixkostenPositionWert{
			Logik: kd.Logik,
			Typ:   kd.Typ,
			Wert:  demoKostenpositionWerte[kd.Key],
		}
	}

	const monthCount = 39
	for i := 0; i < monthCount; i++ {
		date := demoMonth(now, i)
		in := FixkostenInput{
			Monat:    date.Format("2006-01") + "-01",
			Personen: map[int64]int64{1: 2, 2: 2},
			Werte:    werte,
			Abschlag: demoAbschlag,
		}
		if _, err := CreateFixkostenEingabe(db, in); err != nil {
			return fmt.Errorf("fixkosten monat %s: %w", in.Monat, err)
		}
	}
	return nil
}
