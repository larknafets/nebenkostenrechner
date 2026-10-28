package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Verbrauch is the consumption of one meter: its reading in current minus
// the one in previous (the Vorperiode's). ok is false if either reading is
// missing ("nicht berechenbar", never computed against 0) - always for a
// meter without a stand, whether required or an optional Einzelzähler. The
// one place that forms this difference.
func Verbrauch(current, previous map[string]float64, key string) (verbrauch float64, ok bool) {
	cur, okCur := current[key]
	prev, okPrev := previous[key]
	if !okCur || !okPrev {
		return 0, false
	}
	return cur - prev, true
}

// inTx runs fn in one transaction, committing only if fn succeeds.
func inTx(db *sql.DB, fn func(tx *sql.Tx) error) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// PeriodInput is one monthly reading, ready to be persisted.
type PeriodInput struct {
	ReadingDate string // YYYY-MM-DD
	// Monat is the Abrechnungsmonat-Label (Issue #86), "YYYY-MM-01" - an
	// empty string means "nicht erfasst" (Teilstand, Ticket #128), the
	// column's own DEFAULT ''.
	Monat string
	// Readings: a missing key means "nicht erfasst" (Teilstand, Ticket #128)
	// - CreatePeriod/UpdatePeriod simply don't write a row for it, rather
	// than requiring every MeterKeys entry to be present. Prices, switches
	// and Personenzahl belong to the Fixkosten-Eingabe (#359, #368).
	Readings map[string]float64 // meter key -> Zählerstand
}

// Validate checks the field rules of one reading, pure and without the
// database: the reading date (format YYYY-MM-DD), the Abrechnungsmonat (same
// format, empty is allowed: Teilstand, #128). It returns the first violation as
// *EingabeFehler. Meter values are not part of it (negative or outlying meter values are warnings), and neither
// are the Ablesungsreihe rules (reihe.go), which need the database.
func (in PeriodInput) Validate() error {
	if _, err := time.Parse("2006-01-02", in.ReadingDate); err != nil {
		return &EingabeFehler{Feld: FeldReadingDate, Wert: in.ReadingDate}
	}
	if in.Monat != "" {
		if _, err := time.Parse("2006-01-02", in.Monat); err != nil {
			return &EingabeFehler{Feld: FeldMonat, Wert: in.Monat}
		}
	}
	return nil
}

// Float64 returns a pointer to v - a constructor for the optional price
// fields of FixkostenInput (Ticket #128), since Go has no address-of operator
// for a literal.
func Float64(v float64) *float64 { return &v }

