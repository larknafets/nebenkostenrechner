package store

import "testing"

// The per-Ablesung switches are gone (#368): the Switches descriptor now
// belongs to the Fixkosten-Eingabe alone, its roundtrip is covered in
// fixkosten_preise_test.go. What stays here is the read of an Ablesung that
// does not exist.

func TestGetPeriodDetails_UnbekannteId(t *testing.T) {
	db := openTestDB(t)
	if got, err := GetPeriodDetails(db, 4711); got != nil || err != nil {
		t.Errorf("GetPeriodDetails(unknown) = %v, %v, want nil, nil", got, err)
	}
}
