package web

import (
	"net/http"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// meterDisplay describes how one meter's reading is labelled on the
// Ablesung detail view. Order matches the wizard's step grouping.
type meterDisplay struct {
	Key   string
	Label string
	Unit  string
}

var meterDisplays = []meterDisplay{
	{"strom_gesamt", "Strom Gesamt (Netzbezug)", "kWh"},
	{"strom_wohnung2", "Strom Wohnung 2", "kWh"},
	{"strom_waermepumpe", "Strom Wärmepumpe", "kWh"},
	{"strom_wallbox", "Strom Wallboxen", "kWh"},
	{"wasser_gesamt", "Wasser Gesamt", "m³"},
	{"wasser_wohnung2", "Wasser Wohnung 2", "m³"},
	{"wasser_warmwasseraufbereitung", "Wasser Warmwasseraufbereitung", "m³"},
	{"waerme_wohnung1", "Wärme Wohnung 1", "MWh"},
	{"waerme_wohnung2", "Wärme Wohnung 2", "MWh"},
	{"strom_einspeisung", "Einspeisung (PV)", "kWh"},
}

// formatMeterDiff renders one meter's absolute change since the previous
// reading (Ticket #73), "+"-prefixed when it rose (the normal case - a
// meter reading only falls after a meter swap/correction, where the bare
// minus sign from formatDecimalDE2 already reads correctly), padded to 2
// decimal places (Ticket #76) like every other displayed consumption
// value.
func formatMeterDiff(current, previous float64) string {
	diff := current - previous
	sign := ""
	if diff > 0 {
		sign = "+"
	}
	return sign + formatDecimalDE2(diff)
}

// handleAblesungDetail shows one period's meter readings and full cost
// breakdown (Kostenaufstellung, Ticket #43, generalized from the old
// "letzte Ablesung" view to any period by id), with a dropdown to jump to
// any other period.
func handleAblesungDetail(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}

		period, err := store.GetPeriodDetails(db, periodID)
		if err != nil {
			http.Error(w, "period: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if period == nil {
			http.NotFound(w, r)
			return
		}

		allPeriods, err := store.AllPeriods(db)
		if err != nil {
			http.Error(w, "periods: "+err.Error(), http.StatusInternalServerError)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// ZeitraumStart is the immediately preceding period's ReadingDate -
		// the values shown here are the *consumption over that interval*,
		// not just a single point-in-time reading, so the detail view
		// spells out the whole span, not only its end date.
		var zeitraumStart string
		vorperiode, err := store.PeriodReadingsBefore(db, period.ID, 1)
		if err != nil {
			http.Error(w, "vorperiode: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if len(vorperiode) > 0 {
			zeitraumStart = vorperiode[0].ReadingDate
		}

		status := newTeilstandStatus(period, apartments)

		// Diff is each meter's absolute change since the previous reading
		// ("+23,00" / "-5,00"), formatted ready-to-print - empty for the
		// oldest period (no previous period to diff against). Erfasst
		// (recorded, Ticket #131) comes from status.MeterErfasst - unlike
		// Value (always 0 for a missing meter reading, since Go's map
		// access has no "missing" distinction).
		var meters []struct {
			Label   string
			Value   float64
			Unit    string
			Diff    string
			Erfasst bool
		}
		for _, m := range meterDisplays {
			entry := struct {
				Label   string
				Value   float64
				Unit    string
				Diff    string
				Erfasst bool
			}{Label: m.Label, Value: period.Readings[m.Key], Unit: m.Unit, Erfasst: status.MeterErfasst[m.Key]}
			if len(vorperiode) > 0 {
				entry.Diff = formatMeterDiff(period.Readings[m.Key], vorperiode[0].Readings[m.Key])
			}
			meters = append(meters, entry)
		}

		k, err := berechneKosten(db, period.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			navData
			teilstandStatus
			Aktuell       string
			Period        *store.LatestPeriod
			AllPeriods    []periodListItem
			Apartments    []store.Apartment
			Personen      map[int64]int64
			ZeitraumStart string
			Meters        []struct {
				Label   string
				Value   float64
				Unit    string
				Diff    string
				Erfasst bool
			}
			Strom       *calc.StromErgebnis
			Wasser      *calc.WasserErgebnis
			Heizung     *calc.HeizungErgebnis
			Einspeisung *calc.EinspeisungErgebnis
			KostenNote  string
			MonatLabel  string
		}{
			navData:         a.NavData(r),
			teilstandStatus: status,
			Aktuell:         "ablesungen-detail",
			Period:          period,
			AllPeriods:      periodListItems(allPeriods),
			Apartments:      apartments,
			Personen:        period.PersonenByApartment,
			ZeitraumStart:   zeitraumStart,
			Meters:          meters,
			Strom:           k.Strom,
			Wasser:          k.Wasser,
			Heizung:         k.Heizung,
			Einspeisung:     k.Einspeisung,
			KostenNote:      k.KostenNote,
			MonatLabel:      germanPeriodLabel(period.Monat),
		}

		if err := ablesungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
