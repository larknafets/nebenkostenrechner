package web

import (
	"database/sql"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// csvHeader is the canonical CSV column order for both export (Ticket #53)
// and import (Ticket #54) - reading_date, every meter key (meter readings,
// not consumption), the period-level prices/weighting, then occupant count
// per apartment (fixed ids 1/2, see store's seed()). No qm_1/qm_2 columns
// (Issue #61 moved apartment size off the reading onto /stammdaten - hard
// cut, no backward compatibility with the old format).
var csvHeader = append(append([]string{"reading_date", "monat"}, store.MeterKeys...),
	"strompreis", "frischwasser_preis", "abwasser_preis", "heizung_gewichtung", "einspeisung_preis",
	"personen_1", "personen_2",
)

// handleExportCSV streams every reading as CSV (Ticket #53).
func handleExportCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		details, err := store.AllPeriodDetails(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "periods: "+err.Error(), http.StatusInternalServerError)
			return
		}
		RunCSVExport(w, "ablesungen.csv", csvHeader, details, formatPeriodCSVRow)
	}
}

func formatPeriodCSVRow(p *store.LatestPeriod) []string {
	row := make([]string, 0, len(csvHeader))
	row = append(row, p.ReadingDate, p.Monat)
	for _, key := range store.MeterKeys {
		row = append(row, formatDecimalDE(p.Readings[key]))
	}
	row = append(row,
		// store.OrZero: a partial reading (Ticket #128) exports its missing
		// price here as "0" - CSV import/export of/for partial readings is
		// explicitly not part of this spec (#127), so this can't actually
		// occur in practice yet.
		formatDecimalDE(store.OrZero(p.Strompreis)),
		formatDecimalDE(store.OrZero(p.FrischwasserPreis)),
		formatDecimalDE(store.OrZero(p.AbwasserPreis)),
		formatDecimalDE(p.HeizungWaermeGewichtung),
		formatDecimalDE(store.OrZero(p.EinspeisungPreis)),
		formatDecimalDE(float64(p.PersonenByApartment[1])),
		formatDecimalDE(float64(p.PersonenByApartment[2])),
	)
	return row
}

// importRow pairs a parsed PeriodInput with its original CSV line number,
// so warnings can still point at the uploaded file after the rows are
// re-sorted into chronological order.
type importRow struct {
	input store.PeriodInput
	line  int
}

// handleImportCSV bootstraps a completely empty database from a CSV in the
// csvHeader format (Ticket #54) - rejected if any reading already exists,
// even though the form button is already hidden in that case (defense in
// depth). A hard error in any row aborts the whole import (all or
// nothing); negative-consumption/outlier warnings never block, just get
// reported afterwards on the readings overview.
func handleImportCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())

		cfg := ImportConfig[importRow]{
			Header: csvHeader,
			CheckExisting: func(db *sql.DB) (bool, error) {
				existing, err := store.AllPeriods(db)
				return len(existing) > 0, err
			},
			ExistingErrMsg: "Import nur möglich, solange noch keine Ablesung existiert",
			ParseRow:       parseImportRow,
			EmptyErrMsg:    "CSV enthält keine Ablesungen",
			SortKey:        func(row importRow) string { return row.input.ReadingDate },
			Insert: func(db *sql.DB, rows []importRow) ([]int64, error) {
				inputs := make([]store.PeriodInput, len(rows))
				for i, row := range rows {
					inputs[i] = row.input
				}
				return store.ImportPeriods(db, inputs)
			},
		}

		ids, rows, ok := RunCSVImport(w, r, db, cfg)
		if !ok {
			return
		}

		warnings := importWarnings(rows, ids)
		redirectURL := fmt.Sprintf("%s/ablesungen?imported=%d", requestBase(r), len(ids))
		for _, msg := range warnings {
			redirectURL += "&warning=" + url.QueryEscape(msg)
		}
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// parseImportCSV reads csvHeader-formatted CSV (semicolon-separated,
// comma-decimal, optional UTF-8 BOM) and validates every row into an
// importRow, without touching the DB or HTTP layer - kept as a leaf entry
// point so row-parsing tests don't need a *http.Request/*sql.DB.
func parseImportCSV(file io.Reader) ([]importRow, error) {
	return parseImportCSVRows(file, ImportConfig[importRow]{
		Header:      csvHeader,
		ParseRow:    parseImportRow,
		EmptyErrMsg: "CSV enthält keine Ablesungen",
	})
}

