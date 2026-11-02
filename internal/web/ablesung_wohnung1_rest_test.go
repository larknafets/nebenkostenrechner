package web

import (
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// restReadings returns a full set of required readings, every meter at base,
// plus the given overrides.
func restReadings(base float64, extra map[string]float64) map[string]float64 {
	r := make(map[string]float64, len(store.MeterKeys))
	for _, k := range store.MeterKeys {
		r[k] = base
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

func TestWohnung1RestZeile_Strom(t *testing.T) {
	vor := restReadings(100, nil)
	// 50 kWh grid draw, nothing taken by the submeters: Wohnung 1 keeps it.
	cur := restReadings(100, map[string]float64{"strom_gesamt": 150})
	z := wohnung1RestZeile(store.MeterKeyWallboxWohnung2, "kWh", cur, vor)
	if z == nil || z.Diff != "+50,00" || !z.Abgeleitet {
		t.Fatalf("Rest > 0: got %+v", z)
	}

	// PV covered the submeters, nothing of the grid draw is left.
	cur = restReadings(100, map[string]float64{"strom_gesamt": 110, "strom_wohnung2": 130})
	if z := wohnung1RestZeile(store.MeterKeyWallboxWohnung2, "kWh", cur, vor); z != nil {
		t.Fatalf("Rest 0: want no row, got %+v", z)
	}

	// A required meter missing in the newer reading (Teilstand): not computable.
	cur = restReadings(100, map[string]float64{"strom_gesamt": 150})
	delete(cur, "strom_wohnung2")
	if z := wohnung1RestZeile(store.MeterKeyWallboxWohnung2, "kWh", cur, vor); z != nil {
		t.Fatalf("missing meter: want no row, got %+v", z)
	}
}

func TestWohnung1RestZeile_Wasser(t *testing.T) {
	vor := restReadings(10, nil)
	cur := restReadings(10, map[string]float64{"wasser_gesamt": 26.5, "wasser_wohnung2": 14, "wasser_warmwasseraufbereitung": 12})
	z := wohnung1RestZeile("wasser_warmwasseraufbereitung", "m³", cur, vor)
	if z == nil || z.Diff != "+10,50" {
		t.Fatalf("got %+v", z)
	}
	delete(cur, "wasser_wohnung2")
	if z := wohnung1RestZeile("wasser_warmwasseraufbereitung", "m³", cur, vor); z != nil {
		t.Fatalf("missing meter: want no row, got %+v", z)
	}
	if z := wohnung1RestZeile("strom_gesamt", "kWh", cur, vor); z != nil {
		t.Fatalf("other meter: want no row, got %+v", z)
	}
}

// The rest rows hang on the last meter of their group, the anchors must stay
// the last Strom and the last Wasser meter of meterDisplayOrder.
func TestWohnung1RestZeile_AnkerLetzterZaehler(t *testing.T) {
	var strom, wasser string
	for _, m := range meterDisplays {
		switch {
		case strings.HasPrefix(m.Key, "strom_"):
			strom = m.Key
		case strings.HasPrefix(m.Key, "wasser_"):
			wasser = m.Key
		}
	}
	if strom != store.MeterKeyWallboxWohnung2 || wasser != "wasser_warmwasseraufbereitung" {
		t.Fatalf("Anker verschoben: %s, %s", strom, wasser)
	}
}
