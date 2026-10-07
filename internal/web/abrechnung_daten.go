package web

import (
	"database/sql"
	"fmt"

	abr "github.com/larknafets/nebenkostenrechner/internal/abrechnung"
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// abrechnungDaten is everything the Jahresabrechnung reads, loaded once per
// request by ladeAbrechnungDaten. Jahresvorauswahl, Prüfung and Berechnung
// work on it instead of loading again (the Berechnung still hands its db to
// calc for the Fixkosten).
type abrechnungDaten struct {
	// Pruef is the subset the Prüfung and the Jahresvorauswahl need.
	Pruef abr.Daten
	// Eingaben are the Fixkosten-Eingaben with Werte/Personen/Abschlag,
	// oldest first. Pruef.Eingaben is derived from them.
	Eingaben         []*store.FixkostenEingabeDetails
	Kostenpositionen []store.Kostenposition
	Meters           []store.Meter
	// Fixkosten are the Fixkosten results of every Eingabe in Eingaben.
	Fixkosten *calc.Fixkostenreihe
	// Kosten are the Verbrauchskosten of every Ablesung in Pruef.Periods.
	Kosten *calc.Verbrauchskosten
}

// ladeAbrechnungDaten loads all data of the Jahresabrechnung (Issue #165).
func ladeAbrechnungDaten(db *sql.DB) (abrechnungDaten, error) {
	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abrechnungDaten{}, fmt.Errorf("periods: %w", err)
	}
	eingaben, err := store.AllFixkostenEingabenDetails(db)
	if err != nil {
		return abrechnungDaten{}, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abrechnungDaten{}, fmt.Errorf("apartments: %w", err)
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abrechnungDaten{}, fmt.Errorf("haus: %w", err)
	}
	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return abrechnungDaten{}, fmt.Errorf("kostenpositionen: %w", err)
	}
	meters, err := store.Meters(db)
	if err != nil {
		return abrechnungDaten{}, err
	}

	summaries := make([]store.FixkostenEingabeSummary, len(eingaben))
	for i, e := range eingaben {
		summaries[i] = store.FixkostenEingabeSummary{ID: e.ID, Monat: e.Monat}
	}
	return abrechnungDaten{
		Pruef:            abr.Daten{Periods: periods, Eingaben: summaries, Apartments: apartments, Haus: haus},
		Eingaben:         eingaben,
		Kostenpositionen: kostenpositionen,
		Meters:           meters,
		Fixkosten:        calc.NewFixkostenreihe(calc.FixkostenDaten{Eingaben: eingaben, Kostenpositionen: kostenpositionen, Apartments: apartments}),
		Kosten:           calc.New(calc.Daten{Periods: periods, Apartments: apartments, Haus: haus}),
	}, nil
}
