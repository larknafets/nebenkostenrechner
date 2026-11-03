package web

import (
	"reflect"
	"testing"
)

func TestNeueMonatsauswahl(t *testing.T) {
	data := []string{"2025-11-01", "2026-02-01", "2026-01-01", "2026-03-01", "2025-12-01"}
	tests := []struct {
		name    string
		monate  []string
		w       wunsch
		want    monatsauswahl
		grenze  string
		zurueck string
		vor     string
	}{
		{
			name:   "kein gültiger Monat gibt das laufende Jahr und leere Listen",
			monate: []string{"", "quatsch"}, w: wunsch{jahr: 2020, bis: "2026-01-01"},
			want: monatsauswahl{Jahr: 2027, NeuesJahr: 2027},
		},
		{
			name:   "neuestes Jahr, Monate dedupliziert und neueste zuerst",
			monate: append(data, "2026-03-01", "x"),
			want: monatsauswahl{Jahr: 2026, NeuesJahr: 2026,
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2026-03-01", "2026-02-01", "2026-01-01"}},
			zurueck: "?jahr=2025",
		},
		{
			name:   "jahr ohne Monat mit Daten gibt das neueste Jahr",
			monate: data, w: wunsch{jahr: 2020},
			want: monatsauswahl{Jahr: 2026, NeuesJahr: 2026,
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2026-03-01", "2026-02-01", "2026-01-01"}},
			zurueck: "?jahr=2025",
		},
		{
			name:   "älteres Jahr ohne bis: Grenze ist dessen letzter Monat",
			monate: data, w: wunsch{jahr: 2025},
			want: monatsauswahl{Jahr: 2025, NeuesJahr: 2026,
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2025-12-01", "2025-11-01"}},
			grenze: "2025-12-01", vor: "?",
		},
		{
			name:   "gültiges bis",
			monate: data, w: wunsch{bis: "2026-02-01"},
			want: monatsauswahl{Jahr: 2026, NeuesJahr: 2026, Bis: "2026-02-01",
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2026-03-01", "2026-02-01", "2026-01-01"}},
			grenze: "2026-02-01", zurueck: "?jahr=2025",
		},
		{
			name:   "bis im älteren Jahr",
			monate: data, w: wunsch{jahr: 2025, bis: "2025-11-01"},
			want: monatsauswahl{Jahr: 2025, NeuesJahr: 2026, Bis: "2025-11-01",
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2025-12-01", "2025-11-01"}},
			grenze: "2025-11-01", vor: "?",
		},
		{
			name:   "ungültiges, fremdes, datenloses und neuestes bis sind kein Limit",
			monate: data, w: wunsch{bis: "quatsch"},
			want: monatsauswahl{Jahr: 2026, NeuesJahr: 2026,
				Monate:    []string{"2026-03-01", "2026-02-01", "2026-01-01", "2025-12-01", "2025-11-01"},
				BisMonate: []string{"2026-03-01", "2026-02-01", "2026-01-01"}},
			zurueck: "?jahr=2025",
		},
		{
			name:   "Dezember und Januar in verschiedenen Jahren: das neueste Jahr zählt",
			monate: []string{"2026-12-01", "2027-01-01"},
			want: monatsauswahl{Jahr: 2027, NeuesJahr: 2027,
				Monate:    []string{"2027-01-01", "2026-12-01"},
				BisMonate: []string{"2027-01-01"}},
			zurueck: "?jahr=2026",
		},
		{
			name:   "nur Sonderkosten-Monate",
			monate: []string{"2027-02-01"},
			want:   monatsauswahl{Jahr: 2027, NeuesJahr: 2027, Monate: []string{"2027-02-01"}, BisMonate: []string{"2027-02-01"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := neueMonatsauswahl(tt.monate, tt.w, 2027)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("neueMonatsauswahl = %+v, want %+v", got, tt.want)
			}
			if g := got.Grenze(); g != tt.grenze {
				t.Errorf("Grenze = %q, want %q", g, tt.grenze)
			}
			if z, v := got.JahrLinks(); z != tt.zurueck || v != tt.vor {
				t.Errorf("JahrLinks = %q, %q, want %q, %q", z, v, tt.zurueck, tt.vor)
			}
		})
	}

	// bis that equals the newest month, or is another year's or has no data.
	for _, bis := range []string{"2026-03-01", "2025-12-01", "2026-04-01"} {
		if got := neueMonatsauswahl(data, wunsch{bis: bis}, 2027); got.Bis != "" {
			t.Errorf("bis %q: Bis = %q, want kein Limit", bis, got.Bis)
		}
	}
}
