package store

// Wohnungsregeln answers which items are visible for one apartment, given its
// Wohnungsstatus and the flags of the Haus. It is the single place of the
// Wohnungsstatus table (GLOSSARY.md); callers ask a named question instead of
// reading Status or the flags themselves.
type Wohnungsregeln struct {
	apartment Apartment
	haus      Haus
	// hatHaus is false for rules built without a Haus: the flag-dependent
	// questions then panic instead of answering as if every flag were off.
	hatHaus bool
}

// NewWohnungsregeln builds the rules for one apartment with the flags of the
// Haus. Use it whenever the caller has the Haus.
func NewWohnungsregeln(a Apartment, h Haus) Wohnungsregeln {
	return Wohnungsregeln{apartment: a, haus: h, hatHaus: true}
}

// NewWohnungsregelnOhneHaus builds the rules for callers that have no Haus
// at hand and only ask Haus-free questions (FixkostenAbschlagMiete,
// FixkostenDetailMiete, Mieteinnahmen, AbrechnungPVGutschrift). A question
// that depends on a Haus flag (AbrechnungStrom, AbrechnungWallbox) panics on
// these rules: a programming error, not a silent "flag off".
func NewWohnungsregelnOhneHaus(a Apartment) Wohnungsregeln {
	return Wohnungsregeln{apartment: a}
}

// flags returns the Haus flags and panics if the rules were built without
// a Haus.
func (s Wohnungsregeln) flags() Haus {
	if !s.hatHaus {
		panic("store: Wohnungsregeln ohne Haus: Frage braucht die Haus-Flags")
	}
	return s.haus
}

// AbrechnungStrom reports whether the Haushaltsstrom of the apartment stands
// in its Jahresabrechnung. Eigennutzung: always. Vermietung: only with the
// flag "Stromverbrauch weiterberechnen", which applies to Wohnung 1 and 2 alike.
func (s Wohnungsregeln) AbrechnungStrom() bool {
	h := s.flags()
	return !s.apartment.Vermietet() || h.StromWeiterberechnen
}

// AbrechnungWallbox reports whether the Wallbox block stands in the
// Jahresabrechnung. Eigennutzung: always. Vermietung: only with the flag
// "Wallbox weiterberechnen".
func (s Wohnungsregeln) AbrechnungWallbox() bool {
	h := s.flags()
	return !s.apartment.Vermietet() || h.WallboxWeiterberechnen
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

// AbrechnungPVGutschrift reports whether the PV-Gutschrift stands in the
// Jahresabrechnung: Wohnung 1 in Eigennutzung only (the Gutschrift follows its
// assignment to Wohnung 1).
func (s Wohnungsregeln) AbrechnungPVGutschrift() bool {
	return s.apartment.ID == 1 && !s.apartment.Vermietet()
}
