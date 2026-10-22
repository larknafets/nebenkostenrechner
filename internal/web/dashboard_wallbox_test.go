package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboard_wallbox_test.go covers the category "Wallbox" and the negative
// line "PV-Gutschrift" in Dashboard and widgets (#227) through the data and
// HTTP seams.

func segmentByLabel(segs []dashboardSegment, label string) *dashboardSegment {
	for i := range segs {
		if segs[i].Label == label {
			return &segs[i]
		}
	}
	return nil
}

// wallboxPVAblesungen stores wallboxAblesungen (Wallbox Wohnung 2 at 15 kWh,
// so 3,30 EUR for Wohnung 2 and 1,10 EUR for Wohnung 1) and, in the second
// Ablesung, 52,5 kWh feed-in (4,20 EUR) with the given PV switch.
func wallboxPVAblesungen(t *testing.T, db *sql.DB, pvSchalter bool) {
	t.Helper()
	w2 := 15.0
	first := seedPeriodInputAt("2026-10-01")
	first.Readings[wbW1], first.Readings[wbW2] = 0, 0
	if _, err := store.CreatePeriod(db, first); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
	second := seedPeriodInputAt("2026-11-01")
	second.Readings["strom_gesamt"] = 100
	second.Readings["strom_wohnung2"] = 30
	second.Readings["strom_waermepumpe"] = 50
	second.Readings["strom_wallbox"] = 40
	second.Readings["strom_einspeisung"] = 52.5
	second.Readings[wbW2] = w2
	second.PVGutschriftW1 = pvSchalter
	if _, err := store.CreatePeriod(db, second); err != nil {
		t.Fatalf("CreatePeriod: %v", err)
	}
}

func TestJahreskarte_WallboxKategorieBeiBeidenWohnungen(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	dd, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}

	for _, c := range []struct {
		apt  int64
		want float64
	}{{1, 1.10}, {2, 3.30}} {
		card, _ := buildEntityView(dd, c.apt)
		seg := segmentByLabel(card.Segmente, "Wallbox")
		if seg == nil {
			t.Fatalf("Wohnung %d: Segment Wallbox fehlt: %+v", c.apt, card.Segmente)
		}
		if seg.Kosten != c.want {
			t.Errorf("Wohnung %d: Wallbox = %v EUR, want %v", c.apt, seg.Kosten, c.want)
		}
		if card.WallboxEUR != c.want {
			t.Errorf("Wohnung %d: WallboxEUR = %v, want %v", c.apt, card.WallboxEUR, c.want)
		}
		// Wallbox is a consumption cost: it is part of Verbrauch and Gesamt.
		if want := card.StromEUR + card.WallboxEUR + card.HeizungEUR + card.WasserEUR; card.VerbrauchEUR != calc.Round2(want) {
			t.Errorf("Wohnung %d: VerbrauchEUR = %v, want %v", c.apt, card.VerbrauchEUR, want)
		}
	}

	// Strom stays the household electricity of Wohnung 2 (30 kWh * 0,22 EUR).
	card2, _ := buildEntityView(dd, 2)
	if card2.StromEUR != 6.60 {
		t.Errorf("Wohnung 2: StromEUR = %v, want 6.60 (ohne Wallbox)", card2.StromEUR)
	}
}

func TestJahreskarte_WallboxImmerSichtbarAuchMitNull(t *testing.T) {
	db := openTestDB(t)
	// Without Einzelzähler and switch the Wallbox costs stay informative.
	wallboxAblesungen(t, db, nil, nil, false)
	dd, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	for _, apt := range []int64{1, 2} {
		card, verlauf := buildEntityView(dd, apt)
		seg := segmentByLabel(card.Segmente, "Wallbox")
		if seg == nil || seg.Kosten != 0 {
			t.Errorf("Wohnung %d: Jahreskarte soll Wallbox mit 0 EUR zeigen, got %+v", apt, seg)
		}
		m := verlauf.Eintraege[0].Monat
		if m == nil || segmentByLabel(m.VerbrauchSegmente, "Wallbox") == nil {
			t.Errorf("Wohnung %d: Monatsverlauf soll Wallbox auch mit 0 EUR zeigen", apt)
		}
	}
}

