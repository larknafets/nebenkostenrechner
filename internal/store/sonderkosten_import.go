package store

import "database/sql"

// ImportSonderkosten inserts every entry in one transaction: all or nothing.
// Like CreateSonderkosten it validates every entry (*SonderkostenFehler),
// inside the transaction, so a bad entry rolls everything back.
func ImportSonderkosten(db *sql.DB, eintraege []Sonderkosten) (ids []int64, err error) {
	err = inTx(db, func(tx *sql.Tx) error {
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
