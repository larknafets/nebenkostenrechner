package web

import "fmt"

// abrechnungAnlage is one numbered Anlage of the Anhang. The list on page 1
// and the headings of the Anhang both come from abrechnungAnlagen, so title
// and number cannot drift apart.
type abrechnungAnlage struct {
	// Key names the Anlage in the templates, Nr is its fixed number (the
	// Anlagen of the Spec are numbered 1 to 7, a not yet built one leaves a
	// gap).
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

// anlageUeberschrift is the heading of an Anlage, "Anlage N: Titel".
func anlageUeberschrift(key string) (string, error) {
	for _, a := range abrechnungAnlagen {
		if a.Key == key {
			return fmt.Sprintf("Anlage %d: %s", a.Nr, a.Titel), nil
		}
	}
	return "", fmt.Errorf("unbekannte Anlage %q", key)
}

// anlagenListe is the list of the Anlagen for page 1.
func anlagenListe() []abrechnungAnlage {
	return abrechnungAnlagen
}
