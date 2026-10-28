package web

import (
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// offenerMonat is the projection of a month whose Ablesungen are not
// calculable because the Fixkosten-Eingabe is missing or lacks a price
// (#364). It is returned next to dashboardData, never inside it, like the
// Miete (the golden test dumps dashboardData by reflection).
type offenerMonat struct {
	Monat string
	// EingabeID is the incomplete Fixkosten-Eingabe of the month, 0 = there is
	// none.
	EingabeID int64
}

// offeneMonate lists the months with an Ablesung that waits for its
// Fixkosten, oldest first, each month once.
func offeneMonate(ds *store.Datenstand, kosten *calc.Verbrauchskosten) []offenerMonat {
	eingabeJeMonat := make(map[string]int64, len(ds.Eingaben))
	for _, e := range ds.Eingaben {
		eingabeJeMonat[e.Monat] = e.ID
	}
	var out []offenerMonat
	seen := map[string]bool{}
	for _, a := range kosten.FixkostenOffen() {
		if seen[a.Period.Monat] {
			continue
		}
		seen[a.Period.Monat] = true
		out = append(out, offenerMonat{Monat: a.Period.Monat, EingabeID: eingabeJeMonat[a.Period.Monat]})
	}
	return out
}

// monateText lists month labels in German: "A", "A und B", "A, B und C".
func monateText(monate []offenerMonat) string {
	labels := make([]string, len(monate))
	for i, m := range monate {
		labels[i] = store.MonatLabel(m.Monat)
	}
	if len(labels) < 2 {
		return strings.Join(labels, "")
	}
	return strings.Join(labels[:len(labels)-1], ", ") + " und " + labels[len(labels)-1]
}

// offenHinweis is the Dashboard banner for open months.
type offenHinweis struct {
	Text string
	// Pfad is where the Vermieter:innen fix it: the new form if a month has no
	// Eingabe at all, the edit page if a single month has an incomplete one,
	// the list if several do.
	Pfad string
}

// offenerHinweis builds the banner, nil = no open month.
func offenerHinweis(offen []offenerMonat) *offenHinweis {
	if len(offen) == 0 {
		return nil
	}
	pfad := "/fixkosten"
	if len(offen) == 1 {
		pfad = fixkostenErfassenPfad(offen[0].EingabeID)
	}
	for _, o := range offen {
		if o.EingabeID == 0 {
			pfad = fixkostenErfassenPfad(0)
			break
		}
	}
	return &offenHinweis{
		Text: "Für " + monateText(offen) + " fehlen die Fixkosten (oder sind unvollständig). Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht.",
		Pfad: pfad,
	}
}

// vorlaeufigText is the addition on Gesamt and Saldo of a Jahreskarte whose
// year holds open months, "" = none.
func vorlaeufigText(offen []offenerMonat, jahr int) string {
	var imJahr []offenerMonat
	for _, o := range offen {
		if y, ok := store.Abrechnungsmonat(o.Monat).Jahr(); ok && y == jahr {
			imJahr = append(imJahr, o)
		}
	}
	if len(imJahr) == 0 {
		return ""
	}
	return "vorläufig, ohne " + monateText(imJahr)
}
