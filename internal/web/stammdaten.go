package web

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// formularFehler is a user-facing problem with the submitted form (a field
// that does not parse). stammdatenFehler answers it with 400 and its text.
type formularFehler struct{ err error }

func (e *formularFehler) Error() string { return e.err.Error() }
func (e *formularFehler) Unwrap() error { return e.err }

func formFehler(format string, args ...any) error {
	return &formularFehler{err: fmt.Errorf(format, args...)}
}

// parseFormFloat reads and parses one form field, returning a German
// user-facing error naming fieldLabel/apartmentID on failure - shared by
// /stammdaten's per-apartment Wohnungsgröße/Flurstücksgröße fields.
func parseFormFloat(form url.Values, name, fieldLabel, apartmentID string) (float64, error) {
	v, err := strconv.ParseFloat(form.Get(name), 64)
	if err != nil {
		return 0, formFehler("ungültiger Wert für %s bei Wohnung %s", fieldLabel, apartmentID)
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

// heizungGewichtungFehler is the user-facing message for a form value that is
// no allowed Heizungs-Gewichtung, built from store.HeizungGewichtungOptions.
func heizungGewichtungFehler(raw string) error {
	werte := make([]string, len(store.HeizungGewichtungOptions))
	for i, v := range store.HeizungGewichtungOptions {
		werte[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	erlaubt := strings.Join(werte[:len(werte)-1], ", ") + " oder " + werte[len(werte)-1]
	return fmt.Errorf("ungültige Heizungs-Gewichtung %q (muss %s sein)", raw, erlaubt)
}

// parseHeizungGewichtung reads the Heizungs-Gewichtung from the form. Whether
// the value is an allowed one is decided by store.SaveStammdaten.
func parseHeizungGewichtung(raw string) (float64, error) {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, &formularFehler{err: heizungGewichtungFehler(raw)}
	}
	return v, nil
}

// formText reads a free-text form field: surrounding whitespace is trimmed
// and line breaks are normalized to \n (a textarea submits \r\n).
func formText(form url.Values, name string) string {
	return strings.TrimSpace(strings.ReplaceAll(form.Get(name), "\r\n", "\n"))
}

// berechtigungZeile is one row of the Berechtigungen table on /stammdaten: a
// right with its checkbox state per role.
type berechtigungZeile struct {
	Recht           string
	Label           string
	NichtAngemeldet bool
	Mieter          bool
}

func berechtigungZeilen(b store.Berechtigungen) []berechtigungZeile {
	zeilen := make([]berechtigungZeile, 0, len(store.Rechte))
	for _, info := range store.Rechte {
		zeilen = append(zeilen, berechtigungZeile{
			Recht:           string(info.Recht),
			Label:           info.Label,
			NichtAngemeldet: b.Erlaubt(store.RolleNichtAngemeldet, info.Recht),
			Mieter:          b.Erlaubt(store.RolleMieter, info.Recht),
		})
	}
	return zeilen
}

// berechtigungenAusFormular overlays the checkboxes of the form onto the saved
// rights (which it leaves untouched, it works on a copy). The form names the
// role columns it contains in the hidden fields "berechtigung_rolle" (the
// Mieter column is missing without a Mieter login), the other columns keep
// their saved values. An unchecked checkbox is absent from the form, so a
// missing field in a shown column means "No".
func berechtigungenAusFormular(form url.Values, gespeichert store.Berechtigungen) store.Berechtigungen {
	neu := make(store.Berechtigungen, len(gespeichert))
	for rolle, rechte := range gespeichert {
		neu[rolle] = maps.Clone(rechte)
	}
	for _, rolle := range form["berechtigung_rolle"] {
		for _, info := range store.Rechte {
			if _, bekannt := neu[store.Rolle(rolle)]; !bekannt {
				continue
			}
			neu[store.Rolle(rolle)][info.Recht] = form.Get("recht_"+rolle+"_"+string(info.Recht)) == "1"
		}
	}
	return neu
}

// stammdatenAusFormular turns the submitted /stammdaten form into what the
// store saves, without HTTP and database: the caller hands in the apartments,
// the Kostenpositionen and the saved Berechtigungen. Only a form that carries
// the Berechtigungen table touches them (nil leaves them untouched). Whether
// a Wohnungsstatus, "Mieter seit" or Gewichtung is allowed is decided by
// store.SaveStammdaten.
func stammdatenAusFormular(form url.Values, wohnungen []store.Apartment, kostenpositionen []store.Kostenposition, gespeichert store.Berechtigungen) (store.StammdatenSave, error) {
	in := make(map[int64]store.StammdatenInput, len(wohnungen))
	details := make(map[int64]store.WohnungDetails, len(wohnungen))
	for _, a := range wohnungen {
		idStr := strconv.FormatInt(a.ID, 10)
		qm, err := parseFormFloat(form, "qm_"+idStr, "Wohnungsgröße", idStr)
		if err != nil {
			return store.StammdatenSave{}, err
		}
		flurstueckGroesse, err := parseFormFloat(form, "flurstueck_groesse_"+idStr, "Flurstücksgröße", idStr)
		if err != nil {
			return store.StammdatenSave{}, err
		}
		in[a.ID] = store.StammdatenInput{QM: qm, FlurstueckGroesse: flurstueckGroesse}
		details[a.ID] = store.WohnungDetails{
			MieterName:      formText(form, "mieter_name_"+idStr),
			MieterAnschrift: formText(form, "mieter_anschrift_"+idStr),
			Status:          form.Get("status_" + idStr),
			MieterSeit:      monatInput(form.Get("mieter_seit_" + idStr)).toStored(),
		}
	}

	// An unchecked checkbox is simply absent from the form, so a missing
	// field means "No".
	flags := store.StammdatenFlags{
		Umlagefaehig:           make(map[int64]bool, len(kostenpositionen)),
		StromWeiterberechnen:   form.Get("strom_weiterberechnen") == "1",
		WallboxWeiterberechnen: form.Get("wallbox_weiterberechnen") == "1",
	}
	for _, kp := range kostenpositionen {
		flags.Umlagefaehig[kp.ID] = form.Get("umlagefaehig_"+strconv.FormatInt(kp.ID, 10)) == "1"
	}

	gewichtung, err := parseHeizungGewichtung(form.Get("heizung_gewichtung"))
	if err != nil {
		return store.StammdatenSave{}, err
	}
	var berechtigungen store.Berechtigungen
	if len(form["berechtigung_rolle"]) > 0 {
		berechtigungen = berechtigungenAusFormular(form, gespeichert)
	}
	return store.StammdatenSave{
		Berechtigungen:          berechtigungen,
		Apartments:              in,
		Flags:                   flags,
		HeizungWaermeGewichtung: gewichtung,
		Wohnungen:               details,
		Haus: store.HausDetails{
			VermieterName:      formText(form, "vermieter_name"),
			VermieterAnschrift: formText(form, "vermieter_anschrift"),
			ObjektAnschrift:    formText(form, "objekt_anschrift"),
			IBAN:               formText(form, "iban"),
			Kontoinhaber:       formText(form, "kontoinhaber"),
		},
	}, nil
}

// stammdatenFehler maps an error of stammdatenAusFormular or of
// store.SaveStammdaten to the HTTP status and the German text of the answer:
// 400 for a form error and for the three errors of saving, 500 for the rest.
// rohGewichtung is the Heizungs-Gewichtung as submitted, for its message.
func stammdatenFehler(err error, rohGewichtung string) (status int, text string) {
	var form *formularFehler
	var wohnungsstatus *store.InvalidStatusError
	var mieterSeit *store.InvalidMieterSeitError
	var ungueltigeGewichtung *store.InvalidHeizungGewichtungError
	switch {
	case errors.As(err, &form):
		return http.StatusBadRequest, form.Error()
	case errors.As(err, &wohnungsstatus):
		return http.StatusBadRequest, "ungültiger Wohnungsstatus für Wohnung " + strconv.FormatInt(wohnungsstatus.ApartmentID, 10)
	case errors.As(err, &mieterSeit):
		return http.StatusBadRequest, "ungültiges Datum für \"Mieter seit\" bei Wohnung " + strconv.FormatInt(mieterSeit.ApartmentID, 10)
	case errors.As(err, &ungueltigeGewichtung):
		return http.StatusBadRequest, heizungGewichtungFehler(rohGewichtung).Error()
	}
	return http.StatusInternalServerError, "save: " + err.Error()
}

// stammdatenSicht is what the viewer of z gets to see on /stammdaten: without
// Personenbezug the names, addresses, "Mieter seit" and bank details are
// blanked (Issue #164). The Wohnungsstatus is no personal data and stays.
// The arguments are left untouched.
func stammdatenSicht(z Zugriff, wohnungen []store.Apartment, haus store.Haus) ([]store.Apartment, store.Haus) {
	if z.Personenbezug() {
		return wohnungen, haus
	}
	sicht := slices.Clone(wohnungen)
	for i := range sicht {
		sicht[i].MieterName = ""
		sicht[i].MieterAnschrift = ""
		sicht[i].MieterSeit = ""
	}
	haus.VermieterName, haus.VermieterAnschrift, haus.ObjektAnschrift, haus.IBAN, haus.Kontoinhaber = "", "", "", "", ""
	return sicht, haus
}

// handleStammdatenForm serves the /stammdaten page (Issue #61): each
// apartment's current Wohnungsgröße/Flurstücksgröße, editable as live
// values - not historized per Ablesung like the rest of the monthly form.
func handleStammdatenForm(z *Zugang) http.HandlerFunc {
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

		nav := z.NavData(r)
		// Personal data never leaves the server for visitors who are not
		// logged in (Issue #164) - blanked here instead of relying on the
		// template alone.
		apartments, haus = stammdatenSicht(nav.Zugriff, apartments, haus)

		berechtigungen, err := store.GetBerechtigungen(db)
		if err != nil {
			http.Error(w, "berechtigungen: "+err.Error(), http.StatusInternalServerError)
			return
		}

		data := struct {
			navData
			Aktuell          string
			Berechtigungen   []berechtigungZeile
			Apartments       []store.Apartment
			Kostenpositionen []store.Kostenposition
			LetzterIndex     int // Index der letzten Kostenposition, sperrt "nach unten"
			Haus             store.Haus
			GewichtungOpts   []heizungGewichtungOption
		}{
			navData:          nav,
			Aktuell:          "stammdaten",
			Berechtigungen:   berechtigungZeilen(berechtigungen),
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

		kostenpositionen, err := store.Kostenpositionen(db)
		if err != nil {
			http.Error(w, "kostenpositionen: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Only a form that carries the Berechtigungen table needs the saved
		// ones; every other form leaves them alone (nil).
		var gespeichert store.Berechtigungen
		if len(r.Form["berechtigung_rolle"]) > 0 {
			gespeichert, err = store.GetBerechtigungen(db)
			if err != nil {
				http.Error(w, "berechtigungen: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}

		save, err := stammdatenAusFormular(r.Form, apartments, kostenpositionen, gespeichert)
		if err == nil {
			err = store.SaveStammdaten(db, save)
		}
		if err != nil {
			status, text := stammdatenFehler(err, r.Form.Get("heizung_gewichtung"))
			http.Error(w, text, status)
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
