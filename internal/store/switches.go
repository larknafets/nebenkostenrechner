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
// UpdatePeriod does not write the switches any more (#359): they belong to the
// Fixkosten-Eingabe, the Ablesung side stays until the ticket that removes it.
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

	// The Fixkosten-Eingabe carries the same switches (#359): its column has
	// the same name as the periods column. The Ablesung side above stays
	// until the ticket that removes the Ablesung fields.
	fixkostenInput  func(*FixkostenInput) *bool
	fixkostenDetail func(*FixkostenEingabeDetails) *bool
}

// Input returns a pointer to the switch's field on in.
func (s Switch) Input(in *PeriodInput) *bool { return s.input(in) }

// Detail returns a pointer to the switch's field on p.
func (s Switch) Detail(p *LatestPeriod) *bool { return s.detail(p) }

// FixkostenInput returns a pointer to the switch's field on in.
func (s Switch) FixkostenInput(in *FixkostenInput) *bool { return s.fixkostenInput(in) }

// FixkostenDetail returns a pointer to the switch's field on f.
func (s Switch) FixkostenDetail(f *FixkostenEingabeDetails) *bool { return s.fixkostenDetail(f) }

// Switches is the ordered list of all Ablesung switches, as exported to the
// CSV after the optional meters.
var Switches = []Switch{
	{
		Name: "WallboxKomplettW1", Column: "wallbox_komplett_w1", CSVColumn: "wallbox_komplett_wohnung1",
		FormName: "wallbox_komplett_w1", Label: "Wallboxen vollständig Wohnung 1",
		input:           func(in *PeriodInput) *bool { return &in.WallboxKomplettW1 },
		detail:          func(p *LatestPeriod) *bool { return &p.WallboxKomplettW1 },
		fixkostenInput:  func(in *FixkostenInput) *bool { return &in.WallboxKomplettW1 },
		fixkostenDetail: func(f *FixkostenEingabeDetails) *bool { return &f.WallboxKomplettW1 },
	},
	{
		Name: "PVGutschriftW1", Column: "pv_gutschrift_w1", CSVColumn: "pv_gutschrift_wohnung1",
		FormName: "pv_gutschrift_w1", Label: "PV-Gutschrift Wohnung 1",
		input:           func(in *PeriodInput) *bool { return &in.PVGutschriftW1 },
		detail:          func(p *LatestPeriod) *bool { return &p.PVGutschriftW1 },
		fixkostenInput:  func(in *FixkostenInput) *bool { return &in.PVGutschriftW1 },
		fixkostenDetail: func(f *FixkostenEingabeDetails) *bool { return &f.PVGutschriftW1 },
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

// updatePeriodSQL is the UPDATE of one period by id. Monat uses COALESCE
// (Teilstand). Prices and switches are not written any more: they belong to
// the Fixkosten-Eingabe (#359).
func updatePeriodSQL() string {
	return `UPDATE periods SET reading_date = ?, monat = COALESCE(NULLIF(?, ''), monat) WHERE id = ?`
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

// fixkostenEingabeSpalten are the columns of fixkosten_eingaben besides id:
// the Monat, the four prices and the switches (Switches order).
func fixkostenEingabeSpalten() []string {
	return append([]string{"monat", "strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis"}, switchColumns()...)
}

// fixkostenEingabeArgs returns the INSERT/UPDATE arguments for
// fixkostenEingabeSpalten.
func fixkostenEingabeArgs(in FixkostenInput) []any {
	args := []any{in.Monat, in.Strompreis, in.FrischwasserPreis, in.AbwasserPreis, in.EinspeisungPreis}
	for _, s := range Switches {
		args = append(args, boolToInt(*s.FixkostenInput(&in)))
	}
	return args
}

// insertFixkostenEingabeSQL is the INSERT of one Fixkosten-Eingabe row.
func insertFixkostenEingabeSQL() string {
	cols := fixkostenEingabeSpalten()
	return `INSERT INTO fixkosten_eingaben (` + strings.Join(cols, ", ") + `) VALUES (` + placeholders(len(cols)) + `)`
}

// updateFixkostenEingabeSQL is the UPDATE of one row by id. Unlike the
// Ablesung it overwrites every column, nil prices included (#359).
func updateFixkostenEingabeSQL() string {
	var sets []string
	for _, c := range fixkostenEingabeSpalten() {
		sets = append(sets, c+" = ?")
	}
	return `UPDATE fixkosten_eingaben SET ` + strings.Join(sets, ", ") + ` WHERE id = ?`
}

// selectFixkostenEingabeSQL selects what scanFixkostenEingabeRow reads,
// oldest first.
func selectFixkostenEingabeSQL() string {
	return `SELECT id, ` + strings.Join(fixkostenEingabeSpalten(), ", ") + ` FROM fixkosten_eingaben ORDER BY monat ASC, id ASC`
}

// scanFixkostenEingabeRow scans one row of selectFixkostenEingabeSQL into f.
func scanFixkostenEingabeRow(rows *sql.Rows, f *FixkostenEingabeDetails) error {
	flags := make([]int, len(Switches))
	dest := []any{&f.ID, &f.Monat, &f.Strompreis, &f.FrischwasserPreis, &f.AbwasserPreis, &f.EinspeisungPreis}
	for i := range flags {
		dest = append(dest, &flags[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return err
	}
	for i, s := range Switches {
		*s.FixkostenDetail(f) = flags[i] != 0
	}
	return nil
}

// ensureFixkostenEingabenPreisColumns adds the four price columns (nullable,
// empty = nicht erfasst) and the switch columns (Default) to a
// fixkosten_eingaben table that predates them (#359). Idempotent.
func ensureFixkostenEingabenPreisColumns(db *sql.DB) error {
	type spalte struct{ name, def string }
	add := []spalte{
		{"strompreis", "REAL"}, {"frischwasser_preis", "REAL"}, {"abwasser_preis", "REAL"}, {"einspeisung_preis", "REAL"},
	}
	for _, s := range Switches {
		add = append(add, spalte{s.Column, fmt.Sprintf("INTEGER NOT NULL DEFAULT %d", boolToInt(s.Default))})
	}
	for _, c := range add {
		has, err := hasColumn(db, "fixkosten_eingaben", c.name)
		if err != nil {
			return fmt.Errorf("migrate fixkosten_eingaben.%s: %w", c.name, err)
		}
		if has {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE fixkosten_eingaben ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
			return fmt.Errorf("migrate fixkosten_eingaben.%s: %w", c.name, err)
		}
	}
	return nil
}
