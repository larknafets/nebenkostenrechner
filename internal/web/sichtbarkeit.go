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

// entitaetArt is the kind of one entity of the Dashboard: an apartment or
// one of the whole-house series.
type entitaetArt int

const (
	// entitaetWohnung is an apartment, identified by entitaet.ApartmentID.
	entitaetWohnung entitaetArt = iota
	entitaetWallbox
	entitaetPV
)

// entitaet names one of the 4 entities (Wohnung 1/2, Wallboxen, PV-Anlage).
type entitaet struct {
	Art         entitaetArt
	ApartmentID int64 // only for entitaetWohnung
}

func wohnungEntitaet(id int64) entitaet {
	return entitaet{Art: entitaetWohnung, ApartmentID: id}
}

// sichtbar is the visibility table (#293): whether b may see e at all. It is
// the one place that table lives; both entry points below ask it.
func (b betrachter) sichtbar(e entitaet) bool {
	if b != betrachterAnonym {
		return true
	}
	// Anonymous: only apartment 2 (Ticket #112); Wallboxen/PV-Anlage are
	// whole-house entities, excluded like apartment 1 (Ticket #67).
	return e.Art == entitaetWohnung && e.ApartmentID == 2
}

// sichtbareAnsicht decides the visibility for b and calls the builders (kept
// apart, ADR 0002) for what stays visible. miete is read only for
// betrachterAngemeldet (Mieteinnahmen, #278); it is ignored for the others, so
// the caller may pass nil.
func sichtbareAnsicht(b betrachter, dd dashboardData, miete []mieteMonat) ansicht {
	var v ansicht
	for _, a := range dd.Apartments {
		if b.sichtbar(wohnungEntitaet(a.ID)) {
			v.addWohnung(b, dd, a, miete)
		}
	}
	if b.sichtbar(entitaet{Art: entitaetWallbox}) {
		v.Wallbox = buildSichtbarerSimple(wallboxSeries, dd)
	}
	if b.sichtbar(entitaet{Art: entitaetPV}) {
		v.PV = buildSichtbarerSimple(pvSeries, dd)
	}
	return v
}

// sichtbareEntitaet is sichtbareAnsicht for a single entity: same visibility
// table and same output for that entity, but only that entity is built. The
// view stays empty if b may not see it or an apartment id is not in the data.
func sichtbareEntitaet(b betrachter, dd dashboardData, e entitaet, miete []mieteMonat) ansicht {
	var v ansicht
	if !b.sichtbar(e) {
		return v
	}
	switch e.Art {
	case entitaetWohnung:
		for _, a := range dd.Apartments {
			if a.ID == e.ApartmentID {
				v.addWohnung(b, dd, a, miete)
				break
			}
		}
	case entitaetWallbox:
		v.Wallbox = buildSichtbarerSimple(wallboxSeries, dd)
	case entitaetPV:
		v.PV = buildSichtbarerSimple(pvSeries, dd)
	}
	return v
}

// addWohnung builds card and Verlauf of one visible apartment.
func (v *ansicht) addWohnung(b betrachter, dd dashboardData, a store.Apartment, miete []mieteMonat) {
	card, verlauf := buildEntityView(dd, a.ID)
	// Mieteinnahmen (#278): only for a rented apartment and a logged-in
	// user, so the rent stays out of the view otherwise.
	if b == betrachterAngemeldet && store.NewWohnungsregeln(a, store.Haus{}).Mieteinnahmen() {
		summe := mieteinnahmen(a.ID, dd.Jahr, miete)
		card.Mieteinnahmen = &summe
	}
	v.Cards = append(v.Cards, card)
	v.VerlaufSpalten = append(v.VerlaufSpalten, verlauf)
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
