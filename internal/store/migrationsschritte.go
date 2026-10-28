package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
)

// periodsHatAblesungsPreise reports whether periods still carries the Ablesung
// prices, which is the mark of a database from before #368. The older steps
// that work on those columns (add einspeisung_preis, make the prices nullable,
// add the switch columns) run only then: on a fresh or already migrated
// database they would add the columns back just for the drop at the end of
// the list to remove them again. strompreis is the mark because
// dropPeriodsAblesungsFelder drops it last, so an interrupted drop is still
// recognised and finished.
func periodsHatAblesungsPreise(db *sql.DB) (bool, error) {
	return hasColumn(db, "periods", "strompreis")
}

// periodsAblesungsFelder are the columns dropPeriodsAblesungsFelder removes
// from periods (#368), in the order of dropping: strompreis last, see
// periodsHatAblesungsPreise.
var periodsAblesungsFelder = []string{
	"einspeisung_preis", "abwasser_preis", "frischwasser_preis",
	"wallbox_komplett_w1", "pv_gutschrift_w1", "strompreis",
}

// dropPeriodsAblesungsFelder removes the prices and switches from periods
// (#368, spec #359 step 4, destructive): they live on fixkosten_eingaben now,
// where backfillFixkostenEingabenAusAblesungen has copied them. Needs no
// index drop and no table rebuild. A column that is already gone is skipped,
// so the step is idempotent. It runs at the end of the list, with
// dropPeriodOccupancyTable.
func dropPeriodsAblesungsFelder(db *sql.DB) error {
	for _, c := range periodsAblesungsFelder {
		has, err := hasColumn(db, "periods", c)
		if err != nil {
			return fmt.Errorf("drop periods.%s: %w", c, err)
		}
		if !has {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE periods DROP COLUMN ` + c); err != nil {
			return fmt.Errorf("drop periods.%s: %w", c, err)
		}
	}
	return nil
}

// dropPeriodOccupancyTable removes period_occupancy (#368, destructive): the
// Personenzahl belongs to the Fixkosten-Eingabe (fixkosten_personen) since
// #359. IF EXISTS makes the step idempotent.
func dropPeriodOccupancyTable(db *sql.DB) error {
	_, err := db.Exec(`DROP TABLE IF EXISTS period_occupancy`)
	return err
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
	// Once the weighting lives on haus (Issue #162) periods must not get the
	// column back, or every restart would re-add it.
	if migrated, err := hasColumn(db, "haus", "heizung_waerme_gewichtung"); err != nil {
		return err
	} else if migrated {
		return nil
	}

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
// until corrected. Skipped once the Ablesung prices are gone (#368), see
// periodsHatAblesungsPreise.
func ensurePeriodsEinspeisungPreisColumn(db *sql.DB) error {
	if legacy, err := periodsHatAblesungsPreise(db); err != nil || !legacy {
		return err
	}
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

// ensureKostenpositionenUmlagefaehigColumn adds the umlagefaehig column to a
// kostenpositionen table that predates it (Issue #163) and sets every
// existing row to the starting value from KostenpositionDefaults. The
// starting values are applied only in the call that adds the column, so a
// flag the user changed later is never overwritten by a restart. On a
// brand-new database the column already comes from schema.sql, the check
// finds it and does nothing - the rows are seeded with their starting
// value by seed().
func ensureKostenpositionenUmlagefaehigColumn(db *sql.DB) error {
	rows, err := db.Query(`PRAGMA table_info(kostenpositionen)`)
	if err != nil {
		return fmt.Errorf("inspect kostenpositionen columns: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("scan kostenpositionen column: %w", err)
		}
		if name == "umlagefaehig" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE kostenpositionen ADD COLUMN umlagefaehig INTEGER NOT NULL DEFAULT 1`); err != nil {
		return fmt.Errorf("add umlagefaehig column: %w", err)
	}
	for _, kp := range KostenpositionDefaults {
		if _, err := tx.Exec(`UPDATE kostenpositionen SET umlagefaehig = ? WHERE id = ?`, boolToInt(kp.Umlagefaehig), kp.ID); err != nil {
			return fmt.Errorf("set umlagefaehig for kostenposition %q: %w", kp.Key, err)
		}
	}
	return tx.Commit()
}

// ensureKostenpositionenSortColumn adds the sort column to a kostenpositionen
// table that predates it and gives the existing rows the order of
// KostenpositionDefaults. The display order is not the id order, so ids that
// fixkosten_werte references never change. Afterwards sort is a user-edited
// Stammdaten value (MoveKostenposition), so only the call that adds the column
// sets it.
func ensureKostenpositionenSortColumn(db *sql.DB) error {
	ok, err := hasColumn(db, "kostenpositionen", "sort")
	if err != nil {
		return fmt.Errorf("inspect kostenpositionen columns: %w", err)
	}
	if ok {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE kostenpositionen ADD COLUMN sort INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add sort column: %w", err)
	}
	for i, kp := range KostenpositionDefaults {
		if _, err := tx.Exec(`UPDATE kostenpositionen SET sort = ? WHERE id = ?`, i+1, kp.ID); err != nil {
			return fmt.Errorf("set sort for kostenposition %q: %w", kp.Key, err)
		}
	}
	return tx.Commit()
}

// ensureApartmentsDetailsColumns adds mieter_name, mieter_anschrift and
// status to an apartments table that predates them (Issue #164). The
// starting status (Wohnung 1 Eigennutzung, Wohnung 2 vermietet) is applied
// only in the call that adds the columns, so a status the user changed
// later survives every restart. On a brand-new database schema.sql already
// has the columns and seed() sets the starting status.
func ensureApartmentsDetailsColumns(db *sql.DB) error {
	has, err := hasColumn(db, "apartments", "status")
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, stmt := range []string{
		`ALTER TABLE apartments ADD COLUMN mieter_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE apartments ADD COLUMN mieter_anschrift TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE apartments ADD COLUMN status TEXT NOT NULL DEFAULT 'vermietet'`,
	} {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("add apartments column: %w", err)
		}
	}
	if _, err := tx.Exec(`UPDATE apartments SET status = ? WHERE id = 1`, StatusEigennutzung); err != nil {
		return fmt.Errorf("set starting status: %w", err)
	}
	return tx.Commit()
}

