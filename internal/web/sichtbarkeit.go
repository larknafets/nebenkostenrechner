package web

import (
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
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

// ansicht is the fully filtered view for one Zugriff: what is not
// visible to them is not in it, so the rest of the data never reaches a
// template (server-side hiding in the template would leave it in the HTML).
type ansicht struct {
	// Cards and VerlaufSpalten hold the visible apartments in the same order.
	Cards          []dashboardJahresCard
	VerlaufSpalten []dashboardVerlaufSpalte
	// Haus lists the visible whole-house entries in the order of the cards
	// and the dropdown (after the apartments): Wärmepumpe, Wallboxen, PV-Anlage.
	// What a role does not see is not in it. The order is set only in
	// sichtbareAnsicht.
	Haus []sichtbarerSimple
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

// rechtFuer is the right that lets a Mieter or a visitor who is not logged in
// see e; ok is false for an entity no right exists for.
func (e entitaet) rechtFuer() (recht store.Recht, ok bool) {
	switch e.Art {
	case entitaetWP:
		return store.RechtWaermepumpe, true
	case entitaetWallbox:
		return store.RechtWallboxen, true
	case entitaetPV:
		return store.RechtPVAnlage, true
	case entitaetWohnung:
		return store.WohnungRecht(e.ApartmentID)
	}
	return "", false
}

// sichtbar is the visibility table (#293, #311): whether z may see e at all.
// Who sees what is the Zugriff's business (Zugriff.Sichtbar); what exists is
// the Dashboard's: the Wallboxen card exists for nobody unless a Wohnung
// carries the Wallbox in its Abrechnung (#297).
func (dd dashboardData) sichtbar(z Zugriff, e entitaet) bool {
	if e.Art == entitaetWallbox && !dd.wallboxInAbrechnung() {
		return false
	}
	return z.Sichtbar(e)
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

// sichtbareAnsicht decides the visibility for z and calls the builders (kept
// apart, ADR 0002) for what stays visible. miete is read only when z may see
// the Miete (#278); it is ignored otherwise, so the caller may pass nil.
func sichtbareAnsicht(z Zugriff, dd dashboardData, miete []mieteMonat, offen []calc.OffenerMonat) ansicht {
	var v ansicht
	for _, a := range dd.Apartments {
		if dd.sichtbar(z, wohnungEntitaet(a.ID)) {
			v.addWohnung(z, dd, a, miete, offen)
		}
	}
	// The one place that fixes the order of the whole-house cards and dropdown
	// entries.
	for _, h := range []struct {
		art    entitaetArt
		series simpleSeries
	}{{entitaetWP, wpSeries}, {entitaetWallbox, wallboxSeries}, {entitaetPV, pvSeries}} {
		if dd.sichtbar(z, entitaet{Art: h.art}) {
			v.Haus = append(v.Haus, buildSichtbarerSimple(h.series, dd))
		}
	}
	return v
}

// addWohnung builds card and Verlauf of one visible apartment.
func (v *ansicht) addWohnung(z Zugriff, dd dashboardData, a store.Apartment, miete []mieteMonat, offen []calc.OffenerMonat) {
	card, verlauf := buildEntityView(dd, offen, a.ID)
	// Miete (#278, #311): only for a rented apartment and a role that has the
	// right, so the rent stays out of the view otherwise.
	if store.NewWohnungsregelnOhneHaus(a).Mieteinnahmen() && z.Darf(store.RechtMiete) {
		summe := mieteinnahmen(a.ID, dd.Jahr, miete)
		card.Mieteinnahmen = &summe
	}
	v.Cards = append(v.Cards, card)
	v.VerlaufSpalten = append(v.VerlaufSpalten, verlauf)
}

func buildSichtbarerSimple(s simpleSeries, dd dashboardData) sichtbarerSimple {
	return sichtbarerSimple{
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
