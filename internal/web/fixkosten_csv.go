package web

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkostenCsvHeader is the canonical CSV column order for both export
// (Issue #132) and import (Issue #133) - monat, then Logik/Typ/Wert per
// Kostenposition (in store.KostenpositionDefaults' fixed id order, since
// Logik/Typ are editable per Eingabe and can't be omitted like a period's
// static meter keys), then Personen/Abschlag per apartment (fixed ids 1/2,
// see store's seed()).
var fixkostenCsvHeader = buildFixkostenCsvHeader()

func buildFixkostenCsvHeader() []string {
	header := []string{"monat"}
	for _, kd := range store.KostenpositionDefaults {
		header = append(header, kd.Key+"_logik", kd.Key+"_typ", kd.Key+"_wert")
	}
	return append(header, "personen_1", "personen_2", "abschlag_1", "abschlag_2")
}

// handleExportFixkostenCSV streams every Fixkosten-Eingabe as CSV (Issue
// #132) - same Excel-DE dialect (semicolon-separated, comma-decimal, UTF-8
// with BOM) as the Ablesungen export, same fixkostenCsvHeader the import
// (Issue #133) reads back.
func handleExportFixkostenCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		details, err := store.AllFixkostenEingabenDetails(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "fixkosten eingaben: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="fixkosten.csv"`)
		w.Write([]byte{0xEF, 0xBB, 0xBF})

		cw := csv.NewWriter(w)
		cw.Comma = ';'
		if err := cw.Write(fixkostenCsvHeader); err != nil {
			return
		}
		for _, f := range details {
			row := make([]string, 0, len(fixkostenCsvHeader))
			row = append(row, f.Monat)
			for _, kd := range store.KostenpositionDefaults {
				w := f.Werte[kd.ID]
				row = append(row, w.Logik, w.Typ, formatDecimalDE(w.Wert))
			}
			row = append(row,
				formatDecimalDE(float64(f.Personen[1])),
				formatDecimalDE(float64(f.Personen[2])),
				formatDecimalDE(f.Abschlag[1]),
				formatDecimalDE(f.Abschlag[2]),
			)
			if err := cw.Write(row); err != nil {
				return
			}
		}
		cw.Flush()
	}
}

// fixkostenImportMaxBytes caps the CSV upload (Issue #133), same limit as
// the Ablesungen import.
const fixkostenImportMaxBytes = 2 << 20 // 2 MiB

