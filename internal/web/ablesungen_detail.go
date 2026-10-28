package web

import (
	"net/http"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// meterDisplayOrder is the order of the meters on the Ablesung detail view,
// it matches the wizard's step grouping. Label, unit, Optional and
// Eingerueckt come from the store's meter descriptor.
var meterDisplayOrder = []string{
	"strom_gesamt",
	"strom_einspeisung",
	"strom_wohnung2",
	"strom_waermepumpe",
	"strom_wallbox",
	store.MeterKeyWallboxWohnung1,
	store.MeterKeyWallboxWohnung2,
	"waerme_wohnung1",
	"waerme_wohnung2",
	"wasser_gesamt",
	"wasser_wohnung2",
	"wasser_warmwasseraufbereitung",
}

// meterDisplays resolves meterDisplayOrder against the meter descriptor.
var meterDisplays = func() []store.Meter {
	out := make([]store.Meter, 0, len(meterDisplayOrder))
	for _, key := range meterDisplayOrder {
		m, ok := store.MeterByKey(key)
		if !ok {
			panic("unknown meter key " + key)
		}
		if m.Unit == "m3" {
			m.Unit = "m³"
		}
		out = append(out, m)
	}
	return out
}()

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
func handleAblesungDetail(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}

		// One load serves the Ablesung, its Vorperiode, the dropdown and the
		// Kosten: ds.Periods is ordered oldest first by reading date, then id.
		ds, err := store.LadeDatenstand(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		apartments := ds.Apartments

		var period *store.LatestPeriod
		var vorperiode []store.PeriodReadings
		allPeriods := make([]store.PeriodSummary, len(ds.Periods))
		for i, p := range ds.Periods {
			allPeriods[len(ds.Periods)-1-i] = store.PeriodSummary{ID: p.ID, ReadingDate: p.ReadingDate, Monat: p.Monat}
			if p.ID != periodID {
				continue
			}
			period = p
			if i > 0 {
				v := ds.Periods[i-1]
				vorperiode = []store.PeriodReadings{{ID: v.ID, ReadingDate: v.ReadingDate, Readings: v.Readings}}
			}
		}
		if period == nil {
			http.NotFound(w, r)
			return
		}

		// ZeitraumStart is the immediately preceding period's ReadingDate -
		// the values shown here are the *consumption over that interval*,
		// not just a single point-in-time reading, so the detail view
		// spells out the whole span, not only its end date.
		var zeitraumStart string
		if len(vorperiode) > 0 {
			zeitraumStart = vorperiode[0].ReadingDate
		}

		status := period.Teilstand()

		// Diff is each meter's absolute change since the previous reading
		// ("+23,00" / "-5,00"), formatted ready-to-print - empty for the
		// oldest period (no previous period to diff against). Erfasst
		// (recorded, Ticket #131) comes from status.MeterErfasst - unlike
		// Value (always 0 for a missing meter reading, since Go's map
		// access has no "missing" distinction).
		var meters []meterZeile
		for _, m := range meterDisplays {
			entry := meterZeile{Label: m.AblesungLabel, Value: period.Readings[m.Key], Unit: m.Unit, Erfasst: status.MeterErfasst[m.Key], Optional: m.Optional, Eingerueckt: m.Eingerueckt}
			if len(vorperiode) > 0 {
				// No consumption unless both readings exist, never computed
				// against 0. A recorded optional Einzelzähler (#224) without
				// a Vorperiode stand is "nicht berechenbar".
				if d, ok := store.Verbrauch(period.Readings, vorperiode[0].Readings, m.Key); ok {
					entry.Diff = formatMeterDiff(d, 0)
				} else if m.Optional && entry.Erfasst {
					entry.NichtBerechenbar = true
				}
			}
			meters = append(meters, entry)
		}

		v := calc.KostenAus(ds)
		ablesung, _ := v.Ablesung(period.ID)
		k := ablesung.Kosten

		fix := fixkostenAnsicht(ds, period)
		var hinweis *fixkostenHinweis
		var wallboxKWh float64
		if ablesung.Grund == calc.GrundFixkostenFehlen {
			hinweis = ablesungFixkostenHinweis(v, period.Monat)
			if len(vorperiode) > 0 {
				verbrauch := make(map[string]float64, len(store.MeterKeys))
				for _, key := range store.MeterKeys {
					verbrauch[key], _ = store.Verbrauch(period.Readings, vorperiode[0].Readings, key)
				}
				wallboxKWh = calc.WallboxNetzbezugKWh(verbrauch)
			}
		}

		data := struct {
			navData
			store.TeilstandStatus
			Aktuell       string
			Period        *store.LatestPeriod
			AllPeriods    []periodListItem
			Apartments    []store.Apartment
			ZeitraumStart string
			Meters        []meterZeile
			Strom         *calc.StromErgebnis
			Wasser        *calc.WasserErgebnis
			Heizung       *calc.HeizungErgebnis
			Einspeisung   *calc.EinspeisungErgebnis
			KostenNote    string
			MonatLabel    string
			// FixkostenBanner is set right after saving an Ablesung whose
			// Abrechnungsmonat lacks its Fixkosten (#366).
			FixkostenBanner *fixkostenHinweis

			// Fix reads the Fixkosten-Eingabe of the month, FixkostenHinweis
			// is set when it is missing (#359), instead of the Euro blocks.
			Fix              ablesungFixkosten
			FixkostenHinweis *fixkostenHinweis
			// WallboxKWh is the grid-draw share of the Wallboxen, a pure meter
			// value shown next to the hint.
			WallboxKWh float64
		}{
			navData:          z.NavData(r),
			TeilstandStatus:  status,
			Aktuell:          "ablesungen-detail",
			Period:           period,
			AllPeriods:       periodListItems(allPeriods),
			Apartments:       apartments,
			ZeitraumStart:    zeitraumStart,
			Meters:           meters,
			Strom:            k.Strom,
			Wasser:           k.Wasser,
			Heizung:          k.Heizung,
			Einspeisung:      k.Einspeisung,
			KostenNote:       kostenHinweis(ablesung.Grund),
			MonatLabel:       store.MonatLabel(period.Monat),
			Fix:              fix,
			FixkostenHinweis: hinweis,
			WallboxKWh:       wallboxKWh,
		}
		// One note per page: the banner only where the Kostenbereich shows none
		// (Teilstand, first Ablesung).
		if r.URL.Query().Get(fixkostenFehlenParam) != "" && hinweis == nil {
			data.FixkostenBanner = ablesungFixkostenHinweis(v, period.Monat)
		}

		if err := ablesungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// ablesungFixkosten is what the Ablesung detail view reads from the
// Fixkosten-Eingabe of the Abrechnungsmonat: prices and the two switches
// (#359). Vorhanden is false when the month has no Eingabe.
type ablesungFixkosten struct {
	Vorhanden         bool
	EingabeID         int64
	Preise            []fixkostenDetailPreis
	WallboxKomplettW1 bool
	PVGutschriftW1    bool
}

// fixkostenAnsicht finds the Fixkosten-Eingabe of the period's Abrechnungsmonat
// (the last one if there are several, like the calculation does).
func fixkostenAnsicht(ds *store.Datenstand, period *store.LatestPeriod) ablesungFixkosten {
	var eingabe *store.FixkostenEingabeDetails
	for _, e := range ds.Eingaben {
		if e.Monat == period.Monat {
			eingabe = e
		}
	}
	if eingabe == nil {
		return ablesungFixkosten{}
	}
	preise, _ := buildFixkostenDetailPreise(eingabe)
	return ablesungFixkosten{
		Vorhanden: true, EingabeID: eingabe.ID, Preise: preise,
		WallboxKomplettW1: eingabe.WallboxKomplettW1, PVGutschriftW1: eingabe.PVGutschriftW1,
	}
}