// parseImportRow validates one CSV record (semicolon-separated, comma-decimal)
// into a PeriodInput, paired with its line number for later warnings.
func parseImportRow(record []string, colIdx map[string]int, line int) (importRow, error) {
	cell := func(col string) string { return record[colIdx[col]] }

	readingDate := strings.TrimSpace(cell("reading_date"))
	if _, err := time.Parse("2006-01-02", readingDate); err != nil {
		return importRow{}, fmt.Errorf("Zeile %d: ungültiges Ablesedatum %q (Format JJJJ-MM-TT)", line, readingDate)
	}

	monat := strings.TrimSpace(cell("monat"))
	if _, err := time.Parse("2006-01-02", monat); err != nil {
		return importRow{}, fmt.Errorf("Zeile %d: ungültiger Abrechnungsmonat %q (Format JJJJ-MM-01)", line, monat)
	}

	readings := make(map[string]float64, len(store.MeterKeys))
	for _, key := range store.MeterKeys {
		v, err := parseDecimalDE(cell(key))
		if err != nil {
			return importRow{}, fmt.Errorf("Zeile %d: ungültiger Wert für %s: %q", line, key, cell(key))
		}
		readings[key] = v
	}

	strompreis, err1 := parseDecimalDE(cell("strompreis"))
	frischwasserPreis, err2 := parseDecimalDE(cell("frischwasser_preis"))
	abwasserPreis, err3 := parseDecimalDE(cell("abwasser_preis"))
	einspeisungPreis, err4 := parseDecimalDE(cell("einspeisung_preis"))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return importRow{}, fmt.Errorf("Zeile %d: ungültiger Preiswert", line)
	}

	heizungGewichtung, err := parseHeizungGewichtung(strings.ReplaceAll(cell("heizung_gewichtung"), ",", "."))
	if err != nil {
		return importRow{}, fmt.Errorf("Zeile %d: %v", line, err)
	}

	personen := make(map[int64]int64, 2)
	for _, id := range [2]int64{1, 2} {
		personenCol := fmt.Sprintf("personen_%d", id)
		p, err := parseDecimalDE(cell(personenCol))
		if err != nil {
			return importRow{}, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q", line, id, cell(personenCol))
		}
		personen[id] = int64(p)
	}

	return importRow{
		input: store.PeriodInput{
			ReadingDate:             readingDate,
			Monat:                   monat,
			Strompreis:              store.Float64(strompreis),
			FrischwasserPreis:       store.Float64(frischwasserPreis),
			AbwasserPreis:           store.Float64(abwasserPreis),
			HeizungWaermeGewichtung: heizungGewichtung,
			EinspeisungPreis:        store.Float64(einspeisungPreis),
			Readings:                readings,
			Personen:                personen,
		},
		line: line,
	}, nil
}

// importWarnings reproduces the wizard's client-side negative-consumption/
// outlier checks server-side (Ticket #54) - the bulk import has no
// per-field JS to run them, but a bulk import of historical data is exactly
// where a typo is easiest to miss. rows must already be chronologically
// sorted, ids in the same order (ImportPeriods preserves it).
func importWarnings(rows []importRow, ids []int64) []string {
	var warnings []string
	var history []store.PeriodReadings // newest-first, capped at 4

	for i, row := range rows {
		if i > 0 {
			prev := history[0]
			for _, key := range store.MeterKeys {
				newVal, prevVal := row.input.Readings[key], prev.Readings[key]
				if newVal < prevVal {
					warnings = append(warnings, fmt.Sprintf("Zeile %d (%s): negativer Verbrauch bei %s (%s < Vorstand %s)",
						row.line, row.input.ReadingDate, key, formatDecimalDE(newVal), formatDecimalDE(prevVal)))
				}
			}
			if avg, ok := outlierAvg(history); ok {
				for _, key := range store.MeterKeys {
					a := avg[key]
					if a == 0 {
						continue
					}
					consumption := row.input.Readings[key] - prev.Readings[key]
					if math.Abs(consumption-a) > 0.5*math.Abs(a) {
						warnings = append(warnings, fmt.Sprintf("Zeile %d (%s): Ausreißer bei %s (Verbrauch %s weicht >50%% vom Schnitt der letzten 3 Ablesungen %s ab)",
							row.line, row.input.ReadingDate, key, formatDecimalDE(consumption), formatDecimalDE(a)))
					}
				}
			}
		}

		history = append([]store.PeriodReadings{{ID: ids[i], ReadingDate: row.input.ReadingDate, Readings: row.input.Readings}}, history...)
		if len(history) > 4 {
			history = history[:4]
		}
	}
	return warnings
}
