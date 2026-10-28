package store

import (
	"database/sql"
	"fmt"
	"log"
	"slices"
	"strings"
)

// backfillFixkostenEingabenAusAblesungen gives every Fixkosten-Eingabe the
// prices and switches of the Ablesungen of its Abrechnungsmonat (#361, spec
// #359, step 1, non-destructive: the Ablesung columns stay).
//
// The Ablesungen of a month are read newest first (reading_date, then id).
// Each price takes the newest value that is not empty, so a Teilstand without
// prices deletes nothing. The switches come from the newest Ablesung: false is
// a real answer there. A month without an Ablesung stays as it is (prices
// empty, switches off).
//
// Only empty target fields are written (a price that is NULL, a switch that
// is off), so the step can run on every start and never overwrites what was
// entered on the Fixkosten-Eingabe. If several Ablesungen of one month differ
// in a price or a switch, one log line names the month. The step skips
// silently if a source column on periods is gone.
func backfillFixkostenEingabenAusAblesungen(db *sql.DB) error {
	preise := []string{"strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis"}
	quellen := slices.Clone(preise)
	for _, s := range Switches {
		quellen = append(quellen, s.Column)
	}
	for _, c := range quellen {
		has, err := hasColumn(db, "periods", c)
		if err != nil {
			return fmt.Errorf("backfill fixkosten_eingaben: %w", err)
		}
		if !has {
			return nil
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("backfill fixkosten_eingaben: %w", err)
	}
	defer tx.Rollback()

	type eingabe struct {
		id    int64
		monat string
	}
	rows, err := tx.Query(`SELECT id, monat FROM fixkosten_eingaben ORDER BY monat, id`)
	if err != nil {
		return fmt.Errorf("backfill fixkosten_eingaben: %w", err)
	}
	var eingaben []eingabe
	for rows.Next() {
		var e eingabe
		if err := rows.Scan(&e.id, &e.monat); err != nil {
			rows.Close()
			return fmt.Errorf("backfill fixkosten_eingaben: %w", err)
		}
		eingaben = append(eingaben, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("backfill fixkosten_eingaben: %w", err)
	}

	for _, e := range eingaben {
		if err := backfillEineEingabe(tx, e.id, e.monat, preise); err != nil {
			return fmt.Errorf("backfill fixkosten_eingaben %s: %w", e.monat, err)
		}
	}
	return tx.Commit()
}

// backfillEineEingabe is backfillFixkostenEingabenAusAblesungen for one
// Fixkosten-Eingabe.
func backfillEineEingabe(tx *sql.Tx, id int64, monat string, preise []string) error {
	cols := slices.Clone(preise)
	for _, s := range Switches {
		cols = append(cols, s.Column)
	}
	rows, err := tx.Query(`SELECT `+strings.Join(cols, ", ")+` FROM periods WHERE monat = ? ORDER BY reading_date DESC, id DESC`, monat)
	if err != nil {
		return err
	}

	preis := make([]sql.NullFloat64, len(preise))
	haekchen := make([]bool, len(Switches))
	konflikt := false
	n := 0
	for rows.Next() {
		zeile := make([]sql.NullFloat64, len(preise))
		flags := make([]int, len(Switches))
		dest := make([]any, 0, len(cols))
		for i := range zeile {
			dest = append(dest, &zeile[i])
		}
		for i := range flags {
			dest = append(dest, &flags[i])
		}
		if err := rows.Scan(dest...); err != nil {
			rows.Close()
			return err
		}
		for i, z := range zeile {
			if !z.Valid {
				continue
			}
			if !preis[i].Valid {
				preis[i] = z
			} else if preis[i].Float64 != z.Float64 {
				konflikt = true
			}
		}
		for i, f := range flags {
			if n == 0 {
				haekchen[i] = f != 0
			} else if haekchen[i] != (f != 0) {
				konflikt = true
			}
		}
		n++
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	if konflikt {
		log.Printf("store: Ablesungen des Abrechnungsmonats %s weichen in Preisen oder Häkchen voneinander ab, übernommen wird je Preis der jüngste Wert, die Häkchen kommen von der jüngsten Ablesung", monat)
	}

	for i, p := range preise {
		if !preis[i].Valid {
			continue
		}
		if _, err := tx.Exec(`UPDATE fixkosten_eingaben SET `+p+` = ? WHERE id = ? AND `+p+` IS NULL`, preis[i].Float64, id); err != nil {
			return err
		}
	}
	for i, s := range Switches {
		if !haekchen[i] {
			continue
		}
		if _, err := tx.Exec(`UPDATE fixkosten_eingaben SET `+s.Column+` = 1 WHERE id = ? AND `+s.Column+` = 0`, id); err != nil {
			return err
		}
	}
	return nil
}
