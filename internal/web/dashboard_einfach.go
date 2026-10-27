package web

import (
	"sort"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

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
type simpleWert func(k calc.Kosten) (segs []dashboardSegment, eur float64, ok bool)

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
	Wert: func(k calc.Kosten) (segs []dashboardSegment, eur float64, ok bool) {
		if k.Strom == nil {
			return nil, 0, false
		}
		return []dashboardSegment{
			{Farbe: "strom", Label: "Stromkosten", Kosten: k.Strom.KostenWallbox, Verbrauch: k.Strom.WallboxAnteilKWh, Einheit: "kWh"},
			{Farbe: "pv", Label: "PV-Anteil", Verbrauch: k.Strom.PVAnteilWallboxKWh, Einheit: "kWh"},
		}, k.Strom.KostenWallbox, true
	},
}

// wpSeries reads the heat pump's grid-draw share from StromErgebnis, shaped
// like wallboxSeries: "Stromkosten" (WPAnteilKWh, the billed share) and
// "PV-Anteil" (PVAnteilWPKWh, the gap to the sub-meter consumption).
var wpSeries = simpleSeries{
	ID: "waermepumpe", Name: "Wärmepumpe", IstErtrag: false,
	Wert: func(k calc.Kosten) (segs []dashboardSegment, eur float64, ok bool) {
		if k.Strom == nil {
			return nil, 0, false
		}
		kosten := calc.Round2(k.Strom.KostenWPGesamtUnrounded)
		return []dashboardSegment{
			{Farbe: "strom", Label: "Stromkosten", Kosten: kosten, Verbrauch: k.Strom.WPAnteilKWh, Einheit: "kWh"},
			{Farbe: "pv", Label: "PV-Anteil", Verbrauch: k.Strom.PVAnteilWPKWh, Einheit: "kWh"},
		}, kosten, true
	},
}

var pvSeries = simpleSeries{
	ID: "pv", Name: "PV-Anlage", IstErtrag: true,
	Wert: func(k calc.Kosten) (segs []dashboardSegment, eur float64, ok bool) {
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
