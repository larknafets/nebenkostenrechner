package web

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// parseFormFloat reads and parses one form field, returning a German
// user-facing error naming fieldLabel/apartmentID on failure - shared by
// /stammdaten's per-apartment Wohnungsgröße/Flurstücksgröße fields.
func parseFormFloat(r *http.Request, name, fieldLabel, apartmentID string) (float64, error) {
	v, err := strconv.ParseFloat(r.FormValue(name), 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s for apartment %s", fieldLabel, apartmentID)
	}
	return v, nil
}

// handleStammdatenForm serves the /stammdaten page (Issue #61): each
// apartment's current Wohnungsgröße/Flurstücksgröße, editable as live
// values - not historized per Ablesung like the rest of the monthly form.
func handleStammdatenForm(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
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
		haus, err := store.GetHaus(db)
		if err != nil {
			http.Error(w, "haus: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			navData
			Aktuell          string
			Apartments       []store.Apartment
			Kostenpositionen []store.Kostenposition
			Haus             store.Haus
		}{
			navData:          a.NavData(r),
			Aktuell:          "stammdaten",
			Apartments:       apartments,
			Kostenpositionen: kostenpositionen,
			Haus:             haus,
		}

		if err := stammdatenTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// handleUpdateStammdaten saves every apartment's Wohnungsgröße/
// Flurstücksgröße and the umlagefähig/Strom flags from the /stammdaten form.
func handleUpdateStammdaten() http.HandlerFunc {
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

		in := make(map[int64]store.StammdatenInput, len(apartments))
		for _, a := range apartments {
			idStr := strconv.FormatInt(a.ID, 10)
			qm, err := parseFormFloat(r, "qm_"+idStr, "Wohnungsgröße", idStr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			flurstueckGroesse, err := parseFormFloat(r, "flurstueck_groesse_"+idStr, "Flurstücksgröße", idStr)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			in[a.ID] = store.StammdatenInput{QM: qm, FlurstueckGroesse: flurstueckGroesse}
		}

		kostenpositionen, err := store.Kostenpositionen(db)
		if err != nil {
			http.Error(w, "kostenpositionen: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// An unchecked checkbox is simply absent from the form, so a
		// missing field means "No".
		flags := store.StammdatenFlags{
			Umlagefaehig:         make(map[int64]bool, len(kostenpositionen)),
			StromWeiterberechnen: r.FormValue("strom_weiterberechnen") == "1",
		}
		for _, kp := range kostenpositionen {
			flags.Umlagefaehig[kp.ID] = r.FormValue("umlagefaehig_"+strconv.FormatInt(kp.ID, 10)) == "1"
		}

		if err := store.SaveStammdaten(db, in, flags); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, requestBase(r)+"/stammdaten", http.StatusFound)
	}
}
