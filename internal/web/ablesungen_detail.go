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
	// Optional marks a meter a period may leave out (#224): shown as "nicht
	// erfasst" instead of "noch offen", consumption only with both readings.
	Optional bool
	// Eingerueckt indents the row below its parent meter (Wallboxen).
	Eingerueckt bool
}

var meterDisplays = []meterDisplay{
	{Key: "strom_gesamt", Label: "Strom Netzbezug (1.8.0)", Unit: "kWh"},
	{Key: "strom_einspeisung", Label: "Strom Einspeisung (2.8.0)", Unit: "kWh"},
	{Key: "strom_wohnung2", Label: "Strom Wohnung 2", Unit: "kWh"},
	{Key: "strom_waermepumpe", Label: "Strom Wärmepumpe", Unit: "kWh"},
	{Key: "strom_wallbox", Label: "Strom Wallboxen", Unit: "kWh"},
	{Key: store.MeterKeyWallboxWohnung1, Label: "Wallbox Wohnung 1", Unit: "kWh", Optional: true, Eingerueckt: true},
	{Key: store.MeterKeyWallboxWohnung2, Label: "Wallbox Wohnung 2", Unit: "kWh", Optional: true, Eingerueckt: true},
	{Key: "waerme_wohnung1", Label: "Wärme Wohnung 1", Unit: "MWh"},
	{Key: "waerme_wohnung2", Label: "Wärme Wohnung 2", Unit: "MWh"},
	{Key: "wasser_gesamt", Label: "Wasser Gesamt", Unit: "m³"},
	{Key: "wasser_wohnung2", Label: "Wasser Wohnung 2", Unit: "m³"},
	{Key: "wasser_warmwasseraufbereitung", Label: "Wasser Warmwasseraufbereitung", Unit: "m³"},
}

// meterZeile is one row of the Zähler table on the Ablesung detail view.
type meterZeile struct {
	Label   string
	Value   float64
	Unit    string
	Diff    string
	Erfasst bool
	// Optional/Eingerueckt: see meterDisplay.
	Optional    bool
	Eingerueckt bool
	// NichtBerechenbar: the Stand exists, but the consumption can't be
	// computed because the previous reading lacks it.
	NichtBerechenbar bool
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

		status := period.Teilstand(apartments)

		// Diff is each meter's absolute change since the previous reading
		// ("+23,00" / "-5,00"), formatted ready-to-print - empty for the
		// oldest period (no previous period to diff against). Erfasst
		// (recorded, Ticket #131) comes from status.MeterErfasst - unlike
		// Value (always 0 for a missing meter reading, since Go's map
		// access has no "missing" distinction).
		var meters []meterZeile
		for _, m := range meterDisplays {
			entry := meterZeile{Label: m.Label, Value: period.Readings[m.Key], Unit: m.Unit, Erfasst: status.MeterErfasst[m.Key], Optional: m.Optional, Eingerueckt: m.Eingerueckt}
			if len(vorperiode) > 0 {
				if m.Optional {
					// Optional Einzelzähler (#224): no consumption unless both
					// readings exist, never computed against 0.
					if d, ok := store.VerbrauchOptional(period.Readings, vorperiode[0].Readings, m.Key); ok {
						entry.Diff = formatMeterDiff(d, 0)
					} else if entry.Erfasst {
						entry.NichtBerechenbar = true
					}
				} else {
					entry.Diff = formatMeterDiff(period.Readings[m.Key], vorperiode[0].Readings[m.Key])
				}
			}
			meters = append(meters, entry)
		}

		verbrauchskosten, err := calc.Load(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		ablesung, _ := verbrauchskosten.Ablesung(period.ID)
		k := ablesung.Kosten

		data := struct {
			navData
			store.TeilstandStatus
			Aktuell       string
			Period        *store.LatestPeriod
			AllPeriods    []periodListItem
			Apartments    []store.Apartment
			Personen      map[int64]int64
			ZeitraumStart string
			Meters        []meterZeile
			Strom         *calc.StromErgebnis
			Wasser        *calc.WasserErgebnis
			Heizung       *calc.HeizungErgebnis
			Einspeisung   *calc.EinspeisungErgebnis
			KostenNote    string
			MonatLabel    string
		}{
			navData:         a.NavData(r),
			TeilstandStatus: status,
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
			KostenNote:      kostenHinweis(ablesung.Grund),
			MonatLabel:      store.MonatLabel(period.Monat),
		}

		if err := ablesungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
