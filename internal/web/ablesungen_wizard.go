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
// (recording, a fresh reading, prefilled from the previous period as a
// convenience) and "korrigieren" (correcting, Ticket #34: editing the
// latest reading in place, prefilled with its own current values).
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

	// Prefill for the "Preise & Personen" (prices & occupants) step
	// (Ticket #28): the previous period's values in create mode, or
	// (Ticket #34) the reading's own current values in edit mode -
	// either way just an editable starting value, not a data-prev
	// warning target like the meter readings above.
	PreviousStrompreis        float64
	PreviousFrischwasserPreis float64
	PreviousAbwasserPreis     float64
	PreviousEinspeisungPreis  float64
	PreviousPersonen          map[int64]int64
	// PreviousXErfasst (Ticket #131): whether the respective price was
	// even already set in a completed Teilstand (partial reading) -
	// Previous* above is always a plain float64 (store.OrZero-coalesced),
	// which can no longer distinguish "missing" from "0"; the wizard
	// needs that distinction for the value= prefill (empty instead of
	// "0") and the live progress indicator.
	PreviousStrompreisErfasst        bool
	PreviousFrischwasserPreisErfasst bool
	PreviousAbwasserPreisErfasst     bool
	PreviousEinspeisungPreisErfasst  bool
	// PreviousHeizungGewichtung always has a valid value (defaulting to
	// 0.7, Ticket #27's default) since the radio group needs exactly one
	// option checked - unlike the blank-when-absent price fields above,
	// this can't just be left empty.
	PreviousHeizungGewichtung float64

	// NoPeriods gates the CSV import button (Ticket #54) - only offered
	// as a bootstrap path into a genuinely empty database, never set in
	// edit mode (handleEditWizardForm leaves it at its zero value,
	// false).
	NoPeriods bool
}

// openTeilstandID returns the id of the chronologically newest period, if
// it's a Teilstand (partial reading) - 0/false if there's no period yet
// or the newest one is already complete. Only the newest period may ever
// be incomplete (Ticket #129), so this is the one check that decides
// whether creating another new reading must be blocked, instead of
// allowing a second open reading.
func openTeilstandID(db *sql.DB) (id int64, ok bool, err error) {
	latest, err := store.GetLatestPeriod(db)
	if err != nil {
		return 0, false, fmt.Errorf("latest period: %w", err)
	}
	if latest == nil {
		return 0, false, nil
	}
	complete, err := store.PeriodComplete(db, latest.ID)
	if err != nil {
		return 0, false, fmt.Errorf("period complete: %w", err)
	}
	return latest.ID, !complete, nil
}

