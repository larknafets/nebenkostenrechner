package web

import "github.com/larknafets/nebenkostenrechner/internal/store"

// teilstandStatus is the itemized "what's still open" breakdown of a
// Teilstand (partial reading, Ticket #128-#131) - which price, which meter,
// which apartment's Personen are still missing, computed once from an
// already-hydrated *store.LatestPeriod instead of every view re-deriving
// its own nil-checks against Strompreis/Readings/PersonenByApartment.
// store.PeriodComplete stays the DB-level completeness gate (used where
// only a bare periodID is in hand, e.g. requireLoginUnlessTeilstand) - this
// type is specifically the display-shaped itemization for views that
// already paid for a full LatestPeriod.
type teilstandStatus struct {
	MonatErfasst            bool
	StrompreisErfasst       bool
	FrischwasserErfasst     bool
	AbwasserErfasst         bool
	EinspeisungPreisErfasst bool
	MeterErfasst            map[string]bool
	PersonenErfasst         map[int64]bool
	// IstTeilstand is true if any field above is missing - equivalent to
	// !store.PeriodComplete for the same period, but derived from data
	// already in hand instead of a 2nd DB round-trip.
	IstTeilstand bool
}

// newTeilstandStatus computes p's itemized completeness against apartments
// (needed for PersonenErfasst - a missing apartment in PersonenByApartment
// can't be told apart from "0 occupants" otherwise, Ticket #131).
func newTeilstandStatus(p *store.LatestPeriod, apartments []store.Apartment) teilstandStatus {
	s := teilstandStatus{
		MonatErfasst:            p.Monat != "",
		StrompreisErfasst:       p.Strompreis != nil,
		FrischwasserErfasst:     p.FrischwasserPreis != nil,
		AbwasserErfasst:         p.AbwasserPreis != nil,
		EinspeisungPreisErfasst: p.EinspeisungPreis != nil,
		MeterErfasst:            make(map[string]bool, len(store.MeterKeys)),
		PersonenErfasst:         make(map[int64]bool, len(apartments)),
	}

	complete := s.MonatErfasst && s.StrompreisErfasst && s.FrischwasserErfasst && s.AbwasserErfasst && s.EinspeisungPreisErfasst

	for _, key := range store.MeterKeys {
		_, erfasst := p.Readings[key]
		s.MeterErfasst[key] = erfasst
		complete = complete && erfasst
	}
	for _, ap := range apartments {
		_, erfasst := p.PersonenByApartment[ap.ID]
		s.PersonenErfasst[ap.ID] = erfasst
		complete = complete && erfasst
	}

	s.IstTeilstand = !complete
	return s
}
