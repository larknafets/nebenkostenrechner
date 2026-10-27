// Package store owns the SQLite schema, migrations, and seed data for the
// Nebenkostenrechner. See https://github.com/larknafets/nebenkostenrechner/issues/6
// for the schema decision this package implements.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Open creates the database file's parent directory if needed, opens the
// SQLite database at path, saves it if a migration is due (see
// sichereVorMigration), applies the schema, runs the migration steps and seeds
// the fixed master data (apartments, meters) if it isn't present yet.
func Open(path string) (*sql.DB, error) {
	return open(path, true)
}

// OpenDemo opens the demo database like Open, without the save before a
// migration: it holds no data of its own, it is reset on every demo login.
func OpenDemo(path string) (*sql.DB, error) {
	return open(path, false)
}

func open(path string, sichern bool) (*sql.DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Before the schema is applied: even CREATE TABLE IF NOT EXISTS would
	// already change a database that is about to be migrated.
	var sicherung string
	if sichern {
		var err error
		if sicherung, err = sichereVorMigration(db, path); err != nil {
			db.Close()
			return nil, err
		}
	}

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		if sicherung != "" {
			verwirfDieseSicherung(path, sicherung)
		}
		return nil, err
	}
	if sichern {
		raeumeSicherungenAuf(path)
	}

	return db, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// hasColumn reports whether table has a column named column.
