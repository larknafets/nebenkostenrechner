package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// logikLabels renders a cost position's (Kostenposition) allocation logic
// (Logik) as the German label shown throughout the Fixkosten/Stammdaten
// UI - shared by the form, detail, and Stammdaten Kostenpositionen-Jahre
// templates.
var logikLabels = map[string]string{
	store.LogikWohneinheit: "Je Wohneinheit",
	store.LogikFlurstueck:  "Je anteiliges Flurstück",
	store.LogikQM:          "Je anteilige Wohnungsgröße",
	store.LogikPersonen:    "Je Anzahl Personen",
}

// logikOption is one <select> choice for a cost position's allocation logic.
type logikOption struct{ Value, Label string }

// logikOptions is logikLabels in a stable, display order - the Stammdaten
// Kostenpositionen-Jahre logic dropdown's option list.
var logikOptions = []logikOption{
	{store.LogikWohneinheit, logikLabels[store.LogikWohneinheit]},
	{store.LogikFlurstueck, logikLabels[store.LogikFlurstueck]},
	{store.LogikQM, logikLabels[store.LogikQM]},
	{store.LogikPersonen, logikLabels[store.LogikPersonen]},
}

// parseFixkostenMonat turns the form's <input type="month"> value ("YYYY-MM")
// into the stored "YYYY-MM-01" convention (same day-1 convention as
// PeriodInput.ReadingDate, minus the day-of-month the user never picks).
func parseFixkostenMonat(raw string) (string, error) {
	t, err := time.Parse("2006-01", raw)
	if err != nil {
		return "", fmt.Errorf("ungültiger Monat %q", raw)
	}
	return t.Format("2006-01") + "-01", nil
}

// monatForInput is parseFixkostenMonat's inverse, for prefilling the
// <input type="month"> value from a stored "YYYY-MM-01" Monat.
func monatForInput(monat string) string {
	t, err := time.Parse("2006-01-02", monat)
	if err != nil {
		return ""
	}
	return t.Format("2006-01")
}

// fixkostenPositionRow is one cost position's row on the fixed-costs form
// - Logik/Typ/Wert are editable for every position (Issue #105/#108), no
// more year-based special handling.
type fixkostenPositionRow struct {
	ID    int64
	Label string
	Logik string
	Typ   string
	Wert  float64
}

// fixkostenFormData is the fixed-costs form's template data - shared by
// "neu" (new, prefilled from the latest entry) and "bearbeiten" (edit,
// prefilled with the entry's own current values), same split as
// wizardData for readings.
type fixkostenFormData struct {
	navData
	Aktuell          string
	FormAction       string
	IsEdit           bool
	NoEingaben       bool   // gates the CSV import button (Issue #133), only offered while the DB is empty
	Monat            string // "YYYY-MM", <input type="month"> value
	Apartments       []store.Apartment
	PreviousPersonen map[int64]int64
	PreviousAbschlag map[int64]float64
	Positionen       []fixkostenPositionRow
}

// buildFixkostenPositionRows assembles one row per cost position, using
// values (Logik/Typ/Wert per position, from the latest entry in "neu"
// mode, or the entry's own values in "bearbeiten" mode) for prefilling. A
// position with no entry in values (the very first fixed-costs entry ever
// created) falls back to KostenpositionDefaults - the same starting
// values a freshly created Kostenpositionen-Jahre row used to get.
func buildFixkostenPositionRows(kostenpositionen []store.Kostenposition, values map[int64]store.FixkostenPositionWert) []fixkostenPositionRow {
	defaultByID := map[int64]store.KostenpositionDefault{}
	for _, kd := range store.KostenpositionDefaults {
		defaultByID[kd.ID] = kd
	}

	rows := make([]fixkostenPositionRow, 0, len(kostenpositionen))
	for _, kp := range kostenpositionen {
		row := fixkostenPositionRow{ID: kp.ID, Label: kp.Label}
		if w, ok := values[kp.ID]; ok {
			row.Logik, row.Typ, row.Wert = w.Logik, w.Typ, w.Wert
		} else if kd, ok := defaultByID[kp.ID]; ok {
			row.Logik, row.Typ = kd.Logik, kd.Typ
		}
		rows = append(rows, row)
	}
	return rows
}

