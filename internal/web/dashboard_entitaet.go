package web

import (
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// findApartment returns the apartment with the given id, or the zero value
// if absent (only reachable if the DB's fixed 2-apartment seed data was
// somehow removed).
func findApartment(apartments []store.Apartment, id int64) store.Apartment {
	for _, a := range apartments {
		if a.ID == id {
			return a
		}
	}
	return store.Apartment{}
}

// wallboxBlock builds the apartment's Wallbox block from its yearly totals:
// one full-width "Stromkosten" segment, as in the Wallboxen card but without
// a PV share (nothing allocates the PV share to an apartment).
func wallboxBlock(card dashboardJahresCard) *dashboardSimpleCard {
	seg := dashboardSegment{Farbe: "strom", Label: "Stromkosten", Kosten: card.WallboxEUR, Verbrauch: card.WallboxKWh, Einheit: "kWh"}
	if card.WallboxKWh > 0 {
		seg.ProzentNeuestesGesamt = 100
	}
	return &dashboardSimpleCard{Name: "Wallbox", GesamtEUR: card.WallboxEUR, Segmente: []dashboardSegment{seg}}
}

// buildEntityView bundles what every apartmentID-based Dashboard
// view needs in one place anyway: the yearly-totals card, the monthly
// history, and the balance wired consistently between both (#99,
// candidate 3). Always builds the full history, even when a
// caller only needs the card (#101: no card-only mode, avoiding
// speculative generality). Only intended for the 2 apartments - Wallboxen/
// PV-Anlage have no balance concept and run via buildSimpleJahresCard/
// buildSimpleVerlauf.
func buildEntityView(dd dashboardData, offen []calc.OffenerMonat, apartmentID int64) (dashboardJahresCard, dashboardVerlaufSpalte) {
	a := findApartment(dd.Apartments, apartmentID)
	verlauf := buildDashboardVerlauf(a.ID, a.Name, dd.PeriodenKosten, dd.FixkostenListe, offen)
	card := buildJahresCard(a.ID, a.Name, a.QM, a.FlurstueckGroesse, dd.Jahr, dd.PeriodenKosten, dd.FixkostenListe)
	// The Wallbox block (#304) shows only when the Wohnung carries the
	// Wallbox in its Abrechnung and the costs are assigned to the apartments.
	// Totals and Saldo stay untouched (the Wallbox is in them either way).
	regeln := store.NewWohnungsregeln(a, store.Haus{WallboxWeiterberechnen: dd.WallboxWeiterberechnen})
	if regeln.AbrechnungWallbox() && card.WallboxVerteilt {
		card.Wallbox = wallboxBlock(card)
	}
	card.Saldo = verlauf.LatestSaldo
	card.Vorlaeufig = vorlaeufigText(offen, dd.Jahr)
	return card, verlauf
}
