package calc

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

// wasserEingabe is everything the water allocation reads: the period's
// prices, its meter consumptions (store.Verbrauch) and the occupants of
// both apartments.
type wasserEingabe struct {
	FrischwasserPreis, AbwasserPreis float64
	Verbrauch                        map[string]float64
	PersonenW1, PersonenW2           int64
}

// berechneWasser is the allocation itself, a pure function of its input.
func berechneWasser(in wasserEingabe) *WasserErgebnis {
	verbrauch := in.Verbrauch
	p1, p2 := in.PersonenW1, in.PersonenW2

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

		KostenFrischwasserW1: Round2(frischwasserW1 * in.FrischwasserPreis),
		KostenFrischwasserW2: Round2(frischwasserW2 * in.FrischwasserPreis),
		KostenAbwasserW1:     Round2(frischwasserW1 * in.AbwasserPreis),
		KostenAbwasserW2:     Round2(frischwasserW2 * in.AbwasserPreis),
	}
}
