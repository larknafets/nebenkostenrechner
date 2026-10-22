package store

import (
	"database/sql"
	"fmt"
	"sort"
	"time"
)

type Apartment struct {
	ID                int64
	Name              string
	QM                float64
	FlurstueckGroesse float64
	// MieterName/MieterAnschrift name the tenant (or resident) and their
	// delivery address (Issue #164). Personal data: shown and edited only
	// for logged-in users.
	MieterName      string
	MieterAnschrift string
	// Status is StatusVermietet or StatusEigennutzung. It only steers how
	// the Jahresabrechnung is presented and which Stammdaten it requires,
	// never the calculation.
	Status string
	// MieterSeit is the Abrechnungsmonat ("YYYY-MM-01") the current tenant
	// moved in, or "" for "since the start of the recording". Only meaningful
	// for a rented apartment. A single current value like MieterName, not
	// historized. Personal data: shown and edited only for logged-in users.
	MieterSeit string
}

// Wohnungsstatus values for apartments.status (Issue #164).
const (
	StatusVermietet    = "vermietet"
	StatusEigennutzung = "eigennutzung"
)

// validStatus reports whether s is a known Wohnungsstatus.
func validStatus(s string) bool {
	return s == StatusVermietet || s == StatusEigennutzung
}

// Apartments returns the 2 apartments ordered by id.
func Apartments(db *sql.DB) ([]Apartment, error) {
	return apartments(db)
}

