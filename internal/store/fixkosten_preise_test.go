package store

import (
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

// fixkosten_preise_test.go covers the four prices and two switches of the
// Fixkosten-Eingabe (#360, spec #359): persisted, overwritten, validated,
// present in the schema of a fresh and of a migrated database, and the query
// for the Ablesungen an Eingabe affects.

// seedFixkostenInput creates a Fixkosten-Eingabe for monat with the standard
// prices and both switches on, and returns its id. It writes only the
// Fixkosten-Eingabe side: the calculation still reads the Ablesung, so no
// existing expectation moves (the next ticket switches the reading over).
func seedFixkostenInput(t *testing.T, db *sql.DB, monat string) int64 {
	t.Helper()
	in := fixkostenInputMitPreisen(monat)
	id, err := CreateFixkostenEingabe(db, in)
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe %s: %v", monat, err)
	}
	return id
}

// fixkostenInputMitPreisen is a valid Fixkosten-Eingabe with the standard
// prices of completePeriodInput and both switches on.
func fixkostenInputMitPreisen(monat string) FixkostenInput {
	in := FixkostenInput{
		Monat:             monat,
		Personen:          map[int64]float64{1: 2, 2: 1},
		Strompreis:        Float64(0.22),
		FrischwasserPreis: Float64(1.46),
		AbwasserPreis:     Float64(4.87),
		EinspeisungPreis:  Float64(0.08),
	}
	for _, s := range Switches {
		*s.FixkostenInput(&in) = true
	}
	return in
}

func TestFixkostenEingabe_PreiseUndHaekchenRoundtrip(t *testing.T) {
	db := openTestDB(t)

	id := seedFixkostenInput(t, db, "2026-06-01")
	got, err := GetFixkostenEingabeDetails(db, id)
	if err != nil || got == nil {
		t.Fatalf("GetFixkostenEingabeDetails = %v, %v", got, err)
	}
	if orZero(got.Strompreis) != 0.22 || orZero(got.FrischwasserPreis) != 1.46 ||
		orZero(got.AbwasserPreis) != 4.87 || orZero(got.EinspeisungPreis) != 0.08 {
		t.Errorf("Preise = %v %v %v %v, want 0.22 1.46 4.87 0.08",
			got.Strompreis, got.FrischwasserPreis, got.AbwasserPreis, got.EinspeisungPreis)
	}
	for _, s := range Switches {
		if !*s.FixkostenDetail(got) {
			t.Errorf("Häkchen %s aus, want an", s.Name)
		}
	}

	all, err := AllFixkostenEingabenDetails(db)
	if err != nil || len(all) != 1 || orZero(all[0].Strompreis) != 0.22 {
		t.Errorf("AllFixkostenEingabenDetails = %+v, %v, want one with Strompreis 0.22", all, err)
	}
	for _, s := range Switches {
		if !*s.FixkostenDetail(all[0]) {
			t.Errorf("AllFixkostenEingabenDetails: Häkchen %s aus, want an", s.Name)
		}
	}
}

func TestFixkostenEingabe_LeerePreiseBleibenLeerHaekchenStandardAus(t *testing.T) {
	db := openTestDB(t)

	id := mustCreateFixkostenEingabe(t, db, "2026-06-01", nil)
	got, _ := GetFixkostenEingabeDetails(db, id)
	if got.Strompreis != nil || got.FrischwasserPreis != nil || got.AbwasserPreis != nil || got.EinspeisungPreis != nil {
		t.Errorf("Preise = %v %v %v %v, want alle nil (nicht erfasst)",
			got.Strompreis, got.FrischwasserPreis, got.AbwasserPreis, got.EinspeisungPreis)
	}
	for _, s := range Switches {
		if *s.FixkostenDetail(got) != s.Default {
			t.Errorf("Häkchen %s = %v, want Standard %v", s.Name, !s.Default, s.Default)
		}
	}
}

func TestFixkostenEingabe_UpdateUeberschreibtPreiseUndHaekchenImmer(t *testing.T) {
	db := openTestDB(t)
	id := seedFixkostenInput(t, db, "2026-06-01")

	// Ein leerer Preis überschreibt (anders als bei der Ablesung), falsch ist
	// eine echte Antwort für ein Häkchen.
	in := fixkostenInputMitPreisen("2026-06-01")
	in.Strompreis = nil
	in.AbwasserPreis = Float64(5.5)
	for _, s := range Switches {
		*s.FixkostenInput(&in) = false
	}
	if err := UpdateFixkostenEingabe(db, id, in); err != nil {
		t.Fatalf("UpdateFixkostenEingabe: %v", err)
	}
	got, _ := GetFixkostenEingabeDetails(db, id)
	if got.Strompreis != nil {
		t.Errorf("Strompreis = %v, want nil nach Überschreiben mit leer", *got.Strompreis)
	}
	if orZero(got.AbwasserPreis) != 5.5 {
		t.Errorf("AbwasserPreis = %v, want 5.5", got.AbwasserPreis)
	}
	for _, s := range Switches {
		if *s.FixkostenDetail(got) {
			t.Errorf("Häkchen %s an, want aus", s.Name)
		}
	}
}