// redirectIfOpenTeilstand redirects to the open Teilstand's completion
// (Vervollständigung) and reports whether it did - the shared "block a
// second open reading" check for both handleWizardForm (GET) and
// handleCreateAblesung (POST). Callers must return immediately when this
// reports true (or a non-nil err, which it has already turned into a 500).
func redirectIfOpenTeilstand(w http.ResponseWriter, r *http.Request, db *sql.DB) (redirected bool) {
	teilstandID, hasOpenTeilstand, err := openTeilstandID(db)
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

func handleWizardForm(a auth) http.HandlerFunc {
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
			navData:                   a.NavData(r),
			Aktuell:                   "ablesungen",
			FormAction:                requestBase(r) + "/ablesungen",
			ReadingDate:               time.Now().Format("2006-01-02"),
			Monat:                     time.Now().Format("2006-01"),
			Apartments:                apartments,
			PreviousHeizungGewichtung: 0.7,
			NoPeriods:                 previousPeriod == nil,
		}
		if len(recent) > 0 {
			data.HasPrevious = true
			data.PreviousReadings = recent[0].Readings
			data.PreviousReadingDate = recent[0].ReadingDate
		}
		if previousPeriod != nil {
			// store.OrZero: previousPeriod is Teilstand-capable (partial-
			// reading-capable, Ticket #128) - but always complete here,
			// since a new reading can't be created while the previous
			// one is still a Teilstand (openTeilstandID/
			// redirectIfOpenTeilstand, Ticket #129). The Erfasst
			// (recorded) flags below are therefore always true,
			// computed directly from the same source instead of
			// hardcoded to true, so they stay correct if this invariant
			// ever changes.
			status := newTeilstandStatus(previousPeriod, apartments)
			data.PreviousStrompreis = store.OrZero(previousPeriod.Strompreis)
			data.PreviousFrischwasserPreis = store.OrZero(previousPeriod.FrischwasserPreis)
			data.PreviousAbwasserPreis = store.OrZero(previousPeriod.AbwasserPreis)
			data.PreviousEinspeisungPreis = store.OrZero(previousPeriod.EinspeisungPreis)
			data.PreviousStrompreisErfasst = status.StrompreisErfasst
			data.PreviousFrischwasserPreisErfasst = status.FrischwasserErfasst
			data.PreviousAbwasserPreisErfasst = status.AbwasserErfasst
			data.PreviousEinspeisungPreisErfasst = status.EinspeisungPreisErfasst
			data.PreviousPersonen = previousPeriod.PersonenByApartment
			data.PreviousHeizungGewichtung = previousPeriod.HeizungWaermeGewichtung
		}
		data.OutlierAvg, data.HasOutlierBaseline = outlierAvg(recent)

		if err := wizardTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// requireLoginUnlessTeilstand wraps next with a's login gate, except when
// the request's {id} path value names a Teilstand (partial reading,
// Ticket #130) - completing an open reading needs no login, just as
// little as creating it in the first place (Ticket #129). An already
// complete reading stays gated as before. An id that doesn't (or no
// longer) exist returns 404 directly instead of a login redirect - its
// existence isn't information worth protecting here. Only intended for
// routes with a numeric {id} path parameter that are also already behind
// withDB (dbFromContext needs its DB in the context).
func requireLoginUnlessTeilstand(a auth, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		periodID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid period id", http.StatusBadRequest)
			return
		}
		complete, err := store.PeriodComplete(db, periodID)
		if err != nil {
			if errors.Is(err, store.ErrPeriodNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !complete {
			next(w, r)
			return
		}
		a.RequireLogin(next)(w, r)
	}
}

// handleEditWizardForm serves the "korrigieren" (correcting) form for an
// arbitrary reading (Ticket #44 - generalized from Ticket #34's
// latest-only version), prefilled with its own current values. The
// negative-consumption/outlier-warning baseline always compares against
// the genuine previous period - the one chronologically before the
// reading being edited, regardless of whether newer readings exist after
// it.
func handleEditWizardForm(a auth) http.HandlerFunc {
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

		// Ticket #131: target can be a Teilstand (partial reading) - these
		// flags let the wizard distinguish "missing" from "0".
		status := newTeilstandStatus(target, apartments)

		data := wizardData{
			navData:                          a.NavData(r),
			Aktuell:                          "ablesungen",
			FormAction:                       fmt.Sprintf("%s/ablesungen/%d", requestBase(r), target.ID),
			IsEdit:                           true,
			ReadingDate:                      target.ReadingDate,
			Monat:                            string(monatInputFromStored(target.Monat)),
			Apartments:                       apartments,
			EditReadings:                     target.Readings,
			PreviousStrompreis:               store.OrZero(target.Strompreis),
			PreviousFrischwasserPreis:        store.OrZero(target.FrischwasserPreis),
			PreviousAbwasserPreis:            store.OrZero(target.AbwasserPreis),
			PreviousEinspeisungPreis:         store.OrZero(target.EinspeisungPreis),
			PreviousPersonen:                 target.PersonenByApartment,
			PreviousHeizungGewichtung:        target.HeizungWaermeGewichtung,
			PreviousStrompreisErfasst:        status.StrompreisErfasst,
			PreviousFrischwasserPreisErfasst: status.FrischwasserErfasst,
			PreviousAbwasserPreisErfasst:     status.AbwasserErfasst,
			PreviousEinspeisungPreisErfasst:  status.EinspeisungPreisErfasst,
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

// heizungGewichtungOptions are the only allowed heating-split weightings
// (Ticket #27) - a fixed choice, not a free-text field, so an invalid or
// out-of-range value can't silently skew every apartment's heating costs.
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
// short to safely take the first 7 characters from - CreatePeriod never
// validates Monat (only UpdatePeriod does, see checkMonatNeighbors), so a
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

// parseOptionalFloat parses a Teilstand-capable (partial-reading-capable)
// form field (Ticket #129): deliberately left blank (raw == "") is not an
// error, just nil - a value actually typed in but invalid (e.g. text
// instead of a number) stays an error.
func parseOptionalFloat(raw string) (*float64, error) {
	if raw == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func parsePeriodInput(r *http.Request, apartments []store.Apartment) (store.PeriodInput, error) {
	readings := make(map[string]float64, len(store.MeterKeys))
	for _, key := range store.MeterKeys {
		raw := r.FormValue(key)
		if raw == "" {
			continue // Teilstand (Ticket #129): deliberately left blank
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return store.PeriodInput{}, fmt.Errorf("invalid value for %s", key)
		}
		readings[key] = v
	}

	strompreis, err1 := parseOptionalFloat(r.FormValue("strompreis"))
	frischwasserPreis, err2 := parseOptionalFloat(r.FormValue("frischwasser_preis"))
	abwasserPreis, err3 := parseOptionalFloat(r.FormValue("abwasser_preis"))
	einspeisungPreis, err4 := parseOptionalFloat(r.FormValue("einspeisung_preis"))
	if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
		return store.PeriodInput{}, fmt.Errorf("invalid price value")
	}

	heizungGewichtung, err := parseHeizungGewichtung(r.FormValue("heizung_gewichtung"))
	if err != nil {
		return store.PeriodInput{}, err
	}

	personen := make(map[int64]int64, len(apartments))
	for _, a := range apartments {
		idStr := strconv.FormatInt(a.ID, 10)
		raw := r.FormValue("personen_" + idStr)
		if raw == "" {
			continue // Teilstand (Ticket #129): deliberately left blank
		}
		p, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return store.PeriodInput{}, fmt.Errorf("invalid Personenzahl for apartment %s", idStr)
		}
		personen[a.ID] = p
	}

	return store.PeriodInput{
		ReadingDate:             r.FormValue("reading_date"),
		Monat:                   monatInput(r.FormValue("monat")).toStored(),
		Strompreis:              strompreis,
		FrischwasserPreis:       frischwasserPreis,
		AbwasserPreis:           abwasserPreis,
		HeizungWaermeGewichtung: heizungGewichtung,
		EinspeisungPreis:        einspeisungPreis,
		Readings:                readings,
		Personen:                personen,
	}, nil
}

func handleCreateAblesung() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}

		// Teilstand/partial reading (Ticket #129): see
		// redirectIfOpenTeilstand - a second open reading must not be
		// created, not even via a direct POST (e.g. a stale form).
		if redirectIfOpenTeilstand(w, r, db) {
			return
		}

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		in, err := parsePeriodInput(r, apartments)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		periodID, err := store.CreatePeriod(db, in)
		if err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, fmt.Sprintf("%s/ablesungen/%d", requestBase(r), periodID), http.StatusFound)
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

		apartments, err := store.Apartments(db)
		if err != nil {
			http.Error(w, "apartments: "+err.Error(), http.StatusInternalServerError)
			return
		}

		in, err := parsePeriodInput(r, apartments)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if err := store.UpdatePeriod(db, periodID, in); err != nil {
			var tooEarly *store.PeriodDateTooEarlyError
			var tooLate *store.PeriodDateTooLateError
			var monatTooEarly *store.PeriodMonatTooEarlyError
			var monatTooLate *store.PeriodMonatTooLateError
			switch {
			case errors.As(err, &tooEarly):
				http.Error(w, fmt.Sprintf("Ablesedatum muss nach der Vorperiode (%s) liegen", tooEarly.Neighbor), http.StatusBadRequest)
			case errors.As(err, &tooLate):
				http.Error(w, fmt.Sprintf("Ablesedatum muss vor der Folgeperiode (%s) liegen", tooLate.Neighbor), http.StatusBadRequest)
			case errors.As(err, &monatTooEarly):
				http.Error(w, fmt.Sprintf("Abrechnungsmonat darf nicht vor dem der Vorperiode (%s) liegen", monatTooEarly.Neighbor), http.StatusBadRequest)
			case errors.As(err, &monatTooLate):
				http.Error(w, fmt.Sprintf("Abrechnungsmonat darf nicht nach dem der Folgeperiode (%s) liegen", monatTooLate.Neighbor), http.StatusBadRequest)
			default:
				http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}

		http.Redirect(w, r, fmt.Sprintf("%s/ablesungen/%d", requestBase(r), periodID), http.StatusFound)
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
