package calc

import "sort"

// The Monatsverlauf of one apartment: per Abrechnungsmonat the consumption
// costs of all Ablesungen of the month, the Fixkosten, the Nebenkostenabschlag
// and the PV-Gutschrift, accumulated to a running balance. The Dashboard and
// the Jahresabrechnung both read it, so what counts in the balance is decided
// here once; they differ only in the Posten they pass.

// MonatsKosten is one Ablesung's Verbrauchskosten for its Abrechnungsmonat
// ("YYYY-MM-01"). Several Ablesungen may share a month.
type MonatsKosten struct {
	Monat  string
	Kosten Kosten
}

// MonatsFixkosten is the Fixkosten-Eingabe of an Abrechnungsmonat: its result
// and the Nebenkostenabschlag per apartment id (a missing entry is no value
// recorded, counts as 0).
type MonatsFixkosten struct {
	Monat    string
	Ergebnis *FixkostenErgebnis
	Abschlag map[int64]float64
}

// Posten says which items count for the apartment: the passed-on electricity,
// the Wallbox block and the PV-Gutschrift, and which Kostenpositionen of the
// Fixkosten. The Dashboard counts everything, the Jahresabrechnung only what
// it shows (Wohnungsstatus, Umlagefähig).
type Posten struct {
	Strom, Wallbox, PVGutschrift bool
	// Fixkosten reports whether the Kostenposition with the given key counts,
	// nil counts all.
	Fixkosten func(key string) bool
}

// Monatswert is the value of one Abrechnungsmonat. The sums are not rounded.
type Monatswert struct {
	Monat string
	// Verbrauch is Heizung plus Wasser plus, per Posten, Strom and Wallbox,
	// summed over the month's Ablesungen. HatVerbrauch is false for a month
	// without a computable Ablesung.
	Verbrauch    float64
	HatVerbrauch bool
	// Fixkosten and Abschlag come from the month's Fixkosten-Eingabe,
	// HatFixkosten is false for a month without one.
	Fixkosten, Abschlag float64
	HatFixkosten        bool
	// Gutschrift is the PV-Gutschrift, 0 if it is not part of the Posten.
	Gutschrift float64
}

// Monatswerte folds the Ablesungen and Fixkosten-Eingaben into one value per
// Abrechnungsmonat for apartmentID, oldest month first. A month with neither
// has no value. If a month has several Fixkosten-Eingaben the first one given
// counts.
func Monatswerte(apartmentID int64, kosten []MonatsKosten, fixkosten []MonatsFixkosten, p Posten) []Monatswert {
	byMonat := map[string]*Monatswert{}
	wert := func(monat string) *Monatswert {
		w, ok := byMonat[monat]
		if !ok {
			w = &Monatswert{Monat: monat}
			byMonat[monat] = w
		}
		return w
	}

	for _, k := range kosten {
		w := wert(k.Monat)
		anteil := k.Kosten.Anteil(apartmentID)
		w.HatVerbrauch = true
		w.Verbrauch += anteil.HeizungKosten + anteil.WasserKosten
		if p.Strom {
			w.Verbrauch += anteil.StromKosten
		}
		if p.Wallbox {
			w.Verbrauch += anteil.WallboxKosten
		}
		if p.PVGutschrift {
			w.Gutschrift += anteil.PVGutschrift
		}
	}
	for _, f := range fixkosten {
		w := wert(f.Monat)
		if w.HatFixkosten {
			continue
		}
		w.HatFixkosten = true
		w.Abschlag = f.Abschlag[apartmentID]
		if f.Ergebnis != nil {
			for _, pos := range f.Ergebnis.Positionen {
				if p.Fixkosten == nil || p.Fixkosten(pos.Key) {
					w.Fixkosten += pos.KostenFor(apartmentID)
				}
			}
		}
	}

	out := make([]Monatswert, 0, len(byMonat))
	for _, w := range byMonat {
		out = append(out, *w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Monat < out[j].Monat })
	return out
}

// Monatszeile is one month of a Verlauf: the values rounded to the cent and
// the running balance after the month.
type Monatszeile struct {
	Monat                                      string
	Verbrauch, Fixkosten, Abschlag, Gutschrift float64
	// Saldo is the running balance (Abschlag plus Gutschrift minus Verbrauch
	// and Fixkosten) from the first month, starting at the Übertrag.
	Saldo float64
}

// Verlauf accumulates the balance over werte (oldest first), starting at
// uebertrag. Every part is rounded to the cent first, then the months are
// accumulated with Saldoverlauf. It returns the lines and the Endsaldo, which
// is uebertrag for no months.
func Verlauf(uebertrag float64, werte []Monatswert) (zeilen []Monatszeile, endsaldo float64) {
	zeilen = make([]Monatszeile, len(werte))
	reihe := make([]SaldoMonat, len(werte))
	for i, w := range werte {
		z := Monatszeile{Monat: w.Monat, Verbrauch: Round2(w.Verbrauch), Fixkosten: Round2(w.Fixkosten), Abschlag: Round2(w.Abschlag), Gutschrift: Round2(w.Gutschrift)}
		zeilen[i] = z
		reihe[i] = SaldoMonat{Abschlag: z.Abschlag, Kosten: z.Fixkosten + z.Verbrauch, Gutschrift: z.Gutschrift}
	}
	saldi, endsaldo := Saldoverlauf(uebertrag, reihe)
	for i := range zeilen {
		zeilen[i].Saldo = saldi[i]
	}
	return zeilen, endsaldo
}
