package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// logikOption is one <select> choice for a cost position's allocation logic.
type logikOption struct{ Value, Label string }

// logikOptions is calc.LogikLabels in a stable, display order - the Stammdaten
// Kostenpositionen-Jahre logic dropdown's option list.
var logikOptions = []logikOption{
	{store.LogikWohneinheit, calc.LogikLabels[store.LogikWohneinheit]},
	{store.LogikFlurstueck, calc.LogikLabels[store.LogikFlurstueck]},
	{store.LogikQM, calc.LogikLabels[store.LogikQM]},
	{store.LogikPersonen, calc.LogikLabels[store.LogikPersonen]},
	{store.LogikWohnung1, calc.LogikLabels[store.LogikWohnung1]},
	{store.LogikWohnung2, calc.LogikLabels[store.LogikWohnung2]},
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
	PreviousPersonen map[int64]float64
	Vermietet        []fixkostenMieteRow // Abschlag and Miete fields, one per apartment with Status Vermietung only
	Positionen       []fixkostenPositionRow
	Preise           []fixkostenPreisFeld    // the four prices of the month
	Haekchen         []fixkostenHaekchenFeld // the two switches of the month
}

// fixkostenMieteRow is the Abschlag and Miete input pair of one rented
// apartment. The values are ready for value=, "" means nothing to prefill.
type fixkostenMieteRow struct {
	ID              int64
	Name            string
	Abschlag, Miete string
}

// buildFixkostenMieteRows makes one row per apartment with Status Vermietung
// (an owner-occupied apartment gets no field). abschlag and miete are the
// stored values of the Eingabe to prefill from. A Miete that was never
// recorded stays empty, so the required field asks for it. The Abschlag keeps
// its older prefill: 0 when the Eingabe has no value for the apartment.
func buildFixkostenMieteRows(apartments []store.Apartment, abschlag, miete map[int64]float64, hatVorlage bool) []fixkostenMieteRow {
	var rows []fixkostenMieteRow
	for _, a := range apartments {
		if !store.NewWohnungsregelnOhneHaus(a).FixkostenAbschlagMiete() {
			continue
		}
		row := fixkostenMieteRow{ID: a.ID, Name: a.Name}
		if hatVorlage {
			row.Abschlag = inputNumber(abschlag[a.ID])
		}
		if v, ok := miete[a.ID]; ok {
			row.Miete = inputNumber(v)
		}
		rows = append(rows, row)
	}
	return rows
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
func handleFixkostenForm(z *Zugang) http.HandlerFunc {
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
			navData:    z.NavData(r),
			Aktuell:    "fixkosten",
			FormAction: requestBase(r) + "/fixkosten",
			NoEingaben: latest == nil,
			Monat:      monat,
			Apartments: apartments,
		}
		var werte map[int64]store.FixkostenPositionWert
		if latest != nil {
			data.PreviousPersonen = latest.Personen
			data.Vermietet = buildFixkostenMieteRows(apartments, latest.Abschlag, latest.Miete, true)
			werte = latest.Werte
		} else {
			data.Vermietet = buildFixkostenMieteRows(apartments, nil, nil, false)
		}
		data.Positionen = buildFixkostenPositionRows(kostenpositionen, werte)
		data.Preise, data.Haekchen = buildFixkostenPreisFelder(latest)

		if err := fixkostenFormTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleFixkostenEditForm serves the "bearbeiten" (edit) fixed-costs form
// for an existing entry, prefilled with its own current values.
func handleFixkostenEditForm(z *Zugang) http.HandlerFunc {
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
			navData:          z.NavData(r),
			Aktuell:          "fixkosten",
			FormAction:       fmt.Sprintf("%s/fixkosten/%d", requestBase(r), target.ID),
			IsEdit:           true,
			Monat:            monatForInput(target.Monat),
			Apartments:       apartments,
			PreviousPersonen: target.Personen,
			Vermietet:        buildFixkostenMieteRows(apartments, target.Abschlag, target.Miete, true),
			Positionen:       positionen,
		}
		data.Preise, data.Haekchen = buildFixkostenPreisFelder(target)

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
	labels := make(map[int64]string, len(kostenpositionen))
	for _, kp := range kostenpositionen {
		idStr := strconv.FormatInt(kp.ID, 10)
		labels[kp.ID] = kp.Label

		v, err := strconv.ParseFloat(r.FormValue("wert_"+idStr), 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("ungültiger Wert für %s", kp.Label)
		}
		// Logik and Typ are checked by FixkostenInput.Validate below.
		werte[kp.ID] = store.FixkostenPositionWert{Logik: r.FormValue("logik_" + idStr), Typ: r.FormValue("typ_" + idStr), Wert: v}
	}

	personen := make(map[int64]float64, len(apartments))
	abschlag := make(map[int64]float64, len(apartments))
	miete := make(map[int64]float64, len(apartments))
	names := make(map[int64]string, len(apartments))
	for _, a := range apartments {
		idStr := strconv.FormatInt(a.ID, 10)
		names[a.ID] = a.Name
		p, err := strconv.ParseFloat(r.FormValue("personen_"+idStr), 64)
		if err != nil {
			return store.FixkostenInput{}, personenKeineZahl(a.Name)
		}
		personen[a.ID] = p

		// Only a rented apartment has the fields. For an owner-occupied one
		// nothing is read: an Abschlag stored earlier stays as it is (the
		// store leaves an apartment without an entry alone on update).
		if !store.NewWohnungsregelnOhneHaus(a).FixkostenAbschlagMiete() {
			continue
		}
		v, err := strconv.ParseFloat(r.FormValue("abschlag_"+idStr), 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("ungültiger Abschlag für %s", a.Name)
		}
		abschlag[a.ID] = v

		m, err := strconv.ParseFloat(r.FormValue("miete_"+idStr), 64)
		if err != nil {
			return store.FixkostenInput{}, fmt.Errorf("ungültige Miete für %s", a.Name)
		}
		miete[a.ID] = m
	}

	in := store.FixkostenInput{Monat: monat, Personen: personen, Werte: werte, Abschlag: abschlag, Miete: miete}
	if err := parseFixkostenPreise(r.FormValue, &in); err != nil {
		return store.FixkostenInput{}, err
	}
	if err := in.Validate(); err != nil {
		return store.FixkostenInput{}, fixkostenFormularFehler(err, labels, names)
	}
	return in, nil
}

