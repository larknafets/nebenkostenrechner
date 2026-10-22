package web

import (
	"errors"
	"fmt"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// reiheFehler translates a violation of the Ablesungsreihe rules (see
// store/reihe.go) into the German user-facing message and the name of the
// offending form field. ok is false for every other error.
func reiheFehler(err error) (msg, feld string, ok bool) {
	var tooEarly *store.PeriodDateTooEarlyError
	var tooLate *store.PeriodDateTooLateError
	var monatTooEarly *store.PeriodMonatTooEarlyError
	var monatTooLate *store.PeriodMonatTooLateError
	switch {
	case errors.As(err, &tooEarly):
		return fmt.Sprintf("Ablesedatum muss nach der Vorperiode (%s) liegen", tooEarly.Neighbor), "reading_date", true
	case errors.As(err, &tooLate):
		return fmt.Sprintf("Ablesedatum muss vor der Folgeperiode (%s) liegen", tooLate.Neighbor), "reading_date", true
	case errors.As(err, &monatTooEarly):
		return fmt.Sprintf("Abrechnungsmonat darf nicht vor dem der Vorperiode (%s) liegen", monatTooEarly.Neighbor), "monat", true
	case errors.As(err, &monatTooLate):
		return fmt.Sprintf("Abrechnungsmonat darf nicht nach dem der Folgeperiode (%s) liegen", monatTooLate.Neighbor), "monat", true
	}
	return "", "", false
}
