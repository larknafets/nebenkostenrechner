package web

import (
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// The bar of a Jahreskarte shows the consumption categories only: the
// Fixkosten stay a chip and in the total, but take no bar width (#302).
func TestJahreskarte_BalkenOhneFixkosten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	dd := mustLoadDashboard(t, db)

	for _, id := range []int64{1, 2} {
		card, _ := buildEntityView(dd, id)
		if card.FixkostenEUR <= 0 {
			t.Fatalf("Wohnung %d: FixkostenEUR = %v, want > 0 (Testdaten ohne Fixkosten)", id, card.FixkostenEUR)
		}
		if seg := segmentByLabel(card.Segmente, "Fixkosten"); seg == nil || seg.Kosten != card.FixkostenEUR {
			t.Errorf("Wohnung %d: Chip Fixkosten = %+v, want Kosten %v", id, seg, card.FixkostenEUR)
		}
		var summe float64
		for _, seg := range card.Segmente {
			if seg.Farbe == "fix" {
				if seg.ProzentNeuestesGesamt != 0 {
					t.Errorf("Wohnung %d: Fixkosten-Segment hat Balkenbreite %v, want 0", id, seg.ProzentNeuestesGesamt)
				}
				continue
			}
			summe += seg.ProzentNeuestesGesamt
		}
		if summe < 99.9 || summe > 100.1 {
			t.Errorf("Wohnung %d: Balken ergeben %v %%, want 100 (nur Verbrauch)", id, summe)
		}
	}

	if body := dashboardHTML(t, db); strings.Contains(body, `stat-bar-seg cat-fix`) {
		t.Error("Dashboard: Balken der Jahreskarte enthält ein Fixkosten-Segment")
	}
}
