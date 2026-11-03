package web

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestLoeschenFrageText(t *testing.T) {
	personen := func(monat string) store.Sonderkosten {
		return store.Sonderkosten{Monat: monat, Umlegbar: true, Umsetzung: store.UmsetzungHaus, Logik: store.LogikPersonen}
	}
	tests := []struct {
		name string
		alle []store.Sonderkosten
		want string
	}{
		{"kein Eintrag", nil, ""},
		{"ein Eintrag", []store.Sonderkosten{personen("2026-10-01")},
			"Dann zählt 1 Sonderkosten-Eintrag mit \"Je Anzahl Personen\" für " + store.MonatLabel("2026-10-01") + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."},
		{"mehrere Einträge", []store.Sonderkosten{personen("2026-10-01"), personen("2026-10-01")},
			"Dann zählen 2 Sonderkosten-Einträge mit \"Je Anzahl Personen\" für " + store.MonatLabel("2026-10-01") + " nicht mehr, bis die Fixkosteneingabe neu erfasst ist."},
		{"anderer Monat", []store.Sonderkosten{personen("2026-09-01")}, ""},
		{"nicht umlegbar", []store.Sonderkosten{{Monat: "2026-10-01", Umsetzung: store.UmsetzungHaus, Logik: store.LogikPersonen}}, ""},
		{"andere Umsetzung", []store.Sonderkosten{{Monat: "2026-10-01", Umlegbar: true, Umsetzung: store.UmsetzungWohnung1, Logik: store.LogikPersonen}}, ""},
	}
	for _, tt := range tests {
		if got := loeschenFrageText(tt.alle, "2026-10-01"); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
