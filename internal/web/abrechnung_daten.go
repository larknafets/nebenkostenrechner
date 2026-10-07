package web

import (
	"database/sql"
	"fmt"

	abr "github.com/larknafets/nebenkostenrechner/internal/abrechnung"
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// ladeAbrechnungDaten loads all data of the Jahresabrechnung once per request
// (Issue #165). Jahresvorauswahl, Prüfung and Berechnung work on the result
// (Pruef is the subset the Prüfung and the Jahresvorauswahl need) instead of
// loading again.
func ladeAbrechnungDaten(db *sql.DB) (abr.BerechnungsDaten, error) {
	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abr.BerechnungsDaten{}, fmt.Errorf("periods: %w", err)
	}
	eingaben, err := store.AllFixkostenEingabenDetails(db)
	if err != nil {
		return abr.BerechnungsDaten{}, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abr.BerechnungsDaten{}, fmt.Errorf("apartments: %w", err)
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abr.BerechnungsDaten{}, fmt.Errorf("haus: %w", err)
	}
	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return abr.BerechnungsDaten{}, fmt.Errorf("kostenpositionen: %w", err)
	}
	meters, err := store.Meters(db)
	if err != nil {
		return abr.BerechnungsDaten{}, err
	}

	summaries := make([]store.FixkostenEingabeSummary, len(eingaben))
	for i, e := range eingaben {
		summaries[i] = store.FixkostenEingabeSummary{ID: e.ID, Monat: e.Monat}
	}
	return abr.BerechnungsDaten{
		Pruef:            abr.Daten{Periods: periods, Eingaben: summaries, Apartments: apartments, Haus: haus},
		Kostenpositionen: kostenpositionen,
		Meters:           meters,
		Fixkosten:        calc.NewFixkostenreihe(calc.FixkostenDaten{Eingaben: eingaben, Kostenpositionen: kostenpositionen, Apartments: apartments}),
		Kosten:           calc.New(calc.Daten{Periods: periods, Apartments: apartments, Haus: haus}),
	}, nil
}
