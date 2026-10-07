package calc

import "testing"

func TestAbschlagSaldo(t *testing.T) {
	for _, tc := range []struct {
		name             string
		wert             float64
		wantBetrag       float64
		wantGuthaben     bool
		wantNachzahlung  bool
		wantAusgeglichen bool
		wantLabel        string
		wantCSSClass     string
	}{
		{"positiv -> Guthaben", 128.4, 128.4, true, false, false, "Guthaben", "guthaben"},
		{"negativ -> Nachzahlung", -61.5, 61.5, false, true, false, "Nachzahlung", "nachzahlung"},
		{"exakt 0 -> Ausgeglichen", 0, 0, false, false, true, "Ausgeglichen", "ausgeglichen"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := NewAbschlagSaldo(tc.wert)
			if got := s.Wert(); got != tc.wert {
				t.Errorf("Wert() = %v, want %v", got, tc.wert)
			}
			if got := s.Betrag(); got != tc.wantBetrag {
				t.Errorf("Betrag() = %v, want %v", got, tc.wantBetrag)
			}
			if got := s.Guthaben(); got != tc.wantGuthaben {
				t.Errorf("Guthaben() = %v, want %v", got, tc.wantGuthaben)
			}
			if got := s.Nachzahlung(); got != tc.wantNachzahlung {
				t.Errorf("Nachzahlung() = %v, want %v", got, tc.wantNachzahlung)
			}
			if got := s.Ausgeglichen(); got != tc.wantAusgeglichen {
				t.Errorf("Ausgeglichen() = %v, want %v", got, tc.wantAusgeglichen)
			}
			if got := s.Label(); got != tc.wantLabel {
				t.Errorf("Label() = %q, want %q", got, tc.wantLabel)
			}
			if got := s.CSSClass(); got != tc.wantCSSClass {
				t.Errorf("CSSClass() = %q, want %q", got, tc.wantCSSClass)
			}
		})
	}
}
