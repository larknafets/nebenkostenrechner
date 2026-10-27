package calc

import (
	"database/sql"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Kosten is the Verbrauchskosten of one calculable Ablesung: the four calc
// results, none of them nil.
type Kosten struct {
	Strom       *StromErgebnis
	Wasser      *WasserErgebnis
	Heizung     *HeizungErgebnis
	Einspeisung *EinspeisungErgebnis

	// PVGutschriftW1 is the Ertrag of this Ablesung when its switch "PV-Gutschrift
	// Wohnung 1" is on (#226), else 0. Not a cost position, only apartment 1
	// ever gets it.
	PVGutschriftW1 float64
}

// Wohnungsanteil is one apartment's share of an Ablesung's Verbrauchskosten
// together with the house totals - the one place that decides which of the
// calc results belongs to which apartment. Dashboard (kategorien) and
// Jahresabrechnung both read it.
//
// Both apartments carry a Strom position (HatStrom): apartment 2 its submeter
// share, apartment 1 the rest of the grid draw (see Strom, #279). Frischwasser
// and Abwasser are combined into one Wasser amount since they share one raw m³
// consumption (no separate wastewater meter, see Wasser).
type Wohnungsanteil struct {
	HatStrom      bool
	StromKosten   float64
	StromKWh      float64
	HeizungKosten float64
	HeizungGesamt float64 // both apartments
	HeizungWPKWh  float64 // this apartment's heat pump electricity
	HeizungMWh    float64 // this apartment's heat meter consumption
	WasserKosten  float64
	WasserGesamt  float64 // both apartments
	WasserM3      float64

	// WallboxKosten/WallboxKWh are this apartment's share of the Zwischenzähler
	// "Wallboxen" (#225), both 0 while the Wallbox costs are not assigned to
	// the apartments (StromErgebnis.WallboxVerteilt).
	WallboxKosten float64
	WallboxKWh    float64

	// PVGutschrift is a credit in EUR (#226), only ever set for apartment 1 and
	// only with the switch on. Kept apart from the costs: it lowers the balance,
	// not the consumption costs.
	PVGutschrift float64
	// HatPVGutschrift says that this apartment carries the PV-Gutschrift line
	// (apartment 1), whatever its value. Like HatStrom it does not depend on
	// the data.
	HatPVGutschrift bool

	// PVAnteilKWh is the part of this apartment's submeter consumption not
	// allocated to grid draw (PV), see StromErgebnis.PVAnteilW2KWh. Only set
	// for apartment 2: apartment 1 has no own electricity meter.
	PVAnteilKWh float64
	// HatPVAnteil says that this apartment carries the PV-Anteil line
	// (apartment 2), whatever its value.
	HatPVAnteil bool
}

// Anteil returns apartmentID's share of these Kosten.
func (k Kosten) Anteil(apartmentID int64) Wohnungsanteil {
	a := Wohnungsanteil{
		HeizungGesamt: Round2(k.Heizung.KostenHeizungW1 + k.Heizung.KostenHeizungW2),
		WasserGesamt:  Round2(k.Wasser.KostenFrischwasserW1 + k.Wasser.KostenAbwasserW1 + k.Wasser.KostenFrischwasserW2 + k.Wasser.KostenAbwasserW2),
	}
	if apartmentID == 2 {
		a.HatStrom, a.StromKosten, a.StromKWh = true, k.Strom.KostenW2, k.Strom.W2VerbrauchKWh
		a.HeizungKosten, a.HeizungWPKWh, a.HeizungMWh = k.Heizung.KostenHeizungW2, k.Heizung.WPVerbrauchW2KWh, k.Heizung.WaermeW2MWh
		a.WasserKosten, a.WasserM3 = Round2(k.Wasser.KostenFrischwasserW2+k.Wasser.KostenAbwasserW2), k.Wasser.FrischwasserW2
		a.WallboxKosten, a.WallboxKWh = k.Strom.KostenWallboxW2, k.Strom.WallboxW2KWh
		a.PVAnteilKWh, a.HatPVAnteil = k.Strom.PVAnteilW2KWh, true
		return a
	}
	a.HatStrom, a.StromKosten, a.StromKWh = true, k.Strom.KostenW1, k.Strom.W1RestKWh
	a.WallboxKosten, a.WallboxKWh = k.Strom.KostenWallboxW1, k.Strom.WallboxW1KWh
	a.PVGutschrift, a.HatPVGutschrift = k.PVGutschriftW1, true
	a.HeizungKosten, a.HeizungWPKWh, a.HeizungMWh = k.Heizung.KostenHeizungW1, k.Heizung.WPVerbrauchW1KWh, k.Heizung.WaermeW1MWh
	a.WasserKosten, a.WasserM3 = Round2(k.Wasser.KostenFrischwasserW1+k.Wasser.KostenAbwasserW1), k.Wasser.FrischwasserW1
	return a
}

// Grund says why an Ablesung has no Verbrauchskosten. GrundKeiner means it
// has.
type Grund int

const (
	GrundKeiner Grund = iota
	// GrundTeilstand: an incomplete reading does not flow into the
	// calculation (Ticket #129) - otherwise missing meter readings/prices
	// would silently be treated as 0 and show a wrong cost amount instead
	// of "not yet calculable".
	GrundTeilstand
	// GrundKeineVorperiode: the oldest Ablesung has no earlier one to
	// diff its Zählerstände against, so it has no Verbrauch.
	GrundKeineVorperiode
)

// Ablesung is one Ablesung together with its Verbrauchskosten. Kosten is
// only set when Grund is GrundKeiner.
type Ablesung struct {
	Period *store.LatestPeriod
	Kosten Kosten
	Grund  Grund
}

// Berechenbar reports whether the Ablesung has Verbrauchskosten.
func (a Ablesung) Berechenbar() bool { return a.Grund == GrundKeiner }

// Daten is everything the Verbrauchskosten read. Periods must be ordered
// oldest first by reading date, then id (as store.AllPeriodDetails
// returns them): the Vorperiode of an Ablesung is the one before it.
type Daten struct {
	Periods    []*store.LatestPeriod
	Apartments []store.Apartment
	Haus       store.Haus
}

// Verbrauchskosten holds the Verbrauchskosten of every Ablesung, computed
// once.
type Verbrauchskosten struct {
	alle []Ablesung
	byID map[int64]int
}

// Load reads the Datenstand and computes the Verbrauchskosten of it.
func Load(db *sql.DB) (*Verbrauchskosten, error) {
	ds, err := store.LadeDatenstand(db)
	if err != nil {
		return nil, err
	}
	return KostenAus(ds), nil
}

// KostenAus computes the Verbrauchskosten of an already loaded Datenstand.
func KostenAus(ds *store.Datenstand) *Verbrauchskosten {
	return New(Daten{Periods: ds.Periods, Apartments: ds.Apartments, Haus: ds.Haus})
}

// New computes the Verbrauchskosten of every Ablesung in d.
func New(d Daten) *Verbrauchskosten {
	v := &Verbrauchskosten{alle: make([]Ablesung, len(d.Periods)), byID: make(map[int64]int, len(d.Periods))}
	qmW1, qmW2 := apartmentValues(d.Apartments, func(a store.Apartment) float64 { return a.QM })
	for i, p := range d.Periods {
		v.byID[p.ID] = i
		v.alle[i] = Ablesung{Period: p}
		if p.Teilstand(d.Apartments).IstTeilstand {
			v.alle[i].Grund = GrundTeilstand
			continue
		}
		if i == 0 {
			v.alle[i].Grund = GrundKeineVorperiode
			continue
		}
		verbrauch := make(map[string]float64, len(store.MeterKeys))
		for _, key := range store.MeterKeys {
			verbrauch[key], _ = store.Verbrauch(p.Readings, d.Periods[i-1].Readings, key)
		}
		einzel := make(map[string]float64, len(store.OptionalMeterKeys))
		for _, key := range store.OptionalMeterKeys {
			if v, ok := store.Verbrauch(p.Readings, d.Periods[i-1].Readings, key); ok {
				einzel[key] = v
			}
		}
		strom := berechneStrom(stromEingabe{Strompreis: *p.Strompreis, Verbrauch: verbrauch, EinzelVerbrauch: einzel, WallboxKomplettW1: p.WallboxKomplettW1})
		v.alle[i].Kosten = Kosten{
			Strom: strom,
			Wasser: berechneWasser(wasserEingabe{
				FrischwasserPreis: *p.FrischwasserPreis, AbwasserPreis: *p.AbwasserPreis,
				Verbrauch: verbrauch, PersonenW1: p.PersonenByApartment[1], PersonenW2: p.PersonenByApartment[2],
			}),
			Heizung:     berechneHeizung(heizungEingabe{Strom: strom, WaermeGewichtung: d.Haus.HeizungWaermeGewichtung, Verbrauch: verbrauch, QMW1: qmW1, QMW2: qmW2}),
			Einspeisung: berechneEinspeisung(einspeisungEingabe{EinspeisungPreis: *p.EinspeisungPreis, Verbrauch: verbrauch}),
		}
		if p.PVGutschriftW1 {
			v.alle[i].Kosten.PVGutschriftW1 = v.alle[i].Kosten.Einspeisung.Ertrag
		}
	}
	return v
}

// Ablesung returns the Ablesung with the given period id.
func (v *Verbrauchskosten) Ablesung(periodID int64) (Ablesung, bool) {
	i, ok := v.byID[periodID]
	if !ok {
		return Ablesung{}, false
	}
	return v.alle[i], true
}

// Berechenbare returns the Ablesungen that have Verbrauchskosten, oldest
// first.
func (v *Verbrauchskosten) Berechenbare() []Ablesung {
	var out []Ablesung
	for _, a := range v.alle {
		if a.Berechenbar() {
			out = append(out, a)
		}
	}
	return out
}
