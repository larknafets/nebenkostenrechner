package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

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

// heizungGewichtungLabels are the radio labels of the allowed Heizungs-
// Gewichtungen, keyed by the stored share of the heat consumption.
var heizungGewichtungLabels = map[float64]string{0.7: "70 % / 30 %", 0.6: "60 % / 40 %", 0.5: "50 % / 50 %"}

type heizungGewichtungOption struct {
	Value    string
	Label    string
	Selected bool
}

// heizungGewichtungOptionsFor builds the radio options with the current
// value selected.
func heizungGewichtungOptionsFor(current float64) []heizungGewichtungOption {
	opts := make([]heizungGewichtungOption, 0, len(store.HeizungGewichtungOptions))
	for _, v := range store.HeizungGewichtungOptions {
		opts = append(opts, heizungGewichtungOption{
			Value:    strconv.FormatFloat(v, 'f', -1, 64),
			Label:    heizungGewichtungLabels[v],
			Selected: v == current,
		})
	}
	return opts
}

// parseHeizungGewichtung validates the form value against the allowed
// Heizungs-Gewichtungen.
func parseHeizungGewichtung(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || !store.ValidHeizungGewichtung(v) {
		return 0, fmt.Errorf("ungültige Heizungs-Gewichtung %q (muss 0.7, 0.6 oder 0.5 sein)", raw)
	}
	return v, nil
}

// formText reads a free-text form field: surrounding whitespace is trimmed
// and line breaks are normalized to \n (a textarea submits \r\n).
func formText(r *http.Request, name string) string {
	return strings.TrimSpace(strings.ReplaceAll(r.FormValue(name), "\r\n", "\n"))
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

		nav := a.NavData(r)
		if !nav.IsLoggedIn {
			// Personal data never leaves the server for visitors who are not
			// logged in (Issue #164) - blank it here instead of relying on
			// the template alone. The Wohnungsstatus is no personal data and
			// stays.
			for i := range apartments {
				apartments[i].MieterName = ""
				apartments[i].MieterAnschrift = ""
			}
			haus.VermieterName, haus.VermieterAnschrift, haus.ObjektAnschrift, haus.IBAN, haus.Kontoinhaber = "", "", "", "", ""
		}

		data := struct {
			navData
			Aktuell          string
			Apartments       []store.Apartment
			Kostenpositionen []store.Kostenposition
			Haus             store.Haus
			GewichtungOpts   []heizungGewichtungOption
		}{
			navData:          nav,
			Aktuell:          "stammdaten",
			Apartments:       apartments,
			Kostenpositionen: kostenpositionen,
			Haus:             haus,
			GewichtungOpts:   heizungGewichtungOptionsFor(haus.HeizungWaermeGewichtung),
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
		wohnungen := make(map[int64]store.WohnungDetails, len(apartments))
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

			status := r.FormValue("status_" + idStr)
			if !store.ValidStatus(status) {
				http.Error(w, "ungültiger Wohnungsstatus für Wohnung "+idStr, http.StatusBadRequest)
				return
			}
			wohnungen[a.ID] = store.WohnungDetails{
				MieterName:      formText(r, "mieter_name_"+idStr),
				MieterAnschrift: formText(r, "mieter_anschrift_"+idStr),
				Status:          status,
			}
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

		gewichtung, err := parseHeizungGewichtung(r.FormValue("heizung_gewichtung"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		save := store.StammdatenSave{
			Apartments:              in,
			Flags:                   flags,
			HeizungWaermeGewichtung: gewichtung,
			Wohnungen:               wohnungen,
			Haus: store.HausDetails{
				VermieterName:      formText(r, "vermieter_name"),
				VermieterAnschrift: formText(r, "vermieter_anschrift"),
				ObjektAnschrift:    formText(r, "objekt_anschrift"),
				IBAN:               formText(r, "iban"),
				Kontoinhaber:       formText(r, "kontoinhaber"),
			},
		}
		if err := store.SaveStammdaten(db, save); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}

		http.Redirect(w, r, requestBase(r)+"/stammdaten", http.StatusFound)
	}
}
