package web

import (
	"database/sql"
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboardData is every piece of already-computed data the Dashboard builds
// its views from: every period's and fixed-costs entry's costs, walked once.
type dashboardData struct {
	HasAnyData     bool
	Apartments     []store.Apartment
	PeriodenKosten []periodKosten
	FixkostenListe []fixkostenKosten
	Jahr           int
	// Monate are all Abrechnungsmonate with a reading or a Fixkosteneingabe,
	// newest first. NeuesJahr is the year of the newest one, Jahr the shown
	// year (NeuesJahr until waehleJahr picks another).
	Monate    []string
	NeuesJahr int
	// BisMonate are the Monate of Jahr: the months of the timeline. Bis is the
	// chosen one ("YYYY-MM-01"), "" = no limit (the newest month of Jahr).
	BisMonate []string
	Bis       string
	// WallboxWeiterberechnen is the house flag of the same name; with the
	// Wohnungsstatus it decides whether a zero Wallbox line is shown.
	WallboxWeiterberechnen bool
}

// verlaufOption is one entry of the dropdown that picks the Monatsverlauf
// shown: an apartment or one of the whole-house series the viewer may see.
type verlaufOption struct {
	ID   string
	Name string
}

// verlaufAuswahl lists the dropdown entries of the visible Verläufe in the
// order of the cards: the apartments, then the whole-house entries in the order
// of v.Haus.
func verlaufAuswahl(v ansicht) []verlaufOption {
	var out []verlaufOption
	for _, c := range v.Cards {
		out = append(out, verlaufOption{ID: strconv.FormatInt(c.ApartmentID, 10), Name: c.ApartmentName})
	}
	for _, s := range v.Haus {
		out = append(out, verlaufOption{ID: s.Verlauf.ID, Name: s.Verlauf.Name})
	}
	return out
}

// mieteMonat is the Miete projection of one Fixkosteneingabe: its
// Abrechnungsmonat and the monthly Miete per apartment. It is returned next to
// dashboardData, never inside it, so the Miete cannot reach a view by accident (#278).
type mieteMonat struct {
	Monat string
	Miete map[int64]float64
}

// loadDashboardData runs the DB-heavy first half of handleDashboard:
// apartments, every period's
// costs (stopping at the oldest period without a previous period, same as
// before), every fixed-costs entry's result, and the auto-following
// display year. HasAnyData=false (zero-value everything else) when
// neither readings nor fixed-costs entries exist yet. The second return value
// is the Miete projection taken from the same Datenstand (no second query),
// the third the months whose Ablesungen wait for their Fixkosten (#364).
func loadDashboardData(db *sql.DB) (dashboardData, []mieteMonat, []calc.OffenerMonat, error) {
	ds, err := store.LadeDatenstand(db)
	if err != nil {
		return dashboardData{}, nil, nil, err
	}
	miete := make([]mieteMonat, len(ds.Eingaben))
	for i, e := range ds.Eingaben {
		miete[i] = mieteMonat{Monat: e.Monat, Miete: e.Miete}
	}
	apartments := ds.Apartments

	// Newest first, like store.AllPeriods and store.AllFixkostenEingaben.
	allPeriods := make([]store.PeriodSummary, len(ds.Periods))
	for i, p := range ds.Periods {
		allPeriods[len(ds.Periods)-1-i] = store.PeriodSummary{ID: p.ID, ReadingDate: p.ReadingDate, Monat: p.Monat}
	}
	// Teilstand/partial reading (Ticket #129): only the newest period can
	// be incomplete (enforced on creation, see store.CreatePeriod) -
	// stays completely excluded here (no yearly-totals/monthly-history
	// entry, no effect on HasAnyData or the display year) until it's
	// completed.
	if len(ds.Periods) > 0 && ds.Periods[len(ds.Periods)-1].Teilstand().IstTeilstand {
		allPeriods = allPeriods[1:]
	}
	fixkostenEingaben := make([]store.FixkostenEingabeSummary, len(ds.Eingaben))
	for i, e := range ds.Eingaben {
		fixkostenEingaben[len(ds.Eingaben)-1-i] = store.FixkostenEingabeSummary{ID: e.ID, Monat: e.Monat}
	}

	if len(allPeriods) == 0 && len(fixkostenEingaben) == 0 {
		return dashboardData{Apartments: apartments}, miete, nil, nil
	}

	// Verbrauchskosten newest -> oldest. The very first period ever
	// recorded has no Vorperiode and so no Verbrauchskosten, which is the
	// natural end of the available history.
	verbrauchskosten := calc.KostenAus(ds)
	var periodenKosten []periodKosten
	berechenbar := verbrauchskosten.Berechenbare()
	for i := len(berechenbar) - 1; i >= 0; i-- {
		a := berechenbar[i]
		periodenKosten = append(periodenKosten, periodKosten{ReadingDate: a.Period.ReadingDate, Monat: a.Period.Monat, K: a.Kosten})
	}

	fixkostenListe := alleFixkostenKosten(calc.FixkostenreiheAus(ds))

	jahr := anzeigeJahr(allPeriods, fixkostenEingaben)
	var monate []string
	for _, p := range allPeriods {
		monate = append(monate, p.Monat)
	}
	for _, e := range fixkostenEingaben {
		monate = append(monate, e.Monat)
	}
	alle := monateImJahr(monate, 0)

	return dashboardData{
		HasAnyData:             true,
		Apartments:             apartments,
		PeriodenKosten:         periodenKosten,
		FixkostenListe:         fixkostenListe,
		Jahr:                   jahr,
		Monate:                 alle,
		NeuesJahr:              jahr,
		BisMonate:              monateImJahr(alle, jahr),
		WallboxWeiterberechnen: ds.Haus.WallboxWeiterberechnen,
	}, miete, verbrauchskosten.OffeneMonate(), nil
}

// monateImJahr returns the distinct months of jahr (0 = every year), newest
// first.
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

// waehleJahr shows another year. A wish without any month of data (or 0) keeps
// the newest year.
func (dd *dashboardData) waehleJahr(wunsch int) {
	if monate := monateImJahr(dd.Monate, wunsch); wunsch != 0 && len(monate) > 0 {
		dd.Jahr, dd.BisMonate = wunsch, monate
	}
}

// jahrLinks are the links to the year before and after Jahr that hold data, ""
// = none. The newest year links to the page without a parameter.
func jahrLinks(dd dashboardData) (zurueck, vor string) {
	var jahre []int
	for _, m := range dd.Monate {
		if y, ok := store.Abrechnungsmonat(m).Jahr(); ok && !slices.Contains(jahre, y) {
			jahre = append(jahre, y)
		}
	}
	slices.Sort(jahre)
	i := slices.Index(jahre, dd.Jahr)
	if i > 0 {
		zurueck = jahrAbfrage(jahre[i-1], dd.NeuesJahr)
	}
	if i >= 0 && i < len(jahre)-1 {
		vor = jahrAbfrage(jahre[i+1], dd.NeuesJahr)
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

// waehleBis sets the month the yearly totals run up to. A wish that is not one
// of BisMonate (bad format, other year, month without data) or the newest month
// means no limit.
func (dd *dashboardData) waehleBis(wunsch string) {
	if len(dd.BisMonate) > 0 && wunsch != dd.BisMonate[0] && slices.Contains(dd.BisMonate, wunsch) {
		dd.Bis = wunsch
	}
}

// bisGrenze is the last month that counts into the yearly totals and the
// Saldo: the Bis-Monat, for an older year its last month, "" = no limit.
func (dd dashboardData) bisGrenze() string {
	if dd.Bis == "" && dd.Jahr != dd.NeuesJahr && len(dd.BisMonate) > 0 {
		return dd.BisMonate[0]
	}
	return dd.Bis
}

func pkMonat(pk periodKosten) string        { return pk.Monat }
func fkMonat(fk fixkostenKosten) string     { return fk.Monat }
func mieteMonatKey(m mieteMonat) string     { return m.Monat }
func offenMonat(o calc.OffenerMonat) string { return o.Monat }

// bisMonat keeps the entries up to and including bis ("" = all). Monat values
// are "YYYY-MM-01", so they compare as strings.
func bisMonat[T any](xs []T, bis string, monat func(T) string) []T {
	if bis == "" {
		return xs
	}
	var out []T
	for _, x := range xs {
		if monat(x) <= bis {
			out = append(out, x)
		}
	}
	return out
}

// handleDashboard serves the redesigned Dashboard (Issue #60): yearly-
// totals cards per apartment for the auto-following display year, then an
// apartment switcher with a combined consumption+fixed-costs monthly
// history (4 modes).
func handleDashboard(version, buildDate string, z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		dd, miete, offen, err := loadDashboardData(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		nav := z.NavData(r)
		latestVersion, updateAvailable := checkForUpdate(version)

		if !dd.HasAnyData {
			data := struct {
				navData
				Aktuell         string
				HasAnyData      bool
				Version         string
				BuildDate       string
				UpdateAvailable bool
				LatestVersion   string
			}{
				navData:         nav,
				Aktuell:         "dashboard",
				Version:         version,
				BuildDate:       buildDate,
				UpdateAvailable: updateAvailable,
				LatestVersion:   latestVersion,
			}
			if err := dashboardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}
		if y, err := strconv.Atoi(r.URL.Query().Get("jahr")); err == nil {
			dd.waehleJahr(y)
		}
		dd.waehleBis(r.URL.Query().Get("bis"))
		jahr := dd.Jahr

		if err := nav.Fehler(); err != nil {
			http.Error(w, "berechtigungen: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// What the viewer may see is decided in one place (#293, #311).
		view := sichtbareAnsicht(nav.Zugriff, dd, miete, offen)
		// The rows of open months link to the form, the template needs the
		// base path and the right for that.
		for i := range view.VerlaufSpalten {
			view.VerlaufSpalten[i].Base, view.VerlaufSpalten[i].MitLink = nav.Base, nav.Vermieter
		}
		bisPunkte, bisFuell := bisZeitleiste(dd)
		jahrZurueck, jahrVor := jahrLinks(dd)
		data := struct {
			navData
			Aktuell          string
			FixkostenHinweis *fixkostenHinweis
			HasAnyData       bool
			AnzeigeJahr      int
			BisZeitleiste    []bisPunkt
			BisFuell         int
			JahrZurueck      string
			JahrVor          string
			BisLabel         string
			Cards            []dashboardJahresCard
			VerlaufSpalten   []dashboardVerlaufSpalte
			Haus             []sichtbarerSimple
			VerlaufAuswahl   []verlaufOption
			Version          string
			BuildDate        string
			UpdateAvailable  bool
			LatestVersion    string
		}{
			navData:          nav,
			Aktuell:          "dashboard",
			FixkostenHinweis: fixkostenFehlenHinweis(kontextDashboard, offen, nil),
			HasAnyData:       true,
			AnzeigeJahr:      jahr,
			BisLabel:         bisLabel(dd.Bis),
			BisZeitleiste:    bisPunkte,
			BisFuell:         bisFuell,
			JahrZurueck:      jahrZurueck,
			JahrVor:          jahrVor,
			Cards:            view.Cards,
			VerlaufSpalten:   view.VerlaufSpalten,
			Haus:             view.Haus,
			VerlaufAuswahl:   verlaufAuswahl(view),
			Version:          version,
			BuildDate:        buildDate,
			UpdateAvailable:  updateAvailable,
			LatestVersion:    latestVersion,
		}

		if err := dashboardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// bisPunkt is one month of the timeline above the yearly totals: a link that
// sets the Bis-Monat. Drin = the month counts into the totals, Gewaehlt = it is
// the Bis-Monat.
type bisPunkt struct {
	Link, Kurz, Name string
	Drin, Gewaehlt   bool
}

// bisZeitleiste lists the months oldest first with the share (0-100) of the
// line that is filled up to the chosen month. The newest month links to the
// page without a limit.
func bisZeitleiste(dd dashboardData) (punkte []bisPunkt, fuellProzent int) {
	n := len(dd.BisMonate)
	gewaehlt := n - 1 // index in oldest-first order
	if dd.Bis != "" {
		gewaehlt = n - 1 - slices.Index(dd.BisMonate, dd.Bis)
	}
	for i := n - 1; i >= 0; i-- {
		m := dd.BisMonate[i]
		pos := n - 1 - i
		link := jahrAbfrage(dd.Jahr, dd.NeuesJahr)
		if i != 0 && link == "?" {
			link = "?bis=" + m
		} else if i != 0 {
			link += "&bis=" + m
		}
		kurz := m
		if t, err := time.Parse("2006-01-02", m); err == nil {
			kurz = store.Monatsnamen[t.Month()-1][:3]
		}
		punkte = append(punkte, bisPunkt{Link: link, Kurz: kurz, Name: "bis " + store.MonatLabel(m), Drin: pos <= gewaehlt, Gewaehlt: pos == gewaehlt})
	}
	if n > 1 {
		fuellProzent = gewaehlt * 100 / (n - 1)
	}
	return punkte, fuellProzent
}

// bisLabel is the month name for the heading, "" without a limit.
func bisLabel(bis string) string {
	if bis == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", bis)
	if err != nil {
		return ""
	}
	return store.Monatsnamen[t.Month()-1]
}
