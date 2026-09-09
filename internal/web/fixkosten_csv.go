package web

import (
	"database/sql"
	"fmt"
	"net/http"
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
		formatDecimalDE(float64(f.Personen[1])),
		formatDecimalDE(float64(f.Personen[2])),
		formatDecimalDE(f.Abschlag[1]),
		formatDecimalDE(f.Abschlag[2]),
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

		cfg := ImportConfig[store.FixkostenInput]{
			Header: fixkostenCsvHeader,
			CheckExisting: func(db *sql.DB) (bool, error) {
				existing, err := store.AllFixkostenEingaben(db)
				return len(existing) > 0, err
			},
			ExistingErrMsg: "Import nur möglich, solange noch keine Fixkosten-Eingabe existiert",
			ParseRow:       parseImportFixkostenRow,
			EmptyErrMsg:    "CSV enthält keine Fixkosten-Eingaben",
			SortKey:        func(in store.FixkostenInput) string { return in.Monat },
			Insert:         store.ImportFixkostenEingaben,
		}

		ids, _, ok := RunCSVImport(w, r, db, cfg)
		if !ok {
			return
		}

		redirectURL := fmt.Sprintf("%s/fixkosten?imported=%d", requestBase(r), len(ids))
		http.Redirect(w, r, redirectURL, http.StatusFound)
	}
}

// parseImportFixkostenRow validates one CSV record (semicolon-separated,
// comma-decimal) into a FixkostenInput.
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
