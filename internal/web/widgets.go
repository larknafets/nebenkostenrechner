package web

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// widgetEntity resolves a widget route's {entity} path segment to one of the
// 4 entities (Wohnung 1/2 - apartment 1/2, Wallboxen/PV-Anlage - wallboxes/PV
// system) the Dashboard's yearly-totals cards/apartment tabs already show.
// ok is false for an unknown slug.
func widgetEntity(slug string) (e entitaet, ok bool) {
	switch slug {
	case "wohnung-1":
		return wohnungEntitaet(1), true
	case "wohnung-2":
		return wohnungEntitaet(2), true
	case "wallboxen":
		return entitaet{Art: entitaetWallbox}, true
	case "pv-anlage":
		return entitaet{Art: entitaetPV}, true
	}
	return entitaet{}, false
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

// widgetView bundles what any widget route might show for one entity -
// always both Card/Verlauf (or SimpleCard/SimpleVerlauf), mirroring
// buildEntityView's own "no card-only mode, avoiding speculative
// generality" convention (Ticket #101) rather than a per-handler subset.
// Exactly one of the Simple*/non-Simple pairs is set, matching whichever
// widgetEntity matched.
type widgetView struct {
	Card          *dashboardJahresCard
	SimpleCard    *dashboardSimpleCard
	Verlauf       *dashboardVerlaufSpalte
	SimpleVerlauf *dashboardSimpleSpalte
}

// resolveWidgetView builds the Sichtbarkeit view of the one requested entity
// for the widget betrachter - shared by all 3 widget handlers instead of each
// hand-writing the same simple/apartment branch. An apartment id that is not
// in the data yields an empty view.
func resolveWidgetView(dd dashboardData, e entitaet) widgetView {
	v := sichtbareEntitaet(betrachterWidget, dd, e, nil)
	switch e.Art {
	case entitaetWallbox:
		if v.Wallbox != nil {
			return widgetView{SimpleCard: &v.Wallbox.Card, SimpleVerlauf: &v.Wallbox.Verlauf}
		}
	case entitaetPV:
		if v.PV != nil {
			return widgetView{SimpleCard: &v.PV.Card, SimpleVerlauf: &v.PV.Verlauf}
		}
	default:
		if w, ok := v.Wohnung(e.ApartmentID); ok {
			return widgetView{Card: &w.Card, Verlauf: &w.Verlauf}
		}
	}
	return widgetView{}
}

// handleWidgetJahressumme serves the ingress-free HA widget route (Issue
// #77 ff.): exactly one entity's yearly-totals card, without nav/footer/
// theme toggle - intended for a Lovelace "Webpage card" iframe. {entity}
// is one of the 4 fixed apartment-tab entities (see widgetEntity).
func handleWidgetJahressumme(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		entity, ok := widgetEntity(r.PathValue("entity"))
		if !ok {
			http.NotFound(w, r)
			return
		}

		dd, _, err := loadDashboardData(db)
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
			view := resolveWidgetView(dd, entity)
			data.Card, data.SimpleCard = view.Card, view.SimpleCard
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
		entity, ok := widgetEntity(r.PathValue("entity"))
		if !ok {
			http.NotFound(w, r)
			return
		}

		dd, _, err := loadDashboardData(db)
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
			view := resolveWidgetView(dd, entity)
			data.Verlauf, data.SimpleVerlauf = view.Verlauf, view.SimpleVerlauf
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
		entity, ok := widgetEntity(r.PathValue("entity"))
		if !ok {
			http.NotFound(w, r)
			return
		}

		dd, _, err := loadDashboardData(db)
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
			view := resolveWidgetView(dd, entity)
			data.Card, data.SimpleCard = view.Card, view.SimpleCard
			data.Verlauf, data.SimpleVerlauf = view.Verlauf, view.SimpleVerlauf
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
