package store

import (
	"database/sql"
	"fmt"
)

// Datenstand is everything the cost calculations, the Prüfung and the
// Dashboard read, loaded once: the Ablesungen and Fixkosten-Eingaben with
// their details, and the Stammdaten they depend on. Plain data without a
// database handle, so a test can build one by hand. calc.KostenAus and
// calc.FixkostenreiheAus compute from it, the Abrechnung takes it as is.
type Datenstand struct {
	// Periods are the Ablesungen, oldest first by reading date, then id.
	Periods []*LatestPeriod
	// Eingaben are the Fixkosten-Eingaben, oldest first.
	Eingaben         []*FixkostenEingabeDetails
	Apartments       []Apartment
	Haus             Haus
	Kostenpositionen []Kostenposition
	Meters           []Meter
}

// LadeDatenstand reads the whole Datenstand, every table at most once.
func LadeDatenstand(db *sql.DB) (*Datenstand, error) {
	periods, err := AllPeriodDetails(db)
	if err != nil {
		return nil, fmt.Errorf("periods: %w", err)
	}
	eingaben, err := AllFixkostenEingabenDetails(db)
	if err != nil {
		return nil, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	apartments, err := Apartments(db)
	if err != nil {
		return nil, fmt.Errorf("apartments: %w", err)
	}
	haus, err := GetHaus(db)
	if err != nil {
		return nil, fmt.Errorf("haus: %w", err)
	}
	kostenpositionen, err := Kostenpositionen(db)
	if err != nil {
		return nil, fmt.Errorf("kostenpositionen: %w", err)
	}
	meters, err := Meters(db)
	if err != nil {
		return nil, fmt.Errorf("meters: %w", err)
	}
	return &Datenstand{Periods: periods, Eingaben: eingaben, Apartments: apartments, Haus: haus, Kostenpositionen: kostenpositionen, Meters: meters}, nil
}
