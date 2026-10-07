// Package abrechnung holds the pure rules of the Jahresabrechnung: the
// Zeitraum of a year, the Prüfung whether it can be settled and the Berechnung
// of the Jahresabrechnung with its Übertrag Vorjahre and Monatsverlauf. It
// reads only the data handed in (Daten, BerechnungsDaten), never a database.
package abrechnung

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// MangelArt classifies one finding of the Abrechnungsprüfung.
type MangelArt string

const (
	MangelKeinZeitraum      MangelArt = "kein_zeitraum"
	MangelAblesungFehlt     MangelArt = "ablesung_fehlt"
	MangelAblesungTeilstand MangelArt = "ablesung_teilstand"
	MangelFixkostenFehlt    MangelArt = "fixkosten_fehlt"
	MangelFixkostenMehrfach MangelArt = "fixkosten_mehrfach"
	MangelStammdaten        MangelArt = "stammdaten"
)

// Mangel is one reason a Jahresabrechnung cannot be produced
// (Issue #165): what is wrong and where to fix it.
type Mangel struct {
	Art MangelArt
	// Monat is the Abrechnungsmonat ("YYYY-MM-01") the finding is about, empty
	// for one that belongs to no month (Stammdaten).
	Monat string
	// Text names the problem for the user, e.g. "Fixkosten-Eingabe fehlt:
	// November 2027".
	Text string
	// Aktion labels the link that fixes it, e.g. "Fixkosten erfassen".
	Aktion string
	// Pfad is the target inside the app, without the base path (the page
	// prefixes requestBase). It is empty only for a finding the user cannot
	// fix in the app: a year before the first Ablesung simply has no data,
	// there is nothing to enter or correct. See HatZiel.
	Pfad string
}

// HatZiel reports whether the finding links to a place where it can be
// fixed. A finding without one is deliberately not fixable, the page shows
// its text without a link.
func (m Mangel) HatZiel() bool { return m.Pfad != "" }

// Monatsbereich is the range of months of a year a Jahresabrechnung covers,
// as month numbers 1-12 (volle Monate, for a Mieterwechsel inside the year).
type Monatsbereich struct{ Von, Bis int }

// GanzesJahr is the default: the whole calendar year.
var GanzesJahr = Monatsbereich{Von: 1, Bis: 12}

// Gueltig reports whether the range is 1 <= Von <= Bis <= 12.
func (b Monatsbereich) Gueltig() bool { return 1 <= b.Von && b.Von <= b.Bis && b.Bis <= 12 }

// Zeitraum is the billing period of one Jahresabrechnung.
type Zeitraum struct {
	Jahr int
	// Teilzeitraum is true if the period was narrowed to some months of the
	// year (a Mieterwechsel). The first Erfassungsjahr alone is a TeilJahr,
	// not a Teilzeitraum.
	Teilzeitraum bool
	// LetzterMonat is the last Abrechnungsmonat of the period (first of
	// month): December unless narrowed.
	LetzterMonat time.Time
	// ErsterMonat is the first Abrechnungsmonat of the period (first of
	// month): the month of the first Ablesung in the first Erfassungsjahr,
	// else January.
	ErsterMonat time.Time
	// Von/Bis are the first and last day. Von is the day of the first
	// Ablesung in the first Erfassungsjahr, else 1 January.
	Von, Bis time.Time
	// TeilJahr is true for the first Erfassungsjahr, whose period starts at
	// the first Ablesung (the Ausgangsstand) instead of on 1 January.
	TeilJahr bool
	// Zusatz explains a Teiljahr in the head of the Abrechnung ("Fixkosten
	// und Vorauszahlungen ab Oktober 2026"), empty for a full year.
	Zusatz string
}

// Pruefung is the result of checking whether a year can be
// settled for one apartment. An empty Maengel list means abrechenbar.
type Pruefung struct {
	// Zeitraum is nil if the year has no period at all (before the first
	// Ablesung, or no Ablesung yet) - then Maengel says why.
	Zeitraum *Zeitraum
	Maengel  []Mangel
}

// Titel is the heading of the Abrechnung.
func (z Zeitraum) Titel() string {
	if !z.Teilzeitraum {
		return fmt.Sprintf("Nebenkostenabrechnung %d", z.Jahr)
	}
	return "Nebenkostenabrechnung " + MonatLabel(z.Von.Format("2006-01-02")) + " bis " + MonatLabel(z.Bis.Format("2006-01-02"))
}

