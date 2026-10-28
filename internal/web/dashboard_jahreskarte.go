package web

import (
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// anzeigeJahr is the Dashboard's auto-following display year (Issue #60
// Story 22) - the year of whichever is chronologically newest across both
// readings and fixed-costs entries, purely data-driven (not wall-clock
// time). Falls back to the real current year only when neither series has
// any data yet (fresh install).
func anzeigeJahr(allPeriods []store.PeriodSummary, fixkostenEingaben []store.FixkostenEingabeSummary) int {
	var jahr int
	if len(allPeriods) > 0 {
		if y, ok := store.Abrechnungsmonat(allPeriods[0].ReadingDate).Jahr(); ok {
			jahr = y
		}
	}
	if len(fixkostenEingaben) > 0 {
		if y, ok := store.Abrechnungsmonat(fixkostenEingaben[0].Monat).Jahr(); ok && y > jahr {
			jahr = y
		}
	}
	if jahr == 0 {
		jahr = time.Now().Year()
	}
	return jahr
}

// dashboardJahresCard is one apartment's yearly-totals card (Issue #60
// Story 21) - fixed costs first, then Strom/Heizung/Wasser (electricity/
// heating/water), same field order the mini-bar segments use. Also feeds
// the KPI strip below the apartment switcher (VerbrauchEUR/FixkostenEUR/
// GesamtEUR), so the numbers always match between the two.
type dashboardJahresCard struct {
	ApartmentID         int64
	ApartmentName       string
	ApartmentQM         float64
	ApartmentFlurstueck float64
	PersonenSchnitt     float64
	FixkostenEUR        float64
	StromEUR            float64
	WallboxEUR          float64
	WallboxKWh          float64 // yearly kWh of the Wallbox share (not a bar segment, see Wallbox)
	HeizungEUR          float64
	WasserEUR           float64
	VerbrauchEUR        float64
	// PVGutschriftEUR is the yearly PV-Gutschrift (#227), a positive amount
	// shown as a negative line, only ever set for apartment 1. It lowers
	// GesamtEUR but not VerbrauchEUR and is no bar segment.
	PVGutschriftEUR float64
	// HatPVGutschrift shows the PV-Gutschrift line (apartment 1), also at 0.
	HatPVGutschrift bool
	GesamtEUR       float64
	// Vorlaeufig is "vorläufig, ohne <Monate>" when the year holds a month
	// whose Ablesungen wait for their Fixkosten (#364), shown at Gesamt and
	// Saldo. "" = none.
	Vorlaeufig string
	Segmente   []dashboardSegment

	// WallboxVerteilt is true when at least one period of the year assigns
	// the Wallbox costs to the apartments (StromErgebnis.WallboxVerteilt).
	WallboxVerteilt bool
	// Wallbox is the apartment's own Wallbox block (#304): one bar
	// "Stromkosten" in EUR and kWh. nil = no block. Set by buildEntityView
	// from the Wohnungsregeln, the Wallbox no longer sits in Segmente.
	Wallbox *dashboardSimpleCard

	// PVAnteilKWh is the yearly total of StromErgebnis.PVAnteilW2KWh ("not
	// allocated to grid draw (PV)", GLOSSARY.md) - only set for apartment 2
	// (Issue #98), since apartment 1 has no own electricity meter. 0 =
	// row is hidden.
	PVAnteilKWh float64
	// HatPVAnteil shows the PV-Anteil line (apartment 2), also at 0.
	HatPVAnteil bool

	// Nebenkostenabschlag-Saldo (utility advance payment balance):
	// continuously accumulated since recording began (no yearly reset),
	// as of the latest month - see buildDashboardVerlauf, which computes
	// the balance per month; taken over from there (handleDashboard), not
	// computed here in buildJahresCard. nil = no balance calculable (see
	// AbschlagSaldo).
	Saldo *calc.AbschlagSaldo

	// Mieteinnahmen is the yearly sum of the Miete (#278), rounded to the
	// cent, shown below the Saldo. nil = no block: set only by
	// handleDashboard for a rented apartment and a logged-in user, so the
	// rent never reaches the template otherwise.
	Mieteinnahmen *float64
}

// mieteinnahmen sums the Miete of the apartment over every Miete projection
// of jahr (the Abrechnungsmonat counts), rounded to the cent. Pure display
// value, it enters no calculation.
func mieteinnahmen(apartmentID int64, jahr int, eingaben []mieteMonat) float64 {
	var summe float64
	for _, e := range eingaben {
		if y, ok := store.Abrechnungsmonat(e.Monat).Jahr(); ok && y == jahr {
			summe += e.Miete[apartmentID]
		}
	}
	return calc.Round2(summe)
}

// buildJahresCard sums the given apartment's consumption and fixed costs
// over every period/entry whose date falls in jahr, and (Ticket #75)
// averages the apartment's occupant count over jahr's Fixkosten-Eingaben
// (exactly one per Monat, so each month weighs the same; shown with up to 2
// decimal places, 0 if the year has no entry yet).
func buildJahresCard(apartmentID int64, apartmentName string, apartmentQM, apartmentFlurstueck float64, jahr int, periodenKosten []periodKosten, fixkostenListe []fixkostenKosten) dashboardJahresCard {
	var strom, wallbox, heizung, wasser, fix, gutschrift float64
	var wallboxKWh, pvAnteilKWh float64
	var wallboxVerteilt bool
	var personenSumme float64
	var personenAnzahl int
	for _, pk := range periodenKosten {
		y, ok := store.Abrechnungsmonat(pk.Monat).Jahr()
		if !ok || y != jahr {
			continue
		}
		for _, kat := range kategorien(apartmentID, pk.K) {
			switch kat.Kind {
			case kategorieKindStrom:
				strom += kat.Kosten
			case kategorieKindWallbox:
				wallbox += kat.Kosten
				wallboxKWh += kat.Verbrauch
			case kategorieKindHeizung:
				heizung += kat.Kosten
			case kategorieKindWasser:
				wasser += kat.Kosten
			}
		}
		anteil := pk.K.Anteil(apartmentID)
		gutschrift += anteil.PVGutschrift
		pvAnteilKWh += anteil.PVAnteilKWh
		if pk.K.Strom != nil && pk.K.Strom.WallboxVerteilt {
			wallboxVerteilt = true
		}
	}
	for _, fk := range fixkostenListe {
		y, ok := store.Abrechnungsmonat(fk.Monat).Jahr()
		if !ok || y != jahr {
			continue
		}
		fix += fk.Erg.KostenFor(apartmentID)
		if p, ok := fk.Personen[apartmentID]; ok {
			personenSumme += p
			personenAnzahl++
		}
	}
	var personenSchnitt float64
	if personenAnzahl > 0 {
		personenSchnitt = personenSumme / float64(personenAnzahl)
	}
	strom, wallbox, heizung, wasser, fix, gutschrift = calc.Round2(strom), calc.Round2(wallbox), calc.Round2(heizung), calc.Round2(wasser), calc.Round2(fix), calc.Round2(gutschrift)
	verbrauch := calc.Round2(strom + wallbox + heizung + wasser)
	brutto := calc.Round2(verbrauch + fix)
	gesamt := calc.Round2(brutto - gutschrift)

	segs := []dashboardSegment{
		{Farbe: "fix", Label: "Fixkosten", Kosten: fix},
		{Farbe: "strom", Label: "Strom", Kosten: strom},
		{Farbe: "heizung", Label: "Heizung/Ww", Kosten: heizung},
		{Farbe: "wasser", Label: "Wasser", Kosten: wasser},
	}
	// The bar shows the consumption categories only: the Fixkosten grow too
	// fast and crowd out the consumption (they stay a chip and in the total).
	// The Wallbox has its own block (#304), so it is no segment either. The
	// segments add up to 100% of what the bar shows, the Fixkosten segment
	// (first) gets no width.
	setSegmentPct(segs[1:], calc.Round2(strom+heizung+wasser))

	// Which lines the card carries comes from the Wohnungsanteil, not from the
	// data: an empty Kosten tells it without a period.
	zeilen := calc.Kosten{Strom: &calc.StromErgebnis{}, Wasser: &calc.WasserErgebnis{}, Heizung: &calc.HeizungErgebnis{}}.Anteil(apartmentID)

	return dashboardJahresCard{
		HatPVGutschrift: zeilen.HatPVGutschrift, HatPVAnteil: zeilen.HatPVAnteil,
		ApartmentID: apartmentID, ApartmentName: apartmentName, ApartmentQM: apartmentQM,
		ApartmentFlurstueck: apartmentFlurstueck, PersonenSchnitt: personenSchnitt,
		FixkostenEUR: fix, StromEUR: strom, WallboxEUR: wallbox, HeizungEUR: heizung, WasserEUR: wasser,
		VerbrauchEUR: verbrauch, PVGutschriftEUR: gutschrift, GesamtEUR: gesamt, Segmente: segs,
		PVAnteilKWh: pvAnteilKWh,
		WallboxKWh:  wallboxKWh, WallboxVerteilt: wallboxVerteilt,
	}
}
