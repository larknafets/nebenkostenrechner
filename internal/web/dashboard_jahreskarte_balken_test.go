package web

import (
	"regexp"
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

// The chips of a Jahreskarte sit in two rows: Strom, Heizung/Ww, Wasser first,
// then Fixkosten and the PV values (Einspeisung Wohnung 1, PV-kWh Wohnung 2).
func TestJahreskarte_ChipsInZweiZeilen(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	body := dashboardHTML(t, db)

	zeile := regexp.MustCompile(`(?s)<div class="muted stat-kategorien">(.*?)\n    </div>`)
	cards := strings.Split(body, `<div class="panel stat-card">`)
	var geprueft int
	for _, card := range cards {
		if !strings.Contains(card, `title="Wohnungsgröße"`) {
			continue
		}
		rows := zeile.FindAllStringSubmatch(card, 2)
		if len(rows) != 2 {
			t.Fatalf("Karte hat %d Chip-Zeilen, want 2", len(rows))
		}
		for _, t1 := range []string{`title="Strom"`, `title="Heizung/Ww"`, `title="Wasser"`} {
			if !strings.Contains(rows[0][1], t1) {
				t.Errorf("Zeile 1 ohne %s", t1)
			}
		}
		if strings.Contains(rows[0][1], `title="Fixkosten"`) {
			t.Error("Fixkosten stehen in Zeile 1")
		}
		if !strings.Contains(rows[1][1], `title="Fixkosten"`) {
			t.Error("Fixkosten fehlen in Zeile 2")
		}
		if !strings.Contains(rows[1][1], `title="PV-Gutschrift"`) && !strings.Contains(rows[1][1], titelPVAnteil) {
			t.Error("Zeile 2 ohne PV-Gutschrift oder PV-Anteil")
		}
		geprueft++
	}
	if geprueft != 2 {
		t.Fatalf("%d Wohnungskarten geprüft, want 2", geprueft)
	}
}

// The Wallbox block of an apartment uses the Wallbox icon, not the bolt of
// "Stromkosten".
func TestJahreskarte_WallboxBlockMitWallboxIcon(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	body := dashboardHTML(t, db)

	i := strings.Index(body, `class="stat-wallbox"`)
	if i < 0 {
		t.Fatal("kein Wallbox-Block")
	}
	block := body[i:]
	block = block[:strings.Index(block, "</div>\n    </div>")]
	if !strings.Contains(block, kategorieIconPaths["Wallbox"]) {
		t.Error("Wallbox-Block ohne Wallbox-Icon")
	}
	if strings.Contains(block, kategorieIconPaths["Stromkosten"]) {
		t.Error("Wallbox-Block mit Blitz-Icon")
	}
}

// The house-wide cards stand in the order Wärmepumpe, Wallboxen, PV-Anlage.
func TestDashboard_HausweiteKartenReihenfolge(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)
	body := dashboardHTML(t, db)

	last := -1
	for _, name := range []string{"Wärmepumpe", "Wallboxen", "PV-Anlage"} {
		i := strings.Index(body, "<strong>"+name+"</strong>")
		if i < 0 {
			t.Fatalf("Karte %s fehlt", name)
		}
		if i < last {
			t.Errorf("Karte %s steht vor ihrem Vorgänger", name)
		}
		last = i
	}
}
