package store

// Wohnungsregeln answers which items are visible for one apartment, given its
// Wohnungsstatus and the flags of the Haus. It is the single place of the
// Wohnungsstatus table (GLOSSARY.md); callers ask a named question instead of
// reading Status or the flags themselves.
type Wohnungsregeln struct {
	apartment Apartment
	haus      Haus
}

// NewWohnungsregeln builds the rules for one apartment.
func NewWohnungsregeln(a Apartment, h Haus) Wohnungsregeln {
	return Wohnungsregeln{apartment: a, haus: h}
}

// AbrechnungStrom reports whether the Haushaltsstrom of the apartment stands
// in its Jahresabrechnung. Eigennutzung: always. Vermietung: only with the
// flag "Stromverbrauch weiterberechnen", which applies to Wohnung 1 and 2 alike.
func (s Wohnungsregeln) AbrechnungStrom() bool {
	return !s.apartment.Vermietet() || s.haus.StromWeiterberechnen
}

// AbrechnungWallbox reports whether the Wallbox block stands in the
// Jahresabrechnung. Eigennutzung: always. Vermietung: only with the flag
// "Wallbox weiterberechnen".
func (s Wohnungsregeln) AbrechnungWallbox() bool {
	return !s.apartment.Vermietet() || s.haus.WallboxWeiterberechnen
}

// FixkostenAbschlagMiete reports whether the Fixkosten form shows and reads
// the Abschlag and the Miete of the apartment: Vermietung only.
func (s Wohnungsregeln) FixkostenAbschlagMiete() bool {
	return s.apartment.Vermietet()
}

// FixkostenDetailMiete reports whether the Fixkosten detail page shows the
// Miete of the apartment: Vermietung only.
func (s Wohnungsregeln) FixkostenDetailMiete() bool {
	return s.apartment.Vermietet()
}

// Mieteinnahmen reports whether Mieteinnahmen exist for the apartment:
// Vermietung only.
func (s Wohnungsregeln) Mieteinnahmen() bool {
	return s.apartment.Vermietet()
}

// JahreskarteWallboxZeile reports whether the Jahreskarte of the Dashboard
// shows the Wallbox line. It drops for a rented apartment whose Wallbox is not
// passed on ("Wallbox weiterberechnen" off) and whose amount is 0; the amount
// stays an argument because it comes from the cost calculation.
func (s Wohnungsregeln) JahreskarteWallboxZeile(betragEUR float64) bool {
	return !(s.apartment.Vermietet() && !s.haus.WallboxWeiterberechnen && betragEUR == 0)
}

// AbrechnungPVGutschrift reports whether the PV-Gutschrift stands in the
// Jahresabrechnung: Wohnung 1 in Eigennutzung only (the Gutschrift follows its
// assignment to Wohnung 1).
func (s Wohnungsregeln) AbrechnungPVGutschrift() bool {
	return s.apartment.ID == 1 && !s.apartment.Vermietet()
}
