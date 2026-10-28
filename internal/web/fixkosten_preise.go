package web

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkostenPreis describes one of the four prices of a Fixkosten-Eingabe
// (#359). Name is the form field and the CSV column (the old Ablesung CSV
// column names), Label the row label, Fehler the short name in messages.
type fixkostenPreis struct {
	Name, Label, Fehler, Einheit string
	feld                         store.Feldart
	eingabe                      func(*store.FixkostenInput) **float64
	detail                       func(*store.FixkostenEingabeDetails) *float64
}

// fixkostenPreise is the ordered list of the four prices, in the order of the
// form, the detail and the CSV.
var fixkostenPreise = []fixkostenPreis{
	{
		Name: "strompreis", Label: "Strompreis", Fehler: "Strom", Einheit: "EUR/kWh", feld: store.FeldStrompreis,
		eingabe: func(in *store.FixkostenInput) **float64 { return &in.Strompreis },
		detail:  func(f *store.FixkostenEingabeDetails) *float64 { return f.Strompreis },
	},
	{
		Name: "frischwasser_preis", Label: "Frischwasser", Fehler: "Frischwasser", Einheit: "EUR/m³", feld: store.FeldFrischwasserPreis,
		eingabe: func(in *store.FixkostenInput) **float64 { return &in.FrischwasserPreis },
		detail:  func(f *store.FixkostenEingabeDetails) *float64 { return f.FrischwasserPreis },
	},
	{
		Name: "abwasser_preis", Label: "Abwasser", Fehler: "Abwasser", Einheit: "EUR/m³", feld: store.FeldAbwasserPreis,
		eingabe: func(in *store.FixkostenInput) **float64 { return &in.AbwasserPreis },
		detail:  func(f *store.FixkostenEingabeDetails) *float64 { return f.AbwasserPreis },
	},
	{
		Name: "einspeisung_preis", Label: "Einspeisung", Fehler: "Einspeisung", Einheit: "EUR/kWh", feld: store.FeldEinspeisungPreis,
		eingabe: func(in *store.FixkostenInput) **float64 { return &in.EinspeisungPreis },
		detail:  func(f *store.FixkostenEingabeDetails) *float64 { return f.EinspeisungPreis },
	},
}

// fixkostenPreisFuerFeld returns the price a store field belongs to.
func fixkostenPreisFuerFeld(feld store.Feldart) (fixkostenPreis, bool) {
	for _, p := range fixkostenPreise {
		if p.feld == feld {
			return p, true
		}
	}
	return fixkostenPreis{}, false
}

// fixkostenSwitchHinweis explains a switch in the form, by Switch.Name.
var fixkostenSwitchHinweis = map[string]string{
	"WallboxKomplettW1": "Alle Wallbox-Kosten (kWh und EUR) dieses Monats gehen an Wohnung 1, die Einzelzähler werden dann nicht verwendet.",
	"PVGutschriftW1":    "Die Einspeisevergütung dieses Monats wird Wohnung 1 gutgeschrieben und erhöht deren Guthaben.",
}

// fixkostenPreisFeld is a price input of the form, Wert ready for value=.
type fixkostenPreisFeld struct {
	Name, Label, Wert string
}

// fixkostenHaekchenFeld is a switch checkbox of the form.
type fixkostenHaekchenFeld struct {
	Name, Label, Hinweis string
	An                   bool
}

// buildFixkostenPreisFelder makes the form group from the Eingabe to prefill
// from (nil on the very first Eingabe: empty prices, switches off). A price
// that is not recorded stays empty, so the required field asks for it.
func buildFixkostenPreisFelder(vorlage *store.FixkostenEingabeDetails) ([]fixkostenPreisFeld, []fixkostenHaekchenFeld) {
	preise := make([]fixkostenPreisFeld, 0, len(fixkostenPreise))
	for _, p := range fixkostenPreise {
		f := fixkostenPreisFeld{Name: p.Name, Label: p.Label + " (" + p.Einheit + ")"}
		if vorlage != nil {
			if v := p.detail(vorlage); v != nil {
				f.Wert = inputNumber(*v)
			}
		}
		preise = append(preise, f)
	}
	haekchen := make([]fixkostenHaekchenFeld, 0, len(store.Switches))
	for _, s := range store.Switches {
		h := fixkostenHaekchenFeld{Name: s.FormName, Label: s.Label, Hinweis: fixkostenSwitchHinweis[s.Name]}
		if vorlage != nil {
			h.An = *s.FixkostenDetail(vorlage)
		}
		haekchen = append(haekchen, h)
	}
	return preise, haekchen
}