// ensureMieterSeitColumn adds apartments.mieter_seit to a database that
// predates it. The default "" means "since the start of the recording", so
// existing data keeps its meaning. On a brand-new database schema.sql
// already has the column.
func ensureMieterSeitColumn(db *sql.DB) error {
	has, err := hasColumn(db, "apartments", "mieter_seit")
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE apartments ADD COLUMN mieter_seit TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add apartments mieter_seit column: %w", err)
	}
	return nil
}

// ensureHausDetailsColumns adds the Vermieter/Objekt/IBAN/Kontoinhaber columns to a haus
// table created by Issue #163, before it had them (Issue #164). A brand-new
// database gets them from schema.sql.
func ensureHausDetailsColumns(db *sql.DB) error {
	// DDL cannot take parameters, so each statement is a complete literal
	// instead of a concatenation with the column name.
	for _, c := range []struct{ column, stmt string }{
		{"vermieter_name", `ALTER TABLE haus ADD COLUMN vermieter_name TEXT NOT NULL DEFAULT ''`},
		{"vermieter_anschrift", `ALTER TABLE haus ADD COLUMN vermieter_anschrift TEXT NOT NULL DEFAULT ''`},
		{"objekt_anschrift", `ALTER TABLE haus ADD COLUMN objekt_anschrift TEXT NOT NULL DEFAULT ''`},
		{"iban", `ALTER TABLE haus ADD COLUMN iban TEXT NOT NULL DEFAULT ''`},
		{"kontoinhaber", `ALTER TABLE haus ADD COLUMN kontoinhaber TEXT NOT NULL DEFAULT ''`},
	} {
		has, err := hasColumn(db, "haus", c.column)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := db.Exec(c.stmt); err != nil {
			return fmt.Errorf("add haus column %s: %w", c.column, err)
		}
	}
	return nil
}

// ensureHausWallboxWeiterberechnenColumn adds the flag "Wallbox
// weiterberechnen" (#228) to a haus table that predates it. Existing
// databases get 1 (on), so a rented apartment keeps seeing the Wallbox costs
// once they are assigned.
func ensureHausWallboxWeiterberechnenColumn(db *sql.DB) error {
	has, err := hasColumn(db, "haus", "wallbox_weiterberechnen")
	if err != nil || has {
		return err
	}
	_, err = db.Exec(`ALTER TABLE haus ADD COLUMN wallbox_weiterberechnen INTEGER NOT NULL DEFAULT 1`)
	return err
}

