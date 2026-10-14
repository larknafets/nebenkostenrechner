package web

import (
	"fmt"
	"math"
	"strconv"
)

// validatePersonenzahl is the single check for an occupant count: finite,
// >= 0 (0 stays allowed) and at most 2 decimal places. The forms use it now,
// the CSV import can call it later.
func validatePersonenzahl(p float64) error {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return fmt.Errorf("Personenzahl muss eine Zahl sein")
	}
	if p < 0 {
		return fmt.Errorf("Personenzahl darf nicht negativ sein")
	}
	if math.Abs(p*100-math.Round(p*100)) > 1e-9 {
		return fmt.Errorf("Personenzahl darf höchstens 2 Nachkommastellen haben")
	}
	return nil
}

// parsePersonenzahl parses a form value (the browser sends a decimal point)
// and validates it with validatePersonenzahl.
func parsePersonenzahl(raw string) (float64, error) {
	p, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("Personenzahl muss eine Zahl sein")
	}
	if err := validatePersonenzahl(p); err != nil {
		return 0, err
	}
	return p, nil
}
