package store

import "testing"

func TestBrauchtFixkosteneingabe(t *testing.T) {
	tests := []struct {
		name string
		s    Sonderkosten
		want bool
	}{
		{"umlegbar, Haus, Personen", Sonderkosten{Umlegbar: true, Umsetzung: UmsetzungHaus, Logik: LogikPersonen}, true},
		{"nicht umlegbar", Sonderkosten{Umsetzung: UmsetzungHaus, Logik: LogikPersonen}, false},
		{"Umsetzung Wohnung 1 (Altdaten)", Sonderkosten{Umlegbar: true, Umsetzung: UmsetzungWohnung1, Logik: LogikPersonen}, false},
		{"andere Logik", Sonderkosten{Umlegbar: true, Umsetzung: UmsetzungHaus, Logik: LogikQM}, false},
	}
	for _, tt := range tests {
		if got := tt.s.BrauchtFixkosteneingabe(); got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}
