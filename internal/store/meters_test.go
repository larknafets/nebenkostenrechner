package store

import (
	"reflect"
	"testing"
)

// Every consumer must see the same meters: the database seed, Meters, the
// key lists and the descriptor all come from meterDefs.
func TestMeters_AlleVerbraucherSehenDieselbeListe(t *testing.T) {
	db := openTestDB(t)

	meters, err := Meters(db)
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	var keys []string
	for _, m := range meters {
		keys = append(keys, m.Key)
	}

	var defKeys []string
	for _, m := range MeterDefs() {
		defKeys = append(defKeys, m.Key)
	}
	if !reflect.DeepEqual(keys, defKeys) {
		t.Errorf("Meters = %v, MeterDefs = %v, want gleich", keys, defKeys)
	}
	if want := append(append([]string{}, MeterKeys...), OptionalMeterKeys...); !reflect.DeepEqual(allMeterKeys(), want) {
		t.Errorf("allMeterKeys = %v, want %v", allMeterKeys(), want)
	}

	for i, m := range meters {
		d := MeterDefs()[i]
		if m != d {
			t.Errorf("Meters[%d] = %+v, want der Descriptor %+v", i, m, d)
		}
		if m.Kurzname == "" || m.AblesungLabel == "" {
			t.Errorf("Zähler %s ohne Kurzname oder Ablesungs-Label", m.Key)
		}
		if got := IsOptionalMeter(m.Key); got != m.Optional {
			t.Errorf("IsOptionalMeter(%s) = %v, want %v", m.Key, got, m.Optional)
		}
	}
}
