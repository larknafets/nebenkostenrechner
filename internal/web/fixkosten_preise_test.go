package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// Tests for #362: form, detail and CSV of the Fixkosten-Eingabe carry the four
// prices and the two switches of the month.

var preisFelder = []string{"strompreis", "frischwasser_preis", "abwasser_preis", "einspeisung_preis"}

func TestFixkostenFormular_ZeigtGruppeLeerBeiErsterEingabe(t *testing.T) {
	db := openTestDB(t)
	body := getFixkostenForm(t, db, "")

	if !strings.Contains(body, "Preise und Einstellungen des Monats") {
		t.Error("Gruppe fehlt")
	}
	if !strings.Contains(body, "Die Einspeisevergütung dieses Monats wird Wohnung 1 gutgeschrieben") {
		t.Error("Erklärtext zur PV-Gutschrift fehlt")
	}
	for _, n := range preisFelder {
		tag := inputTag(t, body, n)
		if !strings.Contains(tag, "required") {
			t.Errorf("%s ist keine Pflicht: %s", n, tag)
		}
		if strings.Contains(tag, "value=") {
			t.Errorf("%s bei der ersten Eingabe nicht leer: %s", n, tag)
		}
	}
	for _, s := range store.Switches {
		if strings.Contains(inputTag(t, body, s.FormName), "checked") {
			t.Errorf("Häkchen %s bei der ersten Eingabe gesetzt", s.FormName)
		}
	}
	if strings.Index(body, "Preise und Einstellungen des Monats") > strings.Index(body, "<h3>Fixkosten</h3>") {
		t.Error("Gruppe steht nicht oberhalb der Positionen")
	}
}

func TestFixkostenFormular_BelegtAusLetzterEingabeVor(t *testing.T) {
	db := openTestDB(t)
	in := seedFixkostenInput("2026-05-01")
	in.Strompreis, in.FrischwasserPreis = store.Float64(0.31), store.Float64(1.9)
	in.AbwasserPreis, in.EinspeisungPreis = store.Float64(2.4), store.Float64(0.0786)
	in.PVGutschriftW1 = true
	if _, err := store.CreateFixkostenEingabe(db, in); err != nil {
		t.Fatal(err)
	}
	body := getFixkostenForm(t, db, "")
	for n, want := range map[string]string{"strompreis": "0.31", "frischwasser_preis": "1.9", "abwasser_preis": "2.4", "einspeisung_preis": "0.0786"} {
		if tag := inputTag(t, body, n); !strings.Contains(tag, `value="`+want+`"`) {
			t.Errorf("%s nicht vorbelegt mit %s: %s", n, want, tag)
		}
	}
	if !strings.Contains(inputTag(t, body, "pv_gutschrift_w1"), "checked") {
		t.Error("PV-Gutschrift nicht vorbelegt")
	}
	if strings.Contains(inputTag(t, body, "wallbox_komplett_w1"), "checked") {
		t.Error("Wallbox-Häkchen fälschlich gesetzt")
	}
}

func TestFixkostenFormular_SpeichertPreiseUndHaekchen(t *testing.T) {
	db := openTestDB(t)
	f := fixkostenForm("2026-09")
	f.Set("einspeisung_preis", "0")
	f.Set("wallbox_komplett_w1", "1")
	if w := postFixkosten(t, db, f); w.Code != http.StatusFound {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	all, _ := store.AllFixkostenEingabenDetails(db)
	d := all[0]
	if d.Strompreis == nil || *d.Strompreis != 0.3 || d.EinspeisungPreis == nil || *d.EinspeisungPreis != 0 {
		t.Errorf("Preise = %v %v, want 0.3 und 0 (erlaubt)", d.Strompreis, d.EinspeisungPreis)
	}
	if !d.WallboxKomplettW1 || d.PVGutschriftW1 {
		t.Errorf("Häkchen = %v %v, want an und aus", d.WallboxKomplettW1, d.PVGutschriftW1)
	}
}

func TestFixkostenFormular_PreiseSindPflichtUndGeprueft(t *testing.T) {
	for _, c := range []struct {
		name, feld, wert, want string
	}{
		{"fehlt", "strompreis", "", "ungültiger Preis für Strom: Preis fehlt"},
		{"keine Zahl", "frischwasser_preis", "abc", "ungültiger Preis für Frischwasser: Preis muss eine Zahl sein"},
		{"NaN", "abwasser_preis", "NaN", "ungültiger Preis für Abwasser: Preis muss eine Zahl sein"},
		{"negativ", "einspeisung_preis", "-0.01", "ungültiger Preis für Einspeisung: Preis darf nicht negativ sein"},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			f := fixkostenForm("2026-09")
			f.Set(c.feld, c.wert)
			w := postFixkosten(t, db, f)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("status %d body %q, want 400 mit %q", w.Code, w.Body.String(), c.want)
			}
			if all, _ := store.AllFixkostenEingaben(db); len(all) != 0 {
				t.Error("trotz Fehler gespeichert")
			}
		})
	}
}

