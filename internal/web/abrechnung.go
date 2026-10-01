package web

import (
	"database/sql"
	"fmt"
	"math"
	"sort"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// abrechnungZeile is one line of the cost overview: a position (or a block
// of consecutive months of it with the same Verteilerschlüssel) with the
// whole-house cost and the share of the settled apartment.
type abrechnungZeile struct {
	Position string
	// Von/Bis are the first and last Abrechnungsmonat of the block
	// ("YYYY-MM-01").
	Von, Bis string
	// Schluessel is the Verteilerschlüssel as text, e.g. "Je Wohneinheit".
	Schluessel string
	// Gesamt is the whole-house cost, the sum of the months' values rounded
	// to the cent. Betrag is the apartment's share, the sum of the months'
	// shares each rounded to the cent (same convention as the Dashboard, so
	// Abrechnung, Saldo and Dashboard agree to the cent). Prozent is the
	// effective share Betrag/Gesamt - it can differ by a few cents from
	// "Gesamt times the Schlüssel" because of that rounding.
	Gesamt, Betrag, Prozent float64
}

// abrechnungStrom is the electricity consumption of Wohnung 2 passed on by
// agreement (not a Betriebskostenart).
type abrechnungStrom struct {
	VerbrauchKWh, Kosten float64
}

// abrechnungVerbrauchZeile is one meter in the Anhang: the Zählerstand at
// the start and the end of the period and the consumption in between.
type abrechnungVerbrauchZeile struct {
	Zaehler, Einheit        string
	Beginn, Ende, Verbrauch float64
}

// abrechnungHeizungMonat is one Abrechnungsmonat of the heating table in
// the Anhang.
type abrechnungHeizungMonat struct {
	Monat string
	// WPStromKWh is the heat pump electricity of the whole house, Waerme*
	// the heat meter consumption per apartment (MWh), Anteil the settled
	// apartment's heating cost of the month.
	WPStromKWh, WaermeW1MWh, WaermeW2MWh, Anteil float64
}

// abrechnungPersonenMonat is the occupant count of one month from its two
// independent sources. A nil map means the source has no value that month
// (e.g. no Fixkosten-Eingabe in the month of the first Ablesung).
type abrechnungPersonenMonat struct {
	Monat     string
	Fixkosten map[int64]int64
	Ablesung  map[int64]int64
}

// abrechnung is the computed Jahresabrechnung of one apartment and year.
type abrechnung struct {
	Jahr      int
	Apartment store.Apartment
	Zeitraum  abrechnungZeitraum

	// Fixkosten are the umlagefähige Kostenpositionen, Heizung and Wasser
	// the two consumption-based lines (zero Gesamt if there is no data).
	Fixkosten []abrechnungZeile
	Heizung   abrechnungZeile
	Wasser    abrechnungZeile
	// Betriebskosten is the sum of all three.
	Betriebskosten float64
	// StromW2 is nil unless Wohnung 2 is settled and the Stammdaten flag
	// "Stromverbrauch weiterberechnen" is on.
	StromW2 *abrechnungStrom
	// Vorauszahlungen is the sum of the Nebenkostenabschläge of the period
	// (a month without a recorded Abschlag counts as 0).
	Vorauszahlungen float64
	// Saldo is Vorauszahlungen minus Betriebskosten minus the passed-on
	// electricity: positive Guthaben, negative Nachzahlung. Not the
	// cumulative balance of the Dashboard.
	Saldo *AbschlagSaldo

	// Anhang data.
	Verbrauch      []abrechnungVerbrauchZeile
	HeizungMonate  []abrechnungHeizungMonat
	PersonenMonate []abrechnungPersonenMonat
	Gewichtung     float64
	Apartments     []store.Apartment
}

// abrechnungErgebnis is what the page needs: the check and, if it found
// nothing, the Abrechnung.
type abrechnungErgebnis struct {
	Pruefung abrechnungPruefung
	// Abrechnung is nil if the Pruefung found Mängel.
	Abrechnung *abrechnung
}

// berechneAbrechnung checks whether jahr can be settled for apartmentID and,
// if so, computes the Jahresabrechnung (Issue #166). It reuses
// calc.Fixkosten per Fixkosten-Eingabe and berechneKosten per Ablesung and
// only sums them, there is no second cost formula.
func berechneAbrechnung(db *sql.DB, jahr int, apartmentID int64) (abrechnungErgebnis, error) {
	pruefung, err := pruefeAbrechnung(db, jahr, apartmentID)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	if !pruefung.Abrechenbar() {
		return abrechnungErgebnis{Pruefung: pruefung}, nil
	}

	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abrechnungErgebnis{}, fmt.Errorf("periods: %w", err)
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abrechnungErgebnis{}, fmt.Errorf("apartments: %w", err)
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abrechnungErgebnis{}, fmt.Errorf("haus: %w", err)
	}
	kostenpositionen, err := store.Kostenpositionen(db)
	if err != nil {
		return abrechnungErgebnis{}, fmt.Errorf("kostenpositionen: %w", err)
	}
	eingaben, err := store.AllFixkostenEingabenDetails(db)
	if err != nil {
		return abrechnungErgebnis{}, fmt.Errorf("fixkosten eingaben: %w", err)
	}
	meters, err := store.Meters(db)
	if err != nil {
		return abrechnungErgebnis{}, err
	}

	var apartment store.Apartment
	for _, a := range apartments {
		if a.ID == apartmentID {
			apartment = a
		}
	}

	z := *pruefung.Zeitraum
	var monate []string // the Abrechnungsmonate of the period, oldest first
	for m := z.ErsterMonat; !m.After(dezember(jahr)); m = m.AddDate(0, 1, 0) {
		monate = append(monate, m.Format("2006-01-02"))
	}
	imZeitraum := make(map[string]bool, len(monate))
	for _, m := range monate {
		imZeitraum[m] = true
	}

	ab := &abrechnung{Jahr: jahr, Apartment: apartment, Zeitraum: z, Gewichtung: haus.HeizungWaermeGewichtung, Apartments: apartments}

	// Fixkosten: one Eingabe per month (the Pruefung guarantees at most
	// one, and none only in the month of the first Ablesung).
	eingabeJeMonat := map[string]*store.FixkostenEingabeDetails{}
	for _, e := range eingaben {
		if imZeitraum[e.Monat] {
			eingabeJeMonat[e.Monat] = e
		}
	}
	fixErgebnis := map[string]*calc.FixkostenErgebnis{}
	for _, m := range monate {
		e := eingabeJeMonat[m]
		if e == nil {
			continue
		}
		erg, err := calc.Fixkosten(db, e.ID)
		if err != nil {
			return abrechnungErgebnis{}, fmt.Errorf("fixkosten %d: %w", e.ID, err)
		}
		fixErgebnis[m] = erg
		ab.Vorauszahlungen += e.Abschlag[apartmentID]
	}
	for _, kp := range kostenpositionen {
		if !kp.Umlagefaehig {
			continue
		}
		ab.Fixkosten = append(ab.Fixkosten, fixkostenZeilen(kp, monate, fixErgebnis, apartmentID)...)
	}

	// Verbrauch: every complete Ablesung of the period, except the Ausgangs-
	// stand of the first year, which has no predecessor and so no consumption.
	var heizungMonate []abrechnungHeizungMonat
	heizungIdx := map[string]int{}
	var strom abrechnungStrom
	ab.Heizung = abrechnungZeile{Position: "Heizung und Warmwasser (Wärmepumpe)", Schluessel: heizungSchluessel(haus.HeizungWaermeGewichtung)}
	ab.Wasser = abrechnungZeile{Position: "Wasser und Abwasser (Verbrauch)", Schluessel: "Gemessener Verbrauch"}
	for _, p := range periods {
		if !imZeitraum[p.Monat] || newTeilstandStatus(p, apartments).IstTeilstand {
			continue
		}
		k, err := berechneKosten(db, p.ID)
		if err != nil {
			return abrechnungErgebnis{}, err
		}
		if k.KostenNote != "" {
			continue
		}

		heizungGesamt := k.Heizung.KostenHeizungW1 + k.Heizung.KostenHeizungW2
		heizungAnteil := k.Heizung.KostenHeizungW1
		wasserGesamt := k.Wasser.KostenFrischwasserW1 + k.Wasser.KostenAbwasserW1 + k.Wasser.KostenFrischwasserW2 + k.Wasser.KostenAbwasserW2
		wasserAnteil := k.Wasser.KostenFrischwasserW1 + k.Wasser.KostenAbwasserW1
		if apartmentID == 2 {
			heizungAnteil = k.Heizung.KostenHeizungW2
			wasserAnteil = k.Wasser.KostenFrischwasserW2 + k.Wasser.KostenAbwasserW2
			strom.VerbrauchKWh += k.Strom.W2VerbrauchKWh
			strom.Kosten += k.Strom.KostenW2
		}
		ab.Heizung.Gesamt += heizungGesamt
		ab.Heizung.Betrag += heizungAnteil
		ab.Wasser.Gesamt += wasserGesamt
		ab.Wasser.Betrag += wasserAnteil
		ab.Heizung.Von, ab.Heizung.Bis = vonBis(ab.Heizung.Von, ab.Heizung.Bis, p.Monat)
		ab.Wasser.Von, ab.Wasser.Bis = vonBis(ab.Wasser.Von, ab.Wasser.Bis, p.Monat)

		i, ok := heizungIdx[p.Monat]
		if !ok {
			i = len(heizungMonate)
			heizungIdx[p.Monat] = i
			heizungMonate = append(heizungMonate, abrechnungHeizungMonat{Monat: p.Monat})
		}
		heizungMonate[i].WPStromKWh += k.Heizung.WPVerbrauchW1KWh + k.Heizung.WPVerbrauchW2KWh
		heizungMonate[i].WaermeW1MWh += k.Heizung.WaermeW1MWh
		heizungMonate[i].WaermeW2MWh += k.Heizung.WaermeW2MWh
		heizungMonate[i].Anteil += heizungAnteil
	}
	for i := range heizungMonate {
		heizungMonate[i].Anteil = calc.Round2(heizungMonate[i].Anteil)
	}
	ab.HeizungMonate = heizungMonate
	for _, zeile := range []*abrechnungZeile{&ab.Heizung, &ab.Wasser} {
		zeile.Gesamt, zeile.Betrag = calc.Round2(zeile.Gesamt), calc.Round2(zeile.Betrag)
		zeile.Prozent = prozent(zeile.Betrag, zeile.Gesamt)
	}

	if apartmentID == 2 && haus.StromWeiterberechnen {
		ab.StromW2 = &abrechnungStrom{VerbrauchKWh: strom.VerbrauchKWh, Kosten: calc.Round2(strom.Kosten)}
	}

	ab.Betriebskosten = ab.Heizung.Betrag + ab.Wasser.Betrag
	for _, z := range ab.Fixkosten {
		ab.Betriebskosten += z.Betrag
	}
	ab.Betriebskosten = calc.Round2(ab.Betriebskosten)
	ab.Vorauszahlungen = calc.Round2(ab.Vorauszahlungen)
	saldo := ab.Vorauszahlungen - ab.Betriebskosten
	if ab.StromW2 != nil {
		saldo -= ab.StromW2.Kosten
	}
	ab.Saldo = newAbschlagSaldo(calc.Round2(saldo))

	ab.Verbrauch = verbrauchUebersicht(periods, apartments, meters, imZeitraum)
	ab.PersonenMonate = personenMonate(monate, eingabeJeMonat, periods, apartments)

	return abrechnungErgebnis{Pruefung: pruefung, Abrechnung: ab}, nil
}

