package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenImportRow keeps the CSV line next to the entry so the sort by
// month stays stable for entries of the same month.
type sonderkostenImportRow struct {
	s    store.Sonderkosten
	line int
}

// handleSonderkostenImport takes over Sonderkosten from a CSV in the export
// format (#409), only while none exist. All or nothing, the first bad row
// aborts with its line number. Each row goes through the same
// store.PruefeSonderkosten as the entry form.
func handleSonderkostenImport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		cfg := ImportConfig[sonderkostenImportRow]{
			Header: sonderkostenCsvHeader,
			CheckExisting: func(db *sql.DB) (bool, error) {
				alle, err := store.AllSonderkosten(db)
				return len(alle) > 0, err
			},
			ExistingErrMsg: "Import nur möglich, solange noch keine Sonderkosten existieren",
			ParseRow: func(record []string, colIdx map[string]int, line int) (sonderkostenImportRow, error) {
				return parseSonderkostenImportRow(db, record, colIdx, line)
			},
			EmptyErrMsg: "CSV enthält keine Sonderkosten",
			SortKey:     func(row sonderkostenImportRow) string { return fmt.Sprintf("%s%08d", row.s.Monat, row.line) },
			Insert: func(db *sql.DB, rows []sonderkostenImportRow) ([]int64, error) {
				eintraege := make([]store.Sonderkosten, len(rows))
				for i, row := range rows {
					eintraege[i] = row.s
				}
				ids, err := store.ImportSonderkosten(db, eintraege)
				var fehler *store.SonderkostenFehler
				if errors.As(err, &fehler) { // race after the per-row pre-check
					return nil, &importEingabeFehler{msg: strings.Join(fehler.Meldungen, " ")}
				}
				return ids, err
			},
		}
		if _, _, ok := RunCSVImport(w, r, db, cfg); !ok {
			return
		}
		http.Redirect(w, r, requestBase(r)+"/sonderkosten", http.StatusFound)
	}
}

func parseSonderkostenImportRow(db *sql.DB, record []string, colIdx map[string]int, line int) (sonderkostenImportRow, error) {
	cell := func(col string) string { return strings.TrimSpace(record[colIdx[col]]) }
	fehler := func(msg string) (sonderkostenImportRow, error) {
		return sonderkostenImportRow{}, fmt.Errorf("Zeile %d: %s", line, msg)
	}
	s := store.Sonderkosten{
		Monat: cell("monat"), Betreff: csvTextLesen(cell("betreff")), Beschreibung: csvTextLesen(cell("beschreibung")),
		Firma: csvTextLesen(cell("firma")), Rechnungsnummer: csvTextLesen(cell("rechnungsnummer")), Logik: cell("logik"),
	}
	var ok bool
	if s.Art, ok = sonderkostenArtSchluessel(cell("art")); !ok {
		return fehler(fmt.Sprintf("Die Art ist ungültig: %q", cell("art")))
	}
	if s.Umsetzung, ok = sonderkostenUmsetzungSchluessel(cell("umsetzung")); !ok {
		return fehler(fmt.Sprintf("Die Umsetzung ist ungültig: %q", cell("umsetzung")))
	}
	switch cell("umlegbar") {
	case "1":
		s.Umlegbar = true
	case "0":
	default:
		return fehler(fmt.Sprintf("Umlegbar muss 0 oder 1 sein: %q", cell("umlegbar")))
	}
	var err error
	if s.Betrag, err = parseDecimalDE(cell("betrag")); err != nil {
		return fehler(fmt.Sprintf("Der Betrag ist ungültig: %q", cell("betrag")))
	}
	msgs, err := store.PruefeSonderkosten(db, s)
	if err != nil {
		return sonderkostenImportRow{}, err
	}
	if len(msgs) > 0 {
		return fehler(strings.Join(msgs, " "))
	}
	return sonderkostenImportRow{s: s, line: line}, nil
}
