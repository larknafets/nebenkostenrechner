package web

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboardSegment is one colored slice of a Dashboard bar - shared shape
// for the yearly-totals cards' mini-bar, the Monatsverlauf's consumption/
// fixed-costs/combined bars, and (via ProzentNeuestesGesamt/ProzentGesamt,
// reused for either "percent of the newest month" or "percent of this
// card's own total" depending on the call site) every percentage-scaled bar
// on the page.
type dashboardSegment struct {
	Farbe                 string
	Label                 string
	Kosten                float64
	Verbrauch             float64 // amount (kWh/MWh/m³) - only set for consumption categories, 0 for Fixkosten/Kombiniert
	Einheit               string
	ProzentNeuestesGesamt float64

	// Verbrauch2/Einheit2 is a second, optional quantity shown behind
	// Verbrauch/Einheit in the consumption values view - only set for
	// Heizung/Warmwasser (heating/hot water, see kategorie.Verbrauch2).
	// Einheit2 == "" means no second value.
	Verbrauch2 float64
	Einheit2   string
}

// setSegmentPct fills every segment's ProzentNeuestesGesamt as its share of
// denom (percent, not compressed to fit - Ticket #19's established
// convention: an older/bigger total can run past 100%). No-op if denom<=0.
func setSegmentPct(segs []dashboardSegment, denom float64) {
	if denom <= 0 {
		return
	}
	for i := range segs {
		segs[i].ProzentNeuestesGesamt = segs[i].Kosten / denom * 100
	}
}

// fixkostenKosten is one fixed-costs entry's month (Monat) and computed
// result (Ergebnis).
type fixkostenKosten struct {
	Monat    string
	Erg      *calc.FixkostenErgebnis
	Abschlag map[int64]float64 // apartment id -> utility advance payment value, nil/missing entry = no value recorded
}

// alleFixkostenKosten returns every computable fixed-costs entry's result,
// newest first (store.AllFixkostenEingaben's own order).
func alleFixkostenKosten(db *sql.DB) ([]fixkostenKosten, error) {
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return nil, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	abschlaege, err := store.AllAbschlaege(db)
	if err != nil {
		return nil, fmt.Errorf("nebenkosten abschlaege: %w", err)
	}

	out := make([]fixkostenKosten, 0, len(eingaben))
	for _, e := range eingaben {
		erg, err := calc.Fixkosten(db, e.ID)
		if err != nil {
			return nil, fmt.Errorf("fixkosten %d: %w", e.ID, err)
		}
		out = append(out, fixkostenKosten{Monat: e.Monat, Erg: erg, Abschlag: abschlaege[e.ID]})
	}
	return out, nil
}

// fixkostenGruppen groups one fixed-costs result's 14 positions by Logik
// (allocation logic) into the 4 fixed buckets the "Fixkosten" mode bar
// shows (always all 4, even at 0 - same "show every category" convention
// as kategorien()).
func fixkostenGruppen(apartmentID int64, erg *calc.FixkostenErgebnis) []dashboardSegment {
	sums := map[string]float64{}
	for _, p := range erg.Positionen {
		sums[p.Logik] += p.KostenFor(apartmentID)
	}
	logiken := []string{store.LogikWohneinheit, store.LogikFlurstueck, store.LogikQM, store.LogikPersonen}
	out := make([]dashboardSegment, len(logiken))
	for i, logik := range logiken {
		out[i] = dashboardSegment{Farbe: "logik-" + logik, Label: logikLabels[logik], Kosten: calc.Round2(sums[logik])}
	}
	return out
}

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
	HeizungEUR          float64
	WasserEUR           float64
	VerbrauchEUR        float64
	GesamtEUR           float64
	Segmente            []dashboardSegment

	// PVAnteilKWh is the yearly total of StromErgebnis.PVAnteilW2KWh ("not
	// allocated to grid draw (PV)", CONTEXT.md) - only set for apartment 2
	// (Issue #98), since apartment 1 has no own electricity meter. 0 =
	// row is hidden.
	PVAnteilKWh float64

	// Nebenkostenabschlag-Saldo (utility advance payment balance):
	// continuously accumulated since recording began (no yearly reset),
	// as of the latest month - see buildDashboardVerlauf, which computes
	// the balance per month; taken over from there (handleDashboard), not
	// computed here in buildJahresCard. nil = no balance calculable (see
	// AbschlagSaldo).
	Saldo *AbschlagSaldo
}

