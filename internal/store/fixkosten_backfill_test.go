package store

import (
	"bytes"
	"database/sql"
	"log"
	"strings"
	"testing"
)

// fixkosten_backfill_test.go covers the step that copies prices and switches
// from the Ablesungen of a month into its Fixkosten-Eingabe (#361, spec #359).

// eingabeRoh inserts a bare Fixkosten-Eingabe (all prices empty, switches off).
func eingabeRoh(t *testing.T, db *sql.DB, monat string) int64 {
	t.Helper()
	res, err := db.Exec(`INSERT INTO fixkosten_eingaben (monat) VALUES (?)`, monat)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return id
}

// ablesungRoh inserts an Ablesung row with exactly the given prices and
// switches (nil = empty price).
func ablesungRoh(t *testing.T, db *sql.DB, datum, monat string, strom, frisch, abwasser, einsp *float64, wallbox, pv bool) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO periods (reading_date, monat, strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis, wallbox_komplett_w1, pv_gutschrift_w1)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, datum, monat, strom, frisch, abwasser, einsp, boolToInt(wallbox), boolToInt(pv)); err != nil {
		t.Fatal(err)
	}
}

type eingabeStand struct {
	strom, frisch, abwasser, einsp *float64
	wallbox, pv                    bool
}

func eingabeStandVon(t *testing.T, db *sql.DB, id int64) eingabeStand {
	t.Helper()
	var s eingabeStand
	var w, p int
	err := db.QueryRow(`SELECT strompreis, frischwasser_preis, abwasser_preis, einspeisung_preis, wallbox_komplett_w1, pv_gutschrift_w1
		FROM fixkosten_eingaben WHERE id = ?`, id).Scan(&s.strom, &s.frisch, &s.abwasser, &s.einsp, &w, &p)
	if err != nil {
		t.Fatal(err)
	}
	s.wallbox, s.pv = w != 0, p != 0
	return s
}

func gleich(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func pruefe(t *testing.T, name string, got eingabeStand, strom, frisch, abwasser, einsp *float64, wallbox, pv bool) {
	t.Helper()
	if !gleich(got.strom, strom) || !gleich(got.frisch, frisch) || !gleich(got.abwasser, abwasser) || !gleich(got.einsp, einsp) ||
		got.wallbox != wallbox || got.pv != pv {
		t.Errorf("%s: got %+v (wallbox %v, pv %v), want strom %v frisch %v abwasser %v einsp %v wallbox %v pv %v",
			name, got, got.wallbox, got.pv, deref(strom), deref(frisch), deref(abwasser), deref(einsp), wallbox, pv)
	}
}

func deref(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

func logAbfangen(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	alt := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(alt) })
	return &buf
}

func TestBackfillFixkostenEingaben_NimmtJuengsteAblesungDesMonats(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	id := eingabeRoh(t, db, "2026-07-01")
	ablesungRoh(t, db, "2026-07-10", "2026-07-01", Float64(0.20), Float64(1.40), Float64(4.00), Float64(0.05), false, false)
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", Float64(0.22), Float64(1.46), Float64(4.87), Float64(0.08), true, false)
	// Another month must not leak in.
	ablesungRoh(t, db, "2026-08-31", "2026-08-01", Float64(0.99), nil, nil, nil, true, true)

	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	pruefe(t, "Juli", eingabeStandVon(t, db, id), Float64(0.22), Float64(1.46), Float64(4.87), Float64(0.08), true, false)
}

func TestBackfillFixkostenEingaben_GleichesDatumEntscheidetDieId(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	id := eingabeRoh(t, db, "2026-07-01")
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", Float64(0.20), nil, nil, nil, false, false)
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", Float64(0.25), nil, nil, nil, false, false)

	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	pruefe(t, "Juli", eingabeStandVon(t, db, id), Float64(0.25), nil, nil, nil, false, false)
}

func TestBackfillFixkostenEingaben_PreisNimmtJuengstenNichtLeerenWert(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	id := eingabeRoh(t, db, "2026-07-01")
	ablesungRoh(t, db, "2026-07-10", "2026-07-01", Float64(0.22), Float64(1.46), Float64(4.87), Float64(0.08), true, true)
	// Teilstand: newest, no prices, switches differ. It deletes no price,
	// but its switches are the newest answer.
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", nil, nil, nil, nil, false, true)

	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	pruefe(t, "Juli", eingabeStandVon(t, db, id), Float64(0.22), Float64(1.46), Float64(4.87), Float64(0.08), false, true)
}

