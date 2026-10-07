package web

import (
	"database/sql"
	"fmt"
	"math"
	"sort"
	"time"

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
	// Geteilt is true if the position has several lines because its
	// Verteilerschlüssel changed during the period.
	Geteilt bool
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
	// Eigen is true for a meter of the settled apartment, shown in bold.
	Eigen bool
}

// abrechnungZaehlerSpalte is one meter column of the Zählerstände table.
type abrechnungZaehlerSpalte struct {
	Zaehler, Einheit string
	// Eigen is true for a meter of the settled apartment, shown in bold.
	Eigen bool
}

// abrechnungZaehlerstandZeile is one Ablesung of the Zählerstände table.
type abrechnungZaehlerstandZeile struct {
	// Ablesedatum is the reading date ("YYYY-MM-DD"), Monat the
	// Abrechnungsmonat ("YYYY-MM-01") the Ablesung is assigned to.
	Ablesedatum, Monat string
	// Ausgangsstand is true for the first row, the start of the period.
	Ausgangsstand bool
	// Staende are the Zählerstände in the order of the columns.
	Staende []abrechnungZaehlerstand
}

// abrechnungZaehlerstand is one Zählerstand of a row, Eigen if its meter
// belongs to the settled apartment (shown in bold).
type abrechnungZaehlerstand struct {
	Wert  float64
	Eigen bool
}

// abrechnungZaehlerstaende is the Zählerstände table (Anlage 7).
type abrechnungZaehlerstaende struct {
	Spalten []abrechnungZaehlerSpalte
	Zeilen  []abrechnungZaehlerstandZeile
}