// buildJahresCard sums the given apartment's consumption and fixed costs
// over every period/entry whose date falls in jahr, and (Ticket #75)
// averages the apartment's occupant count over jahr's readings (1 decimal
// place, 0 if the year has no reading yet).
func buildJahresCard(apartmentID int64, apartmentName string, apartmentQM, apartmentFlurstueck float64, jahr int, periodenKosten []periodKosten, fixkostenListe []fixkostenKosten) dashboardJahresCard {
	var strom, heizung, wasser, fix float64
	var pvAnteilKWh float64
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
			case kategorieKindHeizung:
				heizung += kat.Kosten
			case kategorieKindWasser:
				wasser += kat.Kosten
			}
		}
		if apartmentID == 2 && pk.K.Strom != nil {
			pvAnteilKWh += pk.K.Strom.PVAnteilW2KWh
		}
		if p, ok := pk.Personen[apartmentID]; ok {
			personenSumme += float64(p)
			personenAnzahl++
		}
	}
	var personenSchnitt float64
	if personenAnzahl > 0 {
		personenSchnitt = personenSumme / float64(personenAnzahl)
	}
	for _, fk := range fixkostenListe {
		y, ok := store.Abrechnungsmonat(fk.Monat).Jahr()
		if !ok || y != jahr {
			continue
		}
		fix += fk.Erg.KostenFor(apartmentID)
	}
	strom, heizung, wasser, fix = calc.Round2(strom), calc.Round2(heizung), calc.Round2(wasser), calc.Round2(fix)
	verbrauch := calc.Round2(strom + heizung + wasser)
	gesamt := calc.Round2(verbrauch + fix)

	segs := []dashboardSegment{{Farbe: "fix", Label: "Fixkosten", Kosten: fix}}
	if apartmentID == 2 {
		segs = append(segs, dashboardSegment{Farbe: "strom", Label: "Strom", Kosten: strom})
	}
	segs = append(segs,
		dashboardSegment{Farbe: "heizung", Label: "Heizung/Ww", Kosten: heizung},
		dashboardSegment{Farbe: "wasser", Label: "Wasser", Kosten: wasser},
	)
	setSegmentPct(segs, gesamt)

	return dashboardJahresCard{
		ApartmentID: apartmentID, ApartmentName: apartmentName, ApartmentQM: apartmentQM,
		ApartmentFlurstueck: apartmentFlurstueck, PersonenSchnitt: personenSchnitt,
		FixkostenEUR: fix, StromEUR: strom, HeizungEUR: heizung, WasserEUR: wasser,
		VerbrauchEUR: verbrauch, GesamtEUR: gesamt, Segmente: segs,
		PVAnteilKWh: pvAnteilKWh,
	}
}

