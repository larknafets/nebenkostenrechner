package store

import (
	"database/sql"
	"fmt"
)

// Recht is one right the Vermieter can grant to the roles Nicht angemeldet and
// Mieter with a checkbox (GLOSSARY.md, decision "Berechtigungsmatrix"). The
// Vermieter always has every right, so there is no Vermieter column.
type Recht string

const (
	RechtWohnung1          Recht = "wohnung1"
	RechtWohnung2          Recht = "wohnung2"
	RechtWaermepumpe       Recht = "waermepumpe"
	RechtWallboxen         Recht = "wallboxen"
	RechtPVAnlage          Recht = "pv_anlage"
	RechtAblesungenAnsehen Recht = "ablesungen_ansehen"
	RechtAblesungAnlegen   Recht = "ablesung_anlegen"
	RechtStammdatenAnsehen Recht = "stammdaten_ansehen"
	RechtFixkostenAnsehen  Recht = "fixkosten_ansehen"
	RechtMiete             Recht = "miete"
)

// Rolle is a role whose rights are configurable.
type Rolle string

const (
	RolleNichtAngemeldet Rolle = "nicht_angemeldet"
	RolleMieter          Rolle = "mieter"
)

// Rollen lists the configurable roles in display order.
var Rollen = []Rolle{RolleNichtAngemeldet, RolleMieter}

// RechtInfo describes one right: its label and the default per role, which is
// the behavior before the Berechtigungen existed (plus the new Wärmepumpe), so
// an update breaks nothing.
type RechtInfo struct {
	Recht                  Recht
	Label                  string
	DefaultNichtAngemeldet bool
	DefaultMieter          bool
}

// Rechte lists every configurable right in display order, the one place of
// the matrix.
var Rechte = []RechtInfo{
	{RechtWohnung1, "Wohnung 1 (Karte, Monatsübersicht)", false, false},
	{RechtWohnung2, "Wohnung 2 (Karte, Monatsübersicht)", true, true},
	{RechtWaermepumpe, "Wärmepumpe (Karte, Monatsübersicht)", true, true},
	{RechtWallboxen, "Wallboxen (Karte, Monatsübersicht)", false, false},
	{RechtPVAnlage, "PV-Anlage (Karte, Monatsübersicht)", false, false},
	{RechtAblesungenAnsehen, "Ablesungen ansehen", true, true},
	{RechtAblesungAnlegen, "Ablesung anlegen, Teilstand vervollständigen", true, true},
	{RechtStammdatenAnsehen, "Stammdaten ansehen (ohne Personenbezug)", true, true},
	{RechtFixkostenAnsehen, "Fixkosten ansehen", true, true},
	{RechtMiete, "Miete (Block in der Jahreskarte)", false, false},
}

// Default returns the default of the right for the role.
func (i RechtInfo) Default(r Rolle) bool {
	if r == RolleMieter {
		return i.DefaultMieter
	}
	return i.DefaultNichtAngemeldet
}

// Berechtigungen holds, per role, which rights are granted. A value from
// GetBerechtigungen or DefaultBerechtigungen is complete: every right of Rechte
// has an entry for every role of Rollen.
type Berechtigungen map[Rolle]map[Recht]bool

// DefaultBerechtigungen returns the defaults of every right.
func DefaultBerechtigungen() Berechtigungen {
	b := make(Berechtigungen, len(Rollen))
	for _, rolle := range Rollen {
		b[rolle] = make(map[Recht]bool, len(Rechte))
		for _, info := range Rechte {
			b[rolle][info.Recht] = info.Default(rolle)
		}
	}
	return b
}

// Erlaubt reports whether the role has the right. Unknown roles and rights
// have nothing.
func (b Berechtigungen) Erlaubt(rolle Rolle, recht Recht) bool {
	return b[rolle][recht]
}

// GetBerechtigungen reads the saved rights, filling every right that was never
// saved with its default.
func GetBerechtigungen(db *sql.DB) (Berechtigungen, error) {
	return getBerechtigungen(db)
}

func getBerechtigungen(q querier) (Berechtigungen, error) {
	b := DefaultBerechtigungen()
	rows, err := q.Query(`SELECT recht, rolle, erlaubt FROM berechtigungen`)
	if err != nil {
		return nil, fmt.Errorf("query berechtigungen: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var recht, rolle string
		var erlaubt bool
		if err := rows.Scan(&recht, &rolle, &erlaubt); err != nil {
			return nil, fmt.Errorf("scan berechtigung: %w", err)
		}
		// A saved right that no longer exists is ignored.
		if _, ok := b[Rolle(rolle)][Recht(recht)]; ok {
			b[Rolle(rolle)][Recht(recht)] = erlaubt
		}
	}
	return b, rows.Err()
}

// saveBerechtigungenTx writes every right of every configurable role. Keys the
// matrix does not know are not written.
func saveBerechtigungenTx(tx *sql.Tx, b Berechtigungen) error {
	for _, rolle := range Rollen {
		for _, info := range Rechte {
			if _, err := tx.Exec(
				`INSERT INTO berechtigungen (recht, rolle, erlaubt) VALUES (?, ?, ?)
				 ON CONFLICT(recht, rolle) DO UPDATE SET erlaubt = excluded.erlaubt`,
				string(info.Recht), string(rolle), b[rolle][info.Recht],
			); err != nil {
				return fmt.Errorf("save berechtigung %s/%s: %w", rolle, info.Recht, err)
			}
		}
	}
	return nil
}
