package abrechnung

// Berechnung of the Jahresabrechnung, Übertrag Vorjahre and Monatsverlauf.

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Zeile is one line of the cost overview: a position (or a block
// of consecutive months of it with the same Verteilerschlüssel) with the
// whole-house cost and the share of the settled apartment.
type Zeile struct {
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

// Strom is the household electricity of the settled apartment (#280), passed
// on by agreement when rented (not a Betriebskostenart).
type Strom struct {
	VerbrauchKWh, Kosten float64
}

// Wallbox is the Wallbox cost of the settled apartment (#228): its share of
// the Zwischenzähler "Wallboxen", a block of its own next to the Strom block.
type Wallbox struct {
	VerbrauchKWh, Kosten float64
}

// PVGutschrift is the credit of the PV system for Wohnung 1 (#228).
type PVGutschrift struct {
	Betrag float64
}

// VerbrauchZeile is one meter in the Anhang: the Zählerstand at
// the start and the end of the period and the consumption in between.
type VerbrauchZeile struct {
	Zaehler, Einheit        string
	Beginn, Ende, Verbrauch float64
	// BeginnFehlt/EndeFehlt are true for an optional Einzelzähler without a
	// reading at that end ("nicht erfasst", shown as "-"); the consumption is
	// then not calculable and shown as "-" too.
	BeginnFehlt, EndeFehlt bool
	// Eigen is true for a meter of the settled apartment, shown in bold.
	Eigen bool
}

// ZaehlerSpalte is one meter column of the Zählerstände table.
type ZaehlerSpalte struct {
	Zaehler, Einheit string
	// Eigen is true for a meter of the settled apartment, shown in bold.
	Eigen bool
}

// ZaehlerstandZeile is one Ablesung of the Zählerstände table.
type ZaehlerstandZeile struct {
	// Ablesedatum is the reading date ("YYYY-MM-DD"), Monat the
	// Abrechnungsmonat ("YYYY-MM-01") the Ablesung is assigned to.
	Ablesedatum, Monat string
	// Ausgangsstand is true for the first row, the start of the period.
	Ausgangsstand bool
	// Staende are the Zählerstände in the order of the columns.
	Staende []Zaehlerstand
}

// Zaehlerstand is one Zählerstand of a row, Eigen if its meter
// belongs to the settled apartment (shown in bold).
type Zaehlerstand struct {
	Wert  float64
	Eigen bool
	// Fehlt is true for an optional Einzelzähler without a reading in this
	// Ablesung, shown as "-".
	Fehlt bool
}

// Zaehlerstaende is the Zählerstände table (Anlage 7).
type Zaehlerstaende struct {
	Spalten []ZaehlerSpalte
	Zeilen  []ZaehlerstandZeile
}

// Bezugsgroesse is one row of the Bezugsgrößen table: the
// reference value of a Verteilerschlüssel per apartment and in total.
type Bezugsgroesse struct {
	Schluessel     string
	W1, W2, Gesamt float64
}

// HeizungMonat is one Abrechnungsmonat of the heating table in
// the Anhang.
type HeizungMonat struct {
	Monat string
	// WPStromKWh is the heat pump electricity of the whole house, Waerme*
	// the heat meter consumption per apartment (MWh), Anteil the settled
	// apartment's heating cost of the month.
	WPStromKWh, WaermeW1MWh, WaermeW2MWh, Anteil float64
}

// PersonenMonat is the occupant count of one month from its two
// independent sources. A nil map means the source has no value that month
// (e.g. no Fixkosten-Eingabe in the month of the first Ablesung).
type PersonenMonat struct {
	Monat     string
	Fixkosten map[int64]float64
	Ablesung  map[int64]float64
}

// MonatZeile is one Abrechnungsmonat of the Monatsverlauf (Anlage
// 1): the month's costs of the settled apartment against its
// Nebenkostenabschlag, and the balance accumulated up to and including the
// month. All values are rounded to the cent.
type MonatZeile struct {
	Monat string
	// Fixkosten are the umlagefähige Kostenpositionen, Verbrauch is
	// Heizung plus Wasser and, if passed on, the electricity of Wohnung 2.
	Fixkosten, Verbrauch, Abschlag float64
	// Gutschrift is the PV-Gutschrift of the month (#228), 0 if it is not
	// part of this Abrechnung.
	Gutschrift float64
	// Saldo is the running balance (Abschlag plus Gutschrift minus costs)
	// from the first month of the period; positive Guthaben, negative
	// Nachzahlung.
	Saldo float64
}

// Monatsverlauf is the Monatsverlauf with its sum row.
type Monatsverlauf struct {
	Zeilen []MonatZeile
	// Uebertrag is the balance carried in from the months before the period,
	// the running balance starts with it.
	Uebertrag Uebertrag
	// Fixkosten, Verbrauch and Abschlag are the column sums of the period,
	// Endsaldo the balance of the last month (including the Übertrag).
	Fixkosten, Verbrauch, Abschlag, Gutschrift, Endsaldo float64
}

// Uebertrag is the "Übertrag Vorjahre" of the Monatsverlauf: the
// balance (Abschlag minus costs) of the months from the start month (the
// later of "Mieter seit" and the start of the recording) up to the month
// before the period.
type Uebertrag struct {
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

// Jahresabrechnung is the computed Jahresabrechnung of one apartment and year.
type Jahresabrechnung struct {
	Jahr      int
	Apartment store.Apartment
	Zeitraum  Zeitraum

	// Fixkosten are the umlagefähige Kostenpositionen, Heizung and Wasser
	// the two consumption-based lines (zero Gesamt if there is no data).
	Fixkosten []Zeile
	Heizung   Zeile
	Wasser    Zeile
	// Betriebskosten is the sum of all three.
	Betriebskosten float64
	// Strom is nil for a rented apartment unless the Stammdaten flag
	// "Stromverbrauch weiterberechnen" is on (Eigennutzung always shows it).
	Strom *Strom
	// Wallbox is the apartment's Wallbox share (#228). It is nil unless the
	// Wohnungsstatus table shows it (Eigennutzung always, Vermietung only
	// with the flag "Wallbox weiterberechnen") and the costs are assigned to
	// the apartments in the period.
	Wallbox *Wallbox
	// PVGutschrift is nil unless Wohnung 1 is settled, in Eigennutzung, and
	// the period has a PV-Gutschrift.
	PVGutschrift *PVGutschrift
	// Vorauszahlungen is the sum of the Nebenkostenabschläge of the period
	// (a month without a recorded Abschlag counts as 0).
	Vorauszahlungen float64
	// Saldo is Vorauszahlungen minus Betriebskosten minus the passed-on
	// electricity minus Wallbox plus PV-Gutschrift, exactly the items shown:
	// positive Guthaben, negative Nachzahlung. Not the cumulative balance of
	// the Dashboard.
	Saldo *calc.AbschlagSaldo

	// Anhang data.
	Monatsverlauf  Monatsverlauf
	Zaehlerstaende Zaehlerstaende
	Verbrauch      []VerbrauchZeile
	HeizungMonate  []HeizungMonat
	PersonenMonate []PersonenMonat
	Bezugsgroessen []Bezugsgroesse
	Gewichtung     float64
	Apartments     []store.Apartment
}

// BerechnungsDaten is everything Berechne reads, all of it loaded by the
// caller: Berechne itself never touches a database.
type BerechnungsDaten struct {
	// Pruef is the subset the Prüfung and the Übertrag need.
	Pruef            Daten
	Kostenpositionen []store.Kostenposition
	Meters           []store.Meter
	// Fixkosten are the Fixkosten results of every Fixkosten-Eingabe.
	Fixkosten *calc.Fixkostenreihe
	// Kosten are the Verbrauchskosten of every Ablesung in Pruef.Periods.
	Kosten *calc.Verbrauchskosten
}

// Ergebnis is what the page needs: the check and, if it found
// nothing, the Abrechnung.
type Ergebnis struct {
	Pruefung Pruefung
	// Abrechnung is nil if the Pruefung found Mängel.
	Abrechnung *Jahresabrechnung
}

// Berechne checks whether jahr can be settled for apartmentID and,
// if so, computes the Jahresabrechnung (Issue #166). It reuses
// the Fixkostenreihe and the Verbrauchskosten per Ablesung and
// only sums them, there is no second cost formula. The Übertrag Vorjahre
// and the Jahresabrechnung read their months through the same fold
// (faltMonate), only the range differs.
func Berechne(d BerechnungsDaten, jahr int, bereich Monatsbereich, apartmentID int64) (Ergebnis, error) {
	pruefung, err := Pruefe(d.Pruef, jahr, bereich, apartmentID)
	if err != nil {
		return Ergebnis{}, err
	}
	if !pruefung.Abrechenbar() {
		return Ergebnis{Pruefung: pruefung}, nil
	}

	periods, apartments, haus := d.Pruef.Periods, d.Pruef.Apartments, d.Pruef.Haus
	kostenpositionen, meters := d.Kostenpositionen, d.Meters

	var apartment store.Apartment
	for _, a := range apartments {
		if a.ID == apartmentID {
			apartment = a
		}
	}

	z := *pruefung.Zeitraum
	p := postenFuer(apartment, haus)
	// The months of the period, oldest first.
	f := faltMonate(d, z.ErsterMonat, z.LetzterMonat, apartmentID, p)

	ab := &Jahresabrechnung{Jahr: jahr, Apartment: apartment, Zeitraum: z, Gewichtung: haus.HeizungWaermeGewichtung, Apartments: apartments}

	// Fixkosten: one Eingabe per month (the Pruefung guarantees at most
	// one, and none only in the month of the first Ablesung).
	for _, m := range f.monate {
		if fm, ok := f.fixkosten[m]; ok {
			ab.Vorauszahlungen += fm.Eingabe.Abschlag[apartmentID]
		}
	}
	for _, kp := range kostenpositionen {
		if !kp.Umlagefaehig {
			continue
		}
		zeilen := fixkostenZeilen(kp, f.monate, f.fixkosten, apartmentID)
		for i := range zeilen {
			zeilen[i].Geteilt = len(zeilen) > 1
		}
		ab.Fixkosten = append(ab.Fixkosten, zeilen...)
	}

	// Verbrauch: every complete Ablesung of the period, except the Ausgangs-
	// stand of the first year, which has no predecessor and so no consumption.
	var heizungMonate []HeizungMonat
	heizungIdx := map[string]int{}
	var strom Strom
	var wallbox Wallbox
	var wallboxVerteilt bool
	var pvGutschrift float64
	ab.Heizung = Zeile{Position: "Heizung und Warmwasser (Wärmepumpe)", Schluessel: heizungSchluessel(haus.HeizungWaermeGewichtung)}
	ab.Wasser = Zeile{Position: "Wasser und Abwasser (Verbrauch)", Schluessel: "Gemessener Verbrauch"}
	for _, a := range f.ablesungen {
		period := a.Period

		anteil := a.Kosten.Anteil(apartmentID)
		heizungGesamt, heizungAnteil := anteil.HeizungGesamt, anteil.HeizungKosten
		wasserGesamt, wasserAnteil := anteil.WasserGesamt, anteil.WasserKosten
		strom.VerbrauchKWh += anteil.StromKWh
		strom.Kosten += anteil.StromKosten
		wallbox.VerbrauchKWh += anteil.WallboxKWh
		wallbox.Kosten += anteil.WallboxKosten
		wallboxVerteilt = wallboxVerteilt || a.Kosten.Strom.WallboxVerteilt
		pvGutschrift += anteil.PVGutschrift
		ab.Heizung.Gesamt += heizungGesamt
		ab.Heizung.Betrag += heizungAnteil
		ab.Wasser.Gesamt += wasserGesamt
		ab.Wasser.Betrag += wasserAnteil
		ab.Heizung.Von, ab.Heizung.Bis = vonBis(ab.Heizung.Von, ab.Heizung.Bis, period.Monat)
		ab.Wasser.Von, ab.Wasser.Bis = vonBis(ab.Wasser.Von, ab.Wasser.Bis, period.Monat)

		i, ok := heizungIdx[period.Monat]
		if !ok {
			i = len(heizungMonate)
			heizungIdx[period.Monat] = i
			heizungMonate = append(heizungMonate, HeizungMonat{Monat: period.Monat})
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
	for _, zeile := range []*Zeile{&ab.Heizung, &ab.Wasser} {
		zeile.Gesamt, zeile.Betrag = calc.Round2(zeile.Gesamt), calc.Round2(zeile.Betrag)
		zeile.Prozent = prozent(zeile.Betrag, zeile.Gesamt)
	}

	if p.strom {
		ab.Strom = &Strom{VerbrauchKWh: strom.VerbrauchKWh, Kosten: calc.Round2(strom.Kosten)}
	}
	if p.wallbox && wallboxVerteilt {
		ab.Wallbox = &Wallbox{VerbrauchKWh: wallbox.VerbrauchKWh, Kosten: calc.Round2(wallbox.Kosten)}
	}
	if p.pvGutschrift && pvGutschrift != 0 {
		ab.PVGutschrift = &PVGutschrift{Betrag: calc.Round2(pvGutschrift)}
	}

	ab.Betriebskosten = ab.Heizung.Betrag + ab.Wasser.Betrag
	for _, z := range ab.Fixkosten {
		ab.Betriebskosten += z.Betrag
	}
	ab.Betriebskosten = calc.Round2(ab.Betriebskosten)
	ab.Vorauszahlungen = calc.Round2(ab.Vorauszahlungen)
	// The Jahressaldo is one period of the same fold the Monatsverlauf uses.
	periode := calc.SaldoMonat{Abschlag: ab.Vorauszahlungen, Kosten: ab.Betriebskosten}
	if ab.Strom != nil {
		periode.Kosten += ab.Strom.Kosten
	}
	if ab.Wallbox != nil {
		periode.Kosten += ab.Wallbox.Kosten
	}
	if ab.PVGutschrift != nil {
		periode.Gutschrift = ab.PVGutschrift.Betrag
	}
	_, jahressaldo := calc.Saldoverlauf(0, []calc.SaldoMonat{periode})
	ab.Saldo = calc.NewAbschlagSaldo(jahressaldo)

	uebertrag := berechneUebertrag(d, z, apartment, p)
	ab.Monatsverlauf = f.verlauf(uebertrag.Betrag)
	ab.Monatsverlauf.Uebertrag = uebertrag
	ab.Verbrauch = verbrauchUebersicht(periods, apartments, meters, f.imBereich, apartmentID)
	ab.Zaehlerstaende = zaehlerstaende(periods, apartments, meters, f.imBereich, apartmentID)
	ab.Bezugsgroessen = bezugsgroessen(apartments)
	ab.PersonenMonate = personenMonate(f.monate, f.fixkosten, periods, apartments)

	return Ergebnis{Pruefung: pruefung, Abrechnung: ab}, nil
}

// monatsfaltung is the monthly fold shared by the Jahresabrechnung and the
// Übertrag Vorjahre: for a range of Abrechnungsmonate and one apartment, the
// Fixkosten-Eingabe and its result per month and the consumption cost per
// month. Everything else (cost overview, Monatsverlauf, Übertrag) is a sum
// over it.
type monatsfaltung struct {
	// monate are the Abrechnungsmonate of the range ("YYYY-MM-01"), oldest
	// first; imBereich is the same as a set.
	monate    []string
	imBereich map[string]bool
	// fixkosten is the Fixkosten-Eingabe and its result per month (a month
	// without an Eingabe is missing).
	fixkosten map[string]calc.FixkostenMonat
	// ablesungen are the Ablesungen of the range that have Verbrauchskosten,
	// oldest first. werte are the values per Abrechnungsmonat of the
	// Monatsverlauf (calc.Monatswerte), counting exactly the posten of the
	// Abrechnung.
	ablesungen []calc.Ablesung
	werte      []calc.Monatswert
}

// posten says which optional items count for the settled apartment: the
// passed-on electricity of Wohnung 2, the Wallbox block and the PV-Gutschrift
// of Wohnung 1. The Jahressaldo, the Monatsverlauf and the Übertrag Vorjahre
// all read the same posten, so they count exactly the items the Abrechnung
// shows (#228).
type posten struct {
	strom, wallbox, pvGutschrift bool
}

// postenFuer applies the table of the Wohnungsstatus. Eigennutzung: Strom and
// Wallbox always (the flags do not apply) and the PV-Gutschrift (Wohnung 1
// only, the Gutschrift follows its assignment to Wohnung 1). Vermietung: Strom
// only with the flag "Stromverbrauch weiterberechnen", Wallbox only with
// "Wallbox weiterberechnen", never the PV-Gutschrift. The Strom flag applies
// to Wohnung 1 and 2 alike.
func postenFuer(apartment store.Apartment, haus store.Haus) posten {
	vermietet := apartment.Status == store.StatusVermietet
	return posten{
		strom:        !vermietet || haus.StromWeiterberechnen,
		wallbox:      !vermietet || haus.WallboxWeiterberechnen,
		pvGutschrift: apartment.ID == 1 && !vermietet,
	}
}

// faltMonate folds the months von to bis (first of month, both included) for
// apartmentID.
func faltMonate(d BerechnungsDaten, von, bis time.Time, apartmentID int64, p posten) monatsfaltung {
	f := monatsfaltung{imBereich: map[string]bool{}}
	for m := von; !m.After(bis); m = m.AddDate(0, 1, 0) {
		key := m.Format("2006-01-02")
		f.monate = append(f.monate, key)
		f.imBereich[key] = true
	}
	f.fixkosten = fixkostenJeMonat(d.Fixkosten, f.monate)
	var kosten []calc.MonatsKosten
	for _, a := range d.Kosten.Berechenbare() {
		if !f.imBereich[a.Period.Monat] {
			continue
		}
		f.ablesungen = append(f.ablesungen, a)
		kosten = append(kosten, calc.MonatsKosten{Monat: a.Period.Monat, Kosten: a.Kosten})
	}
	var fixkosten []calc.MonatsFixkosten
	for _, m := range f.monate {
		if fm, ok := f.fixkosten[m]; ok {
			fixkosten = append(fixkosten, calc.MonatsFixkosten{Monat: m, Ergebnis: fm.Ergebnis, Abschlag: fm.Eingabe.Abschlag})
		}
	}
	umlagefaehig := map[string]bool{}
	for _, kp := range d.Kostenpositionen {
		umlagefaehig[kp.Key] = kp.Umlagefaehig
	}
	f.werte = calc.Monatswerte(apartmentID, kosten, fixkosten, calc.Posten{
		Strom: p.strom, Wallbox: p.wallbox, PVGutschrift: p.pvGutschrift,
		Fixkosten: func(key string) bool { return umlagefaehig[key] },
	})
	return f
}

// fixkostenJeMonat returns the Fixkosten-Eingabe and its calculated result per
// Abrechnungsmonat for the given months (a month without an Eingabe is
// missing from the map).
func fixkostenJeMonat(reihe *calc.Fixkostenreihe, monate []string) map[string]calc.FixkostenMonat {
	out := map[string]calc.FixkostenMonat{}
	for _, m := range monate {
		if fm, ok := reihe.Monat(m); ok {
			out[m] = fm
		}
	}
	return out
}

// berechneUebertrag calculates the "Übertrag Vorjahre" for the period z: the
// balance of all months from the start month up to the month before the
// period. The start month is the later of "Mieter seit" (only for a rented
// apartment) and the month of the first Ablesung. Months of the period itself
// are not part of it. If a month before the period has a Mangel of the
// Abrechnungsprüfung the carry-over cannot be calculated and the result says
// why instead of a wrong number. The month range and the first month needing
// a Fixkosten-Eingabe follow the same rules as the Zeitraum of the Prüfung
// (Zeitraum.Vormonat, fixkostenBeginn).
func berechneUebertrag(d BerechnungsDaten, z Zeitraum, apartment store.Apartment, p posten) Uebertrag {
	periods := d.Pruef.Periods
	if len(periods) == 0 {
		return Uebertrag{}
	}
	_, beginn, err := Erfassungsbeginn(periods[0])
	if err != nil {
		return Uebertrag{} // the Prüfung already reported it
	}
	start := beginn
	if apartment.Status == store.StatusVermietet && apartment.MieterSeit != "" {
		seit, err := time.Parse("2006-01-02", apartment.MieterSeit)
		if err == nil {
			if seit.After(z.ErsterMonat) {
				return Uebertrag{Hinweis: "Kein Übertrag: Mieter seit " + store.MonatLabel(apartment.MieterSeit) + ", also nach dem Beginn dieses Zeitraums."}
			}
			if seit.After(start) {
				start = seit
			}
		}
	}
	letzter := z.Vormonat()
	if letzter.Before(start) {
		return Uebertrag{} // nothing before the period
	}

	maengel := PruefeMonate(d.Pruef, start, letzter, fixkostenBeginn(beginn, start))
	if len(maengel) > 0 {
		erster := maengel[0]
		for _, m := range maengel[1:] {
			if m.Monat < erster.Monat {
				erster = m
			}
		}
		return Uebertrag{Hinweis: "Übertrag nicht berechenbar: " + erster.Text}
	}

	v := faltMonate(d, start, letzter, apartment.ID, p).verlauf(0)
	if len(v.Zeilen) == 0 {
		return Uebertrag{} // only the month of the Ausgangsstand, no costs yet
	}
	return Uebertrag{Vorhanden: true, Betrag: v.Endsaldo, Von: v.Zeilen[0].Monat, Bis: v.Zeilen[len(v.Zeilen)-1].Monat}
}

// verlauf builds the Monatsverlauf (Anlage 1), its balance starting at
// uebertrag, from the same monthly values the cost overview sums, so there is
// no second cost formula. A month with neither a Fixkosten-Eingabe nor a
// computable Ablesung (the month of the first Ablesung, which has no
// consumption) has no row. The balance accumulates in calc.Verlauf.
func (f monatsfaltung) verlauf(uebertrag float64) Monatsverlauf {
	zeilen, endsaldo := calc.Verlauf(uebertrag, f.werte)
	v := Monatsverlauf{Endsaldo: endsaldo}
	for _, z := range zeilen {
		v.Zeilen = append(v.Zeilen, MonatZeile{Monat: z.Monat, Verbrauch: z.Verbrauch, Fixkosten: z.Fixkosten, Abschlag: z.Abschlag, Gutschrift: z.Gutschrift, Saldo: z.Saldo})
		v.Fixkosten += z.Fixkosten
		v.Verbrauch += z.Verbrauch
		v.Abschlag += z.Abschlag
		v.Gutschrift += z.Gutschrift
	}
	v.Fixkosten, v.Verbrauch, v.Abschlag, v.Gutschrift = calc.Round2(v.Fixkosten), calc.Round2(v.Verbrauch), calc.Round2(v.Abschlag), calc.Round2(v.Gutschrift)
	return v
}

// fixkostenZeilen builds the lines of one Kostenposition: consecutive months
// with the same Logik form one line (a change of Typ or Wert alone does not
// split it, the months' values are simply summed). A block in which the
// settled apartment pays nothing - e.g. a position the landlord allocated
// fully to the other apartment - is left out.
func fixkostenZeilen(kp store.Kostenposition, monate []string, erg map[string]calc.FixkostenMonat, apartmentID int64) []Zeile {
	var out []Zeile
	var cur *Zeile
	var curLogik string
	for _, m := range monate {
		fm, ok := erg[m]
		if !ok || fm.Ergebnis == nil {
			continue
		}
		e := fm.Ergebnis
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
			out = append(out, Zeile{Position: kp.Label, Von: m, Schluessel: calc.LogikLabels[pos.Logik]})
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
func zaehlerstaende(periods []*store.LatestPeriod, apartments []store.Apartment, meters []store.Meter, imZeitraum map[string]bool, apartmentID int64) Zaehlerstaende {
	complete, first, last := ablesungenImZeitraum(periods, apartments, imZeitraum)
	if first == -1 {
		return Zaehlerstaende{}
	}
	von := first
	if first > 0 {
		von = first - 1
	}
	var z Zaehlerstaende
	meters = mitEinzelzaehlern(meters, complete[von:last+1])
	for _, m := range meters {
		unit := m.Unit
		if unit == "m3" {
			unit = "m³"
		}
		z.Spalten = append(z.Spalten, ZaehlerSpalte{Zaehler: zaehlerKurzname(m), Einheit: unit, Eigen: m.ApartmentID == apartmentID})
	}
	for i := von; i <= last; i++ {
		p := complete[i]
		zeile := ZaehlerstandZeile{Ablesedatum: p.ReadingDate, Monat: p.Monat, Ausgangsstand: i == von}
		for _, m := range meters {
			wert, vorhanden := p.Readings[m.Key]
			zeile.Staende = append(zeile.Staende, Zaehlerstand{Wert: wert, Eigen: m.ApartmentID == apartmentID, Fehlt: !vorhanden && m.Optional})
		}
		z.Zeilen = append(z.Zeilen, zeile)
	}
	return z
}

// mitEinzelzaehlern lists the required meters and inserts the optional
// Einzelzähler (#224) directly after the Zwischenzähler "Wallboxen" (#228).
// meters holds both kinds (store.Meters), the Anlage filters here. An
// Einzelzähler without a single reading in rows (the period's Ablesungen) is
// left out, a missing reading in some of them stays visible as such. Without
// the Zwischenzähler no Einzelzähler is listed.
func mitEinzelzaehlern(meters []store.Meter, rows []*store.LatestPeriod) []store.Meter {
	var vorhanden []store.Meter
	for _, m := range meters {
		if !m.Optional {
			continue
		}
		for _, p := range rows {
			if _, ok := p.Readings[m.Key]; ok {
				vorhanden = append(vorhanden, m)
				break
			}
		}
	}
	out := make([]store.Meter, 0, len(meters))
	for _, m := range meters {
		if m.Optional {
			continue
		}
		out = append(out, m)
		if m.Key == "strom_wallbox" {
			out = append(out, vorhanden...)
		}
	}
	return out
}

// zaehlerKurzname is the short column heading of the Zählerstände table.
func zaehlerKurzname(m store.Meter) string {
	if m.Kurzname != "" {
		return m.Kurzname
	}
	return m.Label
}

// verbrauchUebersicht lists every meter with its Zählerstand at the start
// and end of the period. The start is the stand of the Ablesung before the
// period's first one - or, in the first Erfassungsjahr, the stand of that
// first Ablesung itself (the Ausgangsstand). The end is the stand of the
// period's last Ablesung. Teilstände are ignored.
func verbrauchUebersicht(periods []*store.LatestPeriod, apartments []store.Apartment, meters []store.Meter, imZeitraum map[string]bool, apartmentID int64) []VerbrauchZeile {
	complete, first, last := ablesungenImZeitraum(periods, apartments, imZeitraum)
	if first == -1 {
		return nil
	}
	beginn := complete[first]
	if first > 0 {
		beginn = complete[first-1]
	}
	ende := complete[last]

	von := first
	if first > 0 {
		von = first - 1
	}
	meters = mitEinzelzaehlern(meters, complete[von:last+1])
	out := make([]VerbrauchZeile, 0, len(meters))
	for _, m := range meters {
		b, okB := beginn.Readings[m.Key]
		e, okE := ende.Readings[m.Key]
		optional := m.Optional
		unit := m.Unit
		if unit == "m3" {
			unit = "m³"
		}
		verbrauch, _ := store.Verbrauch(ende.Readings, beginn.Readings, m.Key)
		out = append(out, VerbrauchZeile{
			Zaehler: m.Label, Einheit: unit, Beginn: b, Ende: e, Verbrauch: verbrauch,
			BeginnFehlt: optional && !okB, EndeFehlt: optional && !okE,
			Eigen: m.ApartmentID == apartmentID,
		})
	}
	return out
}

// bezugsgroessen lists the Wohnfläche and Flurstück of both apartments with
// their totals - the reference values of the Verteilerschlüssel.
func bezugsgroessen(apartments []store.Apartment) []Bezugsgroesse {
	wohnflaeche := Bezugsgroesse{Schluessel: "Wohnfläche (m²)"}
	flurstueck := Bezugsgroesse{Schluessel: "Flurstück (m²)"}
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
	return []Bezugsgroesse{wohnflaeche, flurstueck}
}

// personenMonate lists the occupant count of every month from both sources:
// the Fixkosten-Eingabe (for the Schlüssel "Personen") and the Ablesung (for
// the warm water split). With several Ablesungen in a month the latest one
// is shown.
func personenMonate(monate []string, fixkosten map[string]calc.FixkostenMonat, periods []*store.LatestPeriod, apartments []store.Apartment) []PersonenMonat {
	ablesung := map[string]map[int64]float64{}
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

	out := make([]PersonenMonat, 0, len(monate))
	for _, m := range monate {
		row := PersonenMonat{Monat: m, Ablesung: ablesung[m]}
		if fm, ok := fixkosten[m]; ok {
			row.Fixkosten = fm.Eingabe.Personen
		}
		out = append(out, row)
	}
	return out
}