// handleFixkostenForm serves the "neu" (new) fixed-costs form, prefilled
// from the latest entry (Issue #60 Story 2/9) - today's month as the
// default Monat, same convention as the reading wizard's ReadingDate
// default.
func handleFixkostenForm(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		latest, err := store.GetLatestFixkostenEingabe(db)
		if err != nil {
			http.Error(w, "latest fixkosten eingabe: "+err.Error(), http.StatusInternalServerError)
			return
		}

		monat := time.Now().Format("2006-01")

		kostenpositionen, err := store.Kostenpositionen(db)
		if err != nil {
			http.Error(w, "kostenpositionen: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := fixkostenFormData{
			navData:    a.NavData(r),
			Aktuell:    "fixkosten",
			FormAction: requestBase(r) + "/fixkosten",
			NoEingaben: latest == nil,
			Monat:      monat,
			Apartments: apartments,
		}
		var werte map[int64]store.FixkostenPositionWert
		if latest != nil {
			data.PreviousPersonen = latest.Personen
			data.PreviousAbschlag = latest.Abschlag
			werte = latest.Werte
		}
		data.Positionen = buildFixkostenPositionRows(kostenpositionen, werte)

		if err := fixkostenFormTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleFixkostenEditForm serves the "bearbeiten" (edit) fixed-costs form
// for an existing entry, prefilled with its own current values.
func handleFixkostenEditForm(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingabeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid fixkosten eingabe id", http.StatusBadRequest)
			return
		}

		target, err := store.GetFixkostenEingabeDetails(db, eingabeID)
		if err != nil {
			http.Error(w, "fixkosten eingabe: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if target == nil {
			http.NotFound(w, r)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		kostenpositionen, err := store.Kostenpositionen(db)
		if err != nil {
			http.Error(w, "kostenpositionen: "+err.Error(), http.StatusInternalServerError)
			return
		}

		positionen := buildFixkostenPositionRows(kostenpositionen, target.Werte)

		data := fixkostenFormData{
			navData:          a.NavData(r),
			Aktuell:          "fixkosten",
			FormAction:       fmt.Sprintf("%s/fixkosten/%d", requestBase(r), target.ID),
			IsEdit:           true,
			Monat:            monatForInput(target.Monat),
			Apartments:       apartments,
			PreviousPersonen: target.Personen,
			PreviousAbschlag: target.Abschlag,
			Positionen:       positionen,
		}

		if err := fixkostenFormTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// parseFixkostenInput parses a fixed-costs form submission (shared by
// handleCreateFixkosten and handleUpdateFixkosten) - Logik/Typ/Wert are
// read for all 14 cost positions (Issue #105/#108), each entry carries
// its own independent state.
func parseFixkostenInput(r *http.Request, apartments []store.Apartment) (store.FixkostenInput, error) {
	db := dbFromContext(r.Context())
	monat, err := parseFixkostenMonat(r.FormValue("monat"))
	if err != nil {
		return store.FixkostenInput{}, err
	}

	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return store.FixkostenInput{}, fmt.Errorf("kostenpositionen: %w", err)
	}

	werte := make(map[int64]store.FixkostenPositionWert, len(kostenpositionen))
	for _, kp := range kostenpositionen {
		idStr := strconv.FormatInt(kp.ID, 10)

		logik := r.FormValue("logik_" + idStr)
		if _, ok := logikLabels[logik]; !ok {
			return store.FixkostenInput{}, fmt.Errorf("ungültige Berechnungslogik für %s", kp.Label)
		}

		typ := r.FormValue("typ_" + idStr)
		if typ != store.TypJaehrlich && typ != store.TypMonatlich {
			return store.FixkostenInput{}, fmt.Errorf("ungültiger Typ für %s", kp.Label)
		}

		v, err := strconv.ParseFloat(r.FormValue("wert_"+idStr), 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("ungültiger Wert für %s", kp.Label)
		}
		werte[kp.ID] = store.FixkostenPositionWert{Logik: logik, Typ: typ, Wert: v}
	}

	personen := make(map[int64]int64, len(apartments))
	abschlag := make(map[int64]float64, len(apartments))
	for _, a := range apartments {
		p, err := strconv.ParseInt(r.FormValue("personen_"+strconv.FormatInt(a.ID, 10)), 10, 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("invalid Personenzahl for apartment %d", a.ID)
		}
		personen[a.ID] = p

		v, err := strconv.ParseFloat(r.FormValue("abschlag_"+strconv.FormatInt(a.ID, 10)), 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("ungültiger Abschlag für %s", a.Name)
		}
		abschlag[a.ID] = v
	}

	return store.FixkostenInput{Monat: monat, Personen: personen, Werte: werte, Abschlag: abschlag}, nil
}

func handleCreateFixkosten() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		in, err := parseFixkostenInput(r, apartments)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		eingabeID, err := store.CreateFixkostenEingabe(db, in)
		if err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("%s/fixkosten/%d", requestBase(r), eingabeID), http.StatusFound)
	}
}

