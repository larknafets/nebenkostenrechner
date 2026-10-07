package calc_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	// Wohnungsgröße lives on apartments now (Issue #61), not the period - set it
	// once so every calc test that used to pass it per-period (e.g. Heizung's
	// Wohnungsgrößen-Verhältnis fallback) still has a value to read.
	saveStammdaten(t, db, func(s *store.StammdatenSave) {
		s.Apartments = map[int64]store.StammdatenInput{
			1: {QM: 116.23},
			2: {QM: 86},
		}
	})

	return db
}

// baseReadings returns a full 9-meter reading map, defaulting every meter
// to 0 except the overrides given.
func baseReadings(overrides map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(store.MeterKeys))
	for _, k := range store.MeterKeys {
		out[k] = 0
	}
	for k, v := range overrides {
		out[k] = v
	}
	return out
}

func mustCreatePeriod(t *testing.T, db *sql.DB, date string, strompreis float64, readings map[string]float64) int64 {
	t.Helper()
	id, err := store.CreatePeriod(db, store.PeriodInput{
		ReadingDate:       date,
		Monat:             monatVon(date),
		EinspeisungPreis:  store.Float64(0.08),
		Strompreis:        store.Float64(strompreis),
		FrischwasserPreis: store.Float64(1.46),
		AbwasserPreis:     store.Float64(4.87),
		Readings:          readings,
		Personen:          map[int64]int64{1: 2, 2: 1},
	})
	if err != nil {
		t.Fatalf("create period %s: %v", date, err)
	}
	return id
}

func TestStrom_SequentialAllocation(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.22, baseReadings(nil))
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.22, baseReadings(map[string]float64{
		"strom_gesamt":      18420,
		"strom_wohnung2":    6120,
		"strom_waermepumpe": 9840,
	}))

	got := kostenVon(t, db, p2).Strom

	if got.NetzbezugGesamtKWh != 18420 {
		t.Errorf("NetzbezugGesamtKWh = %v, want 18420", got.NetzbezugGesamtKWh)
	}
	if got.W2AnteilKWh != 6120 {
		t.Errorf("W2AnteilKWh = %v, want 6120", got.W2AnteilKWh)
	}
	if got.WPAnteilKWh != 9840 {
		t.Errorf("WPAnteilKWh = %v, want 9840 (Rest1=12300, gedeckelt auf Verbrauch)", got.WPAnteilKWh)
	}
	if got.KostenW2 != 1346.40 {
		t.Errorf("KostenW2 = %v, want 1346.40", got.KostenW2)
	}
	if got.KostenWPGesamtUnrounded != 2164.80 {
		t.Errorf("KostenWPGesamtUnrounded = %v, want 2164.80", got.KostenWPGesamtUnrounded)
	}
	if got.PVAnteilW2KWh != 0 || got.PVAnteilWPKWh != 0 {
		t.Errorf("ohne Deckelung sollte kein PV-Anteil anfallen, got W2=%v WP=%v", got.PVAnteilW2KWh, got.PVAnteilWPKWh)
	}
}

func TestStrom_WallboxDritteZuteilungsstufe(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.30, baseReadings(nil))
	// Grid draw 1000, apartment 2 400, heat pump 300 -> rest2 = 300 for the
	// wallbox, even though it would have consumed 500 kWh itself.
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.30, baseReadings(map[string]float64{
		"strom_gesamt":      1000,
		"strom_wohnung2":    400,
		"strom_waermepumpe": 300,
		"strom_wallbox":     500,
	}))

	got := kostenVon(t, db, p2).Strom
	if got.WallboxAnteilKWh != 300 {
		t.Errorf("WallboxAnteilKWh = %v, want 300 (gedeckelt auf Rest-Netzbezug nach W2+WP)", got.WallboxAnteilKWh)
	}
	if got.PVAnteilWallboxKWh != 200 {
		t.Errorf("PVAnteilWallboxKWh = %v, want 200 (500 Verbrauch - 300 angerechnet, durch PV gedeckt)", got.PVAnteilWallboxKWh)
	}
	if got.KostenWallbox != 90.00 {
		t.Errorf("KostenWallbox = %v, want 90.00 (300 * 0.30)", got.KostenWallbox)
	}
}

func TestStrom_W2VerbrauchKWh_TatsaechlicherWertOhnePVAbzug(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.30, baseReadings(nil))
	// Grid draw 400 < apartment 2's submeter 500 -> 100 kWh of apartment 2's
	// consumption is covered by PV, but is still actual consumption.
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.30, baseReadings(map[string]float64{
		"strom_gesamt":   400,
		"strom_wohnung2": 500,
	}))

	got := kostenVon(t, db, p2).Strom
	if got.W2AnteilKWh != 400 {
		t.Errorf("W2AnteilKWh = %v, want 400 (abgerechneter, auf Netzbezug gedeckelter Anteil)", got.W2AnteilKWh)
	}
	if got.W2VerbrauchKWh != 500 {
		t.Errorf("W2VerbrauchKWh = %v, want 500 (tatsächlicher Unterzähler-Verbrauch, ohne PV-Abzug)", got.W2VerbrauchKWh)
	}
}

