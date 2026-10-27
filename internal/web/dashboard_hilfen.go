package web

import (
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboardSegment is one colored slice of a Dashboard bar - shared shape
// for the yearly-totals cards' mini-bar, the Monatsverlauf's consumption/
// fixed-costs/combined bars, and (via ProzentNeuestesGesamt/ProzentGesamt,
// reused for either "percent of the newest month" or "percent of this
// card's own total" depending on the call site) every percentage-scaled bar
// on the page.
type dashboardSegment struct {
	Farbe                 string
	Label                 string
	Kosten                float64
	Verbrauch             float64 // amount (kWh/MWh/m³) - only set for consumption categories, 0 for Fixkosten/Kombiniert
	Einheit               string
	ProzentNeuestesGesamt float64

	// Verbrauch2/Einheit2 is a second, optional quantity shown behind
	// Verbrauch/Einheit in the consumption values view - only set for
	// Heizung/Warmwasser (heating/hot water, see kategorie.Verbrauch2).
	// Einheit2 == "" means no second value.
	Verbrauch2 float64
	Einheit2   string
}

// setSegmentPct fills every segment's ProzentNeuestesGesamt as its share of
// denom (percent, not compressed to fit - Ticket #19's established
// convention: an older/bigger total can run past 100%). No-op if denom<=0.
func setSegmentPct(segs []dashboardSegment, denom float64) {
	if denom <= 0 {
		return
	}
	for i := range segs {
		segs[i].ProzentNeuestesGesamt = segs[i].Kosten / denom * 100
	}
}

// fixkostenKosten is one fixed-costs entry's month (Monat) and computed
// result (Ergebnis).
type fixkostenKosten struct {
	Monat    string
	Erg      *calc.FixkostenErgebnis
	Abschlag map[int64]float64 // apartment id -> utility advance payment value, nil/missing entry = no value recorded
	Personen map[int64]float64 // apartment id -> occupant count of the Fixkosten-Eingabe (one per Monat)
}

// alleFixkostenKosten returns every computable fixed-costs entry's result,
// newest first.
func alleFixkostenKosten(reihe *calc.Fixkostenreihe) []fixkostenKosten {
	neueste := reihe.Neueste()
	out := make([]fixkostenKosten, 0, len(neueste))
	for _, m := range neueste {
		out = append(out, fixkostenKosten{Monat: m.Eingabe.Monat, Erg: m.Ergebnis, Abschlag: m.Eingabe.Abschlag, Personen: m.Eingabe.Personen})
	}
	return out
}

// fixkostenGruppen groups one fixed-costs result's 14 positions by Logik
// (allocation logic) into the 4 fixed buckets the "Fixkosten" mode bar
// shows (always all 4, even at 0 - same "show every category" convention
// as kategorien()).
func fixkostenGruppen(apartmentID int64, erg *calc.FixkostenErgebnis) []dashboardSegment {
	sums := map[string]float64{}
	for _, p := range erg.Positionen {
		sums[p.Logik] += p.KostenFor(apartmentID)
	}
	logiken := []string{store.LogikWohneinheit, store.LogikFlurstueck, store.LogikQM, store.LogikPersonen, store.LogikWohnung1, store.LogikWohnung2}
	out := make([]dashboardSegment, len(logiken))
	for i, logik := range logiken {
		out[i] = dashboardSegment{Farbe: "logik-" + logik, Label: calc.LogikLabels[logik], Kosten: calc.Round2(sums[logik])}
	}
	return out
}
