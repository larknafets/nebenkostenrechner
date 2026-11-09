package store

import (
	"database/sql"
	"sync"
)

// sonderkostenImportMu serializes the imports of this process: two deferred
// SQLite transactions that both read "empty" cannot both upgrade to a write,
// the loser would fail with a database error instead of SonderkostenImportNurLeer.
// shortcut: one process per database, a second app process would not be covered.
var sonderkostenImportMu sync.Mutex

// SonderkostenImportNurLeer is the message when the table already has entries.
const SonderkostenImportNurLeer = "Import nur möglich, solange noch keine Sonderkosten existieren"

// ImportSonderkosten inserts every entry in one transaction: all or nothing.
// Like CreateSonderkosten it validates every entry (*SonderkostenFehler),
// inside the transaction, so a bad entry rolls everything back. It checks that
// the table is empty inside the transaction too: the check of the caller and
// the insert are two steps, a second import or an entry created in between
// would otherwise be doubled.
func ImportSonderkosten(db *sql.DB, eintraege []Sonderkosten) (ids []int64, err error) {
	sonderkostenImportMu.Lock()
	defer sonderkostenImportMu.Unlock()
	err = inTx(db, func(tx *sql.Tx) error {
		var vorhanden int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM sonderkosten`).Scan(&vorhanden); err != nil {
			return err
		}
		if vorhanden > 0 {
			return &SonderkostenFehler{Meldungen: []string{SonderkostenImportNurLeer}}
		}
		ids = make([]int64, 0, len(eintraege))
		for _, s := range eintraege {
			id, err := insertSonderkosten(tx, s)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	return ids, err
}
