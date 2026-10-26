package store

import "testing"

// TestWohnungsregeln_AnzeigeUndErfassung covers the display and capture
// questions: status x Wallbox flag x Wallbox amount.
func TestWohnungsregeln_AnzeigeUndErfassung(t *testing.T) {
	for _, status := range []string{StatusEigennutzung, StatusVermietet} {
		for _, wallbox := range []bool{false, true} {
			for _, betrag := range []float64{0, 12.5} {
				vermietet := status == StatusVermietet
				// Written out from the table, not from the rule.
				wantFormular := vermietet
				wantMieteinnahmen := vermietet
				wantDetailMiete := vermietet
				wantWallboxZeile := !(vermietet && !wallbox && betrag == 0)

				s := NewWohnungsregeln(
					Apartment{ID: 1, Status: status},
					Haus{WallboxWeiterberechnen: wallbox},
				)
				if got := s.FixkostenAbschlagMiete(); got != wantFormular {
					t.Errorf("%s wallbox=%v: FixkostenAbschlagMiete=%v, want %v", status, wallbox, got, wantFormular)
				}
				if got := s.Mieteinnahmen(); got != wantMieteinnahmen {
					t.Errorf("%s wallbox=%v: Mieteinnahmen=%v, want %v", status, wallbox, got, wantMieteinnahmen)
				}
				if got := s.FixkostenDetailMiete(); got != wantDetailMiete {
					t.Errorf("%s wallbox=%v: FixkostenDetailMiete=%v, want %v", status, wallbox, got, wantDetailMiete)
				}
				if got := s.JahreskarteWallboxZeile(betrag); got != wantWallboxZeile {
					t.Errorf("%s wallbox=%v betrag=%v: JahreskarteWallboxZeile=%v, want %v", status, wallbox, betrag, got, wantWallboxZeile)
				}
			}
		}
	}
}

// TestWohnungsregelnOhneHaus pins the questions that need no Haus flags: they
// answer from the Wohnungsstatus alone, and the flag-dependent ones refuse to
// answer instead of silently assuming "flag off".
func TestWohnungsregelnOhneHaus(t *testing.T) {
	for _, status := range []string{StatusEigennutzung, StatusVermietet} {
		for _, id := range []int64{1, 2} {
			vermietet := status == StatusVermietet
			s := NewWohnungsregelnOhneHaus(Apartment{ID: id, Status: status})
			if got := s.FixkostenAbschlagMiete(); got != vermietet {
				t.Errorf("%s W%d: FixkostenAbschlagMiete=%v, want %v", status, id, got, vermietet)
			}
			if got := s.FixkostenDetailMiete(); got != vermietet {
				t.Errorf("%s W%d: FixkostenDetailMiete=%v, want %v", status, id, got, vermietet)
			}
			if got := s.Mieteinnahmen(); got != vermietet {
				t.Errorf("%s W%d: Mieteinnahmen=%v, want %v", status, id, got, vermietet)
			}
			if got, want := s.AbrechnungPVGutschrift(), !vermietet && id == 1; got != want {
				t.Errorf("%s W%d: AbrechnungPVGutschrift=%v, want %v", status, id, got, want)
			}

			for name, ask := range map[string]func(){
				"AbrechnungStrom":         func() { s.AbrechnungStrom() },
				"AbrechnungWallbox":       func() { s.AbrechnungWallbox() },
				"JahreskarteWallboxZeile": func() { s.JahreskarteWallboxZeile(0) },
			} {
				func() {
					defer func() {
						if recover() == nil {
							t.Errorf("%s W%d: %s without Haus did not panic", status, id, name)
						}
					}()
					ask()
				}()
			}
		}
	}
}

// TestWohnungsregeln_Abrechnung is the Wohnungsstatus table of GLOSSARY.md at
// the seam of Wohnungsregeln: status x both flags x apartment.
func TestWohnungsregeln_Abrechnung(t *testing.T) {
	for _, status := range []string{StatusEigennutzung, StatusVermietet} {
		for _, id := range []int64{1, 2} {
			for _, strom := range []bool{false, true} {
				for _, wallbox := range []bool{false, true} {
					vermietet := status == StatusVermietet
					// Written out from the table, not from the rule.
					wantStrom := !vermietet || strom
					wantWallbox := !vermietet || wallbox
					wantPV := !vermietet && id == 1

					s := NewWohnungsregeln(
						Apartment{ID: id, Status: status},
						Haus{StromWeiterberechnen: strom, WallboxWeiterberechnen: wallbox},
					)
					if got := s.AbrechnungStrom(); got != wantStrom {
						t.Errorf("%s W%d strom=%v wallbox=%v: AbrechnungStrom=%v, want %v", status, id, strom, wallbox, got, wantStrom)
					}
					if got := s.AbrechnungWallbox(); got != wantWallbox {
						t.Errorf("%s W%d strom=%v wallbox=%v: AbrechnungWallbox=%v, want %v", status, id, strom, wallbox, got, wantWallbox)
					}
					if got := s.AbrechnungPVGutschrift(); got != wantPV {
						t.Errorf("%s W%d strom=%v wallbox=%v: AbrechnungPVGutschrift=%v, want %v", status, id, strom, wallbox, got, wantPV)
					}
				}
			}
		}
	}
}
