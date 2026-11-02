package web

import (
	"fmt"
	"slices"

	abr "github.com/larknafets/nebenkostenrechner/internal/abrechnung"
)

// abrechnungAnlage is one numbered Anlage of the Anhang. The list on page 1
// and the headings of the Anhang both come from abrechnungAnlagen, so title
// and number cannot drift apart.
type abrechnungAnlage struct {
	// Key names the Anlage in the templates, Nr is its fixed number (1 to 7).
	Key   string
	Nr    int
	Titel string
}

// abrechnungAnlagen lists the Anlagen in print order.
var abrechnungAnlagen = []abrechnungAnlage{
	{Key: "monatsverlauf", Nr: 1, Titel: "Monatsverlauf und Saldo"},
	{Key: "heizung", Nr: 2, Titel: "Heizung und Warmwasser je Monat"},
	{Key: "bezug", Nr: 3, Titel: "Bezugsgrößen der Verteilerschlüssel"},
	{Key: "personen", Nr: 4, Titel: "Personenzahl je Monat"},
	{Key: "schluessel", Nr: 5, Titel: "Verteilerschlüssel"},
	{Key: "verbrauch", Nr: 6, Titel: "Verbrauchsübersicht"},
	{Key: "zaehlerstaende", Nr: 7, Titel: "Zählerstände"},
}

// sonderkostenAnlage comes last and only with umlegbar Sonderkosten in the
// period (#407), so the numbers 1 to 7 never move.
var sonderkostenAnlage = abrechnungAnlage{Key: "sonderkosten", Nr: 8, Titel: "Sonderkosten"}

// anlageUeberschrift is the heading of an Anlage, "Anlage N: Titel".
func anlageUeberschrift(key string) (string, error) {
	for _, a := range alleAnlagen() {
		if a.Key == key {
			return fmt.Sprintf("Anlage %d: %s", a.Nr, a.Titel), nil
		}
	}
	return "", fmt.Errorf("unbekannte Anlage %q", key)
}

// anlagenListe is the list of the Anlagen of ab for page 1.
func anlagenListe(ab *abr.Jahresabrechnung) []abrechnungAnlage {
	if len(ab.SonderkostenAnlage) == 0 {
		return abrechnungAnlagen
	}
	return alleAnlagen()
}

// alleAnlagen is abrechnungAnlagen plus the Sonderkosten Anlage, as a copy.
func alleAnlagen() []abrechnungAnlage {
	return append(slices.Clone(abrechnungAnlagen), sonderkostenAnlage)
}
