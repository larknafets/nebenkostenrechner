package calc

// SaldoMonat is one month of a Saldoverlauf: the Nebenkostenabschlag paid and
// the costs charged against it (the caller decides which positions count).
type SaldoMonat struct {
	Abschlag, Kosten float64
}

// Saldoverlauf accumulates the monthly balance oldest to newest, beginning
// with start (the Übertrag, 0 for none). Each month adds Round2(Abschlag -
// Kosten) and the running balance is rounded to the cent, so the result does
// not depend on the caller's own rounding. It returns the balance after each
// month (same order as monate) and the Endsaldo, which is start for no months.
func Saldoverlauf(start float64, monate []SaldoMonat) (saldi []float64, endsaldo float64) {
	saldi = make([]float64, len(monate))
	endsaldo = start
	for i, m := range monate {
		endsaldo = Round2(endsaldo + Round2(m.Abschlag-m.Kosten))
		saldi[i] = endsaldo
	}
	return saldi, endsaldo
}
