package store

import (
	"database/sql"
	"fmt"
)

// Period is one periods row without its readings/occupancy (see
// LatestPeriod for the fuller read-view shape).
type Period struct {
	ID                int64
	ReadingDate       string
	Strompreis        float64
	FrischwasserPreis float64
	AbwasserPreis     float64
	EinspeisungPreis  float64
}

// GetPeriodByID fetches a single period by id. A Teilstand's (partial
// reading) not-yet-entered price (Ticket #128, periods.<preis> IS NULL)
// reads back as 0 here, not nil - a coalesce, not a real "was it entered"
// answer (see LatestPeriod and Teilstand for that).
func GetPeriodByID(db *sql.DB, id int64) (*Period, error) {
	var p Period
	var strompreis, frischwasserPreis, abwasserPreis, einspeisungPreis sql.NullFloat64
	err := db.QueryRow(
		`SELECT id, reading_date, strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis
		 FROM periods WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.ReadingDate, &strompreis, &frischwasserPreis, &abwasserPreis, &einspeisungPreis)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("period %d not found", id)
		}
		return nil, fmt.Errorf("query period %d: %w", id, err)
	}
	p.Strompreis, p.FrischwasserPreis, p.AbwasserPreis, p.EinspeisungPreis = strompreis.Float64, frischwasserPreis.Float64, abwasserPreis.Float64, einspeisungPreis.Float64
	return &p, nil
}

func readingsByMeterKey(db *sql.DB, periodID int64) (map[string]float64, error) {
	rows, err := db.Query(
		`SELECT m.key, r.zaehlerstand
		 FROM meter_readings r JOIN meters m ON m.id = r.meter_id
		 WHERE r.period_id = ?`,
		periodID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]float64{}
	for rows.Next() {
		var key string
		var value float64
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		out[key] = value
	}
	return out, rows.Err()
}
