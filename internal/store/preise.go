package store

// Preis is one of the four prices of a Fixkosten-Eingabe (#359). Preise is
// the single place a price is written down: the fixkosten_eingaben column, its
// INSERT, UPDATE and scan, the form field, the CSV column, the validation,
// PreiseVollstaendig and the messages all derive from it.
//
// Adding a price means: one entry in Preise, a Feldart, the typed field on
// FixkostenInput and FixkostenEingabeDetails (read by calc), the column in
// schema.sql and a migration step.
type Preis struct {
	// Spalte is the REAL column of fixkosten_eingaben, the form field name and
	// the CSV column in one.
	Spalte string
	// Label is the row label, Kurz the short name in messages.
	Label, Kurz string
	Einheit     string
	// Feld is the Feldart an EingabeFehler reports for this price.
	Feld Feldart

	fixkostenInput  func(*FixkostenInput) **float64
	fixkostenDetail func(*FixkostenEingabeDetails) **float64
}

// FixkostenInput returns a pointer to the price's field on in.
func (p Preis) FixkostenInput(in *FixkostenInput) **float64 { return p.fixkostenInput(in) }

// FixkostenDetail returns a pointer to the price's field on f.
func (p Preis) FixkostenDetail(f *FixkostenEingabeDetails) **float64 { return p.fixkostenDetail(f) }

// Preise is the ordered list of the four prices, in the order of the form,
// the detail view and the CSV.
var Preise = []Preis{
	{
		Spalte: "strompreis", Label: "Strompreis", Kurz: "Strom", Einheit: "EUR/kWh", Feld: FeldStrompreis,
		fixkostenInput:  func(in *FixkostenInput) **float64 { return &in.Strompreis },
		fixkostenDetail: func(f *FixkostenEingabeDetails) **float64 { return &f.Strompreis },
	},
	{
		Spalte: "frischwasser_preis", Label: "Frischwasser", Kurz: "Frischwasser", Einheit: "EUR/m³", Feld: FeldFrischwasserPreis,
		fixkostenInput:  func(in *FixkostenInput) **float64 { return &in.FrischwasserPreis },
		fixkostenDetail: func(f *FixkostenEingabeDetails) **float64 { return &f.FrischwasserPreis },
	},
	{
		Spalte: "abwasser_preis", Label: "Abwasser", Kurz: "Abwasser", Einheit: "EUR/m³", Feld: FeldAbwasserPreis,
		fixkostenInput:  func(in *FixkostenInput) **float64 { return &in.AbwasserPreis },
		fixkostenDetail: func(f *FixkostenEingabeDetails) **float64 { return &f.AbwasserPreis },
	},
	{
		Spalte: "einspeisung_preis", Label: "Einspeisung", Kurz: "Einspeisung", Einheit: "EUR/kWh", Feld: FeldEinspeisungPreis,
		fixkostenInput:  func(in *FixkostenInput) **float64 { return &in.EinspeisungPreis },
		fixkostenDetail: func(f *FixkostenEingabeDetails) **float64 { return &f.EinspeisungPreis },
	},
}

// PreisFuerFeld returns the price a Feldart belongs to.
func PreisFuerFeld(feld Feldart) (Preis, bool) {
	for _, p := range Preise {
		if p.Feld == feld {
			return p, true
		}
	}
	return Preis{}, false
}
