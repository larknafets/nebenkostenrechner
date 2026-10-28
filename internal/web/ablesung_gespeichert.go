package web

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// fixkostenFehlenParam is the query parameter that asks the Ablesung page for
// the banner "Fixkosten fehlen" after saving (the app has no flash mechanism,
// same convention as the CSV import's result on the list).
const fixkostenFehlenParam = "fixkosten_fehlen"

// ablesungGespeichertURL is where to go after saving the Ablesung periodID: its
// detail page, with fixkostenFehlenParam when the Fixkosten of its
// Abrechnungsmonat are missing.
func ablesungGespeichertURL(r *http.Request, db *sql.DB, periodID int64) (string, error) {
	url := fmt.Sprintf("%s/ablesungen/%d", requestBase(r), periodID)
	ds, err := store.LadeDatenstand(db)
	if err != nil {
		return "", err
	}
	for _, p := range ds.Periods {
		if p.ID == periodID && len(fixkostenFehlenMonate(ds.Eingaben, []string{p.Monat})) > 0 {
			return url + "?" + fixkostenFehlenParam + "=1", nil
		}
	}
	return url, nil
}

// fixkostenFehlenBanner is the banner text of an Ablesung whose Abrechnungsmonat
// lacks its Fixkosten, empty when it has them (or no Abrechnungsmonat yet).
func fixkostenFehlenBanner(eingaben []*store.FixkostenEingabeDetails, monat string) string {
	if len(fixkostenFehlenMonate(eingaben, []string{monat})) == 0 {
		return ""
	}
	return "Für " + store.MonatLabel(monat) + " fehlen die Fixkosten (oder sind unvollständig). Die Ablesung ist noch nicht berechnet, bis sie erfasst sind."
}