// dashboardMonat is one calendar month's combined consumption+fixed-costs
// Monatsverlauf row - up to 3 independently pct-scaled bar variants (one
// per numeric mode); the 4th mode ("Verbrauchswerte"/consumption values)
// reuses VerbrauchSegmente's raw Verbrauch/Einheit as text instead of a
// bar.
type dashboardMonat struct {
	Label     string
	Jahr      int
	IsCurrent bool

	HasVerbrauch      bool
	VerbrauchSegmente []dashboardSegment
	VerbrauchGesamt   float64

	HasFixkosten      bool
	FixkostenSegmente []dashboardSegment
	FixkostenGesamt   float64

	HasKombiniert      bool
	KombiniertSegmente []dashboardSegment
	KombiniertGesamt   float64

	// Nebenkostenabschlag-Saldo (utility advance payment balance, 5th mode
	// "abschlag"): continuously accumulated balance up to and including
	// this month - abschlag(m) - KombiniertGesamt(m) summed per month
	// since the very first recorded month, no yearly reset. nil if the
	// month is not HasKombiniert (missing months leave the balance
	// unchanged, see buildDashboardVerlauf).
	Saldo *AbschlagSaldo

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
	GesamtSumme    float64

	// Endstand (closing balance): the accumulated balance at year end (or
	// at the latest recorded month, for a running year) - not a sum,
	// since the balance is continuous and can't meaningfully be added up
	// per year (see #92/#94). Own field name instead of "Saldo" like on
	// dashboardMonat, because this refers to a different point in time
	// (year end, not the current month).
	Endstand *AbschlagSaldo
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

	// LatestSaldo is the newest month with a balance (newest-first, skips
	// year-summary rows and gapped months) - set directly from the
	// backward loop in buildDashboardVerlauf, instead of a 2nd function
	// (latestAbschlagSaldo) walking through Eintraege afterwards to find
	// the same value again (#99/#102: duplicated code). nil = no balance
	// calculable.
	LatestSaldo *AbschlagSaldo
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
// are already newest-first).
func buildDashboardVerlauf(apartmentID int64, apartmentName string, periodenKosten []periodKosten, fixkostenListe []fixkostenKosten) dashboardVerlaufSpalte {
	type bucket struct {
		label         string
		jahr          int
		verbrauchKats []kategorie
		fixErg        *calc.FixkostenErgebnis
		abschlagWert  float64 // 0 if no value recorded (see #92: a missing value counts as 0)
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
			b.abschlagWert = fk.Abschlag[apartmentID]
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(order)))

	monate := make([]dashboardMonat, 0, len(order))
	abschlagWerte := make([]float64, 0, len(order))
	for _, key := range order {
		b := buckets[key]
		dm := dashboardMonat{Label: b.label, Jahr: b.jahr}
		abschlagWerte = append(abschlagWerte, b.abschlagWert)

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
			if dm.HasVerbrauch {
				dm.KombiniertSegmente = append(dm.KombiniertSegmente, dashboardSegment{Farbe: "verbrauch-gesamt", Label: "Verbrauch", Kosten: dm.VerbrauchGesamt})
			}
			if dm.HasFixkosten {
				dm.KombiniertSegmente = append(dm.KombiniertSegmente, dashboardSegment{Farbe: "fix", Label: "Fixkosten", Kosten: dm.FixkostenGesamt})
			}
			dm.KombiniertGesamt = calc.Round2(dm.VerbrauchGesamt + dm.FixkostenGesamt)
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
		if m.HasKombiniert && m.KombiniertGesamt > maxKombiniert {
			maxKombiniert = m.KombiniertGesamt
		}
	}
	for i := range monate {
		setSegmentPct(monate[i].VerbrauchSegmente, maxVerbrauch)
		setSegmentPct(monate[i].FixkostenSegmente, maxFixkosten)
		setSegmentPct(monate[i].KombiniertSegmente, maxKombiniert)
	}

	// Nebenkostenabschlag-Saldo (utility advance payment balance):
	// continuously accumulated from oldest to newest month (monate is
	// sorted newest-first, hence backwards) - saldo(m) = abschlag(m) -
	// KombiniertGesamt(m). The gate is HasFixkosten, not HasKombiniert:
	// abschlagWert comes exclusively from a fixed-costs entry (see the
	// fixErg==nil condition above) - a month with only a reading but
	// (still) no fixed-costs entry has no recorded advance-payment value
	// and would otherwise be wrongly computed with abschlag=0, instead of
	// being skipped like a month with no data (#94: "no data" instead of
	// an implicit jump).
	var laufenderSaldo float64
	saldi := make([]float64, len(monate))
	hatSaldo := make([]bool, len(monate))
	var maxAbsSaldo float64
	for i := len(monate) - 1; i >= 0; i-- {
		if !monate[i].HasFixkosten {
			continue
		}
		laufenderSaldo = calc.Round2(laufenderSaldo + calc.Round2(abschlagWerte[i]-monate[i].KombiniertGesamt))
		saldi[i] = laufenderSaldo
		hatSaldo[i] = true
		if abs := math.Abs(laufenderSaldo); abs > maxAbsSaldo {
			maxAbsSaldo = abs
		}
	}
	for i := range monate {
		if !hatSaldo[i] {
			continue
		}
		monate[i].Saldo = newAbschlagSaldo(saldi[i])
		if maxAbsSaldo > 0 {
			monate[i].AbschlagProzent = math.Abs(saldi[i]) / maxAbsSaldo * 50
		}
	}

	var latestSaldo *AbschlagSaldo
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

// buildEntityView bundles what every apartmentID-based Dashboard/widget
// route needs in one place anyway: the yearly-totals card, the monthly
// history, and the balance wired consistently between both (#99,
// candidate 3) - replaces 3 call sites that used to rebuild this manually
// (dashboard.go handleDashboard, widgets.go handleWidgetJahressumme/
// handleWidgetUebersicht). Always builds the full history, even when a
// caller only needs the card (#101: no card-only mode, avoiding
// speculative generality). Only intended for the 2 apartments - Wallboxen/
// PV-Anlage have no balance concept and run via buildSimpleJahresCard/
// buildSimpleVerlauf.
func buildEntityView(dd dashboardData, apartmentID int64) (dashboardJahresCard, dashboardVerlaufSpalte) {
	a := findApartment(dd.Apartments, apartmentID)
	verlauf := buildDashboardVerlauf(a.ID, a.Name, dd.PeriodenKosten, dd.FixkostenListe)
	card := buildJahresCard(a.ID, a.Name, a.QM, a.FlurstueckGroesse, dd.Jahr, dd.PeriodenKosten, dd.FixkostenListe)
	card.Saldo = verlauf.LatestSaldo
	return card, verlauf
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
		var vSumme, fSumme float64
		var endstand *AbschlagSaldo
		for _, m := range g.Items {
			vSumme += m.VerbrauchGesamt
			fSumme += m.FixkostenGesamt
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
			VerbrauchSumme: calc.Round2(vSumme), FixkostenSumme: calc.Round2(fSumme), GesamtSumme: calc.Round2(vSumme + fSumme),
			Endstand: endstand,
		}})
	}
	return out
}

