package store

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func TestValidierePersonenzahl(t *testing.T) {
	for _, c := range []struct {
		name string
		p    float64
		want string // "" = valid
	}{
		{"null", 0, ""},
		{"eins", 1, ""},
		{"zwei", 2, ""},
		{"zwei Nachkommastellen", 1.25, ""},
		{"halb", 0.5, ""},
		{"kleinster Wert", 0.01, ""},
		{"3,1", 3.1, ""},
		{"negativ", -1, "nicht negativ"},
		{"knapp negativ", -0.01, "nicht negativ"},
		{"drei Nachkommastellen", 1.255, "höchstens 2 Nachkommastellen"},
		{"1,001", 1.001, "höchstens 2 Nachkommastellen"},
		{"0,005", 0.005, "höchstens 2 Nachkommastellen"},
		{"NaN", math.NaN(), "muss eine Zahl sein"},
		{"plus unendlich", math.Inf(1), "muss eine Zahl sein"},
		{"minus unendlich", math.Inf(-1), "muss eine Zahl sein"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := ValidierePersonenzahl(c.p)
			if c.want == "" {
				if err != nil {
					t.Fatalf("ValidierePersonenzahl(%v) = %v, want nil", c.p, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("ValidierePersonenzahl(%v) = %v, want Fehler mit %q", c.p, err, c.want)
			}
		})
	}
}

func TestEingabeFehler_UnwrapGibtGrundZurueck(t *testing.T) {
	grund := ValidierePersonenzahl(-1)
	var err error = &EingabeFehler{Feld: FeldPersonen, ID: 2, Wert: "-1", Err: grund}
	var ef *EingabeFehler
	if !errors.As(err, &ef) || ef.Feld != FeldPersonen || ef.ID != 2 {
		t.Fatalf("errors.As = %+v", ef)
	}
	if !errors.Is(err, grund) {
		t.Errorf("errors.Is(err, grund) = false")
	}
}
