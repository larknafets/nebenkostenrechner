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
// link, which only the Vermieter get (the template checks the role). Pfad and
// LinkText are its target and text (fixkostenFehlenHinweis).
type fixkostenHinweis struct {
	Text     string
	Info     bool
	Link     bool
	Pfad     string
	LinkText string
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

// fixkostenKontext is the page a note about missing Fixkosten appears on. It
// only decides the consequence sentence (fixkostenFolgen).
type fixkostenKontext int

const (
	kontextDashboard fixkostenKontext = iota
	kontextAblesung                   // detail page and banner after saving
	kontextCSVImport
	kontextLoeschen // after deleting or moving an Eingabe, names the Ablesungen
)

// fixkostenFolgen is the sentence after the generic one, per Kontext.
var fixkostenFolgen = map[fixkostenKontext]func(ablesungen []calc.Ablesung) string{
	kontextDashboard: func([]calc.Ablesung) string {
		return "Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht."
	},
	kontextAblesung: func([]calc.Ablesung) string {
		return "Die Ablesung ist noch nicht berechnet, bis die Fixkosten erfasst sind."
	},
	kontextCSVImport: func([]calc.Ablesung) string {
		return "Die Ablesungen dieser Monate sind noch nicht berechnet, bis die Fixkosten erfasst sind."
	},
	kontextLoeschen: func(ps []calc.Ablesung) string {
		s, v := ablesungenVom(ps)
		return "Die " + s + " " + v + " unvollständig, bis sie erfasst ist."
	},
}

// fixkostenFehlenHinweis is the one note that the Fixkosten of monate are
// missing or incomplete, nil without a Monat. The sentence is generic, the
// consequence follows the Kontext (ablesungen only names them after a delete).
// The link leads to the edit page of the one Monat with an Eingabe, to the new
// form if any Monat has none, else to the list; its text follows: "ergänzen"
// when every Monat has an Eingabe, else "erfassen". The wizard keeps its own
// short state (wallboxHinweisFehlt).
func fixkostenFehlenHinweis(kontext fixkostenKontext, monate []calc.OffenerMonat, ablesungen []calc.Ablesung) *fixkostenHinweis {
	if len(monate) == 0 {
		return nil
	}
	pfad, text := "/fixkosten", "Fixkosten ergänzen"
	if len(monate) == 1 {
		pfad = fixkostenErfassenPfad(monate[0].EingabeID)
	}
	for _, m := range monate {
		if m.EingabeID == 0 {
			pfad, text = fixkostenErfassenPfad(0), "Fixkosten erfassen"
			break
		}
	}
	return &fixkostenHinweis{
		Text: "Für " + monateText(monate) + " fehlen die Fixkosten (oder sind unvollständig). " + fixkostenFolgen[kontext](ablesungen),
		Link: true, Pfad: pfad, LinkText: text,
	}
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
	return fixkostenFehlenHinweis(kontextLoeschen, []calc.OffenerMonat{{Monat: monat, EingabeID: eingabeID}}, ps)
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
