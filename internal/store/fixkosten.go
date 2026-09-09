package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// JahrFromMonat parses a Fixkosten-Eingabe's Monat ("YYYY-MM-01") into its
// calendar year - shared by calc.Fixkosten and the web layer so both read
// the same "YYYY-MM-01" convention through one place.
func JahrFromMonat(monat string) (int, error) {
	t, err := time.Parse("2006-01-02", monat)
	if err != nil {
		return 0, fmt.Errorf("invalid monat %q: %w", monat, err)
	}
	return t.Year(), nil
}

// Logik values for fixkosten_werte.logik - the 4 allocation rules a
// Kostenposition can be split between Wohnung 1/2 by (Issue #60).
const (
	LogikWohneinheit = "wohneinheit"
	LogikFlurstueck  = "flurstueck"
	LogikQM          = "qm"
	LogikPersonen    = "personen"
)

// Typ values for fixkosten_werte.typ.
const (
	TypJaehrlich = "jaehrlich"
	TypMonatlich = "monatlich"
)

// Kostenposition is one of the 14 fixed cost positions (Issue #60) - id/key/
// label only, app-fixed structure like meters. Logik/Typ/Wert are per-
// Fixkosten-Eingabe data, see FixkostenPositionWert.
type Kostenposition struct {
	ID    int64
	Key   string
	Label string
}

// Kostenpositionen returns the 14 Kostenpositionen ordered by id.
func Kostenpositionen(db *sql.DB) ([]Kostenposition, error) {
	rows, err := db.Query(`SELECT id, key, label FROM kostenpositionen ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query kostenpositionen: %w", err)
	}
	defer rows.Close()

	var out []Kostenposition
	for rows.Next() {
		var kp Kostenposition
		if err := rows.Scan(&kp.ID, &kp.Key, &kp.Label); err != nil {
			return nil, fmt.Errorf("scan kostenposition: %w", err)
		}
		out = append(out, kp)
	}
	return out, rows.Err()
}

// FixkostenPositionWert is one Kostenposition's Logik/Typ/Wert for a single
// Fixkosten-Eingabe (Issue #105/#106/#107) - lives per Eingabe now, each
// Eingabe independent, instead of yearly in kostenpositionen_jahre. For
// Typ "jaehrlich" Wert is an annual total amount (divided by 12 for the
// monthly value, see calc.Fixkosten), for "monatlich" a direct monthly
// amount.
type FixkostenPositionWert struct {
	Logik string
	Typ   string
	Wert  float64
}

// FixkostenInput is one monthly Fixkosten-Eingabe, ready to be persisted.
type FixkostenInput struct {
	Monat    string                          // YYYY-MM-01, same convention as PeriodInput.ReadingDate
	Personen map[int64]int64                 // apartment id -> Personenzahl - own to Fixkosten, not period_occupancy (Issue #60 Story 8)
	Werte    map[int64]FixkostenPositionWert // kostenposition id -> Logik/Typ/Wert for this Eingabe
	Abschlag map[int64]float64               // apartment id -> Nebenkostenabschlag value - not a Kostenposition, covers Fixkosten AND consumption costs together, see nebenkosten_abschlaege
}

// ErrFixkostenEingabeNotFound is returned by UpdateFixkostenEingabe and
// DeleteFixkostenEingabe when the given id doesn't exist.
var ErrFixkostenEingabeNotFound = errors.New("fixkosten eingabe not found")

// insertFixkostenTx inserts one Fixkosten-Eingabe with its Werte/Personen.
// Shared by CreateFixkostenEingabe and (a future bulk-insert, should one
// ever be needed) so the write shape stays in one place.
func insertFixkostenTx(tx *sql.Tx, in FixkostenInput) (eingabeID int64, err error) {
	res, err := tx.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES (?)`, in.Monat)
	if err != nil {
		return 0, fmt.Errorf("insert fixkosten eingabe: %w", err)
	}
	eingabeID, err = res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("fixkosten eingabe id: %w", err)
	}

	for kostenpositionID, w := range in.Werte {
		if _, err := tx.Exec(
			`INSERT INTO fixkosten_werte (fixkosten_eingabe_id, kostenposition_id, wert, logik, typ) VALUES (?, ?, ?, ?, ?)`,
			eingabeID, kostenpositionID, w.Wert, w.Logik, w.Typ,
		); err != nil {
			return 0, fmt.Errorf("insert fixkosten wert for kostenposition %d: %w", kostenpositionID, err)
		}
	}

	for apartmentID, personen := range in.Personen {
		if _, err := tx.Exec(
			`INSERT INTO fixkosten_personen (fixkosten_eingabe_id, apartment_id, personen) VALUES (?, ?, ?)`,
			eingabeID, apartmentID, personen,
		); err != nil {
			return 0, fmt.Errorf("insert fixkosten personen for apartment %d: %w", apartmentID, err)
		}
	}

	for apartmentID, wert := range in.Abschlag {
		if _, err := tx.Exec(
			`INSERT INTO nebenkosten_abschlaege (fixkosten_eingabe_id, apartment_id, wert) VALUES (?, ?, ?)`,
			eingabeID, apartmentID, wert,
		); err != nil {
			return 0, fmt.Errorf("insert nebenkosten abschlag for apartment %d: %w", apartmentID, err)
		}
	}

	return eingabeID, nil
}

