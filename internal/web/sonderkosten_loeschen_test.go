package web

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestSonderkostenLoeschenText(t *testing.T) {
	m := store.MonatLabel("2026-10-01")
	tests := []struct {
		n    int
		want string
	}{
		{0, ""},
		{1, "Dann zählt 1 Sonderkosten-Eintrag mit \"Je Anzahl Personen\" für " + m + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."},
		{2, "Dann zählen 2 Sonderkosten-Einträge mit \"Je Anzahl Personen\" für " + m + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."},
	}
	for _, tt := range tests {
		if got := sonderkostenLoeschenText(tt.n, "2026-10-01"); got != tt.want {
			t.Errorf("n=%d: got %q, want %q", tt.n, got, tt.want)
		}
	}
}
