package web

import "github.com/larknafets/nebenkostenrechner/internal/store"

// betrachter says who looks at the Dashboard or a widget route. It is the
// only input that decides what is visible (#293).
type betrachter int

const (
	// betrachterAnonym is a visitor who is not logged in (LOGIN_PASSWORD set):
	// only Wohnung 2, no Wallboxen/PV-Anlage, no Mieteinnahmen.
	betrachterAnonym betrachter = iota
	// betrachterAngemeldet is a logged-in user: everything, with Mieteinnahmen.
	betrachterAngemeldet
	// betrachterWidget is an HA widget route (no auth, own port): everything,
	// without Mieteinnahmen.
	betrachterWidget
)

// sichtbareWohnung is one visible apartment: its Jahreskarte and Verlauf.
type sichtbareWohnung struct {
	Card    dashboardJahresCard
	Verlauf dashboardVerlaufSpalte
}

// sichtbarerSimple is the card and the Verlauf of Wallboxen or PV-Anlage.
type sichtbarerSimple struct {
	Card    dashboardSimpleCard
	Verlauf dashboardSimpleSpalte
}

// ansicht is the fully filtered view for one betrachter: what is not
// visible to them is not in it, so the rest of the data never reaches a
// template (server-side hiding in the template would leave it in the HTML).
type ansicht struct {
	// Cards and VerlaufSpalten hold the visible apartments in the same order.
	Cards          []dashboardJahresCard
	VerlaufSpalten []dashboardVerlaufSpalte
	// Wallbox and PV are nil for a betrachter who does not see them.
	Wallbox *sichtbarerSimple
	PV      *sichtbarerSimple
}

// sichtbareAnsicht decides the visibility for b and calls the builders (kept
// apart, ADR 0002) for what stays visible. mieteEingaben is read only for
// betrachterAngemeldet (Mieteinnahmen, #278); it is ignored for the others, so
// the caller may pass nil.
func sichtbareAnsicht(b betrachter, dd dashboardData, mieteEingaben []*store.FixkostenEingabeDetails) ansicht {
	apartments := dd.Apartments
	// Anonymous: only apartment 2 goes into the view at all (Ticket #112).
	if b == betrachterAnonym {
		for _, a := range apartments {
			if a.ID == 2 {
				apartments = []store.Apartment{a}
				break
			}
		}
	}

	var v ansicht
	for _, a := range apartments {
		card, verlauf := buildEntityView(dd, a.ID)
		// Mieteinnahmen (#278): only for a rented apartment and a logged-in
		// user, so the rent stays out of the view otherwise.
		if b == betrachterAngemeldet && store.NewWohnungsregeln(a, store.Haus{}).Mieteinnahmen() {
			summe := mieteinnahmen(a.ID, dd.Jahr, mieteEingaben)
			card.Mieteinnahmen = &summe
		}
		v.Cards = append(v.Cards, card)
		v.VerlaufSpalten = append(v.VerlaufSpalten, verlauf)
	}

	// Wallboxen/PV-Anlage (Ticket #67): whole-house entities, excluded for
	// the anonymous betrachter like apartment 1.
	if b != betrachterAnonym {
		v.Wallbox = buildSichtbarerSimple(wallboxSeries, dd)
		v.PV = buildSichtbarerSimple(pvSeries, dd)
	}
	return v
}

func buildSichtbarerSimple(s simpleSeries, dd dashboardData) *sichtbarerSimple {
	return &sichtbarerSimple{
		Card:    buildSimpleJahresCard(s, dd.Jahr, dd.PeriodenKosten),
		Verlauf: buildSimpleVerlauf(s, dd.PeriodenKosten),
	}
}

// Wohnung returns the visible apartment with the given id.
func (v ansicht) Wohnung(id int64) (sichtbareWohnung, bool) {
	for i, c := range v.Cards {
		if c.ApartmentID == id {
			return sichtbareWohnung{Card: c, Verlauf: v.VerlaufSpalten[i]}, true
		}
	}
	return sichtbareWohnung{}, false
}
