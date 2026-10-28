package web

import (
	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// monateText lists the labels of the open months in German.
func monateText(monate []calc.OffenerMonat) string {
	labels := make([]string, len(monate))
	for i, m := range monate {
		labels[i] = store.MonatLabel(m.Monat)
	}
	return monateListe(labels)
}

// monateStored are the Monate ("YYYY-MM-01") of the open months.
func monateStored(monate []calc.OffenerMonat) []string {
	out := make([]string, len(monate))
	for i, m := range monate {
		out[i] = m.Monat
	}
	return out
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
func offenerHinweis(offen []calc.OffenerMonat) *offenHinweis {
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
		Text: fixkostenFehlenSatz(monateStored(offen)) + " Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht.",
		Pfad: pfad,
	}
}

// vorlaeufigText is the addition on Gesamt and Saldo of a Jahreskarte whose
// year holds open months, "" = none.
func vorlaeufigText(offen []calc.OffenerMonat, jahr int) string {
	var imJahr []calc.OffenerMonat
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
