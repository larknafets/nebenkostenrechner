package calc

import (
	"database/sql"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// WasserErgebnis is the water cost allocation result for one period. See
// https://github.com/larknafets/nebenkostenrechner/issues/3 for the
// formula: apartment 2 counts directly via its own submeter, apartment 1
// is the remainder of the total consumption. Hot water preparation is split
// between both apartments by the period's occupant ratio and added to each
// apartment's fresh water share; waste water is assumed to equal the same
// amount as fresh water.
type WasserErgebnis struct {
	PersonenW1 int64
	PersonenW2 int64

	WWAnteilW1 float64
	WWAnteilW2 float64

	FrischwasserW1 float64
	FrischwasserW2 float64
	AbwasserW1     float64
	AbwasserW2     float64

	// Kosten* are rounded to the cent using commercial rounding (Issue #8).
	KostenFrischwasserW1 float64
	KostenFrischwasserW2 float64
	KostenAbwasserW1     float64
	KostenAbwasserW2     float64
}

// Wasser computes the water cost allocation for the given period.
func Wasser(db *sql.DB, periodID int64) (*WasserErgebnis, error) {
	period, err := store.GetPeriodByID(db, periodID)
	if err != nil {
		return nil, err
	}

	verbrauch, err := store.Verbrauch(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("verbrauch: %w", err)
	}

	personen, err := store.PersonenByApartment(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("personen: %w", err)
	}
	p1, p2 := personen[1], personen[2]

	wwGesamt := verbrauch["wasser_warmwasseraufbereitung"]
	ratioP1, ratioP2 := Ratio2(float64(p1), float64(p2))
	wwAnteilW1 := wwGesamt * ratioP1
	wwAnteilW2 := wwGesamt * ratioP2

	frischwasserW2 := verbrauch["wasser_wohnung2"] + wwAnteilW2
	frischwasserW1 := (verbrauch["wasser_gesamt"] - verbrauch["wasser_wohnung2"] - wwGesamt) + wwAnteilW1

	return &WasserErgebnis{
		PersonenW1: p1,
		PersonenW2: p2,

		WWAnteilW1: wwAnteilW1,
		WWAnteilW2: wwAnteilW2,

		FrischwasserW1: frischwasserW1,
		FrischwasserW2: frischwasserW2,
		AbwasserW1:     frischwasserW1,
		AbwasserW2:     frischwasserW2,

		KostenFrischwasserW1: Round2(frischwasserW1 * period.FrischwasserPreis),
		KostenFrischwasserW2: Round2(frischwasserW2 * period.FrischwasserPreis),
		KostenAbwasserW1:     Round2(frischwasserW1 * period.AbwasserPreis),
		KostenAbwasserW2:     Round2(frischwasserW2 * period.AbwasserPreis),
	}, nil
}