func TestFixkostenFormular_UnvollstaendigeEingabeLaesstSichVervollstaendigen(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-09-01")) // no prices
	if err != nil {
		t.Fatal(err)
	}
	sid := strconv.FormatInt(id, 10)

	edit := getFixkostenForm(t, db, sid)
	if tag := inputTag(t, edit, "strompreis"); strings.Contains(tag, "value=") || !strings.Contains(tag, "required") {
		t.Errorf("Strompreis im Bearbeiten-Formular: %s", tag)
	}

	f := fixkostenForm("2026-09")
	f.Del("abwasser_preis")
	if w := postFixkostenForm(t, db, "/fixkosten/"+sid, f, sid); w.Code != http.StatusBadRequest {
		t.Fatalf("ohne Preis: status %d, want 400", w.Code)
	}
	if w := postFixkostenForm(t, db, "/fixkosten/"+sid, fixkostenForm("2026-09"), sid); w.Code != http.StatusFound {
		t.Fatalf("vollständig: status %d (%s)", w.Code, w.Body.String())
	}
	d, _ := store.GetFixkostenEingabeDetails(db, id)
	if d.Strompreis == nil || d.AbwasserPreis == nil || *d.AbwasserPreis != 2 {
		t.Errorf("Preise nach Vervollständigen: %v %v", d.Strompreis, d.AbwasserPreis)
	}
}

func TestFixkostenDetail_ZeigtPreiseUndHaekchen(t *testing.T) {
	zugaenge := map[string]*Zugang{
		"nicht angemeldet": newZugang("", nil),
		"mieter": NewZugang(festeRolle{rolle: store.RolleMieter},
			matrixAus{b: berechtigtMit(store.RolleMieter, store.RechtFixkostenAnsehen)}),
		"vermieter": newZugang("pw", nil),
	}
	for name, z := range zugaenge {
		t.Run(name, func(t *testing.T) {
			db := openTestDB(t)
			voll := seedFixkostenInput("2026-08-01")
			voll.Strompreis, voll.FrischwasserPreis = store.Float64(0.31), store.Float64(1.9)
			voll.AbwasserPreis, voll.EinspeisungPreis = store.Float64(2.4), store.Float64(0.0786)
			voll.PVGutschriftW1 = true
			idVoll, err := store.CreateFixkostenEingabe(db, voll)
			if err != nil {
				t.Fatal(err)
			}
			idLeer, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-09-01"))
			if err != nil {
				t.Fatal(err)
			}

			body := fixkostenDetailHTML(t, db, z, idVoll)
			for label, want := range map[string]string{"Strompreis": "0,31", "Frischwasser": "1,90", "Abwasser": "2,40", "Einspeisung": "0,0786"} {
				if row := tabellenzeile(t, body, label); !strings.Contains(row, want) {
					t.Errorf("%s: %q ohne %q", label, row, want)
				}
			}
			if strings.Contains(body, "nicht erfasst") {
				t.Error("vollständige Eingabe zeigt \"nicht erfasst\"")
			}
			if row := tabellenzeile(t, body, "PV-Gutschrift Wohnung 1"); !strings.Contains(row, "Ja") {
				t.Errorf("PV-Zeile %q ohne Ja", row)
			}
			if row := tabellenzeile(t, body, "Wallboxen vollständig Wohnung 1"); !strings.Contains(row, "Nein") {
				t.Errorf("Wallbox-Zeile %q ohne Nein", row)
			}

			leer := fixkostenDetailHTML(t, db, z, idLeer)
			for _, label := range []string{"Strompreis", "Frischwasser", "Abwasser", "Einspeisung"} {
				if row := tabellenzeile(t, leer, label); !strings.Contains(row, "nicht erfasst") {
					t.Errorf("%s: %q ohne \"nicht erfasst\"", label, row)
				}
			}
		})
	}
}

