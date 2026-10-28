package web

import (
	"encoding/json"
	"strings"

	"github.com/larknafets/nebenkostenrechner/internal/store"
)

// ablesung_hinweise.go holds the notes the Ablesung input shows about the
// Fixkosten-Eingabe (#359): the Wallbox note of the wizard and the check which
// Abrechnungsmonate still lack their Fixkosten. The Ablesung itself carries
// neither prices, Personenzahl nor switches any more.

const (
	wallboxHinweisGesetzt      = "Häkchen gesetzt: die Einzelzähler bestimmen die Kosten nicht"
	wallboxHinweisNichtGesetzt = "nicht gesetzt: Wohnung 2 bestimmt ihren Anteil"
	wallboxHinweisFehlt        = "Fixkosten für %s fehlen"
)

// wallboxHinweisDaten is what the wizard's script needs to update the Wallbox
// note when the month changes: the switch of every month that has a
// Fixkosten-Eingabe (key "YYYY-MM", the format of the month input), and the
// texts, so the wording lives in one place.
type wallboxHinweisDaten struct {
	Gesetzt      string          `json:"gesetzt"`
	NichtGesetzt string          `json:"nichtGesetzt"`
	Fehlt        string          `json:"fehlt"`
	Monate       []string        `json:"monate"`
	Haekchen     map[string]bool `json:"haekchen"`
}

func neueWallboxHinweisDaten(eingaben []*store.FixkostenEingabeDetails) wallboxHinweisDaten {
	d := wallboxHinweisDaten{
		Gesetzt: wallboxHinweisGesetzt, NichtGesetzt: wallboxHinweisNichtGesetzt,
		Fehlt:    strings.Replace(wallboxHinweisFehlt, "%s", "{monat}", 1),
		Monate:   store.Monatsnamen[:],
		Haekchen: make(map[string]bool, len(eingaben)),
	}
	for _, e := range eingaben {
		if m := monatInputFromStored(e.Monat); m != "" {
			d.Haekchen[string(m)] = e.WallboxKomplettW1
		}
	}
	return d
}

// text is the note for a month ("YYYY-MM" or "YYYY-MM-01"): the state of the
// switch in that month's Fixkosten-Eingabe, or that the Fixkosten are missing.
// An empty month has no note.
func (d wallboxHinweisDaten) text(monat string) string {
	m := monatInputFromStored(monat)
	if m == "" {
		return ""
	}
	gesetzt, ok := d.Haekchen[string(m)]
	switch {
	case !ok:
		return strings.Replace(d.Fehlt, "{monat}", store.MonatLabel(m.toStored()), 1)
	case gesetzt:
		return d.Gesetzt
	}
	return d.NichtGesetzt
}

func (d wallboxHinweisDaten) json() string {
	b, err := json.Marshal(d)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// fixkostenFehlenMonate returns the given Abrechnungsmonate ("YYYY-MM-01",
// without duplicates, in the order given) that have no complete Fixkosten-
// Eingabe: none at all or one that lacks a price, the same rule that makes an
// Ablesung nicht berechenbar (calc.GrundFixkostenFehlen). Empty months are
// skipped, a Teilstand without Abrechnungsmonat has none to miss.
func fixkostenFehlenMonate(eingaben []*store.FixkostenEingabeDetails, monate []string) []string {
	komplett := make(map[string]bool, len(eingaben))
	for _, e := range eingaben {
		komplett[e.Monat] = e.PreiseVollstaendig()
	}
	var out []string
	seen := make(map[string]bool, len(monate))
	for _, m := range monate {
		if m == "" || seen[m] {
			continue
		}
		seen[m] = true
		if !komplett[m] {
			out = append(out, m)
		}
	}
	return out
}
