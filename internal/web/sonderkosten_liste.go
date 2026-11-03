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
}

// sonderkostenFilterAus reads the wishes ?jahr= and ?art= against the offered
// years: an unknown year falls back to the current one (heute), an unknown Art
// to no filter, "alle" shows every year.
func sonderkostenFilterAus(jahrQ, artQ string, jahre []int, heute int) sonderkostenFilter {
	f := sonderkostenFilter{Jahr: heute, AlleJahre: jahrQ == "alle"}
	if wunsch, err := strconv.Atoi(jahrQ); err == nil && slices.Contains(jahre, wunsch) {
		f.Jahr = wunsch
	}
	if slices.ContainsFunc(store.SonderkostenArten, func(a store.Auswahl) bool { return a.Wert == artQ }) {
		f.Art = artQ
	}
	return f
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
		if y, _ := store.Abrechnungsmonat(s.Monat).Jahr(); !f.AlleJahre && y != f.Jahr {
			continue
		}
		if f.Art != "" && s.Art != f.Art {
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
			Betreff: s.Betreff, Beschreibung: s.Beschreibung, Art: sonderkostenArtLabel(s.Art), Firma: s.Firma,
			Rechnungsnummer: s.Rechnungsnummer, Betrag: s.Betrag, Umlegbar: s.Umlegbar, Zuordnung: sonderkostenZuordnung(s),
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
