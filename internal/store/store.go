// Package store owns the SQLite schema, migrations, and seed data for the
// Nebenkostenrechner. See https://github.com/larknafets/nebenkostenrechner/issues/6
// for the schema decision this package implements.
package store

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
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

	if err := ensurePeriodsNullablePriceColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate periods price columns nullable: %w", err)
	}

	if err := seed(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("seed master data: %w", err)
	}

	if err := ensureFixkostenWerteLogikTypColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate fixkosten_werte logik/typ columns: %w", err)
	}

	if err := dropKostenpositionenJahreTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("drop kostenpositionen_jahre: %w", err)
	}

	return db, nil
}

// dropKostenpositionenJahreTable removes kostenpositionen_jahre (Issue
// #105/#109): its data has already migrated into fixkosten_werte's
// logik/typ columns (see ensureFixkostenWerteLogikTypColumns, which always
// runs first) - Logik/Typ/Wert live per Fixkosten-Eingabe now, not
// jahresweise. IF EXISTS makes this idempotent, same as every other
// migration here.
func dropKostenpositionenJahreTable(db *sql.DB) error {
	_, err := db.Exec(`DROP TABLE IF EXISTS kostenpositionen_jahre`)
	return err
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

// ensurePeriodsNullablePriceColumns makes periods' 4 price columns
// (strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis)
// nullable (Ticket #128, Teilstand) - a NULL there means "not yet
// entered", the write-side counterpart to PeriodInput's *float64 fields.
// SQLite has no `ALTER TABLE ... ALTER COLUMN ... DROP NOT NULL`, so this
// follows SQLite's documented 12-step table-rebuild procedure
// (https://www.sqlite.org/lang_altertable.html#otheralter) instead of the
// simple ADD COLUMN migrations above. Pinned to a single *sql.Conn: PRAGMA
// foreign_keys is per-connection, and running the OFF/rebuild/ON sequence
// across different pooled connections could leave FK enforcement in the
// wrong state on whichever connection a later request happens to land on.
// Idempotent: checks strompreis's current NOT NULL-ness first and returns
// immediately once it's already nullable. Must run after every ADD COLUMN
// migration above - it assumes periods already has heizung_waerme_gewichtung/
// einspeisung_preis/monat.
func ensurePeriodsNullablePriceColumns(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Close()

	nullable, err := periodsStrompreisNullable(ctx, conn)
	if err != nil {
		return err
	}
	if nullable {
		return nil
	}

	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}
	defer conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`)

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Aufräumen falls ein früherer Migrationsversuch mittendrin abgebrochen
	// ist (Prozess gekillt zwischen CREATE und DROP) und periods_new noch
	// von damals herumliegt - sonst würde das CREATE unten fehlschlagen.
	if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS periods_new`); err != nil {
		return fmt.Errorf("drop stale periods_new: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE periods_new (
		    id                         INTEGER PRIMARY KEY,
		    reading_date               TEXT NOT NULL,
		    strompreis                 REAL,
		    frischwasser_preis         REAL,
		    abwasser_preis             REAL,
		    heizung_waerme_gewichtung  REAL NOT NULL DEFAULT 0.7,
		    einspeisung_preis          REAL,
		    monat                      TEXT NOT NULL DEFAULT ''
		)
	`); err != nil {
		return fmt.Errorf("create periods_new: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO periods_new (id, reading_date, strompreis, frischwasser_preis, abwasser_preis, heizung_waerme_gewichtung, einspeisung_preis, monat)
		SELECT id, reading_date, strompreis, frischwasser_preis, abwasser_preis, heizung_waerme_gewichtung, einspeisung_preis, monat FROM periods
	`); err != nil {
		return fmt.Errorf("copy periods into periods_new: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DROP TABLE periods`); err != nil {
		return fmt.Errorf("drop old periods table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE periods_new RENAME TO periods`); err != nil {
		return fmt.Errorf("rename periods_new to periods: %w", err)
	}

	checkRows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("foreign key check: %w", err)
	}
	hasViolation := checkRows.Next()
	if err := checkRows.Err(); err != nil {
		checkRows.Close()
		return fmt.Errorf("foreign key check: %w", err)
	}
	checkRows.Close()
	if hasViolation {
		return fmt.Errorf("foreign key check failed after periods table rebuild")
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

// periodsStrompreisNullable inspects periods.strompreis's current NOT NULL-
// ness via PRAGMA table_info - strompreis stands in for all 4 price
// columns since ensurePeriodsNullablePriceColumns always rebuilds them
// together.
func periodsStrompreisNullable(ctx context.Context, conn *sql.Conn) (bool, error) {
	rows, err := conn.QueryContext(ctx, `PRAGMA table_info(periods)`)
	if err != nil {
		return false, fmt.Errorf("inspect periods columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scan periods column: %w", err)
		}
		if name == "strompreis" {
			return notnull == 0, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, fmt.Errorf("periods.strompreis column not found")
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

// migrationKostenpositionJahr is kostenpositionen_jahre's shape, read only
// by backfillFixkostenWerteLogikTyp - kostenpositionen_jahre is dropped
// right after this migration runs (see dropKostenpositionenJahreTable), so
// nothing outside this file needs its own exported type/reader anymore.
type migrationKostenpositionJahr struct {
	Logik      string
	Typ        string
	Jahreswert float64
}

// kostenpositionenJahrForMigration reads one Jahr's kostenpositionen_jahre
// rows - the migration-only, unexported remainder of the former public
// KostenpositionenJahr reader (Issue #109: kostenpositionen_jahre isn't a
// standalone concept anymore once this migration has run everywhere).
func kostenpositionenJahrForMigration(db *sql.DB, jahr int) (map[int64]migrationKostenpositionJahr, error) {
	rows, err := db.Query(`SELECT kostenposition_id, logik, typ, jahreswert FROM kostenpositionen_jahre WHERE jahr = ?`, jahr)
	if err != nil {
		return nil, fmt.Errorf("query kostenpositionen_jahre: %w", err)
	}
	defer rows.Close()

	out := map[int64]migrationKostenpositionJahr{}
	for rows.Next() {
		var id int64
		var v migrationKostenpositionJahr
		if err := rows.Scan(&id, &v.Logik, &v.Typ, &v.Jahreswert); err != nil {
			return nil, fmt.Errorf("scan kostenposition_jahr: %w", err)
		}
		out[id] = v
	}
	return out, rows.Err()
}

// latestJaehrlichWertForMigration is the migration-only, unexported
// remainder of the former public LatestJaehrlichWert (Issue #109).
func latestJaehrlichWertForMigration(db *sql.DB, kostenpositionID int64, maxJahr int) (wert float64, ok bool, err error) {
	err = db.QueryRow(
		`SELECT jahreswert FROM kostenpositionen_jahre
		 WHERE kostenposition_id = ? AND jahr <= ? AND typ = ?
		 ORDER BY jahr DESC LIMIT 1`,
		kostenpositionID, maxJahr, TypJaehrlich,
	).Scan(&wert)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("query latest jaehrlich wert for kostenposition %d: %w", kostenpositionID, err)
	}
	return wert, true, nil
}

// backfillFixkostenWerteLogikTyp fills every existing Fixkosten-Eingabe's 14
// Kostenpositionen with the Logik/Typ/Wert that applied to it at its own
// Monat, read from kostenpositionen_jahre (dropped right after this
// migration runs, see dropKostenpositionenJahreTable) - exactly the former
// monatswertFuer's jährlich/monatlich/Fallback rules
// (internal/calc/fixkosten.go), run once here instead of on every
// Berechnung. A Kostenposition with no kostenpositionen_jahre row for that
// Jahr (only possible for an Eingabe whose Jahr was never "angelegt", which
// calc.Fixkosten already refused to compute) falls back to
// KostenpositionDefaults so it still gets a sane Logik/Typ instead of
// staying empty.
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

	jahresdatenCache := map[int]map[int64]migrationKostenpositionJahr{}
	jahresdatenFor := func(jahr int) (map[int64]migrationKostenpositionJahr, error) {
		if kj, ok := jahresdatenCache[jahr]; ok {
			return kj, nil
		}
		kj, err := kostenpositionenJahrForMigration(db, jahr)
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
				} else if letzter, ok, err := latestJaehrlichWertForMigration(db, kp.ID, jahr); err != nil {
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
// only the starting values for the very first Fixkosten-Eingabe ever
// created (Issue #105/#108) - every Eingabe afterwards is prefilled from
// the previous one instead, not reseeded from here.
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
