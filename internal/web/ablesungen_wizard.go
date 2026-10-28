package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// wizardData is the reading form's template data - shared by "erfassen"
// (recording a fresh reading) and "korrigieren" (correcting, Ticket #34:
// editing a reading in place, prefilled with its own current values).
// HasPrevious/PreviousReadings/PreviousReadingDate/OutlierAvg always
// describe the genuine previous period - the negative-consumption/
// outlier-warning baseline the new values are checked against - never
// the reading being edited itself.
type wizardData struct {
	navData
	Aktuell     string
	FormAction  string
	IsEdit      bool
	ReadingDate string
	// Monat is the billing month (Abrechnungsmonat) field's value
	// ("YYYY-MM", <input type="month">'s format), separate from
	// ReadingDate - prefilled from the reading date in create mode, from
	// the reading's own value in edit mode (Issue #86).
	Monat               string
	Apartments          []store.Apartment
	HasPrevious         bool
	PreviousReadings    map[string]float64
	PreviousReadingDate string
	HasOutlierBaseline  bool
	OutlierAvg          map[string]float64

	// EditReadings prefills the meter inputs' value= in edit mode with the
	// reading's own current meter readings (Zählerstände) - distinct from
	// PreviousReadings above, which stays the actual previous period for
	// the warning comparison.
	EditReadings map[string]float64

	// EinzelzaehlerFelder are the optional Wallbox-Einzelzähler inputs
	// (#224), shown below the Wallboxen field.
	EinzelzaehlerFelder []store.Meter

	// WallboxHinweis is the note below the Wallbox-Einzelzähler: the state of
	// "Wallboxen vollständig Wohnung 1" in the Fixkosten-Eingabe of the
	// Abrechnungsmonat (#359), or that the Fixkosten are missing. The switch
	// is no field of the Ablesung. WallboxHinweisJSON lets the script update
	// the note when the month changes.
	WallboxHinweis     string
	WallboxHinweisJSON string

	// NoPeriods gates the CSV import button (Ticket #54) - only offered
	// as a bootstrap path into a genuinely empty database, never set in
	// edit mode (handleEditWizardForm leaves it at its zero value,
	// false).
	NoPeriods bool
}

// einzelzaehlerFelder lists the optional Einzelzähler in form order, taken
// from the store's meter descriptor.
var einzelzaehlerFelder = func() []store.Meter {
	var out []store.Meter
	for _, m := range store.MeterDefs() {
		if m.Optional {
			out = append(out, m)
		}
	}
	return out
}()

// redirectIfOpenTeilstand redirects to the open Teilstand's completion
// (Vervollständigung) and reports whether it did - so the "New reading" form
// leads to completing instead of offering a form the store would refuse
// (store.PeriodOpenTeilstandError). Callers must return immediately when this
// reports true (or a non-nil err, which it has already turned into a 500).
func redirectIfOpenTeilstand(w http.ResponseWriter, r *http.Request, db *sql.DB) (redirected bool) {
	teilstandID, hasOpenTeilstand, err := store.OffenerTeilstand(db)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return true
	}
	if !hasOpenTeilstand {
		return false
	}
	http.Redirect(w, r, fmt.Sprintf("%s/ablesungen/%d/bearbeiten", requestBase(r), teilstandID), http.StatusFound)
	return true
}