func TestFixkostenInputValidate_Preise(t *testing.T) {
	preise := []struct {
		feld Feldart
		set  func(in *FixkostenInput, p *float64)
	}{
		{FeldStrompreis, func(in *FixkostenInput, p *float64) { in.Strompreis = p }},
		{FeldFrischwasserPreis, func(in *FixkostenInput, p *float64) { in.FrischwasserPreis = p }},
		{FeldAbwasserPreis, func(in *FixkostenInput, p *float64) { in.AbwasserPreis = p }},
		{FeldEinspeisungPreis, func(in *FixkostenInput, p *float64) { in.EinspeisungPreis = p }},
	}
	for _, p := range preise {
		for _, c := range []struct {
			name     string
			wert     *float64
			wantFehl bool
			wantWert string
		}{
			{"leer", nil, false, ""},
			{"null", Float64(0), false, ""},
			{"positiv", Float64(1.46), false, ""},
			{"negativ", Float64(-0.01), true, "-0.01"},
			{"NaN", Float64(math.NaN()), true, "NaN"},
			{"plus unendlich", Float64(math.Inf(1)), true, "+Inf"},
			{"minus unendlich", Float64(math.Inf(-1)), true, "-Inf"},
		} {
			in := gueltigeFixkosten()
			p.set(&in, c.wert)
			err := in.Validate()
			if !c.wantFehl {
				if err != nil {
					t.Errorf("Feld %d %s: Validate = %v, want nil", p.feld, c.name, err)
				}
				continue
			}
			var ef *EingabeFehler
			if !errors.As(err, &ef) || ef.Feld != p.feld || ef.Wert != c.wantWert {
				t.Errorf("Feld %d %s: Validate = %v, want Feld %d Wert %q", p.feld, c.name, err, p.feld, c.wantWert)
			}
		}
	}
}

func TestFixkostenEingabe_UngueltigerPreisWirdNichtGeschrieben(t *testing.T) {
	db := openTestDB(t)
	in := fixkostenInputMitPreisen("2026-06-01")
	in.Strompreis = Float64(-1)
	var ef *EingabeFehler
	if _, err := CreateFixkostenEingabe(db, in); !errors.As(err, &ef) || ef.Feld != FeldStrompreis {
		t.Errorf("CreateFixkostenEingabe = %v, want EingabeFehler FeldStrompreis", err)
	}
}

// spaltenVon lists a table's columns sorted, with type, not-null and default,
// to compare a fresh and a migrated schema.
func spaltenVon(t *testing.T, db *sql.DB, tabelle string) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name || ' ' || type || ' ' || "notnull" || ' ' || COALESCE(dflt_value, '-') FROM pragma_table_info(?)`, tabelle)
	if err != nil {
		t.Fatalf("table_info %s: %v", tabelle, err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	slices.Sort(out)
	return out
}

func TestFixkostenEingaben_SpaltenFrischUndMigriertGleich(t *testing.T) {
	neu := openTestDB(t)
	frisch := spaltenVon(t, neu, "fixkosten_eingaben")
	for _, want := range []string{
		"strompreis REAL 0 -", "frischwasser_preis REAL 0 -", "abwasser_preis REAL 0 -", "einspeisung_preis REAL 0 -",
		"wallbox_komplett_w1 INTEGER 1 0", "pv_gutschrift_w1 INTEGER 1 0",
	} {
		if !slices.Contains(frisch, want) {
			t.Errorf("frische Datenbank: Spalte %q fehlt in %v", want, frisch)
		}
	}

	// A database from before: the six columns are gone, the Eingabe stays.
	path := filepath.Join(t.TempDir(), "alt.db")
	alt, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := alt.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES ('2026-06-01')`); err != nil {
		t.Fatal(err)
	}
	for _, c := range []string{"strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis", "wallbox_komplett_w1", "pv_gutschrift_w1"} {
		if _, err := alt.Exec(`ALTER TABLE fixkosten_eingaben DROP COLUMN ` + c); err != nil {
			t.Fatalf("drop %s: %v", c, err)
		}
	}
	alt.Close()

	for i := 0; i < 2; i++ { // twice: the step is idempotent
		migriert, err := Open(path)
		if err != nil {
			t.Fatalf("Open (Lauf %d): %v", i+1, err)
		}
		if got := spaltenVon(t, migriert, "fixkosten_eingaben"); !reflect.DeepEqual(got, frisch) {
			t.Errorf("migriert (Lauf %d) = %v, want wie frisch %v", i+1, got, frisch)
		}
		details, err := AllFixkostenEingabenDetails(migriert)
		if err != nil || len(details) != 1 || details[0].Strompreis != nil {
			t.Errorf("bestehende Eingabe = %+v, %v, want eine mit leeren Preisen", details, err)
		}
		for _, s := range Switches {
			if *s.FixkostenDetail(details[0]) != s.Default {
				t.Errorf("bestehende Eingabe: Häkchen %s nicht auf Standard", s.Name)
			}
		}
		migriert.Close()
	}
}