// fixkostenZeilen builds the lines of one Kostenposition: consecutive months
// with the same Logik form one line (a change of Typ or Wert alone does not
// split it, the months' values are simply summed). A block in which the
// settled apartment pays nothing - e.g. a position the landlord allocated
// fully to the other apartment - is left out.
func fixkostenZeilen(kp store.Kostenposition, monate []string, erg map[string]*calc.FixkostenErgebnis, apartmentID int64) []abrechnungZeile {
	var out []abrechnungZeile
	var cur *abrechnungZeile
	var curLogik string
	for _, m := range monate {
		e := erg[m]
		if e == nil {
			continue
		}
		var pos *calc.FixkostenPosition
		for i := range e.Positionen {
			if e.Positionen[i].Key == kp.Key {
				pos = &e.Positionen[i]
			}
		}
		if pos == nil {
			continue
		}
		if cur == nil || pos.Logik != curLogik {
			out = append(out, abrechnungZeile{Position: kp.Label, Von: m, Schluessel: logikLabels[pos.Logik]})
			cur = &out[len(out)-1]
			curLogik = pos.Logik
		}
		cur.Bis = m
		cur.Gesamt += pos.Monatswert
		cur.Betrag += pos.KostenFor(apartmentID)
	}

	kept := out[:0]
	for _, z := range out {
		z.Gesamt, z.Betrag = calc.Round2(z.Gesamt), calc.Round2(z.Betrag)
		if z.Betrag == 0 {
			continue
		}
		z.Prozent = prozent(z.Betrag, z.Gesamt)
		kept = append(kept, z)
	}
	return kept
}