func handleWizardForm(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())

		// Teilstand/partial reading (Ticket #129): as long as the newest
		// reading is still open, "New reading" leads to completing it
		// instead - no second open reading in parallel.
		if redirectIfOpenTeilstand(w, r, db) {
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// 4 periods give the 3 consumption diffs the outlier-warning
		// baseline averages over (Ticket #13); the newest of them also
		// doubles as "previous" for the negative-consumption/gap checks
		// (Ticket #12).
		recent, err := store.RecentPeriodReadings(db, 4)
		if err != nil {
			http.Error(w, "recent periods: "+err.Error(), http.StatusInternalServerError)
			return
		}

		previousPeriod, err := store.GetLatestPeriod(db)
		if err != nil {
			http.Error(w, "latest period: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := wizardData{
			navData:     z.NavData(r),
			Aktuell:     "ablesungen",
			FormAction:  requestBase(r) + "/ablesungen",
			ReadingDate: time.Now().Format("2006-01-02"),
			Monat:       time.Now().Format("2006-01"),
			Apartments:  apartments,
			NoPeriods:   previousPeriod == nil,

			EinzelzaehlerFelder: einzelzaehlerFelder,
		}
		if len(recent) > 0 {
			data.HasPrevious = true
			data.PreviousReadings = recent[0].Readings
			data.PreviousReadingDate = recent[0].ReadingDate
		}
		if err := data.setzeWallboxHinweis(db, data.Monat); err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data.OutlierAvg, data.HasOutlierBaseline = outlierAvg(recent)

		if err := wizardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// setzeWallboxHinweis fills the Wallbox note for monat ("YYYY-MM") from the
// Fixkosten-Eingaben.
func (d *wizardData) setzeWallboxHinweis(db *sql.DB, monat string) error {
	eingaben, err := store.AllFixkostenEingabenDetails(db)
	if err != nil {
		return err
	}
	hinweise := neueWallboxHinweisDaten(eingaben)
	d.WallboxHinweis = hinweise.text(monat)
	d.WallboxHinweisJSON = hinweise.json()
	return nil
}

// teilstandAnspruch resolves AnlegenOderVermieter for the request (called by
// Gate): the Anspruch of the routes that correct a reading. Completing a Teilstand (partial reading, Ticket #130)
// needs only the right to create a reading, just as creating it in the first
// place (Ticket #129). An already complete reading stays reserved for the
// Vermieter. An id that doesn't (or no longer) exist returns 404 directly
// instead of a login redirect - its existence isn't information worth
// protecting here. Only intended for routes with a numeric {id} path
// parameter that are also already behind withDB (dbFromContext needs its DB in
// the context).
func teilstandAnspruch(w http.ResponseWriter, r *http.Request) (Anspruch, bool) {
	db := dbFromContext(r.Context())
	periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid period id", http.StatusBadRequest)
		return Anspruch{}, false
	}
	complete, err := store.PeriodComplete(db, periodID)
	if err != nil {
		if errors.Is(err, store.ErrPeriodNotFound) {
			http.NotFound(w, r)
			return Anspruch{}, false
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return Anspruch{}, false
	}
	if !complete {
		return Braucht(store.RechtAblesungAnlegen), true
	}
	return NurVermieter, true
}

// handleEditWizardForm serves the "korrigieren" (correcting) form for an
// arbitrary reading (Ticket #44 - generalized from Ticket #34's
// latest-only version), prefilled with its own current values. The
// negative-consumption/outlier-warning baseline always compares against
// the genuine previous period - the one chronologically before the
// reading being edited, regardless of whether newer readings exist after
// it.
func handleEditWizardForm(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		target, err := store.GetPeriodDetails(db, periodID)
		if err != nil {
			http.Error(w, "period: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if target == nil {
			http.NotFound(w, r)
			return
		}

		recent, err := store.PeriodReadingsBefore(db, periodID, 4)
		if err != nil {
			http.Error(w, "recent periods: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := wizardData{
			navData:             z.NavData(r),
			Aktuell:             "ablesungen",
			FormAction:          fmt.Sprintf("%s/ablesungen/%d", requestBase(r), target.ID),
			IsEdit:              true,
			ReadingDate:         target.ReadingDate,
			Monat:               string(monatInputFromStored(target.Monat)),
			Apartments:          apartments,
			EditReadings:        target.Readings,
			EinzelzaehlerFelder: einzelzaehlerFelder,
		}
		if err := data.setzeWallboxHinweis(db, data.Monat); err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if len(recent) > 0 {
			data.HasPrevious = true
			data.PreviousReadings = recent[0].Readings
			data.PreviousReadingDate = recent[0].ReadingDate
		}
		data.OutlierAvg, data.HasOutlierBaseline = outlierAvg(recent)

		if err := wizardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// parsePeriodInput parses a reading form (shared by handleCreateAblesung
// and handleUpdateAblesung - Ticket #34, same fields either way, only what
// happens with the result differs).
// monatInput is an <input type="month">'s value ("YYYY-MM") - the wizard-
// facing billing-month (Abrechnungsmonat) format, distinct from
// periods.monat's persisted "YYYY-MM-01" (Issue #86 code review: this was
// scattered as ad-hoc string slicing/concatenation across parsePeriodInput
// and the edit form).
type monatInput string

// monatInputFromStored converts periods.monat ("YYYY-MM-01") to its
// <input type="month"> value ("YYYY-MM"). Returns "" if stored is too
// short to safely take the first 7 characters from - the store checks the
// order of Monat values (see store/reihe.go), not their format, so a
// malformed value can in principle reach the edit form; a blank field beats
// a panic.
func monatInputFromStored(stored string) monatInput {
	if len(stored) < 7 {
		return ""
	}
	return monatInput(stored[:7])
}

// toStored converts the wizard field's value back to periods.monat's
// format, appending "-01" to a bare "YYYY-MM" if needed. Already-
// normalized input passes through unchanged.
func (m monatInput) toStored() string {
	if len(m) == 7 {
		return string(m) + "-01"
	}
	return string(m)
}

func parsePeriodInput(r *http.Request) (store.PeriodInput, error) {
	readings := make(map[string]float64, len(warnedMeterKeys))
	for _, key := range warnedMeterKeys {
		raw := r.FormValue(key)
		if raw == "" {
			// Teilstand (Ticket #129): deliberately left blank. For an
			// optional Einzelzähler (#224) it simply means "nicht erfasst".
			continue
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return store.PeriodInput{}, fmt.Errorf("ungültiger Zählerstand für %s", key)
		}
		readings[key] = v
	}

	in := store.PeriodInput{
		ReadingDate: r.FormValue("reading_date"),
		Monat:       monatInput(r.FormValue("monat")).toStored(),
		Readings:    readings,
	}
	if err := in.Validate(); err != nil {
		return store.PeriodInput{}, ablesungFormularFehler(err)
	}
	return in, nil
}

// ablesungFormFehler is a parsePeriodInput error that names the form field it is
// about, so the 400 response can carry the field for the Fehler-Overlay.
type ablesungFormFehler struct{ msg, feld string }

func (e *ablesungFormFehler) Error() string { return e.msg }

// ablesungFormularFehler turns the *store.EingabeFehler of PeriodInput.Validate
// into the German form message.
func ablesungFormularFehler(err error) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return err
	}
	switch ef.Feld {
	case store.FeldReadingDate:
		return &ablesungFormFehler{fmt.Sprintf("ungültiges Ablesedatum %q (Format JJJJ-MM-TT)", ef.Wert), "reading_date"}
	case store.FeldMonat:
		return &ablesungFormFehler{fmt.Sprintf("ungültiger Abrechnungsmonat %q (Format JJJJ-MM)", ef.Wert), "monat"}
	}
	return err
}

// ablesungStoreFehler words a *store.EingabeFehler that CreatePeriod or
// UpdatePeriod returned (a caller that skipped parsePeriodInput). It returns
// nil for every other error.
func ablesungStoreFehler(err error) error {
	var ef *store.EingabeFehler
	if !errors.As(err, &ef) {
		return nil
	}
	return ablesungFormularFehler(ef)
}

// badRequestParse answers a parsePeriodInput error with 400, with the field
// name if the error carries one.
func badRequestParse(w http.ResponseWriter, err error) {
	var f *ablesungFormFehler
	if errors.As(err, &f) {
		badRequestFeld(w, f.msg, f.feld)
		return
	}
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func handleCreateAblesung() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}

		in, err := parsePeriodInput(r)
		if err != nil {
			badRequestParse(w, err)
			return
		}

		periodID, err := store.CreatePeriod(db, in)
		if err != nil {
			// Teilstand/partial reading (Ticket #129): a second open
			// reading is refused by the store, even for a direct POST
			// (e.g. a stale form). Lead to completing the open one, like
			// the GET form does.
			var open *store.PeriodOpenTeilstandError
			if errors.As(err, &open) {
				http.Redirect(w, r, fmt.Sprintf("%s/ablesungen/%d/bearbeiten", requestBase(r), open.ID), http.StatusFound)
				return
			}
			if msg, feld, ok := reiheFehler(err); ok {
				badRequestFeld(w, msg, feld)
				return
			}
			if mapped := ablesungStoreFehler(err); mapped != nil {
				badRequestParse(w, mapped)
				return
			}
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		ziel, err := ablesungGespeichertURL(r, db, periodID)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, ziel, http.StatusFound)
	}
}

// handleUpdateAblesung corrects an existing reading in place (Ticket #34,
// generalized to any period by Ticket #44 - no restriction to the latest
// one anymore, see store.UpdatePeriod). The neighbor-date reorder guard
// lives in store.UpdatePeriod itself; this handler only translates its
// typed errors into the German user-facing messages.
func handleUpdateAblesung() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}

		existing, err := store.GetPeriodDetails(db, periodID)
		if err != nil {
			http.Error(w, "period: "+err.Error(), http.StatusInternalServerError)
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

		in, err := parsePeriodInput(r)
		if err != nil {
			badRequestParse(w, err)
			return
		}

		if err := store.UpdatePeriod(db, periodID, in); err != nil {
			if msg, feld, ok := reiheFehler(err); ok {
				badRequestFeld(w, msg, feld)
			} else if mapped := ablesungStoreFehler(err); mapped != nil {
				badRequestParse(w, mapped)
			} else {
				http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}

		ziel, err := ablesungGespeichertURL(r, db, periodID)
		if err != nil {
			http.Error(w, "fixkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, ziel, http.StatusFound)
	}
}

// handleDeleteAblesung deletes a period (Ticket #45). Any period is
// deletable, including the last remaining one - no "closed" (abgeschlossen)
// status exists in this app; the client-side confirm() dialog is the only
// safety net (see ablesung.html).
func handleDeleteAblesung() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}

		if err := store.DeletePeriod(db, periodID); err != nil {
			if errors.Is(err, store.ErrPeriodNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, requestBase(r)+"/ablesungen", http.StatusFound)
	}
}