// dashboardSimpleCard is Wallbox/PV-Anlage's (wallbox/PV system) yearly-
// totals card (Ticket #67) - a single EUR figure, unlike
// dashboardJahresCard: whole-house, no apartment split and no fixed-costs
// share (purely informative, see StromErgebnis.WallboxAnteilKWh/
// calc.Einspeisung).
type dashboardSimpleCard struct {
	Name      string
	GesamtEUR float64
	IstErtrag bool // true for PV-Anlage: "+" prefix, success color (feed-in compensation instead of cost)

	// Segmente is the yearly total per bar segment (kWh via Verbrauch,
	// EUR via Kosten) - 2 for Wallbox (electricity cost/PV share), 1 for
	// PV-Anlage (feed-in compensation), analogous to buildSimpleVerlauf's
	// monthly segments.
	Segmente []dashboardSegment
}

// dashboardSimpleMonat is one calendar month's row in a Wallbox/PV-Anlage
// Monatsverlauf - 1+ bar segments (Segmente, in actual kWh - Ticket #67
// follow-up: Wallbox shows 2 here, "Stromkosten" + "PV-Anteil", instead of
// just the billed share), unlike dashboardMonat's 4 mode variants (there's
// no fixed-costs/combined distinction for these).
type dashboardSimpleMonat struct {
	Label     string
	Jahr      int
	IsCurrent bool
	HasWert   bool
	EUR       float64
	Segmente  []dashboardSegment
}

type dashboardSimpleJahreszeile struct {
	Jahr       int
	IstLaufend bool
	Summe      float64
}

type dashboardSimpleEintrag struct {
	Monat       *dashboardSimpleMonat
	Jahreszeile *dashboardSimpleJahreszeile
}

// dashboardSimpleSpalte is Wallbox/PV-Anlage's Monatsverlauf, newest first -
// analogous to dashboardVerlaufSpalte, but without an apartment split.
type dashboardSimpleSpalte struct {
	ID        string
	Name      string
	IstErtrag bool
	Eintraege []dashboardSimpleEintrag
}

