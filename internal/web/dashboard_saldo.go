package web

import (
	"math"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

// applySaldo writes the Nebenkostenabschlag-Saldo (utility advance payment
// balance) into every month of monate that has a Fixkosten-Eingabe, plus its
// AbschlagProzent. monate is sorted newest-first and index-aligned with keys,
// the Abrechnungsmonat of each. The balance itself is calc.Verlauf over werte
// (oldest first), the same accumulation the Jahresabrechnung uses. The gate is
// HatFixkosten, not HatVerbrauch: the Abschlag comes exclusively from a
// Fixkosten-Eingabe, so a month with only a reading but (still) no entry has
// no recorded advance-payment value and is skipped like a month with no data,
// instead of being computed with Abschlag 0 (#94: "no data" instead of an
// implicit jump).
func applySaldo(monate []dashboardMonat, keys []string, werte []calc.Monatswert) {
	var reihe []calc.Monatswert
	for _, w := range werte {
		if w.HatFixkosten {
			reihe = append(reihe, w)
		}
	}
	zeilen, _ := calc.Verlauf(0, reihe)
	saldi := make(map[string]float64, len(zeilen))
	var maxAbsSaldo float64
	for _, z := range zeilen {
		saldi[z.Monat] = z.Saldo
		if abs := math.Abs(z.Saldo); abs > maxAbsSaldo {
			maxAbsSaldo = abs
		}
	}
	for i, key := range keys {
		saldo, ok := saldi[key]
		if !ok {
			continue
		}
		monate[i].Saldo = calc.NewAbschlagSaldo(saldo)
		if maxAbsSaldo > 0 {
			monate[i].AbschlagProzent = math.Abs(saldo) / maxAbsSaldo * 50
		}
	}
}
