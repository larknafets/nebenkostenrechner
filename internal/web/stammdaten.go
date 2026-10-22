package web

import (
	"errors"
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
		return 0, fmt.Errorf("ungültiger Wert für %s bei Wohnung %s", fieldLabel, apartmentID)
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

// parseHeizungGewichtung reads the Heizungs-Gewichtung from the form. Whether
// the value is an allowed one is decided by store.SaveStammdaten.
func parseHeizungGewichtung(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
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
				apartments[i].MieterSeit = ""
			}
			haus.VermieterName, haus.VermieterAnschrift, haus.ObjektAnschrift, haus.IBAN, haus.Kontoinhaber = "", "", "", "", ""
		}

		data := struct {
			navData
			Aktuell          string
			Apartments       []store.Apartment
			Kostenpositionen []store.Kostenposition
			LetzterIndex     int // Index der letzten Kostenposition, sperrt "nach unten"
			Haus             store.Haus
			GewichtungOpts   []heizungGewichtungOption
		}{
			navData:          nav,
			Aktuell:          "stammdaten",
			Apartments:       apartments,
			Kostenpositionen: kostenpositionen,
			LetzterIndex:     len(kostenpositionen) - 1,
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

			wohnungen[a.ID] = store.WohnungDetails{
				MieterName:      formText(r, "mieter_name_"+idStr),
				MieterAnschrift: formText(r, "mieter_anschrift_"+idStr),
				Status:          r.FormValue("status_" + idStr),
				MieterSeit:      monatInput(r.FormValue("mieter_seit_" + idStr)).toStored(),
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
			Umlagefaehig:           make(map[int64]bool, len(kostenpositionen)),
			StromWeiterberechnen:   r.FormValue("strom_weiterberechnen") == "1",
			WallboxWeiterberechnen: r.FormValue("wallbox_weiterberechnen") == "1",
		}
		for _, kp := range kostenpositionen {
			flags.Umlagefaehig[kp.ID] = r.FormValue("umlagefaehig_"+strconv.FormatInt(kp.ID, 10)) == "1"
		}

		rohGewichtung := r.FormValue("heizung_gewichtung")
		gewichtung, err := parseHeizungGewichtung(rohGewichtung)
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
			var status *store.InvalidStatusError
			var mieterSeit *store.InvalidMieterSeitError
			var ungueltigeGewichtung *store.InvalidHeizungGewichtungError
			switch {
			case errors.As(err, &status):
				http.Error(w, "ungültiger Wohnungsstatus für Wohnung "+strconv.FormatInt(status.ApartmentID, 10), http.StatusBadRequest)
			case errors.As(err, &mieterSeit):
				http.Error(w, "ungültiges Datum für \"Mieter seit\" bei Wohnung "+strconv.FormatInt(mieterSeit.ApartmentID, 10), http.StatusBadRequest)
			case errors.As(err, &ungueltigeGewichtung):
				http.Error(w, fmt.Sprintf("ungültige Heizungs-Gewichtung %q (muss 0.7, 0.6 oder 0.5 sein)", rohGewichtung), http.StatusBadRequest)
			default:
				http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			}
			return
		}

		http.Redirect(w, r, requestBase(r)+"/stammdaten", http.StatusFound)
	}
}

// handleMoveKostenposition moves one Kostenposition up or down in the shared
// display order (Fixkosten-Eingabe, Stammdaten, Abrechnung) and goes back to
// the list on /stammdaten.
func handleMoveKostenposition() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
			return
		}
		id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "ungültige Kostenposition", http.StatusBadRequest)
			return
		}
		var up bool
		switch r.FormValue("richtung") {
		case "hoch":
			up = true
		case "runter":
		default:
			http.Error(w, "ungültige Richtung", http.StatusBadRequest)
			return
		}
		if err := store.MoveKostenposition(db, id, up); err != nil {
			if errors.Is(err, store.ErrKostenpositionNotFound) {
				http.Error(w, "Kostenposition nicht gefunden", http.StatusNotFound)
				return
			}
			http.Error(w, "move: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, requestBase(r)+"/stammdaten#reihenfolge", http.StatusFound)
	}
}
