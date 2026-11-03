package web

import (
	"slices"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// wunsch is what the address asks for: the year (0 = none) and the Bis-Monat
// ("" = none).
type wunsch struct {
	jahr int
	bis  string
}

// monatsauswahl is the Dashboard's choice of display year, month set and
// Bis-Monat. Pure: built once by neueMonatsauswahl, never changed afterwards.
type monatsauswahl struct {
	// Jahr is the shown year.
	Jahr int
	// Monate are all Abrechnungsmonate, newest first. NeuesJahr is the year of
	// the newest one.
	Monate    []string
	NeuesJahr int
	// BisMonate are the Monate of Jahr: the months of the timeline. Bis is the
	// chosen one ("YYYY-MM-01"), "" = no limit (the newest month of Jahr).
	BisMonate []string
	Bis       string
}

// neueMonatsauswahl builds the choice from the Abrechnungsmonate of the
// counting Ablesungen, Fixkosteneingaben and Sonderkosten (any order, with
// duplicates and bad formats), the wishes of the address and today's year.
//   - No valid month: heute, empty lists.
//   - A jahr without a month of data (or 0) shows the newest year.
//   - A bis that is not one of BisMonate (bad format, other year, month
//     without data) or is the newest month means no limit.
func neueMonatsauswahl(monate []string, w wunsch, heute int) monatsauswahl {
	alle := monateImJahr(monate, 0)
	m := monatsauswahl{Jahr: heute, NeuesJahr: heute}
	if len(alle) == 0 {
		return m
	}
	m.Monate = alle
	m.NeuesJahr, _ = store.Abrechnungsmonat(alle[0]).Jahr()
	m.Jahr = m.NeuesJahr
	if w.jahr != 0 && len(monateImJahr(alle, w.jahr)) > 0 {
		m.Jahr = w.jahr
	}
	m.BisMonate = monateImJahr(alle, m.Jahr)
	if w.bis != m.BisMonate[0] && slices.Contains(m.BisMonate, w.bis) {
		m.Bis = w.bis
	}
	return m
}

// monateImJahr returns the distinct valid months of jahr (0 = every year),
// newest first.
func monateImJahr(monate []string, jahr int) []string {
	var out []string
	for _, m := range monate {
		if y, ok := store.Abrechnungsmonat(m).Jahr(); ok && (jahr == 0 || y == jahr) && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	slices.Sort(out)
	slices.Reverse(out)
	return out
}

// Grenze is the last month that counts into the yearly totals and the Saldo:
// the Bis-Monat, for an older year its last month, "" = no limit.
func (m monatsauswahl) Grenze() string {
	if m.Bis == "" && m.Jahr != m.NeuesJahr && len(m.BisMonate) > 0 {
		return m.BisMonate[0]
	}
	return m.Bis
}

// JahrLinks are the links to the year before and after Jahr that hold data, ""
// = none. The newest year links to the page without a parameter.
func (m monatsauswahl) JahrLinks() (zurueck, vor string) {
	var jahre []int
	for _, mo := range m.Monate {
		if y, ok := store.Abrechnungsmonat(mo).Jahr(); ok && !slices.Contains(jahre, y) {
			jahre = append(jahre, y)
		}
	}
	slices.Sort(jahre)
	i := slices.Index(jahre, m.Jahr)
	if i > 0 {
		zurueck = jahrAbfrage(jahre[i-1], m.NeuesJahr)
	}
	if i >= 0 && i < len(jahre)-1 {
		vor = jahrAbfrage(jahre[i+1], m.NeuesJahr)
	}
	return zurueck, vor
}

// jahrAbfrage is the query for showing jahr: empty for the newest year.
func jahrAbfrage(jahr, neuesJahr int) string {
	if jahr == neuesJahr {
		return "?"
	}
	return "?jahr=" + strconv.Itoa(jahr)
}

// bisPunkt is one month of the timeline above the yearly totals: a link that
// sets the Bis-Monat. Drin = the month counts into the totals, Gewaehlt = it is
// the Bis-Monat.
type bisPunkt struct {
	Link, Kurz, Name string
	Drin, Gewaehlt   bool
}

// Zeitleiste lists the months oldest first with the share (0-100) of the
// line that is filled up to the chosen month. The newest month links to the
// page without a limit.
func (m monatsauswahl) Zeitleiste() (punkte []bisPunkt, fuellProzent int) {
	n := len(m.BisMonate)
	gewaehlt := n - 1 // index in oldest-first order
	if m.Bis != "" {
		gewaehlt = n - 1 - slices.Index(m.BisMonate, m.Bis)
	}
	for i := n - 1; i >= 0; i-- {
		mo := m.BisMonate[i]
		pos := n - 1 - i
		link := jahrAbfrage(m.Jahr, m.NeuesJahr)
		if i != 0 && link == "?" {
			link = "?bis=" + mo
		} else if i != 0 {
			link += "&bis=" + mo
		}
		kurz := mo
		if t, err := time.Parse("2006-01-02", mo); err == nil {
			kurz = store.Monatsnamen[t.Month()-1][:3]
		}
		punkte = append(punkte, bisPunkt{Link: link, Kurz: kurz, Name: "bis " + store.MonatLabel(mo), Drin: pos <= gewaehlt, Gewaehlt: pos == gewaehlt})
	}
	if n > 1 {
		fuellProzent = gewaehlt * 100 / (n - 1)
	}
	return punkte, fuellProzent
}
