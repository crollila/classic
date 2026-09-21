package core

import "testing"

func TestPPMChanceWithWeaponSpecials(t *testing.T) {
	ppm := PPMManager{procMasks: []ProcMask{ProcMaskMeleeMH, ProcMaskMeleeOH}, procChances: []float64{.04, .03}, mhSpecialProcChance: .1, ohSpecialProcChance: .08}
	for _, c := range []struct {
		mask ProcMask
		want float64
	}{
		{ProcMaskMeleeMHAuto, .04}, {ProcMaskMeleeOHAuto, .03},
		{ProcMaskMeleeMHSpecial, .1}, {ProcMaskMeleeOHSpecial, .08}, {ProcMaskSpellDamage, 0},
	} {
		if got := ppm.ChanceWithWeaponSpecials(c.mask); got != c.want {
			t.Fatalf("mask %v: got %g want %g", c.mask, got, c.want)
		}
	}
	if got := ppm.ChanceWithWeaponSpecials(ProcMaskMeleeMHSpecial) * 2; got != .2 {
		t.Fatalf("100%% increase must be 20%%, not two 10%% rolls: %g", got)
	}
}
