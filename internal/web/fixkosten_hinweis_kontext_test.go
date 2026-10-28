package web

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkosten_hinweis_kontext_test.go: Text, Link and Linktext of the note on the
// rendered page per Kontext and state of the Eingabe (#384).

func pruefeHinweisSeite(t *testing.T, was, body, text, pfad, linkText string) {
	t.Helper()
	if !strings.Contains(body, text) {
		t.Errorf("%s: Text %q fehlt", was, text)
	}
	if want := fmt.Sprintf(`href="%s">%s</a>`, pfad, linkText); !strings.Contains(body, want) {
		t.Errorf("%s: Link %q fehlt", was, want)
	}
}

func TestFixkostenHinweis_DashboardKeineUndUnvollstaendigeEingabe(t *testing.T) {
	db := openTestDB(t)
	if _, err := store.CreatePeriod(db, seedPeriodInputAt("2026-10-01")); err != nil {
		t.Fatal(err)
	}
	text := "Für Oktober 2026 fehlen die Fixkosten (oder sind unvollständig). Die Ablesungen dieser Monate sind noch nicht berechnet, Jahressummen und Saldo enthalten sie nicht."
	pruefeHinweisSeite(t, "Dashboard, keine Eingabe", dashboardHTML(t, db), text, "/fixkosten/neu", "Fixkosten erfassen")

	id, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	pruefeHinweisSeite(t, "Dashboard, unvollständig", dashboardHTML(t, db), text, fmt.Sprintf("/fixkosten/%d/bearbeiten", id), "Fixkosten ergänzen")
}

func TestFixkostenHinweis_AblesungDetailKeineUndUnvollstaendigeEingabe(t *testing.T) {
	db := openTestDB(t)
	id, err := store.CreatePeriod(db, seedPeriodInputAt("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	z := newZugang("", nil)
	text := "Für Oktober 2026 fehlen die Fixkosten (oder sind unvollständig). Die Ablesung ist noch nicht berechnet, bis die Fixkosten erfasst sind."
	pruefeHinweisSeite(t, "Ablesung, keine Eingabe", detailBodyMit(t, z, db, id, "?fixkosten_fehlen=1"), text, "/fixkosten/neu", "Fixkosten erfassen")

	eid, err := store.CreateFixkostenEingabe(db, seedFixkostenInput("2026-10-01"))
	if err != nil {
		t.Fatal(err)
	}
	pruefeHinweisSeite(t, "Ablesung, unvollständig", detailBodyMit(t, z, db, id, "?fixkosten_fehlen=1"), text, fmt.Sprintf("/fixkosten/%d/bearbeiten", eid), "Fixkosten ergänzen")
}

// The CSV note is a plain message in the redirect, it carries no link.
func TestFixkostenHinweis_CSVImportKeineUndUnvollstaendigeEingabe(t *testing.T) {
	text := "Für Juni 2026 fehlen die Fixkosten (oder sind unvollständig). Die Ablesungen dieser Monate sind noch nicht berechnet, bis die Fixkosten erfasst sind."
	for name, eingabe := range map[string]bool{"keine Eingabe": false, "unvollständig": true} {
		db := openTestDB(t)
		if eingabe {
			fk := fixkostenMitPreisen("2026-06-01")
			fk.Strompreis = nil
			if _, err := store.CreateFixkostenEingabe(db, fk); err != nil {
				t.Fatal(err)
			}
		}
		csvText := strings.Join(csvHeader, ";") + "\n" + csvRow("2026-06-01", "100") + "\n"
		w := httptest.NewRecorder()
		handleImportCSV()(w, requestWithDB(csvUploadRequest(t, csvText), db))
		loc, err := url.Parse(w.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}
		warnings := loc.Query()["warning"]
		var found bool
		for _, h := range warnings {
			if h == text {
				found = true
			}
			if strings.Contains(h, "href") || strings.Contains(h, "Fixkosten erfassen") || strings.Contains(h, "Fixkosten ergänzen") {
				t.Errorf("CSV, %s: Hinweis %q enthält einen Link", name, h)
			}
		}
		if !found {
			t.Errorf("CSV, %s: Text %q fehlt in %q", name, text, warnings)
		}
	}
}

func TestFixkostenHinweis_NachLoeschenKeineUndUnvollstaendigeEingabe(t *testing.T) {
	db := openTestDB(t)
	id := hinweisSeed(t, db)
	fixkostenPost(t, db, handleDeleteFixkosten(), "/fixkosten/x/loeschen", id, nil)
	z := newZugang("", nil)
	text := "Für Juli 2026 fehlen die Fixkosten (oder sind unvollständig). Die Ablesungen vom 10.07.2026 und 20.07.2026 sind unvollständig, bis sie erfasst ist."
	pruefeHinweisSeite(t, "Löschen, keine Eingabe", fixkostenListeHTML(t, db, z, "?geloescht=2026-07-01"), text, "/fixkosten/neu", "Fixkosten erfassen")

	unvollstaendig := hinweisFixkosten("2026-07-01")
	unvollstaendig.Strompreis = nil
	eid, err := store.CreateFixkostenEingabe(db, unvollstaendig)
	if err != nil {
		t.Fatal(err)
	}
	pruefeHinweisSeite(t, "Löschen, unvollständig", fixkostenListeHTML(t, db, z, "?geloescht=2026-07-01"), text, fmt.Sprintf("/fixkosten/%d/bearbeiten", eid), "Fixkosten ergänzen")
}