func TestStrom_WallboxKeinRestUebrig(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.30, baseReadings(nil))
	// Apartment 2 + heat pump already consume the entire grid draw - wallbox
	// usage must then be fully covered by PV, cost 0.
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.30, baseReadings(map[string]float64{
		"strom_gesamt":      500,
		"strom_wohnung2":    300,
		"strom_waermepumpe": 200,
		"strom_wallbox":     150,
	}))

	got := kostenVon(t, db, p2).Strom
	if got.WallboxAnteilKWh != 0 {
		t.Errorf("WallboxAnteilKWh = %v, want 0", got.WallboxAnteilKWh)
	}
	if got.PVAnteilWallboxKWh != 150 {
		t.Errorf("PVAnteilWallboxKWh = %v, want 150 (kompletter Wallbox-Verbrauch durch PV gedeckt)", got.PVAnteilWallboxKWh)
	}
	if got.KostenWallbox != 0 {
		t.Errorf("KostenWallbox = %v, want 0", got.KostenWallbox)
	}
}

func TestStrom_WaermepumpeGedeckeltAufRest(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.22, baseReadings(nil))
	// Grid draw 1000, apartment 2 consumes 900 -> only 100 remains for the
	// heat pump, even though it would have consumed 5000 kWh itself.
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.22, baseReadings(map[string]float64{
		"strom_gesamt":      1000,
		"strom_wohnung2":    900,
		"strom_waermepumpe": 5000,
	}))

	got := kostenVon(t, db, p2).Strom
	if got.WPAnteilKWh != 100 {
		t.Errorf("WPAnteilKWh = %v, want 100 (gedeckelt auf Rest-Netzbezug)", got.WPAnteilKWh)
	}
	if got.PVAnteilWPKWh != 4900 {
		t.Errorf("PVAnteilWPKWh = %v, want 4900 (5000 Verbrauch - 100 angerechnet, durch PV gedeckt)", got.PVAnteilWPKWh)
	}
}

func TestStrom_PVUeberschussBeideNull(t *testing.T) {
	db := openTestDB(t)
	mustCreatePeriod(t, db, "2026-10-01", 0.22, baseReadings(nil))
	p2 := mustCreatePeriod(t, db, "2026-11-01", 0.22, baseReadings(map[string]float64{
		"strom_gesamt":      0,
		"strom_wohnung2":    300,
		"strom_waermepumpe": 400,
	}))

	got := kostenVon(t, db, p2).Strom
	if got.W2AnteilKWh != 0 || got.WPAnteilKWh != 0 {
		t.Errorf("bei Netzbezug=0 sollten beide Anteile 0 sein, got W2=%v WP=%v", got.W2AnteilKWh, got.WPAnteilKWh)
	}
	if got.KostenW2 != 0 || got.KostenWPGesamtUnrounded != 0 {
		t.Errorf("bei Netzbezug=0 sollten beide Kosten 0 sein, got KostenW2=%v KostenWP=%v", got.KostenW2, got.KostenWPGesamtUnrounded)
	}
	if got.PVAnteilW2KWh != 300 || got.PVAnteilWPKWh != 400 {
		t.Errorf("bei Netzbezug=0 sollte der volle Verbrauch dem PV-Anteil zugerechnet werden, got W2=%v WP=%v", got.PVAnteilW2KWh, got.PVAnteilWPKWh)
	}
}

// saveStammdaten applies mutate to a SaveStammdaten prefilled with the
// database's current house values, so a test changes only what it names
// (SaveStammdaten always writes the Heizungs-Gewichtung, the Strom flag and
// the house details).
func saveStammdaten(t *testing.T, db *sql.DB, mutate func(s *store.StammdatenSave)) {
	t.Helper()
	haus, err := store.GetHaus(db)
	if err != nil {
		t.Fatalf("GetHaus: %v", err)
	}
	s := store.StammdatenSave{
		Flags:                   store.StammdatenFlags{StromWeiterberechnen: haus.StromWeiterberechnen},
		HeizungWaermeGewichtung: haus.HeizungWaermeGewichtung,
		Haus: store.HausDetails{
			VermieterName: haus.VermieterName, VermieterAnschrift: haus.VermieterAnschrift,
			ObjektAnschrift: haus.ObjektAnschrift, IBAN: haus.IBAN, Kontoinhaber: haus.Kontoinhaber,
		},
	}
	mutate(&s)
	if err := store.SaveStammdaten(db, s); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}
}

// monatVon returns the Abrechnungsmonat ("YYYY-MM-01") of a reading date.
func monatVon(date string) string { return date[:8] + "01" }

// kostenVon loads the Verbrauchskosten and returns those of the Ablesung id,
// failing the test if it has none.
func kostenVon(t *testing.T, db *sql.DB, id int64) calc.Kosten {
	t.Helper()
	v, err := calc.Load(db)
	if err != nil {
		t.Fatalf("calc.Load: %v", err)
	}
	a, ok := v.Ablesung(id)
	if !ok || !a.Berechenbar() {
		t.Fatalf("Ablesung %d: ok=%v, Grund=%v, want berechenbar", id, ok, a.Grund)
	}
	return a.Kosten
}
