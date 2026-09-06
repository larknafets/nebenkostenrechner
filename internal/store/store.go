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
// SQLite database at path, applies the schema, and seeds the fixed master
// data (apartments, meters) if it isn't present yet.
func Open(path string) (*sql.DB, error) {
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

	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}

	// CREATE TABLE IF NOT EXISTS above only creates periods on a brand-new
	// database - a pre-existing one (from before Issue #27) needs this
	// column added explicitly. SQLite has no "ADD COLUMN IF NOT EXISTS", so
	// check first.
	if err := ensurePeriodsHeizungGewichtungColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate heizung_waerme_gewichtung column: %w", err)
	}

	if err := ensurePeriodsEinspeisungPreisColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate einspeisung_preis column: %w", err)
	}

	if err := ensureApartmentsFlurstueckGroesseColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate flurstueck_groesse column: %w", err)
	}

	if err := ensurePeriodsMonatColumn(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate monat column: %w", err)
	}

	if err := seed(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed master data: %w", err)
	}

	if err := ensureFixkostenWerteLogikTypColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate fixkosten_werte logik/typ columns: %w", err)
	}

	return db, nil
}

// ensurePeriodsHeizungGewichtungColumn adds the heizung_waerme_gewichtung
// column to an existing periods table that predates it (Issue #27),
// defaulting existing rows to 0.7 (the previously hardcoded 70/30 split).
func ensurePeriodsHeizungGewichtungColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(periods)`)
	if err != nil {
		return fmt.Errorf("inspect periods columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan periods column: %w", err)
		}
		if name == "heizung_waerme_gewichtung" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE periods ADD COLUMN heizung_waerme_gewichtung REAL NOT NULL DEFAULT 0.7`)
	return err
}

// ensurePeriodsEinspeisungPreisColumn adds the einspeisung_preis column to
// an existing periods table that predates it (Issue #47), defaulting
// existing rows to 0 - historical periods simply show no Einspeisevergütung
// until corrected.
func ensurePeriodsEinspeisungPreisColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(periods)`)
	if err != nil {
		return fmt.Errorf("inspect periods columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan periods column: %w", err)
		}
		if name == "einspeisung_preis" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE periods ADD COLUMN einspeisung_preis REAL NOT NULL DEFAULT 0`)
	return err
}

// ensureApartmentsFlurstueckGroesseColumn adds the flurstueck_groesse column
// to an existing apartments table that predates it (Issue #61), defaulting
// existing rows to 0 - additive, the existing qm value is untouched.
func ensureApartmentsFlurstueckGroesseColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(apartments)`)
	if err != nil {
		return fmt.Errorf("inspect apartments columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan apartments column: %w", err)
		}
		if name == "flurstueck_groesse" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	_, err = db.Exec(`ALTER TABLE apartments ADD COLUMN flurstueck_groesse REAL NOT NULL DEFAULT 0`)
	return err
}

// ensurePeriodsMonatColumn adds the monat column to an existing periods
// table that predates it (Issue #86), backfilling every pre-existing row's
// Abrechnungsmonat from its own reading_date (YYYY-MM-01) - unlike the
// static-default migrations above, this needs a computed value per row, so
// the backfill runs once, right here, in the same call that adds the
// column. A later re-run finds the column already present and returns
// before touching any row, so a manual monat override (via UpdatePeriod)
// is never clobbered.
func ensurePeriodsMonatColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(periods)`)
	if err != nil {
		return fmt.Errorf("inspect periods columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan periods column: %w", err)
		}
		if name == "monat" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := db.Exec(`ALTER TABLE periods ADD COLUMN monat TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE periods SET monat = substr(reading_date, 1, 7) || '-01' WHERE monat = ''`)
	return err
}

