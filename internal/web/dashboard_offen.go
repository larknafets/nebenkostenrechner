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
