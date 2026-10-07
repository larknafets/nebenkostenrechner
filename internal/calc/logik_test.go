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
