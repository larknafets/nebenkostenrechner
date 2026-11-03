package store

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Art values for sonderkosten.art.
const (
	ArtReparatur      = "reparatur"
	ArtNeuanschaffung = "neuanschaffung"
	ArtWartung        = "wartung"
	ArtModernisierung = "modernisierung"
	ArtSonstiges      = "sonstiges"
)

// Umsetzung values for sonderkosten.umsetzung: who carries the bill.
const (
	UmsetzungHaus     = "haus"
	UmsetzungWohnung1 = "wohnung1"
	UmsetzungWohnung2 = "wohnung2"
)

// Auswahl is one choice of a fixed list: the stored key and its German label.
type Auswahl struct{ Wert, Label string }

// SonderkostenArten lists the Arten in display order.
var SonderkostenArten = []Auswahl{
	{ArtReparatur, "Reparatur"},
	{ArtNeuanschaffung, "Neuanschaffung"},
	{ArtWartung, "Wartung"},
	{ArtModernisierung, "Modernisierung"},
	{ArtSonstiges, "Sonstiges"},
}

// SonderkostenUmsetzungen lists the Umsetzungen in display order.
var SonderkostenUmsetzungen = []Auswahl{
	{UmsetzungHaus, "Haus"},
	{UmsetzungWohnung1, "Wohnung 1"},
	{UmsetzungWohnung2, "Wohnung 2"},
}

// Sonderkosten is one invoice outside the Kostenpositionen (GLOSSARY.md).
// Monat is the Abrechnungsmonat ("YYYY-MM-01"), Betrag the gross amount.
// Logik is set only for Umsetzung Haus and Umlegbar, empty otherwise.
type Sonderkosten struct {
	ID              int64
	Monat           string
	Betreff         string
	Beschreibung    string
	Art             string
	Firma           string
	Rechnungsnummer string
	Betrag          float64
	Umsetzung       string
	Umlegbar        bool
	Logik           string
}

// BrauchtFixkosteneingabe is the one place of the rule: an entry that is
// umlegbar, assigned to the Haus and split by Je Anzahl Personen needs the
// Fixkosteneingabe of its Abrechnungsmonat, without it it counts nowhere.
func (s Sonderkosten) BrauchtFixkosteneingabe() bool {
	return s.Umlegbar && s.Umsetzung == UmsetzungHaus && s.Logik == LogikPersonen
}

// SonderkostenFehler is the error of a write that failed validation, one
// German message per violation (see PruefeSonderkosten).
type SonderkostenFehler struct{ Meldungen []string }

func (e *SonderkostenFehler) Error() string { return strings.Join(e.Meldungen, " ") }

// sonderkostenDB is the part of *sql.DB and *sql.Tx the writes need.
type sonderkostenDB interface {
	Exec(string, ...any) (sql.Result, error)
	QueryRow(string, ...any) *sql.Row
}

// CreateSonderkosten validates s (*SonderkostenFehler) and inserts it.
func CreateSonderkosten(db *sql.DB, s Sonderkosten) (int64, error) {
	return insertSonderkosten(db, s)
}

// PruefeSonderkosten returns the validation messages for s (empty: valid),
// looking up itself whether the Abrechnungsmonat has a Fixkosteneingabe. The
// entry form and the CSV parser call it to answer before they write.
func PruefeSonderkosten(db *sql.DB, s Sonderkosten) ([]string, error) {
	return pruefeSonderkosten(db, s)
}

func pruefeSonderkosten(q sonderkostenDB, s Sonderkosten) ([]string, error) {
	var n int
	if err := q.QueryRow(`SELECT COUNT(*) FROM fixkosten_eingaben WHERE monat = ?`, s.Monat).Scan(&n); err != nil {
		return nil, fmt.Errorf("check fixkosteneingabe %s: %w", s.Monat, err)
	}
	return validiereSonderkosten(s, n > 0), nil
}

