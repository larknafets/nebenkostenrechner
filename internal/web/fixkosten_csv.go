package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkostenCsvHeader is the canonical CSV column order for both export
// (Issue #132) and import (Issue #133) - monat, then Logik/Typ/Wert per
// Kostenposition (in store.KostenpositionDefaults' fixed order, not the display order, since
// Logik/Typ are editable per Eingabe and can't be omitted like a period's
// static meter keys), then Personen/Abschlag/Miete per apartment (fixed ids
// 1/2, see store's seed()). The export always has all columns, an empty cell
// means "not recorded" (#277).
var fixkostenCsvHeader = buildFixkostenCsvHeader()

// fixkostenCsvMieteSpalten are the columns an import may lack: files exported
// before the Miete existed stay importable.
var fixkostenCsvMieteSpalten = []string{"miete_1", "miete_2"}

func buildFixkostenCsvHeader() []string {
	header := []string{"monat"}
	for _, kd := range store.KostenpositionDefaults {
		header = append(header, kd.Key+"_logik", kd.Key+"_typ", kd.Key+"_wert")
	}
	header = append(header, "personen_1", "personen_2", "abschlag_1", "abschlag_2")
	return append(header, fixkostenCsvMieteSpalten...)
}

// fixkostenCsvPflichtSpalten is the header an import requires: everything
// the export writes except the optional Miete columns.
func fixkostenCsvPflichtSpalten() []string {
	return fixkostenCsvHeader[:len(fixkostenCsvHeader)-len(fixkostenCsvMieteSpalten)]
}

// formatErfasst renders a recorded value, "" for a value that is not recorded.
func formatErfasst(m map[int64]float64, id int64) string {
	v, ok := m[id]
	if !ok {
		return ""
	}
	return formatDecimalDE(v)
}

// parseErfasst reads an optional cell: empty means "not recorded" (ok false).
func parseErfasst(cell string) (v float64, ok bool, err error) {
	if strings.TrimSpace(cell) == "" {
		return 0, false, nil
	}
	v, err = parseDecimalDE(cell)
	return v, err == nil, err
}

// handleExportFixkostenCSV streams every Fixkosten-Eingabe as CSV (Issue #132).
func handleExportFixkostenCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		details, err := store.AllFixkostenEingabenDetails(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "fixkosten eingaben: "+err.Error(), http.StatusInternalServerError)
			return
		}
		RunCSVExport(w, "fixkosten.csv", fixkostenCsvHeader, details, formatFixkostenCSVRow)
	}
}

func formatFixkostenCSVRow(f *store.FixkostenEingabeDetails) []string {
	row := make([]string, 0, len(fixkostenCsvHeader))
	row = append(row, f.Monat)
	for _, kd := range store.KostenpositionDefaults {
		w := f.Werte[kd.ID]
		row = append(row, w.Logik, w.Typ, formatDecimalDE(w.Wert))
	}
	row = append(row,
		formatDecimalDE(f.Personen[1]),
		formatDecimalDE(f.Personen[2]),
		formatErfasst(f.Abschlag, 1),
		formatErfasst(f.Abschlag, 2),
		formatErfasst(f.Miete, 1),
		formatErfasst(f.Miete, 2),
	)
	return row
}

// handleImportFixkostenCSV bootstraps a completely empty Fixkosten history
// from a CSV in the fixkostenCsvHeader format (Issue #133) - rejected if
// any Fixkosten-Eingabe already exists, even though the form button is
// already hidden in that case (defense in depth). A hard error in any row
// aborts the whole import (all or nothing); unlike the Ablesungen import,
// there are no plausibility warnings to report back (Issue #133).
func handleImportFixkostenCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())

		cfg := ImportConfig[fixkostenImportRow]{
			Header: fixkostenCsvPflichtSpalten(),
			CheckExisting: func(db *sql.DB) (bool, error) {
				existing, err := store.AllFixkostenEingaben(db)
				return len(existing) > 0, err
			},
			ExistingErrMsg: "Import nur möglich, solange noch keine Fixkosten-Eingabe existiert",
			ParseRow:       parseImportFixkostenRow,
			EmptyErrMsg:    "CSV enthält keine Fixkosten-Eingaben",
			SortKey:        func(row fixkostenImportRow) string { return row.input.Monat },
			ValidateRows: func(rows []fixkostenImportRow) error {
				return validateFixkostenMonateEindeutig(fixkostenInputs(rows))
			},
			Insert: insertFixkostenRows,
		}

		ids, _, ok := RunCSVImport(w, r, db, cfg)
		if !ok {
			return
		}

		redirectURL := fmt.Sprintf("%s/fixkosten?imported=%d", requestBase(r), len(ids))
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// fixkostenImportRow keeps the CSV line number next to the parsed input, so
// the safety net can still point at the uploaded file after the rows are
// re-sorted into chronological order.
type fixkostenImportRow struct {
	input store.FixkostenInput
	line  int
}

func fixkostenInputs(rows []fixkostenImportRow) []store.FixkostenInput {
	inputs := make([]store.FixkostenInput, len(rows))
	for i, row := range rows {
		inputs[i] = row.input
	}
	return inputs
}

