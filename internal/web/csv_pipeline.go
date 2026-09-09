package web

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
)

// csvImportMaxBytes caps every CSV upload (Ticket #54, Issue #133) - single-user
// app, no real threat model, just a guard against an accidental huge file.
const csvImportMaxBytes = 2 << 20 // 2 MiB

// ImportConfig drives RunCSVImport for one entity type T (store.PeriodInput,
// store.FixkostenInput, ...). Every field is mechanics or a per-entity callback;
// domain logic that isn't CSV-shaped (e.g. periods' post-import plausibility
// warnings) stays out of this struct and runs on the caller's side of
// RunCSVImport's return.
type ImportConfig[T any] struct {
	Header []string

	// CheckExisting reports whether an import must be rejected because data
	// already exists, plus the message to show when it does.
	CheckExisting  func(db *sql.DB) (bool, error)
	ExistingErrMsg string

	// ParseRow validates one CSV record into T. line is 1-based, counting the
	// header as line 1, matching csv.Reader's own row numbering.
	ParseRow func(record []string, colIdx map[string]int, line int) (T, error)
	// EmptyErrMsg is returned when the CSV has a valid header but no rows.
	EmptyErrMsg string

	// SortKey orders parsed rows chronologically before insert.
	SortKey func(T) string

	Insert func(db *sql.DB, items []T) ([]int64, error)
}

// RunCSVImport handles the mechanics shared by every CSV import (Ablesungen
// #54, Fixkosten #133): multipart size guard, BOM stripping, semicolon
// reader, header validation, per-row parsing, chronological sort, and
// insert. It writes its own error response and returns ok=false on any
// failure. On success it returns the inserted ids and the sorted rows that
// produced them, so the caller can still build its own success redirect
// (e.g. appending warnings) without RunCSVImport knowing about them.
func RunCSVImport[T any](w http.ResponseWriter, r *http.Request, db *sql.DB, cfg ImportConfig[T]) (ids []int64, sortedRows []T, ok bool) {
	existing, err := cfg.CheckExisting(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return nil, nil, false
	}
	if existing {
		http.Error(w, cfg.ExistingErrMsg, http.StatusBadRequest)
		return nil, nil, false
	}

	r.Body = http.MaxBytesReader(w, r.Body, csvImportMaxBytes)
	if err := r.ParseMultipartForm(csvImportMaxBytes); err != nil {
		http.Error(w, "Datei zu groß oder ungültig (Limit 2 MB): "+err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}
	file, _, err := r.FormFile("csv")
	if err != nil {
		http.Error(w, "keine CSV-Datei hochgeladen", http.StatusBadRequest)
		return nil, nil, false
	}
	defer file.Close()

	rows, err := parseImportCSVRows(file, cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, false
	}

	sort.Slice(rows, func(i, j int) bool { return cfg.SortKey(rows[i]) < cfg.SortKey(rows[j]) })

	ids, err = cfg.Insert(db, rows)
	if err != nil {
		http.Error(w, "import: "+err.Error(), http.StatusInternalServerError)
		return nil, nil, false
	}

	return ids, rows, true
}

func parseImportCSVRows[T any](file io.Reader, cfg ImportConfig[T]) ([]T, error) {
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
	for _, want := range cfg.Header {
		if _, ok := colIdx[want]; !ok {
			return nil, fmt.Errorf("CSV: Spalte %q fehlt", want)
		}
	}

	var rows []T
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

		row, err := cfg.ParseRow(record, colIdx, line)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("%s", cfg.EmptyErrMsg)
	}
	return rows, nil
}

// RunCSVExport writes header, then every item, in the Excel-DE dialect
// (semicolon-separated, comma-decimal, UTF-8 with BOM) shared by every CSV
// export (Ablesungen #53, Fixkosten #132).
func RunCSVExport[T any](w http.ResponseWriter, filename string, header []string, items []T, formatRow func(T) []string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	w.Write([]byte{0xEF, 0xBB, 0xBF})

	cw := csv.NewWriter(w)
	cw.Comma = ';'
	if err := cw.Write(header); err != nil {
		return
	}
	for _, item := range items {
		if err := cw.Write(formatRow(item)); err != nil {
			return
		}
	}
	cw.Flush()
}
