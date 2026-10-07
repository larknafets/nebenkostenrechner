package abrechnung

import "github.com/larknafets/nebenkostenrechner/internal/store"

// LogikLabels renders a cost position's (Kostenposition) allocation logic
// (Logik) as the German label shown throughout the app: in the Fixkosten
// forms and as the Verteilerschlüssel of a Zeile.
var LogikLabels = map[string]string{
	store.LogikWohneinheit: "Je Wohneinheit",
	store.LogikFlurstueck:  "Je anteiliges Flurstück",
	store.LogikQM:          "Je anteilige Wohnungsgröße",
	store.LogikPersonen:    "Je Anzahl Personen",
	store.LogikWohnung1:    "Wohnung 1",
	store.LogikWohnung2:    "Wohnung 2",
}
