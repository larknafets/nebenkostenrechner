package web

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

type kosten struct {
	Strom       *calc.StromErgebnis
	Wasser      *calc.WasserErgebnis
	Heizung     *calc.HeizungErgebnis
	Einspeisung *calc.EinspeisungErgebnis
	KostenNote  string
}

func berechneKosten(db *sql.DB, periodID int64) (kosten, error) {
	// Teilstand/partial reading (Ticket #129): an incomplete reading does
	// not flow into the calculation - otherwise missing meter readings/
	// prices would silently be treated as 0 and show a wrong cost amount
	// instead of "not yet calculable".
	complete, err := store.PeriodComplete(db, periodID)
	if err != nil {
		return kosten{}, fmt.Errorf("period complete: %w", err)
	}
	if !complete {
		return kosten{KostenNote: "Diese Ablesung ist ein Teilstand - Kosten werden erst berechnet, sobald sie vollständig ist."}, nil
	}

	strom, err := calc.Strom(db, periodID)
	if errors.Is(err, store.ErrNoPreviousPeriod) {
		return kosten{KostenNote: "Kosten können erst ab der zweiten Ablesung berechnet werden (Verbrauch braucht eine Vorperiode)."}, nil
	} else if err != nil {
		return kosten{}, fmt.Errorf("strom kosten: %w", err)
	}

	wasser, err := calc.Wasser(db, periodID)
	if err != nil {
		return kosten{}, fmt.Errorf("wasser kosten: %w", err)
	}

	heizung, err := calc.Heizung(db, periodID)
	if err != nil {
		return kosten{}, fmt.Errorf("heizung kosten: %w", err)
	}

	einspeisung, err := calc.Einspeisung(db, periodID)
	if err != nil {
		return kosten{}, fmt.Errorf("einspeisung: %w", err)
	}

	return kosten{Strom: strom, Wasser: wasser, Heizung: heizung, Einspeisung: einspeisung}, nil
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

// kategorien builds the given apartment's cost breakdown for the period.
// Apartment 1's Strom (electricity) has no own cost position - its grid
// draw stays implicit (see calc.Strom) - so only apartment 2 gets a Strom
// category. Frischwasser (fresh water) and Abwasser (wastewater) are
// combined into a single Wasser category since they share one raw m³
// consumption (no separate wastewater meter, see calc.Wasser).
func kategorien(apartmentID int64, k kosten) []kategorie {
	var list []kategorie
	if apartmentID == 2 {
		list = append(list, kategorie{Kind: kategorieKindStrom, Label: "Strom", Kosten: k.Strom.KostenW2, Verbrauch: k.Strom.W2VerbrauchKWh, Einheit: "kWh", Farbe: "strom"})
	}

	heizungKosten, wpVerbrauchKWh, waermeMWh := k.Heizung.KostenHeizungW1, k.Heizung.WPVerbrauchW1KWh, k.Heizung.WaermeW1MWh
	frischwasserKosten, abwasserKosten, wasserM3 := k.Wasser.KostenFrischwasserW1, k.Wasser.KostenAbwasserW1, k.Wasser.FrischwasserW1
	if apartmentID == 2 {
		heizungKosten, wpVerbrauchKWh, waermeMWh = k.Heizung.KostenHeizungW2, k.Heizung.WPVerbrauchW2KWh, k.Heizung.WaermeW2MWh
		frischwasserKosten, abwasserKosten, wasserM3 = k.Wasser.KostenFrischwasserW2, k.Wasser.KostenAbwasserW2, k.Wasser.FrischwasserW2
	}
	list = append(list,
		kategorie{Kind: kategorieKindHeizung, Label: "Heizung/Warmwasser", Kosten: heizungKosten, Verbrauch: wpVerbrauchKWh, Einheit: "kWh", Farbe: "heizung", Verbrauch2: waermeMWh, Einheit2: "MWh"},
		kategorie{Kind: kategorieKindWasser, Label: "Wasser", Kosten: calc.Round2(frischwasserKosten + abwasserKosten), Verbrauch: wasserM3, Einheit: "m³", Farbe: "wasser"},
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

// periodKosten is one period's already-computed kosten, for the yearly-
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
	K        kosten
	Personen map[int64]int64
}
