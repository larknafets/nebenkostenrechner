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
	// Sonderkosten are the entries that count per apartment id (calc.SonderkostenMonate).
	Sonderkosten map[int64][]calc.MonatsSonderkosten
	// monatsauswahl is the chosen year, month set and Bis-Monat.
	monatsauswahl
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
// neither readings, fixed-costs entries nor umlegbare Sonderkosten exist yet. The second return value
// is the Miete projection taken from the same Datenstand (no second query),
// the third the months whose Ablesungen wait for their Fixkosten (#364).
func loadDashboardData(db *sql.DB, w wunsch, heute int) (dashboardData, []mieteMonat, []calc.OffenerMonat, error) {
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

	reihe := calc.FixkostenreiheAus(ds)
	sonderkosten := map[int64][]calc.MonatsSonderkosten{}
	var skMonate []string
	for _, a := range apartments {
		sonderkosten[a.ID] = calc.SonderkostenMonate(ds.Sonderkosten, apartments, reihe, a.ID)
		for _, s := range sonderkosten[a.ID] {
			skMonate = append(skMonate, s.Monat)
		}
	}

	if len(allPeriods) == 0 && len(fixkostenEingaben) == 0 && len(skMonate) == 0 {
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

	fixkostenListe := alleFixkostenKosten(reihe)

	// Months of the counting Ablesungen (the Teilstand is already out, see
	// above and the rule in calc), the Fixkosteneingaben and the Sonderkosten;
	// monatsauswahl decides year, month set and Bis-Monat.
	monate := slices.Clone(skMonate)
	for _, p := range allPeriods {
		monate = append(monate, p.Monat)
	}
	for _, e := range fixkostenEingaben {
		monate = append(monate, e.Monat)
	}

	return dashboardData{
		HasAnyData:             true,
		Apartments:             apartments,
		PeriodenKosten:         periodenKosten,
		FixkostenListe:         fixkostenListe,
		Sonderkosten:           sonderkosten,
		monatsauswahl:          neueMonatsauswahl(monate, w, heute),
		WallboxWeiterberechnen: ds.Haus.WallboxWeiterberechnen,
	}, miete, verbrauchskosten.OffeneMonate(), nil
}

func pkMonat(pk periodKosten) string            { return pk.Monat }
func fkMonat(fk fixkostenKosten) string         { return fk.Monat }
func skMonat(sk calc.MonatsSonderkosten) string { return sk.Monat }
func mieteMonatKey(m mieteMonat) string         { return m.Monat }
func offenMonat(o calc.OffenerMonat) string     { return o.Monat }

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
		jahrWunsch, _ := strconv.Atoi(r.URL.Query().Get("jahr"))
		dd, miete, offen, err := loadDashboardData(dbFromContext(r.Context()), wunsch{jahr: jahrWunsch, bis: r.URL.Query().Get("bis")}, time.Now().Year())
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
		bisPunkte, bisFuell := dd.Zeitleiste()
		jahrZurueck, jahrVor := dd.JahrLinks()
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
