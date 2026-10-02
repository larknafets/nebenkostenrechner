package web

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// pruefeAbrechnungDB loads the data and checks it, the way the page does.
func pruefeAbrechnungDB(db *sql.DB, jahr int, apartmentID int64) (abrechnungPruefung, error) {
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		return abrechnungPruefung{}, err
	}
	return pruefeAbrechnungDaten(d.Pruef, jahr, ganzesJahr, apartmentID)
}

// berechneAbrechnungDB loads the data and computes the Abrechnung, the way
// the page does.
func berechneAbrechnungDB(db *sql.DB, jahr int, apartmentID int64) (abrechnungErgebnis, error) {
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		return abrechnungErgebnis{}, err
	}
	return berechneAbrechnung(db, d, jahr, ganzesJahr, apartmentID)
}

// pruefeAbrechnungAlteLadung is the loading of the Prüfung from before
// abrechnungDaten (per-table store calls, Eingaben as newest-first
// summaries), kept as the reference the new data path must agree with.
func pruefeAbrechnungAlteLadung(db *sql.DB, jahr int, apartmentID int64) (abrechnungPruefung, error) {
	periods, err := store.AllPeriodDetails(db)
	if err != nil {
		return abrechnungPruefung{}, err
	}
	eingaben, err := store.AllFixkostenEingaben(db)
	if err != nil {
		return abrechnungPruefung{}, err
	}
	apartments, err := store.Apartments(db)
	if err != nil {
		return abrechnungPruefung{}, err
	}
	haus, err := store.GetHaus(db)
	if err != nil {
		return abrechnungPruefung{}, err
	}
	return pruefeAbrechnungDaten(abrechnungPruefDaten{Periods: periods, Eingaben: eingaben, Apartments: apartments, Haus: haus}, jahr, ganzesJahr, apartmentID)
}

// vollePeriode is a complete Ablesung (every meter, prices, Personen).
func vollePeriode(id int64, datum, monat string) *store.LatestPeriod {
	readings := make(map[string]float64, len(store.MeterKeys))
	for _, k := range store.MeterKeys {
		readings[k] = 0
	}
	return &store.LatestPeriod{
		ID: id, ReadingDate: datum, Monat: monat,
		Strompreis: store.Float64(0.22), FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis: store.Float64(4.87), EinspeisungPreis: store.Float64(0.08),
		Readings:            readings,
		PersonenByApartment: map[int64]int64{1: 2, 2: 1},
	}
}

// teilstandPeriode is an Ablesung with only the date and Abrechnungsmonat.
func teilstandPeriode(id int64, datum, monat string) *store.LatestPeriod {
	return &store.LatestPeriod{ID: id, ReadingDate: datum, Monat: monat, Readings: map[string]float64{}, PersonenByApartment: map[int64]int64{}}
}