func TestMonatsverlauf_WallboxKWhAlsZugeordneterNetzbezug(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	dd, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	// 20 kWh grid draw are left for the Wallboxen: Wohnung 2 15, Wohnung 1 the rest.
	for _, c := range []struct {
		apt  int64
		want float64
	}{{1, 5}, {2, 15}} {
		_, verlauf := buildEntityView(dd, c.apt)
		m := verlauf.Eintraege[0].Monat
		seg := segmentByLabel(m.VerbrauchSegmente, "Wallbox")
		if seg == nil || seg.Verbrauch != c.want || seg.Einheit != "kWh" {
			t.Errorf("Wohnung %d: Verbrauchswert Wallbox = %+v, want %v kWh", c.apt, seg, c.want)
		}
	}
}

func TestPVGutschrift_NegativeZeileBeiWohnung1(t *testing.T) {
	for _, schalter := range []bool{true, false} {
		db := openTestDB(t)
		wallboxPVAblesungen(t, db, schalter)
		dd, err := loadDashboardData(db)
		if err != nil {
			t.Fatalf("loadDashboardData: %v", err)
		}
		want := 0.0
		if schalter {
			want = 4.20
		}

		card, verlauf := buildEntityView(dd, 1)
		if card.PVGutschriftEUR != want {
			t.Errorf("Schalter %v: PVGutschriftEUR = %v, want %v", schalter, card.PVGutschriftEUR, want)
		}
		if brutto := calc.Round2(card.VerbrauchEUR + card.FixkostenEUR); card.GesamtEUR != calc.Round2(brutto-want) {
			t.Errorf("Schalter %v: GesamtEUR = %v, want %v (Verbrauch+Fix %v minus Gutschrift)", schalter, card.GesamtEUR, brutto-want, brutto)
		}
		if segmentByLabel(card.Segmente, "PV-Gutschrift") != nil {
			t.Errorf("Schalter %v: PV-Gutschrift darf kein Segment im Mini-Balken sein", schalter)
		}

		m := verlauf.Eintraege[0].Monat
		if m.PVGutschrift != want {
			t.Errorf("Schalter %v: Monat.PVGutschrift = %v, want %v", schalter, m.PVGutschrift, want)
		}
		if brutto := calc.Round2(m.VerbrauchGesamt); m.KombiniertGesamt != calc.Round2(brutto-want) {
			t.Errorf("Schalter %v: KombiniertGesamt = %v, want %v", schalter, m.KombiniertGesamt, brutto-want)
		}
		// Neither in Verbrauch nor in Verbrauchswerte (both read VerbrauchSegmente).
		if segmentByLabel(m.VerbrauchSegmente, "PV-Gutschrift") != nil || segmentByLabel(m.KombiniertSegmente, "PV-Gutschrift") != nil {
			t.Errorf("Schalter %v: PV-Gutschrift darf kein Balkensegment sein", schalter)
		}

		// Wohnung 2 never gets a credit.
		card2, verlauf2 := buildEntityView(dd, 2)
		if card2.PVGutschriftEUR != 0 || verlauf2.Eintraege[0].Monat.PVGutschrift != 0 {
			t.Errorf("Schalter %v: Wohnung 2 hat eine Gutschrift", schalter)
		}
	}
}

func TestSaldo_GutschriftErhoehtGuthabenOhneDeckel(t *testing.T) {
	monate := []dashboardMonat{{Label: "Nov"}, {Label: "Okt"}}
	werte := []calc.Monatswert{
		{Monat: "2026-10-01", HatFixkosten: true, Fixkosten: 30, Abschlag: 100},
		{Monat: "2026-11-01", HatFixkosten: true, Fixkosten: 25, Abschlag: 100, Gutschrift: 5},
	}
	// Brutto Nov = 25 (net 20 + 5 credit): saldo Okt 100-30 = 70, Nov 70 + 100 - 20 = 150.
	applySaldo(monate, []string{"2026-11-01", "2026-10-01"}, werte)
	if got := monate[1].Saldo; got == nil || got.Betrag() != 70 {
		t.Fatalf("Okt.Saldo = %+v, want 70", got)
	}
	if got := monate[0].Saldo; got == nil || got.Betrag() != 150 || !got.Guthaben() {
		t.Fatalf("Nov.Saldo = %+v, want 150 Guthaben (Gutschrift ohne Deckel)", got)
	}
}

