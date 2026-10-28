package web

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_hinweis.go: the notes after a Fixkosten-Eingabe was deleted or its
// Monat changed (#359, Entscheidung #355). The app has no flash mechanism, so
// the redirect carries only the Monat as a query parameter and the target page
// derives the affected Ablesungen and the note from the stored data (#382).

// fixkostenHinweis is one note above a Fixkosten page. Info marks the friendly
// variant (an Ablesung became berechenbar), Link the "Fixkosten erfassen"
// link, which only the Vermieter get (the template checks the role). Pfad is
// its target (fixkostenErfassenPfad).
type fixkostenHinweis struct {
	Text string
	Info bool
	Link bool
	Pfad string
}

// fixkostenErfassenPfad is where "Fixkosten erfassen" leads: the edit page of
// the Eingabe of the month if there is one (it lacks a price and gets
// completed), the form for a new one otherwise (eingabeID 0).
func fixkostenErfassenPfad(eingabeID int64) string {
	if eingabeID == 0 {
		return "/fixkosten/neu"
	}
	return "/fixkosten/" + strconv.FormatInt(eingabeID, 10) + "/bearbeiten"
}

// Query parameters of the redirect targets.
const (
	paramGeloescht  = "geloescht" // Monat of a deleted Eingabe
	paramAlterMonat = "alt"       // Monat an Eingabe moved away from
)

// datumListe renders the Ablesedaten as "a", "a und b" or "a, b und c".
func datumListe(ps []calc.Ablesung) string {
	daten := make([]string, len(ps))
	for i, p := range ps {
		daten[i] = formatDatumDE(p.Period.ReadingDate)
	}
	if len(daten) <= 1 {
		return strings.Join(daten, "")
	}
	return strings.Join(daten[:len(daten)-1], ", ") + " und " + daten[len(daten)-1]
}

// ablesungenVom is "Ablesung vom <Datum>" / "Ablesungen vom <Daten>" together
// with the matching form of the verb ("ist" or "sind").
func ablesungenVom(ps []calc.Ablesung) (subjekt, verb string) {
	if len(ps) == 1 {
		return "Ablesung vom " + datumListe(ps), "ist"
	}
	return "Ablesungen vom " + datumListe(ps), "sind"
}

// monateListe joins month labels in German: "A", "A und B", "A, B und C".
func monateListe(labels []string) string {
	if len(labels) < 2 {
		return strings.Join(labels, "")
	}
	return strings.Join(labels[:len(labels)-1], ", ") + " und " + labels[len(labels)-1]
}

// fixkostenFehlenSatz is the one sentence that says the Fixkosten of the given
// Monate ("YYYY-MM-01") are missing or incomplete. Dashboard banner, Ablesung
// page, banner after saving and the CSV import note all start with it and add
// their own consequence. Only the note after a delete or a move
// (fixkostenFehlenText) names the missing Eingabe itself, and the wizard keeps
// its short state (wallboxHinweisFehlt, Story 44).
func fixkostenFehlenSatz(monate []string) string {
	labels := make([]string, len(monate))
	for i, m := range monate {
		labels[i] = store.MonatLabel(m)
	}
	return "Für " + monateListe(labels) + " fehlen die Fixkosten (oder sind unvollständig)."
}

// fixkostenFehlenText is the one wording of the note that an Eingabe is
// missing (also used for the banner after a deletion).
func fixkostenFehlenText(monat string, ps []calc.Ablesung) string {
	s, v := ablesungenVom(ps)
	return "Für " + store.MonatLabel(monat) + " fehlt die Fixkosten-Eingabe. Die " + s + " " + v + " unvollständig, bis sie erfasst ist."
}

// fixkostenLoeschenFrage is the additional confirmation before deleting an
// Eingabe that affects Ablesungen, "" when it affects none.
func fixkostenLoeschenFrage(monat string, ps []calc.Ablesung) string {
	if len(ps) == 0 {
		return ""
	}
	s, v := ablesungenVom(ps)
	return "Dann " + v + " die " + s + " unvollständig, bis die Fixkosten-Eingabe für " + store.MonatLabel(monat) + " neu erfasst ist."
}

// fixkostenBerechenbarText is the information for the new Monat of a moved
// Eingabe: its Ablesungen can be calculated now.
func fixkostenBerechenbarText(monat string, ps []calc.Ablesung) string {
	s, v := ablesungenVom(ps)
	return "Für " + store.MonatLabel(monat) + " ist die Fixkosten-Eingabe erfasst. Die " + s + " " + v + " damit berechenbar."
}

// gueltigerMonat reports whether s is a stored Monat ("YYYY-MM-01").
func gueltigerMonat(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Day() == 1
}

// fehlenHinweisAusQuery is the warning for the Monat named by param (geloescht
// or alt), or nil: no such parameter, the Monat is complete, or there is no
// Ablesung to name. The link leads to the incomplete Eingabe of the Monat or
// to the form for a new one.
func fehlenHinweisAusQuery(v *calc.Verbrauchskosten, q url.Values, param string) *fixkostenHinweis {
	monat := q.Get(param)
	if !gueltigerMonat(monat) || v.MonatVollstaendig(monat) {
		return nil
	}
	ps := v.BetroffeneAblesungen(monat)
	if len(ps) == 0 {
		return nil
	}
	var eingabeID int64
	if e := v.Eingabe(monat); e != nil {
		eingabeID = e.ID
	}
	return &fixkostenHinweis{Text: fixkostenFehlenText(monat, ps), Link: true, Pfad: fixkostenErfassenPfad(eingabeID)}
}

// berechenbarHinweis is the information for the new Monat of a moved Eingabe,
// or nil when it is not complete or has no Ablesung that was waiting for it.
func berechenbarHinweis(v *calc.Verbrauchskosten, monat string) *fixkostenHinweis {
	ps := v.BetroffeneAblesungen(monat)
	if !v.MonatVollstaendig(monat) || len(ps) == 0 {
		return nil
	}
	return &fixkostenHinweis{Text: fixkostenBerechenbarText(monat, ps), Info: true}
}

// fixkostenHinweiseNachAenderung are the notes on the detail page after a move
// of the Eingabe to another Monat: the warning for the old Monat and the
// information for the new one (the query carries "alt" only after a move).
func fixkostenHinweiseNachAenderung(v *calc.Verbrauchskosten, q url.Values, neuerMonat string) []fixkostenHinweis {
	if !gueltigerMonat(q.Get(paramAlterMonat)) {
		return nil
	}
	var out []fixkostenHinweis
	if h := fehlenHinweisAusQuery(v, q, paramAlterMonat); h != nil {
		out = append(out, *h)
	}
	if h := berechenbarHinweis(v, neuerMonat); h != nil {
		out = append(out, *h)
	}
	return out
}
