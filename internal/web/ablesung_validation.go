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
	avg = make(map[string]float64, len(warnedMeterKeys))
	// A meter has a baseline only if all 4 readings exist, a gap never
	// counts as 0 (#224).
	for _, key := range warnedMeterKeys {
		sum, complete := 0.0, true
		for i := 0; i < 3; i++ {
			d, ok := store.Verbrauch(recent[i].Readings, recent[i+1].Readings, key)
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
