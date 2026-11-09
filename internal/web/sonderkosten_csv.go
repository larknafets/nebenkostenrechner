package web

import (
	"net/http"
	"slices"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenCsvHeader is the column order of the Sonderkosten CSV, shared by
// the export (#408) and the import (#409). art and objekt carry the German
// label, logik the technical key like the Fixkosten CSV.
// rechnungsdatum comes last and is optional on import (#454), so the import
// requires only sonderkostenCsvImportHeader.
var sonderkostenCsvImportHeader = []string{
	"monat", "betreff", "beschreibung", "art", "firma", "rechnungsnummer",
	"betrag", "objekt", "umlegbar", "logik",
}

var sonderkostenCsvHeader = append(slices.Clone(sonderkostenCsvImportHeader), "rechnungsdatum")

// auswahlLabel returns the German label of key, the key itself when unknown.
func auswahlLabel(liste []store.Auswahl, key string) string {
	if i := slices.IndexFunc(liste, func(a store.Auswahl) bool { return a.Wert == key }); i >= 0 {
		return liste[i].Label
	}
	return key
}

// auswahlSchluessel is the reverse of auswahlLabel: ok is false for a label
// the list does not know.
func auswahlSchluessel(liste []store.Auswahl, label string) (string, bool) {
	if i := slices.IndexFunc(liste, func(a store.Auswahl) bool { return a.Label == label }); i >= 0 {
		return liste[i].Wert, true
	}
	return "", false
}

func sonderkostenArtLabel(key string) string { return auswahlLabel(store.SonderkostenArten, key) }

func sonderkostenArtSchluessel(label string) (string, bool) {
	return auswahlSchluessel(store.SonderkostenArten, label)
}

func sonderkostenObjektLabel(key string) string {
	return auswahlLabel(store.SonderkostenObjekte, key)
}

func sonderkostenObjektSchluessel(label string) (string, bool) {
	return auswahlSchluessel(store.SonderkostenObjekte, label)
}

// csvText defuses a free text for spreadsheets: a text that starts with = + - @
// or a control character would run as a formula, so it gets a leading '.
// csvTextLesen undoes it on import.
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// csvTextLesen is the reverse of csvText: it strips the ' that csvText put in
// front of a text starting with = + - @ or a control character.
func csvTextLesen(s string) string {
	if len(s) > 1 && s[0] == '\'' && strings.ContainsRune("=+-@\t\r", rune(s[1])) {
		return s[1:]
	}
	return s
}

// formatSonderkostenCSVRow renders one entry in sonderkostenCsvHeader order.
func formatSonderkostenCSVRow(s store.Sonderkosten) []string {
	umlegbar := "0"
	if s.Umlegbar {
		umlegbar = "1"
	}
	return []string{
		s.Monat, csvText(s.Betreff), csvText(s.Beschreibung), sonderkostenArtLabel(s.Art), csvText(s.Firma), csvText(s.Rechnungsnummer),
		formatDecimalDE(s.Betrag), sonderkostenObjektLabel(s.Objekt), umlegbar, s.Logik, s.Rechnungsdatum,
	}
}

// handleSonderkostenExport streams every Sonderkosten entry of all years as
// CSV, oldest month first (#408).
func handleSonderkostenExport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alle, err := store.AllSonderkosten(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "sonderkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		RunCSVExport(w, "sonderkosten.csv", sonderkostenCsvHeader, alle, formatSonderkostenCSVRow)
	}
}
