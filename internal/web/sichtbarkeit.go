package web

import "github.com/larknafets/nebenkostenrechner/internal/store"

// betrachter says who looks at the Dashboard. It is the only input that
// decides what is visible (#293).
type betrachter int

const (
	// betrachterAnonym is a visitor who is not logged in (LOGIN_PASSWORD set):
	// only Wohnung 2, no Wallboxen/PV-Anlage, no Mieteinnahmen.
	betrachterAnonym betrachter = iota
	// betrachterAngemeldet is a logged-in user: everything, with Mieteinnahmen.
	betrachterAngemeldet
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
	// Wallbox, WP and PV are nil for a betrachter who does not see them.
	Wallbox *sichtbarerSimple
	WP      *sichtbarerSimple
	PV      *sichtbarerSimple
}

// entitaetArt is the kind of one entity of the Dashboard: an apartment or
// one of the whole-house series.
type entitaetArt int

const (
	// entitaetWohnung is an apartment, identified by entitaet.ApartmentID.
	entitaetWohnung entitaetArt = iota
	entitaetWallbox
	entitaetWP
	entitaetPV
)

// entitaet names one of the 5 entities (Wohnung 1/2, Wallboxen, Wärmepumpe, PV-Anlage).
type entitaet struct {
	Art         entitaetArt
	ApartmentID int64 // only for entitaetWohnung
}

func wohnungEntitaet(id int64) entitaet {
	return entitaet{Art: entitaetWohnung, ApartmentID: id}
}

// sichtbar is the visibility table (#293): whether b may see e at all. It is
// the one place that table lives; both entry points below ask it. dd decides
// whether the Wallboxen card exists for a logged-in user (#297).
func (b betrachter) sichtbar(e entitaet, dd dashboardData) bool {
	switch e.Art {
	case entitaetWP:
		// The heat pump card has no condition, not even a login (#296).
		return true
	case entitaetWallbox:
		// Anonymous: no whole-house Wallboxen (Ticket #67). Logged in: only
		// when a Wohnung carries the Wallbox in its Abrechnung (#297).
		return b == betrachterAngemeldet && dd.wallboxInAbrechnung()
	}
	if b != betrachterAnonym {
		return true
	}
	// Anonymous: only apartment 2 (Ticket #112); PV-Anlage is a whole-house
	// entity, excluded like apartment 1 (Ticket #67).
	return e.Art == entitaetWohnung && e.ApartmentID == 2
}

// wallboxInAbrechnung reports whether at least one Wohnung carries the
// Wallbox in its Jahresabrechnung (Wohnungsregeln.AbrechnungWallbox).
func (dd dashboardData) wallboxInAbrechnung() bool {
	haus := store.Haus{WallboxWeiterberechnen: dd.WallboxWeiterberechnen}
	for _, a := range dd.Apartments {
		if store.NewWohnungsregeln(a, haus).AbrechnungWallbox() {
			return true
		}
	}
	return false
}

// sichtbareAnsicht decides the visibility for b and calls the builders (kept
// apart, ADR 0002) for what stays visible. miete is read only for
// betrachterAngemeldet (Mieteinnahmen, #278); it is ignored for the others, so
// the caller may pass nil.
func sichtbareAnsicht(b betrachter, dd dashboardData, miete []mieteMonat) ansicht {
	var v ansicht
	for _, a := range dd.Apartments {
		if b.sichtbar(wohnungEntitaet(a.ID), dd) {
			v.addWohnung(b, dd, a, miete)
		}
	}
	if b.sichtbar(entitaet{Art: entitaetWallbox}, dd) {
		v.Wallbox = buildSichtbarerSimple(wallboxSeries, dd)
	}
	if b.sichtbar(entitaet{Art: entitaetWP}, dd) {
		v.WP = buildSichtbarerSimple(wpSeries, dd)
	}
	if b.sichtbar(entitaet{Art: entitaetPV}, dd) {
		v.PV = buildSichtbarerSimple(pvSeries, dd)
	}
	return v
}

// addWohnung builds card and Verlauf of one visible apartment.
func (v *ansicht) addWohnung(b betrachter, dd dashboardData, a store.Apartment, miete []mieteMonat) {
	card, verlauf := buildEntityView(dd, a.ID)
	// Mieteinnahmen (#278): only for a rented apartment and a logged-in
	// user, so the rent stays out of the view otherwise.
	if b == betrachterAngemeldet && store.NewWohnungsregelnOhneHaus(a).Mieteinnahmen() {
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
