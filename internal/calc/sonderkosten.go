package calc

import "github.com/larknafets/nebenkostenrechner/internal/store"

// SonderkostenAnteil is what one Sonderkosten entry costs each apartment.
// W1 is rounded to the cent, W2 is the rest, so W1+W2 is exactly the Betrag.
// Berechenbar is false only for Logik "Je Anzahl Personen" in a month without
// a Fixkosteneingabe: such an entry has no amounts yet and counts nowhere. An
// entry that is not umlegbar has amounts 0 (it counts nowhere either).
type SonderkostenAnteil struct {
	W1, W2      float64
	Berechenbar bool
}

// For returns the apartment's share (apartment 2, else 1), like KostenFor.
func (a SonderkostenAnteil) For(apartmentID int64) float64 {
	if apartmentID == 2 {
		return a.W2
	}
	return a.W1
}

// SonderkostenAnteile splits one entry onto both apartments. The ratios are
// the Fixkosten ones (splitRatio): Personen come from the Fixkosteneingabe of
// the entry's Abrechnungsmonat in fixkosten (nil = none). Like Umlagefähig of
// a Kostenposition, umlegbar decides alone, the Wohnungsstatus does not.
func SonderkostenAnteile(s store.Sonderkosten, apartments []store.Apartment, fixkosten *Fixkostenreihe) SonderkostenAnteil {
	if !s.Umlegbar {
		return SonderkostenAnteil{Berechenbar: true}
	}
	switch s.Umsetzung {
	case store.UmsetzungWohnung1:
		return SonderkostenAnteil{W1: Round2(s.Betrag), Berechenbar: true}
	case store.UmsetzungWohnung2:
		return SonderkostenAnteil{W2: Round2(s.Betrag), Berechenbar: true}
	}
	var personenW1, personenW2 float64
	if s.BrauchtFixkosteneingabe() {
		if fixkosten == nil {
			return SonderkostenAnteil{}
		}
		fm, ok := fixkosten.Monat(s.Monat)
		if !ok {
			return SonderkostenAnteil{}
		}
		personenW1, personenW2 = fm.Eingabe.Personen[1], fm.Eingabe.Personen[2]
	}
	qmW1, qmW2 := apartmentValues(apartments, func(a store.Apartment) float64 { return a.QM })
	flurstueckW1, flurstueckW2 := apartmentValues(apartments, func(a store.Apartment) float64 { return a.FlurstueckGroesse })
	r1, _ := splitRatio(s.Logik, qmW1, qmW2, flurstueckW1, flurstueckW2, personenW1, personenW2)
	w1 := Round2(s.Betrag * r1)
	return SonderkostenAnteil{W1: w1, W2: Round2(s.Betrag - w1), Berechenbar: true}
}

// MonatsSonderkosten is one entry's amount for one apartment in its
// Abrechnungsmonat. Logik is the key the amount is shown under: the entry's
// Logik, or "wohnung1"/"wohnung2" for an entry assigned to one apartment.
type MonatsSonderkosten struct {
	Monat, Logik string
	Betrag       float64
}

// SonderkostenMonate returns the entries that count for apartmentID: umlegbar,
// berechenbar and with an amount above 0 for the apartment, in the order of
// list. Dashboard and Jahresabrechnung both feed it to Monatswerte.
func SonderkostenMonate(list []store.Sonderkosten, apartments []store.Apartment, fixkosten *Fixkostenreihe, apartmentID int64) []MonatsSonderkosten {
	var out []MonatsSonderkosten
	for _, s := range list {
		a := SonderkostenAnteile(s, apartments, fixkosten)
		betrag := a.For(apartmentID)
		if !a.Berechenbar || betrag == 0 {
			continue
		}
		logik := s.Logik
		switch s.Umsetzung {
		case store.UmsetzungWohnung1:
			logik = store.LogikWohnung1
		case store.UmsetzungWohnung2:
			logik = store.LogikWohnung2
		}
		if logik == "" {
			logik = store.LogikWohneinheit
		}
		out = append(out, MonatsSonderkosten{Monat: s.Monat, Logik: logik, Betrag: betrag})
	}
	return out
}
