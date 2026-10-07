package abrechnung

import (
	"reflect"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestZaehlerstaende_UntermonatigeAblesungenEigeneZeilenTeilstandFehlt(t *testing.T) {
	apartments := []store.Apartment{{ID: 1}, {ID: 2}}
	meters := []store.Meter{{Key: "strom_gesamt", Label: "Stromzähler Gesamt", Unit: "kWh"}}
	preis := store.Float64(0.3)
	ablesung := func(id int64, datum, monat string, stand float64, vollstaendig bool) *store.LatestPeriod {
		p := &store.LatestPeriod{ID: id, ReadingDate: datum, Monat: monat, Readings: map[string]float64{}, PersonenByApartment: map[int64]int64{1: 1, 2: 1}}
		if vollstaendig {
			p.Strompreis, p.FrischwasserPreis, p.AbwasserPreis, p.EinspeisungPreis = preis, preis, preis, preis
		}
		for _, key := range store.MeterKeys {
			p.Readings[key] = stand
		}
		return p
	}
	periods := []*store.LatestPeriod{
		ablesung(4, "2026-03-01", "2026-03-01", 40, false), // Teilstand
		ablesung(3, "2026-02-20", "2026-02-01", 30, true),
		ablesung(2, "2026-02-10", "2026-02-01", 20, true),
		ablesung(1, "2026-01-10", "2026-01-01", 10, true),
	}
	imZeitraum := map[string]bool{"2026-02-01": true, "2026-03-01": true}
	z := zaehlerstaende(periods, apartments, meters, imZeitraum, 2)
	var datum []string
	for _, r := range z.Zeilen {
		datum = append(datum, r.Ablesedatum)
	}
	// January is the Ausgangsstand, both February Ablesungen are rows, the
	// Teilstand of March is not.
	if want := []string{"2026-01-10", "2026-02-10", "2026-02-20"}; !reflect.DeepEqual(datum, want) {
		t.Errorf("Ablesedatum = %v, want %v", datum, want)
	}
}
