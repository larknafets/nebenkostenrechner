package store

import "testing"

// TestSichtbarkeit_Abrechnung is the Wohnungsstatus table of GLOSSARY.md at
// the seam of Sichtbarkeit: status x both flags x apartment.
func TestSichtbarkeit_Abrechnung(t *testing.T) {
	for _, status := range []string{StatusEigennutzung, StatusVermietet} {
		for _, id := range []int64{1, 2} {
			for _, strom := range []bool{false, true} {
				for _, wallbox := range []bool{false, true} {
					vermietet := status == StatusVermietet
					// Written out from the table, not from the rule.
					wantStrom := !vermietet || strom
					wantWallbox := !vermietet || wallbox
					wantPV := !vermietet && id == 1

					s := NewSichtbarkeit(
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