func monatKey(jahr int, m time.Month) string {
	return time.Date(jahr, m, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// monatlichePerioden returns one complete Ablesung per month from von to
// bis (inclusive, first of month), reading date the 28th, ids from startID.
func monatlichePerioden(startID int64, vonJahr int, vonMonat time.Month, bisJahr int, bisMonat time.Month) []*store.LatestPeriod {
	var out []*store.LatestPeriod
	id := startID
	for m := time.Date(vonJahr, vonMonat, 1, 0, 0, 0, 0, time.UTC); !m.After(time.Date(bisJahr, bisMonat, 1, 0, 0, 0, 0, time.UTC)); m = m.AddDate(0, 1, 0) {
		out = append(out, vollePeriode(id, m.Format("2006-01")+"-28", m.Format("2006-01-02")))
		id++
	}
	return out
}

func eingabenFuer(jahr int, von, bis time.Month) []store.FixkostenEingabeSummary {
	var out []store.FixkostenEingabeSummary
	for m := von; m <= bis; m++ {
		out = append(out, store.FixkostenEingabeSummary{ID: int64(jahr*100) + int64(m), Monat: monatKey(jahr, m)})
	}
	return out
}

func stammdatenOK() ([]store.Apartment, store.Haus) {
	return []store.Apartment{
		{ID: 1, Name: "Wohnung 1", QM: 88, FlurstueckGroesse: 600, Status: store.StatusEigennutzung},
		{ID: 2, Name: "Wohnung 2", QM: 42, FlurstueckGroesse: 400, Status: store.StatusVermietet, MieterName: "Erika Beispiel", MieterAnschrift: "Beispielweg 1\n12345 Musterstadt"},
	}, store.Haus{
		StromWeiterberechnen: true, HeizungWaermeGewichtung: 0.7,
		VermieterName: "Max Mustermann", VermieterAnschrift: "Hauptstraße 5", ObjektAnschrift: "Beispielweg 1",
	}
}

func maengelTexte(p abrechnungPruefung) []string {
	out := make([]string, len(p.Maengel))
	for i, m := range p.Maengel {
		out[i] = m.Text
	}
	return out
}

func arten(p abrechnungPruefung) []mangelArt {
	out := make([]mangelArt, len(p.Maengel))
	for i, m := range p.Maengel {
		out[i] = m.Art
	}
	return out
}

// vollesJahrDaten is a complete 2027 with the Ausgangsstand at 2026-12-28.
func vollesJahrDaten() abrechnungPruefDaten {
	apartments, haus := stammdatenOK()
	periods := monatlichePerioden(1, 2026, time.December, 2027, time.December)
	return abrechnungPruefDaten{Periods: periods, Eingaben: eingabenFuer(2027, time.January, time.December), Apartments: apartments, Haus: haus}
}

func TestPruefeAbrechnung_VollesJahrAbrechenbar(t *testing.T) {
	got, err := pruefeAbrechnungDaten(vollesJahrDaten(), 2027, ganzesJahr, 2)
	if err != nil {
		t.Fatalf("pruefeAbrechnungDaten: %v", err)
	}
	if !got.Abrechenbar() {
		t.Fatalf("Maengel = %v, want none", maengelTexte(got))
	}
	z := got.Zeitraum
	if z == nil || z.TeilJahr || z.Zusatz != "" {
		t.Fatalf("Zeitraum = %+v, want a full year without Zusatz", z)
	}
	if z.Von.Format("2006-01-02") != "2027-01-01" || z.Bis.Format("2006-01-02") != "2027-12-31" {
		t.Errorf("Zeitraum = %s bis %s, want 2027-01-01 bis 2027-12-31", z.Von.Format("2006-01-02"), z.Bis.Format("2006-01-02"))
	}
}

// A missing Nebenkostenabschlag is not part of the data the check reads at
// all (it counts as 0), so a year without any Abschlag is still abrechenbar:
// vollesJahrDaten carries no Abschläge.
func TestPruefeAbrechnung_FehlenderAbschlagIstKeinMangel(t *testing.T) {
	got, _ := pruefeAbrechnungDaten(vollesJahrDaten(), 2027, ganzesJahr, 2)
	for _, m := range got.Maengel {
		if strings.Contains(strings.ToLower(m.Text), "abschlag") {
			t.Errorf("unexpected finding about the Abschlag: %q", m.Text)
		}
	}
}

func TestPruefeAbrechnung_AblesungUndFixkosten(t *testing.T) {
	d := vollesJahrDaten()
	// Juni: no Ablesung. Dezember: only a Teilstand (replace the complete one).
	var periods []*store.LatestPeriod
	for _, p := range d.Periods {
		switch p.Monat {
		case "2027-06-01":
			continue
		case "2027-12-01":
			p = teilstandPeriode(p.ID, p.ReadingDate, p.Monat)
		}
		periods = append(periods, p)
	}
	d.Periods = periods
	// November: no Fixkosten-Eingabe. Februar: two.
	var eingaben []store.FixkostenEingabeSummary
	for _, e := range d.Eingaben {
		if e.Monat == "2027-11-01" {
			continue
		}
		eingaben = append(eingaben, e)
	}
	eingaben = append(eingaben, store.FixkostenEingabeSummary{ID: 999, Monat: "2027-02-01"})
	d.Eingaben = eingaben

	got, err := pruefeAbrechnungDaten(d, 2027, ganzesJahr, 2)
	if err != nil {
		t.Fatalf("pruefeAbrechnungDaten: %v", err)
	}
	want := []string{
		"Ablesung fehlt: Juni 2027",
		"Ablesung ist ein Teilstand: Dezember 2027",
		"Mehr als eine Fixkosten-Eingabe: Februar 2027",
		"Fixkosten-Eingabe fehlt: November 2027",
	}
	if fmt.Sprint(maengelTexte(got)) != fmt.Sprint(want) {
		t.Fatalf("Maengel = %v, want %v", maengelTexte(got), want)
	}
	wantArten := []mangelArt{mangelAblesungFehlt, mangelAblesungTeilstand, mangelFixkostenMehrfach, mangelFixkostenFehlt}
	if fmt.Sprint(arten(got)) != fmt.Sprint(wantArten) {
		t.Errorf("Arten = %v, want %v", arten(got), wantArten)
	}

	// Each finding links to where it is fixed.
	wantPfade := []string{"/ablesungen/neu", "/ablesungen/" + fmt.Sprint(d.Periods[len(d.Periods)-1].ID) + "/bearbeiten", "/fixkosten", "/fixkosten/neu"}
	for i, m := range got.Maengel {
		if m.Pfad != wantPfade[i] || m.Aktion == "" {
			t.Errorf("Mangel %d (%s): Pfad=%q Aktion=%q, want Pfad %q and an Aktion", i, m.Text, m.Pfad, m.Aktion, wantPfade[i])
		}
	}
}

// A Teilstand next to a complete Ablesung of the same month is no finding:
// the complete one counts, the Teilstand never does.
func TestPruefeAbrechnung_TeilstandNebenVollstaendigerAblesungIstKeinMangel(t *testing.T) {
	d := vollesJahrDaten()
	d.Periods = append(d.Periods, teilstandPeriode(500, "2027-12-30", "2027-12-01"))
	got, _ := pruefeAbrechnungDaten(d, 2027, ganzesJahr, 2)
	if !got.Abrechenbar() {
		t.Errorf("Maengel = %v, want none", maengelTexte(got))
	}
}

// Einzug im September: the first Ablesung (15 Sep) is the Ausgangsstand.
func ersteJahrDaten() abrechnungPruefDaten {
	apartments, haus := stammdatenOK()
	periods := []*store.LatestPeriod{vollePeriode(1, "2026-09-15", "2026-09-01")}
	periods = append(periods, monatlichePerioden(2, 2026, time.October, 2026, time.December)...)
	return abrechnungPruefDaten{Periods: periods, Eingaben: eingabenFuer(2026, time.October, time.December), Apartments: apartments, Haus: haus}
}

func TestPruefeAbrechnung_ErstesErfassungsjahr(t *testing.T) {
	got, err := pruefeAbrechnungDaten(ersteJahrDaten(), 2026, ganzesJahr, 2)
	if err != nil {
		t.Fatalf("pruefeAbrechnungDaten: %v", err)
	}
	if !got.Abrechenbar() {
		t.Fatalf("Maengel = %v, want none: no Fixkosten-Eingabe is needed in September, the month of the first Ablesung", maengelTexte(got))
	}
	z := got.Zeitraum
	if !z.TeilJahr || z.Von.Format("2006-01-02") != "2026-09-15" || z.Bis.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("Zeitraum = %+v, want a Teiljahr from 2026-09-15 to 2026-12-31", z)
	}
	if !strings.Contains(z.Zusatz, "Fixkosten und Vorauszahlungen ab Oktober 2026") {
		t.Errorf("Zusatz = %q, want it to say Fixkosten und Vorauszahlungen ab Oktober 2026", z.Zusatz)
	}

	t.Run("Fixkosten-Eingabe im ersten Monat ist erlaubt", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Eingaben = append(d.Eingaben, store.FixkostenEingabeSummary{ID: 1, Monat: "2026-09-01"})
		if got, _ := pruefeAbrechnungDaten(d, 2026, ganzesJahr, 2); !got.Abrechenbar() {
			t.Errorf("Maengel = %v, want none", maengelTexte(got))
		}
	})
	t.Run("zwei Eingaben im ersten Monat sind mehrdeutig", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Eingaben = append(d.Eingaben, store.FixkostenEingabeSummary{ID: 1, Monat: "2026-09-01"}, store.FixkostenEingabeSummary{ID: 2, Monat: "2026-09-01"})
		got, _ := pruefeAbrechnungDaten(d, 2026, ganzesJahr, 2)
		if fmt.Sprint(maengelTexte(got)) != "[Mehr als eine Fixkosten-Eingabe: September 2026]" {
			t.Errorf("Maengel = %v, want the duplicate September entry", maengelTexte(got))
		}
	})
	t.Run("ab dem Folgemonat ist die Eingabe Pflicht", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Eingaben = d.Eingaben[1:] // drop October
		got, _ := pruefeAbrechnungDaten(d, 2026, ganzesJahr, 2)
		if fmt.Sprint(maengelTexte(got)) != "[Fixkosten-Eingabe fehlt: Oktober 2026]" {
			t.Errorf("Maengel = %v, want Fixkosten-Eingabe fehlt: Oktober 2026", maengelTexte(got))
		}
	})
	t.Run("die erste Ablesung muss vollstaendig sein", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Periods[0] = teilstandPeriode(1, "2026-09-15", "2026-09-01")
		got, _ := pruefeAbrechnungDaten(d, 2026, ganzesJahr, 2)
		if fmt.Sprint(maengelTexte(got)) != "[Ablesung ist ein Teilstand: September 2026]" {
			t.Errorf("Maengel = %v, want the Teilstand in September", maengelTexte(got))
		}
	})
	t.Run("Folgejahr ist wieder ein volles Jahr", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Periods = append(d.Periods, monatlichePerioden(10, 2027, time.January, 2027, time.December)...)
		d.Eingaben = append(d.Eingaben, eingabenFuer(2027, time.January, time.December)...)
		got, _ := pruefeAbrechnungDaten(d, 2027, ganzesJahr, 2)
		if !got.Abrechenbar() || got.Zeitraum.TeilJahr || got.Zeitraum.Von.Format("2006-01-02") != "2027-01-01" {
			t.Errorf("2027: Maengel %v, Zeitraum %+v, want abrechenbar and a full year", maengelTexte(got), got.Zeitraum)
		}
	})
}