// Abrechenbar reports whether the Abrechnung can be produced.
func (p Pruefung) Abrechenbar() bool { return len(p.Maengel) == 0 }

// Daten is everything Pruefe reads, so the
// rules themselves are a pure function of this data.
type Daten struct {
	// Periods are all Ablesungen, oldest first (store.AllPeriodDetails).
	Periods    []*store.LatestPeriod
	Eingaben   []store.FixkostenEingabeSummary
	Apartments []store.Apartment
	Haus       store.Haus
}

// Pruefe applies the rules to already loaded data. It
// returns an error only for an unknown apartmentID.
//
// The period is the calendar year, except in the first Erfassungsjahr: it
// starts at the day of the first Ablesung ever (the Ausgangsstand) and ends
// on 31 December. The consumption of every later Ablesung is the difference
// to its chronological predecessor, so it is always computable inside the
// period - the first Ablesung is the only one without a predecessor and it
// is exactly where the period starts. That is why there is no separate
// finding for a missing predecessor.
//
// Rules for the months of the period:
//  1. Every month from the one of the first Ablesung needs at least one
//     complete Ablesung (no Teilstand) with that Abrechnungsmonat.
//  2. Every month needs exactly one Fixkosten-Eingabe - except the month of
//     the first Ablesung in the first year, where none is needed (one that
//     exists counts, two are still ambiguous).
//  3. A missing Nebenkostenabschlag is not a finding, it counts as 0.
//  4. Stammdaten: Vermieter name/address, Objektanschrift and both
//     Wohnflächen are always required, plus Mieter name and Zustellanschrift
//     if the apartment is vermietet. The IBAN is optional.
//
// A bereich narrows the period to some months of the year (a Mieterwechsel
// inside the year, always volle Monate): the rules then apply to those months
// only. A period starting after the first month needs nothing special, its
// first consumption is the difference to the Ablesung before it.
func Pruefe(d Daten, jahr int, bereich Monatsbereich, apartmentID int64) (Pruefung, error) {
	var apartment *store.Apartment
	for i := range d.Apartments {
		if d.Apartments[i].ID == apartmentID {
			apartment = &d.Apartments[i]
		}
	}
	if apartment == nil {
		return Pruefung{}, fmt.Errorf("unknown apartment %d", apartmentID)
	}

	var res Pruefung
	zeitraum, ablesungVon, fixkostenVon, ok := bestimmeZeitraum(d.Periods, jahr, bereich, &res)
	if !ok {
		return res, nil
	}
	res.Zeitraum = &zeitraum

	res.Maengel = append(res.Maengel, PruefeMonate(d, ablesungVon, zeitraum.LetzterMonat, fixkostenVon)...)
	res.Maengel = append(res.Maengel, pruefeStammdaten(d, *apartment)...)
	return res, nil
}

