package web

import (
	"fmt"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// parseImportPersonenzahl parses and validates one CSV cell (comma decimal)
// for the Personenzahl of Wohnung id. The error carries the line number and
// the reason from store.ValidierePersonenzahl.
func parseImportPersonenzahl(raw string, line int, id int64) (float64, error) {
	p, err := parseDecimalDE(raw)
	if err != nil {
		return 0, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q", line, id, raw)
	}
	if err := store.ValidierePersonenzahl(p); err != nil {
		return 0, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q (%w)", line, id, raw, err)
	}
	return p, nil
}

// parsePersonenzahl parses a form value (the browser sends a decimal point)
// and validates it with store.ValidierePersonenzahl.
func parsePersonenzahl(raw string) (float64, error) {
	p, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("Personenzahl muss eine Zahl sein")
	}
	if err := store.ValidierePersonenzahl(p); err != nil {
		return 0, err
	}
	return p, nil
}
