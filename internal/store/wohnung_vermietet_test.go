package store

import "testing"

func TestApartmentVermietet(t *testing.T) {
	if !(Apartment{Status: StatusVermietet}).Vermietet() {
		t.Error("Status Vermietung: want Vermietet() true")
	}
	if (Apartment{Status: StatusEigennutzung}).Vermietet() {
		t.Error("Status Eigennutzung: want Vermietet() false")
	}
}
