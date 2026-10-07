package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// MeterKeys is the stable, ordered list of the meter keys every period must
// have a reading for. See Issue #6 / seed() for the full definitions.
var MeterKeys = []string{
	"strom_gesamt",
	"strom_wohnung2",
	"strom_waermepumpe",
	"strom_wallbox",
	"wasser_gesamt",
	"wasser_wohnung2",
	"wasser_warmwasseraufbereitung",
	"waerme_wohnung1",
	"waerme_wohnung2",
	"strom_einspeisung",
}

// Meter is one fixed meter (Zähler) with its display label and unit.
type Meter struct {
	Key   string
	Label string
	Unit  string
	// ApartmentID is the apartment a meter belongs to, 0 for a house-wide
	// meter.
	ApartmentID int64
}

// Meters returns the 9 meters ordered by id, the Anhang's source for the
// meter labels and units.
func Meters(db *sql.DB) ([]Meter, error) {
	rows, err := db.Query(`SELECT key, label, unit, COALESCE(apartment_id, 0) FROM meters ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query meters: %w", err)
	}
	defer rows.Close()

	var out []Meter
	for rows.Next() {
		var m Meter
		if err := rows.Scan(&m.Key, &m.Label, &m.Unit, &m.ApartmentID); err != nil {
			return nil, fmt.Errorf("scan meter: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
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
	// Strompreis/FrischwasserPreis/AbwasserPreis/EinspeisungPreis are nil
	// when not (yet) entered - a Teilstand (Ticket #128). CreatePeriod
	// persists nil as SQL NULL; UpdatePeriod leaves the existing stored
	// value untouched for a nil field, same "don't touch what wasn't
	// given" rule Readings/Personen below already followed.
	Strompreis        *float64
	FrischwasserPreis *float64
	AbwasserPreis     *float64
	EinspeisungPreis  *float64
	// Readings/Personen: a missing key means "nicht erfasst" (Teilstand,
	// Ticket #128) - CreatePeriod/UpdatePeriod simply don't write a row for
	// it, rather than requiring every MeterKeys/apartment id to be present.
	Readings map[string]float64 // meter key -> Zählerstand
	Personen map[int64]int64    // apartment id -> Personenzahl
}

// Float64 returns a pointer to v - a constructor for PeriodInput/
// LatestPeriod's optional price fields (Ticket #128), since Go has no
// address-of operator for a literal.
func Float64(v float64) *float64 { return &v }

// OrZero dereferences a Teilstand-fähiges Preis-Feld, treating "nicht
// erfasst" (nil) as 0 - for callers that don't (yet) distinguish "fehlt"
// from "0", such as a prefill or an export column (Ticket #128).
func OrZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// insertPeriodTx inserts one period with its meter readings and occupancy.
// Wohnungsgröße/Flurstücksgröße live on apartments and are edited separately on
// /stammdaten (Issue #61), not per period. Shared by CreatePeriod (one
// period, own transaction) and ImportPeriods (many periods, one shared
// transaction - Ticket #54).
func insertPeriodTx(tx *sql.Tx, in PeriodInput) (periodID int64, err error) {
	res, err := tx.Exec(
		`INSERT INTO periods (reading_date, monat, strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		in.ReadingDate, in.Monat, in.Strompreis, in.FrischwasserPreis, in.AbwasserPreis, in.EinspeisungPreis,
	)
	if err != nil {
		return 0, fmt.Errorf("insert period: %w", err)
	}
	periodID, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("period id: %w", err)
	}

	for _, key := range MeterKeys {
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

	for apartmentID, personen := range in.Personen {
		if _, err := tx.Exec(
			`INSERT INTO period_occupancy (period_id, apartment_id, personen) VALUES (?, ?, ?)`,
			periodID, apartmentID, personen,
		); err != nil {
			return 0, fmt.Errorf("insert occupancy for apartment %d: %w", apartmentID, err)
		}
	}

	return periodID, nil
}

// CreatePeriod inserts a new period in its own transaction.
func CreatePeriod(db *sql.DB, in PeriodInput) (periodID int64, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	periodID, err = insertPeriodTx(tx, in)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return periodID, nil
}

// ImportPeriods inserts every given period in one shared transaction - all
// or nothing, for the CSV bulk import (Ticket #54): a failure on any input
// rolls back every period from this call, not just the failing one.
func ImportPeriods(db *sql.DB, inputs []PeriodInput) (ids []int64, err error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	ids = make([]int64, 0, len(inputs))
	for _, in := range inputs {
		id, err := insertPeriodTx(tx, in)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return ids, nil
}

// PeriodDateTooEarlyError is returned by UpdatePeriod when in.ReadingDate
// would move periodID at or before its chronological predecessor's date.
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
// calls this; CreatePeriod leaves both unvalidated.
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

// PeriodMonatTooEarlyError is returned by UpdatePeriod when in.Monat would
// move periodID's Abrechnungsmonat before its chronological predecessor's.
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

// UpdatePeriod overwrites an existing period's fields, readings, and
// occupancy in place - no new row, no history of the previous values
// (Ticket #34: only the latest period is ever editable, in-place, no
// audit log). Costs aren't stored anywhere (calc.Verbrauchskosten reads
// live from the DB on every request), so overwriting here is all that's
// needed for the change to show up - nothing to invalidate. The neighbor-date
// invariant (see checkReadingOrder) is enforced here, not just by the web
// wizard, so any caller gets the same protection.
func UpdatePeriod(db *sql.DB, periodID int64, in PeriodInput) error {
	if err := checkReadingOrder(db, periodID, in); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		// Teilstand (Ticket #128): only overwrite monat/the 3 prices when
		// this round actually brings a value (Monat != '', Preis != NULL) -
		// a deliberately empty field must not silently delete a
		// previously-entered value. The same "only touch what was given"
		// rule now applies to Readings below (previously an error on a
		// missing meter) and already applied to Personen before (never a
		// mandatory coverage check).
		`UPDATE periods SET
		   reading_date = ?,
		   monat = COALESCE(NULLIF(?, ''), monat),
		   strompreis = COALESCE(?, strompreis),
		   frischwasser_preis = COALESCE(?, frischwasser_preis),
		   abwasser_preis = COALESCE(?, abwasser_preis),
		   einspeisung_preis = COALESCE(?, einspeisung_preis)
		 WHERE id = ?`,
		in.ReadingDate, in.Monat, in.Strompreis, in.FrischwasserPreis, in.AbwasserPreis, in.EinspeisungPreis, periodID,
	)
	if err != nil {
		return fmt.Errorf("update period: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("update period rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
	}

	for _, key := range MeterKeys {
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

	for apartmentID, personen := range in.Personen {
		if _, err := tx.Exec(
			`INSERT INTO period_occupancy (period_id, apartment_id, personen) VALUES (?, ?, ?)
			 ON CONFLICT(period_id, apartment_id) DO UPDATE SET personen = excluded.personen`,
			periodID, apartmentID, personen,
		); err != nil {
			return fmt.Errorf("update occupancy for apartment %d: %w", apartmentID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// LatestPeriod is a period together with its readings and occupancy, as
// shown on the "letzte Ablesung" view.
type LatestPeriod struct {
	ID          int64
	ReadingDate string
	Monat       string
	// Strompreis/FrischwasserPreis/AbwasserPreis/EinspeisungPreis are nil
	// when not (yet) entered - a Teilstand (Ticket #128).
	Strompreis          *float64
	FrischwasserPreis   *float64
	AbwasserPreis       *float64
	EinspeisungPreis    *float64
	Readings            map[string]float64
	PersonenByApartment map[int64]int64
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

// AllPeriodDetails returns every period with its full readings and
// occupancy, oldest first - the CSV export's data source (Ticket #53). 3
// batched queries total (periods, readings, occupancy), not one per period.
func AllPeriodDetails(db *sql.DB) ([]*LatestPeriod, error) {
	rows, err := db.Query(
		`SELECT id, reading_date, monat, strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis
		 FROM periods ORDER BY reading_date ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("query periods: %w", err)
	}
	defer rows.Close()

	byID := map[int64]*LatestPeriod{}
	var out []*LatestPeriod
	for rows.Next() {
		p := &LatestPeriod{
			Readings:            map[string]float64{},
			PersonenByApartment: map[int64]int64{},
		}
		if err := rows.Scan(&p.ID, &p.ReadingDate, &p.Monat, &p.Strompreis, &p.FrischwasserPreis, &p.AbwasserPreis, &p.EinspeisungPreis); err != nil {
			return nil, fmt.Errorf("scan period: %w", err)
		}
		byID[p.ID] = p
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	readingRows, err := db.Query(
		`SELECT r.period_id, m.key, r.zaehlerstand FROM meter_readings r JOIN meters m ON m.id = r.meter_id`,
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

	occRows, err := db.Query(`SELECT period_id, apartment_id, personen FROM period_occupancy`)
	if err != nil {
		return nil, fmt.Errorf("query occupancy: %w", err)
	}
	defer occRows.Close()
	for occRows.Next() {
		var periodID, apartmentID, personen int64
		if err := occRows.Scan(&periodID, &apartmentID, &personen); err != nil {
			return nil, fmt.Errorf("scan occupancy: %w", err)
		}
		if p, ok := byID[periodID]; ok {
			p.PersonenByApartment[apartmentID] = personen
		}
	}
	if err := occRows.Err(); err != nil {
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

// GetPeriodDetails returns the given period together with its readings and
// occupancy, or nil if it doesn't exist - the per-id generalization of
// GetLatestPeriod (Ticket #43/#44: viewing/editing an Ablesung isn't limited
// to the latest period anymore). Reuses AllPeriodDetails' single hydration
// path rather than its own queries - data volume here is tens to low
// hundreds of rows, so a full fetch-then-filter costs nothing measurable.
func GetPeriodDetails(db *sql.DB, id int64) (*LatestPeriod, error) {
	periods, err := AllPeriodDetails(db)
	if err != nil {
		return nil, err
	}
	for _, p := range periods {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, nil
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

// DeletePeriod removes a period together with its readings and occupancy in
// one transaction (Ticket #45). No soft-delete, no restriction on which
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
	if _, err := tx.Exec(`DELETE FROM period_occupancy WHERE period_id = ?`, periodID); err != nil {
		return fmt.Errorf("delete occupancy: %w", err)
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
// Teilstand (Ticket #128): complete means all 4 price columns are set,
// Monat isn't empty, every store.MeterKeys entry has a meter_readings row,
// and every apartment has a period_occupancy row. Purely derived - no
// dedicated "complete" flag, so there's no second source of truth.
func PeriodComplete(db *sql.DB, periodID int64) (bool, error) {
	var pricesSet int
	var monat string
	if err := db.QueryRow(
		`SELECT (strompreis IS NOT NULL AND frischwasser_preis IS NOT NULL AND abwasser_preis IS NOT NULL AND einspeisung_preis IS NOT NULL), monat
		 FROM periods WHERE id = ?`,
		periodID,
	).Scan(&pricesSet, &monat); err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("%w: period %d", ErrPeriodNotFound, periodID)
		}
		return false, fmt.Errorf("query period %d: %w", periodID, err)
	}
	if pricesSet == 0 || monat == "" {
		return false, nil
	}

	var readingCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM meter_readings WHERE period_id = ?`, periodID).Scan(&readingCount); err != nil {
		return false, fmt.Errorf("count readings for period %d: %w", periodID, err)
	}
	if readingCount < len(MeterKeys) {
		return false, nil
	}

	var apartmentCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apartments`).Scan(&apartmentCount); err != nil {
		return false, fmt.Errorf("count apartments: %w", err)
	}
	var occupancyCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM period_occupancy WHERE period_id = ?`, periodID).Scan(&occupancyCount); err != nil {
		return false, fmt.Errorf("count occupancy for period %d: %w", periodID, err)
	}
	return occupancyCount >= apartmentCount, nil
}

// PersonenByApartment returns the given period's occupancy (apartment id ->
// Personenzahl), as recorded at that period's Ablesung.
func PersonenByApartment(db *sql.DB, periodID int64) (map[int64]int64, error) {
	rows, err := db.Query(
		`SELECT apartment_id, personen FROM period_occupancy WHERE period_id = ?`,
		periodID,
	)
	if err != nil {
		return nil, fmt.Errorf("query occupancy: %w", err)
	}
	defer rows.Close()

	out := map[int64]int64{}
	for rows.Next() {
		var apartmentID, personen int64
		if err := rows.Scan(&apartmentID, &personen); err != nil {
			return nil, fmt.Errorf("scan occupancy: %w", err)
		}
		out[apartmentID] = personen
	}
	return out, rows.Err()
}