func TestBetroffeneAblesungen(t *testing.T) {
	db := openTestDB(t)

	// Series: Ausgangsstand (June, no Vorperiode), two complete July
	// Ablesungen, one July Teilstand (newest, no prices), one August.
	mustCreatePeriod(t, db, "2026-06-30", baseReadings(nil))
	juli1 := mustInsertUnchecked(t, db, completePeriodInput("2026-07-10", baseReadings(nil)))
	juli2 := mustInsertUnchecked(t, db, completePeriodInput("2026-07-20", baseReadings(nil)))
	teil := completePeriodInput("2026-07-25", map[string]float64{})
	mustInsertUnchecked(t, db, teil)
	mustInsertUnchecked(t, db, completePeriodInput("2026-08-05", baseReadings(nil)))
	seedFixkostenInput(t, db, "2026-07-01")
	seedFixkostenInput(t, db, "2026-06-01")

	got, err := BetroffeneAblesungen(db, "2026-07-01")
	if err != nil {
		t.Fatalf("BetroffeneAblesungen: %v", err)
	}
	var ids []int64
	for _, p := range got {
		ids = append(ids, p.ID)
		if p.Monat != "2026-07-01" {
			t.Errorf("Ablesung %d Monat = %q, want 2026-07-01", p.ID, p.Monat)
		}
	}
	if !reflect.DeepEqual(ids, []int64{juli1, juli2}) {
		t.Errorf("betroffene Ablesungen = %v, want [%d %d] (vollständig, mit Vorperiode, ältere zuerst)", ids, juli1, juli2)
	}

	// The first Ablesung of the series has no Vorperiode: not berechenbar.
	if got, _ := BetroffeneAblesungen(db, "2026-06-01"); len(got) != 0 {
		t.Errorf("Ausgangsstand: %v, want leer", got)
	}
	if got, _ := BetroffeneAblesungen(db, "2026-12-01"); len(got) != 0 {
		t.Errorf("Monat ohne Ablesung: %v, want leer", got)
	}
}

func TestBetroffeneAblesungen_NurMitVollstaendigerEingabe(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-06-30", baseReadings(nil))
	juli := mustInsertUnchecked(t, db, completePeriodInput("2026-07-10", baseReadings(nil)))

	// No Eingabe for the month: nothing was berechenbar, nothing is affected.
	if got, _ := BetroffeneAblesungen(db, "2026-07-01"); len(got) != 0 {
		t.Errorf("ohne Eingabe: %v, want leer", got)
	}

	// An incomplete Eingabe (a price missing) does not make it berechenbar.
	in := fixkostenInputMitPreisen("2026-07-01")
	in.AbwasserPreis = nil
	id, err := CreateFixkostenEingabe(db, in)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := BetroffeneAblesungen(db, "2026-07-01"); len(got) != 0 {
		t.Errorf("unvollständige Eingabe: %v, want leer", got)
	}

	// Completing it makes the Ablesung affected.
	in.AbwasserPreis = Float64(4.87)
	if err := UpdateFixkostenEingabe(db, id, in); err != nil {
		t.Fatal(err)
	}
	got, _ := BetroffeneAblesungen(db, "2026-07-01")
	if len(got) != 1 || got[0].ID != juli {
		t.Errorf("vollständige Eingabe: %v, want [%d]", got, juli)
	}
}

// orZero reads a price that may be empty (nicht erfasst) as 0.
func orZero(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
