package web

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_fehlen_test.go: Ablesungen whose Abrechnungsmonat has no
// Fixkosten-Eingabe (or one without prices) are not calculable (#359). The
// views that read the Verbrauchskosten must render them without a crash.

func TestAnsichten_AblesungenOhneFixkostenEingabe(t *testing.T) {
	db := openTestDB(t)
	// Three Ablesungen with consumption: the month of the second has no
	// Eingabe, the month of the third an Eingabe without prices.
	var ids []int64
	for i, datum := range []string{"2026-09-01", "2026-10-01", "2026-11-01"} {
		in := seedPeriodInputAt(datum)
		in.Readings["strom_gesamt"] = float64(100 * i)
		id, err := store.CreatePeriod(db, in)
		if err != nil {
			t.Fatalf("CreatePeriod %s: %v", datum, err)
		}
		ids = append(ids, id)
	}
	if _, err := store.CreateFixkostenEingabe(db, store.FixkostenInput{Monat: "2026-11-01", Personen: map[int64]float64{1: 2, 2: 1}}); err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}

	kosten, err := calc.Load(db)
	if err != nil {
		t.Fatalf("calc.Load: %v", err)
	}
	for _, id := range ids[1:] {
		a, _ := kosten.Ablesung(id)
		if a.Grund != calc.GrundFixkostenFehlen {
			t.Fatalf("Ablesung %d: Grund = %v, want GrundFixkostenFehlen", id, a.Grund)
		}
	}

	dashboardHTML(t, db)
	for _, id := range ids[1:] {
		if detail := detailBody(t, db, id); !strings.Contains(detail, "fehlen die Fixkosten") {
			t.Errorf("Detail der Ablesung %d zeigt den Hinweis auf die fehlenden Fixkosten nicht", id)
		}
	}
	if code, _ := getAbrechnung(t, NewMux(db, openTestDB(t), "", ""), "?jahr=2026&wohnung=2", nil); code != http.StatusOK {
		t.Errorf("Abrechnung status = %d, want 200", code)
	}
}