// CreateFixkostenEingabe inserts a new Fixkosten-Eingabe in its own
// transaction.
func CreateFixkostenEingabe(db *sql.DB, in FixkostenInput) (eingabeID int64, err error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	eingabeID, err = insertFixkostenTx(tx, in)
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit tx: %w", err)
	}
	return eingabeID, nil
}

// UpdateFixkostenEingabe overwrites an existing Fixkosten-Eingabe's Monat/
// Werte/Personen in place - no new row, no history (same "korrigieren"
// convention as UpdatePeriod). Unlike Ablesungen, Fixkosten-Monatswerte
// don't depend on a chronological Vorperiode (no Verbrauch/Diff involved),
// so there's no neighbor-date reordering constraint to enforce here.
func UpdateFixkostenEingabe(db *sql.DB, eingabeID int64, in FixkostenInput) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`UPDATE fixkosten_eingaben SET monat = ? WHERE id = ?`, in.Monat, eingabeID)
	if err != nil {
		return fmt.Errorf("update fixkosten eingabe: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("update fixkosten eingabe rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("%w: eingabe %d", ErrFixkostenEingabeNotFound, eingabeID)
	}

	for kostenpositionID, w := range in.Werte {
		if _, err := tx.Exec(
			`INSERT INTO fixkosten_werte (fixkosten_eingabe_id, kostenposition_id, wert, logik, typ) VALUES (?, ?, ?, ?, ?)
			 ON CONFLICT(fixkosten_eingabe_id, kostenposition_id) DO UPDATE SET wert = excluded.wert, logik = excluded.logik, typ = excluded.typ`,
			eingabeID, kostenpositionID, w.Wert, w.Logik, w.Typ,
		); err != nil {
			return fmt.Errorf("update fixkosten wert for kostenposition %d: %w", kostenpositionID, err)
		}
	}

	for apartmentID, personen := range in.Personen {
		if _, err := tx.Exec(
			`INSERT INTO fixkosten_personen (fixkosten_eingabe_id, apartment_id, personen) VALUES (?, ?, ?)
			 ON CONFLICT(fixkosten_eingabe_id, apartment_id) DO UPDATE SET personen = excluded.personen`,
			eingabeID, apartmentID, personen,
		); err != nil {
			return fmt.Errorf("update fixkosten personen for apartment %d: %w", apartmentID, err)
		}
	}

	for apartmentID, wert := range in.Abschlag {
		if _, err := tx.Exec(
			`INSERT INTO nebenkosten_abschlaege (fixkosten_eingabe_id, apartment_id, wert) VALUES (?, ?, ?)
			 ON CONFLICT(fixkosten_eingabe_id, apartment_id) DO UPDATE SET wert = excluded.wert`,
			eingabeID, apartmentID, wert,
		); err != nil {
			return fmt.Errorf("update nebenkosten abschlag for apartment %d: %w", apartmentID, err)
		}
	}

	return tx.Commit()
}

