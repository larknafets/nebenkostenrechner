package store

import (
	"database/sql"
	"fmt"
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
