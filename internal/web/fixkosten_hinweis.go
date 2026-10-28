package web

import (
	"database/sql"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_hinweis.go: the notes after a Fixkosten-Eingabe was deleted or its
// Monat changed (#359, Entscheidung #355). The app has no flash mechanism, so
// the redirect carries the Monat and the ids of the affected Ablesungen as
// query parameters and the target page words the note from them.

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

// eingabeIDFuerMonat is the id of the Eingabe of monat, 0 = there is none.
func eingabeIDFuerMonat(eingaben []*store.FixkostenEingabeDetails, monat string) int64 {
	for _, e := range eingaben {
		if e.Monat == monat {
			return e.ID
		}
	}
	return 0
}

// Query parameters of the redirect targets.
const (
	paramGeloescht  = "geloescht"  // Monat of a deleted Eingabe
	paramAlterMonat = "alt"        // Monat an Eingabe moved away from
	paramAblesungen = "ablesungen" // comma separated ids of the affected Ablesungen
)

// datumListe renders the Ablesedaten as "a", "a und b" or "a, b und c".
func datumListe(ps []store.PeriodSummary) string {
	daten := make([]string, len(ps))
	for i, p := range ps {
		daten[i] = formatDatumDE(p.ReadingDate)
	}
	if len(daten) <= 1 {
		return strings.Join(daten, "")
	}
	return strings.Join(daten[:len(daten)-1], ", ") + " und " + daten[len(daten)-1]
}

// ablesungenVom is "Ablesung vom <Datum>" / "Ablesungen vom <Daten>" together
// with the matching form of the verb ("ist" or "sind").
func ablesungenVom(ps []store.PeriodSummary) (subjekt, verb string) {
	if len(ps) == 1 {
		return "Ablesung vom " + datumListe(ps), "ist"
	}
	return "Ablesungen vom " + datumListe(ps), "sind"
}

// fixkostenFehlenText is the one wording of the note that an Eingabe is
// missing (also used for the banner after a deletion).
func fixkostenFehlenText(monat string, ps []store.PeriodSummary) string {
	s, v := ablesungenVom(ps)
	return "Für " + store.MonatLabel(monat) + " fehlt die Fixkosten-Eingabe. Die " + s + " " + v + " unvollständig, bis sie erfasst ist."
}

// fixkostenLoeschenFrage is the additional confirmation before deleting an
// Eingabe that affects Ablesungen, "" when it affects none.
func fixkostenLoeschenFrage(monat string, ps []store.PeriodSummary) string {
	if len(ps) == 0 {
		return ""
	}
	s, v := ablesungenVom(ps)
	return "Dann " + v + " die " + s + " unvollständig, bis die Fixkosten-Eingabe für " + store.MonatLabel(monat) + " neu erfasst ist."
}

// fixkostenBerechenbarText is the information for the new Monat of a moved
// Eingabe: its Ablesungen can be calculated now.
func fixkostenBerechenbarText(monat string, ps []store.PeriodSummary) string {
	s, v := ablesungenVom(ps)
	return "Für " + store.MonatLabel(monat) + " ist die Fixkosten-Eingabe erfasst. Die " + s + " " + v + " damit berechenbar."
}

// ablesungenQuery encodes the Monat and the affected Ablesungen for a redirect.
func ablesungenQuery(param, monat string, ps []store.PeriodSummary) string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = strconv.FormatInt(p.ID, 10)
	}
	q := url.Values{param: {monat}}
	if len(ids) > 0 {
		q.Set(paramAblesungen, strings.Join(ids, ","))
	}
	return "?" + q.Encode()
}

// ablesungenAusQuery resolves the ids of the query against the stored
// Ablesungen of monat, oldest first. Unknown ids and other months are dropped,
// so a hand-written URL cannot show anything the data does not back.
func ablesungenAusQuery(db *sql.DB, q url.Values, monat string) ([]store.PeriodSummary, error) {
	gewollt := map[int64]bool{}
	for _, s := range strings.Split(q.Get(paramAblesungen), ",") {
		if id, err := strconv.ParseInt(s, 10, 64); err == nil {
			gewollt[id] = true
		}
	}
	alle, err := store.AllPeriods(db) // newest first
	if err != nil {
		return nil, err
	}
	var out []store.PeriodSummary
	for i := len(alle) - 1; i >= 0; i-- {
		if p := alle[i]; gewollt[p.ID] && p.Monat == monat {
			out = append(out, p)
		}
	}
	return out, nil
}

// gueltigerMonat reports whether s is a stored Monat ("YYYY-MM-01").
func gueltigerMonat(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Day() == 1
}

// eingabeFuerMonat is the id of the Eingabe of monat, 0 = there is none.
func eingabeFuerMonat(db *sql.DB, monat string) (int64, error) {
	alle, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return 0, err
	}
	for _, e := range alle {
		if e.Monat == monat {
			return e.ID, nil
		}
	}
	return 0, nil
}

// fehlenHinweisAusQuery is the warning for the Monat named by param (geloescht
// or alt), or nil: no such parameter, no Ablesung left to name, or the Monat
// got an Eingabe in the meantime.
func fehlenHinweisAusQuery(db *sql.DB, q url.Values, param string) (*fixkostenHinweis, error) {
	monat := q.Get(param)
	if !gueltigerMonat(monat) {
		return nil, nil
	}
	id, err := eingabeFuerMonat(db, monat)
	if err != nil || id != 0 {
		return nil, err
	}
	ps, err := ablesungenAusQuery(db, q, monat)
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	return &fixkostenHinweis{Text: fixkostenFehlenText(monat, ps), Link: true, Pfad: fixkostenErfassenPfad(0)}, nil
}

// berechenbarHinweis is the information for the new Monat of a moved Eingabe,
// or nil when it has no Ablesung that was waiting for it.
func berechenbarHinweis(db *sql.DB, monat string) (*fixkostenHinweis, error) {
	ps, err := store.BetroffeneAblesungen(db, monat)
	if err != nil || len(ps) == 0 {
		return nil, err
	}
	return &fixkostenHinweis{Text: fixkostenBerechenbarText(monat, ps), Info: true}, nil
}

// fixkostenHinweiseNachAenderung are the notes on the detail page after a move
// of the Eingabe to another Monat: the warning for the old Monat and the
// information for the new one (the query carries "alt" only after a move).
func fixkostenHinweiseNachAenderung(db *sql.DB, q url.Values, neuerMonat string) ([]fixkostenHinweis, error) {
	if !gueltigerMonat(q.Get(paramAlterMonat)) {
		return nil, nil
	}
	var out []fixkostenHinweis
	if h, err := fehlenHinweisAusQuery(db, q, paramAlterMonat); err != nil {
		return nil, err
	} else if h != nil {
		out = append(out, *h)
	}
	if h, err := berechenbarHinweis(db, neuerMonat); err != nil {
		return nil, err
	} else if h != nil {
		out = append(out, *h)
	}
	return out, nil
}