// personenKeineZahl is the form error for a Personenzahl field that does not
// parse as a number (shared by the Ablesung and Fixkosten forms).
func personenKeineZahl(wohnung string) error {
	return fmt.Errorf("ungültige Personenzahl für %s: Personenzahl muss eine Zahl sein", wohnung)
}

// fixkostenFormularFehler words a store.EingabeFehler for the form. labels
// are the Kostenposition names by id, names the Wohnung names by id. Any
// other error passes through.
func fixkostenFormularFehler(err error, labels, names map[int64]string) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return err
	}
	switch ef.Feld {
	case store.FeldMonat:
		return fmt.Errorf("ungültiger Monat %q", ef.Wert)
	case store.FeldLogik:
		return fmt.Errorf("ungültige Berechnungslogik für %s", labels[ef.ID])
	case store.FeldTyp:
		return fmt.Errorf("ungültiger Typ für %s", labels[ef.ID])
	case store.FeldPersonen:
		return fmt.Errorf("ungültige Personenzahl für %s: %w", names[ef.ID], ef.Err)
	}
	if p, ok := fixkostenPreisFuerFeld(ef.Feld); ok {
		return preisFormularFehler(p, ef.Err)
	}
	return err
}

// fixkostenStoreFehler words a *store.EingabeFehler that CreateFixkostenEingabe
// or UpdateFixkostenEingabe returned (a caller that skipped parseFixkostenInput).
// It returns nil for every other error.
func fixkostenStoreFehler(db *sql.DB, err error, apartments []store.Apartment) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return nil
	}
	labels := map[int64]string{}
	if kostenpositionen, kerr := store.Kostenpositionen(db); kerr == nil {
		for _, kp := range kostenpositionen {
			labels[kp.ID] = kp.Label
		}
	}
	names := make(map[int64]string, len(apartments))
	for _, a := range apartments {
		names[a.ID] = a.Name
	}
	return fixkostenFormularFehler(ef, labels, names)
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
			if errors.Is(err, store.ErrFixkostenMonatBelegt) {
				badRequestFeld(w, monatBelegtMessage(in.Monat), "monat")
				return
			}
			if mapped := fixkostenStoreFehler(db, err, apartments); mapped != nil {
				http.Error(w, mapped.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("%s/fixkosten/%d", requestBase(r), eingabeID), http.StatusFound)
	}
}

