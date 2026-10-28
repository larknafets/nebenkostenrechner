package web

import "github.com/larknafets/nebenkostenrechner/internal/store"

// ablesungFelder says which of the fields the Ablesung still carries (prices,
// Personenzahl) were recorded. They no longer make a Teilstand and the
// calculation no longer reads them (#359); the views and the form show them
// until the next ticket takes them off the Ablesung.
type ablesungFelder struct {
	StrompreisErfasst       bool
	FrischwasserErfasst     bool
	AbwasserErfasst         bool
	EinspeisungPreisErfasst bool
	PersonenErfasst         map[int64]bool
}

func erfasseFelder(p *store.LatestPeriod, apartments []store.Apartment) ablesungFelder {
	f := ablesungFelder{
		StrompreisErfasst:       p.Strompreis != nil,
		FrischwasserErfasst:     p.FrischwasserPreis != nil,
		AbwasserErfasst:         p.AbwasserPreis != nil,
		EinspeisungPreisErfasst: p.EinspeisungPreis != nil,
		PersonenErfasst:         make(map[int64]bool, len(apartments)),
	}
	for _, ap := range apartments {
		_, f.PersonenErfasst[ap.ID] = p.PersonenByApartment[ap.ID]
	}
	return f
}
