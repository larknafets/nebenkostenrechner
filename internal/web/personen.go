package web

import (
	"fmt"

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
