package web

import (
	"database/sql"

	abr "github.com/larknafets/nebenkostenrechner/internal/abrechnung"
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// ladeAbrechnungDaten loads all data of the Jahresabrechnung once per request
// (Issue #165). Jahresvorauswahl, Prüfung and Berechnung work on the result
// (Pruef is the subset the Prüfung and the Jahresvorauswahl need) instead of
// loading again.
func ladeAbrechnungDaten(db *sql.DB) (abr.BerechnungsDaten, error) {
	ds, err := store.LadeDatenstand(db)
	if err != nil {
		return abr.BerechnungsDaten{}, err
	}
	return abrechnungsDaten(ds), nil
}

// abrechnungsDaten is the Abrechnung's view of a Datenstand.
func abrechnungsDaten(ds *store.Datenstand) abr.BerechnungsDaten {
	summaries := make([]store.FixkostenEingabeSummary, len(ds.Eingaben))
	for i, e := range ds.Eingaben {
		summaries[i] = store.FixkostenEingabeSummary{ID: e.ID, Monat: e.Monat}
	}
	return abr.BerechnungsDaten{
		Pruef:            abr.Daten{Periods: ds.Periods, Eingaben: summaries, Apartments: ds.Apartments, Haus: ds.Haus},
		Kostenpositionen: ds.Kostenpositionen,
		Meters:           ds.Meters,
		Fixkosten:        calc.FixkostenreiheAus(ds),
		Kosten:           calc.KostenAus(ds),
	}
}