// insertPeriodTx inserts one period with its meter readings.
// Wohnungsgröße/Flurstücksgröße live on apartments and are edited separately on
// /stammdaten (Issue #61), not per period. Shared by CreatePeriod (one
// period, own transaction) and ImportPeriods (many periods, one shared
// transaction - Ticket #54). It does not validate: PeriodInput.Validate runs
// in CreatePeriod, ImportPeriods and UpdatePeriod before the insert.
func insertPeriodTx(tx *sql.Tx, in PeriodInput) (periodID int64, err error) {
	res, err := tx.Exec(
		insertPeriodSQL(),
		in.ReadingDate, in.Monat,
	)
	if err != nil {
		return 0, fmt.Errorf("insert period: %w", err)
	}
	periodID, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("period id: %w", err)
	}

	for _, key := range allMeterKeys() {
		value, ok := in.Readings[key]
		if !ok {
			// Teilstand (Ticket #128): no meter reading for this meter -
			// simply don't create a row, instead of raising an error.
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO meter_readings (period_id, meter_id, zaehlerstand)
			 SELECT ?, id, ? FROM meters WHERE key = ?`,
			periodID, value, key,
		); err != nil {
			return 0, fmt.Errorf("insert reading for %q: %w", key, err)
		}
	}

	return periodID, nil
}

// CreatePeriod appends a new period to the Ablesungsreihe in its own
// transaction. The series rules (see checkAppend) are enforced here, not
// just by the web wizard, so any caller gets the same protection.
func CreatePeriod(db *sql.DB, in PeriodInput) (periodID int64, err error) {
	if err := in.Validate(); err != nil {
		return 0, err
	}
	err = inTx(db, func(tx *sql.Tx) error {
		if err := checkAppend(tx, in); err != nil {
			return err
		}
		periodID, err = insertPeriodTx(tx, in)
		return err
	})
	if err != nil {
		return 0, err
	}
	return periodID, nil
}

// ImportPeriods appends every given period in one shared transaction - all
// or nothing, for the CSV bulk import (Ticket #54): a failure on any input
// rolls back every period from this call, not just the failing one. The
// inputs must be in reading-date order; each one passes the same series
// rules as CreatePeriod (Validate, then checkAppend), and a violation names
// the position and the reading date of the offending input (ImportRowError).
func ImportPeriods(db *sql.DB, inputs []PeriodInput) (ids []int64, err error) {
	err = inTx(db, func(tx *sql.Tx) error {
		ids = make([]int64, 0, len(inputs))
		for i, in := range inputs {
			if err := in.Validate(); err != nil {
				return &ImportRowError{Index: i, ReadingDate: in.ReadingDate, Err: err}
			}
			if err := checkAppend(tx, in); err != nil {
				return &ImportRowError{Index: i, ReadingDate: in.ReadingDate, Err: err}
			}
			id, err := insertPeriodTx(tx, in)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// UpdatePeriod overwrites an existing period's fields and readings in place - no new row, no history of the previous values
// (Ticket #34: only the latest period is ever editable, in-place, no
// audit log). Costs aren't stored anywhere (calc.Verbrauchskosten reads
// live from the DB on every request), so overwriting here is all that's
// needed for the change to show up - nothing to invalidate. The neighbor-date
// invariant (see checkReadingOrder) is enforced here, not just by the web
// wizard, so any caller gets the same protection.
func UpdatePeriod(db *sql.DB, periodID int64, in PeriodInput) error {
	if err := in.Validate(); err != nil {
		return err
	}
	if err := checkReadingOrder(db, periodID, in); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		// Teilstand (Ticket #128): only overwrite monat when this round
		// brings a value (Monat != ''), a deliberately empty field must not
		// silently delete a previously-entered value. The same "only touch
		// what was given" rule applies to Readings below.
		updatePeriodSQL(),
		in.ReadingDate, in.Monat, periodID,
	)
	if err != nil {
		return fmt.Errorf("update period: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("update period rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
	}

	for _, key := range allMeterKeys() {
		value, ok := in.Readings[key]
		if !ok {
			// Teilstand (Ticket #128): no new value for this meter in this
			// round - leave the existing meter reading untouched.
			continue
		}
		if _, err := tx.Exec(
			`INSERT INTO meter_readings (period_id, meter_id, zaehlerstand)
			 SELECT ?, id, ? FROM meters WHERE key = ?
			 ON CONFLICT(period_id, meter_id) DO UPDATE SET zaehlerstand = excluded.zaehlerstand`,
			periodID, value, key,
		); err != nil {
			return fmt.Errorf("update reading for %q: %w", key, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// LatestPeriod is a period together with its readings, as shown on the
// "letzte Ablesung" view.
type LatestPeriod struct {
	ID          int64
	ReadingDate string
	Monat       string
	Readings    map[string]float64
}

// PeriodReadings is one period's per-meter Zählerstand, as used for the
// Ausreißer-Warnung baseline (Ticket #13).
type PeriodReadings struct {
	ID          int64
	ReadingDate string
	Readings    map[string]float64
}

// idDate is a period's id/reading_date pair, ordered a query returns them
// in - the input periodReadingsFor turns into full PeriodReadings.
type idDate struct {
	id   int64
	date string
}

// periodReadingsFor fetches each id's readings and zips them with the
// already-known dates - the shared tail of RecentPeriodReadings and
// PeriodReadingsBefore, which differ only in how they select the ids.
func periodReadingsFor(db *sql.DB, ids []idDate) ([]PeriodReadings, error) {
	out := make([]PeriodReadings, 0, len(ids))
	for _, d := range ids {
		readings, err := readingsByMeterKey(db, d.id)
		if err != nil {
			return nil, fmt.Errorf("readings for period %d: %w", d.id, err)
		}
		out = append(out, PeriodReadings{ID: d.id, ReadingDate: d.date, Readings: readings})
	}
	return out, nil
}

// RecentPeriodReadings returns the most recent `limit` periods (newest
// first) together with their per-meter Zählerstand.
func RecentPeriodReadings(db *sql.DB, limit int) ([]PeriodReadings, error) {
	rows, err := db.Query(
		`SELECT id, reading_date FROM periods ORDER BY reading_date DESC, id DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query recent periods: %w", err)
	}
	defer rows.Close()

	var ids []idDate
	for rows.Next() {
		var d idDate
		if err := rows.Scan(&d.id, &d.date); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		ids = append(ids, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return periodReadingsFor(db, ids)
}

// PeriodSummary identifies one period without its readings, for views that
// only need to iterate periods (e.g. the Dashboard Verlauf, Ticket #19).
type PeriodSummary struct {
	ID          int64
	ReadingDate string
	Monat       string
}

// AllPeriods returns every period (newest first), without readings.
func AllPeriods(db *sql.DB) ([]PeriodSummary, error) {
	rows, err := db.Query(`SELECT id, reading_date, monat FROM periods ORDER BY reading_date DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query periods: %w", err)
	}
	defer rows.Close()

	var out []PeriodSummary
	for rows.Next() {
		var p PeriodSummary
		if err := rows.Scan(&p.ID, &p.ReadingDate, &p.Monat); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// AllPeriodDetails returns every period with its full readings, oldest first
// - the CSV export's data source (Ticket #53). 2 batched queries total
// (periods, readings), not one per period.
func AllPeriodDetails(db *sql.DB) ([]*LatestPeriod, error) {
	return loadPeriodDetails(db, 0)
}

// loadPeriodDetails is the single hydration path of AllPeriodDetails (id 0)
// and GetPeriodDetails (one id): the same two queries, the latter filtered
// to the one period.
func loadPeriodDetails(q querier, id int64) ([]*LatestPeriod, error) {
	periodWhere, childWhere, args := "", "", []any{}
	if id != 0 {
		periodWhere, childWhere, args = "WHERE id = ?", "WHERE r.period_id = ?", []any{id}
	}
	rows, err := q.Query(selectPeriodSQL(periodWhere), args...)
	if err != nil {
		return nil, fmt.Errorf("query periods: %w", err)
	}
	defer rows.Close()

	byID := map[int64]*LatestPeriod{}
	var out []*LatestPeriod
	for rows.Next() {
		p, err := scanPeriodRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		byID[p.ID] = p
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	readingRows, err := q.Query(
		`SELECT r.period_id, m.key, r.zaehlerstand FROM meter_readings r JOIN meters m ON m.id = r.meter_id `+childWhere, args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query readings: %w", err)
	}
	defer readingRows.Close()
	for readingRows.Next() {
		var periodID int64
		var key string
		var value float64
		if err := readingRows.Scan(&periodID, &key, &value); err != nil {
			return nil, fmt.Errorf("scan reading: %w", err)
		}
		if p, ok := byID[periodID]; ok {
			p.Readings[key] = value
		}
	}
	if err := readingRows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

// GetLatestPeriod returns the most recently dated period, or nil if none
// exist yet.
func GetLatestPeriod(db *sql.DB) (*LatestPeriod, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM periods ORDER BY reading_date DESC, id DESC LIMIT 1`).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest period id: %w", err)
	}
	return GetPeriodDetails(db, id)
}

// GetPeriodDetails returns the given period together with its readings, or nil if it doesn't exist - the per-id generalization of
// GetLatestPeriod (Ticket #43/#44: viewing/editing an Ablesung isn't limited
// to the latest period anymore). Uses AllPeriodDetails' hydration path,
// filtered to this one period.
func GetPeriodDetails(db *sql.DB, id int64) (*LatestPeriod, error) {
	periods, err := loadPeriodDetails(db, id)
	if err != nil || len(periods) == 0 {
		return nil, err
	}
	return periods[0], nil
}

// PeriodReadingsBefore returns up to `limit` periods chronologically before
// the given periodID (newest of those first), together with their per-meter
// Zählerstand - the Ausreißer-Warnung/Vorperiode-Vergleich baseline for
// editing an arbitrary (not necessarily latest) period (Ticket #44).
func PeriodReadingsBefore(db *sql.DB, periodID int64, limit int) ([]PeriodReadings, error) {
	target, err := GetPeriodByID(db, periodID)
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(
		`SELECT id, reading_date FROM periods
		 WHERE reading_date < ? OR (reading_date = ? AND id < ?)
		 ORDER BY reading_date DESC, id DESC LIMIT ?`,
		target.ReadingDate, target.ReadingDate, target.ID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query periods before %d: %w", periodID, err)
	}
	defer rows.Close()

	var ids []idDate
	for rows.Next() {
		var d idDate
		if err := rows.Scan(&d.id, &d.date); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		ids = append(ids, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return periodReadingsFor(db, ids)
}

// ErrPeriodNotFound is returned by UpdatePeriod and DeletePeriod when the
// given periodID doesn't exist - callers use it to tell "nothing to do"
// apart from a genuine storage error (e.g. to answer with 404 instead of
// 500).
var ErrPeriodNotFound = errors.New("period not found")

// DeletePeriod removes a period together with its readings in one transaction (Ticket #45). No soft-delete, no restriction on which
// period can be deleted - Verbrauch/Kosten are computed live from the DB
// (see internal/store/verbrauch.go), so a neighboring period's consumption
// simply recomputes against its new adjacent period on the next read.
func DeletePeriod(db *sql.DB, periodID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM meter_readings WHERE period_id = ?`, periodID); err != nil {
		return fmt.Errorf("delete readings: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM periods WHERE id = ?`, periodID)
	if err != nil {
		return fmt.Errorf("delete period: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete period rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
	}

	return tx.Commit()
}

// PeriodComplete reports whether periodID is a fully entered Ablesung or a
// Teilstand (Ticket #128): the answer of LatestPeriod.Teilstand, the one
// place that knows what "complete" means, for callers that only hold a bare
// periodID (e.g. teilstandAnspruch). Purely derived - no
// dedicated "complete" flag, so there's no second source of truth.
func PeriodComplete(db *sql.DB, periodID int64) (bool, error) {
	return periodComplete(db, periodID)
}

// periodComplete is PeriodComplete against a *sql.DB or a *sql.Tx, so the
// Ablesungsreihe rules can ask inside the transaction that is about to write.
func periodComplete(q querier, periodID int64) (bool, error) {
	periods, err := loadPeriodDetails(q, periodID)
	if err != nil {
		return false, fmt.Errorf("period %d: %w", periodID, err)
	}
	if len(periods) == 0 {
		return false, fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
	}
	return !periods[0].Teilstand().IstTeilstand, nil
}
