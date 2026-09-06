package web

// AbschlagSaldo is one apartment's Guthaben/Nachzahlung-Stand at a point in
// the Monatsverlauf - the difference between the erfassten Nebenkostenabschlag
// and the tatsächlichen Fixkosten+Verbrauch, kumuliert (siehe buildDashboardVerlauf).
// A nil *AbschlagSaldo means "kein Saldo für diesen Zeitpunkt berechenbar"
// (z.B. der Monat hat keine Fixkosten-Eingabe/Ablesung) - callers and
// templates check for nil instead of a separate Has-Bool. wert is signed and
// unexported: every other fact about the Saldo (Betrag, Richtung, Label,
// CSSClass) is derived from it, so there's exactly one number that can ever
// disagree with itself.
type AbschlagSaldo struct {
	wert float64
}

// newAbschlagSaldo wraps an already Round2'd, signed Saldo-Wert. The single
// construction point for the whole package, so the "wert is already gerundet"
// contract lives in one place instead of at every call site.
func newAbschlagSaldo(wert float64) *AbschlagSaldo {
	return &AbschlagSaldo{wert: wert}
}

// Betrag is the Saldo's absolute Betrag, immer >= 0 - Vorzeichen steckt in
// Guthaben/Nachzahlung, nicht hier.
func (s *AbschlagSaldo) Betrag() float64 {
	if s.wert < 0 {
		return -s.wert
	}
	return s.wert
}

// Guthaben is true for a positive Saldo (Abschlag deckt mehr als Fixkosten+Verbrauch).
func (s *AbschlagSaldo) Guthaben() bool { return s.wert > 0 }

// Nachzahlung is true for a negative Saldo (Abschlag deckt weniger als Fixkosten+Verbrauch).
func (s *AbschlagSaldo) Nachzahlung() bool { return s.wert < 0 }

// Ausgeglichen is true for a Saldo von exakt 0.
func (s *AbschlagSaldo) Ausgeglichen() bool { return s.wert == 0 }

// Label is the German Anzeige-Text für die Richtung - "Guthaben"/
// "Nachzahlung"/"Ausgeglichen", gemeinsam genutzt von der Dashboard-Karte und
// dem Monatsverlauf-Reiter.
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

// CSSClass is Label in Kleinschreibung, für die "guthaben"/"nachzahlung"/
// "ausgeglichen"-Farbklassen in layout.html (.abschlag-value.*, .value-label.*,
// .bar-track-center .fill.*).
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
