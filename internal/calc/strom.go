package calc

import (
	"database/sql"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// StromErgebnis is the PV grid-draw allocation result for one period. See
// https://github.com/larknafets/nebenkostenrechner/issues/2 for the
// formula: grid draw is allocated to apartment 2 first (capped at its own
// consumption), then the heat pump against the remainder, then the wallboxes
// against whatever remains after that (Ticket #67 follow-up) - whatever's
// left after that implicitly counts toward apartment 1 (no cost position of
// its own).
type StromErgebnis struct {
	NetzbezugGesamtKWh float64
	W2AnteilKWh        float64
	WPAnteilKWh        float64
	WallboxAnteilKWh   float64

	// W2VerbrauchKWh is apartment 2's actual (raw) submeter consumption,
	// without the PV deduction - unlike W2AnteilKWh (the billed share, capped
	// at grid draw). Equal to W2AnteilKWh+PVAnteilW2KWh.
	W2VerbrauchKWh float64

	// PVAnteilW2KWh/PVAnteilWPKWh/PVAnteilWallboxKWh is the gap between the
	// submeter's own consumption and what the min()-cap actually attributed
	// to grid draw - since the submeters read gross consumption while grid
	// draw is net of PV self-consumption, a gap can only exist because
	// PV covered it (Ticket #50: "not attributed to grid draw (PV)").
	PVAnteilW2KWh      float64
	PVAnteilWPKWh      float64
	PVAnteilWallboxKWh float64

	// KostenW2 is the displayed cost position for apartment 2 - rounded to
	// the cent using commercial rounding (Issue #8).
	KostenW2 float64

	// KostenWPGesamtUnrounded is the (not yet rounded) total cost of the
	// heat pump's electricity - it isn't a displayed position on its own,
	// it's the input the heating cost calculation (Ticket #7 / #16)
	// splits 70/30 between the two apartments.
	KostenWPGesamtUnrounded float64

	// KostenWallbox is purely informational (Ticket #67) - wallbox usage
	// doesn't get its own apartment allocation, it still implicitly runs
	// through apartment 1's remainder. Rounded to the cent using commercial
	// rounding (Issue #8).
	KostenWallbox float64
}

// Strom computes the electricity cost allocation for the given period.
func Strom(db *sql.DB, periodID int64) (*StromErgebnis, error) {
	period, err := store.GetPeriodByID(db, periodID)
	if err != nil {
		return nil, err
	}

	verbrauch, err := store.Verbrauch(db, periodID)
	if err != nil {
		return nil, fmt.Errorf("verbrauch: %w", err)
	}

	netzbezugGesamt := verbrauch["strom_gesamt"]

	w2Anteil := min(netzbezugGesamt, verbrauch["strom_wohnung2"])
	rest1 := netzbezugGesamt - w2Anteil
	wpAnteil := min(rest1, verbrauch["strom_waermepumpe"])
	rest2 := rest1 - wpAnteil
	wallboxAnteil := min(rest2, verbrauch["strom_wallbox"])

	return &StromErgebnis{
		NetzbezugGesamtKWh:      netzbezugGesamt,
		W2AnteilKWh:             w2Anteil,
		WPAnteilKWh:             wpAnteil,
		WallboxAnteilKWh:        wallboxAnteil,
		W2VerbrauchKWh:          verbrauch["strom_wohnung2"],
		PVAnteilW2KWh:           max(0, verbrauch["strom_wohnung2"]-w2Anteil),
		PVAnteilWPKWh:           max(0, verbrauch["strom_waermepumpe"]-wpAnteil),
		PVAnteilWallboxKWh:      max(0, verbrauch["strom_wallbox"]-wallboxAnteil),
		KostenW2:                Round2(w2Anteil * period.Strompreis),
		KostenWPGesamtUnrounded: wpAnteil * period.Strompreis,
		KostenWallbox:           Round2(wallboxAnteil * period.Strompreis),
	}, nil
}
