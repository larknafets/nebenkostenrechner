package web

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenKarte is one entry of the list, ready for the template.
type sonderkostenKarte struct {
	ID                                                 int64
	Betreff, Beschreibung, Art, Firma, Rechnungsnummer string
	Betrag                                             float64
	Umlegbar                                           bool
	// Zuordnung says who carries the bill (GLOSSARY.md, Sonderkosten).
	Zuordnung string
	// Maske is the prefilled form for Korrigieren.
	Maske sonderkostenMaske
	// LoeschenZusatz is the extra line of the delete overlay, only for umlegbar entries.
	LoeschenZusatz string
}

// sonderkostenMaske is the data of the shared form fields ("sk-felder"):
// P prefixes the element ids, the rest prefills the fields.
type sonderkostenMaske struct {
	P                  string
	ID                 int64
	Monat, Betrag      string
	S                  store.Sonderkosten
	Arten, Umsetzungen []store.Auswahl
	Logiken, Hinweise  []store.Auswahl
}

func neueMaske(p string, s store.Sonderkosten, monat, betrag string) sonderkostenMaske {
	return sonderkostenMaske{P: p, ID: s.ID, Monat: monat, Betrag: betrag, S: s, Arten: store.SonderkostenArten,
		Umsetzungen: store.SonderkostenUmsetzungen, Logiken: sonderkostenLogiken, Hinweise: sonderkostenHinweise}
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

// sonderkostenLogiken are the Berechnungslogiken of a Haus entry, in display order.
var sonderkostenLogiken = []store.Auswahl{
	{Wert: store.LogikWohneinheit, Label: calc.LogikLabels[store.LogikWohneinheit]},
	{Wert: store.LogikFlurstueck, Label: calc.LogikLabels[store.LogikFlurstueck]},
	{Wert: store.LogikQM, Label: calc.LogikLabels[store.LogikQM]},
	{Wert: store.LogikPersonen, Label: calc.LogikLabels[store.LogikPersonen]},
}

// sonderkostenHinweise is the notice under the Umlegbar checkbox of the form,
// by Art (Wert). Wartung has none. Only a notice, never a block.
var sonderkostenHinweise = []store.Auswahl{
	{Wert: store.ArtReparatur, Label: hinweisPraefix("Reparaturen")},
	{Wert: store.ArtNeuanschaffung, Label: hinweisPraefix("Neuanschaffungen")},
	{Wert: store.ArtModernisierung, Label: hinweisPraefix("Modernisierungen")},
	{Wert: store.ArtSonstiges, Label: "Hinweis: Nicht jede sonstige Rechnung ist umlegbar. Bitte vor dem Umlegen prüfen."},
}

func hinweisPraefix(was string) string {
	return "Hinweis: " + was + " sind in der Regel keine umlegbaren Betriebskosten. Bitte vor dem Umlegen prüfen."
}

// sonderkostenAusForm reads and validates the entry form, for Erfassen and
// Korrigieren alike. The Logik is dropped unless it applies (the form always
// sends its hidden select), then store.ValidiereSonderkosten decides. On an
// error it has answered already and returns false.
func sonderkostenAusForm(w http.ResponseWriter, r *http.Request) (store.Sonderkosten, bool) {
	db := dbFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form: "+err.Error(), http.StatusBadRequest)
		return store.Sonderkosten{}, false
	}
	f := r.PostForm
	s := store.Sonderkosten{
		Betreff: strings.TrimSpace(f.Get("betreff")), Beschreibung: strings.TrimSpace(f.Get("beschreibung")),
		Art: f.Get("art"), Firma: strings.TrimSpace(f.Get("firma")), Rechnungsnummer: strings.TrimSpace(f.Get("rechnungsnummer")),
		Umsetzung: f.Get("umsetzung"), Umlegbar: f.Get("umlegbar") != "", Logik: f.Get("logik"),
	}
	// An unparsable month or amount stays empty/0, the validation reports it.
	s.Monat, _ = parseFixkostenMonat(f.Get("monat"))
	s.Betrag, _ = parseDecimalDE(f.Get("betrag"))
	if s.Umsetzung != store.UmsetzungHaus || !s.Umlegbar {
		s.Logik = ""
	}
	hatEingabe, err := store.MonatHatFixkosteneingabe(db, s.Monat)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return s, false
	}
	if fehler := store.ValidiereSonderkosten(s, hatEingabe); len(fehler) > 0 {
		http.Error(w, strings.Join(fehler, "\n"), http.StatusBadRequest)
		return s, false
	}
	return s, true
}