func TestBackfillFixkostenEingaben_MonatOhneAblesungBleibtLeer(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	id := eingabeRoh(t, db, "2026-05-01")
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", Float64(0.22), nil, nil, nil, true, true)

	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	pruefe(t, "Mai", eingabeStandVon(t, db, id), nil, nil, nil, nil, false, false)
}

func TestBackfillFixkostenEingaben_WiederholungAendertNichtsUndLaesstGefuelltes(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	leer := eingabeRoh(t, db, "2026-07-01")
	erfasst := eingabeRoh(t, db, "2026-08-01")
	ablesungRoh(t, db, "2026-07-31", "2026-07-01", Float64(0.22), Float64(1.46), nil, nil, true, false)
	ablesungRoh(t, db, "2026-08-31", "2026-08-01", Float64(0.30), Float64(2.00), Float64(5.00), Float64(0.10), true, true)
	// August was already entered on the Fixkosten-Eingabe: strompreis stays.
	if _, err := db.Exec(`UPDATE fixkosten_eingaben SET strompreis = 0.11 WHERE id = ?`, erfasst); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
			t.Fatalf("Lauf %d: %v", i+1, err)
		}
		pruefe(t, "Juli", eingabeStandVon(t, db, leer), Float64(0.22), Float64(1.46), nil, nil, true, false)
		pruefe(t, "August", eingabeStandVon(t, db, erfasst), Float64(0.11), Float64(2.00), Float64(5.00), Float64(0.10), true, true)
	}
}

func TestBackfillFixkostenEingaben_KonfliktErzeugtEineLogzeileJeMonat(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	eingabeRoh(t, db, "2026-07-01")
	eingabeRoh(t, db, "2026-08-01")
	eingabeRoh(t, db, "2026-09-01")
	// July: three Ablesungen, prices and switches differ -> one line.
	ablesungRoh(t, db, "2026-07-05", "2026-07-01", Float64(0.20), nil, nil, nil, false, false)
	ablesungRoh(t, db, "2026-07-15", "2026-07-01", Float64(0.21), nil, nil, nil, true, false)
	ablesungRoh(t, db, "2026-07-25", "2026-07-01", Float64(0.22), nil, nil, nil, true, true)
	// August: only a Teilstand without prices, same switches -> no conflict.
	ablesungRoh(t, db, "2026-08-05", "2026-08-01", Float64(0.30), nil, nil, nil, true, false)
	ablesungRoh(t, db, "2026-08-25", "2026-08-01", nil, nil, nil, nil, true, false)
	// September: only the switches differ -> a line.
	ablesungRoh(t, db, "2026-09-05", "2026-09-01", nil, nil, nil, nil, true, false)
	ablesungRoh(t, db, "2026-09-25", "2026-09-01", nil, nil, nil, nil, false, false)

	buf := logAbfangen(t)
	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if n := strings.Count(out, "2026-07-01"); n != 2 { // once per run
		t.Errorf("Logzeilen für Juli = %d, want 2 (eine je Lauf):\n%s", n, out)
	}
	if strings.Contains(out, "2026-08-01") {
		t.Errorf("August ist kein Konflikt, Log:\n%s", out)
	}
	if n := strings.Count(out, "2026-09-01"); n != 2 {
		t.Errorf("Logzeilen für September = %d, want 2:\n%s", n, out)
	}
}

func TestBackfillFixkostenEingaben_OhneQuellspaltenKeinFehler(t *testing.T) {
	db := datenbankMitAblesungsFeldern(t)
	id := eingabeRoh(t, db, "2026-07-01")
	if _, err := db.Exec(`ALTER TABLE periods DROP COLUMN wallbox_komplett_w1`); err != nil {
		t.Fatal(err)
	}
	if err := backfillFixkostenEingabenAusAblesungen(db); err != nil {
		t.Fatalf("fehlende Quellspalte: %v", err)
	}
	pruefe(t, "Juli", eingabeStandVon(t, db, id), nil, nil, nil, nil, false, false)
}

func TestMigrationen_BackfillLaeuftNachDenSpalten(t *testing.T) {
	index := map[string]int{}
	for i, m := range migrationen {
		index[m.name] = i
	}
	v, okV := index["ensureFixkostenEingabenPreisColumns"]
	n, okN := index["backfillFixkostenEingabenAusAblesungen"]
	if !okV || !okN || v >= n {
		t.Errorf("backfillFixkostenEingabenAusAblesungen muss nach ensureFixkostenEingabenPreisColumns laufen (%d, %d)", v, n)
	}
}
