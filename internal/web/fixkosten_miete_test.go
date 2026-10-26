package web

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Tests for #277: Abschlag and Miete are recorded only for apartments with
// the Status Vermietung (default: Wohnung 2), through the HTTP handlers.

func postFixkostenForm(t *testing.T, db *sql.DB, path string, form url.Values, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	if id == "" {
		handleCreateFixkosten()(w, requestWithDB(req, db))
	} else {
		req.SetPathValue("id", id)
		handleUpdateFixkosten()(w, requestWithDB(req, db))
	}
	return w
}

func getFixkostenForm(t *testing.T, db *sql.DB, edit string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/fixkosten/neu", nil)
	w := httptest.NewRecorder()
	if edit == "" {
		handleFixkostenForm(newAuth("", nil))(w, requestWithDB(req, db))
	} else {
		req.SetPathValue("id", edit)
		handleFixkostenEditForm(newAuth("", nil))(w, requestWithDB(req, db))
	}
	if w.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body %s", w.Code, w.Body.String())
	}
	return w.Body.String()
}

func setStatus(t *testing.T, db *sql.DB, id int64, status string) {
	t.Helper()
	if _, err := db.Exec(`UPDATE apartments SET status = ? WHERE id = ?`, status, id); err != nil {
		t.Fatalf("set status: %v", err)
	}
}