// DeleteFixkostenEingabe removes a Fixkosten-Eingabe together with its
// Werte/Personen in one transaction.
func DeleteFixkostenEingabe(db *sql.DB, eingabeID int64) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM fixkosten_werte WHERE fixkosten_eingabe_id = ?`, eingabeID); err != nil {
		return fmt.Errorf("delete fixkosten werte: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM fixkosten_personen WHERE fixkosten_eingabe_id = ?`, eingabeID); err != nil {
		return fmt.Errorf("delete fixkosten personen: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM nebenkosten_abschlaege WHERE fixkosten_eingabe_id = ?`, eingabeID); err != nil {
		return fmt.Errorf("delete nebenkosten abschlaege: %w", err)
	}

	res, err := tx.Exec(`DELETE FROM fixkosten_eingaben WHERE id = ?`, eingabeID)
	if err != nil {
		return fmt.Errorf("delete fixkosten eingabe: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("delete fixkosten eingabe rows affected: %w", err)
	} else if n == 0 {
		return fmt.Errorf("%w: eingabe %d", ErrFixkostenEingabeNotFound, eingabeID)
	}

	return tx.Commit()
}

// FixkostenEingabeSummary identifies one Fixkosten-Eingabe without its
// Werte/Personen, for the /fixkosten Übersicht.
type FixkostenEingabeSummary struct {
	ID    int64
	Monat string
}

// AllFixkostenEingaben returns every Fixkosten-Eingabe (newest first),
// without Werte/Personen.
func AllFixkostenEingaben(db *sql.DB) ([]FixkostenEingabeSummary, error) {
	rows, err := db.Query(`SELECT id, monat FROM fixkosten_eingaben ORDER BY monat DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query fixkosten eingaben: %w", err)
	}
	defer rows.Close()

	var out []FixkostenEingabeSummary
	for rows.Next() {
		var f FixkostenEingabeSummary
		if err := rows.Scan(&f.ID, &f.Monat); err != nil {
			return nil, fmt.Errorf("scan fixkosten eingabe: %w", err)
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// FixkostenEingabeDetails is one Fixkosten-Eingabe together with its
// Werte/Personen.
type FixkostenEingabeDetails struct {
	ID       int64
	Monat    string
	Personen map[int64]int64                 // apartment id -> Personenzahl
	Werte    map[int64]FixkostenPositionWert // kostenposition id -> Logik/Typ/Wert
	Abschlag map[int64]float64               // apartment id -> Nebenkostenabschlag-Wert
}

// GetFixkostenEingabeDetails returns the given Fixkosten-Eingabe with its
// Werte/Personen, or nil if it doesn't exist.
func GetFixkostenEingabeDetails(db *sql.DB, eingabeID int64) (*FixkostenEingabeDetails, error) {
	f := FixkostenEingabeDetails{
		Personen: map[int64]int64{},
		Werte:    map[int64]FixkostenPositionWert{},
		Abschlag: map[int64]float64{},
	}
	if err := db.QueryRow(`SELECT id, monat FROM fixkosten_eingaben WHERE id = ?`, eingabeID).Scan(&f.ID, &f.Monat); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query fixkosten eingabe %d: %w", eingabeID, err)
	}

	werteRows, err := db.Query(`SELECT kostenposition_id, wert, logik, typ FROM fixkosten_werte WHERE fixkosten_eingabe_id = ?`, eingabeID)
	if err != nil {
		return nil, fmt.Errorf("query fixkosten werte: %w", err)
	}
	defer werteRows.Close()
	for werteRows.Next() {
		var kostenpositionID int64
		var w FixkostenPositionWert
		if err := werteRows.Scan(&kostenpositionID, &w.Wert, &w.Logik, &w.Typ); err != nil {
			return nil, fmt.Errorf("scan fixkosten wert: %w", err)
		}
		f.Werte[kostenpositionID] = w
	}
	if err := werteRows.Err(); err != nil {
		return nil, err
	}

	personenRows, err := db.Query(`SELECT apartment_id, personen FROM fixkosten_personen WHERE fixkosten_eingabe_id = ?`, eingabeID)
	if err != nil {
		return nil, fmt.Errorf("query fixkosten personen: %w", err)
	}
	defer personenRows.Close()
	for personenRows.Next() {
		var apartmentID, personen int64
		if err := personenRows.Scan(&apartmentID, &personen); err != nil {
			return nil, fmt.Errorf("scan fixkosten personen: %w", err)
		}
		f.Personen[apartmentID] = personen
	}
	if err := personenRows.Err(); err != nil {
		return nil, err
	}

	abschlagRows, err := db.Query(`SELECT apartment_id, wert FROM nebenkosten_abschlaege WHERE fixkosten_eingabe_id = ?`, eingabeID)
	if err != nil {
		return nil, fmt.Errorf("query nebenkosten abschlaege: %w", err)
	}
	defer abschlagRows.Close()
	for abschlagRows.Next() {
		var apartmentID int64
		var wert float64
		if err := abschlagRows.Scan(&apartmentID, &wert); err != nil {
			return nil, fmt.Errorf("scan nebenkosten abschlag: %w", err)
		}
		f.Abschlag[apartmentID] = wert
	}
	if err := abschlagRows.Err(); err != nil {
		return nil, err
	}

	return &f, nil
}

// AllAbschlaege returns every recorded Nebenkostenabschlag-Wert, keyed by
// fixkosten_eingabe_id then apartment_id - alleFixkostenKosten's source for
// attaching Abschlag-Werte to each Monat's Fixkosten-Ergebnis without a
// per-Eingabe query.
func AllAbschlaege(db *sql.DB) (map[int64]map[int64]float64, error) {
	rows, err := db.Query(`SELECT fixkosten_eingabe_id, apartment_id, wert FROM nebenkosten_abschlaege`)
	if err != nil {
		return nil, fmt.Errorf("query nebenkosten abschlaege: %w", err)
	}
	defer rows.Close()

	out := map[int64]map[int64]float64{}
	for rows.Next() {
		var eingabeID, apartmentID int64
		var wert float64
		if err := rows.Scan(&eingabeID, &apartmentID, &wert); err != nil {
			return nil, fmt.Errorf("scan nebenkosten abschlag: %w", err)
		}
		if out[eingabeID] == nil {
			out[eingabeID] = map[int64]float64{}
		}
		out[eingabeID][apartmentID] = wert
	}
	return out, rows.Err()
}

// GetLatestFixkostenEingabe returns the most recently dated Fixkosten-
// Eingabe, or nil if none exist yet - the "neu erfassen" form's prefill
// source (Issue #60 Story 2/9).
func GetLatestFixkostenEingabe(db *sql.DB) (*FixkostenEingabeDetails, error) {
	var id int64
	err := db.QueryRow(`SELECT id FROM fixkosten_eingaben ORDER BY monat DESC, id DESC LIMIT 1`).Scan(&id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("query latest fixkosten eingabe id: %w", err)
	}
	return GetFixkostenEingabeDetails(db, id)
}
