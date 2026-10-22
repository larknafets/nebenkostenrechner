package store

import (
	"database/sql"
	"fmt"
)

// The Ablesungsreihe rules (GLOSSARY.md) live in this file: the reading date
// rises strictly, the Abrechnungsmonat never falls, and only the newest
// Ablesung may be a Teilstand. CreatePeriod and ImportPeriods append to the
// series (checkAppend), UpdatePeriod corrects inside it (checkReadingOrder),
// DeletePeriod needs no rule. Existing data is never re-checked, only writes.

// querier is the read side shared by *sql.DB and *sql.Tx, so a rule can run
// inside the transaction that is about to write.
type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

// PeriodOpenTeilstandError is returned by CreatePeriod and ImportPeriods when
// the newest Ablesung is still a Teilstand: a second open reading must not
// be appended. ID is the open Teilstand, so a caller can lead to completing it.
type PeriodOpenTeilstandError struct {
	ID int64
}

func (e *PeriodOpenTeilstandError) Error() string {
	return fmt.Sprintf("period %d is still a Teilstand, complete it before adding a new reading", e.ID)
}

// ImportRowError is returned by ImportPeriods when an input breaks an
// Ablesungsreihe rule: the store knows no CSV line numbers, so the error
// names the input's reading date. Err is the rule's own error.
type ImportRowError struct {
	ReadingDate string
	Err         error
}

func (e *ImportRowError) Error() string {
	return fmt.Sprintf("reading %s: %v", e.ReadingDate, e.Err)
}

func (e *ImportRowError) Unwrap() error { return e.Err }

// checkAppend validates in as a new, newest Ablesung: the newest existing one
// must be complete, the reading date must lie after its date and the
// Abrechnungsmonat must not lie before its Monat. An empty Monat (Teilstand,
// Ticket #128) is not a conflict until it is entered. Without any Ablesung
// everything is allowed.
func checkAppend(q querier, in PeriodInput) error {
	var newest PeriodSummary
	err := q.QueryRow(`SELECT id, reading_date, monat FROM periods ORDER BY reading_date DESC, id DESC LIMIT 1`).
		Scan(&newest.ID, &newest.ReadingDate, &newest.Monat)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return fmt.Errorf("query newest period: %w", err)
	}

	complete, err := periodComplete(q, newest.ID)
	if err != nil {
		return err
	}
	if !complete {
		return &PeriodOpenTeilstandError{ID: newest.ID}
	}
	if in.ReadingDate <= newest.ReadingDate {
		return &PeriodDateTooEarlyError{Neighbor: newest.ReadingDate}
	}
	if in.Monat != "" && in.Monat < newest.Monat {
		return &PeriodMonatTooEarlyError{Neighbor: newest.Monat}
	}
	return nil
}

// PeriodDateTooEarlyError is returned by CreatePeriod, ImportPeriods and
// UpdatePeriod when a reading date would lie at or before its chronological
// predecessor's date.
type PeriodDateTooEarlyError struct {
	Neighbor string // the predecessor's ReadingDate
}

func (e *PeriodDateTooEarlyError) Error() string {
	return fmt.Sprintf("reading date must be after the previous period (%s)", e.Neighbor)
}

// PeriodDateTooLateError is UpdatePeriod's mirror of PeriodDateTooEarlyError
// for the chronological successor.
type PeriodDateTooLateError struct {
	Neighbor string // the successor's ReadingDate
}

func (e *PeriodDateTooLateError) Error() string {
	return fmt.Sprintf("reading date must be before the next period (%s)", e.Neighbor)
}

// neighborValues finds, among `all` periods excluding periodID, the
// closest ReadingDate below and above `currentDate`, returning what
// `value` reads off each of those two neighbors - the shared lookup behind
// dateOrderRule and monatOrderRule (Issue #86 code review: these
// were an exact structural duplicate, differing only in which field they
// read off the neighbor). The range is the one a correction may move
// periodID's own field within without silently reordering it past a
// neighbor (Ticket #44 review finding: generalizing "korrigieren" to any
// period means a change can now shift which period is whose Vorperiode for
// everyone in between, not just itself).
func neighborValues(all []PeriodSummary, periodID int64, currentDate string, value func(PeriodSummary) string) (prev, next string, hasPrev, hasNext bool) {
	var prevDate, nextDate string
	for _, p := range all {
		if p.ID == periodID {
			continue
		}
		if p.ReadingDate <= currentDate && (!hasPrev || p.ReadingDate > prevDate) {
			prevDate, prev, hasPrev = p.ReadingDate, value(p), true
		}
		if p.ReadingDate >= currentDate && (!hasNext || p.ReadingDate < nextDate) {
			nextDate, next, hasNext = p.ReadingDate, value(p), true
		}
	}
	return
}

