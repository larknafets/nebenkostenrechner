package web

import (
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// dashboard_sonderkosten_test.go: umlegbare Sonderkosten count in the Summen
// of the Dashboard (Jahreskarte, Verlauf, Jahreszeile, Saldo), the same number
// as in the Jahresabrechnung (#406). Not umlegbar entries change nothing.

func sonderkostenDashboardDB(t *testing.T, eintraege ...store.Sonderkosten) (card dashboardJahresCard, verlauf dashboardVerlaufSpalte, body string) {
	t.Helper()
	db := openTestDB(t)
	for i, monat := range []string{"2026-10-01", "2026-11-01"} {
		in := seedPeriodInputAt(monat)
		in.Readings["strom_gesamt"] = float64(100 * i)
		if _, err := createPeriod(db, in); err != nil {
			t.Fatalf("createPeriod %s: %v", monat, err)
		}
	}
	for _, s := range eintraege {
		if _, err := store.CreateSonderkosten(db, s); err != nil {
			t.Fatalf("CreateSonderkosten: %v", err)
		}
	}
	card, verlauf = buildEntityView(mustLoadDashboard(t, db), nil, 2)
	return card, verlauf, dashboardHTML(t, db)
}

func verlaufMonat(t *testing.T, v dashboardVerlaufSpalte, label string) dashboardMonat {
	t.Helper()
	for _, e := range v.Eintraege {
		if e.Monat != nil && e.Monat.Label == label {
			return *e.Monat
		}
	}
	t.Fatalf("Verlauf hat keine Zeile %q", label)
	return dashboardMonat{}
}

func TestDashboard_SonderkostenInDenSummen(t *testing.T) {
	cardVorher, verlaufVorher, _ := sonderkostenDashboardDB(t)

	// Wohnung 2 bears 50 of the umlegbar house entry (Je Wohneinheit, Rest-Cent
	// at Wohnung 2) in November, 40 of its own in December (a month without
	// Ablesung and Fixkosteneingabe). The entry that is not umlegbar and the one
	// of Wohnung 1 change nothing for Wohnung 2.
	cardNachher, verlaufNachher, body := sonderkostenDashboardDB(t,
		store.Sonderkosten{Monat: "2026-11-01", Betreff: "Pumpe", Art: store.ArtReparatur, Betrag: 100.01, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikWohneinheit},
		store.Sonderkosten{Monat: "2026-12-01", Betreff: "Spülmaschine", Art: store.ArtNeuanschaffung, Betrag: 40, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true},
		store.Sonderkosten{Monat: "2026-11-01", Betreff: "Eigene Ausgabe", Art: store.ArtSonstiges, Betrag: 999, Umsetzung: store.UmsetzungWohnung2},
		store.Sonderkosten{Monat: "2026-11-01", Betreff: "Wohnung 1", Art: store.ArtReparatur, Betrag: 500, Umsetzung: store.UmsetzungWohnung1, Umlegbar: true},
	)

	if got, want := cardNachher.FixkostenEUR, calc.Round2(cardVorher.FixkostenEUR+50+40); got != want {
		t.Errorf("Jahreskarte Fixkosten = %v, want %v", got, want)
	}
	if got, want := cardNachher.GesamtEUR, calc.Round2(cardVorher.GesamtEUR+90); got != want {
		t.Errorf("Jahreskarte Gesamt = %v, want %v", got, want)
	}
	if got, want := verlaufMonat(t, verlaufNachher, "Nov 2026").FixkostenGesamt, calc.Round2(verlaufMonat(t, verlaufVorher, "Nov 2026").FixkostenGesamt+50); got != want {
		t.Errorf("Verlauf November Fixkosten = %v, want %v", got, want)
	}
	// The Saldo is the same cumulative number everywhere: -90 against before.
	if got, want := cardNachher.Saldo.Wert(), calc.Round2(cardVorher.Saldo.Wert()-90); got != want {
		t.Errorf("Saldo der Jahreskarte = %v, want %v", got, want)
	}
	if got, want := verlaufNachher.LatestSaldo.Wert(), cardNachher.Saldo.Wert(); got != want {
		t.Errorf("Saldo des Verlaufs = %v, Jahreskarte %v: nicht dieselbe Zahl", got, want)
	}
	dezember := verlaufMonat(t, verlaufNachher, "Dez 2026")
	if dezember.FixkostenFehlen || dezember.FixkostenGesamt != 40 || dezember.Saldo == nil {
		t.Errorf("Dezember nur mit Sonderkosten = %+v, want Zeile mit 40 € und Saldo, ohne Fixkosten fehlen", dezember)
	}
	// The HTTP page carries the sums (de-formatted like the card).
	if !strings.Contains(body, formatEuroDE(cardNachher.FixkostenEUR)) || strings.Contains(body, bannerFixkostenFehlen) {
		t.Errorf("Dashboard zeigt die Fixkosten %v nicht oder meldet Fixkosten fehlen", cardNachher.FixkostenEUR)
	}
}

// A Haus entry with Logik Je Anzahl Personen in a month without Fixkosten-
// eingabe is not calculable and counts nowhere.
func TestDashboard_SonderkostenPersonenOhneEingabeWirktNirgends(t *testing.T) {
	cardVorher, _, _ := sonderkostenDashboardDB(t)
	cardNachher, verlauf, _ := sonderkostenDashboardDB(t,
		store.Sonderkosten{Monat: "2026-12-01", Betreff: "Personen", Art: store.ArtSonstiges, Betrag: 300, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikPersonen},
	)
	if cardNachher.FixkostenEUR != cardVorher.FixkostenEUR || cardNachher.Saldo.Wert() != cardVorher.Saldo.Wert() {
		t.Errorf("Jahreskarte ändert sich: Fixkosten %v -> %v", cardVorher.FixkostenEUR, cardNachher.FixkostenEUR)
	}
	for _, e := range verlauf.Eintraege {
		if e.Monat != nil && e.Monat.Label == "Dez 2026" {
			t.Error("Verlauf hat eine Zeile für einen nicht berechenbaren Eintrag")
		}
	}
}

// A month with only Sonderkosten, newer than every Ablesung and Fixkosten-
// eingabe, is reachable: its year by the year arrows, its month in the Bis
// selection (#413). Entries that are not umlegbar or not calculable add no month.
func TestDashboard_NurSonderkostenMonatIstErreichbar(t *testing.T) {
	db := openTestDB(t)
	for i, monat := range []string{"2026-10-01", "2026-11-01"} {
		in := seedPeriodInputAt(monat)
		in.Readings["strom_gesamt"] = float64(100 * i)
		if _, err := createPeriod(db, in); err != nil {
			t.Fatalf("createPeriod %s: %v", monat, err)
		}
	}
	for _, s := range []store.Sonderkosten{
		{Monat: "2027-02-01", Betreff: "Dach", Art: store.ArtReparatur, Betrag: 40, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true},
		{Monat: "2027-03-01", Betreff: "Eigene", Art: store.ArtSonstiges, Betrag: 40, Umsetzung: store.UmsetzungWohnung2},
		{Monat: "2027-04-01", Betreff: "Personen", Art: store.ArtSonstiges, Betrag: 40, Umsetzung: store.UmsetzungHaus, Umlegbar: true, Logik: store.LogikPersonen},
	} {
		if _, err := store.CreateSonderkosten(db, s); err != nil {
			t.Fatalf("CreateSonderkosten: %v", err)
		}
	}
	dd := mustLoadDashboard(t, db)
	if dd.Jahr != 2027 || dd.NeuesJahr != 2027 || len(dd.BisMonate) != 1 || dd.BisMonate[0] != "2027-02-01" {
		t.Errorf("Jahr %d, BisMonate %v, want 2027 mit [2027-02-01]", dd.Jahr, dd.BisMonate)
	}
	if zurueck, vor := jahrLinks(dd); zurueck != "?jahr=2026" || vor != "" {
		t.Errorf("jahrLinks = %q, %q, want ?jahr=2026 und leer", zurueck, vor)
	}
}

// Without any Ablesung and Fixkosteneingabe the Dashboard still shows the
// umlegbare Sonderkosten (#413).
func TestDashboard_NurSonderkostenOhneAblesungUndFixkosten(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreateSonderkosten(db, store.Sonderkosten{Monat: "2026-05-01", Betreff: "Dach", Art: store.ArtReparatur, Betrag: 123, Umsetzung: store.UmsetzungWohnung2, Umlegbar: true}); err != nil {
		t.Fatalf("CreateSonderkosten: %v", err)
	}
	card, verlauf := buildEntityView(mustLoadDashboard(t, db), nil, 2)
	if card.FixkostenEUR != 123 || card.Saldo.Wert() != -123 {
		t.Errorf("Jahreskarte Fixkosten %v, Saldo %v, want 123 und -123", card.FixkostenEUR, card.Saldo.Wert())
	}
	if m := verlaufMonat(t, verlauf, "Mai 2026"); m.FixkostenGesamt != 123 {
		t.Errorf("Verlauf Mai = %+v, want 123 €", m)
	}
	if body := dashboardHTML(t, db); !strings.Contains(body, formatEuroDE(123)) {
		t.Error("Dashboard zeigt die Sonderkosten nicht")
	}
}
