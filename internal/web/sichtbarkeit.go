package web

import "github.com/larknafets/nebenkostenrechner/internal/store"

// betrachter says who looks at the Dashboard: the role of the visitor. It is
// the only input, besides the saved Berechtigungen, that decides what is
// visible (#293).
type betrachter int

const (
	// betrachterNichtAngemeldet is a visitor who is not logged in
	// (VERMIETER_PASSWORD set): what the Berechtigungen grant to Nicht
	// angemeldet.
	betrachterNichtAngemeldet betrachter = iota
	// betrachterMieter is a logged-in Mieter: what the Berechtigungen grant to
	// the Mieter.
	betrachterMieter
	// betrachterVermieter is the Vermieter (or the demo session): everything,
	// with Miete.
	betrachterVermieter
)

// betrachterVon maps the Zugriff of a request to the role whose view applies
// (until the Dashboard asks the Zugriff directly).
func betrachterVon(z Zugriff) betrachter {
	switch {
	case z.Vermieter:
		return betrachterVermieter
	case z.Rolle == store.RolleMieter:
		return betrachterMieter
	}
	return betrachterNichtAngemeldet
}

// rolle returns the role whose Berechtigungen apply to b; the Vermieter has
// none to look up, he has every right.
func (b betrachter) rolle() store.Rolle {
	if b == betrachterMieter {
		return store.RolleMieter
	}
	return store.RolleNichtAngemeldet
}

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
		switch e.ApartmentID {
		case 1:
			return store.RechtWohnung1, true
		case 2:
			return store.RechtWohnung2, true
		}
	}
	return "", false
}

// orDefaults returns b, or the defaults when b is nil (a database that never
// saved any, tests).
func orDefaults(b store.Berechtigungen) store.Berechtigungen {
	if b == nil {
		return store.DefaultBerechtigungen()
	}
	return b
}

// sichtbar is the visibility table (#293, #311): whether b may see e at all.
// It is the one place that table lives; both entry points below ask it. The
// Vermieter sees everything; the others see what the saved Berechtigungen
// (ber, nil for the defaults) grant. The Wallboxen card exists for nobody
// unless a Wohnung carries the Wallbox in its Abrechnung (#297).
func (b betrachter) sichtbar(e entitaet, dd dashboardData, ber store.Berechtigungen) bool {
	if e.Art == entitaetWallbox && !dd.wallboxInAbrechnung() {
		return false
	}
	if b == betrachterVermieter {
		return true
	}
	recht, ok := e.rechtFuer()
	return ok && orDefaults(ber).Erlaubt(b.rolle(), recht)
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
// apart, ADR 0002) for what stays visible. miete is read only for a betrachter
// who may see the Miete (#278); it is ignored for the others, so the caller may
// pass nil.
func sichtbareAnsicht(b betrachter, dd dashboardData, ber store.Berechtigungen, miete []mieteMonat) ansicht {
	var v ansicht
	for _, a := range dd.Apartments {
		if b.sichtbar(wohnungEntitaet(a.ID), dd, ber) {
			v.addWohnung(b, dd, ber, a, miete)
		}
	}
	if b.sichtbar(entitaet{Art: entitaetWallbox}, dd, ber) {
		v.Wallbox = buildSichtbarerSimple(wallboxSeries, dd)
	}
	if b.sichtbar(entitaet{Art: entitaetWP}, dd, ber) {
		v.WP = buildSichtbarerSimple(wpSeries, dd)
	}
	if b.sichtbar(entitaet{Art: entitaetPV}, dd, ber) {
		v.PV = buildSichtbarerSimple(pvSeries, dd)
	}
	return v
}

// addWohnung builds card and Verlauf of one visible apartment.
func (v *ansicht) addWohnung(b betrachter, dd dashboardData, ber store.Berechtigungen, a store.Apartment, miete []mieteMonat) {
	card, verlauf := buildEntityView(dd, a.ID)
	// Miete (#278, #311): only for a rented apartment and a betrachter who has
	// the right, so the rent stays out of the view otherwise.
	if store.NewWohnungsregelnOhneHaus(a).Mieteinnahmen() && (b == betrachterVermieter || orDefaults(ber).Erlaubt(b.rolle(), store.RechtMiete)) {
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