// insertFixkostenRows imports the parsed rows. A rule violation the store
// reports (store.FixkostenImportError) becomes a 400 with the CSV line number.
func insertFixkostenRows(db *sql.DB, rows []fixkostenImportRow) ([]int64, error) {
	ids, err := store.ImportFixkostenEingaben(db, fixkostenInputs(rows))
	var row *store.FixkostenImportError
	if errors.As(err, &row) {
		var ef *store.EingabeFehler
		if errors.As(row.Err, &ef) && row.Index < len(rows) {
			// parseImportFixkostenRow validates already, so this is a safety net.
			return nil, &importEingabeFehler{msg: fixkostenCsvFehler(ef, rows[row.Index].line, nil, nil).Error()}
		}
	}
	return ids, err
}

// validateFixkostenMonateEindeutig rejects a CSV that has more than one row
// for the same month (Issue #161: one Fixkosten-Eingabe per month).
func validateFixkostenMonateEindeutig(rows []store.FixkostenInput) error {
	seen := make(map[string]bool, len(rows))
	var doppelt []string
	for _, in := range rows {
		if seen[in.Monat] {
			doppelt = append(doppelt, store.MonatLabel(in.Monat))
			continue
		}
		seen[in.Monat] = true
	}
	if len(doppelt) > 0 {
		return fmt.Errorf("CSV: je Monat darf es nur eine Fixkosten-Eingabe geben, mehrfach vorhanden: %s", strings.Join(doppelt, ", "))
	}
	return nil
}

// parseImportFixkostenRow validates one CSV record (semicolon-separated,
// comma-decimal) into a FixkostenInput.
func parseImportFixkostenRow(record []string, colIdx map[string]int, line int) (fixkostenImportRow, error) {
	// A column an old file lacks (the Miete columns) reads as an empty cell.
	cell := func(col string) string {
		i, ok := colIdx[col]
		if !ok {
			return ""
		}
		return record[i]
	}

	monat := strings.TrimSpace(cell("monat"))

	werte := make(map[int64]store.FixkostenPositionWert, len(store.KostenpositionDefaults))
	for _, kd := range store.KostenpositionDefaults {
		wert, err := parseDecimalDE(cell(kd.Key + "_wert"))
		if err != nil {
			return fixkostenImportRow{}, fmt.Errorf("Zeile %d: ungültiger Wert für %s: %q", line, kd.Label, cell(kd.Key+"_wert"))
		}
		// Logik and Typ are checked by FixkostenInput.Validate below.
		werte[kd.ID] = store.FixkostenPositionWert{
			Logik: strings.TrimSpace(cell(kd.Key + "_logik")),
			Typ:   strings.TrimSpace(cell(kd.Key + "_typ")),
			Wert:  wert,
		}
	}

	personen := make(map[int64]float64, 2)
	abschlag := make(map[int64]float64, 2)
	miete := make(map[int64]float64, 2)
	for _, id := range [2]int64{1, 2} {
		personenCol := fmt.Sprintf("personen_%d", id)
		p, err := parseDecimalDE(cell(personenCol))
		if err != nil {
			return fixkostenImportRow{}, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q", line, id, cell(personenCol))
		}
		personen[id] = p

		abschlagCol := fmt.Sprintf("abschlag_%d", id)
		// Taken regardless of the apartment's status, empty = not recorded.
		a, ok, err := parseErfasst(cell(abschlagCol))
		if err != nil {
			return fixkostenImportRow{}, fmt.Errorf("Zeile %d: ungültiger Abschlag für Wohnung %d: %q", line, id, cell(abschlagCol))
		}
		if ok {
			abschlag[id] = a
		}

		mieteCol := fmt.Sprintf("miete_%d", id)
		m, ok, err := parseErfasst(cell(mieteCol))
		if err != nil {
			return fixkostenImportRow{}, fmt.Errorf("Zeile %d: ungültige Miete für Wohnung %d: %q", line, id, cell(mieteCol))
		}
		if ok {
			miete[id] = m
		}
	}

	in := store.FixkostenInput{
		Monat:    monat,
		Personen: personen,
		Werte:    werte,
		Abschlag: abschlag,
		Miete:    miete,
	}
	if err := in.Validate(); err != nil {
		return fixkostenImportRow{}, fixkostenCsvFehler(err, line, colIdx, record)
	}
	return fixkostenImportRow{input: in, line: line}, nil
}

// fixkostenCsvFehler words a store.EingabeFehler for the CSV import with the
// line number. Any other error passes through.
func fixkostenCsvFehler(err error, line int, colIdx map[string]int, record []string) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return err
	}
	label := func(id int64) string {
		for _, kd := range store.KostenpositionDefaults {
			if kd.ID == id {
				return kd.Label
			}
		}
		return ""
	}
	switch ef.Feld {
	case store.FeldMonat:
		return fmt.Errorf("Zeile %d: ungültiger Monat %q (Format JJJJ-MM-01)", line, ef.Wert)
	case store.FeldLogik:
		return fmt.Errorf("Zeile %d: ungültige Berechnungslogik für %s: %q", line, label(ef.ID), ef.Wert)
	case store.FeldTyp:
		return fmt.Errorf("Zeile %d: ungültiger Typ für %s: %q", line, label(ef.ID), ef.Wert)
	case store.FeldPersonen:
		// The message quotes the cell as written, not the parsed number.
		raw := ef.Wert
		if i, ok := colIdx[fmt.Sprintf("personen_%d", ef.ID)]; ok {
			raw = record[i]
		}
		return fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q (%w)", line, ef.ID, raw, ef.Err)
	}
	return err
}