// handleImportFixkostenCSV bootstraps a completely empty Fixkosten history
// from a CSV in the fixkostenCsvHeader format (Issue #133) - rejected if
// any Fixkosten-Eingabe already exists, even though the form button is
// already hidden in that case (defense in depth). A hard error in any row
// aborts the whole import (all or nothing); unlike the Ablesungen import,
// there are no plausibility warnings to report back (Issue #133).
func handleImportFixkostenCSV() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		existing, err := store.AllFixkostenEingaben(db)
		if err != nil {
			http.Error(w, "fixkosten eingaben: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if len(existing) > 0 {
			http.Error(w, "Import nur möglich, solange noch keine Fixkosten-Eingabe existiert", http.StatusBadRequest)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, fixkostenImportMaxBytes)
		if err := r.ParseMultipartForm(fixkostenImportMaxBytes); err != nil {
			http.Error(w, "Datei zu groß oder ungültig (Limit 2 MB): "+err.Error(), http.StatusBadRequest)
			return
		}
		file, _, err := r.FormFile("csv")
		if err != nil {
			http.Error(w, "keine CSV-Datei hochgeladen", http.StatusBadRequest)
			return
		}
		defer file.Close()

		inputs, err := parseImportFixkostenCSV(file)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sort.Slice(inputs, func(i, j int) bool { return inputs[i].Monat < inputs[j].Monat })

		ids, err := store.ImportFixkostenEingaben(db, inputs)
		if err != nil {
			http.Error(w, "import: "+err.Error(), http.StatusInternalServerError)
			return
		}

		redirectURL := fmt.Sprintf("%s/fixkosten?imported=%d", requestBase(r), len(ids))
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// parseImportFixkostenCSV reads fixkostenCsvHeader-formatted CSV (semicolon-
// separated, comma-decimal, optional UTF-8 BOM) and validates every row
// into a FixkostenInput. Returns the first hard error encountered - the
// caller aborts the whole import on any error, so there's no point
// collecting more than one.
func parseImportFixkostenCSV(file io.Reader) ([]store.FixkostenInput, error) {
	reader := bufio.NewReader(file)
	if bom, err := reader.Peek(3); err == nil && bom[0] == 0xEF && bom[1] == 0xBB && bom[2] == 0xBF {
		reader.Discard(3)
	}

	cr := csv.NewReader(reader)
	cr.Comma = ';'

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("CSV: Kopfzeile konnte nicht gelesen werden: %w", err)
	}
	colIdx := make(map[string]int, len(header))
	for i, name := range header {
		colIdx[strings.TrimSpace(name)] = i
	}
	for _, want := range fixkostenCsvHeader {
		if _, ok := colIdx[want]; !ok {
			return nil, fmt.Errorf("CSV: Spalte %q fehlt", want)
		}
	}

	var inputs []store.FixkostenInput
	line := 1
	for {
		record, err := cr.Read()
		if err == io.EOF {
			break
		}
		line++
		if err != nil {
			return nil, fmt.Errorf("Zeile %d: %v", line, err)
		}

		in, err := parseImportFixkostenRow(record, colIdx, line)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, in)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("CSV enthält keine Fixkosten-Eingaben")
	}
	return inputs, nil
}

func parseImportFixkostenRow(record []string, colIdx map[string]int, line int) (store.FixkostenInput, error) {
	cell := func(col string) string { return record[colIdx[col]] }

	monat := strings.TrimSpace(cell("monat"))
	if _, err := time.Parse("2006-01-02", monat); err != nil {
		return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültiger Monat %q (Format JJJJ-MM-01)", line, monat)
	}

	werte := make(map[int64]store.FixkostenPositionWert, len(store.KostenpositionDefaults))
	for _, kd := range store.KostenpositionDefaults {
		logik := strings.TrimSpace(cell(kd.Key + "_logik"))
		if _, ok := logikLabels[logik]; !ok {
			return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültige Berechnungslogik für %s: %q", line, kd.Label, logik)
		}

		typ := strings.TrimSpace(cell(kd.Key + "_typ"))
		if typ != store.TypJaehrlich && typ != store.TypMonatlich {
			return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültiger Typ für %s: %q", line, kd.Label, typ)
		}

		wert, err := parseDecimalDE(cell(kd.Key + "_wert"))
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültiger Wert für %s: %q", line, kd.Label, cell(kd.Key+"_wert"))
		}

		werte[kd.ID] = store.FixkostenPositionWert{Logik: logik, Typ: typ, Wert: wert}
	}

	personen := make(map[int64]int64, 2)
	abschlag := make(map[int64]float64, 2)
	for _, id := range [2]int64{1, 2} {
		personenCol := fmt.Sprintf("personen_%d", id)
		p, err := parseDecimalDE(cell(personenCol))
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültige Personenzahl für Wohnung %d: %q", line, id, cell(personenCol))
		}
		personen[id] = int64(p)

		abschlagCol := fmt.Sprintf("abschlag_%d", id)
		a, err := parseDecimalDE(cell(abschlagCol))
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("Zeile %d: ungültiger Abschlag für Wohnung %d: %q", line, id, cell(abschlagCol))
		}
		abschlag[id] = a
	}

	return store.FixkostenInput{
		Monat:    monat,
		Personen: personen,
		Werte:    werte,
		Abschlag: abschlag,
	}, nil
}
