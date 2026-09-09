package calc

import (
	"database/sql"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// EinspeisungErgebnis is the PV feed-in compensation result for one period -
// whole-house, no apartment split (the feed-in meter isn't apartment-
// specific), unlike StromErgebnis/WasserErgebnis/HeizungErgebnis (Ticket #47).
type EinspeisungErgebnis struct {
	EinspeisungKWh float64

	// Ertrag is rounded to the cent using commercial rounding (Issue #8 convention).
	Ertrag float64
}

// Einspeisung computes the PV feed-in compensation for the given period:
// fed-in kWh (consumption of the feed-in meter) times the feed-in price.
func Einspeisung(db *sql.DB, periodID int64) (*EinspeisungErgebnis, error) {
	period, err := store.GetPeriodByID(db, periodID)
	if err != nil {
		return nil, err
	}

	verbrauch, err := store.Verbrauch(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("verbrauch: %w", err)
	}

	kwh := verbrauch["strom_einspeisung"]
	return &EinspeisungErgebnis{
		EinspeisungKWh: kwh,
		Ertrag:         Round2(kwh * period.EinspeisungPreis),
	}, nil
}