// migrateHeizungGewichtungToHaus moves the Heizungs-Gewichtung from every
// Ablesung (periods.heizung_waerme_gewichtung) to a single Stammdaten value
// (haus.heizung_waerme_gewichtung, Issue #162): the contract fixes the
// heating split for the whole year, so it is no longer chosen per reading.
// The value of the newest Ablesung becomes the Stammdaten value (default
// 0.7 for an empty or invalid one), then the periods column is dropped.
// Runs once: the haus column is the marker that the migration happened,
// a brand-new database already has it from schema.sql.
func migrateHeizungGewichtungToHaus(db *sql.DB) error {
	migrated, err := hasColumn(db, "haus", "heizung_waerme_gewichtung")
	if err != nil {
		return err
	}
	if migrated {
		return nil
	}
	periodsHas, err := hasColumn(db, "periods", "heizung_waerme_gewichtung")
	if err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`ALTER TABLE haus ADD COLUMN heizung_waerme_gewichtung REAL NOT NULL DEFAULT 0.7`); err != nil {
		return fmt.Errorf("add haus column: %w", err)
	}
	if periodsHas {
		var newest float64
		err := tx.QueryRow(`SELECT heizung_waerme_gewichtung FROM periods ORDER BY reading_date DESC, id DESC LIMIT 1`).Scan(&newest)
		if err != nil && err != sql.ErrNoRows {
			return fmt.Errorf("read newest weighting: %w", err)
		}
		if err == nil && ValidHeizungGewichtung(newest) {
			if _, err := tx.Exec(`UPDATE haus SET heizung_waerme_gewichtung = ? WHERE id = 1`, newest); err != nil {
				return fmt.Errorf("write haus weighting: %w", err)
			}
		}
		if _, err := tx.Exec(`ALTER TABLE periods DROP COLUMN heizung_waerme_gewichtung`); err != nil {
			return fmt.Errorf("drop periods column: %w", err)
		}
	}
	return tx.Commit()
}

// ensureFixkostenEingabenMonatUnique makes sure every month has at most one
// Fixkosten-Eingabe (Issue #161) by adding a UNIQUE index on
// fixkosten_eingaben.monat. Duplicates from before this rule are never
// resolved on their own - which of two entries is right is the user's call.
// The app still starts, so they can be fixed on /fixkosten (delete one or
// move it to a free month): the start logs a warning naming the affected
// months and skips the index, which is created by a later start once the
// duplicates are gone. New duplicates are refused by the writes themselves
// (ErrFixkostenMonatBelegt) either way. The index is created here, not in
// schema.sql, so a database with duplicates does not fail while the schema
// is applied.
func ensureFixkostenEingabenMonatUnique(db *sql.DB) error {
	rows, err := db.Query(`SELECT monat FROM fixkosten_eingaben GROUP BY monat HAVING COUNT(*) > 1 ORDER BY monat`)
	if err != nil {
		return fmt.Errorf("check duplicate fixkosten months: %w", err)
	}
	defer rows.Close()

	var doppelt []string
	for rows.Next() {
		var monat string
		if err := rows.Scan(&monat); err != nil {
			return fmt.Errorf("scan duplicate fixkosten month: %w", err)
		}
		doppelt = append(doppelt, monat)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(doppelt) > 0 {
		log.Printf("WARNUNG: mehrere Fixkosten-Eingaben im selben Monat (%s). Je Monat darf es nur eine geben, bitte die doppelten Eingaben unter /fixkosten loeschen oder in einen freien Monat verschieben. Die Jahresabrechnung ist fuer diese Monate gesperrt.", strings.Join(doppelt, ", "))
		return nil
	}

	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_fixkosten_eingaben_monat ON fixkosten_eingaben(monat)`); err != nil {
		return fmt.Errorf("create unique index on fixkosten_eingaben.monat: %w", err)
	}
	return nil
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
// einspeisung_preis/monat. Skipped once the Ablesung prices are gone (#368),
// see periodsHatAblesungsPreise.
func ensurePeriodsNullablePriceColumns(db *sql.DB) error {
	if legacy, err := periodsHatAblesungsPreise(db); err != nil || !legacy {
		return err
	}
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
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Clean up in case an earlier migration attempt aborted midway (process
	// killed between CREATE and DROP) and periods_new is still lying
	// around from back then - otherwise the CREATE below would fail.
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
