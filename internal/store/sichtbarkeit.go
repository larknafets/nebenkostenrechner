package store

// Sichtbarkeit answers which items are visible for one apartment, given its
// Wohnungsstatus and the flags of the Haus. It is the single place of the
// Wohnungsstatus table (GLOSSARY.md); callers ask a named question instead of
// reading Status or the flags themselves.
type Sichtbarkeit struct {
	apartment Apartment
	haus      Haus
}

// NewSichtbarkeit builds the rules for one apartment.
func NewSichtbarkeit(a Apartment, h Haus) Sichtbarkeit {
	return Sichtbarkeit{apartment: a, haus: h}
}

// AbrechnungStrom reports whether the Haushaltsstrom of the apartment stands
// in its Jahresabrechnung. Eigennutzung: always. Vermietung: only with the
// flag "Stromverbrauch weiterberechnen", which applies to Wohnung 1 and 2 alike.
func (s Sichtbarkeit) AbrechnungStrom() bool {
	return !s.apartment.Vermietet() || s.haus.StromWeiterberechnen
}

// AbrechnungWallbox reports whether the Wallbox block stands in the
// Jahresabrechnung. Eigennutzung: always. Vermietung: only with the flag
// "Wallbox weiterberechnen".
func (s Sichtbarkeit) AbrechnungWallbox() bool {
	return !s.apartment.Vermietet() || s.haus.WallboxWeiterberechnen
}

// AbrechnungPVGutschrift reports whether the PV-Gutschrift stands in the
// Jahresabrechnung: Wohnung 1 in Eigennutzung only (the Gutschrift follows its
// assignment to Wohnung 1).
func (s Sichtbarkeit) AbrechnungPVGutschrift() bool {
	return s.apartment.ID == 1 && !s.apartment.Vermietet()
}