// vonBis widens the month range [von, bis] to include monat.
func vonBis(von, bis, monat string) (string, string) {
	if von == "" || monat < von {
		von = monat
	}
	if monat > bis {
		bis = monat
	}
	return von, bis
}

func prozent(betrag, gesamt float64) float64 {
	if gesamt == 0 {
		return 0
	}
	return betrag / gesamt * 100
}

// heizungSchluessel describes the Heizungs-Gewichtung as text, e.g.
// "70 % Wärmeverbrauch, 30 % Wohnfläche".
func heizungSchluessel(gewichtung float64) string {
	waerme := int(math.Round(gewichtung * 100))
	return fmt.Sprintf("%d %% Wärmeverbrauch, %d %% Wohnfläche", waerme, 100-waerme)
}

// verbrauchUebersicht lists every meter with its Zählerstand at the start
// and end of the period. The start is the stand of the Ablesung before the
// period's first one - or, in the first Erfassungsjahr, the stand of that
// first Ablesung itself (the Ausgangsstand). The end is the stand of the
// period's last Ablesung. Teilstände are ignored.
func verbrauchUebersicht(periods []*store.LatestPeriod, apartments []store.Apartment, meters []store.Meter, imZeitraum map[string]bool) []abrechnungVerbrauchZeile {
	var complete []*store.LatestPeriod
	for _, p := range periods {
		if !newTeilstandStatus(p, apartments).IstTeilstand {
			complete = append(complete, p)
		}
	}
	sort.SliceStable(complete, func(i, j int) bool { return complete[i].ReadingDate < complete[j].ReadingDate })

	first, last := -1, -1
	for i, p := range complete {
		if imZeitraum[p.Monat] {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	if first == -1 {
		return nil
	}
	beginn := complete[first]
	if first > 0 {
		beginn = complete[first-1]
	}
	ende := complete[last]

	out := make([]abrechnungVerbrauchZeile, 0, len(meters))
	for _, m := range meters {
		b, e := beginn.Readings[m.Key], ende.Readings[m.Key]
		out = append(out, abrechnungVerbrauchZeile{Zaehler: m.Label, Einheit: m.Unit, Beginn: b, Ende: e, Verbrauch: e - b})
	}
	return out
}

// personenMonate lists the occupant count of every month from both sources:
// the Fixkosten-Eingabe (for the Schlüssel "Personen") and the Ablesung (for
// the warm water split). With several Ablesungen in a month the latest one
// is shown.
func personenMonate(monate []string, eingabeJeMonat map[string]*store.FixkostenEingabeDetails, periods []*store.LatestPeriod, apartments []store.Apartment) []abrechnungPersonenMonat {
	ablesung := map[string]map[int64]int64{}
	spaetestes := map[string]string{}
	for _, p := range periods {
		if newTeilstandStatus(p, apartments).IstTeilstand {
			continue
		}
		if prev, ok := spaetestes[p.Monat]; !ok || p.ReadingDate >= prev {
			spaetestes[p.Monat] = p.ReadingDate
			ablesung[p.Monat] = p.PersonenByApartment
		}
	}

	out := make([]abrechnungPersonenMonat, 0, len(monate))
	for _, m := range monate {
		row := abrechnungPersonenMonat{Monat: m, Ablesung: ablesung[m]}
		if e := eingabeJeMonat[m]; e != nil {
			row.Fixkosten = e.Personen
		}
		out = append(out, row)
	}
	return out
}
