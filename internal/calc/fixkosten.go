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

// FixkostenMonat is one Abrechnungsmonat's Fixkosten-Eingabe together with
// its calculated result.
type FixkostenMonat struct {
	Eingabe  *store.FixkostenEingabeDetails
	Ergebnis *FixkostenErgebnis
}

// FixkostenDaten is everything the Fixkostenreihe reads. Eingaben must be
// ordered oldest first (as store.AllFixkostenEingabenDetails returns them).
// Logik/Typ/Wert come directly from each Eingabe (Issue #105/#107) - each
// entry carries its own independent state.
type FixkostenDaten struct {
	Eingaben         []*store.FixkostenEingabeDetails
	Kostenpositionen []store.Kostenposition
	Apartments       []store.Apartment
}

// Fixkostenreihe holds the Fixkosten result of every Eingabe, computed once.
// There is exactly one Eingabe per Abrechnungsmonat.
type Fixkostenreihe struct {
	alle    []FixkostenMonat
	byID    map[int64]int
	byMonat map[string]int
}

// LoadFixkostenreihe reads all Fixkosten-Eingaben and the Stammdaten they
// depend on.
func LoadFixkostenreihe(db *sql.DB) (*Fixkostenreihe, error) {
	eingaben, err := store.AllFixkostenEingabenDetails(db)
	if err != nil {
		return nil, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return nil, fmt.Errorf("kostenpositionen: %w", err)
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return nil, fmt.Errorf("apartments: %w", err)
	}
	return NewFixkostenreihe(FixkostenDaten{Eingaben: eingaben, Kostenpositionen: kostenpositionen, Apartments: apartments}), nil
}

// NewFixkostenreihe computes the result of every Eingabe in d.
func NewFixkostenreihe(d FixkostenDaten) *Fixkostenreihe {
	r := &Fixkostenreihe{alle: make([]FixkostenMonat, len(d.Eingaben)), byID: make(map[int64]int, len(d.Eingaben)), byMonat: make(map[string]int, len(d.Eingaben))}
	qmW1, qmW2 := apartmentValues(d.Apartments, func(a store.Apartment) float64 { return a.QM })
	flurstueckW1, flurstueckW2 := apartmentValues(d.Apartments, func(a store.Apartment) float64 { return a.FlurstueckGroesse })
	for i, e := range d.Eingaben {
		r.byID[e.ID] = i
		r.byMonat[e.Monat] = i
		r.alle[i] = FixkostenMonat{Eingabe: e, Ergebnis: berechneFixkosten(fixkostenEingabe{
			Eingabe: e, Kostenpositionen: d.Kostenpositionen,
			QMW1: qmW1, QMW2: qmW2, FlurstueckW1: flurstueckW1, FlurstueckW2: flurstueckW2,
		})}
	}
	return r
}

// Eingabe returns the Fixkosten-Eingabe with the given id.
func (r *Fixkostenreihe) Eingabe(id int64) (FixkostenMonat, bool) {
	i, ok := r.byID[id]
	if !ok {
		return FixkostenMonat{}, false
	}
	return r.alle[i], true
}

// Monat returns the Fixkosten-Eingabe of the given Abrechnungsmonat.
func (r *Fixkostenreihe) Monat(monat string) (FixkostenMonat, bool) {
	i, ok := r.byMonat[monat]
	if !ok {
		return FixkostenMonat{}, false
	}
	return r.alle[i], true
}

// Alle returns every Eingabe with its result, oldest first.
func (r *Fixkostenreihe) Alle() []FixkostenMonat { return r.alle }

// fixkostenEingabe is everything the Fixkosten distribution reads: the entry
// itself, the Kostenpositionen and both apartments' Wohnungs-/Flurstücksgröße.
type fixkostenEingabe struct {
	Eingabe                    *store.FixkostenEingabeDetails
	Kostenpositionen           []store.Kostenposition
	QMW1, QMW2                 float64
	FlurstueckW1, FlurstueckW2 float64
}

// berechneFixkosten is the distribution itself, a pure function of its input.
func berechneFixkosten(in fixkostenEingabe) *FixkostenErgebnis {
	eingabe, kostenpositionen := in.Eingabe, in.Kostenpositionen
	qmW1, qmW2, flurstueckW1, flurstueckW2 := in.QMW1, in.QMW2, in.FlurstueckW1, in.FlurstueckW2
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

	return &ergebnis
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
	case store.LogikWohnung1:
		return 1, 0
	case store.LogikWohnung2:
		return 0, 1
	default: // store.LogikWohneinheit
		return 0.5, 0.5
	}
}
