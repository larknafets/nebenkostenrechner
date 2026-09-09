package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// periodListItem is one period's reading date (Ablesedatum) together
// with its period (Zeitraum), combined into a single label (the gap to
// the chronologically previous period, "keine Vorperiode" for the
// oldest) - the reading detail's "show other reading" dropdown, where a
// single-line option leaves no room for 2 columns.
type periodListItem struct {
	ID    int64
	Label string
}

// periodListItems builds one periodListItem per period. periods must be in
// store.AllPeriods' own order (newest first) - the predecessor of
// periods[i] is then periods[i+1], the next-older one.
func periodListItems(periods []store.PeriodSummary) []periodListItem {
	out := make([]periodListItem, len(periods))
	for i, p := range periods {
		var label string
		if i+1 < len(periods) {
			label = fmt.Sprintf("%s (%s–%s)", formatDatumDE(p.ReadingDate), formatDatumDE(periods[i+1].ReadingDate), formatDatumDE(p.ReadingDate))
		} else {
			label = fmt.Sprintf("%s (keine Vorperiode)", formatDatumDE(p.ReadingDate))
		}
		out[i] = periodListItem{ID: p.ID, Label: label}
	}
	return out
}

// periodOverviewRow is one period's reading date and period (Ablesedatum/
// Zeitraum) as 2 separate values - the readings overview's table, which
// has room for its own column per field (unlike the dropdown's single-
// line option).
type periodOverviewRow struct {
	ID           int64
	ReadingDate  string
	Zeitraum     string
	IstTeilstand bool
}

// periodMonatGroup is every reading of one billing month (Monat, Issue
// #86), newest first - the readings overview's rowspan column (Ticket
// #83 variant B): a month with 1 reading renders a single row, a month
// with several (sub-monthly readings) spans the group under one month
// cell.
type periodMonatGroup struct {
	MonatLabel string
	Rows       []periodOverviewRow
}

// periodOverviewGroups builds one periodMonatGroup per distinct month,
// newest first, same order/predecessor rule as periodListItems for each
// row's Zeitraum. periods must already be newest-first (store.AllPeriods'
// own order). teilstandID marks that one row IstTeilstand (Ticket #131,
// pass 0 when there's no open Teilstand) - only the newest period can
// ever be one (enforced at creation, Ticket #129), so the caller only
// needs to know that single id rather than checking every row's
// completeness.
func periodOverviewGroups(periods []store.PeriodSummary, teilstandID int64) []periodMonatGroup {
	var out []periodMonatGroup
	var currentMonat string
	for i, p := range periods {
		var zeitraum string
		if i+1 < len(periods) {
			zeitraum = fmt.Sprintf("%s–%s", formatDatumDE(periods[i+1].ReadingDate), formatDatumDE(p.ReadingDate))
		} else {
			zeitraum = "keine Vorperiode"
		}
		row := periodOverviewRow{ID: p.ID, ReadingDate: formatDatumDE(p.ReadingDate), Zeitraum: zeitraum, IstTeilstand: teilstandID != 0 && p.ID == teilstandID}

		if len(out) > 0 && p.Monat == currentMonat {
			out[len(out)-1].Rows = append(out[len(out)-1].Rows, row)
			continue
		}
		currentMonat = p.Monat
		out = append(out, periodMonatGroup{MonatLabel: germanPeriodLabel(p.Monat), Rows: []periodOverviewRow{row}})
	}
	return out
}

// handleAblesungenListe lists every recorded period (Ticket #43), newest
// first, linking each to its detail view. ImportedCount/Warnings surface the
// CSV import's result (Ticket #54) - passed via query params since the app
// has no session/flash mechanism.
func handleAblesungenListe(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periods, err := store.AllPeriods(db)
		if err != nil {
			http.Error(w, "periods: "+err.Error(), http.StatusInternalServerError)
			return
		}
		teilstandID, hasOpenTeilstand, err := openTeilstandID(db)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !hasOpenTeilstand {
			teilstandID = 0
		}

		importedCount, _ := strconv.Atoi(r.URL.Query().Get("imported"))

		data := struct {
			navData
			Aktuell       string
			MonatGruppen  []periodMonatGroup
			ImportedCount int
			Warnings      []string
		}{
			navData:       a.NavData(r),
			Aktuell:       "ablesungen",
			MonatGruppen:  periodOverviewGroups(periods, teilstandID),
			ImportedCount: importedCount,
			Warnings:      r.URL.Query()["warning"],
		}

		if err := ablesungenTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
