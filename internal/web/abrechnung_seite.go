package web

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// personenZelle renders one occupant count for the Anhang: "-" if the source
// has no value for the apartment that month.
func personenZelle(m map[int64]int64, apartmentID int64) string {
	n, ok := m[apartmentID]
	if !ok {
		return "-"
	}
	return strconv.FormatInt(n, 10)
}

// abrechnungJahre lists the years there is data for - from the Ablesungen
// (by Abrechnungsmonat, else reading date) and the Fixkosten-Eingaben - newest
// first.
func abrechnungJahre(d abrechnungPruefDaten) []int {
	seen := map[int]bool{}
	for _, p := range d.Periods {
		if y, ok := store.Abrechnungsmonat(p.Monat).Jahr(); ok {
			seen[y] = true
		} else if t, err := time.Parse("2006-01-02", p.ReadingDate); err == nil {
			seen[t.Year()] = true
		}
	}
	for _, e := range d.Eingaben {
		if y, ok := store.Abrechnungsmonat(e.Monat).Jahr(); ok {
			seen[y] = true
		}
	}
	out := make([]int, 0, len(seen))
	for y := range seen {
		out = append(out, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out
}

// standardWohnung is the preselected apartment: the first rented one, else
// the first apartment.
func standardWohnung(apartments []store.Apartment) int64 {
	for _, a := range apartments {
		if a.Status == store.StatusVermietet {
			return a.ID
		}
	}
	return apartments[0].ID
}

// standardJahr is the preselected year: the newest one the check passes for
// the apartment, else the newest year (the page then shows its Mängelliste).
func standardJahr(d abrechnungPruefDaten, jahre []int, apartmentID int64) (int, error) {
	for _, y := range jahre {
		p, err := pruefeAbrechnungDaten(d, y, ganzesJahr, apartmentID)
		if err != nil {
			return 0, err
		}
		if p.Abrechenbar() {
			return y, nil
		}
	}
	return jahre[0], nil
}

// monatOption is one entry of the Von-/Bis-Monat selection.
type monatOption struct {
	Nr   int
	Name string
}

// abrechnungSeite is the data of the /abrechnung page.
type abrechnungSeite struct {
	navData
	Aktuell string

	// KeineDaten is true if there is no Ablesung or Fixkosten-Eingabe at
	// all - the page then shows only a note, no selection.
	KeineDaten bool
	Jahre      []int
	Jahr       int
	// Teilzeitraum is the checkbox "abweichender Zeitraum", Bereich its
	// Von-/Bis-Monat (1-12, the whole year unless the checkbox is set and
	// the range is valid).
	Teilzeitraum bool
	Bereich      monatsbereich
	Monate       []monatOption
	Apartments   []store.Apartment
	ApartmentID  int64
	Apartment    store.Apartment
	Haus         store.Haus

	Ergebnis abrechnungErgebnis
	// Von/Bis are the period's days ("YYYY-MM-DD") and Erstellt the day of
	// the call, for the head.
	Von, Bis, Erstellt string
	Eigennutzung       bool
	// GewichtungWaerme/GewichtungFlaeche are the Heizungs-Gewichtung as
	// whole percent, for the explanation of the Verteilerschlüssel.
	GewichtungWaerme, GewichtungFlaeche int
	// SaldoText is the balance with a leading minus for a Nachzahlung, so it
	// stays readable in black and white print.
	SaldoText string
	// Hinweise are notes for the person creating the Abrechnung, shown on
	// screen only (not in print).
	Hinweise []string
}

// handleAbrechnung serves GET /abrechnung?jahr=&wohnung= (Issue #167). It
// sits behind RequireLogin: the Abrechnung carries names, addresses and the
// IBAN. An invalid or missing jahr/wohnung falls back to the preselection.
func handleAbrechnung(a auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db := dbFromContext(r.Context())

		daten, err := ladeAbrechnungDaten(db)
		if err != nil {
			http.Error(w, "abrechnung: "+err.Error(), http.StatusInternalServerError)
			return
		}
		apartments, haus := daten.Pruef.Apartments, daten.Pruef.Haus

		data := abrechnungSeite{navData: a.NavData(r), Aktuell: "abrechnung", Apartments: apartments, Haus: haus}
		data.Jahre = abrechnungJahre(daten.Pruef)
		if len(data.Jahre) == 0 {
			data.KeineDaten = true
			renderAbrechnung(w, data)
			return
		}

		data.ApartmentID = standardWohnung(apartments)
		if id, err := strconv.ParseInt(r.URL.Query().Get("wohnung"), 10, 64); err == nil {
			for _, ap := range apartments {
				if ap.ID == id {
					data.ApartmentID = id
				}
			}
		}
		for _, ap := range apartments {
			if ap.ID == data.ApartmentID {
				data.Apartment = ap
			}
		}
		data.Eigennutzung = data.Apartment.Status != store.StatusVermietet

		data.Jahr, err = standardJahr(daten.Pruef, data.Jahre, data.ApartmentID)
		if err != nil {
			http.Error(w, "abrechnung: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if y, err := strconv.Atoi(r.URL.Query().Get("jahr")); err == nil {
			for _, known := range data.Jahre {
				if known == y {
					data.Jahr = y
				}
			}
		}

		data.Bereich = ganzesJahr
		for i, name := range germanMonths {
			data.Monate = append(data.Monate, monatOption{Nr: i + 1, Name: name})
		}
		if r.URL.Query().Get("teil") == "1" {
			von, _ := strconv.Atoi(r.URL.Query().Get("von"))
			bis, _ := strconv.Atoi(r.URL.Query().Get("bis"))
			if b := (monatsbereich{Von: von, Bis: bis}); b.gueltig() {
				data.Teilzeitraum, data.Bereich = true, b
			}
		}

		data.Ergebnis, err = berechneAbrechnung(db, daten, data.Jahr, data.Bereich, data.ApartmentID)
		if err != nil {
			http.Error(w, "abrechnung: "+err.Error(), http.StatusInternalServerError)
			return
		}
		data.Erstellt = time.Now().Format("2006-01-02")
		if ab := data.Ergebnis.Abrechnung; ab != nil {
			data.Von, data.Bis = ab.Zeitraum.Von.Format("2006-01-02"), ab.Zeitraum.Bis.Format("2006-01-02")
			data.SaldoText = saldoText(ab.Saldo)
			data.GewichtungWaerme = int(math.Round(ab.Gewichtung * 100))
			data.GewichtungFlaeche = 100 - data.GewichtungWaerme
			data.Hinweise = abrechnungHinweise(ab, data.Eigennutzung)
		}

		renderAbrechnung(w, data)
	}
}

func renderAbrechnung(w http.ResponseWriter, data abrechnungSeite) {
	if err := abrechnungTemplate.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// saldoText formats the balance as "Betrag EUR", with a leading "- " for a
// Nachzahlung.
func saldoText(s *AbschlagSaldo) string {
	text := formatEuroDE(s.Betrag()) + " €"
	if s.Nachzahlung() {
		return "- " + text
	}
	return text
}

// abrechnungHinweise are the screen-only notes for the person creating the
// Abrechnung - only for a rented apartment, the Eigennutzung overview has no
// legal deadlines.
func abrechnungHinweise(ab *abrechnung, eigennutzung bool) []string {
	if eigennutzung {
		return nil
	}
	var out []string
	if ab.Saldo.Nachzahlung() {
		frist := time.Date(ab.Zeitraum.Bis.Year()+1, ab.Zeitraum.Bis.Month()+1, 0, 0, 0, 0, 0, time.UTC)
		out = append(out, fmt.Sprintf("Frist: Die Abrechnung muss dem Mieter bis zum %s zugehen, sonst ist eine Nachforderung ausgeschlossen (§ 556 Abs. 3 BGB).", frist.Format("02.01.2006")))
	}
	if ab.Zeitraum.Teilzeitraum {
		out = append(out, "Mieterwechsel: Ein Monat gehört ganz zu dem Zeitraum, in den die Ablesung seines Abrechnungsmonats fällt, auch bei einem Wechsel mitten im Monat. Mieter und Anschrift kommen aus den Stammdaten: den Stand vor dem Umschreiben als PDF sichern.")
	}
	out = append(out, "Aufbewahrung: Den verschickten Stand als PDF aufbewahren, die App speichert nichts.")

	var geteilt []string
	for _, z := range ab.Fixkosten {
		if z.Geteilt && (len(geteilt) == 0 || geteilt[len(geteilt)-1] != z.Position) {
			geteilt = append(geteilt, z.Position)
		}
	}
	if len(geteilt) > 0 {
		out = append(out, fmt.Sprintf("Wechsel des Verteilerschlüssels im Jahr bei: %s. Ein anderer Umlageschlüssel muss dem Mieter in Textform vor Beginn des Abrechnungszeitraums erklärt werden (§ 556a Abs. 2 BGB).", joinDE(geteilt)))
	}
	return out
}

// joinDE joins names with ", ", the last one with " und ".
func joinDE(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	out := ""
	for i, n := range names[:len(names)-1] {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out + " und " + names[len(names)-1]
}
