package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Keys of the optional Wallbox-Einzelzähler behind the Zwischenzähler
// "Wallboxen" (#224).
const (
	MeterKeyWallboxWohnung1 = "strom_wallbox_wohnung1"
	MeterKeyWallboxWohnung2 = "strom_wallbox_wohnung2"
)

// Meter is one fixed meter (Zähler) with everything the consumers need to
// know about it. meterDefs is the single place these values are written
// down, seed(), Meters and the key lists below all derive from it.
type Meter struct {
	Key string
	// Label is the full name, also the label stored in the meters table.
	Label string
	// Kurzname is the short column heading of the Zählerstände table
	// (Anlage 7), 10 columns plus the dates have to fit on a landscape page.
	Kurzname string
	// AblesungLabel is the label on the Ablesung detail view.
	AblesungLabel string
	Unit          string
	// ApartmentID is the apartment a meter belongs to, 0 for a house-wide
	// meter.
	ApartmentID int64
	// Optional marks a meter a period may leave out without becoming a
	// Teilstand (#224). A missing reading means "nicht erfasst", never 0.
	// Optional meters are persisted and exported like the others, but
	// completeness checks and the cost calculation only read the required
	// ones.
	Optional bool
	// Eingerueckt indents the meter below its parent meter (Wallboxen) where
	// it is listed.
	Eingerueckt bool
}

// meterDef is a Meter with the master data only the seed needs.
type meterDef struct {
	id        int64
	meterType string
	Meter
}

// meterDefs lists all 12 meters (10 required, 2 optional) ordered by id.
// The ids and keys are part of the database, never renumber them.
var meterDefs = []meterDef{
	{1, "strom", Meter{Key: "strom_gesamt", Label: "Stromzähler Gesamt", Kurzname: "Strom gesamt", AblesungLabel: "Strom Netzbezug (1.8.0)", Unit: "kWh"}},
	{2, "strom", Meter{Key: "strom_wohnung2", Label: "Zwischenstromzähler Wohnung 2", Kurzname: "Strom Whg 2", AblesungLabel: "Strom Wohnung 2", Unit: "kWh", ApartmentID: 2}},
	{3, "strom", Meter{Key: "strom_waermepumpe", Label: "Zwischenstromzähler Wärmepumpe", Kurzname: "Strom WP", AblesungLabel: "Strom Wärmepumpe", Unit: "kWh"}},
	{4, "strom", Meter{Key: "strom_wallbox", Label: "Zwischenzähler Wallboxen", Kurzname: "Wallboxen", AblesungLabel: "Strom Wallboxen", Unit: "kWh"}},
	{5, "wasser", Meter{Key: "wasser_gesamt", Label: "Wasserzähler Gesamt", Kurzname: "Wasser gesamt", AblesungLabel: "Wasser Gesamt", Unit: "m3"}},
	{6, "wasser", Meter{Key: "wasser_wohnung2", Label: "Zwischenwasserzähler Wohnung 2", Kurzname: "Wasser Whg 2", AblesungLabel: "Wasser Wohnung 2", Unit: "m3", ApartmentID: 2}},
	{7, "wasser", Meter{Key: "wasser_warmwasseraufbereitung", Label: "Zwischenwasserzähler Warmwasseraufbereitung", Kurzname: "Warmwasser", AblesungLabel: "Wasser Warmwasseraufbereitung", Unit: "m3"}},
	{8, "waerme", Meter{Key: "waerme_wohnung1", Label: "Wärmemengenzähler Wohnung 1", Kurzname: "Wärme Whg 1", AblesungLabel: "Wärme Wohnung 1", Unit: "MWh", ApartmentID: 1}},
	{9, "waerme", Meter{Key: "waerme_wohnung2", Label: "Wärmemengenzähler Wohnung 2", Kurzname: "Wärme Whg 2", AblesungLabel: "Wärme Wohnung 2", Unit: "MWh", ApartmentID: 2}},
	{10, "strom", Meter{Key: "strom_einspeisung", Label: "Einspeisezähler (PV)", Kurzname: "Einspeisung (PV)", AblesungLabel: "Strom Einspeisung (2.8.0)", Unit: "kWh"}},
	{11, "strom", Meter{Key: MeterKeyWallboxWohnung1, Label: "Wallbox Wohnung 1", Kurzname: "Wallbox Whg 1", AblesungLabel: "Wallbox Wohnung 1", Unit: "kWh", ApartmentID: 1, Optional: true, Eingerueckt: true}},
	{12, "strom", Meter{Key: MeterKeyWallboxWohnung2, Label: "Wallbox Wohnung 2", Kurzname: "Wallbox Whg 2", AblesungLabel: "Wallbox Wohnung 2", Unit: "kWh", ApartmentID: 2, Optional: true, Eingerueckt: true}},
}

