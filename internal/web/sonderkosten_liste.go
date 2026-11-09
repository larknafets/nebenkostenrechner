package web

import (
	"slices"
	"strconv"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// sonderkostenFilter is the filter of the Sonderkosten page.
type sonderkostenFilter struct {
	Jahr      int
	AlleJahre bool   // the Jahr filter is "Alle Jahre"
	Art       string // "" means no Art filter
	Objekt    string // "" means no Objekt filter
}

// sonderkostenFilterAus reads the wishes ?jahr=, ?art= and ?objekt= against the offered
// years: an unknown year falls back to the current one (heute), an unknown Art
// or Objekt to no filter, "alle" shows every year.
func sonderkostenFilterAus(jahrQ, artQ, objektQ string, jahre []int, heute int) sonderkostenFilter {
	f := sonderkostenFilter{Jahr: heute, AlleJahre: jahrQ == "alle"}
	if j, err := strconv.Atoi(jahrQ); err == nil && slices.Contains(jahre, j) {
		f.Jahr = j
	}
	if slices.ContainsFunc(store.SonderkostenArten, func(a store.Auswahl) bool { return a.Wert == artQ }) {
		f.Art = artQ
	}
	if slices.ContainsFunc(store.SonderkostenObjekte, func(a store.Auswahl) bool { return a.Wert == objektQ }) {
		f.Objekt = objektQ
	}
	return f
}

// passt reports whether s passes the Jahr, Art and Objekt filter.
func (f sonderkostenFilter) passt(s store.Sonderkosten) bool {
	y, _ := store.Abrechnungsmonat(s.Monat).Jahr()
	return (f.AlleJahre || y == f.Jahr) && (f.Art == "" || s.Art == f.Art) && (f.Objekt == "" || s.Objekt == f.Objekt)
}

// sonderkostenJahre lists every year with entries and always the current one,
// newest first.
func sonderkostenJahre(alle []store.Sonderkosten, heute int) []int {
	jahre := []int{heute}
	for _, s := range alle {
		if y, ok := store.Abrechnungsmonat(s.Monat).Jahr(); ok && !slices.Contains(jahre, y) {
			jahre = append(jahre, y)
		}
	}
	slices.Sort(jahre)
	slices.Reverse(jahre)
	return jahre
}

// sonderkostenListe is the filtered list model: month groups (newest first)
// and the sums over the shown entries.
type sonderkostenListe struct {
	Gesamt, Umlegbar float64
	Monate           []sonderkostenMonat
}

// VermieterTraegt is the part of Gesamt that is not umlegbar.
func (l sonderkostenListe) VermieterTraegt() float64 { return l.Gesamt - l.Umlegbar }

// baueSonderkostenListe filters alle (oldest month first, as the store returns
// it) and groups the cards by Abrechnungsmonat. fixMonate are the months
// ("YYYY-MM") with a Fixkosteneingabe.
func baueSonderkostenListe(alle []store.Sonderkosten, fixMonate []string, f sonderkostenFilter) sonderkostenListe {
	var l sonderkostenListe
	for _, s := range alle {
		if !f.passt(s) {
			continue
		}
		l.Gesamt += s.Betrag
		if s.Umlegbar {
			l.Umlegbar += s.Betrag
		}
		if n := len(l.Monate); n == 0 || l.Monate[n-1].Label != store.MonatLabel(s.Monat) {
			l.Monate = append(l.Monate, sonderkostenMonat{Label: store.MonatLabel(s.Monat)})
		}
		m := &l.Monate[len(l.Monate)-1]
		k := sonderkostenKarte{
			Betreff: s.Betreff, Beschreibung: s.Beschreibung, Art: sonderkostenArtLabel(s.Art), Objekt: sonderkostenObjektLabel(s.Objekt), Firma: s.Firma,
			Rechnungsnummer: s.Rechnungsnummer, Rechnungsdatum: s.Rechnungsdatum, Betrag: s.Betrag, Umlegbar: s.Umlegbar, Zuordnung: sonderkostenZuordnung(s),
			NichtBerechenbar: s.BrauchtFixkosteneingabe() && !slices.Contains(fixMonate, s.Monat[:7]),
			ID:               s.ID, Maske: neueMaske("sk-"+strconv.FormatInt(s.ID, 10), s, s.Monat[:7], formatDecimalDE2(s.Betrag)),
		}
		if s.Umlegbar {
			m.Umlegbar += s.Betrag
			k.LoeschenZusatz = "Saldo und Abrechnung ändern sich."
		}
		m.Karten = append(m.Karten, k)
	}
	// Newest month first, like the Dashboard.
	slices.Reverse(l.Monate)
	return l
}

// sonderkostenAufstellung is the table of invoice amounts by Art and Objekt
// under the sum cards: one block per year ("Alle Jahre", newest first), else
// a single block without a year.
type sonderkostenAufstellung struct{ Bloecke []aufstellungBlock }

type aufstellungBlock struct {
	Jahr   int // 0: no year heading
	Zeilen []aufstellungZeile
	Summe  aufstellungZeile
}

// aufstellungZeile holds the amounts per Objekt in the order of
// store.SonderkostenObjekte.
type aufstellungZeile struct {
	Art     string // label
	n       int    // entries counted
	Objekte []float64
}

// baueSonderkostenAufstellung sums the invoice amounts of the entries passing f
// by Art and Objekt. Only Arten with entries appear, no entries give no block.
func baueSonderkostenAufstellung(alle []store.Sonderkosten, f sonderkostenFilter) sonderkostenAufstellung {
	var a sonderkostenAufstellung
	for _, s := range alle {
		if !f.passt(s) {
			continue
		}
		ai := slices.IndexFunc(store.SonderkostenArten, func(x store.Auswahl) bool { return x.Wert == s.Art })
		o := slices.IndexFunc(store.SonderkostenObjekte, func(x store.Auswahl) bool { return x.Wert == s.Objekt })
		if ai < 0 || o < 0 {
			continue // shortcut: an unknown Art or Objekt (only a hand-edited database has one) is left out of the table, list it when one occurs
		}
		jahr := 0
		if f.AlleJahre {
			jahr, _ = store.Abrechnungsmonat(s.Monat).Jahr()
		}
		// alle is oldest month first, so the entries of one year are contiguous.
		if n := len(a.Bloecke); n == 0 || a.Bloecke[n-1].Jahr != jahr {
			neu := aufstellungBlock{Jahr: jahr, Summe: aufstellungZeile{Objekte: make([]float64, len(store.SonderkostenObjekte))}}
			for _, art := range store.SonderkostenArten {
				neu.Zeilen = append(neu.Zeilen, aufstellungZeile{Art: art.Label, Objekte: make([]float64, len(store.SonderkostenObjekte))})
			}
			a.Bloecke = append(a.Bloecke, neu)
		}
		b := &a.Bloecke[len(a.Bloecke)-1]
		for _, r := range []*aufstellungZeile{&b.Zeilen[ai], &b.Summe} {
			r.Objekte[o] += s.Betrag
			r.n++
		}
	}
	for i := range a.Bloecke {
		a.Bloecke[i].Zeilen = slices.DeleteFunc(a.Bloecke[i].Zeilen, func(z aufstellungZeile) bool { return z.n == 0 })
	}
	slices.Reverse(a.Bloecke) // newest year first
	return a
}