// insertSonderkosten validates and inserts, on a *sql.DB or a *sql.Tx.
func insertSonderkosten(e sonderkostenDB, s Sonderkosten) (int64, error) {
	if err := sonderkostenGueltig(e, s); err != nil {
		return 0, err
	}
	res, err := e.Exec(
		`INSERT INTO sonderkosten (monat, betreff, beschreibung, art, firma, rechnungsnummer, betrag, umsetzung, umlegbar, logik)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Monat, s.Betreff, s.Beschreibung, s.Art, s.Firma, s.Rechnungsnummer, s.Betrag, s.Umsetzung, s.Umlegbar, s.Logik,
	)
	if err != nil {
		return 0, fmt.Errorf("insert sonderkosten: %w", err)
	}
	return res.LastInsertId()
}

// ErrSonderkostenNotFound is returned by Update/DeleteSonderkosten for an unknown id.
var ErrSonderkostenNotFound = errors.New("sonderkosten not found")

// sonderkostenGueltig is nil for a valid s, else a *SonderkostenFehler.
func sonderkostenGueltig(q sonderkostenDB, s Sonderkosten) error {
	msgs, err := pruefeSonderkosten(q, s)
	if err != nil {
		return err
	}
	if len(msgs) > 0 {
		return &SonderkostenFehler{Meldungen: msgs}
	}
	return nil
}

// UpdateSonderkosten replaces every field of the entry s.ID. Like Create it
// validates the new values completely (an old invalid entry is fixed by it).
// It works in a settled year as well, there is no lock.
func UpdateSonderkosten(db *sql.DB, s Sonderkosten) error {
	if err := sonderkostenGueltig(db, s); err != nil {
		return err
	}
	res, err := db.Exec(
		`UPDATE sonderkosten SET monat=?, betreff=?, beschreibung=?, art=?, firma=?, rechnungsnummer=?, betrag=?, umsetzung=?, umlegbar=?, logik=? WHERE id=?`,
		s.Monat, s.Betreff, s.Beschreibung, s.Art, s.Firma, s.Rechnungsnummer, s.Betrag, s.Umsetzung, s.Umlegbar, s.Logik, s.ID,
	)
	return sonderkostenGeaendert(res, err, "update")
}

// DeleteSonderkosten removes the entry id.
func DeleteSonderkosten(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM sonderkosten WHERE id = ?`, id)
	return sonderkostenGeaendert(res, err, "delete")
}

func sonderkostenGeaendert(res sql.Result, err error, was string) error {
	if err != nil {
		return fmt.Errorf("%s sonderkosten: %w", was, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSonderkostenNotFound
	}
	return nil
}

// logikenHaus are the Berechnungslogiken a Haus entry can be split by.
var logikenHaus = []string{LogikWohneinheit, LogikFlurstueck, LogikQM, LogikPersonen}

// validiereSonderkosten checks s and returns one German message per
// violation (empty: valid). hatFixkosteneingabe says whether the
// Abrechnungsmonat of s has a Fixkosteneingabe, the Personen-Logik needs its
// Personen. Pure, the writes reach it through PruefeSonderkosten.
func validiereSonderkosten(s Sonderkosten, hatFixkosteneingabe bool) []string {
	var fehler []string
	if !validAbrechnungsmonat(s.Monat) {
		fehler = append(fehler, "Der Abrechnungsmonat ist ungültig.")
	}
	if strings.TrimSpace(s.Betreff) == "" {
		fehler = append(fehler, "Der Betreff fehlt.")
	}
	if !slices.ContainsFunc(SonderkostenArten, func(a Auswahl) bool { return a.Wert == s.Art }) {
		fehler = append(fehler, "Die Art ist ungültig.")
	}
	if !slices.ContainsFunc(SonderkostenUmsetzungen, func(a Auswahl) bool { return a.Wert == s.Umsetzung }) {
		fehler = append(fehler, "Die Umsetzung ist ungültig.")
	}
	// Finite and whole cents: Inf passes "> 0", and a fraction of a cent breaks
	// the exact split of the Betrag onto both apartments.
	if !(s.Betrag > 0) || math.IsInf(s.Betrag, 0) || s.Betrag != math.Round(s.Betrag*100)/100 {
		fehler = append(fehler, "Der Betrag muss größer als 0 sein und höchstens zwei Nachkommastellen haben.")
	}
	switch {
	case s.Umsetzung == UmsetzungHaus && s.Umlegbar:
		switch {
		case !slices.Contains(logikenHaus, s.Logik):
			fehler = append(fehler, "Die Berechnungslogik fehlt oder ist ungültig.")
		case s.BrauchtFixkosteneingabe() && !hatFixkosteneingabe:
			fehler = append(fehler, "Je Anzahl Personen geht nur, wenn der Abrechnungsmonat eine Fixkosteneingabe hat.")
		}
	case s.Logik != "":
		fehler = append(fehler, "Die Berechnungslogik gibt es nur bei Umsetzung Haus und umlegbar.")
	}
	return fehler
}

// AllSonderkosten reads every entry, oldest Abrechnungsmonat first, then id.
func AllSonderkosten(db *sql.DB) ([]Sonderkosten, error) {
	rows, err := db.Query(
		`SELECT id, monat, betreff, beschreibung, art, firma, rechnungsnummer, betrag, umsetzung, umlegbar, logik
		 FROM sonderkosten ORDER BY monat, id`)
	if err != nil {
		return nil, fmt.Errorf("query sonderkosten: %w", err)
	}
	defer rows.Close()
	var out []Sonderkosten
	for rows.Next() {
		var s Sonderkosten
		if err := rows.Scan(&s.ID, &s.Monat, &s.Betreff, &s.Beschreibung, &s.Art, &s.Firma, &s.Rechnungsnummer, &s.Betrag, &s.Umsetzung, &s.Umlegbar, &s.Logik); err != nil {
			return nil, fmt.Errorf("scan sonderkosten: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
