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
// line "PV-Gutschrift" in the Dashboard (#227) through the data and
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

func TestJahreskarte_WallboxBlockBeiBeidenWohnungen(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	dd, _, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}

	for _, c := range []struct {
		apt  int64
		euro float64
		kWh  float64
	}{{1, 1.10, 5}, {2, 3.30, 15}} {
		card, _ := buildEntityView(dd, c.apt)
		// The Wallbox leaves the main bar and the chips (#304).
		if seg := segmentByLabel(card.Segmente, "Wallbox"); seg != nil {
			t.Errorf("Wohnung %d: Segment Wallbox im Hauptbalken: %+v", c.apt, seg)
		}
		if card.Wallbox == nil || len(card.Wallbox.Segmente) != 1 {
			t.Fatalf("Wohnung %d: Wallbox-Block = %+v, want ein Segment", c.apt, card.Wallbox)
		}
		seg := card.Wallbox.Segmente[0]
		if seg.Label != "Stromkosten" || seg.Kosten != c.euro || seg.Verbrauch != c.kWh || seg.Einheit != "kWh" || seg.ProzentNeuestesGesamt != 100 {
			t.Errorf("Wohnung %d: Block-Segment = %+v, want Stromkosten %v EUR, %v kWh, 100%%", c.apt, seg, c.euro, c.kWh)
		}
		if card.WallboxEUR != c.euro || card.WallboxKWh != c.kWh {
			t.Errorf("Wohnung %d: WallboxEUR/KWh = %v/%v, want %v/%v", c.apt, card.WallboxEUR, card.WallboxKWh, c.euro, c.kWh)
		}
		// Wallbox is a consumption cost: it stays part of Verbrauch and Gesamt.
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

// Without an assignment of the Wallbox costs there is no block, also not
// with 0 (#304); the Monatsverlauf still lists the Wallbox with 0 EUR (#227).
func TestJahreskarte_WallboxOhneZuordnungKeinBlock(t *testing.T) {
	db := openTestDB(t)
	// Without Einzelzähler and switch the Wallbox costs stay informative.
	wallboxAblesungen(t, db, nil, nil, false)
	dd, _, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	for _, apt := range []int64{1, 2} {
		card, verlauf := buildEntityView(dd, apt)
		if card.Wallbox != nil {
			t.Errorf("Wohnung %d: Wallbox-Block ohne Zuordnung: %+v", apt, card.Wallbox)
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
	dd, _, err := loadDashboardData(db)
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
		dd, _, err := loadDashboardData(db)
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
	handleDashboard("v0", "", newZugang("", nil))(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	return w.Body.String()
}

func TestDashboardHTML_WallboxUndPVGutschrift(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, true)
	body := dashboardHTML(t, db)

	if n := strings.Count(body, `class="stat-wallbox"`); n != 2 {
		t.Errorf("Wallbox-Block %d mal in den Jahreskarten, want 2 (Wohnung 1 und 2)", n)
	}
	if !strings.Contains(body, `title="PV-Gutschrift"`) || !strings.Contains(body, "&minus;4,20") {
		t.Error("Jahreskarte Wohnung 1 ohne Zeile PV-Gutschrift 4,20")
	}
	// The house-wide cards stay as they are.
	for _, s := range []string{"<strong>Wallboxen</strong>", "<strong>PV-Anlage</strong>"} {
		if !strings.Contains(body, s) {
			t.Errorf("haus-weite Karte %s fehlt", s)
		}
	}
}

func TestDashboardHTML_OhneSchalterGutschriftNurMitNull(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	body := dashboardHTML(t, db)
	if !strings.Contains(body, `title="PV-Gutschrift"`) {
		t.Error("Zeile PV-Gutschrift fehlt, sie steht auch mit 0")
	}
	if strings.Contains(body, "&minus;4,20") {
		t.Error("Gutschrift 4,20 ohne Schalter")
	}
}

// The Wärmepumpe has a Monatsverlauf like the Wallboxen (#305): one row per
// month with the segments "Stromkosten" and "PV-Anteil" in kWh.
func TestMonatsverlauf_Waermepumpe(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	dd, _, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	v := sichtbareAnsicht(zugriffMitDefaults(store.RolleNichtAngemeldet), dd, nil)
	if v.WP == nil {
		t.Fatal("Wärmepumpe fehlt in der anonymen Ansicht")
	}
	if v.WP.Verlauf.ID != "waermepumpe" || v.WP.Verlauf.Name != "Wärmepumpe" || len(v.WP.Verlauf.Eintraege) == 0 {
		t.Fatalf("Verlauf = %+v, want ID waermepumpe, Name Wärmepumpe und Einträge", v.WP.Verlauf)
	}
	var monat *dashboardSimpleMonat
	for _, e := range v.WP.Verlauf.Eintraege {
		if e.Monat != nil {
			monat = e.Monat
			break
		}
	}
	if monat == nil || !monat.HasWert {
		t.Fatalf("keine Monatszeile mit Wert: %+v", v.WP.Verlauf.Eintraege)
	}
	// 50 kWh Wärmepumpe of 100 kWh grid draw (Wohnung 2 30 kWh) are billed.
	if seg := segmentByLabel(monat.Segmente, "Stromkosten"); seg == nil || seg.Verbrauch != 50 || seg.Einheit != "kWh" {
		t.Errorf("Segment Stromkosten = %+v, want 50 kWh", seg)
	}
	if segmentByLabel(monat.Segmente, "PV-Anteil") == nil {
		t.Error("Segment PV-Anteil fehlt")
	}
}
