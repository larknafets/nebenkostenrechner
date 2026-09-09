package web

// AbschlagSaldo is one apartment's credit/repayment balance (Guthaben/
// Nachzahlung) at a point in the Monatsverlauf (monthly history) - the
// difference between the recorded utility advance payment
// (Nebenkostenabschlag) and the actual fixed costs plus consumption,
// accumulated (see buildDashboardVerlauf). A nil *AbschlagSaldo means "no
// balance calculable for this point in time" (e.g. the month has no fixed-
// costs entry/reading) - callers and templates check for nil instead of a
// separate Has-bool. wert (value) is signed and unexported: every other
// fact about the balance (Betrag, Richtung, Label, CSSClass - amount,
// direction, label, CSS class) is derived from it, so there's exactly one
// number that can ever disagree with itself.
type AbschlagSaldo struct {
	wert float64
}

// newAbschlagSaldo wraps an already Round2'd, signed balance value. The
// single construction point for the whole package, so the "wert is already
// rounded" contract lives in one place instead of at every call site.
func newAbschlagSaldo(wert float64) *AbschlagSaldo {
	return &AbschlagSaldo{wert: wert}
}

// Betrag is the balance's absolute amount, always >= 0 - the sign lives in
// Guthaben/Nachzahlung, not here.
func (s *AbschlagSaldo) Betrag() float64 {
	if s.wert < 0 {
		return -s.wert
	}
	return s.wert
}

// Guthaben (credit) is true for a positive balance (the advance payment
// covers more than fixed costs + consumption).
func (s *AbschlagSaldo) Guthaben() bool { return s.wert > 0 }

// Nachzahlung (repayment due) is true for a negative balance (the advance
// payment covers less than fixed costs + consumption).
func (s *AbschlagSaldo) Nachzahlung() bool { return s.wert < 0 }

// Ausgeglichen (balanced) is true for a balance of exactly 0.
func (s *AbschlagSaldo) Ausgeglichen() bool { return s.wert == 0 }

// Label is the German display text for the direction - "Guthaben"/
// "Nachzahlung"/"Ausgeglichen", shared by the dashboard card and the
// Monatsverlauf tab.
func (s *AbschlagSaldo) Label() string {
	switch {
	case s.Guthaben():
		return "Guthaben"
	case s.Nachzahlung():
		return "Nachzahlung"
	default:
		return "Ausgeglichen"
	}
}

// CSSClass is Label lowercased, for the "guthaben"/"nachzahlung"/
// "ausgeglichen" color classes in layout.html (.abschlag-value.*,
// .value-label.*, .bar-track-center .fill.*).
func (s *AbschlagSaldo) CSSClass() string {
	switch {
	case s.Guthaben():
		return "guthaben"
	case s.Nachzahlung():
		return "nachzahlung"
	default:
		return "ausgeglichen"
	}
}
