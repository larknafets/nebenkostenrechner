package calc

import "testing"

func TestSaldoverlauf(t *testing.T) {
	t.Run("kumuliert Monat für Monat ab Startwert", func(t *testing.T) {
		saldi, end := Saldoverlauf(10, []SaldoMonat{
			{Abschlag: 100, Kosten: 80.25},
			{Abschlag: 100, Kosten: 130.10},
		})
		if len(saldi) != 2 || saldi[0] != 29.75 || saldi[1] != -0.35 || end != -0.35 {
			t.Fatalf("saldi=%v end=%v", saldi, end)
		}
	})
	t.Run("ohne Monate ist der Endsaldo der Startwert", func(t *testing.T) {
		saldi, end := Saldoverlauf(12.5, nil)
		if len(saldi) != 0 || end != 12.5 {
			t.Fatalf("saldi=%v end=%v", saldi, end)
		}
	})
	t.Run("rundet die Monatsdifferenz auf den Cent", func(t *testing.T) {
		saldi, _ := Saldoverlauf(0, []SaldoMonat{{Abschlag: 50, Kosten: 33.333}})
		if saldi[0] != 16.67 {
			t.Fatalf("saldi=%v", saldi)
		}
	})
}
