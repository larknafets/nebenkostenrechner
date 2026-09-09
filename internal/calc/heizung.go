package calc

import (
	"database/sql"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// HeizungErgebnis is the heating/hot water cost distribution result for one
// period. See https://github.com/larknafets/nebenkostenrechner/issues/16:
// the heat pump's electricity cost (from the electricity cost calculation,
// #14) is distributed to both apartments by heat consumption and apartment
// size, weighted by the period's own HeizungWaermeGewichtung (0.7/0.6/0.5,
// default 0.7 - Issue #27; previously fixed at 70/30).
type HeizungErgebnis struct {
	TotalHeizungskostenUnrounded float64

	WaermeW1MWh float64
	WaermeW2MWh float64
	QMW1        float64
	QMW2        float64

	RatioWaermeW1  float64
	RatioWaermeW2  float64
	RatioFlaecheW1 float64
	RatioFlaecheW2 float64

	// KostenHeizung* are rounded to the cent using commercial rounding
	// (Issue #8).
	KostenHeizungW1 float64
	KostenHeizungW2 float64

	// WPAnteil*KWh is strom.WPAnteilKWh, distributed to the apartments with
	// the same weights as KostenHeizung*.
	WPAnteilW1KWh float64
	WPAnteilW2KWh float64

	// WPVerbrauch*KWh is the actual (raw, without PV deduction) heat pump
	// electricity consumption (strom.WPAnteilKWh+strom.PVAnteilWPKWh),
	// distributed to the apartments with the same weights as WPAnteil*KWh.
	WPVerbrauchW1KWh float64
	WPVerbrauchW2KWh float64
}

// Heizung computes the heating cost allocation for the given period.
func Heizung(db *sql.DB, periodID int64) (*HeizungErgebnis, error) {
	strom, err := Strom(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("strom: %w", err)
	}

	period, err := store.GetPeriodByID(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("period: %w", err)
	}
	gewichtungWaerme := period.HeizungWaermeGewichtung
	gewichtungFlaeche := 1 - gewichtungWaerme

	verbrauch, err := store.Verbrauch(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("verbrauch: %w", err)
	}

	apartments, err := store.Apartments(db)
	if err != nil {
		return nil, fmt.Errorf("apartments: %w", err)
	}
	qmW1, qmW2 := apartmentValues(apartments, func(a store.Apartment) float64 { return a.QM })

	waermeW1 := verbrauch["waerme_wohnung1"]
	waermeW2 := verbrauch["waerme_wohnung2"]

	ratioWaermeW1, ratioWaermeW2 := Ratio2(waermeW1, waermeW2)

	var ratioFlaecheW1, ratioFlaecheW2 float64
	if total := qmW1 + qmW2; total > 0 {
		ratioFlaecheW1 = qmW1 / total
		ratioFlaecheW2 = qmW2 / total
	}

	total := strom.KostenWPGesamtUnrounded

	return &HeizungErgebnis{
		TotalHeizungskostenUnrounded: total,

		WaermeW1MWh: waermeW1,
		WaermeW2MWh: waermeW2,
		QMW1:        qmW1,
		QMW2:        qmW2,

		RatioWaermeW1:  ratioWaermeW1,
		RatioWaermeW2:  ratioWaermeW2,
		RatioFlaecheW1: ratioFlaecheW1,
		RatioFlaecheW2: ratioFlaecheW2,

		KostenHeizungW1: Round2(total * (gewichtungWaerme*ratioWaermeW1 + gewichtungFlaeche*ratioFlaecheW1)),
		KostenHeizungW2: Round2(total * (gewichtungWaerme*ratioWaermeW2 + gewichtungFlaeche*ratioFlaecheW2)),

		WPAnteilW1KWh: strom.WPAnteilKWh * (gewichtungWaerme*ratioWaermeW1 + gewichtungFlaeche*ratioFlaecheW1),
		WPAnteilW2KWh: strom.WPAnteilKWh * (gewichtungWaerme*ratioWaermeW2 + gewichtungFlaeche*ratioFlaecheW2),

		WPVerbrauchW1KWh: (strom.WPAnteilKWh + strom.PVAnteilWPKWh) * (gewichtungWaerme*ratioWaermeW1 + gewichtungFlaeche*ratioFlaecheW1),
		WPVerbrauchW2KWh: (strom.WPAnteilKWh + strom.PVAnteilWPKWh) * (gewichtungWaerme*ratioWaermeW2 + gewichtungFlaeche*ratioFlaecheW2),
	}, nil
}
