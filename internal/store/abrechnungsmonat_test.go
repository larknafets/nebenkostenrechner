package store

import "testing"

func TestMonatLabel(t *testing.T) {
	cases := []struct {
		datum string
		want  string
	}{
		{"2026-11-15", "November 2026"},
		{"2026-01-01", "Januar 2026"},
		{"2026-12-31", "Dezember 2026"},
		{"not-a-date", "not-a-date"},
	}
	for _, c := range cases {
		if got := MonatLabel(c.datum); got != c.want {
			t.Errorf("MonatLabel(%q) = %q, want %q", c.datum, got, c.want)
		}
	}
}