var (
	errPreisFehlt     = errors.New("Preis fehlt")
	errPreisKeineZahl = errors.New("Preis muss eine Zahl sein")
)

// preisFormularFehler words a price problem for the form.
func preisFormularFehler(p fixkostenPreis, grund error) error {
	return fmt.Errorf("ungültiger Preis für %s: %w", p.Fehler, grund)
}

// parseFixkostenPreise reads the four required prices and the switches of a
// submitted form into in. A price is required (0 is allowed). Whether it is
// finite and not negative is checked by FixkostenInput.Validate.
func parseFixkostenPreise(formValue func(string) string, in *store.FixkostenInput) error {
	for _, p := range fixkostenPreise {
		raw := strings.TrimSpace(formValue(p.Name))
		if raw == "" {
			return preisFormularFehler(p, errPreisFehlt)
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return preisFormularFehler(p, errPreisKeineZahl)
		}
		*p.eingabe(in) = &v
	}
	// An unchecked checkbox sends no field: absent means off.
	for _, s := range store.Switches {
		*s.FixkostenInput(in) = formValue(s.FormName) != ""
	}
	return nil
}

// fixkostenDetailPreis is a price row of the detail view.
type fixkostenDetailPreis struct {
	Label, Einheit string
	Erfasst        bool
	Wert           float64
}

// fixkostenDetailHaekchen is a switch row of the detail view.
type fixkostenDetailHaekchen struct {
	Label string
	An    bool
}

func buildFixkostenDetailPreise(f *store.FixkostenEingabeDetails) ([]fixkostenDetailPreis, []fixkostenDetailHaekchen) {
	preise := make([]fixkostenDetailPreis, 0, len(fixkostenPreise))
	for _, p := range fixkostenPreise {
		row := fixkostenDetailPreis{Label: p.Label, Einheit: p.Einheit}
		if v := p.detail(f); v != nil {
			row.Erfasst, row.Wert = true, *v
		}
		preise = append(preise, row)
	}
	haekchen := make([]fixkostenDetailHaekchen, 0, len(store.Switches))
	for _, s := range store.Switches {
		haekchen = append(haekchen, fixkostenDetailHaekchen{Label: s.Label, An: *s.FixkostenDetail(f)})
	}
	return preise, haekchen
}

// fixkostenCsvPreisSpalten are the six columns after the Miete: the four prices
// and the switches, named like the old Ablesung CSV columns. An import may
// lack them, the export always writes them.
var fixkostenCsvPreisSpalten = buildFixkostenCsvPreisSpalten()

func buildFixkostenCsvPreisSpalten() []string {
	var cols []string
	for _, p := range fixkostenPreise {
		cols = append(cols, p.Name)
	}
	for _, s := range store.Switches {
		cols = append(cols, s.CSVColumn)
	}
	return cols
}

// formatPreisCSV renders a price cell, "" for a price that is not recorded.
// It keeps up to 4 decimals, so a re-import gives the same price.
func formatPreisCSV(v *float64) string {
	if v == nil {
		return ""
	}
	return strings.ReplaceAll(strconv.FormatFloat(math.Round(*v*1e4)/1e4, 'f', -1, 64), ".", ",")
}

// formatFixkostenPreisCSVZellen returns the six cells of formatFixkostenCSVRow.
func formatFixkostenPreisCSVZellen(f *store.FixkostenEingabeDetails) []string {
	var cells []string
	for _, p := range fixkostenPreise {
		cells = append(cells, formatPreisCSV(p.detail(f)))
	}
	for _, s := range store.Switches {
		cells = append(cells, formatSwitchCSV(*s.FixkostenDetail(f)))
	}
	return cells
}

// parseFixkostenPreisCSV reads the prices and switches of one CSV row into in.
// cell gives "" for a column the file lacks.
func parseFixkostenPreisCSV(record []string, colIdx map[string]int, cell func(string) string, line int, in *store.FixkostenInput) error {
	for _, p := range fixkostenPreise {
		v, ok, err := parseErfasst(cell(p.Name))
		if err != nil {
			return fmt.Errorf("Zeile %d: ungültiger Preis für %s: %q", line, p.Fehler, cell(p.Name))
		}
		if ok {
			*p.eingabe(in) = &v
		}
	}
	for _, s := range store.Switches {
		on, err := parseSwitchCSV(record, colIdx, s, line)
		if err != nil {
			return err
		}
		*s.FixkostenInput(in) = on
	}
	return nil
}
