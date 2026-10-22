package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// schemaVersion is the version of the schema this build produces, kept in
// PRAGMA user_version. Raise it with every migration step added to
// migrationen: a database below it is saved before the steps run (see
// sichereVorMigration). The steps themselves do not look at it, each one is
// idempotent and checks the schema itself, so a database in an unknown state
// is migrated the same way as before.
const schemaVersion = 1

// sicherungenBehalten is how many saves made before a migration are kept.
const sicherungenBehalten = 3

// jetzt is the clock of the file names of the saves, replaceable in tests.
var jetzt = time.Now

// migration is one idempotent step that brings an existing database up to
// the current schema.
type migration struct {
	name string
	run  func(db *sql.DB) error
}

// migrationen are the steps in the order they must run. The order is part of
// the contract (see TestMigrationen_Reihenfolge):
//   - ensurePeriodSwitchColumns after the periods rebuild of
//     ensurePeriodsNullablePriceColumns, which would drop the columns again,
//   - seed before migrateHeizungGewichtungToHaus, which needs the haus row,
//   - ensureFixkostenWerteLogikTypColumns before dropKostenpositionenJahreTable
//     (ADR 0001): the table is dropped only after its data has moved.
var migrationen = []migration{
	// CREATE TABLE IF NOT EXISTS (schema.sql) only creates tables on a
	// brand-new database. SQLite has no "ADD COLUMN IF NOT EXISTS", so each
	// column step checks first.
	{"ensurePeriodsHeizungGewichtungColumn", ensurePeriodsHeizungGewichtungColumn},
	{"ensurePeriodsEinspeisungPreisColumn", ensurePeriodsEinspeisungPreisColumn},
	{"ensureApartmentsFlurstueckGroesseColumn", ensureApartmentsFlurstueckGroesseColumn},
	{"ensurePeriodsMonatColumn", ensurePeriodsMonatColumn},
	{"ensurePeriodsNullablePriceColumns", ensurePeriodsNullablePriceColumns},
	{"ensurePeriodSwitchColumns", ensurePeriodSwitchColumns},
	{"ensureKostenpositionenUmlagefaehigColumn", ensureKostenpositionenUmlagefaehigColumn},
	{"ensureKostenpositionenSortColumn", ensureKostenpositionenSortColumn},
	{"ensureApartmentsDetailsColumns", ensureApartmentsDetailsColumns},
	{"ensureMieterSeitColumn", ensureMieterSeitColumn},
	{"ensureHausDetailsColumns", ensureHausDetailsColumns},
	{"ensureHausWallboxWeiterberechnenColumn", ensureHausWallboxWeiterberechnenColumn},
	{"seed", seed},
	{"migrateHeizungGewichtungToHaus", migrateHeizungGewichtungToHaus},
	{"ensureFixkostenWerteLogikTypColumns", ensureFixkostenWerteLogikTypColumns},
	{"dropKostenpositionenJahreTable", dropKostenpositionenJahreTable},
	{"ensureFixkostenEingabenMonatUnique", ensureFixkostenEingabenMonatUnique},
}

// migrate runs every step in order and stops at the first error, which names
// the step. Afterwards user_version is raised to schemaVersion (never lowered:
// a database written by a newer build keeps its version).
func migrate(db *sql.DB) error {
	for _, m := range migrationen {
		if err := m.run(db); err != nil {
			return fmt.Errorf("migrate %s: %w", m.name, err)
		}
	}
	version, err := schemaVersionOf(db)
	if err != nil {
		return err
	}
	if version < schemaVersion {
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			return fmt.Errorf("set schema version: %w", err)
		}
	}
	return nil
}

func schemaVersionOf(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return version, nil
}

// sicherungsName matches the file name of a save, after the database file
// name: .vor-schema-<old version>-<UTC time>.
var sicherungsName = regexp.MustCompile(`^\.vor-schema-(\d+)-(\d{8}T\d{6}\.\d{3}Z)$`)

// sichereVorMigration saves a database whose schema version is not
// schemaVersion before anything touches it: VACUUM INTO writes a consistent
// copy next to the database as <path>.vor-schema-<old version>-<UTC time>.
// A newer version (a database written by a newer build) is saved as well, as
// an older build's steps may rebuild tables. A new, empty database has nothing
// to save. It returns the file written, "" if there was nothing to save.
// Without a save the migration must not start, so a failure is returned.
func sichereVorMigration(db *sql.DB, path string) (ziel string, err error) {
	version, err := schemaVersionOf(db)
	if err != nil {
		return "", err
	}
	if version == schemaVersion {
		return "", nil
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables); err != nil {
		return "", fmt.Errorf("count tables: %w", err)
	}
	if tables == 0 {
		return "", nil
	}

	ziel = fmt.Sprintf("%s.vor-schema-%d-%s", path, version, jetzt().UTC().Format("20060102T150405.000Z"))
	_, existierte := os.Stat(ziel)
	if _, err := db.Exec("VACUUM INTO ?", ziel); err != nil {
		if existierte != nil {
			// A failed write may leave half a file that would later count as a save.
			_ = os.Remove(ziel)
		}
		return "", fmt.Errorf("save before migration failed (%s), database unchanged: %w", ziel, err)
	}
	// The save holds personal data (names, IBAN): not readable by more than the database.
	if fi, err := os.Stat(path); err == nil {
		if err := os.Chmod(ziel, fi.Mode().Perm()); err != nil {
			log.Printf("store: permissions of %s not set: %v", ziel, err)
		}
	}
	log.Printf("store: database saved before migration: %s", ziel)
	return ziel, nil
}

// sicherungen lists the saves of the database at path, oldest first.
func sicherungen(path string) ([]string, error) {
	eintraege, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	praefix := filepath.Base(path)
	var out []string
	for _, e := range eintraege {
		if rest, ok := strings.CutPrefix(e.Name(), praefix); ok && sicherungsName.MatchString(rest) {
			out = append(out, filepath.Join(filepath.Dir(path), e.Name()))
		}
	}
	zeit := func(name string) string { return name[strings.LastIndex(name, "-")+1:] }
	sort.Slice(out, func(i, j int) bool { return zeit(out[i]) < zeit(out[j]) })
	return out, nil
}

// raeumeSicherungenAuf keeps the newest sicherungenBehalten saves. It runs
// only after a successful migration: a failed start must not push the one
// clean save out. A failure here is logged, it never stops the start.
func raeumeSicherungenAuf(path string) {
	alle, err := sicherungen(path)
	if err != nil {
		log.Printf("store: saves not listed: %v", err)
		return
	}
	for len(alle) > sicherungenBehalten {
		if err := os.Remove(alle[0]); err != nil {
			log.Printf("store: old save %s not removed: %v", alle[0], err)
		}
		alle = alle[1:]
	}
}

// verwirfDieseSicherung drops the save of a start whose migration failed if
// an earlier save of the same old version exists: that one is from before the
// first attempt, this one holds a half-migrated state.
func verwirfDieseSicherung(path, ziel string) {
	m := sicherungsName.FindStringSubmatch(strings.TrimPrefix(ziel, path))
	if m == nil {
		return
	}
	alle, err := sicherungen(path)
	if err != nil {
		return
	}
	for _, a := range alle {
		if a == ziel {
			continue
		}
		if n := sicherungsName.FindStringSubmatch(strings.TrimPrefix(a, path)); n != nil && n[1] == m[1] {
			if err := os.Remove(ziel); err != nil {
				log.Printf("store: save %s not removed: %v", ziel, err)
			}
			return
		}
	}
}
