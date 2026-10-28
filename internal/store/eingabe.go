package store

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// Logiken is the set of valid allocation logics (fixkosten_werte.logik). The
// store keeps its own copy because calc imports store, so the store cannot
// read calc.LogikLabels. A test in calc keeps both in sync.
var Logiken = map[string]bool{
	LogikWohneinheit: true,
	LogikFlurstueck:  true,
	LogikQM:          true,
	LogikPersonen:    true,
	LogikWohnung1:    true,
	LogikWohnung2:    true,
}

// Feldart says which field of an input violates a rule.
type Feldart int

const (
	FeldReadingDate Feldart = iota
	FeldMonat
	FeldLogik    // ID = Kostenposition
	FeldTyp      // ID = Kostenposition
	FeldPersonen // ID = Wohnung
	// The four prices of a Fixkosten-Eingabe, Err gives the reason.
	FeldStrompreis
	FeldFrischwasserPreis
	FeldAbwasserPreis
	FeldEinspeisungPreis
)

// EingabeFehler is the typed error for an invalid input. It carries no German
// text: the adapters (forms, CSV imports) build their messages from it.
type EingabeFehler struct {
	Feld Feldart
	ID   int64  // Kostenposition or Wohnung, 0 without reference
	Wert string // the offending value as text
	Err  error  // reason, only set for Personenzahl and prices
}

func (e *EingabeFehler) Error() string {
	return fmt.Sprintf("invalid input (field %d, id %d): %q", e.Feld, e.ID, e.Wert)
}

func (e *EingabeFehler) Unwrap() error { return e.Err }

// idsAufsteigend returns the keys of m in ascending order.
func idsAufsteigend[V any](m map[int64]V) []int64 {
	ids := make([]int64, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// validierePreis is the single check for a price of a Fixkosten-Eingabe: finite
// and not negative (0 stays allowed). nil is "nicht erfasst" and valid.
func validierePreis(p *float64) error {
	if p == nil {
		return nil
	}
	if math.IsNaN(*p) || math.IsInf(*p, 0) {
		return errors.New("Preis muss eine Zahl sein")
	}
	if *p < 0 {
		return errors.New("Preis darf nicht negativ sein")
	}
	return nil
}

// ValidierePersonenzahl is the single check for an occupant count: finite,
// >= 0 (0 stays allowed) and at most 2 decimal places. The forms and both
// CSV imports use it.
func ValidierePersonenzahl(p float64) error {
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return errors.New("Personenzahl muss eine Zahl sein")
	}
	if p < 0 {
		return errors.New("Personenzahl darf nicht negativ sein")
	}
	if math.Abs(p*100-math.Round(p*100)) > 1e-9 {
		return errors.New("Personenzahl darf höchstens 2 Nachkommastellen haben")
	}
	return nil
}
