package web

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboard_fixkosten_fehlen_test.go: the Dashboard shows the state
// "Fixkosten fehlen" (#364): banner, own Monatsverlauf row, "vorläufig" on
// the Jahreskarte.

const (
	bannerFixkostenFehlen = `class="dashboard-hinweis"`
	linkFixkostenErfassen = `href="/fixkosten/neu">Fixkosten erfassen</a>`
)

// offeneMonateSetup stores four Ablesungen: September (first one, no Eingabe),
// October (no Fixkosten-Eingabe), November (Eingabe without prices), December
// (complete) and, as the one in the following year, none. Returns the id of
// the incomplete Eingabe of November.
func offeneMonateSetup(t *testing.T, db *sql.DB) (novemberEingabe int64) {
	t.Helper()
	for i, datum := range []string{"2026-09-01", "2026-10-01", "2026-11-01"} {
		in := seedPeriodInputAt(datum)
		in.Readings["strom_gesamt"] = float64(100 * i)
		if _, err := store.CreatePeriod(db, in); err != nil {
			t.Fatalf("CreatePeriod %s: %v", datum, err)
		}
	}
	id, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-11-01"))
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	dezember := seedPeriodInputAt("2026-12-01")
	dezember.Readings["strom_gesamt"] = 300
	if _, err := createPeriod(db, dezember); err != nil {
		t.Fatalf("CreatePeriod Dezember: %v", err)
	}
	return id
}

func zugaengeJeRolle() map[string]*Zugang {
	return map[string]*Zugang{
		"vermieter": newZugang("", nil),
		"mieter": NewZugang(festeRolle{rolle: store.RolleMieter, sitz: true},
			matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtFixkostenAnsehen)}),
		"nicht angemeldet": newZugang("geheim", nil),
	}
}

func TestDashboard_BannerBeiOffenenMonaten(t *testing.T) {
	for name, z := range zugaengeJeRolle() {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			offeneMonateSetup(t, db)
			body := dashboardHTMLMitAuth(t, db, z)

			if n := strings.Count(body, bannerFixkostenFehlen); n != 1 {
				t.Fatalf("Banner %d mal, want 1", n)
			}
			for _, s := range []string{
				"Für September 2026, Oktober 2026 und November 2026 fehlen die Fixkosten (oder sind unvollständig).",
				"Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht.",
			} {
				if !strings.Contains(body, s) {
					t.Errorf("Banner ohne %q", s)
				}
			}
			if got, want := strings.Contains(body, linkFixkostenErfassen), name == "vermieter"; got != want {
				t.Errorf("Link Fixkosten erfassen = %v, want %v", got, want)
			}
		})
	}
}

func TestDashboard_OhneOffeneMonateUnveraendert(t *testing.T) {
	db := openTestDB(t)
	wallboxPVAblesungen(t, db, false)
	body := dashboardHTML(t, db)
	for _, s := range []string{bannerFixkostenFehlen, "Fixkosten fehlen", "vorläufig", `class="verlauf-row verlauf-row-offen`} {
		if strings.Contains(body, s) {
			t.Errorf("%q im Dashboard ohne offene Monate", s)
		}
	}
}

