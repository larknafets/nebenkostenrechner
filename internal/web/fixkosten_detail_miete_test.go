package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Tests for #281: the Fixkosten detail page shows the Miete of a rented
// apartment, without any permission check (like the Nebenkostenabschlag).

func fixkostenDetailHTML(t *testing.T, db *sql.DB, a *Zugang, id int64) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/fixkosten/"+strconv.FormatInt(id, 10), nil)
	req.SetPathValue("id", strconv.FormatInt(id, 10))
	w := httptest.NewRecorder()
	handleFixkostenDetail(a)(w, requestWithDB(req, db))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func mieteDetailEingabe(t *testing.T, db *sql.DB, monat string, miete map[int64]float64) int64 {
	t.Helper()
	in := seedFixkostenInput(monat)
	in.Miete = miete
	id, err := store.CreateFixkostenEingabe(db, in)
	if err != nil {
		t.Fatalf("CreateFixkostenEingabe: %v", err)
	}
	return id
}

// apartmentRow returns the <tr> of the top table that starts with name.
func apartmentRow(t *testing.T, body, name string) string {
	t.Helper()
	i := strings.Index(body, "<tr><td>"+name+"</td>")
	if i < 0 {
		t.Fatalf("Zeile %q fehlt", name)
	}
	j := strings.Index(body[i:], "</tr>")
	return body[i : i+j]
}

func TestFixkostenDetail_MieteNurBeiVermietung(t *testing.T) {
	db := openTestDB(t)
	// Wohnung 1 is Eigennutzung: a stored Miete must stay hidden.
	id := mieteDetailEingabe(t, db, "2026-01-01", map[int64]float64{1: 555.55, 2: 712.34})
	body := fixkostenDetailHTML(t, db, newZugang("", nil), id)

	if !strings.Contains(body, "<th>Miete</th>") {
		t.Error("Spalte Miete fehlt")
	}
	if row := apartmentRow(t, body, "Wohnung 2"); !strings.Contains(row, "712,34") {
		t.Errorf("Wohnung 2 ohne Miete 712,34: %s", row)
	}
	if strings.Contains(body, "555,55") {
		t.Error("Miete der Eigennutzung sichtbar")
	}
}

func TestFixkostenDetail_MieteNullUndNichtErfasst(t *testing.T) {
	db := openTestDB(t)
	zero := mieteDetailEingabe(t, db, "2026-01-01", map[int64]float64{2: 0})
	old := mieteDetailEingabe(t, db, "2026-02-01", nil)

	if row := apartmentRow(t, fixkostenDetailHTML(t, db, newZugang("", nil), zero), "Wohnung 2"); !strings.HasSuffix(row, "<td>0,00&nbsp;€</td>") {
		t.Errorf("erfasste 0 nicht als 0,00 €: %s", row)
	}
	row := apartmentRow(t, fixkostenDetailHTML(t, db, newZugang("", nil), old), "Wohnung 2")
	if !strings.HasSuffix(row, "<td>–</td>") {
		t.Errorf("nicht erfasste Miete nicht als Strich: %s", row)
	}
}

func TestFixkostenDetail_EigennutzungZelleLeer(t *testing.T) {
	db := openTestDB(t)
	id := mieteDetailEingabe(t, db, "2026-01-01", map[int64]float64{1: 555.55, 2: 712.34})
	row := apartmentRow(t, fixkostenDetailHTML(t, db, newZugang("", nil), id), "Wohnung 1")
	if !strings.HasSuffix(row, "<td></td>") {
		t.Errorf("Zelle der Eigennutzung nicht leer: %s", row)
	}
}

func TestFixkostenDetail_MieteOhneLoginSichtbar(t *testing.T) {
	db := openTestDB(t)
	id := mieteDetailEingabe(t, db, "2026-01-01", map[int64]float64{2: 712.34})
	body := fixkostenDetailHTML(t, db, newZugang("geheim", nil), id)
	if strings.Contains(body, "Eingabe korrigieren") {
		t.Fatal("Anonyme gelten als angemeldet, Test prüft nichts")
	}
	if !strings.Contains(body, "712,34") {
		t.Error("Miete ohne Login nicht sichtbar")
	}
}
