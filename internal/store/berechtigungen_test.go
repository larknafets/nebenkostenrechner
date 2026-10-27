package store

import "testing"

// A database without saved rights answers with the defaults: the behavior
// before the Berechtigungen existed, plus the new Wärmepumpe.
func TestGetBerechtigungen_Defaults(t *testing.T) {
	db := openTestDB(t)
	b, err := GetBerechtigungen(db)
	if err != nil {
		t.Fatalf("GetBerechtigungen: %v", err)
	}
	// Written out from the decision, not from Rechte.
	want := map[Recht][2]bool{ // {nicht angemeldet, mieter}
		RechtWohnung1:          {false, false},
		RechtWohnung2:          {true, true},
		RechtWaermepumpe:       {true, true},
		RechtWallboxen:         {false, false},
		RechtPVAnlage:          {false, false},
		RechtAblesungenAnsehen: {true, true},
		RechtAblesungAnlegen:   {true, true},
		RechtStammdatenAnsehen: {true, true},
		RechtFixkostenAnsehen:  {true, true},
		RechtMiete:             {false, false},
	}
	if len(Rechte) != len(want) {
		t.Fatalf("%d Rechte, want %d", len(Rechte), len(want))
	}
	for recht, w := range want {
		if got := b.Erlaubt(RolleNichtAngemeldet, recht); got != w[0] {
			t.Errorf("Nicht angemeldet %s = %v, want %v", recht, got, w[0])
		}
		if got := b.Erlaubt(RolleMieter, recht); got != w[1] {
			t.Errorf("Mieter %s = %v, want %v", recht, got, w[1])
		}
	}
}

func TestSaveStammdaten_BerechtigungenRoundtrip(t *testing.T) {
	db := openTestDB(t)
	b := DefaultBerechtigungen()
	b[RolleMieter][RechtWallboxen] = true
	b[RolleNichtAngemeldet][RechtWohnung2] = false
	if err := SaveStammdaten(db, StammdatenSave{HeizungWaermeGewichtung: 0.7, Berechtigungen: b}); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}
	got, err := GetBerechtigungen(db)
	if err != nil {
		t.Fatalf("GetBerechtigungen: %v", err)
	}
	if !got.Erlaubt(RolleMieter, RechtWallboxen) || got.Erlaubt(RolleNichtAngemeldet, RechtWohnung2) {
		t.Errorf("gespeicherte Rechte kommen nicht zurück: %+v", got)
	}
	if !got.Erlaubt(RolleMieter, RechtWohnung2) {
		t.Error("ungeänderter Wert Mieter/Wohnung 2 hat sich geändert")
	}

	// Saving again overwrites instead of failing on the primary key.
	b[RolleMieter][RechtWallboxen] = false
	if err := SaveStammdaten(db, StammdatenSave{HeizungWaermeGewichtung: 0.7, Berechtigungen: b}); err != nil {
		t.Fatalf("zweites SaveStammdaten: %v", err)
	}
	if got, _ := GetBerechtigungen(db); got.Erlaubt(RolleMieter, RechtWallboxen) {
		t.Error("zweites Speichern hat Mieter/Wallboxen nicht überschrieben")
	}
}

// Without Berechtigungen in the save (nil) the saved rights stay as they are.
func TestSaveStammdaten_OhneBerechtigungenLaesstSieUnberuehrt(t *testing.T) {
	db := openTestDB(t)
	b := DefaultBerechtigungen()
	b[RolleMieter][RechtPVAnlage] = true
	if err := SaveStammdaten(db, StammdatenSave{HeizungWaermeGewichtung: 0.7, Berechtigungen: b}); err != nil {
		t.Fatalf("SaveStammdaten: %v", err)
	}
	if err := SaveStammdaten(db, StammdatenSave{HeizungWaermeGewichtung: 0.7}); err != nil {
		t.Fatalf("SaveStammdaten ohne Berechtigungen: %v", err)
	}
	if got, _ := GetBerechtigungen(db); !got.Erlaubt(RolleMieter, RechtPVAnlage) {
		t.Error("Speichern ohne Berechtigungen hat Mieter/PV-Anlage zurückgesetzt")
	}
}

// A saved right the matrix no longer knows is ignored when reading.
func TestGetBerechtigungen_IgnoriertUnbekannteRechte(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.Exec(`INSERT INTO berechtigungen (recht, rolle, erlaubt) VALUES ('gibt_es_nicht', 'mieter', 1)`); err != nil {
		t.Fatal(err)
	}
	b, err := GetBerechtigungen(db)
	if err != nil {
		t.Fatalf("GetBerechtigungen: %v", err)
	}
	if _, ok := b[RolleMieter][Recht("gibt_es_nicht")]; ok {
		t.Error("unbekanntes Recht taucht in der Matrix auf")
	}
}
