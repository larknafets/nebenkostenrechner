package web

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestParseFixkostenMonat(t *testing.T) {
	got, err := parseFixkostenMonat("2026-09")
	if err != nil {
		t.Fatalf("parseFixkostenMonat: %v", err)
	}
	if got != "2026-09-01" {
		t.Errorf("parseFixkostenMonat(2026-09) = %q, want 2026-09-01", got)
	}

	if _, err := parseFixkostenMonat("not-a-month"); err == nil {
		t.Error("parseFixkostenMonat(not-a-month): want error, got nil")
	}
	if _, err := parseFixkostenMonat(""); err == nil {
		t.Error("parseFixkostenMonat(\"\"): want error, got nil")
	}
}

func TestMonatForInput(t *testing.T) {
	if got := monatForInput("2026-09-01"); got != "2026-09" {
		t.Errorf("monatForInput(2026-09-01) = %q, want 2026-09", got)
	}
	if got := monatForInput("garbage"); got != "" {
		t.Errorf("monatForInput(garbage) = %q, want empty string", got)
	}
}

func TestBuildFixkostenPositionRows(t *testing.T) {
	kostenpositionen := []store.Kostenposition{
		{ID: 1, Key: "grundsteuer", Label: "Grundsteuer"},
		{ID: 10, Key: "strom_grundpreis", Label: "Grundgebühr Strom"},
	}
	values := map[int64]store.FixkostenPositionWert{
		1:  {Logik: store.LogikQM, Typ: store.TypJaehrlich, Wert: 480},
		10: {Logik: store.LogikWohneinheit, Typ: store.TypMonatlich, Wert: 39.90},
	}

	rows := buildFixkostenPositionRows(kostenpositionen, values)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].ID != 1 || rows[0].Logik != store.LogikQM || rows[0].Typ != store.TypJaehrlich || rows[0].Wert != 480 {
		t.Errorf("rows[0] = %+v, want id:1 Logik:%s Typ:%s Wert:480", rows[0], store.LogikQM, store.TypJaehrlich)
	}
	if rows[1].ID != 10 || rows[1].Logik != store.LogikWohneinheit || rows[1].Typ != store.TypMonatlich || rows[1].Wert != 39.90 {
		t.Errorf("rows[1] = %+v, want id:10 Logik:%s Typ:%s Wert:39.90", rows[1], store.LogikWohneinheit, store.TypMonatlich)
	}
}

// TestBuildFixkostenPositionRows_OhneVorherigeEingabe covers #105/#108: the
// very first fixed-cost entry ever created has no values, but doesn't fall
// back to an empty Logik/Typ - instead it falls back to
// store.KostenpositionDefaults, the same starting values a freshly created
// Kostenposition year row used to get.
func TestBuildFixkostenPositionRows_OhneVorherigeEingabe(t *testing.T) {
	kostenpositionen := []store.Kostenposition{
		{ID: 1, Key: "grundsteuer", Label: "Grundsteuer"},
		{ID: 13, Key: "internet", Label: "Grundgebühr Internet"},
	}

	rows := buildFixkostenPositionRows(kostenpositionen, nil)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Logik != store.LogikQM || rows[0].Typ != store.TypJaehrlich || rows[0].Wert != 0 {
		t.Errorf("Grundsteuer ohne Vorwert = %+v, want Logik:%s Typ:%s Wert:0 (aus KostenpositionDefaults)", rows[0], store.LogikQM, store.TypJaehrlich)
	}
	if rows[1].Logik != store.LogikWohneinheit || rows[1].Typ != store.TypMonatlich || rows[1].Wert != 0 {
		t.Errorf("Internet ohne Vorwert = %+v, want Logik:%s Typ:%s Wert:0 (aus KostenpositionDefaults)", rows[1], store.LogikWohneinheit, store.TypMonatlich)
	}
}

func TestLogikLabels_CoversAllLogikKonstanten(t *testing.T) {
	for _, logik := range []string{store.LogikWohneinheit, store.LogikFlurstueck, store.LogikQM, store.LogikPersonen} {
		if logikLabels[logik] == "" {
			t.Errorf("logikLabels missing entry for %q", logik)
		}
	}
}