// MeterKeys is the stable, ordered list of the required meter keys every
// period must have a reading for.
var MeterKeys = meterKeys(false)

// OptionalMeterKeys are the keys of the optional meters (#224), see
// Meter.Optional.
var OptionalMeterKeys = meterKeys(true)

func meterKeys(optional bool) []string {
	var keys []string
	for _, d := range meterDefs {
		if d.Optional == optional {
			keys = append(keys, d.Key)
		}
	}
	return keys
}

// allMeterKeys is MeterKeys followed by OptionalMeterKeys, the meters that
// are persisted and exported.
func allMeterKeys() []string {
	return append(append([]string{}, MeterKeys...), OptionalMeterKeys...)
}

// placeholders returns n comma-separated SQL placeholders.
func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// MeterDefs returns all meters from the descriptor, ordered by id, without
// touching a database.
func MeterDefs() []Meter {
	out := make([]Meter, len(meterDefs))
	for i, d := range meterDefs {
		out[i] = d.Meter
	}
	return out
}

// MeterByKey returns the descriptor of the meter with the given key.
func MeterByKey(key string) (Meter, bool) {
	for _, d := range meterDefs {
		if d.Key == key {
			return d.Meter, true
		}
	}
	return Meter{}, false
}

// IsOptionalMeter reports whether key is one of OptionalMeterKeys.
func IsOptionalMeter(key string) bool {
	m, ok := MeterByKey(key)
	return ok && m.Optional
}

// Meters returns all meters, required and optional, ordered by id. Label,
// unit and apartment come from the meters table, the rest from the
// descriptor. The consumers filter on Meter.Optional themselves, the Anlage
// lists the required ones and adds the optional ones that have readings.
func Meters(db *sql.DB) ([]Meter, error) {
	rows, err := db.Query(`SELECT key, label, unit, COALESCE(apartment_id, 0) FROM meters ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query meters: %w", err)
	}
	defer rows.Close()

	var out []Meter
	for rows.Next() {
		var m Meter
		if err := rows.Scan(&m.Key, &m.Label, &m.Unit, &m.ApartmentID); err != nil {
			return nil, fmt.Errorf("scan meter: %w", err)
		}
		if d, ok := MeterByKey(m.Key); ok {
			m.Kurzname, m.AblesungLabel, m.Optional, m.Eingerueckt = d.Kurzname, d.AblesungLabel, d.Optional, d.Eingerueckt
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// seedMeters inserts the meters from meterDefs that are not yet present.
func seedMeters(db *sql.DB) error {
	for _, d := range meterDefs {
		var apartment *int64
		if d.ApartmentID != 0 {
			apartment = apartmentID(d.ApartmentID)
		}
		if _, err := db.Exec(
			`INSERT INTO meters (id, key, type, unit, apartment_id, label) VALUES (?, ?, ?, ?, ?, ?)
			 ON CONFLICT(id) DO NOTHING`,
			d.id, d.Key, d.meterType, d.Unit, apartment, d.Label,
		); err != nil {
			return fmt.Errorf("seed meter %q: %w", d.Key, err)
		}
	}
	return nil
}
