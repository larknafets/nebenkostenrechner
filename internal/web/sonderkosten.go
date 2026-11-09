package web

import (
	"errors"
	"net/http"
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
	// Rechnungsdatum is "YYYY-MM-DD" or empty, the template formats it.
	Rechnungsdatum string
	Betrag         float64
	Umlegbar       bool
	// Zuordnung says who carries the bill (GLOSSARY.md, Sonderkosten).
	Zuordnung string
	// Maske is the prefilled form for Korrigieren.
	Maske sonderkostenMaske
	// LoeschenZusatz is the extra line of the delete overlay, only for umlegbar entries.
	LoeschenZusatz string
	// NichtBerechenbar: "Je Anzahl Personen" in a month without a
	// Fixkosteneingabe, the entry counts nowhere until one exists.
	NichtBerechenbar bool
}

// sonderkostenMaske is the data of the shared form fields ("sk-felder"):
// P prefixes the element ids, the rest prefills the fields.
type sonderkostenMaske struct {
	P                 string
	ID                int64
	Monat, Betrag     string
	S                 store.Sonderkosten
	Arten, Objekte    []store.Auswahl
	Logiken, Hinweise []store.Auswahl
}

func neueMaske(p string, s store.Sonderkosten, monat, betrag string) sonderkostenMaske {
	return sonderkostenMaske{P: p, ID: s.ID, Monat: monat, Betrag: betrag, S: s, Arten: store.SonderkostenArten,
		Objekte: store.SonderkostenObjekte, Logiken: sonderkostenLogiken, Hinweise: sonderkostenHinweise}
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
	case s.Objekt == store.ObjektWohnung1:
		return "trägt Wohnung 1"
	case s.Objekt == store.ObjektWohnung2:
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
// sends its hidden select), then store.PruefeSonderkosten decides. On an
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
		Rechnungsdatum: strings.TrimSpace(f.Get("rechnungsdatum")),
		Objekt:         f.Get("objekt"), Umlegbar: f.Get("umlegbar") != "", Logik: f.Get("logik"),
	}
	// An unparsable month or amount stays empty/0, the validation reports it.
	s.Monat, _ = parseFixkostenMonat(f.Get("monat"))
	s.Betrag, _ = parseDecimalDE(f.Get("betrag"))
	if s.Objekt != store.ObjektHaus || !s.Umlegbar {
		s.Logik = ""
	}
	fehler, err := store.PruefeSonderkosten(db, s)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return s, false
	}
	if len(fehler) > 0 {
		http.Error(w, strings.Join(fehler, "\n"), http.StatusBadRequest)
		return s, false
	}
	return s, true
}

// sonderkostenFehlerAntwort answers a *SonderkostenFehler of a write with 400
// (the entry became invalid after the pre-check, a race) and reports whether
// it did.
func sonderkostenFehlerAntwort(w http.ResponseWriter, err error) bool {
	var fehler *store.SonderkostenFehler
	if !errors.As(err, &fehler) {
		return false
	}
	http.Error(w, strings.Join(fehler.Meldungen, "\n"), http.StatusBadRequest)
	return true
}

// handleSonderkostenSpeichern creates one entry from the form.
func handleSonderkostenSpeichern() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := sonderkostenAusForm(w, r)
		if !ok {
			return
		}
		if _, err := store.CreateSonderkosten(dbFromContext(r.Context()), s); err != nil {
			if sonderkostenFehlerAntwort(w, err) {
				return
			}
			http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Show the year of the new entry, it may not be the current one.
		jahr, _ := store.Abrechnungsmonat(s.Monat).Jahr()
		http.Redirect(w, r, requestBase(r)+"/sonderkosten?jahr="+strconv.Itoa(jahr), http.StatusFound)
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
			if sonderkostenFehlerAntwort(w, err) {
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

// handleSonderkostenListe shows the Sonderkosten of one year (?jahr=, the
// current year by default, "alle" for every year), grouped by
// Abrechnungsmonat. ?jahr=, ?art= and ?objekt= filter the list and the three sums alike.
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

		heute := time.Now()
		jahre := sonderkostenJahre(alle, heute.Year())
		filter := sonderkostenFilterAus(r.URL.Query().Get("jahr"), r.URL.Query().Get("art"), r.URL.Query().Get("objekt"), jahre, heute.Year())
		liste := baueSonderkostenListe(alle, fixMonate, filter)

		data := struct {
			navData
			Aktuell string
			sonderkostenListe
			Jahr         int
			AlleJahre    bool  // the Jahr filter is "Alle Jahre"
			Jahre        []int // years for the Jahr filter, newest first
			Arten        []store.Auswahl
			Art          string
			Objekte      []store.Auswahl
			Objekt       string
			HatEintraege bool
			NeuMonat     string
			NeuMaske     sonderkostenMaske
			FixMonate    []string // months ("YYYY-MM") with a Fixkosteneingabe, for the Personen option
		}{
			navData: z.NavData(r), Aktuell: "sonderkosten", sonderkostenListe: liste,
			Jahr: filter.Jahr, AlleJahre: filter.AlleJahre, Jahre: jahre,
			Arten: store.SonderkostenArten, Art: filter.Art, Objekte: store.SonderkostenObjekte, Objekt: filter.Objekt, HatEintraege: len(alle) > 0,
			NeuMonat: heute.Format("2006-01"), FixMonate: fixMonate,
			NeuMaske: neueMaske("sk", store.Sonderkosten{Art: store.ArtReparatur, Objekt: store.ObjektHaus}, heute.Format("2006-01"), ""),
		}
		if err := sonderkostenTemplate.ExecuteTemplate(w, "layout", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

// sonderkostenLoeschenText is the extra line when a Fixkosteneingabe is
// deleted: n Sonderkosten of monat with "Je Anzahl Personen" need it and count
// nowhere without it. "" for n = 0. The count is fixkostenWirkung.Sonderkosten.
func sonderkostenLoeschenText(n int, monat string) string {
	switch n {
	case 0:
		return ""
	case 1:
		return "Dann zählt 1 Sonderkosten-Eintrag mit \"Je Anzahl Personen\" für " + store.MonatLabel(monat) + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."
	}
	return "Dann zählen " + strconv.Itoa(n) + " Sonderkosten-Einträge mit \"Je Anzahl Personen\" für " + store.MonatLabel(monat) + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."
}
