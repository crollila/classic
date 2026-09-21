package rogue

import (
	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerStealthAura() {
	rogue.StealthAura = rogue.RegisterAura(core.Aura{
		Label:    "Stealth",
		ActionID: core.ActionID{SpellID: max(1784, rogue.trainerSpellID("Stealth"))},
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if rogue.Forever != nil {
				id := aura.ActionID
				rogue.AddMoveSpeedModifier(&id, .7+.03*float64(rogue.ForeverRank("rogue.talent.camouflage")))
			}
			// Stealth triggered auras
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			if rogue.Forever != nil {
				id := aura.ActionID
				rogue.RemoveMoveSpeedModifier(&id)
			}
		},
		// Stealth breaks on damage taken (if not absorbed)
		// This may be desirable later, but not applicable currently
	})
}