func handleUpdateFixkosten() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingabeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid fixkosten eingabe id", http.StatusBadRequest)
			return
		}

		existing, err := store.GetFixkostenEingabeDetails(db, eingabeID)
		if err != nil {
			http.Error(w, "fixkosten eingabe: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if existing == nil {
			http.NotFound(w, r)
			return
		}

		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		in, err := parseFixkostenInput(r, apartments)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := store.UpdateFixkostenEingabe(db, eingabeID, in); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("%s/fixkosten/%d", requestBase(r), eingabeID), http.StatusFound)
	}
}

func handleDeleteFixkosten() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingabeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid fixkosten eingabe id", http.StatusBadRequest)
			return
		}

		if err := store.DeleteFixkostenEingabe(db, eingabeID); err != nil {
			if errors.Is(err, store.ErrFixkostenEingabeNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, requestBase(r)+"/fixkosten", http.StatusFound)
	}
}

// fixkostenListItem is one fixed-costs entry row in the /fixkosten
// overview and the detail view's "show other entry" dropdown.
type fixkostenListItem struct {
	ID      int64
	Label   string
	SummeW1 float64
	SummeW2 float64
}

// handleFixkostenListe lists every recorded Fixkosten-Eingabe. ImportedCount
// surfaces the CSV import's result (Issue #133) - passed via query param
// since the app has no session/flash mechanism, same convention as
// handleAblesungenListe.
func handleFixkostenListe(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingaben, err := store.AllFixkostenEingaben(db)
		if err != nil {
			http.Error(w, "fixkosten eingaben: "+err.Error(), http.StatusInternalServerError)
			return
		}

		items := make([]fixkostenListItem, 0, len(eingaben))
		for _, e := range eingaben {
			erg, err := calc.Fixkosten(db, e.ID)
			if err != nil {
				http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
				return
			}
			items = append(items, fixkostenListItem{
				ID: e.ID, Label: germanPeriodLabel(e.Monat),
				SummeW1: erg.KostenW1, SummeW2: erg.KostenW2,
			})
		}

		importedCount, _ := strconv.Atoi(r.URL.Query().Get("imported"))

		data := struct {
			navData
			Aktuell       string
			Eingaben      []fixkostenListItem
			ImportedCount int
		}{
			navData:       a.NavData(r),
			Aktuell:       "fixkosten",
			Eingaben:      items,
			ImportedCount: importedCount,
		}

		if err := fixkostenListeTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// fixkostenDetailPosition is one cost position's row on the detail view.
type fixkostenDetailPosition struct {
	Label      string
	LogikLabel string
	KostenW1   float64
	KostenW2   float64
}

func handleFixkostenDetail(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingabeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid fixkosten eingabe id", http.StatusBadRequest)
			return
		}

		eingabe, err := store.GetFixkostenEingabeDetails(db, eingabeID)
		if err != nil {
			http.Error(w, "fixkosten eingabe: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if eingabe == nil {
			http.NotFound(w, r)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		allEingaben, err := store.AllFixkostenEingaben(db)
		if err != nil {
			http.Error(w, "fixkosten eingaben: "+err.Error(), http.StatusInternalServerError)
			return
		}
		allItems := make([]fixkostenListItem, len(allEingaben))
		for i, e := range allEingaben {
			allItems[i] = fixkostenListItem{ID: e.ID, Label: germanPeriodLabel(e.Monat)}
		}

		erg, err := calc.Fixkosten(db, eingabeID)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		positionen := make([]fixkostenDetailPosition, 0, len(erg.Positionen))
		for _, p := range erg.Positionen {
			positionen = append(positionen, fixkostenDetailPosition{
				Label:      p.Label,
				LogikLabel: logikLabels[p.Logik],
				KostenW1:   p.KostenW1,
				KostenW2:   p.KostenW2,
			})
		}

		data := struct {
			navData
			Aktuell     string
			Eingabe     *store.FixkostenEingabeDetails
			MonatLabel  string
			AllEingaben []fixkostenListItem
			Apartments  []store.Apartment
			Positionen  []fixkostenDetailPosition
			KostenW1    float64
			KostenW2    float64
		}{
			navData:     a.NavData(r),
			Aktuell:     "fixkosten-detail",
			Eingabe:     eingabe,
			MonatLabel:  germanPeriodLabel(eingabe.Monat),
			AllEingaben: allItems,
			Apartments:  apartments,
			Positionen:  positionen,
			KostenW1:    erg.KostenW1,
			KostenW2:    erg.KostenW2,
		}

		if err := fixkostenDetailTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
