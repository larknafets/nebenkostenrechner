package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mime/multipart"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

var skKopf = strings.Join(sonderkostenCsvHeader, ";")

func postSonderkostenImport(t *testing.T, mux *http.ServeMux, cookies []*http.Cookie, csvText string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("csv", "sonderkosten.csv")
	_, _ = part.Write([]byte(csvText))
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/sonderkosten/import", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	attachCookies(req, cookies)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// skImportMux has a Fixkosteneingabe for 2026-10, the Personen-Logik needs it.
func skImportMux(t *testing.T) (*http.ServeMux, []*http.Cookie, func() []store.Sonderkosten) {
	t.Helper()
	mux, db := sonderkostenMux(t)
	if _, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	alle := func() []store.Sonderkosten {
		a, err := store.AllSonderkosten(db)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	return mux, loginCookies(t, mux, "vermieter", "geheim"), alle
}

func TestSonderkostenImport_UebernimmtGueltigeCSV(t *testing.T) {
	mux, c, alle := skImportMux(t)
	csvText := "\xEF\xBB\xBF" + skKopf + "\n" +
		"2026-10-01;Heizungspumpe;Pumpe;Reparatur;Meier GmbH;RE-412;1.000,50;Haus;1;personen\n" +
		"2026-09-01;Spülmaschine;;Neuanschaffung;;;649;Wohnung 2;1;\n" +
		"2025-03-01;Wasserhahn;;Reparatur;Lehmann;;184,3;Wohnung 1;0;\n"
	w := postSonderkostenImport(t, mux, c, csvText)
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/sonderkosten" {
		t.Fatalf("status %d, Location %q, body %s", w.Code, w.Header().Get("Location"), w.Body)
	}
	got := alle()
	if len(got) != 3 {
		t.Fatalf("%d Einträge, want 3", len(got))
	}
	p := got[2] // sorted by monat: 2025-03, 2026-09, 2026-10
	if p.Betreff != "Heizungspumpe" || p.Betrag != 1000.5 || p.Art != store.ArtReparatur || p.Objekt != store.ObjektHaus || !p.Umlegbar || p.Logik != store.LogikPersonen || p.Rechnungsnummer != "RE-412" {
		t.Errorf("Eintrag = %+v", p)
	}
}

// Exports from before the rename (#453) call the column umsetzung, the Header
// of an export now says objekt: both import, with the same result.
func TestSonderkostenImport_AlteUndNeueKopfzeile(t *testing.T) {
	neu := skKopf
	alt := strings.Replace(skKopf, "objekt", "umsetzung", 1)
	if alt == neu {
		t.Fatal("Kopfzeile enthält objekt nicht")
	}
	for name, kopf := range map[string]string{"objekt": neu, "umsetzung": alt} {
		t.Run(name, func(t *testing.T) {
			mux, c, alle := skImportMux(t)
			w := postSonderkostenImport(t, mux, c, kopf+"\n2026-09-01;Spülmaschine;;Neuanschaffung;;;649;Wohnung 2;1;\n")
			if w.Code != http.StatusFound {
				t.Fatalf("status %d, body %s", w.Code, w.Body)
			}
			if got := alle(); len(got) != 1 || got[0].Objekt != store.ObjektWohnung2 {
				t.Errorf("Einträge = %+v, want einer mit Objekt wohnung2", got)
			}
		})
	}
	t.Run("keine von beiden", func(t *testing.T) {
		mux, c, alle := skImportMux(t)
		kopf := strings.Replace(skKopf, "objekt", "bereich", 1)
		w := postSonderkostenImport(t, mux, c, kopf+"\n2026-09-01;Spülmaschine;;Neuanschaffung;;;649;Wohnung 2;1;\n")
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"objekt"`) {
			t.Errorf("status %d, body %q, want 400 mit Hinweis auf die Spalte objekt", w.Code, w.Body)
		}
		if n := len(alle()); n != 0 {
			t.Errorf("%d Einträge gespeichert, want 0", n)
		}
	})
}

func TestSonderkostenImport_FehlerBrichtAbWennNichtsGespeichert(t *testing.T) {
	gut := "2026-10-01;Pumpe;;Reparatur;;;10;Haus;1;wohneinheit\n"
	faelle := map[string]string{
		"Betrag 0":                      "2026-10-01;X;;Reparatur;;;0;Haus;0;\n",
		"Betrag unlesbar":               "2026-10-01;X;;Reparatur;;;abc;Haus;0;\n",
		"Art unbekannt":                 "2026-10-01;X;;Basteln;;;5;Haus;0;\n",
		"Objekt unbekannt":              "2026-10-01;X;;Reparatur;;;5;Keller;0;\n",
		"Monat ungültig":                "2026-13-01;X;;Reparatur;;;5;Haus;0;\n",
		"Betreff fehlt":                 "2026-10-01;;;Reparatur;;;5;Haus;0;\n",
		"Umlegbar unlesbar":             "2026-10-01;X;;Reparatur;;;5;Haus;ja;\n",
		"Logik fehlt bei Haus umlegbar": "2026-10-01;X;;Reparatur;;;5;Haus;1;\n",
		"Logik unbekannt":               "2026-10-01;X;;Reparatur;;;5;Haus;1;pauschal\n",
		"Logik bei Wohnung":             "2026-10-01;X;;Reparatur;;;5;Wohnung 1;1;qm\n",
		"Logik bei nicht umlegbar":      "2026-10-01;X;;Reparatur;;;5;Haus;0;qm\n",
		"Personen-Logik ohne Fixkosten": "2026-08-01;X;;Reparatur;;;5;Haus;1;personen\n",
		"zu wenige Spalten":             "2026-10-01;X\n",
	}
	for name, zeile := range faelle {
		t.Run(name, func(t *testing.T) {
			mux, c, alle := skImportMux(t)
			w := postSonderkostenImport(t, mux, c, skKopf+"\n"+gut+zeile)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status %d, want 400", w.Code)
			}
			if !strings.Contains(w.Body.String(), "Zeile 3") {
				t.Errorf("Meldung ohne Zeilennummer: %q", w.Body)
			}
			if n := len(alle()); n != 0 {
				t.Errorf("%d Einträge gespeichert, want 0", n)
			}
		})
	}
}

func TestSonderkostenImport_NurSolangeKeineSonderkostenExistieren(t *testing.T) {
	mux, db := sonderkostenMux(t, beispielSonderkosten[3])
	c := loginCookies(t, mux, "vermieter", "geheim")
	w := postSonderkostenImport(t, mux, c, skKopf+"\n2026-10-01;X;;Reparatur;;;5;Haus;0;\n")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", w.Code)
	}
	if a, _ := store.AllSonderkosten(db); len(a) != 1 {
		t.Errorf("%d Einträge, want 1", len(a))
	}
	if body := getMitCookies(t, mux, "/sonderkosten", c).Body.String(); strings.Contains(body, "/sonderkosten/import") {
		t.Error("Import-Formular sichtbar trotz vorhandener Sonderkosten")
	}
}

func TestSonderkostenImport_FormularNurFuerVermieterBeiLeererListe(t *testing.T) {
	mux, _ := sonderkostenMux(t)
	v := loginCookies(t, mux, "vermieter", "geheim")
	if body := getMitCookies(t, mux, "/sonderkosten", v).Body.String(); !strings.Contains(body, `action="/sonderkosten/import"`) {
		t.Error("Vermieter:in sieht das Import-Formular nicht")
	}
	m := loginCookies(t, mux, "mieter", "mieter")
	if w := postSonderkostenImport(t, mux, m, skKopf+"\n"); w.Code == http.StatusFound || w.Code == http.StatusBadRequest {
		t.Errorf("Mieter:in erreicht den Import (status %d)", w.Code)
	}
}