// apartments is Apartments against a *sql.DB or a *sql.Tx.
func apartments(q querier) ([]Apartment, error) {
	rows, err := q.Query(`SELECT id, name, qm, flurstueck_groesse, mieter_name, mieter_anschrift, status, mieter_seit FROM apartments ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query apartments: %w", err)
	}
	defer rows.Close()

	var out []Apartment
	for rows.Next() {
		var a Apartment
		if err := rows.Scan(&a.ID, &a.Name, &a.QM, &a.FlurstueckGroesse, &a.MieterName, &a.MieterAnschrift, &a.Status, &a.MieterSeit); err != nil {
			return nil, fmt.Errorf("scan apartment: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// StammdatenInput is one apartment's Wohnungsgröße/Flurstücksgröße, as
// edited on the /stammdaten page (Issue #61) - unlike Personen/prices, these
// aren't part of a monthly Ablesung anymore: current values only, no
// history, no per-period freezing.
type StammdatenInput struct {
	QM                float64
	FlurstueckGroesse float64
}

func updateApartmentsTx(tx *sql.Tx, in map[int64]StammdatenInput) error {
	for apartmentID, s := range in {
		if _, err := tx.Exec(
			`UPDATE apartments SET qm = ?, flurstueck_groesse = ? WHERE id = ?`,
			s.QM, s.FlurstueckGroesse, apartmentID,
		); err != nil {
			return fmt.Errorf("update stammdaten for apartment %d: %w", apartmentID, err)
		}
	}
	return nil
}

// Haus is the house-wide Stammdaten (the single row of the haus table).
type Haus struct {
	// StromWeiterberechnen says whether the electricity consumption of
	// Wohnung 2 is passed on in the Jahresabrechnung (Issue #163). Not a
	// Betriebskostenart, only passed on by agreement.
	StromWeiterberechnen bool

	// WallboxWeiterberechnen says whether the Wallbox costs of a rented
	// apartment are passed on in its Jahresabrechnung (#228). Independent of
	// StromWeiterberechnen (the Haushaltsstrom). An apartment in
	// Eigennutzung shows its Wallbox costs regardless of the flag.
	WallboxWeiterberechnen bool

	// HeizungWaermeGewichtung is the share of the heat pump electricity cost
	// split by heat consumption, the rest by Wohnungsgröße (Issue #162):
	// one of HeizungGewichtungOptions. The contract fixes it for the whole
	// year, so it is a single Stammdaten value, not chosen per Ablesung.
	HeizungWaermeGewichtung float64

	// VermieterName/VermieterAnschrift/ObjektAnschrift/IBAN fill the head
	// and the payment note of the Jahresabrechnung (Issue #164). Personal
	// data: shown and edited only for logged-in users. IBAN is optional;
	// Kontoinhaber is the account holder shown with it, empty = Vermieter.
	VermieterName      string
	VermieterAnschrift string
	ObjektAnschrift    string
	IBAN               string
	Kontoinhaber       string
}

// GetHaus returns the house-wide Stammdaten.
func GetHaus(db *sql.DB) (Haus, error) {
	var h Haus
	if err := db.QueryRow(
		`SELECT strom_weiterberechnen, wallbox_weiterberechnen, heizung_waerme_gewichtung, vermieter_name, vermieter_anschrift, objekt_anschrift, iban, kontoinhaber FROM haus WHERE id = 1`,
	).Scan(&h.StromWeiterberechnen, &h.WallboxWeiterberechnen, &h.HeizungWaermeGewichtung, &h.VermieterName, &h.VermieterAnschrift, &h.ObjektAnschrift, &h.IBAN, &h.Kontoinhaber); err != nil {
		return Haus{}, fmt.Errorf("query haus: %w", err)
	}
	return h, nil
}

// HeizungGewichtungOptions are the only allowed heating-split weightings
// (70/30, 60/40, 50/50 by heat consumption / Wohnungsgröße), the lowest
// being the 50 % floor the HeizkostenV names for the consumption share.
var HeizungGewichtungOptions = []float64{0.7, 0.6, 0.5}

// ValidHeizungGewichtung reports whether v is one of HeizungGewichtungOptions.
func ValidHeizungGewichtung(v float64) bool {
	for _, o := range HeizungGewichtungOptions {
		if v == o {
			return true
		}
	}
	return false
}

// StammdatenFlags are the Stammdaten switches the Jahresabrechnung reads
// (Issue #163): which Kostenpositionen are umlagefähig and whether the
// electricity consumption of Wohnung 2 is passed on. Current values, no
// history - a change takes effect for every month and year.
type StammdatenFlags struct {
	// Umlagefaehig maps a kostenposition id to its flag. Only the given
	// positions are touched.
	Umlagefaehig           map[int64]bool
	StromWeiterberechnen   bool
	WallboxWeiterberechnen bool
}

func updateFlagsTx(tx *sql.Tx, in StammdatenFlags) error {
	for kostenpositionID, umlagefaehig := range in.Umlagefaehig {
		if _, err := tx.Exec(
			`UPDATE kostenpositionen SET umlagefaehig = ? WHERE id = ?`,
			boolToInt(umlagefaehig), kostenpositionID,
		); err != nil {
			return fmt.Errorf("update umlagefaehig for kostenposition %d: %w", kostenpositionID, err)
		}
	}
	if _, err := tx.Exec(`UPDATE haus SET strom_weiterberechnen = ?, wallbox_weiterberechnen = ? WHERE id = 1`, boolToInt(in.StromWeiterberechnen), boolToInt(in.WallboxWeiterberechnen)); err != nil {
		return fmt.Errorf("update weiterberechnen flags: %w", err)
	}
	return nil
}

// WohnungDetails is one apartment's tenant data and Wohnungsstatus as
// edited on /stammdaten (Issue #164).
type WohnungDetails struct {
	MieterName      string
	MieterAnschrift string
	Status          string
	// MieterSeit is "" or an Abrechnungsmonat ("YYYY-MM-01").
	MieterSeit string
}

// HausDetails is the house's Vermieter/Objekt/IBAN data as edited on
// /stammdaten (Issue #164).
type HausDetails struct {
	VermieterName      string
	VermieterAnschrift string
	ObjektAnschrift    string
	IBAN               string
	Kontoinhaber       string
}

// StammdatenSave is everything one /stammdaten form submission writes.
type StammdatenSave struct {
	Apartments map[int64]StammdatenInput
	Flags      StammdatenFlags
	// HeizungWaermeGewichtung must be one of HeizungGewichtungOptions.
	HeizungWaermeGewichtung float64
	// Wohnungen maps an apartment id to its details. Only the given
	// apartments are touched.
	Wohnungen map[int64]WohnungDetails
	Haus      HausDetails
}

// InvalidStatusError is returned by SaveStammdaten for a Wohnungsstatus that
// is neither vermietet nor eigennutzung.
type InvalidStatusError struct {
	ApartmentID int64
	Status      string
}

func (e *InvalidStatusError) Error() string {
	return fmt.Sprintf("invalid status %q for apartment %d", e.Status, e.ApartmentID)
}

// InvalidMieterSeitError is returned by SaveStammdaten for a "Mieter seit"
// that is not an Abrechnungsmonat ("YYYY-MM-01") or empty.
type InvalidMieterSeitError struct {
	ApartmentID int64
	MieterSeit  string
}

func (e *InvalidMieterSeitError) Error() string {
	return fmt.Sprintf("invalid mieter_seit %q for apartment %d", e.MieterSeit, e.ApartmentID)
}

// InvalidHeizungGewichtungError is returned by SaveStammdaten for a
// Heizungs-Gewichtung outside HeizungGewichtungOptions.
type InvalidHeizungGewichtungError struct {
	Gewichtung float64
}

func (e *InvalidHeizungGewichtungError) Error() string {
	return fmt.Sprintf("invalid Heizungs-Gewichtung %v", e.Gewichtung)
}

// validate checks every rule of a /stammdaten submission before anything is
// written, the one place for them: the Wohnungsstatus and "Mieter seit" per
// apartment (in id order), then the Heizungs-Gewichtung. A violation is one of
// the Invalid...Error types, so a caller can answer with 400 instead of 500.
func (in StammdatenSave) validate() error {
	ids := make([]int64, 0, len(in.Wohnungen))
	for id := range in.Wohnungen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		w := in.Wohnungen[id]
		if !validStatus(w.Status) {
			return &InvalidStatusError{ApartmentID: id, Status: w.Status}
		}
		if w.MieterSeit != "" && !validAbrechnungsmonat(w.MieterSeit) {
			return &InvalidMieterSeitError{ApartmentID: id, MieterSeit: w.MieterSeit}
		}
	}
	if !ValidHeizungGewichtung(in.HeizungWaermeGewichtung) {
		return &InvalidHeizungGewichtungError{Gewichtung: in.HeizungWaermeGewichtung}
	}
	return nil
}

// validAbrechnungsmonat reports whether s is an Abrechnungsmonat ("YYYY-MM-01").
func validAbrechnungsmonat(s string) bool {
	t, err := time.Parse("2006-01-02", s)
	return err == nil && t.Day() == 1 && len(s) == 10
}

// SaveStammdaten writes one /stammdaten form submission in a single
// transaction, so a failing write never leaves the form half saved. Invalid
// input is refused before anything is written (see validate).
func SaveStammdaten(db *sql.DB, in StammdatenSave) error {
	if err := in.validate(); err != nil {
		return err
	}
	return inTx(db, func(tx *sql.Tx) error {
		if err := updateApartmentsTx(tx, in.Apartments); err != nil {
			return err
		}
		if err := updateFlagsTx(tx, in.Flags); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE haus SET heizung_waerme_gewichtung = ? WHERE id = 1`, in.HeizungWaermeGewichtung); err != nil {
			return fmt.Errorf("update heizung_waerme_gewichtung: %w", err)
		}
		for apartmentID, w := range in.Wohnungen {
			if _, err := tx.Exec(
				`UPDATE apartments SET mieter_name = ?, mieter_anschrift = ?, status = ?, mieter_seit = ? WHERE id = ?`,
				w.MieterName, w.MieterAnschrift, w.Status, w.MieterSeit, apartmentID,
			); err != nil {
				return fmt.Errorf("update details for apartment %d: %w", apartmentID, err)
			}
		}
		if _, err := tx.Exec(
			`UPDATE haus SET vermieter_name = ?, vermieter_anschrift = ?, objekt_anschrift = ?, iban = ?, kontoinhaber = ? WHERE id = 1`,
			in.Haus.VermieterName, in.Haus.VermieterAnschrift, in.Haus.ObjektAnschrift, in.Haus.IBAN, in.Haus.Kontoinhaber,
		); err != nil {
			return fmt.Errorf("update haus details: %w", err)
		}
		return nil
	})
}