// PruefeMonate applies the month rules 1 and 2 (see Pruefe)
// to the months ablesungVon to letzter: every month needs a complete
// Ablesung, every month from fixkostenVon needs exactly one Fixkosten-
// Eingabe (more than one is ambiguous in any month). The Übertrag Vorjahre
// reuses it for the months before the period.
func PruefeMonate(d Daten, ablesungVon, letzter, fixkostenVon time.Time) []Mangel {
	var maengel []Mangel
	// Ablesungen per Abrechnungsmonat, split into complete ones and
	// Teilstände (the newest Ablesung may be one, it never counts).
	vollstaendig := map[string]bool{}
	teilstand := map[string]*store.LatestPeriod{}
	for _, p := range d.Periods {
		if p.Monat == "" {
			continue
		}
		if p.Teilstand(d.Apartments).IstTeilstand {
			if teilstand[p.Monat] == nil {
				teilstand[p.Monat] = p
			}
			continue
		}
		vollstaendig[p.Monat] = true
	}
	for m := ablesungVon; !m.After(letzter); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		if vollstaendig[key] {
			continue
		}
		if p := teilstand[key]; p != nil {
			maengel = append(maengel, Mangel{
				Art:    MangelAblesungTeilstand,
				Monat:  key,
				Text:   "Ablesung ist ein Teilstand: " + MonatLabel(key),
				Aktion: "Ablesung vervollständigen",
				Pfad:   "/ablesungen/" + strconv.FormatInt(p.ID, 10) + "/bearbeiten",
			})
			continue
		}
		maengel = append(maengel, Mangel{
			Art:    MangelAblesungFehlt,
			Monat:  key,
			Text:   "Ablesung fehlt: " + MonatLabel(key),
			Aktion: "Ablesung erfassen",
			Pfad:   "/ablesungen/neu",
		})
	}

	eingabenJeMonat := map[string]int{}
	for _, e := range d.Eingaben {
		eingabenJeMonat[e.Monat]++
	}
	for m := ablesungVon; !m.After(letzter); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		n := eingabenJeMonat[key]
		switch {
		case n > 1:
			maengel = append(maengel, Mangel{
				Art:    MangelFixkostenMehrfach,
				Monat:  key,
				Text:   "Mehr als eine Fixkosten-Eingabe: " + MonatLabel(key),
				Aktion: "Fixkosten korrigieren",
				Pfad:   "/fixkosten",
			})
		case n == 0 && !m.Before(fixkostenVon):
			maengel = append(maengel, Mangel{
				Art:    MangelFixkostenFehlt,
				Monat:  key,
				Text:   "Fixkosten-Eingabe fehlt: " + MonatLabel(key),
				Aktion: "Fixkosten erfassen",
				Pfad:   "/fixkosten/neu",
			})
		}
	}
	return maengel
}

// bestimmeZeitraum derives the billing period of jahr from the Ablesungen.
// It also returns the first month needing an Ablesung and the first month
// needing a Fixkosten-Eingabe. ok is false if the year has no period; the
// reason is then already appended to res.
func bestimmeZeitraum(periods []*store.LatestPeriod, jahr int, bereich Monatsbereich, res *Pruefung) (z Zeitraum, ablesungVon, fixkostenVon time.Time, ok bool) {
	if len(periods) == 0 {
		res.Maengel = append(res.Maengel, Mangel{
			Art:    MangelKeinZeitraum,
			Text:   "Es gibt noch keine Ablesung",
			Aktion: "Ablesung erfassen",
			Pfad:   "/ablesungen/neu",
		})
		return z, ablesungVon, fixkostenVon, false
	}

	first := periods[0]
	firstDate, firstMonat, err := Erfassungsbeginn(first)
	if err != nil {
		res.Maengel = append(res.Maengel, Mangel{
			Art:    MangelKeinZeitraum,
			Text:   "Das Datum der ersten Ablesung ist ungültig: " + first.ReadingDate,
			Aktion: "Ablesung korrigieren",
			Pfad:   "/ablesungen/" + strconv.FormatInt(first.ID, 10) + "/bearbeiten",
		})
		return z, ablesungVon, fixkostenVon, false
	}
	if jahr < firstMonat.Year() {
		res.Maengel = append(res.Maengel, Mangel{
			Art:  MangelKeinZeitraum,
			Text: fmt.Sprintf("Für %d gibt es keine Daten: die erste Ablesung liegt im Jahr %d", jahr, firstMonat.Year()),
		})
		return z, ablesungVon, fixkostenVon, false
	}

	if jahr == firstMonat.Year() && int(firstMonat.Month()) > bereich.Bis {
		res.Maengel = append(res.Maengel, Mangel{
			Art:  MangelKeinZeitraum,
			Text: "Für den gewählten Zeitraum gibt es keine Daten: die erste Ablesung liegt im " + MonatLabel(firstMonat.Format("2006-01-02")),
		})
		return z, ablesungVon, fixkostenVon, false
	}

	ablesungVon = time.Date(jahr, time.Month(bereich.Von), 1, 0, 0, 0, 0, time.UTC)
	letzter := time.Date(jahr, time.Month(bereich.Bis), 1, 0, 0, 0, 0, time.UTC)
	z = Zeitraum{
		Jahr: jahr, Teilzeitraum: bereich != GanzesJahr, ErsterMonat: ablesungVon, LetzterMonat: letzter,
		Von: ablesungVon, Bis: letzter.AddDate(0, 1, -1),
	}
	fixkostenVon = fixkostenBeginn(firstMonat, ablesungVon)

	if jahr == firstMonat.Year() && !firstMonat.Before(ablesungVon) {
		z.TeilJahr = true
		z.ErsterMonat = firstMonat
		ablesungVon = firstMonat
		fixkostenVon = fixkostenBeginn(firstMonat, ablesungVon)
		if firstDate.Year() == jahr {
			z.Von = firstDate
		}
		if fixkostenVon.Year() == jahr {
			z.Zusatz = "Fixkosten und Vorauszahlungen ab " + MonatLabel(fixkostenVon.Format("2006-01-02")) +
				" (Ausgangsstand: erste Ablesung am " + firstDate.Format("02.01.2006") + ")."
		} else {
			z.Zusatz = "Fixkosten und Vorauszahlungen entfallen (Ausgangsstand: erste Ablesung am " + firstDate.Format("02.01.2006") + ")."
		}
	}
	return z, ablesungVon, fixkostenVon, true
}

