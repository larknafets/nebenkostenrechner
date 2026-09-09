package web

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// widgetEntity resolves a widget route's {entity} path segment to either an
// apartment id (Wohnung 1/2 - apartment 1/2) or one of the whole-house
// simpleSeries (Wallboxen/PV-Anlage - wallboxes/PV system) - exactly the 4
// entities the Dashboard's yearly-totals cards/apartment tabs already show.
// "" for an unknown slug.
func widgetEntity(slug string) (apartmentID int64, simple *simpleSeries) {
	switch slug {
	case "wohnung-1":
		return 1, nil
	case "wohnung-2":
		return 2, nil
	case "wallboxen":
		return 0, &wallboxSeries
	case "pv-anlage":
		return 0, &pvSeries
	}
	return 0, nil
}

// findApartment returns the apartment with the given id, or the zero value
// if absent (only reachable if the DB's fixed 2-apartment seed data was
// somehow removed).
func findApartment(apartments []store.Apartment, id int64) store.Apartment {
	for _, a := range apartments {
		if a.ID == id {
			return a
		}
	}
	return store.Apartment{}
}

// handleWidgetJahressumme serves the ingress-free HA widget route (Issue
// #77 ff.): exactly one entity's yearly-totals card, without nav/footer/
// theme toggle - intended for a Lovelace "Webpage card" iframe. {entity}
// is one of the 4 fixed apartment-tab entities (see widgetEntity).
func handleWidgetJahressumme(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apartmentID, simple := widgetEntity(r.PathValue("entity"))
		if apartmentID == 0 && simple == nil {
			http.NotFound(w, r)
			return
		}

		dd, err := loadDashboardData(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			HasAnyData         bool
			AnzeigeJahr        int
			AnzeigeJahrLaufend bool
			Card               *dashboardJahresCard
			SimpleCard         *dashboardSimpleCard
		}{HasAnyData: dd.HasAnyData, AnzeigeJahr: dd.Jahr, AnzeigeJahrLaufend: dd.Jahr == time.Now().Year()}

		if dd.HasAnyData {
			if simple != nil {
				c := buildSimpleJahresCard(*simple, dd.Jahr, dd.PeriodenKosten)
				data.SimpleCard = &c
			} else {
				c, _ := buildEntityView(dd, apartmentID)
				data.Card = &c
			}
		}

		if err := widgetJahressummeTemplate.ExecuteTemplate(w, "widget-layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleWidgetVerbrauchswerte serves the 2nd ingress-free HA widget route:
// exactly one entity's Monatsverlauf (monthly history) panel (the
// consumption/consumption-values/fixed-costs/combined toggle stays usable,
// only the entity is fixed).
func handleWidgetVerbrauchswerte(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apartmentID, simple := widgetEntity(r.PathValue("entity"))
		if apartmentID == 0 && simple == nil {
			http.NotFound(w, r)
			return
		}

		dd, err := loadDashboardData(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			HasAnyData    bool
			Verlauf       *dashboardVerlaufSpalte
			SimpleVerlauf *dashboardSimpleSpalte
		}{HasAnyData: dd.HasAnyData}

		if dd.HasAnyData {
			if simple != nil {
				v := buildSimpleVerlauf(*simple, dd.PeriodenKosten)
				data.SimpleVerlauf = &v
			} else {
				a := findApartment(dd.Apartments, apartmentID)
				v := buildDashboardVerlauf(a.ID, a.Name, dd.PeriodenKosten, dd.FixkostenListe)
				data.Verlauf = &v
			}
		}

		if err := widgetVerbrauchswerteTemplate.ExecuteTemplate(w, "widget-layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleWidgetUebersicht serves the 3rd HA widget route (Issue #78): the
// yearly-totals card and the consumption-values panel of the same entity
// stacked on one page - reuses both body templates
// ("widget-jahressumme-body"/"widget-verbrauchswerte-body", widget_layout.
// html) instead of duplicating them a 3rd time, and loads the data only
// once.
func handleWidgetUebersicht(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apartmentID, simple := widgetEntity(r.PathValue("entity"))
		if apartmentID == 0 && simple == nil {
			http.NotFound(w, r)
			return
		}

		dd, err := loadDashboardData(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			HasAnyData         bool
			AnzeigeJahr        int
			AnzeigeJahrLaufend bool
			Card               *dashboardJahresCard
			SimpleCard         *dashboardSimpleCard
			Verlauf            *dashboardVerlaufSpalte
			SimpleVerlauf      *dashboardSimpleSpalte
		}{HasAnyData: dd.HasAnyData, AnzeigeJahr: dd.Jahr, AnzeigeJahrLaufend: dd.Jahr == time.Now().Year()}

		if dd.HasAnyData {
			if simple != nil {
				c := buildSimpleJahresCard(*simple, dd.Jahr, dd.PeriodenKosten)
				data.SimpleCard = &c
				v := buildSimpleVerlauf(*simple, dd.PeriodenKosten)
				data.SimpleVerlauf = &v
			} else {
				c, v := buildEntityView(dd, apartmentID)
				data.Card = &c
				data.Verlauf = &v
			}
		}

		if err := widgetUebersichtTemplate.ExecuteTemplate(w, "widget-layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// NewWidgetMux serves ONLY the 3 read-only HA widget routes, deliberately
// separate from NewMux's full app (edit/delete readings/fixed costs etc.)
// - intended to run on a 2nd port outside of ingress (see
// cmd/nebenkostenrechner), so without any auth at all: smallest possible
// attack surface, no access to mutating routes.
func NewWidgetMux(db *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widget/jahressumme/{entity}", handleWidgetJahressumme(db))
	mux.HandleFunc("GET /widget/verbrauchswerte/{entity}", handleWidgetVerbrauchswerte(db))
	mux.HandleFunc("GET /widget/uebersicht/{entity}", handleWidgetUebersicht(db))
	return mux
}