// handleSonderkostenSpeichern creates one entry from the form.
func handleSonderkostenSpeichern() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := sonderkostenAusForm(w, r)
		if !ok {
			return
		}
		if _, err := store.CreateSonderkosten(dbFromContext(r.Context()), s); err != nil {
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, requestBase(r)+"/sonderkosten", http.StatusFound)
	}
}

// handleSonderkostenKorrigieren replaces the entry {id} by the form, with the
// same validation as Erfassen. No lock for a settled year.
func handleSonderkostenKorrigieren() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid sonderkosten id", http.StatusBadRequest)
			return
		}
		s, ok := sonderkostenAusForm(w, r)
		if !ok {
			return
		}
		s.ID = id
		if err := store.UpdateSonderkosten(dbFromContext(r.Context()), s); err != nil {
			if errors.Is(err, store.ErrSonderkostenNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Back to the year of the (possibly moved) entry.
		jahr, _ := store.Abrechnungsmonat(s.Monat).Jahr()
		http.Redirect(w, r, requestBase(r)+"/sonderkosten?jahr="+strconv.Itoa(jahr), http.StatusFound)
	}
}

// handleSonderkostenLoeschen deletes the entry {id}.
func handleSonderkostenLoeschen() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "invalid sonderkosten id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteSonderkosten(dbFromContext(r.Context()), id); err != nil {
			if errors.Is(err, store.ErrSonderkostenNotFound) {
				http.NotFound(w, r)
				return
			}
			http.Error(w, "delete: "+err.Error(), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, requestBase(r)+"/sonderkosten", http.StatusFound)
	}
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

		eingaben, err := store.AllFixkostenEingaben(dbFromContext(r.Context()))
		if err != nil {
			http.Error(w, "sonderkosten: "+err.Error(), http.StatusInternalServerError)
			return
		}
		fixMonate := make([]string, 0, len(eingaben))
		for _, e := range eingaben {
			fixMonate = append(fixMonate, e.Monat[:7])
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
				Betreff: s.Betreff, Beschreibung: s.Beschreibung, Art: sonderkostenArtLabel(s.Art), Firma: s.Firma,
				Rechnungsnummer: s.Rechnungsnummer, Betrag: s.Betrag, Umlegbar: s.Umlegbar, Zuordnung: sonderkostenZuordnung(s),
				ID: s.ID, Maske: neueMaske("sk-"+strconv.FormatInt(s.ID, 10), s, s.Monat[:7], formatDecimalDE2(s.Betrag)),
			})
			if s.Umlegbar {
				m.Karten[len(m.Karten)-1].LoeschenZusatz = "Saldo und Abrechnung ändern sich."
			}
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
			NeuMonat        string
			NeuMaske        sonderkostenMaske
			FixMonate       []string // months ("YYYY-MM") with a Fixkosteneingabe, for the Personen option
		}{
			navData: z.NavData(r), Aktuell: "sonderkosten", Jahr: jahr, JahrZurueck: zurueck, JahrVor: vor,
			Gesamt: gesamt, Umlegbar: umlegbar, VermieterTraegt: gesamt - umlegbar,
			Arten: store.SonderkostenArten, Art: art, Monate: monate, HatEintraege: len(alle) > 0,
			NeuMonat: time.Now().Format("2006-01"), FixMonate: fixMonate,
			NeuMaske: neueMaske("sk", store.Sonderkosten{Art: store.ArtReparatur, Umsetzung: store.UmsetzungHaus}, time.Now().Format("2006-01"), ""),
		}
		if err := sonderkostenTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}
