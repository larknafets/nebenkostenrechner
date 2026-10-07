package web

import "github.com/larknafets/nebenkostenrechner/internal/calc"

// kostenHinweis is the text shown on the Ablesung page for an Ablesung
// without Verbrauchskosten.
func kostenHinweis(g calc.Grund) string {
	switch g {
	case calc.GrundTeilstand:
		return "Diese Ablesung ist ein Teilstand - Kosten werden erst berechnet, sobald sie vollständig ist."
	case calc.GrundKeineVorperiode:
		return "Kosten können erst ab der zweiten Ablesung berechnet werden (Verbrauch braucht eine Vorperiode)."
	}
	return ""
}

// kategorieKind identifies a kategorie's cost type at compile time - Kind is
// what callers like buildJahresCard should switch on, Label is display-only
// (architecture review: switching on Label let a typo silently drop a whole
// Kategorie from a caller's totals with no compile-time check).
type kategorieKind int

const (
	kategorieKindStrom kategorieKind = iota
	kategorieKindHeizung
	kategorieKindWasser
)

// kategorie is one cost position (Strom, Heizung, or Wasser), together with
// its share of that apartment's total (ProzentGesamt, the bar segment's
// width) and the raw consumption shown in brackets next to the EUR amount
// (Ticket #18).
type kategorie struct {
	Kind      kategorieKind
	Label     string
	Kosten    float64
	Verbrauch float64
	Einheit   string
	// Farbe is a bare CSS class suffix (e.g. "strom" -> class "cat-strom"),
	// not interpolated into a style attribute - html/template's CSS
	// sanitizer can't statically verify a dynamic var(...) argument there
	// and replaces it with the ZgotmplZ sentinel instead of rendering it.
	Farbe string

	ProzentGesamt float64

	// Verbrauch2/Einheit2 is a second, optional quantity shown behind
	// Verbrauch/Einheit in the consumption values view - only set for
	// Heizung/Warmwasser (heating/hot water): Verbrauch there is the
	// actual (raw, without PV deduction) heat pump electricity in kWh,
	// Verbrauch2 the raw heat meter consumption (MWh, pure space heating)
	// for comparison. Einheit2 == "" means: no second value.
	Verbrauch2 float64
	Einheit2   string
}

// kategorien builds the given apartment's cost breakdown for the period from
// its monatsAnteil.
func kategorien(apartmentID int64, k calc.Kosten) []kategorie {
	a := k.Anteil(apartmentID)
	var list []kategorie
	if a.HatStrom {
		list = append(list, kategorie{Kind: kategorieKindStrom, Label: "Strom", Kosten: a.StromKosten, Verbrauch: a.StromKWh, Einheit: "kWh", Farbe: "strom"})
	}
	list = append(list,
		kategorie{Kind: kategorieKindHeizung, Label: "Heizung/Warmwasser", Kosten: a.HeizungKosten, Verbrauch: a.HeizungWPKWh, Einheit: "kWh", Farbe: "heizung", Verbrauch2: a.HeizungMWh, Einheit2: "MWh"},
		kategorie{Kind: kategorieKindWasser, Label: "Wasser", Kosten: a.WasserKosten, Verbrauch: a.WasserM3, Einheit: "m³", Farbe: "wasser"},
	)

	var total float64
	for _, kat := range list {
		total += kat.Kosten
	}
	if total > 0 {
		for i := range list {
			list[i].ProzentGesamt = list[i].Kosten / total * 100
		}
	}
	return list
}

// periodKosten is one period's already-computed calc.Kosten, for the yearly-
// totals cards and Monatsverlauf (monthly history). ReadingDate stays in
// its raw "YYYY-MM-DD" form (not pre-formatted) since downstream needs it
// for both the month label and calendar-year grouping. Personen is this
// period's occupant count per apartment (Ticket #75's average occupant
// count averages this across a year's periods).
type periodKosten struct {
	ReadingDate string
	// Monat is this period's billing month (Issue #86, "YYYY-MM-01") -
	// the key groupKostenByMonat and the yearly cards group/filter by,
	// distinct from ReadingDate which stays the exact reading date.
	Monat    string
	K        calc.Kosten
	Personen map[int64]int64
}