func TestJahreszeile_NettoMitGutschriftSumme(t *testing.T) {
	k := calc.Kosten{
		Strom: &calc.StromErgebnis{}, Wasser: &calc.WasserErgebnis{},
		Heizung: &calc.HeizungErgebnis{KostenHeizungW1: 30}, PVGutschriftW1: 4.20,
	}
	spalte := buildDashboardVerlauf(1, "Wohnung 1", []periodKosten{{ReadingDate: "2026-11-15", Monat: "2026-11-01", K: k}}, nil)
	var jz *dashboardJahreszeile
	for _, e := range spalte.Eintraege {
		if e.Jahreszeile != nil {
			jz = e.Jahreszeile
		}
	}
	if jz == nil || jz.PVGutschriftSumme != 4.20 || jz.GesamtSumme != 25.80 || jz.VerbrauchSumme != 30 {
		t.Errorf("Jahreszeile = %+v, want Verbrauch 30, Gutschrift 4.20, Gesamt 25.80", jz)
	}
}

func dashboardHTML(t *testing.T, db *sql.DB) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handleDashboard("v0", "", newAuth("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func widgetHTML(t *testing.T, db *sql.DB, route, entity string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/widget/"+route+"/"+entity, nil)
	w := httptest.NewRecorder()
	NewWidgetMux(db).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func TestDashboardHTML_WallboxUndPVGutschrift(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)
	body := dashboardHTML(t, db)

	if n := strings.Count(body, `title="Wallbox"`); n < 2 {
		t.Errorf("Kategorie Wallbox %d mal in der Jahreskarte, want mindestens 2 (Wohnung 1 und 2)", n)
	}
	if !strings.Contains(body, `title="PV-Gutschrift"`) || !strings.Contains(body, "4,20") {
		t.Error("Jahreskarte Wohnung 1 ohne Zeile PV-Gutschrift 4,20")
	}
	// The house-wide cards stay as they are.
	for _, s := range []string{"<strong>Wallboxen</strong>", "<strong>PV-Anlage</strong>"} {
		if !strings.Contains(body, s) {
			t.Errorf("haus-weite Karte %s fehlt", s)
		}
	}
}

func TestDashboardHTML_OhneSchalterKeineGutschriftZeile(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	if body := dashboardHTML(t, db); strings.Contains(body, `title="PV-Gutschrift"`) {
		t.Error("Zeile PV-Gutschrift ohne Schalter")
	}
}

func TestWidgets_ZeigenWallboxUndPVGutschrift(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)

	for _, route := range []string{"jahressumme", "uebersicht"} {
		body := widgetHTML(t, db, route, "wohnung-1")
		if !strings.Contains(body, `title="Wallbox"`) || !strings.Contains(body, `title="PV-Gutschrift"`) {
			t.Errorf("Widget %s Wohnung 1 ohne Wallbox oder PV-Gutschrift", route)
		}
		body2 := widgetHTML(t, db, route, "wohnung-2")
		if !strings.Contains(body2, `title="Wallbox"`) || strings.Contains(body2, `title="PV-Gutschrift"`) {
			t.Errorf("Widget %s Wohnung 2: Wallbox erwartet, PV-Gutschrift nicht", route)
		}
	}
	for _, route := range []string{"verbrauchswerte", "uebersicht"} {
		body := widgetHTML(t, db, route, "wohnung-1")
		if !strings.Contains(body, "Wallbox <span") {
			t.Errorf("Widget %s Wohnung 1: Verbrauchswert Wallbox fehlt", route)
		}
		if !strings.Contains(body, "PV-Gutschrift") {
			t.Errorf("Widget %s Wohnung 1: PV-Gutschrift im Kombiniert-Modus fehlt", route)
		}
	}
}
