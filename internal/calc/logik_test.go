package calc

import (
	"testing"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

func TestLogikLabels_CoversAllLogikKonstanten(t *testing.T) {
	for _, logik := range []string{store.LogikWohneinheit, store.LogikFlurstueck, store.LogikQM, store.LogikPersonen} {
		if LogikLabels[logik] == "" {
			t.Errorf("LogikLabels missing entry for %q", logik)
		}
	}
}

// TestLogikLabels_MatchStoreLogiken: store.Logiken (the store cannot read
// LogikLabels) holds exactly the keys of LogikLabels.
func TestLogikLabels_MatchStoreLogiken(t *testing.T) {
	for logik := range LogikLabels {
		if !store.Logiken[logik] {
			t.Errorf("store.Logiken missing %q from LogikLabels", logik)
		}
	}
	for logik := range store.Logiken {
		if LogikLabels[logik] == "" {
			t.Errorf("LogikLabels missing entry for store.Logiken %q", logik)
		}
	}
}
