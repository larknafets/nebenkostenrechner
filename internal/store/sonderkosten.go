package store

import (
	"database/sql"
	"errors"
	"fmt"
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

// CreateSonderkosten inserts s and returns its id. It validates nothing:
// the entry form and the CSV import check before they call it.
func CreateSonderkosten(db *sql.DB, s Sonderkosten) (int64, error) {
	res, err := db.Exec(
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

// UpdateSonderkosten replaces every field of the entry s.ID. Like Create it
// validates nothing. It works in a settled year as well, there is no lock.
func UpdateSonderkosten(db *sql.DB, s Sonderkosten) error {
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

// ValidiereSonderkosten checks s and returns one German message per
// violation (empty: valid). hatFixkosteneingabe says whether the
// Abrechnungsmonat of s has a Fixkosteneingabe, the Personen-Logik needs its
// Personen. The entry form and the CSV import both call it.
func ValidiereSonderkosten(s Sonderkosten, hatFixkosteneingabe bool) []string {
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
	if !(s.Betrag > 0) {
		fehler = append(fehler, "Der Betrag muss größer als 0 sein.")
	}
	switch {
	case s.Umsetzung == UmsetzungHaus && s.Umlegbar:
		switch {
		case !slices.Contains(logikenHaus, s.Logik):
			fehler = append(fehler, "Die Berechnungslogik fehlt oder ist ungültig.")
		case s.Logik == LogikPersonen && !hatFixkosteneingabe:
			fehler = append(fehler, "Je Anzahl Personen geht nur, wenn der Abrechnungsmonat eine Fixkosteneingabe hat.")
		}
	case s.Logik != "":
		fehler = append(fehler, "Die Berechnungslogik gibt es nur bei Umsetzung Haus und umlegbar.")
	}
	return fehler
}

// MonatHatFixkosteneingabe reports whether monat ("YYYY-MM-01") has a Fixkosteneingabe.
func MonatHatFixkosteneingabe(db *sql.DB, monat string) (bool, error) {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM fixkosten_eingaben WHERE monat = ?`, monat).Scan(&n); err != nil {
		return false, fmt.Errorf("check fixkosteneingabe %s: %w", monat, err)
	}
	return n > 0, nil
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
