package calc

// EinspeisungErgebnis is the PV feed-in compensation result for one period -
// whole-house, no apartment split (the feed-in meter isn't apartment-
// specific), unlike StromErgebnis/WasserErgebnis/HeizungErgebnis (Ticket #47).
type EinspeisungErgebnis struct {
	EinspeisungKWh float64

	// Ertrag is rounded to the cent using commercial rounding (Issue #8 convention).
	Ertrag float64
}

// einspeisungEingabe is everything the feed-in compensation reads.
type einspeisungEingabe struct {
	EinspeisungPreis float64
	Verbrauch        map[string]float64
}

// berechneEinspeisung is the compensation itself, a pure function of its
// input.
func berechneEinspeisung(in einspeisungEingabe) *EinspeisungErgebnis {
	kwh := in.Verbrauch["strom_einspeisung"]
	return &EinspeisungErgebnis{
		EinspeisungKWh: kwh,
		Ertrag:         Round2(kwh * in.EinspeisungPreis),
	}
}
