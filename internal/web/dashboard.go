package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"time"

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
}

// loadDashboardData runs the shared, DB-heavy first half of both
// handleDashboard and the widget handlers: apartments, every period's
// costs (stopping at the oldest period without a previous period, same as
// before), every fixed-costs entry's result, and the auto-following
// display year. HasAnyData=false (zero-value everything else) when
// neither readings nor fixed-costs entries exist yet.
func loadDashboardData(db *sql.DB) (dashboardData, error) {
	apartments, err := store.Apartments(db)
	if err != nil {
		return dashboardData{}, fmt.Errorf("apartments: %w", err)
	}

	allPeriods, err := store.AllPeriods(db)
	if err != nil {
		return dashboardData{}, fmt.Errorf("all periods: %w", err)
	}
	// Teilstand/partial reading (Ticket #129): only the newest period can
	// be incomplete (enforced on creation, see handleCreateAblesung) -
	// stays completely excluded here (no yearly-totals/monthly-history
	// entry) until it's completed. Without this, berechneKosten's own
	// partial-reading KostenNote guard further below would otherwise hide
	// the entire history: the loop runs newest->oldest and stops at the
	// first KostenNote.
	if len(allPeriods) > 0 {
		complete, err := store.PeriodComplete(db, allPeriods[0].ID)
		if err != nil {
			return dashboardData{}, fmt.Errorf("period complete: %w", err)
		}
		if !complete {
			allPeriods = allPeriods[1:]
		}
	}
	fixkostenEingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return dashboardData{}, fmt.Errorf("fixkosten eingaben: %w", err)
	}

	if len(allPeriods) == 0 && len(fixkostenEingaben) == 0 {
		return dashboardData{Apartments: apartments}, nil
	}

	// Verbrauch walks newest -> oldest and stops at the first period
	// without a Vorperiode - that's always the very first period ever
	// recorded (every later one has an earlier neighbour to diff
	// against), so it's the natural end of the available history.
	var periodenKosten []periodKosten
	for _, p := range allPeriods {
		pk, err := berechneKosten(db, p.ID)
		if err != nil {
			return dashboardData{}, err
		}
		if pk.KostenNote != "" {
			break
		}
		personen, err := store.PersonenByApartment(db, p.ID)
		if err != nil {
			return dashboardData{}, fmt.Errorf("personen: %w", err)
		}
		periodenKosten = append(periodenKosten, periodKosten{ReadingDate: p.ReadingDate, Monat: p.Monat, K: pk, Personen: personen})
	}

	fixkostenListe, err := alleFixkostenKosten(db)
	if err != nil {
		return dashboardData{}, err
	}

	return dashboardData{
		HasAnyData:     true,
		Apartments:     apartments,
		PeriodenKosten: periodenKosten,
		FixkostenListe: fixkostenListe,
		Jahr:           anzeigeJahr(allPeriods, fixkostenEingaben),
	}, nil
}

// handleDashboard serves the redesigned Dashboard (Issue #60): yearly-
// totals cards per apartment for the auto-following display year, then an
// apartment switcher with a combined consumption+fixed-costs monthly
// history (4 modes).
func handleDashboard(version, buildDate string, a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dd, err := loadDashboardData(dbFromContext(r.Context()))
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
		apartments, periodenKosten, jahr := dd.Apartments, dd.PeriodenKosten, dd.Jahr

		// Not logged in: only apartment 2 goes into the template data
		// object at all (Ticket #112) - purely server-side hiding in the
		// template would still leave apartment 1's data in the HTML
		// source (finding from the login overlay prototype, Issue #113).
		if !nav.IsLoggedIn {
			for _, a := range apartments {
				if a.ID == 2 {
					apartments = []store.Apartment{a}
					break
				}
			}
		}

		var cards []dashboardJahresCard
		var verlaufSpalten []dashboardVerlaufSpalte
		for _, a := range apartments {
			card, verlauf := buildEntityView(dd, a.ID)
			cards = append(cards, card)
			verlaufSpalten = append(verlaufSpalten, verlauf)
		}

		// Wallboxen/PV-Anlage (wallboxes/PV system, Ticket #67) - whole-
		// house, purely informative entities alongside the apartment tabs,
		// analogous to the prototype
		// (docs/prototypes/fixkosten-prototype.html): own yearly total +
		// monthly history, no fixed-costs share, no apartment allocation.
		// Not logged in: stay completely excluded, like apartment 1 above
		// - the same "only apartment 2 visible" rule applies to the whole
		// house.
		var wallboxCard, pvCard dashboardSimpleCard
		var wallboxVerlauf, pvVerlauf dashboardSimpleSpalte
		if nav.IsLoggedIn {
			wallboxCard = buildSimpleJahresCard(wallboxSeries, jahr, periodenKosten)
			wallboxVerlauf = buildSimpleVerlauf(wallboxSeries, periodenKosten)
			pvCard = buildSimpleJahresCard(pvSeries, jahr, periodenKosten)
			pvVerlauf = buildSimpleVerlauf(pvSeries, periodenKosten)
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
			Cards:              cards,
			VerlaufSpalten:     verlaufSpalten,
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