func TestPruefeAbrechnung_KeinZeitraum(t *testing.T) {
	apartments, haus := stammdatenOK()

	t.Run("noch keine Ablesung", func(t *testing.T) {
		got, _ := pruefeAbrechnungDaten(abrechnungPruefDaten{Apartments: apartments, Haus: haus}, 2027, ganzesJahr, 2)
		if got.Zeitraum != nil || fmt.Sprint(arten(got)) != fmt.Sprint([]mangelArt{mangelKeinZeitraum}) {
			t.Fatalf("got Zeitraum %+v, Arten %v, want no Zeitraum and one kein_zeitraum", got.Zeitraum, arten(got))
		}
		if m := got.Maengel[0]; !m.HatZiel() || m.Pfad != "/ablesungen/neu" {
			t.Errorf("finding: Pfad %q, want a link to /ablesungen/neu (the first Ablesung can be entered)", m.Pfad)
		}
	})
	t.Run("Jahr vor der ersten Ablesung", func(t *testing.T) {
		d := ersteJahrDaten()
		got, _ := pruefeAbrechnungDaten(d, 2025, ganzesJahr, 2)
		if got.Zeitraum != nil || len(got.Maengel) != 1 || !strings.Contains(got.Maengel[0].Text, "Für 2025 gibt es keine Daten") {
			t.Fatalf("got Zeitraum %+v, Maengel %v, want no Zeitraum and the 2025 message", got.Zeitraum, maengelTexte(got))
		}
		// Nothing can be entered or corrected for such a year, so there is
		// deliberately no link (PR #174 review).
		if m := got.Maengel[0]; m.HatZiel() || m.Aktion != "" {
			t.Errorf("finding %q has Pfad %q and Aktion %q, want none: it is not fixable in the app", m.Text, m.Pfad, m.Aktion)
		}
	})
	t.Run("ungueltiges Datum der ersten Ablesung verlinkt die Ablesung", func(t *testing.T) {
		d := ersteJahrDaten()
		d.Periods[0].ReadingDate = "kaputt"
		got, _ := pruefeAbrechnungDaten(d, 2026, ganzesJahr, 2)
		if got.Zeitraum != nil || len(got.Maengel) != 1 || !strings.Contains(got.Maengel[0].Text, "kaputt") {
			t.Fatalf("got Zeitraum %+v, Maengel %v, want no Zeitraum and the invalid-date message", got.Zeitraum, maengelTexte(got))
		}
		m := got.Maengel[0]
		if !m.HatZiel() || m.Pfad != "/ablesungen/1/bearbeiten" || m.Aktion != "Ablesung korrigieren" {
			t.Errorf("finding: Pfad %q Aktion %q, want a link to /ablesungen/1/bearbeiten labelled Ablesung korrigieren", m.Pfad, m.Aktion)
		}
	})
	t.Run("laufendes Jahr nennt die fehlenden Monate", func(t *testing.T) {
		d := ersteJahrDaten()
		got, _ := pruefeAbrechnungDaten(d, 2027, ganzesJahr, 2)
		if got.Abrechenbar() {
			t.Fatal("2027 without any data is abrechenbar, want findings")
		}
		if got.Maengel[0].Text != "Ablesung fehlt: Januar 2027" {
			t.Errorf("first finding = %q, want Ablesung fehlt: Januar 2027", got.Maengel[0].Text)
		}
	})
}