func hasColumn(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, fmt.Errorf("inspect %s columns: %w", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, fmt.Errorf("scan %s column: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

type apartmentSeed struct {
	id     int64
	name   string
	qm     float64
	status string
}

func apartmentID(id int64) *int64 { return &id }

// KostenpositionDefault is one of the fixed Kostenpositionen (Issue #60)
// - id/key/label are app-fixed structure, seeded like meters. Logik/Typ are
// only the starting values for the very first Fixkosten-Eingabe ever
// created (Issue #105/#108) - every Eingabe afterwards is prefilled from
// the previous one instead, not reseeded from here.
type KostenpositionDefault struct {
	ID    int64
	Key   string
	Label string
	Logik string
	Typ   string
	// Umlagefaehig is the starting value of the Umlagefähig flag (Issue
	// #163) - per the research on BetrKV § 2 and the agreed Grundgebühr
	// Strom exception. Afterwards it is a user-edited Stammdaten value.
	Umlagefaehig bool
}

// KostenpositionDefaults is the fixed list of the Kostenpositionen. The
// slice order is the starting display order and the fixed column order of the
// Fixkosten CSV; afterwards the user orders the display on /stammdaten
// (kostenpositionen.sort). A position added later goes to the end.
var KostenpositionDefaults = []KostenpositionDefault{
	{ID: 1, Key: "grundsteuer", Label: "Grundsteuer", Logik: LogikQM, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 2, Key: "gebaeudevers", Label: "Wohngebäudeversicherung", Logik: LogikQM, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 3, Key: "deich_grund", Label: "Deichbeitrag Grund und Boden", Logik: LogikFlurstueck, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 4, Key: "deich_bau", Label: "Deichbeitrag Bauliche Anlagen", Logik: LogikFlurstueck, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 5, Key: "kreisverband", Label: "Kreisverband Wesermarsch der Wasser- und Bodenverbände", Logik: LogikFlurstueck, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 6, Key: "abfall_haushalt", Label: "Abfallwirtschaft Grundgebühr Haushalt", Logik: LogikWohneinheit, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 7, Key: "abfall_personen", Label: "Abfallwirtschaft Grundgebühr Personen", Logik: LogikPersonen, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 8, Key: "abfall_biomuell", Label: "Abfallwirtschaft Biomüll", Logik: LogikWohneinheit, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 9, Key: "abfall_restmuell", Label: "Abfallwirtschaft Restmüll", Logik: LogikWohneinheit, Typ: TypJaehrlich, Umlagefaehig: true},
	{ID: 10, Key: "strom_grundpreis", Label: "Grundgebühr Strom", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: true},
	{ID: 11, Key: "trinkwasser", Label: "Grundgebühr Trinkwasser", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: true},
	{ID: 12, Key: "abwasser", Label: "Grundgebühr Abwasser", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: true},
	{ID: 13, Key: "internet", Label: "Grundgebühr Internet", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: false},
	{ID: 14, Key: "wp_wartung", Label: "Wartungskosten Wärmepumpe", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: true},
	{ID: 17, Key: "pv_wartung", Label: "Wartungskosten PV-Anlage", Logik: LogikWohnung1, Typ: TypJaehrlich, Umlagefaehig: false},
	{ID: 15, Key: "streaming", Label: "Streaming-Dienste", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: false},
	{ID: 16, Key: "sonstige", Label: "Sonstige Kosten", Logik: LogikWohneinheit, Typ: TypMonatlich, Umlagefaehig: false},
}

// seed inserts the fixed master data for the 2 apartments and the meters (meterDefs) if
// they're not already present. Keyed on `apartments.id` / `meters.key` so
// it's safe to run on every startup.
func seed(db *sql.DB) error {
	// qm (and flurstueck_groesse, via the column DEFAULT) start at 0 (Ticket
	// #38) - unlike the apartment id/name, Wohnungsgröße/Flurstücksgröße are
	// user data, not app-fixed structure, so they aren't hardcoded here.
	// Both are live columns, edited on /stammdaten (Issue #61), not seeded
	// with a real value.
	apartments := []apartmentSeed{
		{id: 1, name: "Wohnung 1", qm: 0, status: StatusEigennutzung},
		{id: 2, name: "Wohnung 2", qm: 0, status: StatusVermietet},
	}
	for _, a := range apartments {
		// status is only set on the INSERT: it is a user-edited Stammdaten
		// value (Issue #164), so a restart must never reset it.
		if _, err := db.Exec(
			`INSERT INTO apartments (id, name, qm, status) VALUES (?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			a.id, a.name, a.qm, a.status,
		); err != nil {
			return fmt.Errorf("seed apartment %q: %w", a.name, err)
		}
	}

	if err := seedMeters(db); err != nil {
		return err
	}

	// kostenpositionen (Issue #60) - only id/key/label are fixed structure;
	// Logik/Typ/Jahreswert are user-editable per year in
	// kostenpositionen_jahre, not seeded here. label is kept in sync on
	// every startup (DO UPDATE, not DO NOTHING) - there's no UI to edit it,
	// so it's meant to always match KostenpositionDefaults, unlike key
	// (immutable, referenced by id elsewhere) - Issue #71 renamed 2 labels
	// in code but an already-seeded row silently kept the old text until
	// this upsert.
	for _, kp := range KostenpositionDefaults {
		// umlagefaehig and sort are only set on the INSERT: both are user-
		// edited Stammdaten values (Issue #163, position order), so the DO
		// UPDATE must never reset them. A new row goes to the end of the order.
		if _, err := db.Exec(
			`INSERT INTO kostenpositionen (id, key, label, umlagefaehig, sort)
			 VALUES (?, ?, ?, ?, (SELECT COALESCE(MAX(sort), 0) + 1 FROM kostenpositionen))
			 ON CONFLICT(id) DO UPDATE SET label = excluded.label`,
			kp.ID, kp.Key, kp.Label, boolToInt(kp.Umlagefaehig),
		); err != nil {
			return fmt.Errorf("seed kostenposition %q: %w", kp.Key, err)
		}
	}

	// A position added after an Eingabe was entered has no Wert row there.
	// Give those Eingaben the starting Logik/Typ with Wert 0, so the form,
	// the CSV export and a re-import see the same complete set of positions.
	for _, kp := range KostenpositionDefaults {
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO fixkosten_werte (fixkosten_eingabe_id, kostenposition_id, wert, logik, typ)
			 SELECT id, ?, 0, ?, ? FROM fixkosten_eingaben`,
			kp.ID, kp.Logik, kp.Typ,
		); err != nil {
			return fmt.Errorf("seed fixkosten werte for kostenposition %q: %w", kp.Key, err)
		}
	}

	// haus (Issue #163): the single house-wide Stammdaten row, with the
	// column defaults as starting values.
	if _, err := db.Exec(`INSERT INTO haus (id) VALUES (1) ON CONFLICT(id) DO NOTHING`); err != nil {
		return fmt.Errorf("seed haus: %w", err)
	}

	return nil
}
