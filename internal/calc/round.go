// Package calc computes the monthly cost positions (electricity, heating/
// hot water, water) from a period's consumption. See Issue #8 for the
// rounding decision this package implements.
package calc

import (
	"math"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Round2 rounds to 2 decimal places (cents), using commercial rounding (0.5
// cent always rounds up) - math.Round already rounds half-away-from-zero,
// which for the non-negative EUR amounts here is exactly that rule.
func Round2(x float64) float64 {
	return math.Round(x*100) / 100
}

// Ratio2 splits a and b's shares of their own total (a/(a+b), b/(a+b)).
// When both are 0 it falls back to an even 50/50 split instead of 0/0 - a
// bare zero-guard would silently drop a real cost position from both
// apartments' bill instead of just avoiding the division by zero
// (Issue #26).
func Ratio2(a, b float64) (float64, float64) {
	if total := a + b; total > 0 {
		return a / total, b / total
	}
	return 0.5, 0.5
}

// apartmentValues extracts apartment 1/2's respective values from the fixed
// 2-apartment list via selector - the shared shape behind every calc
// function that needs a per-apartment value off store.Apartment (heating's
// apartment size, fixed-cost's apartment size/lot size).
func apartmentValues(apartments []store.Apartment, selector func(store.Apartment) float64) (w1, w2 float64) {
	for _, a := range apartments {
		switch a.ID {
		case 1:
			w1 = selector(a)
		case 2:
			w2 = selector(a)
		}
	}
	return w1, w2
}