func TestPruefeAbrechnung_Stammdaten(t *testing.T) {
	run := func(mut func(a []store.Apartment, h *store.Haus), apartmentID int64) []string {
		d := vollesJahrDaten()
		mut(d.Apartments, &d.Haus)
		got, err := pruefeAbrechnungDaten(d, 2027, ganzesJahr, apartmentID)
		if err != nil {
			t.Fatalf("pruefeAbrechnungDaten: %v", err)
		}
		for _, m := range got.Maengel {
			if m.Art != mangelStammdaten || m.Pfad != "/stammdaten" {
				t.Errorf("finding %q: Art %q Pfad %q, want a Stammdaten finding linking /stammdaten", m.Text, m.Art, m.Pfad)
			}
		}
		return maengelTexte(got)
	}

	t.Run("alles leer bei vermieteter Wohnung", func(t *testing.T) {
		got := run(func(a []store.Apartment, h *store.Haus) {
			*h = store.Haus{}
			a[0].QM, a[1].QM = 0, 0
			a[1].MieterName, a[1].MieterAnschrift = "", ""
		}, 2)
		want := []string{
			"Name des Vermieters fehlt", "Anschrift des Vermieters fehlt", "Anschrift des Objekts fehlt",
			"Wohnungsgröße von Wohnung 1 fehlt (muss größer als 0 sein)", "Wohnungsgröße von Wohnung 2 fehlt (muss größer als 0 sein)",
			"Name des Mieters von Wohnung 2 fehlt", "Zustellanschrift des Mieters von Wohnung 2 fehlt",
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("Maengel = %v, want %v", got, want)
		}
	})
	t.Run("nur Leerzeichen zaehlt als leer", func(t *testing.T) {
		got := run(func(a []store.Apartment, h *store.Haus) { h.VermieterName = "   \n " }, 2)
		if fmt.Sprint(got) != "[Name des Vermieters fehlt]" {
			t.Errorf("Maengel = %v, want only the Vermieter name", got)
		}
	})
	t.Run("Eigennutzung braucht keinen Mieter", func(t *testing.T) {
		got := run(func(a []store.Apartment, h *store.Haus) {
			a[0].MieterName, a[0].MieterAnschrift = "", ""
		}, 1)
		if len(got) != 0 {
			t.Errorf("Maengel = %v, want none for the Eigennutzung apartment", got)
		}
	})
	t.Run("Mieter der anderen Wohnung ist egal, IBAN ist optional", func(t *testing.T) {
		got := run(func(a []store.Apartment, h *store.Haus) {
			a[1].MieterName = ""
			h.IBAN = ""
		}, 1)
		if len(got) != 0 {
			t.Errorf("Maengel = %v, want none: Wohnung 1 is checked, and the IBAN is optional", got)
		}
	})
}

