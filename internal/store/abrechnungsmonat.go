package store

import (
	"fmt"
	"time"
)

// Abrechnungsmonat is a periods.monat value ("YYYY-MM-01") - see GLOSSARY.md.
type Abrechnungsmonat string

// Jahr returns the calendar year, or ok=false if m isn't a valid "YYYY-MM-01"
// value (CreatePeriod never validates Monat, see PeriodMonatTooEarlyError).
func (m Abrechnungsmonat) Jahr() (jahr int, ok bool) {
	t, err := time.Parse("2006-01-02", string(m))
	if err != nil {
		return 0, false
	}
	return t.Year(), true
}

// Monatsnamen are the German month names, January first.
var Monatsnamen = [...]string{
	"Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember",
}

// MonatLabel renders a date ("YYYY-MM-DD") as its German month name and year
// (e.g. "November 2026"). Falls back to the raw string if it isn't a
// parseable date.
func MonatLabel(datum string) string {
	t, err := time.Parse("2006-01-02", datum)
	if err != nil {
		return datum
	}
	return fmt.Sprintf("%s %d", Monatsnamen[t.Month()-1], t.Year())
}
