package web

import (
	"sort"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboardMonat is one calendar month's combined consumption+fixed-costs
// Monatsverlauf row - up to 3 independently pct-scaled bar variants (one
// per numeric mode); the 4th mode ("Verbrauchswerte"/consumption values)
// reuses VerbrauchSegmente's raw Verbrauch/Einheit as text instead of a
// bar.
type dashboardMonat struct {
	Label     string
	Jahr      int
	IsCurrent bool

	// FixkostenFehlen: the month has an Ablesung but no Fixkosten-Eingabe (#364).
	// It gets its own row without bars and amounts.
	FixkostenFehlen bool
	// VerbrauchOffen: the Eingabe of the month lacks a price, so the Ablesung is
	// not calculable. The row shows the Fixkosten and "offen" for the Verbrauch.
	VerbrauchOffen bool

	HasVerbrauch      bool
	VerbrauchSegmente []dashboardSegment
	VerbrauchGesamt   float64

	HasFixkosten      bool
	FixkostenSegmente []dashboardSegment
	FixkostenGesamt   float64

	HasKombiniert      bool
	KombiniertSegmente []dashboardSegment
	// KombiniertGesamt is display only and net of PVGutschrift (#227):
	// Verbrauch + Fixkosten minus the credit. The Saldo is not derived from
	// it, but from the parts (VerbrauchGesamt, FixkostenGesamt, PVGutschrift). The credit is a negative line next to the bar, not a
	// segment (a bar cannot have a negative width).
	KombiniertGesamt float64
	PVGutschrift     float64

	// Nebenkostenabschlag-Saldo (utility advance payment balance, 5th mode
	// "abschlag"): continuously accumulated balance up to and including
	// this month - abschlag(m) + PVGutschrift(m) - costs(m) summed per month
	// since the very first recorded month, no yearly reset. nil if the
	// month is not HasKombiniert (missing months leave the balance
	// unchanged, see buildDashboardVerlauf).
	Saldo *calc.AbschlagSaldo

	// AbschlagProzent (0-50) is the bar's half-width - share of the
	// largest |Saldo| across the whole series, so a property of the
	// series, not of the individual balance, hence no field on
	// AbschlagSaldo itself.
	AbschlagProzent float64
}

// dashboardJahreszeile is the Monatsverlauf's per-year summary row (Issue
// #60 Story 27) - unlike the old readings-only Verlauf's December-
// triggered separator, every year gets one, including the not-yet-
// complete newest one (IstLaufend), which only sums whatever months are
// recorded so far.
type dashboardJahreszeile struct {
	Jahr           int
	IstLaufend     bool
	VerbrauchSumme float64
	FixkostenSumme float64
	// PVGutschriftSumme is the year's credit (#227), GesamtSumme is net of it.
	PVGutschriftSumme float64
	GesamtSumme       float64
	// GesamtBruttoSumme is Verbrauch plus Fixkosten without the credit, for
	// the modes that do not show the PV-Gutschrift.
	GesamtBruttoSumme float64

	// Endstand (closing balance): the accumulated balance at year end (or
	// at the latest recorded month, for a running year) - not a sum,
	// since the balance is continuous and can't meaningfully be added up
	// per year (see #92/#94). Own field name instead of "Saldo" like on
	// dashboardMonat, because this refers to a different point in time
	// (year end, not the current month).
	Endstand *calc.AbschlagSaldo
}

// dashboardVerlaufEintrag is one row of a Monatsverlauf column: either a
// month (Monat) or (if Jahreszeile is set) a year-summary row. Exactly one
// is set.
type dashboardVerlaufEintrag struct {
	Monat       *dashboardMonat
	Jahreszeile *dashboardJahreszeile
}

// dashboardVerlaufSpalte is one apartment's Monatsverlauf, newest first.
type dashboardVerlaufSpalte struct {
	ApartmentID   int64
	ApartmentName string
	Eintraege     []dashboardVerlaufEintrag

	// Base and MitLink are set by the handler for the rows of open months: the
	// link to the Fixkosten form is only for the Vermieter:innen.
	Base    string
	MitLink bool

	// LatestSaldo is the newest month with a balance (newest-first, skips
	// year-summary rows and gapped months) - set directly from the
	// backward loop in buildDashboardVerlauf, instead of a 2nd function
	// (latestAbschlagSaldo) walking through Eintraege afterwards to find
	// the same value again (#99/#102: duplicated code). nil = no balance
	// calculable.
	LatestSaldo *calc.AbschlagSaldo
}

// groupKostenByMonat merges every periodKosten's kategorien(apartmentID, ...)
// row by billing month (Abrechnungsmonat, Issue #86): 2+ readings sharing
// a month (sub-monthly readings) get their rows summed (Kosten, Verbrauch,
// Verbrauch2), instead of one silently overwriting another - replaces
// buildDashboardVerlauf's old per-ReadingDate "first one wins" bucket,
// which only ever saw 1 period per month in practice.
//
// Merging by position (not by Label) is safe here because kategorien's
// result shape - which Kategorien it returns, in which order - depends
// only on apartmentID, which is fixed for one groupKostenByMonat call
// (code review: matches buildSimpleVerlauf's equivalent per-Monat merge,
// rather than duplicating the same idea as a second, Label-keyed one).
func groupKostenByMonat(apartmentID int64, periodenKosten []periodKosten) map[string][]kategorie {
	out := map[string][]kategorie{}

	for _, pk := range periodenKosten {
		kats := kategorien(apartmentID, pk.K)
		existing, ok := out[pk.Monat]
		if !ok {
			out[pk.Monat] = kats
			continue
		}
		for i := range existing {
			existing[i].Kosten += kats[i].Kosten
			existing[i].Verbrauch += kats[i].Verbrauch
			existing[i].Verbrauch2 += kats[i].Verbrauch2
		}
	}
	return out
}

// buildDashboardVerlauf merges the given apartment's consumption (from
// periodenKosten) and fixed costs (from fixkostenListe) into one calendar-
// month Monatsverlauf. The 2 input series aren't forced 1:1 - a month with
// only a reading, only a fixed-costs entry, or both, is equally valid;
// when a month has 2+ periods/entries, the newest one wins (both inputs
// are already newest-first). A month in offen (#364) gets a row of its own
// even without data: "Fixkosten fehlen" if it has no Eingabe, "offen" for the
// Verbrauch if it has an incomplete one.
func buildDashboardVerlauf(apartmentID int64, apartmentName string, periodenKosten []periodKosten, fixkostenListe []fixkostenKosten, offen []calc.OffenerMonat) dashboardVerlaufSpalte {
	type bucket struct {
		label         string
		jahr          int
		verbrauchKats []kategorie
		fixErg        *calc.FixkostenErgebnis
		offen         bool
	}
	buckets := map[string]*bucket{}
	var order []string

	ensure := func(monat string) *bucket {
		jahr, ok := store.Abrechnungsmonat(monat).Jahr()
		if !ok {
			return nil
		}
		b, exists := buckets[monat]
		if !exists {
			b = &bucket{label: germanPeriodLabelShort(monat), jahr: jahr}
			buckets[monat] = b
			order = append(order, monat)
		}
		return b
	}

	for monat, kats := range groupKostenByMonat(apartmentID, periodenKosten) {
		if b := ensure(monat); b != nil {
			b.verbrauchKats = kats
		}
	}
	for _, fk := range fixkostenListe {
		if b := ensure(fk.Monat); b != nil && b.fixErg == nil {
			b.fixErg = fk.Erg
		}
	}
	for _, o := range offen {
		if b := ensure(o.Monat); b != nil {
			b.offen = true
		}
	}

	// What counts in the balance (Abschlag, PV-Gutschrift, the sums) is
	// decided by calc.Monatswerte, the same fold the Jahresabrechnung reads.
	// The Dashboard counts every item. A missing Abschlag value counts as 0
	// (#92).
	kosten := make([]calc.MonatsKosten, len(periodenKosten))
	for i, pk := range periodenKosten {
		kosten[i] = calc.MonatsKosten{Monat: pk.Monat, Kosten: pk.K}
	}
	fixkosten := make([]calc.MonatsFixkosten, len(fixkostenListe))
	for i, fk := range fixkostenListe {
		fixkosten[i] = calc.MonatsFixkosten{Monat: fk.Monat, Ergebnis: fk.Erg, Abschlag: fk.Abschlag}
	}
	werte := calc.Monatswerte(apartmentID, kosten, fixkosten, calc.Posten{Strom: true, Wallbox: true, PVGutschrift: true, FixkostenJeLogik: true})
	gutschrift := make(map[string]float64, len(werte))
	for _, w := range werte {
		gutschrift[w.Monat] = w.Gutschrift
	}

	sort.Sort(sort.Reverse(sort.StringSlice(order)))

	monate := make([]dashboardMonat, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		dm := dashboardMonat{Label: b.label, Jahr: b.jahr}
		dm.FixkostenFehlen = b.offen && b.fixErg == nil
		dm.VerbrauchOffen = b.offen && b.fixErg != nil

		if b.verbrauchKats != nil {
			dm.HasVerbrauch = true
			for _, kat := range b.verbrauchKats {
				dm.VerbrauchGesamt += kat.Kosten
				dm.VerbrauchSegmente = append(dm.VerbrauchSegmente, dashboardSegment{
					Farbe: kat.Farbe, Label: kat.Label, Kosten: kat.Kosten, Verbrauch: kat.Verbrauch, Einheit: kat.Einheit,
					Verbrauch2: kat.Verbrauch2, Einheit2: kat.Einheit2,
				})
			}
			dm.VerbrauchGesamt = calc.Round2(dm.VerbrauchGesamt)
		}

		if b.fixErg != nil {
			dm.HasFixkosten = true
			dm.FixkostenSegmente = fixkostenGruppen(apartmentID, b.fixErg)
			for _, seg := range dm.FixkostenSegmente {
				dm.FixkostenGesamt += seg.Kosten
			}
			dm.FixkostenGesamt = calc.Round2(dm.FixkostenGesamt)
		}

		if dm.HasVerbrauch || dm.HasFixkosten {
			dm.HasKombiniert = true
			dm.PVGutschrift = calc.Round2(gutschrift[key])
			if dm.HasVerbrauch {
				dm.KombiniertSegmente = append(dm.KombiniertSegmente, dashboardSegment{Farbe: "verbrauch-gesamt", Label: "Verbrauch", Kosten: dm.VerbrauchGesamt})
			}
			if dm.HasFixkosten {
				dm.KombiniertSegmente = append(dm.KombiniertSegmente, dashboardSegment{Farbe: "fix", Label: "Fixkosten", Kosten: dm.FixkostenGesamt})
			}
			dm.KombiniertGesamt = calc.Round2(dm.VerbrauchGesamt + dm.FixkostenGesamt - dm.PVGutschrift)
		}

		monate = append(monate, dm)
	}
	if len(monate) > 0 {
		monate[0].IsCurrent = true
	}

	// Baseline per mode = the largest month across the entire displayed
	// time range - no bar runs past 100% because of this (otherwise cut
	// off hard at the bar-track edge, see architecture review).
	var maxVerbrauch, maxFixkosten, maxKombiniert float64
	for _, m := range monate {
		if m.HasVerbrauch && m.VerbrauchGesamt > maxVerbrauch {
			maxVerbrauch = m.VerbrauchGesamt
		}
		if m.HasFixkosten && m.FixkostenGesamt > maxFixkosten {
			maxFixkosten = m.FixkostenGesamt
		}
		// Scale by the costs before the credit, so the segments fit the track.
		if brutto := m.VerbrauchGesamt + m.FixkostenGesamt; m.HasKombiniert && brutto > maxKombiniert {
			maxKombiniert = brutto
		}
	}
	for i := range monate {
		setSegmentPct(monate[i].VerbrauchSegmente, maxVerbrauch)
		setSegmentPct(monate[i].FixkostenSegmente, maxFixkosten)
		setSegmentPct(monate[i].KombiniertSegmente, maxKombiniert)
	}

	applySaldo(monate, order, werte)

	var latestSaldo *calc.AbschlagSaldo
	for i := range monate {
		if monate[i].Saldo != nil {
			latestSaldo = monate[i].Saldo
			break
		}
	}

	return dashboardVerlaufSpalte{
		ApartmentID: apartmentID, ApartmentName: apartmentName,
		Eintraege:   mitJahreszeilen(monate),
		LatestSaldo: latestSaldo,
	}
}

// jahresGruppe is a contiguous, newest-first run of one calendar year
// within a newest-first sorted list.
type jahresGruppe[T any] struct {
	Jahr       int
	IstLaufend bool
	Items      []T
}

// gruppiereNachJahr splits a newest-first list into consecutive yearly
// runs (#103, replaces walkJahre) - the run containing items[0] is
// IstLaufend. A pure grouping function with no callback-timing contract:
// "also flush at the end" follows automatically from every group
// (including the last) ending up in the result slice.
func gruppiereNachJahr[T any](items []T, jahrVon func(T) int) []jahresGruppe[T] {
	if len(items) == 0 {
		return nil
	}
	neuestesJahr := jahrVon(items[0])
	var out []jahresGruppe[T]
	for _, item := range items {
		jahr := jahrVon(item)
		if len(out) == 0 || out[len(out)-1].Jahr != jahr {
			out = append(out, jahresGruppe[T]{Jahr: jahr, IstLaufend: jahr == neuestesJahr})
		}
		g := &out[len(out)-1]
		g.Items = append(g.Items, item)
	}
	return out
}

// mitJahreszeilen inserts a year-summary row after each calendar-year run
// (Issue #60 Story 27) - every year, including the not-yet-complete
// newest one (IstLaufend), gets exactly one summary row.
func mitJahreszeilen(monate []dashboardMonat) []dashboardVerlaufEintrag {
	if len(monate) == 0 {
		return nil
	}
	out := make([]dashboardVerlaufEintrag, 0, len(monate)+4)
	for _, g := range gruppiereNachJahr(monate, func(m dashboardMonat) int { return m.Jahr }) {
		var vSumme, fSumme, gSumme float64
		var endstand *calc.AbschlagSaldo
		for _, m := range g.Items {
			vSumme += m.VerbrauchGesamt
			fSumme += m.FixkostenGesamt
			gSumme += m.PVGutschrift
			// Endstand = the balance of this year's newest month that has
			// a balance - since g.Items is newest-first, that's the
			// first match.
			if endstand == nil && m.Saldo != nil {
				endstand = m.Saldo
			}
			out = append(out, dashboardVerlaufEintrag{Monat: &m})
		}
		out = append(out, dashboardVerlaufEintrag{Jahreszeile: &dashboardJahreszeile{
			Jahr: g.Jahr, IstLaufend: g.IstLaufend,
			VerbrauchSumme: calc.Round2(vSumme), FixkostenSumme: calc.Round2(fSumme),
			PVGutschriftSumme: calc.Round2(gSumme), GesamtSumme: calc.Round2(vSumme + fSumme - gSumme),
			GesamtBruttoSumme: calc.Round2(vSumme + fSumme),
			Endstand:          endstand,
		}})
	}
	return out
}
