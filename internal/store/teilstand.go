package store

// TeilstandStatus is the itemized "what's still open" breakdown of a
// Teilstand (partial reading, Ticket #128-#131) - whether the Abrechnungsmonat
// and which meter are still missing, computed once from an already-hydrated
// *LatestPeriod instead of every view re-deriving its own nil-checks. Prices,
// switches and Personenzahl are no part of it: they belong to the
// Fixkosten-Eingabe of the month (#359).
// PeriodComplete answers the same question for callers that only hold a bare
// periodID (e.g. teilstandAnspruch) by loading the period and
// asking Teilstand, so there is only one rule.
type TeilstandStatus struct {
	MonatErfasst bool
	MeterErfasst map[string]bool
	// IstTeilstand is true if any field above is missing - the negation of
	// PeriodComplete, derived from data already in hand.
	IstTeilstand bool
}

// Teilstand computes p's itemized completeness: an Ablesung is complete with
// its Abrechnungsmonat and every mandatory meter reading. apartments is not
// read any more and goes with the Personenzahl of the Ablesung (#359).
func (p *LatestPeriod) Teilstand(_ []Apartment) TeilstandStatus {
	s := TeilstandStatus{
		MonatErfasst: p.Monat != "",
		MeterErfasst: make(map[string]bool, len(MeterKeys)),
	}

	complete := s.MonatErfasst

	for _, key := range MeterKeys {
		_, erfasst := p.Readings[key]
		s.MeterErfasst[key] = erfasst
		complete = complete && erfasst
	}
	// The optional Einzelzähler (#224) only report whether they were
	// entered, they never make a Teilstand.
	for _, key := range OptionalMeterKeys {
		_, s.MeterErfasst[key] = p.Readings[key]
	}

	s.IstTeilstand = !complete
	return s
}