func TestPruefeAbrechnung_UnbekannteWohnung(t *testing.T) {
	if _, err := pruefeAbrechnungDaten(vollesJahrDaten(), 2027, ganzesJahr, 3); err == nil {
		t.Error("unknown apartment: err = nil, want an error")
	}
}

// TestPruefeAbrechnung_Demodaten runs the loading function against the demo
// database: months 2023-08 to 2026-10, a complete Ablesung and Fixkosten-
// Eingabe for each, made-up Stammdaten.
func TestPruefeAbrechnung_Demodaten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}

	for _, tc := range []struct {
		jahr     int
		teilJahr bool
	}{{2023, true}, {2024, false}, {2025, false}} {
		got, err := pruefeAbrechnungDB(db, tc.jahr, 2)
		if err != nil {
			t.Fatalf("pruefeAbrechnung(%d): %v", tc.jahr, err)
		}
		if !got.Abrechenbar() {
			t.Errorf("%d: Maengel = %v, want none", tc.jahr, maengelTexte(got))
		}
		if got.Zeitraum == nil || got.Zeitraum.TeilJahr != tc.teilJahr {
			t.Errorf("%d: Zeitraum = %+v, want TeilJahr %v", tc.jahr, got.Zeitraum, tc.teilJahr)
		}
	}

	if got, _ := pruefeAbrechnungDB(db, 2026, 2); got.Abrechenbar() || got.Maengel[0].Text != "Ablesung fehlt: November 2026" {
		t.Errorf("2026: Maengel = %v, want findings starting with the missing November Ablesung", maengelTexte(got))
	}
	if got, _ := pruefeAbrechnungDB(db, 2022, 2); got.Zeitraum != nil || got.Abrechenbar() {
		t.Errorf("2022: Zeitraum %+v, Maengel %v, want no Zeitraum", got.Zeitraum, maengelTexte(got))
	}
}

