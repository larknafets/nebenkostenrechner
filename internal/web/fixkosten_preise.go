package web

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

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
	preise := make([]fixkostenPreisFeld, 0, len(store.Preise))
	for _, p := range store.Preise {
		f := fixkostenPreisFeld{Name: p.Spalte, Label: p.Label + " (" + p.Einheit + ")"}
		if vorlage != nil {
			if v := *p.FixkostenDetail(vorlage); v != nil {
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
func preisFormularFehler(p store.Preis, grund error) error {
	return fmt.Errorf("ungültiger Preis für %s: %w", p.Kurz, grund)
}

// parseFixkostenPreise reads the four required prices and the switches of a
// submitted form into in. A price is required (0 is allowed). Whether it is
// finite and not negative is checked by FixkostenInput.Validate.
func parseFixkostenPreise(formValue func(string) string, in *store.FixkostenInput) error {
	for _, p := range store.Preise {
		raw := strings.TrimSpace(formValue(p.Spalte))
		if raw == "" {
			return preisFormularFehler(p, errPreisFehlt)
		}
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return preisFormularFehler(p, errPreisKeineZahl)
		}
		*p.FixkostenInput(in) = &v
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
	preise := make([]fixkostenDetailPreis, 0, len(store.Preise))
	for _, p := range store.Preise {
		row := fixkostenDetailPreis{Label: p.Label, Einheit: p.Einheit}
		if v := *p.FixkostenDetail(f); v != nil {
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
	for _, p := range store.Preise {
		cols = append(cols, p.Spalte)
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
	for _, p := range store.Preise {
		cells = append(cells, formatPreisCSV(*p.FixkostenDetail(f)))
	}
	for _, s := range store.Switches {
		cells = append(cells, formatSwitchCSV(*s.FixkostenDetail(f)))
	}
	return cells
}

// parseFixkostenPreisCSV reads the prices and switches of one CSV row into in.
// cell gives "" for a column the file lacks.
func parseFixkostenPreisCSV(record []string, colIdx map[string]int, cell func(string) string, line int, in *store.FixkostenInput) error {
	for _, p := range store.Preise {
		v, ok, err := parseErfasst(cell(p.Spalte))
		if err != nil {
			return fmt.Errorf("Zeile %d: ungültiger Preis für %s: %q", line, p.Kurz, cell(p.Spalte))
		}
		if ok {
			*p.FixkostenInput(in) = &v
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
