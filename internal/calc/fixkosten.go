package calc

import (
	"database/sql"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// FixkostenPosition is one of the 14 cost positions' result for one
// Fixkosten entry: its Logik/Typ for that year, the whole-house monthly
// value, and its split onto both apartments.
type FixkostenPosition struct {
	Key        string
	Label      string
	Logik      string
	Typ        string
	Monatswert float64
	KostenW1   float64
	KostenW2   float64
}

// KostenFor returns this position's cost for the given apartment (1 or 2)
// - the shared "which apartment's field" selector callers building per-
// apartment breakdowns (dashboard fixed-cost mode, yearly summary cards)
// would otherwise repeat as their own if/switch.
func (p FixkostenPosition) KostenFor(apartmentID int64) float64 {
	if apartmentID == 2 {
		return p.KostenW2
	}
	return p.KostenW1
}

// FixkostenErgebnis is one Fixkosten entry's full breakdown - all 14
// positions plus each apartment's total (rounded to the cent using
// commercial rounding, Issue #8, same convention as calc.Strom/Heizung/
// Wasser).
type FixkostenErgebnis struct {
	Positionen []FixkostenPosition
	KostenW1   float64
	KostenW2   float64
}

// KostenFor returns this result's total cost for the given apartment (1
// or 2), same selector convention as FixkostenPosition.KostenFor.
func (e FixkostenErgebnis) KostenFor(apartmentID int64) float64 {
	if apartmentID == 2 {
		return e.KostenW2
	}
	return e.KostenW1
}

// Fixkosten computes eingabeID's full cost position breakdown. Logik/
// Typ/Wert come directly from the entry itself (Issue #105/#107) - each
// entry carries its own independent state, no more shared per-year
// master-data source.
func Fixkosten(db *sql.DB, eingabeID int64) (*FixkostenErgebnis, error) {
	eingabe, err := store.GetFixkostenEingabeDetails(db, eingabeID)
	if err != nil {
		return nil, fmt.Errorf("fixkosten eingabe: %w", err)
	}
	if eingabe == nil {
		return nil, fmt.Errorf("fixkosten eingabe %d not found", eingabeID)
	}

	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return nil, fmt.Errorf("kostenpositionen: %w", err)
	}

	apartments, err := store.Apartments(db)
	if err != nil {
		return nil, fmt.Errorf("apartments: %w", err)
	}
	qmW1, qmW2 := apartmentValues(apartments, func(a store.Apartment) float64 { return a.QM })
	flurstueckW1, flurstueckW2 := apartmentValues(apartments, func(a store.Apartment) float64 { return a.FlurstueckGroesse })
	personenW1 := float64(eingabe.Personen[1])
	personenW2 := float64(eingabe.Personen[2])

	ergebnis := FixkostenErgebnis{Positionen: make([]FixkostenPosition, 0, len(kostenpositionen))}

	for _, kp := range kostenpositionen {
		w, ok := eingabe.Werte[kp.ID]
		if !ok {
			// This entry doesn't (yet) have a value for this position - skip
			// instead of guessing a Logik/Typ (analogous to the previous
			// behavior for a missing cost-position/year row).
			continue
		}

		monatswert := w.Wert
		if w.Typ == store.TypJaehrlich {
			monatswert = w.Wert / 12
		}

		ratioW1, ratioW2 := splitRatio(w.Logik, qmW1, qmW2, flurstueckW1, flurstueckW2, personenW1, personenW2)

		kostenW1 := Round2(monatswert * ratioW1)
		kostenW2 := Round2(monatswert * ratioW2)

		ergebnis.Positionen = append(ergebnis.Positionen, FixkostenPosition{
			Key:        kp.Key,
			Label:      kp.Label,
			Logik:      w.Logik,
			Typ:        w.Typ,
			Monatswert: monatswert,
			KostenW1:   kostenW1,
			KostenW2:   kostenW2,
		})
		ergebnis.KostenW1 += kostenW1
		ergebnis.KostenW2 += kostenW2
	}

	ergebnis.KostenW1 = Round2(ergebnis.KostenW1)
	ergebnis.KostenW2 = Round2(ergebnis.KostenW2)

	return &ergebnis, nil
}

// splitRatio returns apartment 1/2's shares for the given Logik.
// wohneinheit is a fixed 50/50; the other 3 delegate to Ratio2 (50/50
// fallback when both inputs are 0, Issue #26's zero-guard convention).
func splitRatio(logik string, qmW1, qmW2, flurstueckW1, flurstueckW2, personenW1, personenW2 float64) (w1, w2 float64) {
	switch logik {
	case store.LogikFlurstueck:
		return Ratio2(flurstueckW1, flurstueckW2)
	case store.LogikQM:
		return Ratio2(qmW1, qmW2)
	case store.LogikPersonen:
		return Ratio2(personenW1, personenW2)
	default: // store.LogikWohneinheit
		return 0.5, 0.5
	}
}