// fixkostenBeginn is the first month needing a Fixkosten-Eingabe in a range of
// months starting at ablesungVon: the month after ablesungVon if that is the
// month of the first Ablesung ever (erfassungsmonat, the Ausgangsstand month
// needs none), else ablesungVon itself. The Zeitraum and the Übertrag
// Vorjahre both apply this rule.
func fixkostenBeginn(erfassungsmonat, ablesungVon time.Time) time.Time {
	if ablesungVon.Equal(erfassungsmonat) {
		return ablesungVon.AddDate(0, 1, 0)
	}
	return ablesungVon
}

// Vormonat is the Abrechnungsmonat before the period, the last month an
// Übertrag Vorjahre can sum up.
func (z Zeitraum) Vormonat() time.Time { return z.ErsterMonat.AddDate(0, -1, 0) }

// Erfassungsbeginn returns the reading date and the Abrechnungsmonat of the
// first Ablesung ever. The Abrechnungsmonat decides which month it belongs
// to; without one (a Teilstand) its reading date does.
func Erfassungsbeginn(first *store.LatestPeriod) (datum, monat time.Time, err error) {
	datum, err = time.Parse("2006-01-02", first.ReadingDate)
	if err != nil {
		return datum, monat, err
	}
	monat = time.Date(datum.Year(), datum.Month(), 1, 0, 0, 0, 0, time.UTC)
	if t, err := time.Parse("2006-01-02", first.Monat); err == nil {
		monat = t
	}
	return datum, monat, nil
}

// pruefeStammdaten checks the Stammdaten the Jahresabrechnung needs for
// apartment (rule 4 above).
func pruefeStammdaten(d Daten, apartment store.Apartment) []Mangel {
	leer := func(s string) bool { return strings.TrimSpace(s) == "" }
	var out []Mangel
	add := func(text string) {
		out = append(out, Mangel{Art: MangelStammdaten, Text: text, Aktion: "Stammdaten ergänzen", Pfad: "/stammdaten"})
	}

	if leer(d.Haus.VermieterName) {
		add("Name des Vermieters fehlt")
	}
	if leer(d.Haus.VermieterAnschrift) {
		add("Anschrift des Vermieters fehlt")
	}
	if leer(d.Haus.ObjektAnschrift) {
		add("Anschrift des Objekts fehlt")
	}
	for _, a := range d.Apartments {
		if a.QM <= 0 {
			add(fmt.Sprintf("Wohnungsgröße von %s fehlt (muss größer als 0 sein)", a.Name))
		}
	}
	if apartment.Status == store.StatusVermietet {
		if leer(apartment.MieterName) {
			add(fmt.Sprintf("Name des Mieters von %s fehlt", apartment.Name))
		}
		if leer(apartment.MieterAnschrift) {
			add(fmt.Sprintf("Zustellanschrift des Mieters von %s fehlt", apartment.Name))
		}
	}
	return out
}

// Monatsnamen are the German month names, January first.
var Monatsnamen = [...]string{
	"Januar", "Februar", "März", "April", "Mai", "Juni",
	"Juli", "August", "September", "Oktober", "November", "Dezember",
}

// MonatLabel renders a date ("YYYY-MM-DD") as its German month name and year
// (e.g. "November 2026"). Falls back to the raw string if it isn't a
// parseable date.
func MonatLabel(datum string) string {
	t, err := time.Parse("2006-01-02", datum)
	if err != nil {
		return datum
	}
	return fmt.Sprintf("%s %d", Monatsnamen[t.Month()-1], t.Year())
}
