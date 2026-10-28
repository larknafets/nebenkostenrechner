package store

import (
	"database/sql"
	"fmt"
)

// Period is one periods row without its readings (see LatestPeriod for the
// fuller read-view shape).
type Period struct {
	ID          int64
	ReadingDate string
}

// GetPeriodByID fetches a single period by id.
func GetPeriodByID(db *sql.DB, id int64) (*Period, error) {
	var p Period
	err := db.QueryRow(
		`SELECT id, reading_date FROM periods WHERE id = ?`,
		id,
	).Scan(&p.ID, &p.ReadingDate)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("period %d not found", id)
		}
		return nil, fmt.Errorf("query period %d: %w", id, err)
	}
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