// TestStandardJahrUndPruefungAufGeladenenDaten: the preselection and the
// check work on the same loaded data and agree with the check per year: the
// preselected year is the newest one whose check passes, else the newest year
// (Demodaten: 2026 has Mängel, 2025 is the newest complete year).
func TestStandardJahrUndPruefungAufGeladenenDaten(t *testing.T) {
	db := openTestDB(t)
	if err := store.SeedDemoData(db, time.Date(2026, time.October, 15, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("SeedDemoData: %v", err)
	}
	d, err := ladeAbrechnungDaten(db)
	if err != nil {
		t.Fatalf("ladeAbrechnungDaten: %v", err)
	}

	jahre := abrechnungJahre(d.Pruef)
	if len(jahre) == 0 || jahre[0] != 2026 {
		t.Fatalf("abrechnungJahre = %v, want newest first starting with 2026", jahre)
	}
	for _, apartmentID := range []int64{1, 2} {
		got, err := standardJahr(d.Pruef, jahre, apartmentID)
		if err != nil {
			t.Fatalf("standardJahr(%d): %v", apartmentID, err)
		}
		if got != 2025 {
			t.Errorf("standardJahr(Wohnung %d) = %d, want 2025", apartmentID, got)
		}
		for _, y := range jahre {
			fromDB, err := pruefeAbrechnungAlteLadung(db, y, apartmentID)
			if err != nil {
				t.Fatalf("pruefeAbrechnungAlteLadung(%d): %v", y, err)
			}
			onData, err := pruefeAbrechnungDaten(d.Pruef, y, ganzesJahr, apartmentID)
			if err != nil {
				t.Fatalf("pruefeAbrechnungDaten(%d): %v", y, err)
			}
			if !reflect.DeepEqual(fromDB, onData) {
				t.Errorf("Wohnung %d, %d: Prüfung differs: %+v vs %+v", apartmentID, y, fromDB, onData)
			}
			if y >= got && onData.Abrechenbar() != (y == got) {
				t.Errorf("Wohnung %d, %d: Abrechenbar = %v, preselection %d", apartmentID, y, onData.Abrechenbar(), got)
			}
		}
	}
}