// periodNeighborContext fetches periodID's own (pre-edit) ReadingDate and
// every period, for the neighbor-bounds check in checkReadingOrder.
func periodNeighborContext(db *sql.DB, periodID int64) (currentDate string, all []PeriodSummary, err error) {
	if err := db.QueryRow(`SELECT reading_date FROM periods WHERE id = ?`, periodID).Scan(&currentDate); err != nil {
		if err == sql.ErrNoRows {
			return "", nil, fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
		}
		return "", nil, fmt.Errorf("query period %d: %w", periodID, err)
	}

	all, err = AllPeriods(db)
	if err != nil {
		return "", nil, fmt.Errorf("periods: %w", err)
	}
	return currentDate, all, nil
}

// orderRule is one chronological-order rule: which field of a period must
// stay within its neighbors' values, whether equality counts as a
// violation, and which error reports a violation on either side. Date and
// Monat differ only in these four things, so the check itself
// (checkReadingOrder) is written once.
type orderRule struct {
	value  func(PeriodSummary) string
	strict bool // true: equal to a neighbor is a violation (<= / >=)
	early  func(neighbor string) error
	late   func(neighbor string) error
}

// dateOrderRule: ReadingDate must lie strictly between its neighbors'.
var dateOrderRule = orderRule{
	value:  func(p PeriodSummary) string { return p.ReadingDate },
	strict: true,
	early:  func(n string) error { return &PeriodDateTooEarlyError{Neighbor: n} },
	late:   func(n string) error { return &PeriodDateTooLateError{Neighbor: n} },
}

// monatOrderRule: Monat must not pass its neighbors' (Issue #86). Unlike
// ReadingDate, equal to a neighbor's Monat is fine (multiple Ablesungen may
// share an Abrechnungsmonat).
var monatOrderRule = orderRule{
	value: func(p PeriodSummary) string { return p.Monat },
	early: func(n string) error { return &PeriodMonatTooEarlyError{Neighbor: n} },
	late:  func(n string) error { return &PeriodMonatTooLateError{Neighbor: n} },
}

// check returns the error for v against the neighbors of the period at
// currentDate (excluding periodID), or nil.
func (r orderRule) check(all []PeriodSummary, periodID int64, currentDate, v string) error {
	prev, next, hasPrev, hasNext := neighborValues(all, periodID, currentDate, r.value)
	if hasPrev && (v < prev || (r.strict && v == prev)) {
		return r.early(prev)
	}
	if hasNext && (v > next || (r.strict && v == next)) {
		return r.late(next)
	}
	return nil
}

// checkReadingOrder validates in.ReadingDate and in.Monat against
// periodID's chronological neighbors before UpdatePeriod writes them. The
// neighbor bounds are computed around periodID's *current* (pre-edit) date,
// not the new one - a correction may only move a value within the gap it
// already occupies, not jump elsewhere and skip the check. The context is
// loaded once for both rules; the date is checked first. Only UpdatePeriod
// calls this, appending is checked by checkAppend.
func checkReadingOrder(db *sql.DB, periodID int64, in PeriodInput) error {
	currentDate, all, err := periodNeighborContext(db, periodID)
	if err != nil {
		return err
	}

	if err := dateOrderRule.check(all, periodID, currentDate, in.ReadingDate); err != nil {
		return err
	}
	if in.Monat == "" {
		// Teilstand (Ticket #128): an Abrechnungsmonat not entered yet is
		// not a chronological conflict with a neighboring period - skip
		// until it's actually entered.
		return nil
	}
	return monatOrderRule.check(all, periodID, currentDate, in.Monat)
}

// PeriodMonatTooEarlyError is returned by CreatePeriod, ImportPeriods and
// UpdatePeriod when an Abrechnungsmonat would lie before its chronological
// predecessor's.
type PeriodMonatTooEarlyError struct {
	Neighbor string // the predecessor's Monat
}

func (e *PeriodMonatTooEarlyError) Error() string {
	return fmt.Sprintf("monat must not be before the previous period's monat (%s)", e.Neighbor)
}

// PeriodMonatTooLateError is UpdatePeriod's mirror of
// PeriodMonatTooEarlyError for the chronological successor.
type PeriodMonatTooLateError struct {
	Neighbor string // the successor's Monat
}

func (e *PeriodMonatTooLateError) Error() string {
	return fmt.Sprintf("monat must not be after the next period's monat (%s)", e.Neighbor)
}
