package web

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/larknafets/nebenkostenrechner/internal/calc"
)

// fixkostenFehlenParam is the query parameter that asks the Ablesung page for
// the banner "Fixkosten fehlen" after saving (the app has no flash mechanism,
// same convention as the CSV import's result on the list).
const fixkostenFehlenParam = "fixkosten_fehlen"

// ablesungGespeichertURL is where to go after saving the Ablesung periodID: its
// detail page, with fixkostenFehlenParam when its Abrechnungsmonat has no
// complete Fixkosten-Eingabe (also for a Teilstand or the first Ablesung).
func ablesungGespeichertURL(r *http.Request, db *sql.DB, periodID int64) (string, error) {
	url := fmt.Sprintf("%s/ablesungen/%d", requestBase(r), periodID)
	v, err := calc.Load(db)
	if err != nil {
		return "", err
	}
	if a, ok := v.Ablesung(periodID); ok && monatOffen(v, a.Period.Monat) {
		return url + "?" + fixkostenFehlenParam + "=1", nil
	}
	return url, nil
}

// monatOffen reports whether monat is an Abrechnungsmonat without a complete
// Fixkosten-Eingabe. A Teilstand without Abrechnungsmonat ("") has none to miss.
func monatOffen(v *calc.Verbrauchskosten, monat string) bool {
	return monat != "" && !v.MonatVollstaendig(monat)
}

// fixkostenFehlenBanner is the banner text of an Ablesung whose Abrechnungsmonat
// lacks its Fixkosten, empty when it has them (or no Abrechnungsmonat yet).
func fixkostenFehlenBanner(v *calc.Verbrauchskosten, monat string) string {
	if !monatOffen(v, monat) {
		return ""
	}
	return fixkostenFehlenSatz([]string{monat}) + " Die Ablesung ist noch nicht berechnet, bis sie erfasst sind."
}
