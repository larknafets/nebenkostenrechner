package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Switch is one per-Ablesung on/off switch (Schalter), e.g. "Wallboxen
// vollständig Wohnung 1" (#225). Switches is the single place a switch is
// written down: the periods column, its migration, INSERT, UPDATE and scan,
// the CSV column and the form field name all derive from it.
//
// Adding a switch means: one entry in Switches, the typed field on
// PeriodInput and LatestPeriod (read by calc), the two accessors below, the
// column in schema.sql and the checkbox in the wizard template.
//
// Overwrite rule (do not unify with the price handling): UpdatePeriod always
// overwrites every switch with the submitted value, because a switch can
// never be "nicht erfasst", false is a real answer (Default). Monat and the
// four prices are the opposite: they use COALESCE and keep the stored value
// when the round brings none, because they can be missing in a Teilstand
// (#128).
type Switch struct {
	// Name is the Go field name on PeriodInput/LatestPeriod, for messages.
	Name string
	// Column is the INTEGER column of the periods table.
	Column string
	// CSVColumn is the optional 0/1 column of the CSV export and import.
	CSVColumn string
	// FormName is the name of the checkbox in the Ablesung form.
	FormName string
	// Label is the German label of the switch.
	Label string
	// Default is the value of a missing field, CSV cell or an Ablesung that
	// predates the switch.
	Default bool

	input  func(*PeriodInput) *bool
	detail func(*LatestPeriod) *bool
}

// Input returns a pointer to the switch's field on in.
func (s Switch) Input(in *PeriodInput) *bool { return s.input(in) }

// Detail returns a pointer to the switch's field on p.
func (s Switch) Detail(p *LatestPeriod) *bool { return s.detail(p) }

// Switches is the ordered list of all Ablesung switches, as exported to the
// CSV after the optional meters.
var Switches = []Switch{
	{
		Name: "WallboxKomplettW1", Column: "wallbox_komplett_w1", CSVColumn: "wallbox_komplett_wohnung1",
		FormName: "wallbox_komplett_w1", Label: "Wallboxen vollständig Wohnung 1",
		input:  func(in *PeriodInput) *bool { return &in.WallboxKomplettW1 },
		detail: func(p *LatestPeriod) *bool { return &p.WallboxKomplettW1 },
	},
	{
		Name: "PVGutschriftW1", Column: "pv_gutschrift_w1", CSVColumn: "pv_gutschrift_wohnung1",
		FormName: "pv_gutschrift_w1", Label: "PV-Gutschrift Wohnung 1",
		input:  func(in *PeriodInput) *bool { return &in.PVGutschriftW1 },
		detail: func(p *LatestPeriod) *bool { return &p.PVGutschriftW1 },
	},
}

// switchColumns returns the switch columns in Switches order.
func switchColumns() []string {
	cols := make([]string, len(Switches))
	for i, s := range Switches {
		cols[i] = s.Column
	}
	return cols
}

// switchArgs returns the INSERT/UPDATE arguments of in's switches.
func switchArgs(in PeriodInput) []any {
	args := make([]any, len(Switches))
	for i, s := range Switches {
		args[i] = boolToInt(*s.Input(&in))
	}
	return args
}

// ensurePeriodSwitchColumns adds every switch column a periods table that
// predates it lacks. Existing Ablesungen get the switch's Default, so their
// costs and balances stay as they were. Runs after the periods rebuild of
// ensurePeriodsNullablePriceColumns, which would drop the columns again.
func ensurePeriodSwitchColumns(db *sql.DB) error {
	for _, s := range Switches {
		has, err := hasColumn(db, "periods", s.Column)
		if err != nil {
			return fmt.Errorf("migrate %s column: %w", s.Column, err)
		}
		if has {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf(
			`ALTER TABLE periods ADD COLUMN %s INTEGER NOT NULL DEFAULT %d`, s.Column, boolToInt(s.Default),
		)); err != nil {
			return fmt.Errorf("migrate %s column: %w", s.Column, err)
		}
	}
	return nil
}

// insertPeriodSQL is the INSERT of one period, switches included.
func insertPeriodSQL() string {
	cols := append([]string{"reading_date", "monat", "strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis"}, switchColumns()...)
	return `INSERT INTO periods (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders(len(cols)) + `)`
}

// updatePeriodSQL is the UPDATE of one period by id. Monat and the prices use
// COALESCE (Teilstand), the switches overwrite unconditionally, see Switch.
func updatePeriodSQL() string {
	sets := []string{
		"reading_date = ?",
		"monat = COALESCE(NULLIF(?, ''), monat)",
		"strompreis = COALESCE(?, strompreis)",
		"frischwasser_preis = COALESCE(?, frischwasser_preis)",
		"abwasser_preis = COALESCE(?, abwasser_preis)",
		"einspeisung_preis = COALESCE(?, einspeisung_preis)",
	}
	for _, c := range switchColumns() {
		sets = append(sets, c+" = ?")
	}
	return `UPDATE periods SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
}

// selectPeriodSQL selects the periods columns scanPeriodRow reads, followed
// by where (empty or a WHERE clause) and the ordering.
func selectPeriodSQL(where string) string {
	cols := append([]string{"id", "reading_date", "monat", "strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis"}, switchColumns()...)
	return `SELECT ` + strings.Join(cols, ", ") + ` FROM periods ` + where + ` ORDER BY reading_date ASC, id ASC`
}

// scanPeriodRow scans one row of selectPeriodSQL into a LatestPeriod.
func scanPeriodRow(rows *sql.Rows) (*LatestPeriod, error) {
	p := &LatestPeriod{
		Readings:            map[string]float64{},
		PersonenByApartment: map[int64]float64{},
	}
	flags := make([]int, len(Switches))
	dest := []any{&p.ID, &p.ReadingDate, &p.Monat, &p.Strompreis, &p.FrischwasserPreis, &p.AbwasserPreis, &p.EinspeisungPreis}
	for i := range flags {
		dest = append(dest, &flags[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, err
	}
	for i, s := range Switches {
		*s.Detail(p) = flags[i] != 0
	}
	return p, nil
}
