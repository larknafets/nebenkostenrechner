// Package web serves the Nebenkostenrechner's HTML wizard and
// read views. Server-rendered html/template, no SPA build step - see
// https://github.com/larknafets/nebenkostenrechner/issues/4.
package web

import (
	"database/sql"
	"embed"
	"html/template"
	"net/http"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

// Each page gets its own *template.Template (layout + exactly one page
// template) - all pages define a "content" block with the same name, so
// parsing them together into one shared template set would let the last
// one silently overwrite the others.
var (
	wizardTemplate     = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/wizard.html"))
	ablesungTemplate   = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/ablesung.html"))
	ablesungenTemplate = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/ablesungen.html"))
	dashboardTemplate  = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/monatsverlauf.html", "templates/dashboard.html"))

	keineBerechtigungTemplate = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/keine_berechtigung.html"))
	berechnungslogikTemplate  = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/berechnungslogik.html"))
	stammdatenTemplate        = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/stammdaten.html"))
	abrechnungTemplate        = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/abrechnung.html"))

	fixkostenListeTemplate  = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/fixkosten_hinweise.html", "templates/fixkosten.html"))
	fixkostenFormTemplate   = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/fixkosten_form.html"))
	fixkostenDetailTemplate = template.Must(template.New("layout.html").Funcs(templateFuncs).ParseFS(templateFS, "templates/layout.html", "templates/fixkosten_hinweise.html", "templates/fixkosten_detail.html"))
)

// kategorieIconPaths maps a yearly-totals card category row's exact Label
// to its MDI icon path data - shown before the value, colored via the
// surrounding .v-<Farbe> span (fill:currentColor). Keyed by Label rather
// than Farbe since 2 Labels share the "pv" Farbe (PV-Anteil/
// Einspeisevergütung) but want visually distinct icons.
var kategorieIconPaths = map[string]string{
	"Fixkosten":          "M3,22L4.5,20.5L6,22L7.5,20.5L9,22L10.5,20.5L12,22L13.5,20.5L15,22L16.5,20.5L18,22L19.5,20.5L21,22V2L19.5,3.5L18,2L16.5,3.5L15,2L13.5,3.5L12,2L10.5,3.5L9,2L7.5,3.5L6,2L4.5,3.5L3,2M18,9H6V7H18M18,13H6V11H18M18,17H6V15H18V17Z",
	"Strom":              "M11 15H6L13 1V9H18L11 23V15Z",
	"Stromkosten":        "M11 15H6L13 1V9H18L11 23V15Z",
	"Heizung/Ww":         "M17.66 11.2C17.43 10.9 17.15 10.64 16.89 10.38C16.22 9.78 15.46 9.35 14.82 8.72C13.33 7.26 13 4.85 13.95 3C13 3.23 12.17 3.75 11.46 4.32C8.87 6.4 7.85 10.07 9.07 13.22C9.11 13.32 9.15 13.42 9.15 13.55C9.15 13.77 9 13.97 8.8 14.05C8.57 14.15 8.33 14.09 8.14 13.93C8.08 13.88 8.04 13.83 8 13.76C6.87 12.33 6.69 10.28 7.45 8.64C5.78 10 4.87 12.3 5 14.47C5.06 14.97 5.12 15.47 5.29 15.97C5.43 16.57 5.7 17.17 6 17.7C7.08 19.43 8.95 20.67 10.96 20.92C13.1 21.19 15.39 20.8 17.03 19.32C18.86 17.66 19.5 15 18.56 12.72L18.43 12.46C18.22 12 17.66 11.2 17.66 11.2M14.5 17.5C14.22 17.74 13.76 18 13.4 18.1C12.28 18.5 11.16 17.94 10.5 17.28C11.69 17 12.4 16.12 12.61 15.23C12.78 14.43 12.46 13.77 12.33 13C12.21 12.26 12.23 11.63 12.5 10.94C12.69 11.32 12.89 11.7 13.13 12C13.9 13 15.11 13.44 15.37 14.8C15.41 14.94 15.43 15.08 15.43 15.23C15.46 16.05 15.1 16.95 14.5 17.5H14.5Z",
	"Wallbox":            "M18,10A1,1 0 0,1 17,9A1,1 0 0,1 18,8A1,1 0 0,1 19,9A1,1 0 0,1 18,10M12,10H6V5H12M19.77,7.23L19.78,7.22L16.06,3.5L15,4.56L17.11,6.67C16.17,7.03 15.5,7.93 15.5,9A2.5,2.5 0 0,0 18,11.5C18.36,11.5 18.69,11.42 19,11.29V18.5A1,1 0 0,1 18,19.5A1,1 0 0,1 17,18.5V14A2,2 0 0,0 15,12H14V5A2,2 0 0,0 12,3H6A2,2 0 0,0 4,5V21H14V13.5H15.5V18.5A2.5,2.5 0 0,0 18,21A2.5,2.5 0 0,0 20.5,18.5V9C20.5,8.31 20.22,7.68 19.77,7.23Z",
	"PV-Gutschrift":      "M11.39 5.45L9.61 4.55L10.87 2H19.34L20.61 4.55L18.83 5.44L18.11 4H12.11L11.39 5.45M21.73 8H17.2L16.41 5H13.81L13 8H8.5L7.21 10.55L9 11.44L9.73 10H20.5L21.21 11.45L23 10.56L21.73 8M20.88 22H18.81L18.57 21.1L15.11 15.9L11.64 21.1L11.41 22H9.34L12.23 11H14.3L13.94 12.35L15.11 14.1L16.27 12.35L15.92 11H18L20.88 22M14.5 15L13.61 13.65L12.43 18.13L14.5 15M17.79 18.12L16.61 13.64L15.71 15L17.79 18.12M9 16L5 12V15H1V17H5V20L9 16Z",
	"Wasser":             "M12,20A6,6 0 0,1 6,14C6,10 12,3.25 12,3.25C12,3.25 18,10 18,14A6,6 0 0,1 12,20Z",
	"PV-Anteil":          "M11.45,2V5.55L15,3.77L11.45,2M10.45,8L8,10.46L11.75,11.71L10.45,8M2,11.45L3.77,15L5.55,11.45H2M10,2H2V10C2.57,10.17 3.17,10.25 3.77,10.25C7.35,10.26 10.26,7.35 10.27,3.75C10.26,3.16 10.17,2.57 10,2M17,22V16H14L19,7V13H22L17,22Z",
	"Einspeisevergütung": "M11.39 5.45L9.61 4.55L10.87 2H19.34L20.61 4.55L18.83 5.44L18.11 4H12.11L11.39 5.45M21.73 8H17.2L16.41 5H13.81L13 8H8.5L7.21 10.55L9 11.44L9.73 10H20.5L21.21 11.45L23 10.56L21.73 8M20.88 22H18.81L18.57 21.1L15.11 15.9L11.64 21.1L11.41 22H9.34L12.23 11H14.3L13.94 12.35L15.11 14.1L16.27 12.35L15.92 11H18L20.88 22M14.5 15L13.61 13.65L12.43 18.13L14.5 15M17.79 18.12L16.61 13.64L15.71 15L17.79 18.12M9 16L5 12V15H1V17H5V20L9 16Z",
}

// kategorieIcon renders the given category row Label's MDI icon as
// inline SVG (fill:currentColor, so it takes on the surrounding
// .v-<Farbe> span's text color) - empty for an unmapped Label.
func kategorieIcon(label string) template.HTML {
	path, ok := kategorieIconPaths[label]
	if !ok {
		return ""
	}
	return template.HTML(`<svg viewBox="0 0 24 24" width="13" height="13" style="vertical-align:-2px;margin-right:3px" fill="currentColor"><path d="` + path + `"/></svg>`)
}

var templateFuncs = template.FuncMap{
	"updateURL":     updateURL,
	"de":            formatDecimalDE,
	"de0":           formatDecimalDE0,
	"de1":           formatDecimalDE1,
	"de2":           formatDecimalDE2,
	"de3":           formatDecimalDE3,
	"deEUR":         formatEuroDE,
	"dePreis":       formatPreisDE,
	"deDatum":       formatDatumDE,
	"monat":         store.MonatLabel,
	"monatKurz":     germanPeriodLabelShort,
	"personen":      personenZelle,
	"mieterSeit":    func(stored string) monatInput { return monatInputFromStored(stored) },
	"anlage":        anlageUeberschrift,
	"anlagen":       anlagenListe,
	"deDatumZeit":   formatDatumZeitDE,
	"kategorieIcon": kategorieIcon,
	// hasReading (Ticket #131): whether the wizard may prefill a meter value
	// from the completing EditReadings - "index" alone silently returns the
	// zero value for a missing key, which couldn't distinguish "not
	// recorded" from an actual 0.
	"inputNumber":      inputNumber,
	"inc":              func(i int) int { return i + 1 },
	"lueckenWarnungJS": lueckenWarnungScript,
	"hasReading":       func(m map[string]float64, k string) bool { _, ok := m[k]; return ok },
	// logikOptions is a zero-arg func rather than a per-page data field -
	// it's a fixed, package-global list (see fixkosten.go), so the
	// shared "monatsverlauf-wohnung-body" partial (monatsverlauf.html)
	// needs no own data transport from every caller.
	"logikOptions": func() []logikOption { return logikOptions },
}

// NewMux builds the server's mux from the table routen: every row is
// registered here, with withDB (picks db or demoDB per request, Ticket
// #119/#120) around the Gate of its Anspruch. This is the only place that
// registers a route of this package (TestRegistrierungNurInNewMux).
func NewMux(db, demoDB *sql.DB, version, buildDate string) *http.ServeMux {
	vermieter, mieter := ladeKennwoerter()
	a := auth{vermieter: vermieter, mieter: mieter, demoDB: demoDB, limit: newLoginLimit(time.Now)}
	return muxFuer(zugangFuer(a), db, demoDB, version, buildDate)
}

// muxFuer is NewMux for a given Zugang, so tests can fix the role.
func muxFuer(z *Zugang, db, demoDB *sql.DB, version, buildDate string) *http.ServeMux {
	mux := http.NewServeMux()
	for _, r := range routen(z, version, buildDate) {
		mux.HandleFunc(r.Methode+" "+r.Pfad, withDB(db, demoDB, z.Gate(r.Anspruch, r.Handler)))
	}
	return mux
}

// ingressBase turns Home Assistant's X-Ingress-Path header (the path
// prefix its Supervisor proxy strips before forwarding the request here -
// see Ticket #22) into a base to prepend onto every generated link, form
// action, and redirect, so they still resolve through the proxy. Empty for
// direct access (no header) - trailing slash trimmed so callers can always
// just concatenate "<base>/some/path" without a double slash.
func ingressBase(header string) string {
	return strings.TrimSuffix(header, "/")
}

func requestBase(r *http.Request) string {
	return ingressBase(r.Header.Get("X-Ingress-Path"))
}

func handleIndex() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, requestBase(r)+"/dashboard", http.StatusFound)
	}
}

// handleBerechnungslogik serves a static, informational explanation of the
// cost formulas (Ticket #33) - no DB access, the content never depends on
// any period's data. Not Ablesungen- or Fixkosten-specific (Berechnungslogik
// as a concept spans both, see GLOSSARY.md), so it stays here rather than in
// either concept's own file.
func handleBerechnungslogik(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			navData
			Aktuell string
		}{navData: z.NavData(r), Aktuell: "berechnungslogik"}
		if err := berechnungslogikTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