func TestDashboard_VerlaufZeileOffenerMonat(t *testing.T) {
	db := openTestDB(t)
	novemberEingabe := offeneMonateSetup(t, db)
	body := dashboardHTML(t, db)

	// October has no Eingabe: own row with badge and link to the form, once
	// per apartment panel (two apartments).
	if n := strings.Count(body, `<span class="badge badge-warning">Fixkosten fehlen</span>`); n != 4 {
		t.Errorf("Badge Fixkosten fehlen %d mal, want 4 (September und Oktober, je Wohnung)", n)
	}
	if n := strings.Count(body, `class="verlauf-row verlauf-row-offen`); n != 4 {
		t.Errorf("Zeilen für offene Monate %d mal, want 4", n)
	}
	if !strings.Contains(body, linkFixkostenErfassen) {
		t.Error("Link Fixkosten erfassen in der Zeile fehlt")
	}

	dd, _, offen, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}
	want := []calc.OffenerMonat{{Monat: "2026-09-01"}, {Monat: "2026-10-01"}, {Monat: "2026-11-01", EingabeID: novemberEingabe}}
	if len(offen) != len(want) || offen[0] != want[0] || offen[1] != want[1] || offen[2] != want[2] {
		t.Fatalf("offene Monate = %+v, want %+v", offen, want)
	}

	_, spalte := buildEntityView(dd, offen, 2)
	var oktober, november *dashboardMonat
	for _, e := range spalte.Eintraege {
		if e.Monat == nil {
			continue
		}
		switch e.Monat.Label {
		case "Okt 2026":
			oktober = e.Monat
		case "Nov 2026":
			november = e.Monat
		}
	}
	if oktober == nil || !oktober.FixkostenFehlen || oktober.HasVerbrauch || oktober.HasFixkosten || oktober.HasKombiniert || oktober.Saldo != nil {
		t.Errorf("Oktober = %+v, want eigene Zeile ohne Balken und Saldo", oktober)
	}
	if november == nil || november.FixkostenFehlen || !november.VerbrauchOffen || !november.HasFixkosten || november.HasVerbrauch {
		t.Errorf("November = %+v, want Fixkosten und Verbrauch offen", november)
	}
	if !strings.Contains(body, `>offen</span>`) {
		t.Error(`"offen" beim Verbrauch des Novembers fehlt`)
	}
}

func TestDashboard_JahreskarteVorlaeufigBeiOffenemMonat(t *testing.T) {
	db := openTestDB(t)
	offeneMonateSetup(t, db)
	dd, _, offen, err := loadDashboardData(db)
	if err != nil {
		t.Fatalf("loadDashboardData: %v", err)
	}

	card, _ := buildEntityView(dd, offen, 2)
	if want := "vorläufig, ohne September 2026, Oktober 2026 und November 2026"; card.Vorlaeufig != want {
		t.Errorf("Vorlaeufig = %q, want %q", card.Vorlaeufig, want)
	}
	if card, _ := buildEntityView(dd, nil, 2); card.Vorlaeufig != "" {
		t.Errorf("Vorlaeufig ohne offene Monate = %q, want leer", card.Vorlaeufig)
	}

	body := dashboardHTML(t, db)
	// Gesamt and Saldo of both apartments carry the addition.
	if n := strings.Count(body, "vorläufig, ohne September 2026, Oktober 2026 und November 2026"); n < 2 {
		t.Errorf("Zusatz vorläufig %d mal, want mindestens je Wohnung für Gesamt", n)
	}
}

// A Teilstand and the first Ablesung without complete Eingabe are open months
// too (#380).
func TestDashboard_OffenerMonatAuchBeiTeilstand(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-09-01")); err != nil {
		t.Fatal(err)
	}
	teil := seedPeriodInputAt("2026-10-01")
	delete(teil.Readings, "strom_gesamt")
	if _, err := store.CreatePeriod(db, teil); err != nil {
		t.Fatal(err)
	}
	_, _, offen, err := loadDashboardData(db)
	if err != nil {
		t.Fatal(err)
	}
	want := []calc.OffenerMonat{{Monat: "2026-09-01"}, {Monat: "2026-10-01"}}
	if len(offen) != 2 || offen[0] != want[0] || offen[1] != want[1] {
		t.Errorf("offene Monate = %+v, want %+v", offen, want)
	}
}

func TestDashboard_JahreskarteNurOffeneMonateDesAnzeigejahres(t *testing.T) {
	dd := dashboardData{Jahr: 2026}
	offen := []calc.OffenerMonat{{Monat: "2025-12-01"}, {Monat: "2026-01-01"}}
	if got, want := vorlaeufigText(offen, dd.Jahr), "vorläufig, ohne Januar 2026"; got != want {
		t.Errorf("vorlaeufigText = %q, want %q", got, want)
	}
	if got := vorlaeufigText(offen[:1], dd.Jahr); got != "" {
		t.Errorf("vorlaeufigText ohne offenen Monat im Jahr = %q, want leer", got)
	}
}
