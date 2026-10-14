package web

import (
	"fmt"
	"math"
	"strconv"
)

// validatePersonenzahl is the single check for an occupant count: finite,
// >= 0 (0 stays allowed) and at most 2 decimal places. The forms and both
// CSV imports use it.
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

// parseImportPersonenzahl parses and validates one CSV cell (comma decimal)
// for the Personenzahl of Wohnung id. The error carries the line number and
// the reason from validatePersonenzahl.
func parseImportPersonenzahl(raw string, line int, id int64) (float64, error) {
	p, err := parseDecimalDE(raw)
	if err != nil {
		return 0, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q", line, id, raw)
	}
	if err := validatePersonenzahl(p); err != nil {
		return 0, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q (%w)", line, id, raw, err)
	}
	return p, nil
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