func TestFixkostenFormular_AbschlagUndMieteNurBeiVermietung(t *testing.T) {
	db := openTestDB(t)

	body := getFixkostenForm(t, db, "")
	for _, want := range []string{`name="abschlag_2"`, `name="miete_2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("Formular ohne %s (Wohnung 2 ist vermietet)", want)
		}
	}
	for _, not := range []string{`name="abschlag_1"`, `name="miete_1"`} {
		if strings.Contains(body, not) {
			t.Errorf("Formular enthält %s, obwohl Wohnung 1 Eigennutzung ist", not)
		}
	}
	for _, name := range []string{"abschlag_2", "miete_2"} {
		if !feldPflicht(body, name) {
			t.Errorf("%s ist kein Pflichtfeld", name)
		}
	}

	setStatus(t, db, 2, store.StatusEigennutzung)
	body = getFixkostenForm(t, db, "")
	if strings.Contains(body, `name="abschlag_2"`) || strings.Contains(body, `name="miete_2"`) || strings.Contains(body, "Nebenkostenabschlag") {
		t.Error("beide Wohnungen Eigennutzung: Formular fragt trotzdem Abschlag/Miete")
	}
}

func feldPflicht(body, name string) bool {
	i := strings.Index(body, `name="`+name+`"`)
	if i < 0 {
		return false
	}
	j := strings.Index(body[i:], ">")
	return strings.Contains(body[i:i+j], "required")
}

func TestFixkostenSpeichern_PflichtfelderBeiVermietung(t *testing.T) {
	db := openTestDB(t)

	f := fixkostenForm("2026-09")
	f.Del("miete_2")
	if w := postFixkostenForm(t, db, "/fixkosten", f, ""); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "Miete") {
		t.Errorf("ohne Miete: status %d body %q, want 400 mit Hinweis auf die Miete", w.Code, w.Body.String())
	}
	f = fixkostenForm("2026-09")
	f.Del("abschlag_2")
	if w := postFixkostenForm(t, db, "/fixkosten", f, ""); w.Code != http.StatusBadRequest {
		t.Errorf("ohne Abschlag: status %d, want 400", w.Code)
	}

	// 0 is allowed.
	f = fixkostenForm("2026-09")
	f.Set("abschlag_2", "0")
	f.Set("miete_2", "0")
	if w := postFixkostenForm(t, db, "/fixkosten", f, ""); w.Code != http.StatusFound {
		t.Fatalf("mit 0: status %d, want 302 (%s)", w.Code, w.Body.String())
	}
	all, _ := store.AllFixkostenEingabenDetails(db)
	if len(all) != 1 {
		t.Fatalf("Eingaben = %d, want 1", len(all))
	}
	if v, ok := all[0].Miete[2]; !ok || v != 0 {
		t.Errorf("Miete Wohnung 2 = %v (erfasst %v), want 0 erfasst", v, ok)
	}
	// Eigennutzung: nothing recorded, the posted abschlag_1 is ignored.
	if _, ok := all[0].Abschlag[1]; ok {
		t.Error("Abschlag Wohnung 1 gespeichert, obwohl Eigennutzung")
	}
	if _, ok := all[0].Miete[1]; ok {
		t.Error("Miete Wohnung 1 gespeichert, obwohl Eigennutzung")
	}
}

func TestFixkostenBearbeiten_BehaeltAbschlagEinerEigennutzung(t *testing.T) {
	db := openTestDB(t)
	// An old entry from before: Wohnung 1 (now Eigennutzung) has an Abschlag.
	in := seedFixkostenInput("2026-08-01")
	in.Abschlag = map[int64]float64{1: 150, 2: 90}
	id, err := store.CreateFixkostenEingabe(db, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	sid := strconv.FormatInt(id, 10)

	body := getFixkostenForm(t, db, sid)
	if strings.Contains(body, `name="abschlag_1"`) {
		t.Error("Bearbeiten zeigt ein Feld für den Abschlag der Eigennutzung")
	}

	f := fixkostenForm("2026-08")
	f.Set("abschlag_1", "999") // a stale client must not overwrite it
	f.Set("abschlag_2", "95")
	if w := postFixkostenForm(t, db, "/fixkosten/"+sid, f, sid); w.Code != http.StatusFound {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	d, _ := store.GetFixkostenEingabeDetails(db, id)
	if d.Abschlag[1] != 150 {
		t.Errorf("Abschlag Wohnung 1 = %v, want unverändert 150", d.Abschlag[1])
	}
	if d.Abschlag[2] != 95 || d.Miete[2] != 750 {
		t.Errorf("Wohnung 2: Abschlag %v Miete %v, want 95 und 750", d.Abschlag[2], d.Miete[2])
	}
}

func TestFixkostenFormular_VorbelegungAusLetzterEingabe(t *testing.T) {
	db := openTestDB(t)
	if w := postFixkostenForm(t, db, "/fixkosten", fixkostenForm("2026-09"), ""); w.Code != http.StatusFound {
		t.Fatalf("create: %d", w.Code)
	}
	body := getFixkostenForm(t, db, "")
	if !strings.Contains(body, `name="miete_2" value="750"`) || !strings.Contains(body, `name="abschlag_2" value="100"`) {
		t.Errorf("Neu-Formular nicht aus der letzten Eingabe vorbelegt")
	}
}

func TestFixkostenFormular_MieteOhneErfassungLeer(t *testing.T) {
	db := openTestDB(t)
	// Entry from before the Miete existed: Abschlag recorded, Miete not.
	in := seedFixkostenInput("2026-08-01")
	in.Miete = nil
	if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	body := getFixkostenForm(t, db, "")
	if !strings.Contains(body, `name="miete_2" value=""`) {
		t.Error("nicht erfasste Miete ist nicht leer vorbelegt")
	}
}

func TestFixkostenCSV_ExportMitMieteSpalten(t *testing.T) {
	db := openTestDB(t)
	in := seedFixkostenInput("2026-06-01")
	in.Abschlag = map[int64]float64{2: 90}
	in.Miete = map[int64]float64{2: 800}
	if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	w := httptest.NewRecorder()
	handleExportFixkostenCSV()(w, requestWithDB(httptest.NewRequest(http.MethodGet, "/fixkosten/export.csv", nil), db))
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(w.Body.String(), "\xEF\xBB\xBF")), "\n")
	if len(lines) != 2 {
		t.Fatalf("Zeilen = %d, want 2", len(lines))
	}
	if !strings.HasSuffix(lines[0], ";personen_1;personen_2;abschlag_1;abschlag_2;miete_1;miete_2") {
		t.Errorf("Kopfzeile endet nicht mit Abschlag und Miete: %s", lines[0])
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), ";2;1;;90;;800") {
		t.Errorf("Datenzeile endet nicht mit leeren Zellen für nicht erfasst: %s", lines[1])
	}
}

func TestFixkostenCSV_ImportMieteLeereZelleUndAlteDatei(t *testing.T) {
	// Values are taken regardless of the status (Wohnung 1 is Eigennutzung).
	db := openTestDB(t)
	row := strings.Replace(fixkostenCSVRow("2026-06-01", "100"), ";2;1;150;90;;800", ";2;1;150;;300;", 1)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + row + "\n"
	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	all, _ := store.AllFixkostenEingabenDetails(db)
	d := all[0]
	if d.Abschlag[1] != 150 || d.Miete[1] != 300 {
		t.Errorf("Wohnung 1 trotz Eigennutzung: Abschlag %v Miete %v, want 150 und 300", d.Abschlag[1], d.Miete[1])
	}
	if _, ok := d.Abschlag[2]; ok {
		t.Error("leere Abschlag-Zelle als erfasst gelesen")
	}
	if _, ok := d.Miete[2]; ok {
		t.Error("leere Miete-Zelle als erfasst gelesen")
	}

	// An old file has no miete columns at all.
	db = openTestDB(t)
	alt := fixkostenCsvHeader[:len(fixkostenCsvHeader)-2]
	altRow := strings.TrimSuffix(fixkostenCSVRow("2026-06-01", "100"), ";;800")
	w = httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, strings.Join(alt, ";")+"\n"+altRow+"\n"), db))
	if w.Code != http.StatusFound {
		t.Fatalf("alte Datei: status %d (%s)", w.Code, w.Body.String())
	}
	all, _ = store.AllFixkostenEingabenDetails(db)
	if len(all[0].Miete) != 0 || all[0].Abschlag[1] != 150 {
		t.Errorf("alte Datei: Miete %v Abschlag %v, want keine Miete und Abschlag 150", all[0].Miete, all[0].Abschlag)
	}
}
