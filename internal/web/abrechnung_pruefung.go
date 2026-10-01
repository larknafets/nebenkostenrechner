package web

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// mangelArt classifies one finding of the Abrechnungsprüfung.
type mangelArt string

const (
	mangelKeinZeitraum      mangelArt = "kein_zeitraum"
	mangelAblesungFehlt     mangelArt = "ablesung_fehlt"
	mangelAblesungTeilstand mangelArt = "ablesung_teilstand"
	mangelFixkostenFehlt    mangelArt = "fixkosten_fehlt"
	mangelFixkostenMehrfach mangelArt = "fixkosten_mehrfach"
	mangelStammdaten        mangelArt = "stammdaten"
)

// abrechnungMangel is one reason a Jahresabrechnung cannot be produced
// (Issue #165): what is wrong and where to fix it.
type abrechnungMangel struct {
	Art mangelArt
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
func (m abrechnungMangel) HatZiel() bool { return m.Pfad != "" }

// abrechnungZeitraum is the billing period of one Jahresabrechnung.
type abrechnungZeitraum struct {
	Jahr int
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

// abrechnungPruefung is the result of checking whether a year can be
// settled for one apartment. An empty Maengel list means abrechenbar.
type abrechnungPruefung struct {
	// Zeitraum is nil if the year has no period at all (before the first
	// Ablesung, or no Ablesung yet) - then Maengel says why.
	Zeitraum *abrechnungZeitraum
	Maengel  []abrechnungMangel
}

// Abrechenbar reports whether the Abrechnung can be produced.
func (p abrechnungPruefung) Abrechenbar() bool { return len(p.Maengel) == 0 }

// abrechnungPruefDaten is everything pruefeAbrechnungDaten reads, loaded
// once, so the rules themselves are a pure function of this data.
type abrechnungPruefDaten struct {
	// Periods are all Ablesungen, oldest first (store.AllPeriodDetails).
	Periods    []*store.LatestPeriod
	Eingaben   []store.FixkostenEingabeSummary
	Apartments []store.Apartment
	Haus       store.Haus
}

// pruefeAbrechnung loads the data and checks whether jahr can be settled
// for apartmentID (Issue #165, decisions in the Wayfinder ticket
// "Vollständigkeitsprüfung").
func pruefeAbrechnung(db *sql.DB, jahr int, apartmentID int64) (abrechnungPruefung, error) {
	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abrechnungPruefung{}, fmt.Errorf("periods: %w", err)
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return abrechnungPruefung{}, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abrechnungPruefung{}, fmt.Errorf("apartments: %w", err)
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abrechnungPruefung{}, fmt.Errorf("haus: %w", err)
	}
	return pruefeAbrechnungDaten(abrechnungPruefDaten{Periods: periods, Eingaben: eingaben, Apartments: apartments, Haus: haus}, jahr, apartmentID)
}

// pruefeAbrechnungDaten applies the rules to already loaded data. It
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
func pruefeAbrechnungDaten(d abrechnungPruefDaten, jahr int, apartmentID int64) (abrechnungPruefung, error) {
	var apartment *store.Apartment
	for i := range d.Apartments {
		if d.Apartments[i].ID == apartmentID {
			apartment = &d.Apartments[i]
		}
	}
	if apartment == nil {
		return abrechnungPruefung{}, fmt.Errorf("unknown apartment %d", apartmentID)
	}

	var res abrechnungPruefung
	zeitraum, ablesungVon, fixkostenVon, ok := bestimmeZeitraum(d.Periods, jahr, &res)
	if !ok {
		return res, nil
	}
	res.Zeitraum = &zeitraum

	// Ablesungen per Abrechnungsmonat, split into complete ones and
	// Teilstände (the newest Ablesung may be one, it never counts).
	vollstaendig := map[string]bool{}
	teilstand := map[string]*store.LatestPeriod{}
	for _, p := range d.Periods {
		if p.Monat == "" {
			continue
		}
		if newTeilstandStatus(p, d.Apartments).IstTeilstand {
			if teilstand[p.Monat] == nil {
				teilstand[p.Monat] = p
			}
			continue
		}
		vollstaendig[p.Monat] = true
	}
	for m := ablesungVon; !m.After(dezember(jahr)); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		if vollstaendig[key] {
			continue
		}
		if p := teilstand[key]; p != nil {
			res.Maengel = append(res.Maengel, abrechnungMangel{
				Art:    mangelAblesungTeilstand,
				Text:   "Ablesung ist ein Teilstand: " + germanPeriodLabel(key),
				Aktion: "Ablesung vervollständigen",
				Pfad:   "/ablesungen/" + strconv.FormatInt(p.ID, 10) + "/bearbeiten",
			})
			continue
		}
		res.Maengel = append(res.Maengel, abrechnungMangel{
			Art:    mangelAblesungFehlt,
			Text:   "Ablesung fehlt: " + germanPeriodLabel(key),
			Aktion: "Ablesung erfassen",
			Pfad:   "/ablesungen/neu",
		})
	}

	eingabenJeMonat := map[string]int{}
	for _, e := range d.Eingaben {
		eingabenJeMonat[e.Monat]++
	}
	for m := ablesungVon; !m.After(dezember(jahr)); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		n := eingabenJeMonat[key]
		switch {
		case n > 1:
			res.Maengel = append(res.Maengel, abrechnungMangel{
				Art:    mangelFixkostenMehrfach,
				Text:   "Mehr als eine Fixkosten-Eingabe: " + germanPeriodLabel(key),
				Aktion: "Fixkosten korrigieren",
				Pfad:   "/fixkosten",
			})
		case n == 0 && !m.Before(fixkostenVon):
			res.Maengel = append(res.Maengel, abrechnungMangel{
				Art:    mangelFixkostenFehlt,
				Text:   "Fixkosten-Eingabe fehlt: " + germanPeriodLabel(key),
				Aktion: "Fixkosten erfassen",
				Pfad:   "/fixkosten/neu",
			})
		}
	}

	res.Maengel = append(res.Maengel, pruefeStammdaten(d, *apartment)...)
	return res, nil
}

