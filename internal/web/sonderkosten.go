package web

import (
	"net/http"
	"slices"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenKarte is one entry of the list, ready for the template.
type sonderkostenKarte struct {
	Betreff, Beschreibung, Art, Firma, Rechnungsnummer string
	Betrag                                             float64
	Umlegbar                                           bool
	// Zuordnung says who carries the bill (GLOSSARY.md, Sonderkosten).
	Zuordnung string
}

// sonderkostenMonat is the group of one Abrechnungsmonat.
type sonderkostenMonat struct {
	Label    string
	Umlegbar float64 // sum of the umlegbar entries shown in the group
	Karten   []sonderkostenKarte
}

// sonderkostenZuordnung is the muted "who carries it" text of a card: the
// Berechnungslogik for Haus plus umlegbar, the Wohnung when it carries the
// bill, otherwise the Vermieter.
func sonderkostenZuordnung(s store.Sonderkosten) string {
	switch {
	case !s.Umlegbar:
		return "trägt Vermieter:in"
	case s.Umsetzung == store.UmsetzungWohnung1:
		return "trägt Wohnung 1"
	case s.Umsetzung == store.UmsetzungWohnung2:
		return "trägt Wohnung 2"
	}
	return calc.LogikLabels[s.Logik]
}

// handleSonderkostenListe shows the Sonderkosten of one year (?jahr=, newest
// by default), grouped by Abrechnungsmonat. ?art= filters the list only, the
// three sums always cover the whole year.
func handleSonderkostenListe(z *Zugang) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		alle, err := store.AllSonderkosten(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "sonderkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}

		var jahre []int
		for _, s := range alle {
			if y, ok := store.Abrechnungsmonat(s.Monat).Jahr(); ok && !slices.Contains(jahre, y) {
				jahre = append(jahre, y)
			}
		}
		slices.Sort(jahre)
		jahr := time.Now().Year()
		if len(jahre) > 0 {
			jahr = jahre[len(jahre)-1]
		}
		if wunsch, err := strconv.Atoi(r.URL.Query().Get("jahr")); err == nil && slices.Contains(jahre, wunsch) {
			jahr = wunsch
		}
		art := r.URL.Query().Get("art")
		if !slices.ContainsFunc(store.SonderkostenArten, func(a store.Auswahl) bool { return a.Wert == art }) {
			art = ""
		}
		artLabel := map[string]string{}
		for _, a := range store.SonderkostenArten {
			artLabel[a.Wert] = a.Label
		}

		var gesamt, umlegbar float64
		var monate []sonderkostenMonat
		for _, s := range alle {
			if y, _ := store.Abrechnungsmonat(s.Monat).Jahr(); y != jahr {
				continue
			}
			gesamt += s.Betrag
			if s.Umlegbar {
				umlegbar += s.Betrag
			}
			if art != "" && s.Art != art {
				continue
			}
			if n := len(monate); n == 0 || monate[n-1].Label != store.MonatLabel(s.Monat) {
				monate = append(monate, sonderkostenMonat{Label: store.MonatLabel(s.Monat)})
			}
			m := &monate[len(monate)-1]
			if s.Umlegbar {
				m.Umlegbar += s.Betrag
			}
			m.Karten = append(m.Karten, sonderkostenKarte{
				Betreff: s.Betreff, Beschreibung: s.Beschreibung, Art: artLabel[s.Art], Firma: s.Firma,
				Rechnungsnummer: s.Rechnungsnummer, Betrag: s.Betrag, Umlegbar: s.Umlegbar, Zuordnung: sonderkostenZuordnung(s),
			})
		}
		// Newest month first, like the Dashboard.
		slices.Reverse(monate)

		var zurueck, vor string
		if i := slices.Index(jahre, jahr); i > 0 {
			zurueck = "?jahr=" + strconv.Itoa(jahre[i-1])
		}
		if i := slices.Index(jahre, jahr); i >= 0 && i < len(jahre)-1 {
			vor = "?jahr=" + strconv.Itoa(jahre[i+1])
		}

		data := struct {
			navData
			Aktuell         string
			Jahr            int
			JahrZurueck     string
			JahrVor         string
			Gesamt          float64
			Umlegbar        float64
			VermieterTraegt float64
			Arten           []store.Auswahl
			Art             string
			Monate          []sonderkostenMonat
			HatEintraege    bool
		}{
			navData: z.NavData(r), Aktuell: "sonderkosten", Jahr: jahr, JahrZurueck: zurueck, JahrVor: vor,
			Gesamt: gesamt, Umlegbar: umlegbar, VermieterTraegt: gesamt - umlegbar,
			Arten: store.SonderkostenArten, Art: art, Monate: monate, HatEintraege: len(alle) > 0,
		}
		if err := sonderkostenTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