// ensureFixkostenWerteLogikTypColumns adds the logik/typ columns to an
// existing fixkosten_werte table that predates them (Issue #106/#105:
// Logik/Typ/Wert je Kostenposition wandern von den jahresweisen Stammdaten
// vollständig in die Fixkosten-Eingabe). Backfills every pre-existing
// Fixkosten-Eingabe's 14 Kostenpositionen from kostenpositionen_jahre in the
// same call that adds the columns, same convention as
// ensurePeriodsMonatColumn's reading_date -> monat backfill. A later re-run
// finds the columns already present and returns before touching any row.
func ensureFixkostenWerteLogikTypColumns(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(fixkosten_werte)`)
	if err != nil {
		return fmt.Errorf("inspect fixkosten_werte columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan fixkosten_werte column: %w", err)
		}
		if name == "logik" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := db.Exec(`ALTER TABLE fixkosten_werte ADD COLUMN logik TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE fixkosten_werte ADD COLUMN typ TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}

	return backfillFixkostenWerteLogikTyp(db)
}

// backfillFixkostenWerteLogikTyp fills every existing Fixkosten-Eingabe's 14
// Kostenpositionen with the Logik/Typ/Wert that applied to it at its own
// Monat, read from kostenpositionen_jahre (about to be dropped in a later
// ticket once nothing reads it anymore) - exactly monatswertFuer's existing
// jährlich/monatlich/Fallback rules (internal/calc/fixkosten.go), run once
// here instead of on every Berechnung. A Kostenposition with no
// kostenpositionen_jahre row for that Jahr (only possible for an Eingabe
// whose Jahr was never "angelegt", which calc.Fixkosten already refused to
// compute) falls back to KostenpositionDefaults so it still gets a sane
// Logik/Typ instead of staying empty.
func backfillFixkostenWerteLogikTyp(db *sql.DB) error {
	eingabeRows, err := db.Query(`SELECT id, monat FROM fixkosten_eingaben`)
	if err != nil {
		return fmt.Errorf("query fixkosten eingaben: %w", err)
	}
	type eingabe struct {
		id    int64
		monat string
	}
	var eingaben []eingabe
	for eingabeRows.Next() {
		var e eingabe
		if err := eingabeRows.Scan(&e.id, &e.monat); err != nil {
			eingabeRows.Close()
			return fmt.Errorf("scan fixkosten eingabe: %w", err)
		}
		eingaben = append(eingaben, e)
	}
	if err := eingabeRows.Err(); err != nil {
		eingabeRows.Close()
		return err
	}
	eingabeRows.Close()

	if len(eingaben) == 0 {
		return nil
	}

	kostenpositionen, err := Kostenpositionen(db)
	if err != nil {
		return fmt.Errorf("kostenpositionen: %w", err)
	}
	defaultByID := map[int64]KostenpositionDefault{}
	for _, kd := range KostenpositionDefaults {
		defaultByID[kd.ID] = kd
	}

	jahresdatenCache := map[int]map[int64]KostenpositionJahr{}
	jahresdatenFor := func(jahr int) (map[int64]KostenpositionJahr, error) {
		if kj, ok := jahresdatenCache[jahr]; ok {
			return kj, nil
		}
		kj, err := KostenpositionenJahr(db, jahr)
		if err != nil {
			return nil, err
		}
		jahresdatenCache[jahr] = kj
		return kj, nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, e := range eingaben {
		jahr, err := JahrFromMonat(e.monat)
		if err != nil {
			continue
		}
		jahresdaten, err := jahresdatenFor(jahr)
		if err != nil {
			return fmt.Errorf("kostenpositionen jahresdaten %d: %w", jahr, err)
		}

		existingWerte := map[int64]float64{}
		werteRows, err := db.Query(`SELECT kostenposition_id, wert FROM fixkosten_werte WHERE fixkosten_eingabe_id = ?`, e.id)
		if err != nil {
			return fmt.Errorf("query fixkosten werte for eingabe %d: %w", e.id, err)
		}
		for werteRows.Next() {
			var kpID int64
			var wert float64
			if err := werteRows.Scan(&kpID, &wert); err != nil {
				werteRows.Close()
				return fmt.Errorf("scan fixkosten wert: %w", err)
			}
			existingWerte[kpID] = wert
		}
		if err := werteRows.Err(); err != nil {
			werteRows.Close()
			return err
		}
		werteRows.Close()

		for _, kp := range kostenpositionen {
			var logik, typ string
			var wert float64

			if kj, ok := jahresdaten[kp.ID]; ok {
				logik, typ = kj.Logik, kj.Typ
				if typ == TypJaehrlich {
					wert = kj.Jahreswert
				} else if existing, ok := existingWerte[kp.ID]; ok {
					wert = existing
				} else if letzter, ok, err := LatestJaehrlichWert(db, kp.ID, jahr); err != nil {
					return fmt.Errorf("latest jaehrlich wert for %d: %w", kp.ID, err)
				} else if ok {
					wert = letzter / 12
				}
			} else if kd, ok := defaultByID[kp.ID]; ok {
				logik, typ = kd.Logik, kd.Typ
				wert = existingWerte[kp.ID]
			}

			if _, err := tx.Exec(
				`INSERT INTO fixkosten_werte (fixkosten_eingabe_id, kostenposition_id, wert, logik, typ)
				 VALUES (?, ?, ?, ?, ?)
				 ON CONFLICT(fixkosten_eingabe_id, kostenposition_id) DO UPDATE SET
				   wert = excluded.wert, logik = excluded.logik, typ = excluded.typ`,
				e.id, kp.ID, wert, logik, typ,
			); err != nil {
				return fmt.Errorf("backfill fixkosten wert eingabe %d position %d: %w", e.id, kp.ID, err)
			}
		}
	}

	return tx.Commit()
}

type apartmentSeed struct {
	id   int64
	name string
	qm   float64
}

type meterSeed struct {
	id          int64
	key         string
	meterType   string
	unit        string
	apartmentID *int64
	label       string
}

func apartmentID(id int64) *int64 { return &id }

// KostenpositionDefault is one of the 14 fixed Kostenpositionen (Issue #60)
// - id/key/label are app-fixed structure, seeded like meters. Logik/Typ are
// only the starting values for a brand-new Kostenpositionen-Jahr that has no
// previous year to copy from (see web's "Jahr anlegen" handler) - every
// year afterwards is user-editable data in kostenpositionen_jahre, not
// reseeded from here.
type KostenpositionDefault struct {
	ID    int64
	Key   string
	Label string
	Logik string
	Typ   string
}

// KostenpositionDefaults is the fixed, ordered list of the 14 Kostenpositionen.
var KostenpositionDefaults = []KostenpositionDefault{
	{ID: 1, Key: "grundsteuer", Label: "Grundsteuer", Logik: LogikQM, Typ: TypJaehrlich},
	{ID: 2, Key: "gebaeudevers", Label: "Wohngebäudeversicherung", Logik: LogikQM, Typ: TypJaehrlich},
	{ID: 3, Key: "deich_grund", Label: "Deichbeitrag Grund und Boden", Logik: LogikFlurstueck, Typ: TypJaehrlich},
	{ID: 4, Key: "deich_bau", Label: "Deichbeitrag Bauliche Anlagen", Logik: LogikFlurstueck, Typ: TypJaehrlich},
	{ID: 5, Key: "kreisverband", Label: "Kreisverband Wesermarsch der Wasser- und Bodenverbände", Logik: LogikFlurstueck, Typ: TypJaehrlich},
	{ID: 6, Key: "abfall_haushalt", Label: "Abfallwirtschaft Grundgebühr Haushalt", Logik: LogikWohneinheit, Typ: TypJaehrlich},
	{ID: 7, Key: "abfall_personen", Label: "Abfallwirtschaft Grundgebühr Personen", Logik: LogikPersonen, Typ: TypJaehrlich},
	{ID: 8, Key: "abfall_biomuell", Label: "Abfallwirtschaft Biomüll", Logik: LogikWohneinheit, Typ: TypJaehrlich},
	{ID: 9, Key: "abfall_restmuell", Label: "Abfallwirtschaft Restmüll", Logik: LogikWohneinheit, Typ: TypJaehrlich},
	{ID: 10, Key: "strom_grundpreis", Label: "Grundgebühr Strom", Logik: LogikWohneinheit, Typ: TypMonatlich},
	{ID: 11, Key: "trinkwasser", Label: "Grundgebühr Trinkwasser", Logik: LogikWohneinheit, Typ: TypMonatlich},
	{ID: 12, Key: "abwasser", Label: "Grundgebühr Abwasser", Logik: LogikWohneinheit, Typ: TypMonatlich},
	{ID: 13, Key: "internet", Label: "Grundgebühr Internet", Logik: LogikWohneinheit, Typ: TypMonatlich},
	{ID: 14, Key: "wp_wartung", Label: "Wartungskosten Wärmepumpe", Logik: LogikWohneinheit, Typ: TypMonatlich},
}

// seed inserts the fixed master data for the 2 apartments and 9 meters if
// they're not already present. Keyed on `apartments.id` / `meters.key` so
// it's safe to run on every startup.
func seed(db *sql.DB) error {
	// qm (and flurstueck_groesse, via the column DEFAULT) start at 0 (Ticket
	// #38) - unlike the apartment id/name, Wohnungsgröße/Flurstücksgröße are
	// user data, not app-fixed structure, so they aren't hardcoded here.
	// Both are live columns, edited on /stammdaten (Issue #61), not seeded
	// with a real value.
	apartments := []apartmentSeed{
		{id: 1, name: "Wohnung 1", qm: 0},
		{id: 2, name: "Wohnung 2", qm: 0},
	}
	for _, a := range apartments {
		if _, err := db.Exec(
			`INSERT INTO apartments (id, name, qm) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			a.id, a.name, a.qm,
		); err != nil {
			return fmt.Errorf("seed apartment %q: %w", a.name, err)
		}
	}

	meters := []meterSeed{
		{id: 1, key: "strom_gesamt", meterType: "strom", unit: "kWh", apartmentID: nil, label: "Stromzähler Gesamt"},
		{id: 2, key: "strom_wohnung2", meterType: "strom", unit: "kWh", apartmentID: apartmentID(2), label: "Zwischenstromzähler Wohnung 2"},
		{id: 3, key: "strom_waermepumpe", meterType: "strom", unit: "kWh", apartmentID: nil, label: "Zwischenstromzähler Wärmepumpe"},
		{id: 4, key: "strom_wallbox", meterType: "strom", unit: "kWh", apartmentID: nil, label: "Zwischenzähler Wallboxen"},
		{id: 5, key: "wasser_gesamt", meterType: "wasser", unit: "m3", apartmentID: nil, label: "Wasserzähler Gesamt"},
		{id: 6, key: "wasser_wohnung2", meterType: "wasser", unit: "m3", apartmentID: apartmentID(2), label: "Zwischenwasserzähler Wohnung 2"},
		{id: 7, key: "wasser_warmwasseraufbereitung", meterType: "wasser", unit: "m3", apartmentID: nil, label: "Zwischenwasserzähler Warmwasseraufbereitung"},
		{id: 8, key: "waerme_wohnung1", meterType: "waerme", unit: "MWh", apartmentID: apartmentID(1), label: "Wärmemengenzähler Wohnung 1"},
		{id: 9, key: "waerme_wohnung2", meterType: "waerme", unit: "MWh", apartmentID: apartmentID(2), label: "Wärmemengenzähler Wohnung 2"},
		{id: 10, key: "strom_einspeisung", meterType: "strom", unit: "kWh", apartmentID: nil, label: "Einspeisezähler (PV)"},
	}
	for _, m := range meters {
		if _, err := db.Exec(
			`INSERT INTO meters (id, key, type, unit, apartment_id, label) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			m.id, m.key, m.meterType, m.unit, m.apartmentID, m.label,
		); err != nil {
			return fmt.Errorf("seed meter %q: %w", m.key, err)
		}
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
		if _, err := db.Exec(
			`INSERT INTO kostenpositionen (id, key, label) VALUES (?, ?, ?)
			 ON CONFLICT(id) DO UPDATE SET label = excluded.label`,
			kp.ID, kp.Key, kp.Label,
		); err != nil {
			return fmt.Errorf("seed kostenposition %q: %w", kp.Key, err)
		}
	}

	return nil
}
