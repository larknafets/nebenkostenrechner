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
	verlauf := buildDashboardVerlauf(a.ID, a.Name, dd.PeriodenKosten, dd.FixkostenListe, offen, dd.Sonderkosten[a.ID])
	// The yearly card runs up to the chosen month (dd.Bis), the Verlauf keeps
	// every month.
	grenze := dd.bisGrenze()
	bisPK := bisMonat(dd.PeriodenKosten, grenze, pkMonat)
	bisFK := bisMonat(dd.FixkostenListe, grenze, fkMonat)
	bisSK := bisMonat(dd.Sonderkosten[a.ID], grenze, skMonat)
	card := buildJahresCard(a.ID, a.Name, a.QM, a.FlurstueckGroesse, dd.Jahr, bisPK, bisFK, bisSK)
	// The Wallbox block (#304) shows only when the Wohnung carries the
	// Wallbox in its Abrechnung and the costs are assigned to the apartments.
	// Totals and Saldo stay untouched (the Wallbox is in them either way).
	regeln := store.NewWohnungsregeln(a, store.Haus{WallboxWeiterberechnen: dd.WallboxWeiterberechnen})
	if regeln.AbrechnungWallbox() && card.WallboxVerteilt {
		card.Wallbox = wallboxBlock(card)
	}
	// The Saldo is cumulative, so up to the chosen month it comes from a Verlauf
	// of the months up to then.
	card.Saldo = verlauf.LatestSaldo
	if grenze != "" {
		bisOffen := bisMonat(offen, grenze, offenMonat)
		card.Saldo = buildDashboardVerlauf(a.ID, a.Name, bisPK, bisFK, bisOffen, bisSK).LatestSaldo
		offen = bisOffen
	}
	card.Vorlaeufig = vorlaeufigText(offen, dd.Jahr)
	return card, verlauf
}
