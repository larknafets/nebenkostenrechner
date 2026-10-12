package calc

import (
	"math"

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

	// KostenWallbox is the cost of the whole Zwischenzähler "Wallboxen"
	// (Ticket #67), rounded to the cent using commercial rounding (Issue
	// #8). Whether and how it is split between the apartments is decided by
	// the KostenWallboxW1/W2 fields below; unassigned, it stays purely
	// informational.
	KostenWallbox float64

	// WallboxVerteilt reports whether the Wallbox costs are assigned to the
	// apartments (#225): the switch "Wallboxen vollständig Wohnung 1" is on,
	// or Wohnung 2's Einzelzähler has a consumption. Otherwise the fields
	// below are all 0 and KostenWallbox stays informational.
	WallboxVerteilt bool
	// KostenWallboxW1/W2 always add up to KostenWallbox (to the cent) when
	// WallboxVerteilt. Wohnung 1 is the remainder, including the share not
	// attributed to grid draw.
	KostenWallboxW1 float64
	KostenWallboxW2 float64
	// WallboxW1KWh/W2KWh split WallboxAnteilKWh the same way.
	WallboxW1KWh float64
	WallboxW2KWh float64

	// WallboxEinzelPruefbar is true when both Einzelzähler have a
	// consumption, only then the plausibility check against the
	// Zwischenzähler is made. WallboxEinzelSumme is the sum of both
	// consumptions, WallboxEinzelAuffaellig says it deviates from the
	// Zwischenzähler by more than 5 % or 2 kWh, whichever is larger. Never
	// blocks anything (#225).
	WallboxEinzelPruefbar   bool
	WallboxEinzelSumme      float64
	WallboxEinzelAuffaellig bool
}

// stromEingabe is everything the electricity allocation reads: the
// period's price and its meter consumptions (store.Verbrauch).
type stromEingabe struct {
	Strompreis float64
	Verbrauch  map[string]float64
	// EinzelVerbrauch holds the consumption of each optional Einzelzähler
	// that is calculable (reading in the Ablesung and the Vorperiode); a
	// missing key means "nicht berechenbar".
	EinzelVerbrauch map[string]float64
	// WallboxKomplettW1 is the period's switch "Wallboxen vollständig
	// Wohnung 1".
	WallboxKomplettW1 bool
}

// Plausibility thresholds for the Wallbox Einzelzähler (#225).
const (
	wallboxAbweichungProzent = 0.05
	wallboxAbweichungMinKWh  = 2.0
)

// berechneStrom is the allocation itself, a pure function of its input.
func berechneStrom(in stromEingabe) *StromErgebnis {
	verbrauch := in.Verbrauch
	netzbezugGesamt := verbrauch["strom_gesamt"]

	w2Anteil := min(netzbezugGesamt, verbrauch["strom_wohnung2"])
	rest1 := netzbezugGesamt - w2Anteil
	wpAnteil := min(rest1, verbrauch["strom_waermepumpe"])
	rest2 := rest1 - wpAnteil
	wallboxAnteil := min(rest2, verbrauch["strom_wallbox"])

	e := &StromErgebnis{
		NetzbezugGesamtKWh:      netzbezugGesamt,
		W2AnteilKWh:             w2Anteil,
		WPAnteilKWh:             wpAnteil,
		WallboxAnteilKWh:        wallboxAnteil,
		W2VerbrauchKWh:          verbrauch["strom_wohnung2"],
		PVAnteilW2KWh:           max(0, verbrauch["strom_wohnung2"]-w2Anteil),
		PVAnteilWPKWh:           max(0, verbrauch["strom_waermepumpe"]-wpAnteil),
		PVAnteilWallboxKWh:      max(0, verbrauch["strom_wallbox"]-wallboxAnteil),
		KostenW2:                Round2(w2Anteil * in.Strompreis),
		KostenWPGesamtUnrounded: wpAnteil * in.Strompreis,
		KostenWallbox:           Round2(wallboxAnteil * in.Strompreis),
	}
	verteileWallbox(e, in)
	return e
}

// verteileWallbox splits the Wallbox costs between the apartments (#225,
// decision in #219): the switch puts everything on apartment 1; otherwise
// apartment 2 pays its own consumption capped at the grid-draw share, and
// apartment 1 carries the rest (so the sum stays cent-exact). Without the
// switch and without Wohnung 2's consumption nothing is assigned.
func verteileWallbox(e *StromErgebnis, in stromEingabe) {
	w1, ok1 := in.EinzelVerbrauch[store.MeterKeyWallboxWohnung1]
	w2, ok2 := in.EinzelVerbrauch[store.MeterKeyWallboxWohnung2]

	if ok1 && ok2 {
		zwischen := in.Verbrauch["strom_wallbox"]
		e.WallboxEinzelPruefbar = true
		e.WallboxEinzelSumme = w1 + w2
		schwelle := max(wallboxAbweichungProzent*math.Abs(zwischen), wallboxAbweichungMinKWh)
		e.WallboxEinzelAuffaellig = math.Abs(e.WallboxEinzelSumme-zwischen) > schwelle
	}

	switch {
	case in.WallboxKomplettW1:
		e.WallboxVerteilt = true
		e.KostenWallboxW1, e.WallboxW1KWh = e.KostenWallbox, e.WallboxAnteilKWh
	case ok2:
		e.WallboxVerteilt = true
		e.WallboxW2KWh = min(max(w2, 0), e.WallboxAnteilKWh)
		e.WallboxW1KWh = e.WallboxAnteilKWh - e.WallboxW2KWh
		e.KostenWallboxW2 = Round2(e.WallboxW2KWh * in.Strompreis)
		e.KostenWallboxW1 = Round2(e.KostenWallbox - e.KostenWallboxW2)
	}
}
