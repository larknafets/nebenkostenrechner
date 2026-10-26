package web

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboardData is every piece of already-computed data the Dashboard and
// the HA widget routes (Issue #77 ff.) build their views from - factored
// out of handleDashboard so both share one implementation of "walk every
// period/fixed-costs entry and compute this year's costs" instead of
// diverging copies.
type dashboardData struct {
	HasAnyData     bool
	Apartments     []store.Apartment
	PeriodenKosten []periodKosten
	FixkostenListe []fixkostenKosten
	Jahr           int
	// WallboxWeiterberechnen is the house flag of the same name; with the
	// Wohnungsstatus it decides whether a zero Wallbox line is shown.
	WallboxWeiterberechnen bool
}

// mieteMonat is the Miete projection of one Fixkosten-Eingabe: its
// Abrechnungsmonat and the monthly Miete per apartment. It is returned next to
// dashboardData, never inside it, so the Miete cannot reach a widget (#278).
type mieteMonat struct {
	Monat string
	Miete map[int64]float64
}

// loadDashboardData runs the shared, DB-heavy first half of both
// handleDashboard and the widget handlers: apartments, every period's
// costs (stopping at the oldest period without a previous period, same as
// before), every fixed-costs entry's result, and the auto-following
// display year. HasAnyData=false (zero-value everything else) when
// neither readings nor fixed-costs entries exist yet. The second return value
// is the Miete projection taken from the same Datenstand (no second query);
// the widget handlers discard it.
func loadDashboardData(db *sql.DB) (dashboardData, []mieteMonat, error) {
	ds, err := store.LadeDatenstand(db)
	if err != nil {
		return dashboardData{}, nil, err
	}
	miete := make([]mieteMonat, len(ds.Eingaben))
	for i, e := range ds.Eingaben {
		miete[i] = mieteMonat{Monat: e.Monat, Miete: e.Miete}
	}
	apartments := ds.Apartments

	// Newest first, like store.AllPeriods and store.AllFixkostenEingaben.
	allPeriods := make([]store.PeriodSummary, len(ds.Periods))
	for i, p := range ds.Periods {
		allPeriods[len(ds.Periods)-1-i] = store.PeriodSummary{ID: p.ID, ReadingDate: p.ReadingDate, Monat: p.Monat}
	}
	// Teilstand/partial reading (Ticket #129): only the newest period can
	// be incomplete (enforced on creation, see store.CreatePeriod) -
	// stays completely excluded here (no yearly-totals/monthly-history
	// entry, no effect on HasAnyData or the display year) until it's
	// completed.
	if len(ds.Periods) > 0 && ds.Periods[len(ds.Periods)-1].Teilstand(apartments).IstTeilstand {
		allPeriods = allPeriods[1:]
	}
	fixkostenEingaben := make([]store.FixkostenEingabeSummary, len(ds.Eingaben))
	for i, e := range ds.Eingaben {
		fixkostenEingaben[len(ds.Eingaben)-1-i] = store.FixkostenEingabeSummary{ID: e.ID, Monat: e.Monat}
	}

	if len(allPeriods) == 0 && len(fixkostenEingaben) == 0 {
		return dashboardData{Apartments: apartments}, miete, nil
	}

	// Verbrauchskosten newest -> oldest. The very first period ever
	// recorded has no Vorperiode and so no Verbrauchskosten, which is the
	// natural end of the available history.
	verbrauchskosten := calc.KostenAus(ds)
	var periodenKosten []periodKosten
	berechenbar := verbrauchskosten.Berechenbare()
	for i := len(berechenbar) - 1; i >= 0; i-- {
		a := berechenbar[i]
		periodenKosten = append(periodenKosten, periodKosten{ReadingDate: a.Period.ReadingDate, Monat: a.Period.Monat, K: a.Kosten})
	}

	fixkostenListe := alleFixkostenKosten(calc.FixkostenreiheAus(ds))

	return dashboardData{
		HasAnyData:             true,
		Apartments:             apartments,
		PeriodenKosten:         periodenKosten,
		FixkostenListe:         fixkostenListe,
		Jahr:                   anzeigeJahr(allPeriods, fixkostenEingaben),
		WallboxWeiterberechnen: ds.Haus.WallboxWeiterberechnen,
	}, miete, nil
}

// handleDashboard serves the redesigned Dashboard (Issue #60): yearly-
// totals cards per apartment for the auto-following display year, then an
// apartment switcher with a combined consumption+fixed-costs monthly
// history (4 modes).
func handleDashboard(version, buildDate string, a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dd, miete, err := loadDashboardData(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nav := a.NavData(r)
		latestVersion, updateAvailable := checkForUpdate(version)

		if !dd.HasAnyData {
			data := struct {
				navData
				Aktuell         string
				HasAnyData      bool
				Version         string
				BuildDate       string
				UpdateAvailable bool
				LatestVersion   string
			}{
				navData:         nav,
				Aktuell:         "dashboard",
				Version:         version,
				BuildDate:       buildDate,
				UpdateAvailable: updateAvailable,
				LatestVersion:   latestVersion,
			}
			if err := dashboardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		jahr := dd.Jahr

		b := betrachterAnonym
		if nav.IsLoggedIn {
			b = betrachterAngemeldet
		}

		// What the viewer may see is decided in one place (#293).
		view := sichtbareAnsicht(b, dd, miete)
		var wallboxCard, pvCard dashboardSimpleCard
		var wallboxVerlauf, pvVerlauf dashboardSimpleSpalte
		if view.Wallbox != nil {
			wallboxCard, wallboxVerlauf = view.Wallbox.Card, view.Wallbox.Verlauf
		}
		if view.PV != nil {
			pvCard, pvVerlauf = view.PV.Card, view.PV.Verlauf
		}

		data := struct {
			navData
			Aktuell            string
			HasAnyData         bool
			AnzeigeJahr        int
			AnzeigeJahrLaufend bool
			Cards              []dashboardJahresCard
			VerlaufSpalten     []dashboardVerlaufSpalte
			WallboxCard        dashboardSimpleCard
			WallboxVerlauf     dashboardSimpleSpalte
			PVCard             dashboardSimpleCard
			PVVerlauf          dashboardSimpleSpalte
			Version            string
			BuildDate          string
			UpdateAvailable    bool
			LatestVersion      string
		}{
			navData:            nav,
			Aktuell:            "dashboard",
			HasAnyData:         true,
			AnzeigeJahr:        jahr,
			AnzeigeJahrLaufend: jahr == time.Now().Year(),
			Cards:              view.Cards,
			VerlaufSpalten:     view.VerlaufSpalten,
			WallboxCard:        wallboxCard,
			WallboxVerlauf:     wallboxVerlauf,
			PVCard:             pvCard,
			PVVerlauf:          pvVerlauf,
			Version:            version,
			BuildDate:          buildDate,
			UpdateAvailable:    updateAvailable,
			LatestVersion:      latestVersion,
		}

		if err := dashboardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