func TestFixkostenCSV_PreiseImportMitUndOhneSpalten(t *testing.T) {
	// With the six columns.
	db := openTestDB(t)
	csvText := strings.Join(fixkostenCsvHeader, ";") + "\n" + fixkostenCSVRow("2026-06-01", "100") + "\n"
	w := httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, csvText), db))
	if w.Code != http.StatusFound {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	all, _ := store.AllFixkostenEingabenDetails(db)
	d := all[0]
	if d.Strompreis == nil || *d.Strompreis != 0.3 || *d.FrischwasserPreis != 1.5 || *d.AbwasserPreis != 2 || *d.EinspeisungPreis != 0.08 {
		t.Errorf("Preise falsch: %v %v %v %v", d.Strompreis, d.FrischwasserPreis, d.AbwasserPreis, d.EinspeisungPreis)
	}
	if d.WallboxKomplettW1 || !d.PVGutschriftW1 {
		t.Errorf("Häkchen = %v %v, want aus und an", d.WallboxKomplettW1, d.PVGutschriftW1)
	}

	// Export writes all six.
	ex := httptest.NewRecorder()
	handleExportFixkostenCSV()(ex, requestWithDB(httptest.NewRequest(http.MethodGet, "/fixkosten/export.csv", nil), db))
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(ex.Body.String(), "\xEF\xBB\xBF")), "\n")
	if !strings.HasSuffix(lines[0], ";strompreis;frischwasser_preis;abwasser_preis;einspeisung_preis;wallbox_komplett_wohnung1;pv_gutschrift_wohnung1") {
		t.Errorf("Kopfzeile: %s", lines[0])
	}
	if !strings.HasSuffix(strings.TrimSpace(lines[1]), ";0,3;1,5;2;0,08;0;1") {
		t.Errorf("Datenzeile: %s", lines[1])
	}

	// Without the six columns: price = nicht erfasst, switch = aus.
	db = openTestDB(t)
	alt := fixkostenCsvHeader[:len(fixkostenCsvHeader)-6]
	altRow := strings.TrimSuffix(fixkostenCSVRow("2026-06-01", "100"), ";"+fixkostenCSVPreiseTail)
	w = httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, strings.Join(alt, ";")+"\n"+altRow+"\n"), db))
	if w.Code != http.StatusFound {
		t.Fatalf("ohne Spalten: status %d (%s)", w.Code, w.Body.String())
	}
	all, _ = store.AllFixkostenEingabenDetails(db)
	d = all[0]
	if d.Strompreis != nil || d.FrischwasserPreis != nil || d.AbwasserPreis != nil || d.EinspeisungPreis != nil || d.WallboxKomplettW1 || d.PVGutschriftW1 {
		t.Errorf("ohne Spalten: %+v, want nicht erfasst und aus", d)
	}

	// Empty cells inside the columns read the same.
	db = openTestDB(t)
	leer := strings.TrimSuffix(fixkostenCSVRow("2026-06-01", "100"), fixkostenCSVPreiseTail) + ";;;;;"
	w = httptest.NewRecorder()
	handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, strings.Join(fixkostenCsvHeader, ";")+"\n"+leer+"\n"), db))
	if w.Code != http.StatusFound {
		t.Fatalf("leere Zellen: status %d (%s)", w.Code, w.Body.String())
	}
	all, _ = store.AllFixkostenEingabenDetails(db)
	if all[0].Strompreis != nil || all[0].WallboxKomplettW1 {
		t.Errorf("leere Zellen: %+v", all[0])
	}
}

func TestFixkostenCSV_UngueltigePreiseWerdenAbgelehnt(t *testing.T) {
	for _, c := range []struct{ name, tail, want string }{
		{"keine Zahl", "x;1,5;2;0,08;0;1", `Zeile 2: ungültiger Preis für Strom: "x"`},
		{"negativ", "0,3;1,5;-2;0,08;0;1", `Zeile 2: ungültiger Preis für Abwasser: "-2" (Preis darf nicht negativ sein)`},
		{"Häkchen", "0,3;1,5;2;0,08;2;1", `Zeile 2: ungültiger Wert für wallbox_komplett_wohnung1: "2" (erlaubt: 0 oder 1)`},
	} {
		t.Run(c.name, func(t *testing.T) {
			db := openTestDB(t)
			row := strings.TrimSuffix(fixkostenCSVRow("2026-06-01", "100"), fixkostenCSVPreiseTail) + c.tail
			w := httptest.NewRecorder()
			handleImportFixkostenCSV()(w, requestWithDB(fixkostenCSVUploadRequest(t, strings.Join(fixkostenCsvHeader, ";")+"\n"+row+"\n"), db))
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), c.want) {
				t.Errorf("status %d body %q, want 400 mit %q", w.Code, w.Body.String(), c.want)
			}
			if all, _ := store.AllFixkostenEingaben(db); len(all) != 0 {
				t.Error("trotz Fehler importiert")
			}
		})
	}
}
