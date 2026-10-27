package store

import "testing"

// Every apartment of the seed has a right in the matrix, an unknown one has none.
func TestWohnungRecht(t *testing.T) {
	for id, want := range map[int64]Recht{1: RechtWohnung1, 2: RechtWohnung2} {
		if got, ok := WohnungRecht(id); !ok || got != want {
			t.Errorf("WohnungRecht(%d) = %q, %v, want %q", id, got, ok, want)
		}
	}
	if got, ok := WohnungRecht(3); ok || got != "" {
		t.Errorf("WohnungRecht(3) = %q, %v, want none", got, ok)
	}
}
