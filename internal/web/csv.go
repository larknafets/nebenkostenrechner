package web

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// csvHeader is the canonical CSV column order for both export (Ticket #53)
// and import (Ticket #54) - reading_date, monat, every meter key (meter
// readings, not consumption), then the optional Wallbox-Einzelzähler (#224).
// A cell left empty means "nicht erfasst". An import also accepts older files
// without the Einzelzähler: csvImportHeader lists only the required columns.
//
// Prices, Personenzahl and the switches belong to the Fixkosten-Eingabe (#359).
// The export no longer writes them; an import accepts the columns of an older
// file (csvVerworfeneSpalten) and drops their values.
var csvHeader = append(append([]string{"reading_date", "monat"}, store.MeterKeys...), store.OptionalMeterKeys...)

// csvImportHeader is csvHeader without the optional columns, the columns an
// import insists on: Ablesedatum, Abrechnungsmonat and the Zähler.
var csvImportHeader = csvHeader[:len(csvHeader)-len(store.OptionalMeterKeys)]

// csvVerworfeneSpalten are the columns of the old Ablesungs-CSV that moved to
// the Fixkosten-Eingabe: the four prices, the Personenzahl per apartment and the
// switches. An import accepts them and drops their values.
var csvVerworfeneSpalten = func() []string {
	var cols []string
	for _, p := range store.Preise {
		cols = append(cols, p.Spalte)
	}
	cols = append(cols, "personen_1", "personen_2")
	for _, s := range store.Switches {
		cols = append(cols, s.CSVColumn)
	}
	return cols
}()

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
	for _, key := range store.OptionalMeterKeys {
		if v, ok := p.Readings[key]; ok {
			row = append(row, formatDecimalDE(v))
		} else {
			row = append(row, "")
		}
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

// hatVerworfeneWerte reports whether the row has a value in a column of the
// old format that moved to the Fixkosten-Eingabe.
func hatVerworfeneWerte(record []string, colIdx map[string]int) bool {
	for _, col := range csvVerworfeneSpalten {
		if idx, ok := colIdx[col]; ok && idx < len(record) && strings.TrimSpace(record[idx]) != "" {
			return true
		}
	}
	return false
}

// importRow pairs a parsed PeriodInput with its original CSV line number,
// so warnings can still point at the uploaded file after the rows are
// re-sorted into chronological order.
type importRow struct {
	input store.PeriodInput
	line  int
	// verworfen is true if the row had a value in a column that moved to the
	// Fixkosten-Eingabe (prices, Personenzahl, switches), dropped on import.
	verworfen bool
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
				ids, err := store.ImportPeriods(db, inputs)
				var row *store.ImportRowError
				if errors.As(err, &row) {
					var ef *store.EingabeFehler
					if errors.As(row.Err, &ef) && row.Index < len(rows) {
						// parseImportRow validates already, so this is a safety net.
						return nil, &importEingabeFehler{msg: ablesungCsvFehler(ef, rows[row.Index].line).Error()}
					}
					if msg, _, ok := reiheFehler(row.Err); ok {
						return nil, &importEingabeFehler{msg: fmt.Sprintf("Ablesung vom %s: %s", row.ReadingDate, msg)}
					}
					if open := new(store.PeriodOpenTeilstandError); errors.As(row.Err, &open) {
						return nil, &importEingabeFehler{msg: fmt.Sprintf("Ablesung vom %s: die vorherige Ablesung ist ein Teilstand, nur die neueste darf eines sein", row.ReadingDate)}
					}
				}
				return ids, err
			},
		}

		ids, rows, ok := RunCSVImport(w, r, db, cfg)
		if !ok {
			return
		}

		hinweise, err := importHinweise(db, rows)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		warnings := append(hinweise, importWarnings(rows, ids)...)
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
	monat := strings.TrimSpace(cell("monat"))
	// The store accepts an empty Monat (Teilstand), the CSV does not.
	if monat == "" {
		return importRow{}, ablesungCsvFehler(&store.EingabeFehler{Feld: store.FeldMonat}, line)
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

	input := store.PeriodInput{
		ReadingDate: readingDate,
		Monat:       monat,
		Readings:    readings,
	}
	if err := input.Validate(); err != nil {
		return importRow{}, ablesungCsvFehler(err, line)
	}
	return importRow{input: input, line: line, verworfen: hatVerworfeneWerte(record, colIdx)}, nil
}

// ablesungCsvFehler turns the *store.EingabeFehler of PeriodInput.Validate into
// the German CSV message with the line number.
func ablesungCsvFehler(err error, line int) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return err
	}
	switch ef.Feld {
	case store.FeldReadingDate:
		return fmt.Errorf("Zeile %d: ungültiges Ablesedatum %q (Format JJJJ-MM-TT)", line, ef.Wert)
	case store.FeldMonat:
		return fmt.Errorf("Zeile %d: ungültiger Abrechnungsmonat %q (Format JJJJ-MM-01)", line, ef.Wert)
	}
	return err
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
				// An optional Einzelzähler without both readings has no
				// consumption, so there is nothing to warn about (#224).
				consumption, ok := warnVerbrauch(row.input.Readings, prev.Readings, key)
				if !ok {
					continue
				}
				if consumption < 0 {
					warnings = append(warnings, fmt.Sprintf("Zeile %d (%s): negativer Verbrauch bei %s (%s < Vorstand %s)",
						row.line, row.input.ReadingDate, key, formatDecimalDE(row.input.Readings[key]), formatDecimalDE(prev.Readings[key])))
				}
			}
			if avg, ok := outlierAvg(history); ok {
				for _, key := range warnedMeterKeys {
					a := avg[key] // absent (no baseline) reads as 0 and is skipped
					if a == 0 {
						continue
					}
					consumption, ok := warnVerbrauch(row.input.Readings, prev.Readings, key)
					if !ok {
						continue
					}
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

// importHinweise are the two notes after an import (#359), one message each:
// the Abrechnungsmonate of the imported Ablesungen that have no complete
// Fixkosten-Eingabe, and that the file held prices, Personenzahlen or switches,
// which are no longer taken over.
func importHinweise(db *sql.DB, rows []importRow) ([]string, error) {
	v, err := calc.Load(db)
	if err != nil {
		return nil, err
	}
	importiert := make(map[string]bool, len(rows))
	verworfen := false
	for _, row := range rows {
		importiert[row.input.Monat] = true
		verworfen = verworfen || row.verworfen
	}
	var fehlen []string
	for _, o := range v.OffeneMonate() {
		if importiert[o.Monat] && o.Monat != "" {
			fehlen = append(fehlen, o.Monat)
		}
	}
	var hinweise []string
	if len(fehlen) > 0 {
		hinweise = append(hinweise, fixkostenFehlenSatz(fehlen)+" Die Ablesungen dieser Monate sind noch nicht berechnet, bis die Fixkosten erfasst sind.")
	}
	if verworfen {
		hinweise = append(hinweise, "Die Datei enthält Preise, Personenzahlen oder Häkchen. Sie gehören zur Fixkosten-Eingabe und werden nicht übernommen.")
	}
	return hinweise, nil
}
