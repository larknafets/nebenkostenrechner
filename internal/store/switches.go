package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Switch is one on/off switch (Schalter) of a Fixkosten-Eingabe, e.g.
// "Wallboxen vollständig Wohnung 1" (#225). Switches is the single place a
// switch is written down: the fixkosten_eingaben column, its migration,
// INSERT, UPDATE and scan, the CSV column and the form field name all derive
// from it. Since #368 the Ablesung carries no switches any more.
//
// Adding a switch means: one entry in Switches, the typed field on
// FixkostenInput and FixkostenEingabeDetails (read by calc), the two
// accessors below, the column in schema.sql and the checkbox in the
// Fixkosten template.
type Switch struct {
	// Name is the Go field name on FixkostenInput/FixkostenEingabeDetails, for messages.
	Name string
	// Column is the INTEGER column of the fixkosten_eingaben table.
	Column string
	// CSVColumn is the optional 0/1 column of the Fixkosten CSV export and import.
	CSVColumn string
	// FormName is the name of the checkbox in the Fixkosten form.
	FormName string
	// Label is the German label of the switch.
	Label string
	// Default is the value of a missing field, CSV cell or an Eingabe that
	// predates the switch.
	Default bool

	fixkostenInput  func(*FixkostenInput) *bool
	fixkostenDetail func(*FixkostenEingabeDetails) *bool
}

// FixkostenInput returns a pointer to the switch's field on in.
func (s Switch) FixkostenInput(in *FixkostenInput) *bool { return s.fixkostenInput(in) }

// FixkostenDetail returns a pointer to the switch's field on f.
func (s Switch) FixkostenDetail(f *FixkostenEingabeDetails) *bool { return s.fixkostenDetail(f) }

// Switches is the ordered list of all switches of the Fixkosten-Eingabe.
var Switches = []Switch{
	{
		Name: "WallboxKomplettW1", Column: "wallbox_komplett_w1", CSVColumn: "wallbox_komplett_wohnung1",
		FormName: "wallbox_komplett_w1", Label: "Wallboxen vollständig Wohnung 1",
		fixkostenInput:  func(in *FixkostenInput) *bool { return &in.WallboxKomplettW1 },
		fixkostenDetail: func(f *FixkostenEingabeDetails) *bool { return &f.WallboxKomplettW1 },
	},
	{
		Name: "PVGutschriftW1", Column: "pv_gutschrift_w1", CSVColumn: "pv_gutschrift_wohnung1",
		FormName: "pv_gutschrift_w1", Label: "PV-Gutschrift Wohnung 1",
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

// ensurePeriodSwitchColumns adds the two switch columns a periods table that
// predates them lacks, with the old default (off), so the costs and balances
// of such an Ablesung stay as they were until the backfill has copied them to
// the Fixkosten-Eingabe. It uses its own list of the two columns, not
// Switches: the columns belong to the history of periods, which #368 ends.
// Runs after the periods rebuild of ensurePeriodsNullablePriceColumns, which
// would drop the columns again, and only while the Ablesung prices exist (see
// periodsHatAblesungsPreise): a fresh database must not get the columns back.
func ensurePeriodSwitchColumns(db *sql.DB) error {
	if legacy, err := periodsHatAblesungsPreise(db); err != nil || !legacy {
		return err
	}
	for _, column := range []string{"wallbox_komplett_w1", "pv_gutschrift_w1"} {
		has, err := hasColumn(db, "periods", column)
		if err != nil {
			return fmt.Errorf("migrate %s column: %w", column, err)
		}
		if has {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE periods ADD COLUMN ` + column + ` INTEGER NOT NULL DEFAULT 0`); err != nil {
			return fmt.Errorf("migrate %s column: %w", column, err)
		}
	}
	return nil
}

// insertPeriodSQL is the INSERT of one period.
func insertPeriodSQL() string {
	return `INSERT INTO periods (reading_date, monat) VALUES (?, ?)`
}

// updatePeriodSQL is the UPDATE of one period by id. Monat uses COALESCE
// (Teilstand).
func updatePeriodSQL() string {
	return `UPDATE periods SET reading_date = ?, monat = COALESCE(NULLIF(?, ''), monat) WHERE id = ?`
}

// selectPeriodSQL selects the periods columns scanPeriodRow reads, followed
// by where (empty or a WHERE clause) and the ordering.
func selectPeriodSQL(where string) string {
	return `SELECT id, reading_date, monat FROM periods ` + where + ` ORDER BY reading_date ASC, id ASC`
}

// scanPeriodRow scans one row of selectPeriodSQL into a LatestPeriod.
func scanPeriodRow(rows *sql.Rows) (*LatestPeriod, error) {
	p := &LatestPeriod{Readings: map[string]float64{}}
	if err := rows.Scan(&p.ID, &p.ReadingDate, &p.Monat); err != nil {
		return nil, err
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
