package web

import (
	"sort"
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// The Ablesung detail view and the wizard take their meters from the
// store's descriptor: a meter added there shows up in both.
func TestZaehlerliste_DetailUndWizardKennenAlleZaehler(t *testing.T) {
	var detail []string
	for _, m := range meterDisplays {
		detail = append(detail, m.Key)
	}
	var alle []string
	for _, m := range store.MeterDefs() {
		alle = append(alle, m.Key)
	}
	sort.Strings(detail)
	sort.Strings(alle)
	if len(detail) != len(alle) {
		t.Fatalf("Detailansicht zeigt %v, want alle Zähler %v", detail, alle)
	}
	for i := range alle {
		if detail[i] != alle[i] {
			t.Fatalf("Detailansicht zeigt %v, want alle Zähler %v", detail, alle)
		}
	}

	if len(einzelzaehlerFelder) != len(store.OptionalMeterKeys) {
		t.Fatalf("Wizard hat %d Einzelzähler-Felder, want %d", len(einzelzaehlerFelder), len(store.OptionalMeterKeys))
	}
	for i, f := range einzelzaehlerFelder {
		if f.Key != store.OptionalMeterKeys[i] {
			t.Errorf("Wizard-Feld %d = %s, want %s", i, f.Key, store.OptionalMeterKeys[i])
		}
	}
}
