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
	if a, ok := v.Ablesung(periodID); ok {
		if _, offen := v.OffenerMonatVon(a.Period.Monat); offen {
			return url + "?" + fixkostenFehlenParam + "=1", nil
		}
	}
	return url, nil
}

// ablesungFixkostenHinweis is the note of an Ablesung whose Abrechnungsmonat
// lacks its Fixkosten (detail page and banner after saving), nil when it has
// them (or no Abrechnungsmonat yet).
func ablesungFixkostenHinweis(v *calc.Verbrauchskosten, monat string) *fixkostenHinweis {
	m, offen := v.OffenerMonatVon(monat)
	if !offen {
		return nil
	}
	return fixkostenFehlenHinweis(kontextAblesung, []calc.OffenerMonat{m}, nil)
}