// abrechnungBezugsgroesse is one row of the Bezugsgrößen table: the
// reference value of a Verteilerschlüssel per apartment and in total.
type abrechnungBezugsgroesse struct {
	Schluessel     string
	W1, W2, Gesamt float64
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

// abrechnungMonatZeile is one Abrechnungsmonat of the Monatsverlauf (Anlage
// 1): the month's costs of the settled apartment against its
// Nebenkostenabschlag, and the balance accumulated up to and including the
// month. All values are rounded to the cent.
type abrechnungMonatZeile struct {
	Monat string
	// Fixkosten are the umlagefähige Kostenpositionen, Verbrauch is
	// Heizung plus Wasser and, if passed on, the electricity of Wohnung 2.
	Fixkosten, Verbrauch, Abschlag float64
	// Saldo is the running balance (Abschlag minus costs) from the first
	// month of the period; positive Guthaben, negative Nachzahlung.
	Saldo float64
}

// abrechnungMonatsverlauf is the Monatsverlauf with its sum row.
type abrechnungMonatsverlauf struct {
	Zeilen []abrechnungMonatZeile
	// Uebertrag is the balance carried in from the months before the period,
	// the running balance starts with it.
	Uebertrag abrechnungUebertrag
	// Fixkosten, Verbrauch and Abschlag are the column sums of the period,
	// Endsaldo the balance of the last month (including the Übertrag).
	Fixkosten, Verbrauch, Abschlag, Endsaldo float64
}

// abrechnungUebertrag is the "Übertrag Vorjahre" of the Monatsverlauf: the
// balance (Abschlag minus costs) of the months from the start month (the
// later of "Mieter seit" and the start of the recording) up to the month
// before the period.
type abrechnungUebertrag struct {
	// Vorhanden is true if the table shows the row. It does not if there is
	// nothing to carry in (no month before the period, e.g. the first
	// Erfassungsjahr or a move-in at the start of the period) or the carry-
	// over cannot be calculated.
	Vorhanden bool
	Betrag    float64
	// Von/Bis are the first and last month with data that are summed
	// ("YYYY-MM-01").
	Von, Bis string
	// Hinweis explains why there is no Übertrag although there could be one:
	// a Mangel in a month before the period, or a "Mieter seit" after the
	// start of the period.
	Hinweis string
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
	Monatsverlauf  abrechnungMonatsverlauf
	Zaehlerstaende abrechnungZaehlerstaende
	Verbrauch      []abrechnungVerbrauchZeile
	HeizungMonate  []abrechnungHeizungMonat
	PersonenMonate []abrechnungPersonenMonat
	Bezugsgroessen []abrechnungBezugsgroesse
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
// calc.Fixkosten per Fixkosten-Eingabe and the Verbrauchskosten per Ablesung and
// only sums them, there is no second cost formula.
func berechneAbrechnung(db *sql.DB, d abrechnungDaten, jahr int, bereich monatsbereich, apartmentID int64) (abrechnungErgebnis, error) {
	pruefung, err := pruefeAbrechnungDaten(d.Pruef, jahr, bereich, apartmentID)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	if !pruefung.Abrechenbar() {
		return abrechnungErgebnis{Pruefung: pruefung}, nil
	}

	periods, apartments, haus := d.Pruef.Periods, d.Pruef.Apartments, d.Pruef.Haus
	kostenpositionen, eingaben, meters := d.Kostenpositionen, d.Eingaben, d.Meters

	var apartment store.Apartment
	for _, a := range apartments {
		if a.ID == apartmentID {
			apartment = a
		}
	}

	z := *pruefung.Zeitraum
	var monate []string // the Abrechnungsmonate of the period, oldest first
	for m := z.ErsterMonat; !m.After(z.LetzterMonat); m = m.AddDate(0, 1, 0) {
		monate = append(monate, m.Format("2006-01-02"))
	}
	imZeitraum := make(map[string]bool, len(monate))
	for _, m := range monate {
		imZeitraum[m] = true
	}

	ab := &abrechnung{Jahr: jahr, Apartment: apartment, Zeitraum: z, Gewichtung: haus.HeizungWaermeGewichtung, Apartments: apartments}

	// Fixkosten: one Eingabe per month (the Pruefung guarantees at most
	// one, and none only in the month of the first Ablesung).
	eingabeJeMonat, fixErgebnis, err := fixkostenJeMonat(db, eingaben, monate)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	for _, m := range monate {
		if e := eingabeJeMonat[m]; e != nil {
			ab.Vorauszahlungen += e.Abschlag[apartmentID]
		}
	}
	for _, kp := range kostenpositionen {
		if !kp.Umlagefaehig {
			continue
		}
		zeilen := fixkostenZeilen(kp, monate, fixErgebnis, apartmentID)
		for i := range zeilen {
			zeilen[i].Geteilt = len(zeilen) > 1
		}
		ab.Fixkosten = append(ab.Fixkosten, zeilen...)
	}

	// Verbrauch: every complete Ablesung of the period, except the Ausgangs-
	// stand of the first year, which has no predecessor and so no consumption.
	var heizungMonate []abrechnungHeizungMonat
	heizungIdx := map[string]int{}
	var strom abrechnungStrom
	stromWeiterberechnet := apartmentID == 2 && haus.StromWeiterberechnen
	verbrauchJeMonat := map[string]float64{}
	ab.Heizung = abrechnungZeile{Position: "Heizung und Warmwasser (Wärmepumpe)", Schluessel: heizungSchluessel(haus.HeizungWaermeGewichtung)}
	ab.Wasser = abrechnungZeile{Position: "Wasser und Abwasser (Verbrauch)", Schluessel: "Gemessener Verbrauch"}
	for _, a := range d.Kosten.Berechenbare() {
		p := a.Period
		if !imZeitraum[p.Monat] {
			continue
		}

		anteil := a.Kosten.Anteil(apartmentID)
		heizungGesamt, heizungAnteil := anteil.HeizungGesamt, anteil.HeizungKosten
		wasserGesamt, wasserAnteil := anteil.WasserGesamt, anteil.WasserKosten
		strom.VerbrauchKWh += anteil.StromKWh
		strom.Kosten += anteil.StromKosten
		verbrauchJeMonat[p.Monat] += monatsVerbrauch(anteil, stromWeiterberechnet)
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
		heizungMonate[i].WPStromKWh += a.Kosten.Heizung.WPVerbrauchW1KWh + a.Kosten.Heizung.WPVerbrauchW2KWh
		heizungMonate[i].WaermeW1MWh += a.Kosten.Heizung.WaermeW1MWh
		heizungMonate[i].WaermeW2MWh += a.Kosten.Heizung.WaermeW2MWh
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

	if stromWeiterberechnet {
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

	uebertrag, err := berechneUebertrag(db, d, z, apartment, stromWeiterberechnet)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	ab.Monatsverlauf = monatsverlauf(monate, kostenpositionen, eingabeJeMonat, fixErgebnis, verbrauchJeMonat, apartmentID, uebertrag.Betrag)
	ab.Monatsverlauf.Uebertrag = uebertrag
	ab.Verbrauch = verbrauchUebersicht(periods, apartments, meters, imZeitraum, apartmentID)
	ab.Zaehlerstaende = zaehlerstaende(periods, apartments, meters, imZeitraum, apartmentID)
	ab.Bezugsgroessen = bezugsgroessen(apartments)
	ab.PersonenMonate = personenMonate(monate, eingabeJeMonat, periods, apartments)

	return abrechnungErgebnis{Pruefung: pruefung, Abrechnung: ab}, nil
}

// fixkostenJeMonat returns the Fixkosten-Eingabe and its calculated result per
// Abrechnungsmonat for the given months (a month without an Eingabe is
// missing from both maps).
func fixkostenJeMonat(db *sql.DB, eingaben []*store.FixkostenEingabeDetails, monate []string) (map[string]*store.FixkostenEingabeDetails, map[string]*calc.FixkostenErgebnis, error) {
	imBereich := make(map[string]bool, len(monate))
	for _, m := range monate {
		imBereich[m] = true
	}
	eingabeJeMonat := map[string]*store.FixkostenEingabeDetails{}
	for _, e := range eingaben {
		if imBereich[e.Monat] {
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
			return nil, nil, fmt.Errorf("fixkosten %d: %w", e.ID, err)
		}
		fixErgebnis[m] = erg
	}
	return eingabeJeMonat, fixErgebnis, nil
}

// monatsVerbrauch is the consumption cost of one Ablesung for the Monats-
// verlauf: Heizung plus Wasser, plus the passed-on electricity of Wohnung 2.
func monatsVerbrauch(anteil calc.Wohnungsanteil, stromWeiterberechnet bool) float64 {
	v := anteil.HeizungKosten + anteil.WasserKosten
	if stromWeiterberechnet {
		v += anteil.StromKosten
	}
	return v
}

// berechneUebertrag calculates the "Übertrag Vorjahre" for the period z: the
// balance of all months from the start month up to the month before the
// period. The start month is the later of "Mieter seit" (only for a rented
// apartment) and the month of the first Ablesung. Months of the period itself
// are not part of it. If a month before the period has a Mangel of the
// Abrechnungsprüfung the carry-over cannot be calculated and the result says
// why instead of a wrong number.
func berechneUebertrag(db *sql.DB, d abrechnungDaten, z abrechnungZeitraum, apartment store.Apartment, stromWeiterberechnet bool) (abrechnungUebertrag, error) {
	periods := d.Pruef.Periods
	if len(periods) == 0 {
		return abrechnungUebertrag{}, nil
	}
	_, beginn, err := erfassungsbeginn(periods[0])
	if err != nil {
		return abrechnungUebertrag{}, nil // the Prüfung already reported it
	}
	start := beginn
	if apartment.Status == store.StatusVermietet && apartment.MieterSeit != "" {
		seit, err := time.Parse("2006-01-02", apartment.MieterSeit)
		if err == nil {
			if seit.After(z.ErsterMonat) {
				return abrechnungUebertrag{Hinweis: "Kein Übertrag: Mieter seit " + germanPeriodLabel(apartment.MieterSeit) + ", also nach dem Beginn dieses Zeitraums."}, nil
			}
			if seit.After(start) {
				start = seit
			}
		}
	}
	letzter := z.ErsterMonat.AddDate(0, -1, 0)
	if letzter.Before(start) {
		return abrechnungUebertrag{}, nil // nothing before the period
	}

	fixkostenVon := start
	if start.Equal(beginn) {
		fixkostenVon = beginn.AddDate(0, 1, 0) // the Ausgangsstand month needs no Fixkosten-Eingabe
	}
	maengel := pruefeMonate(d.Pruef, start, letzter, fixkostenVon)
	if len(maengel) > 0 {
		erster := maengel[0]
		for _, m := range maengel[1:] {
			if m.Monat < erster.Monat {
				erster = m
			}
		}
		return abrechnungUebertrag{Hinweis: "Übertrag nicht berechenbar: " + erster.Text}, nil
	}

	var monate []string
	imBereich := map[string]bool{}
	for m := start; !m.After(letzter); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		monate = append(monate, key)
		imBereich[key] = true
	}
	eingabeJeMonat, fixErgebnis, err := fixkostenJeMonat(db, d.Eingaben, monate)
	if err != nil {
		return abrechnungUebertrag{}, err
	}
	verbrauchJeMonat := map[string]float64{}
	for _, a := range d.Kosten.Berechenbare() {
		if !imBereich[a.Period.Monat] {
			continue
		}
		verbrauchJeMonat[a.Period.Monat] += monatsVerbrauch(a.Kosten.Anteil(apartment.ID), stromWeiterberechnet)
	}
	v := monatsverlauf(monate, d.Kostenpositionen, eingabeJeMonat, fixErgebnis, verbrauchJeMonat, apartment.ID, 0)
	if len(v.Zeilen) == 0 {
		return abrechnungUebertrag{}, nil // only the month of the Ausgangsstand, no costs yet
	}
	return abrechnungUebertrag{Vorhanden: true, Betrag: v.Endsaldo, Von: v.Zeilen[0].Monat, Bis: v.Zeilen[len(v.Zeilen)-1].Monat}, nil
}

// monatsverlauf builds the Monatsverlauf (Anlage 1), its balance starting at
// uebertrag, from the same monthly
// values the cost overview sums, so there is no second cost formula. A month
// with neither a Fixkosten-Eingabe nor a computable Ablesung (the month of
// the first Ablesung, which has no consumption) has no row.
func monatsverlauf(monate []string, kostenpositionen []store.Kostenposition, eingabeJeMonat map[string]*store.FixkostenEingabeDetails, fixErgebnis map[string]*calc.FixkostenErgebnis, verbrauchJeMonat map[string]float64, apartmentID int64, uebertrag float64) abrechnungMonatsverlauf {
	v := abrechnungMonatsverlauf{Endsaldo: uebertrag}
	for _, m := range monate {
		e := eingabeJeMonat[m]
		verbrauch, hatVerbrauch := verbrauchJeMonat[m]
		if e == nil && !hatVerbrauch {
			continue
		}
		z := abrechnungMonatZeile{Monat: m, Verbrauch: calc.Round2(verbrauch)}
		if e != nil {
			z.Abschlag = calc.Round2(e.Abschlag[apartmentID])
		}
		if erg := fixErgebnis[m]; erg != nil {
			for _, kp := range kostenpositionen {
				if !kp.Umlagefaehig {
					continue
				}
				for _, pos := range erg.Positionen {
					if pos.Key == kp.Key {
						z.Fixkosten += pos.KostenFor(apartmentID)
					}
				}
			}
			z.Fixkosten = calc.Round2(z.Fixkosten)
		}
		z.Saldo = calc.Round2(v.Endsaldo + z.Abschlag - z.Fixkosten - z.Verbrauch)
		v.Fixkosten += z.Fixkosten
		v.Verbrauch += z.Verbrauch
		v.Abschlag += z.Abschlag
		v.Endsaldo = z.Saldo
		v.Zeilen = append(v.Zeilen, z)
	}
	v.Fixkosten, v.Verbrauch, v.Abschlag = calc.Round2(v.Fixkosten), calc.Round2(v.Verbrauch), calc.Round2(v.Abschlag)
	return v
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

// ablesungenImZeitraum returns the complete Ablesungen (no Teilstände) oldest
// first and the index range [first, last] of those whose Abrechnungsmonat is
// in the period, first == -1 if there is none.
func ablesungenImZeitraum(periods []*store.LatestPeriod, apartments []store.Apartment, imZeitraum map[string]bool) (complete []*store.LatestPeriod, first, last int) {
	for _, p := range periods {
		if !p.Teilstand(apartments).IstTeilstand {
			complete = append(complete, p)
		}
	}
	sort.SliceStable(complete, func(i, j int) bool { return complete[i].ReadingDate < complete[j].ReadingDate })

	first, last = -1, -1
	for i, p := range complete {
		if imZeitraum[p.Monat] {
			if first == -1 {
				first = i
			}
			last = i
		}
	}
	return complete, first, last
}

// zaehlerstaende lists the Zählerstände of every complete Ablesung of the
// period (Anlage 7), one row per Ablesung with its date and Abrechnungsmonat.
// The first row is the Ausgangsstand: the Ablesung before the period's first
// one, or in the first Erfassungsjahr that first Ablesung itself, the same
// start the Verbrauchsübersicht uses. Several Ablesungen of one month are
// separate rows.
func zaehlerstaende(periods []*store.LatestPeriod, apartments []store.Apartment, meters []store.Meter, imZeitraum map[string]bool, apartmentID int64) abrechnungZaehlerstaende {
	complete, first, last := ablesungenImZeitraum(periods, apartments, imZeitraum)
	if first == -1 {
		return abrechnungZaehlerstaende{}
	}
	von := first
	if first > 0 {
		von = first - 1
	}
	var z abrechnungZaehlerstaende
	for _, m := range meters {
		unit := m.Unit
		if unit == "m3" {
			unit = "m³"
		}
		z.Spalten = append(z.Spalten, abrechnungZaehlerSpalte{Zaehler: zaehlerKurzname(m), Einheit: unit, Eigen: m.ApartmentID == apartmentID})
	}
	for i := von; i <= last; i++ {
		p := complete[i]
		zeile := abrechnungZaehlerstandZeile{Ablesedatum: p.ReadingDate, Monat: p.Monat, Ausgangsstand: i == von}
		for _, m := range meters {
			zeile.Staende = append(zeile.Staende, abrechnungZaehlerstand{Wert: p.Readings[m.Key], Eigen: m.ApartmentID == apartmentID})
		}
		z.Zeilen = append(z.Zeilen, zeile)
	}
	return z
}

// zaehlerKurznamen are the short column headings of the Zählerstände table,
// 10 columns plus the dates have to fit on a landscape page.
var zaehlerKurznamen = map[string]string{
	"strom_gesamt":                  "Strom gesamt",
	"strom_wohnung2":                "Strom Whg 2",
	"strom_waermepumpe":             "Strom WP",
	"strom_wallbox":                 "Wallboxen",
	"wasser_gesamt":                 "Wasser gesamt",
	"wasser_wohnung2":               "Wasser Whg 2",
	"wasser_warmwasseraufbereitung": "Warmwasser",
	"waerme_wohnung1":               "Wärme Whg 1",
	"waerme_wohnung2":               "Wärme Whg 2",
	"strom_einspeisung":             "Einspeisung (PV)",
}

func zaehlerKurzname(m store.Meter) string {
	if n, ok := zaehlerKurznamen[m.Key]; ok {
		return n
	}
	return m.Label
}

// verbrauchUebersicht lists every meter with its Zählerstand at the start
// and end of the period. The start is the stand of the Ablesung before the
// period's first one - or, in the first Erfassungsjahr, the stand of that
// first Ablesung itself (the Ausgangsstand). The end is the stand of the
// period's last Ablesung. Teilstände are ignored.
func verbrauchUebersicht(periods []*store.LatestPeriod, apartments []store.Apartment, meters []store.Meter, imZeitraum map[string]bool, apartmentID int64) []abrechnungVerbrauchZeile {
	complete, first, last := ablesungenImZeitraum(periods, apartments, imZeitraum)
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
		unit := m.Unit
		if unit == "m3" {
			unit = "m³"
		}
		out = append(out, abrechnungVerbrauchZeile{Zaehler: m.Label, Einheit: unit, Beginn: b, Ende: e, Verbrauch: e - b, Eigen: m.ApartmentID == apartmentID})
	}
	return out
}

// bezugsgroessen lists the Wohnfläche and Flurstück of both apartments with
// their totals - the reference values of the Verteilerschlüssel.
func bezugsgroessen(apartments []store.Apartment) []abrechnungBezugsgroesse {
	wohnflaeche := abrechnungBezugsgroesse{Schluessel: "Wohnfläche (m²)"}
	flurstueck := abrechnungBezugsgroesse{Schluessel: "Flurstück (m²)"}
	for _, a := range apartments {
		switch a.ID {
		case 1:
			wohnflaeche.W1, flurstueck.W1 = a.QM, a.FlurstueckGroesse
		case 2:
			wohnflaeche.W2, flurstueck.W2 = a.QM, a.FlurstueckGroesse
		}
	}
	wohnflaeche.Gesamt = wohnflaeche.W1 + wohnflaeche.W2
	flurstueck.Gesamt = flurstueck.W1 + flurstueck.W2
	return []abrechnungBezugsgroesse{wohnflaeche, flurstueck}
}

// personenMonate lists the occupant count of every month from both sources:
// the Fixkosten-Eingabe (for the Schlüssel "Personen") and the Ablesung (for
// the warm water split). With several Ablesungen in a month the latest one
// is shown.
func personenMonate(monate []string, eingabeJeMonat map[string]*store.FixkostenEingabeDetails, periods []*store.LatestPeriod, apartments []store.Apartment) []abrechnungPersonenMonat {
	ablesung := map[string]map[int64]int64{}
	spaetestes := map[string]string{}
	for _, p := range periods {
		if p.Teilstand(apartments).IstTeilstand {
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