// dezember returns the first day of December of jahr.
func dezember(jahr int) time.Time { return time.Date(jahr, time.December, 1, 0, 0, 0, 0, time.UTC) }

// bestimmeZeitraum derives the billing period of jahr from the Ablesungen.
// It also returns the first month needing an Ablesung and the first month
// needing a Fixkosten-Eingabe. ok is false if the year has no period; the
// reason is then already appended to res.
func bestimmeZeitraum(periods []*store.LatestPeriod, jahr int, res *abrechnungPruefung) (z abrechnungZeitraum, ablesungVon, fixkostenVon time.Time, ok bool) {
	if len(periods) == 0 {
		res.Maengel = append(res.Maengel, abrechnungMangel{
			Art:    mangelKeinZeitraum,
			Text:   "Es gibt noch keine Ablesung",
			Aktion: "Ablesung erfassen",
			Pfad:   "/ablesungen/neu",
		})
		return z, ablesungVon, fixkostenVon, false
	}

	first := periods[0]
	firstDate, err := time.Parse("2006-01-02", first.ReadingDate)
	if err != nil {
		res.Maengel = append(res.Maengel, abrechnungMangel{
			Art:    mangelKeinZeitraum,
			Text:   "Das Datum der ersten Ablesung ist ungültig: " + first.ReadingDate,
			Aktion: "Ablesung korrigieren",
			Pfad:   "/ablesungen/" + strconv.FormatInt(first.ID, 10) + "/bearbeiten",
		})
		return z, ablesungVon, fixkostenVon, false
	}
	// The Abrechnungsmonat decides which month the first Ablesung belongs
	// to; without one (a Teilstand) its reading date does.
	firstMonat := time.Date(firstDate.Year(), firstDate.Month(), 1, 0, 0, 0, 0, time.UTC)
	if t, err := time.Parse("2006-01-02", first.Monat); err == nil {
		firstMonat = t
	}

	if jahr < firstMonat.Year() {
		res.Maengel = append(res.Maengel, abrechnungMangel{
			Art:  mangelKeinZeitraum,
			Text: fmt.Sprintf("Für %d gibt es keine Daten: die erste Ablesung liegt im Jahr %d", jahr, firstMonat.Year()),
		})
		return z, ablesungVon, fixkostenVon, false
	}

	z = abrechnungZeitraum{Jahr: jahr, ErsterMonat: time.Date(jahr, time.January, 1, 0, 0, 0, 0, time.UTC), Von: time.Date(jahr, time.January, 1, 0, 0, 0, 0, time.UTC), Bis: time.Date(jahr, time.December, 31, 0, 0, 0, 0, time.UTC)}
	ablesungVon = time.Date(jahr, time.January, 1, 0, 0, 0, 0, time.UTC)
	fixkostenVon = ablesungVon

	if jahr == firstMonat.Year() {
		z.TeilJahr = true
		z.ErsterMonat = firstMonat
		ablesungVon = firstMonat
		fixkostenVon = firstMonat.AddDate(0, 1, 0)
		if firstDate.Year() == jahr {
			z.Von = firstDate
		}
		if fixkostenVon.Year() == jahr {
			z.Zusatz = "Fixkosten und Vorauszahlungen ab " + germanPeriodLabel(fixkostenVon.Format("2006-01-02")) +
				" (Ausgangsstand: erste Ablesung am " + firstDate.Format("02.01.2006") + ")."
		} else {
			z.Zusatz = "Fixkosten und Vorauszahlungen entfallen (Ausgangsstand: erste Ablesung am " + firstDate.Format("02.01.2006") + ")."
		}
	}
	return z, ablesungVon, fixkostenVon, true
}

// pruefeStammdaten checks the Stammdaten the Jahresabrechnung needs for
// apartment (rule 4 above).
func pruefeStammdaten(d abrechnungPruefDaten, apartment store.Apartment) []abrechnungMangel {
	leer := func(s string) bool { return strings.TrimSpace(s) == "" }
	var out []abrechnungMangel
	add := func(text string) {
		out = append(out, abrechnungMangel{Art: mangelStammdaten, Text: text, Aktion: "Stammdaten ergänzen", Pfad: "/stammdaten"})
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
