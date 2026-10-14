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
//
// The optional Wallbox-Einzelzähler (#224) are the last columns. A cell left
// empty means "nicht erfasst". An import also accepts older files without
// them: csvImportHeader lists only the required columns.
//
// After them come the per-Ablesung switches (store.Switches: "Wallboxen
// vollständig Wohnung 1" #225, "PV-Gutschrift Wohnung 1" #226), 1 or 0 each.
// They are optional on import too (empty or missing = the switch's default).
var csvHeader = append(append(append(append([]string{"reading_date", "monat"}, store.MeterKeys...),
	"strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis",
	"personen_1", "personen_2",
), store.OptionalMeterKeys...), csvSwitchColumns()...)

func csvSwitchColumns() []string {
	cols := make([]string, len(store.Switches))
	for i, s := range store.Switches {
		cols[i] = s.CSVColumn
	}
	return cols
}

// csvImportHeader is csvHeader without the optional columns, the columns an
// import insists on.
var csvImportHeader = csvHeader[:len(csvHeader)-len(store.OptionalMeterKeys)-len(store.Switches)]

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
		formatDecimalDE(store.OrZero(p.EinspeisungPreis)),
		formatDecimalDE(p.PersonenByApartment[1]),
		formatDecimalDE(p.PersonenByApartment[2]),
	)
	for _, key := range store.OptionalMeterKeys {
		if v, ok := p.Readings[key]; ok {
			row = append(row, formatDecimalDE(v))
		} else {
			row = append(row, "")
		}
	}
	for _, s := range store.Switches {
		row = append(row, formatSwitchCSV(*s.Detail(p)))
	}
	return row
}

func formatSwitchCSV(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

// parseSwitchCSV reads an optional 0/1 switch column: a missing column or an
// empty cell means the switch's default.
func parseSwitchCSV(record []string, colIdx map[string]int, s store.Switch, line int) (bool, error) {
	idx, ok := colIdx[s.CSVColumn]
	if !ok || idx >= len(record) {
		return s.Default, nil
	}
	switch strings.TrimSpace(record[idx]) {
	case "":
		return s.Default, nil
	case "0":
		return false, nil
	case "1":
		return true, nil
	}
	return false, fmt.Errorf("Zeile %d: ungültiger Wert für %s: %q (erlaubt: 0 oder 1)", line, s.CSVColumn, record[idx])
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
			Header: csvImportHeader,
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
		Header:      csvImportHeader,
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
	// Optional Einzelzähler: the column may be missing (older file) or the
	// cell empty, both mean "nicht erfasst".
	for _, key := range store.OptionalMeterKeys {
		idx, ok := colIdx[key]
		if !ok || idx >= len(record) || strings.TrimSpace(record[idx]) == "" {
			continue
		}
		v, err := parseDecimalDE(record[idx])
		if err != nil {
			return importRow{}, fmt.Errorf("Zeile %d: ungültiger Wert für %s: %q", line, key, record[idx])
		}
		readings[key] = v
	}

	switches := make([]bool, len(store.Switches))
	for i, s := range store.Switches {
		on, err := parseSwitchCSV(record, colIdx, s, line)
		if err != nil {
			return importRow{}, err
		}
		switches[i] = on
	}

	strompreis, err1 := parseDecimalDE(cell("strompreis"))
	frischwasserPreis, err2 := parseDecimalDE(cell("frischwasser_preis"))
	abwasserPreis, err3 := parseDecimalDE(cell("abwasser_preis"))
	einspeisungPreis, err4 := parseDecimalDE(cell("einspeisung_preis"))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return importRow{}, fmt.Errorf("Zeile %d: ungültiger Preiswert", line)
	}

	personen := make(map[int64]float64, 2)
	for _, id := range [2]int64{1, 2} {
		personenCol := fmt.Sprintf("personen_%d", id)
		p, err := parseImportPersonenzahl(cell(personenCol), line, id)
		if err != nil {
			return importRow{}, err
		}
		personen[id] = p
	}

	input := store.PeriodInput{
		ReadingDate:       readingDate,
		Monat:             monat,
		Strompreis:        store.Float64(strompreis),
		FrischwasserPreis: store.Float64(frischwasserPreis),
		AbwasserPreis:     store.Float64(abwasserPreis),
		EinspeisungPreis:  store.Float64(einspeisungPreis),
		Readings:          readings,
		Personen:          personen,
	}
	for i, s := range store.Switches {
		*s.Input(&input) = switches[i]
	}
	return importRow{input: input, line: line}, nil
}

// warnedMeterKeys are the meters the negative-consumption/outlier warnings
// look at: the required ones, then the optional Einzelzähler.
var warnedMeterKeys = append(append([]string{}, store.MeterKeys...), store.OptionalMeterKeys...)

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
			for _, key := range warnedMeterKeys {
				newVal, prevVal := row.input.Readings[key], prev.Readings[key]
				// An optional Einzelzähler without both readings has no
				// consumption, so there is nothing to warn about (#224).
				if _, ok := store.VerbrauchOptional(row.input.Readings, prev.Readings, key); store.IsOptionalMeter(key) && !ok {
					continue
				}
				if newVal < prevVal {
					warnings = append(warnings, fmt.Sprintf("Zeile %d (%s): negativer Verbrauch bei %s (%s < Vorstand %s)",
						row.line, row.input.ReadingDate, key, formatDecimalDE(newVal), formatDecimalDE(prevVal)))
				}
			}
			if avg, ok := outlierAvg(history); ok {
				for _, key := range warnedMeterKeys {
					a := avg[key] // absent (no baseline) reads as 0 and is skipped
					if a == 0 {
						continue
					}
					if _, ok := store.VerbrauchOptional(row.input.Readings, prev.Readings, key); store.IsOptionalMeter(key) && !ok {
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