// simpleWert extracts one period's bar segments (actual kWh per segment,
// not just the billed share) and total EUR for a Wallbox/PV-Anlage series
// - ok=false skips the period entirely (no reading/no previous period).
type simpleWert func(k kosten) (segs []dashboardSegment, eur float64, ok bool)

// simpleSeries bundles a Wallbox/PV-Anlage entity's identity (ID/Name/
// IstErtrag) with its value extractor - the 2 call sites in
// handleDashboard otherwise repeated the same id/name/istErtrag/wert-func
// quadruple twice each, once per build* call.
type simpleSeries struct {
	ID        string
	Name      string
	IstErtrag bool
	Wert      simpleWert
}

// wallboxSeries reads the wallbox share from StromErgebnis - a 3rd,
// PV-grid-draw-capped allocation tier after apartment 2 and the heat pump
// (Ticket #67 follow-up). The bar shows the actual wallbox consumption as
// 2 segments: "Stromkosten" (WallboxAnteilKWh, the billed share) and
// "PV-Anteil" (PVAnteilWallboxKWh, the gap between sub-meter consumption
// and what the grid draw actually provided - analogous to the "not
// allocated to grid draw (PV)" row for apartment 2/heat pump). The sum of
// both segments is the raw sub-meter consumption, not just the billed
// part.
var wallboxSeries = simpleSeries{
	ID: "wallbox", Name: "Wallboxen", IstErtrag: false,
	Wert: func(k kosten) (segs []dashboardSegment, eur float64, ok bool) {
		if k.Strom == nil {
			return nil, 0, false
		}
		return []dashboardSegment{
			{Farbe: "strom", Label: "Stromkosten", Kosten: k.Strom.KostenWallbox, Verbrauch: k.Strom.WallboxAnteilKWh, Einheit: "kWh"},
			{Farbe: "pv", Label: "PV-Anteil", Verbrauch: k.Strom.PVAnteilWallboxKWh, Einheit: "kWh"},
		}, k.Strom.KostenWallbox, true
	},
}

var pvSeries = simpleSeries{
	ID: "pv", Name: "PV-Anlage", IstErtrag: true,
	Wert: func(k kosten) (segs []dashboardSegment, eur float64, ok bool) {
		if k.Einspeisung == nil {
			return nil, 0, false
		}
		return []dashboardSegment{
			{Farbe: "pv", Label: "Einspeisevergütung", Kosten: k.Einspeisung.Ertrag, Verbrauch: k.Einspeisung.EinspeisungKWh, Einheit: "kWh"},
		}, k.Einspeisung.Ertrag, true
	},
}

// buildSimpleJahresCard sums a Wallbox/PV-Anlage series over every period
// whose ReadingDate falls in jahr.
func buildSimpleJahresCard(series simpleSeries, jahr int, periodenKosten []periodKosten) dashboardSimpleCard {
	var sum float64
	var segSummen []dashboardSegment // same order/labels as series.Wert returns
	for _, pk := range periodenKosten {
		y, ok := store.Abrechnungsmonat(pk.Monat).Jahr()
		if !ok || y != jahr {
			continue
		}
		segs, eur, ok := series.Wert(pk.K)
		if !ok {
			continue
		}
		sum += eur
		if segSummen == nil {
			segSummen = make([]dashboardSegment, len(segs))
			for i, s := range segs {
				segSummen[i] = dashboardSegment{Farbe: s.Farbe, Label: s.Label, Einheit: s.Einheit}
			}
		}
		for i, s := range segs {
			segSummen[i].Kosten += s.Kosten
			segSummen[i].Verbrauch += s.Verbrauch
		}
	}

	var gesamtKWh float64
	for _, s := range segSummen {
		gesamtKWh += s.Verbrauch
	}
	for i := range segSummen {
		segSummen[i].Kosten = calc.Round2(segSummen[i].Kosten)
		segSummen[i].Verbrauch = calc.Round2(segSummen[i].Verbrauch)
		if gesamtKWh > 0 {
			segSummen[i].ProzentNeuestesGesamt = segSummen[i].Verbrauch / gesamtKWh * 100
		}
	}

	return dashboardSimpleCard{Name: series.Name, GesamtEUR: calc.Round2(sum), IstErtrag: series.IstErtrag, Segmente: segSummen}
}

