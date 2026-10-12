package web

import (
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// ablesung_validation.go holds the reading (Ablesung) input rules shared by
// both input adapters - the wizard form and the CSV import (Issue #86/#87
// follow-up architecture review): same rules, same seam, two callers.

// outlierAvg computes the outlier-warning baseline (Ticket #13) from up
// to 4 recent periods (newest first): the average of the 3 consumption
// diffs between them. ok is false if fewer than 4 are available.
func outlierAvg(recent []store.PeriodReadings) (avg map[string]float64, ok bool) {
	if len(recent) < 4 {
		return nil, false
	}
	avg = make(map[string]float64, len(store.MeterKeys)+len(store.OptionalMeterKeys))
	for _, key := range store.MeterKeys {
		sum := 0.0
		for i := 0; i < 3; i++ {
			sum += recent[i].Readings[key] - recent[i+1].Readings[key]
		}
		avg[key] = sum / 3
	}
	// An optional Einzelzähler (#224) has a baseline only if all 4 readings
	// exist, a gap never counts as 0.
	for _, key := range store.OptionalMeterKeys {
		sum, complete := 0.0, true
		for i := 0; i < 3; i++ {
			d, ok := store.VerbrauchOptional(recent[i].Readings, recent[i+1].Readings, key)
			if !ok {
				complete = false
				break
			}
			sum += d
		}
		if complete {
			avg[key] = sum / 3
		}
	}
	return avg, true
}