// monatBelegtMessage is the user-facing text when a month already has its
// one Fixkosten-Eingabe (Issue #161).
func monatBelegtMessage(monat string) string {
	return "Für " + store.MonatLabel(monat) + " gibt es bereits eine Fixkosten-Eingabe. Je Monat ist nur eine erlaubt, bitte die bestehende Eingabe korrigieren."
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
			if errors.Is(err, store.ErrFixkostenMonatBelegt) {
				badRequestFeld(w, monatBelegtMessage(in.Monat), "monat")
				return
			}
			if mapped := fixkostenStoreFehler(db, err, apartments); mapped != nil {
				http.Error(w, mapped.Error(), http.StatusBadRequest)
				return
			}
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
func handleFixkostenListe(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		reihe, err := calc.LoadFixkostenreihe(db)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}

		neueste := reihe.Neueste()
		items := make([]fixkostenListItem, 0, len(neueste))
		for _, m := range neueste {
			items = append(items, fixkostenListItem{
				ID: m.Eingabe.ID, Label: store.MonatLabel(m.Eingabe.Monat),
				SummeW1: m.Ergebnis.KostenW1, SummeW2: m.Ergebnis.KostenW2,
			})
		}

		importedCount, _ := strconv.Atoi(r.URL.Query().Get("imported"))

		data := struct {
			navData
			Aktuell       string
			Eingaben      []fixkostenListItem
			ImportedCount int
		}{
			navData:       z.NavData(r),
			Aktuell:       "fixkosten",
			Eingaben:      items,
			ImportedCount: importedCount,
		}

		if err := fixkostenListeTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// fixkostenDetailApartment is one apartment row of the detail view's top
// table. Miete is shown only for Status Vermietung (an empty cell otherwise);
// MieteErfasst is false for entries from before the Miete existed.
type fixkostenDetailApartment struct {
	Name         string
	Personen     float64
	Abschlag     float64
	Vermietet    bool
	MieteErfasst bool
	Miete        float64
}

// fixkostenDetailPosition is one cost position's row on the detail view.
type fixkostenDetailPosition struct {
	Label      string
	LogikLabel string
	KostenW1   float64
	KostenW2   float64
}

func handleFixkostenDetail(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		eingabeID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid fixkosten eingabe id", http.StatusBadRequest)
			return
		}

		reihe, err := calc.LoadFixkostenreihe(db)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fixkosten, ok := reihe.Eingabe(eingabeID)
		if !ok {
			http.NotFound(w, r)
			return
		}
		eingabe, erg := fixkosten.Eingabe, fixkosten.Ergebnis

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		neueste := reihe.Neueste()
		allItems := make([]fixkostenListItem, len(neueste))
		for i, m := range neueste {
			allItems[i] = fixkostenListItem{ID: m.Eingabe.ID, Label: store.MonatLabel(m.Eingabe.Monat)}
		}

		positionen := make([]fixkostenDetailPosition, 0, len(erg.Positionen))
		for _, p := range erg.Positionen {
			positionen = append(positionen, fixkostenDetailPosition{
				Label:      p.Label,
				LogikLabel: calc.LogikLabels[p.Logik],
				KostenW1:   p.KostenW1,
				KostenW2:   p.KostenW2,
			})
		}

		wohnungen := make([]fixkostenDetailApartment, 0, len(apartments))
		for _, ap := range apartments {
			w := fixkostenDetailApartment{
				Name:      ap.Name,
				Personen:  eingabe.Personen[ap.ID],
				Abschlag:  eingabe.Abschlag[ap.ID],
				Vermietet: store.NewWohnungsregelnOhneHaus(ap).FixkostenDetailMiete(),
			}
			w.Miete, w.MieteErfasst = eingabe.Miete[ap.ID]
			wohnungen = append(wohnungen, w)
		}

		data := struct {
			navData
			Aktuell     string
			Eingabe     *store.FixkostenEingabeDetails
			MonatLabel  string
			AllEingaben []fixkostenListItem
			Wohnungen   []fixkostenDetailApartment
			Positionen  []fixkostenDetailPosition
			Preise      []fixkostenDetailPreis
			Haekchen    []fixkostenDetailHaekchen
			KostenW1    float64
			KostenW2    float64
		}{
			navData:     z.NavData(r),
			Aktuell:     "fixkosten-detail",
			Eingabe:     eingabe,
			MonatLabel:  store.MonatLabel(eingabe.Monat),
			AllEingaben: allItems,
			Wohnungen:   wohnungen,
			Positionen:  positionen,
			KostenW1:    erg.KostenW1,
			KostenW2:    erg.KostenW2,
		}
		data.Preise, data.Haekchen = buildFixkostenDetailPreise(eingabe)

		if err := fixkostenDetailTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