// buildSimpleVerlauf builds a Wallbox/PV-Anlage Monatsverlauf - one bar per
// reading month plus a yearly-total separator row per calendar year,
// analogous to buildDashboardVerlauf's fixed-costs-free case. The bar
// scales by the sum of segment kWh (actual consumption), not by EUR -
// for Wallbox, the unbilled PV share would otherwise make the width
// disproportionate.
func buildSimpleVerlauf(series simpleSeries, periodenKosten []periodKosten) dashboardSimpleSpalte {
	id, name, istErtrag, wert := series.ID, series.Name, series.IstErtrag, series.Wert

	type monatAgg struct {
		jahr     int
		hasWert  bool
		eur      float64
		segmente []dashboardSegment
	}
	byMonat := map[string]*monatAgg{}
	var order []string
	for _, pk := range periodenKosten {
		jahr, ok := store.Abrechnungsmonat(pk.Monat).Jahr()
		if !ok {
			continue
		}
		agg, exists := byMonat[pk.Monat]
		if !exists {
			agg = &monatAgg{jahr: jahr}
			byMonat[pk.Monat] = agg
			order = append(order, pk.Monat)
		}
		if segs, eur, ok := wert(pk.K); ok {
			agg.hasWert = true
			agg.eur += eur
			if agg.segmente == nil {
				agg.segmente = make([]dashboardSegment, len(segs))
				for i, s := range segs {
					agg.segmente[i] = dashboardSegment{Farbe: s.Farbe, Label: s.Label, Einheit: s.Einheit}
				}
			}
			for i, s := range segs {
				agg.segmente[i].Kosten += s.Kosten
				agg.segmente[i].Verbrauch += s.Verbrauch
			}
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(order)))

	monate := make([]dashboardSimpleMonat, 0, len(order))
	for _, monat := range order {
		agg := byMonat[monat]
		dm := dashboardSimpleMonat{Label: germanPeriodLabelShort(monat), Jahr: agg.jahr, HasWert: agg.hasWert}
		if agg.hasWert {
			for i := range agg.segmente {
				agg.segmente[i].Kosten = calc.Round2(agg.segmente[i].Kosten)
				agg.segmente[i].Verbrauch = calc.Round2(agg.segmente[i].Verbrauch)
			}
			dm.Segmente = agg.segmente
			dm.EUR = calc.Round2(agg.eur)
		}
		monate = append(monate, dm)
	}
	if len(monate) > 0 {
		monate[0].IsCurrent = true
	}

	segmentSumme := func(segs []dashboardSegment) float64 {
		var sum float64
		for _, s := range segs {
			sum += s.Verbrauch
		}
		return sum
	}

	// Baseline = the largest month across the entire displayed time range
	// (no bar past 100%, see buildDashboardVerlauf).
	var maxKWh float64
	for _, m := range monate {
		if m.HasWert {
			if s := segmentSumme(m.Segmente); s > maxKWh {
				maxKWh = s
			}
		}
	}
	if maxKWh > 0 {
		for i := range monate {
			if !monate[i].HasWert {
				continue
			}
			for j := range monate[i].Segmente {
				monate[i].Segmente[j].ProzentNeuestesGesamt = monate[i].Segmente[j].Verbrauch / maxKWh * 100
			}
		}
	}

	var eintraege []dashboardSimpleEintrag
	for _, g := range gruppiereNachJahr(monate, func(m dashboardSimpleMonat) int { return m.Jahr }) {
		var summe float64
		for _, m := range g.Items {
			summe += m.EUR
			eintraege = append(eintraege, dashboardSimpleEintrag{Monat: &m})
		}
		eintraege = append(eintraege, dashboardSimpleEintrag{Jahreszeile: &dashboardSimpleJahreszeile{
			Jahr: g.Jahr, IstLaufend: g.IstLaufend, Summe: calc.Round2(summe),
		}})
	}

	return dashboardSimpleSpalte{ID: id, Name: name, IstErtrag: istErtrag, Eintraege: eintraege}
}
