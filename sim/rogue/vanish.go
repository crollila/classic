package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerVanishSpell() {
	rogue.VanishAura = rogue.RegisterAura(core.Aura{
		Label:    "Vanish",
		ActionID: core.ActionID{SpellID: 457437},
		Duration: time.Second * 10,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
		},
	})

	rogue.Vanish = rogue.RegisterSpell(core.SpellConfig{
		SpellCode:   SpellCode_RogueVanish,
		ActionID:    core.ActionID{SpellID: 1856},
		SpellSchool: core.SpellSchoolPhysical,
		Flags:       core.SpellFlagAPL,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: 0,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    rogue.NewTimer(),
				Duration: time.Second * time.Duration(300-float64(45*rogue.Talents.Elusiveness)),
			},
		},
		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if rogue.Forever != nil {
				for _, enemy := range rogue.Env.Encounter.TargetUnits {
					threat := 0.
					for _, s := range rogue.Spellbook {
						threat += s.SpellMetrics[enemy.UnitIndex].TotalThreat
					}
					spell.SpellMetrics[enemy.UnitIndex].TotalThreat -= max(0, threat)
				}
			}
			// Pause auto attacks
			rogue.AutoAttacks.CancelAutoSwing(sim)
			// Apply stealth
			rogue.StealthAura.Activate(sim)
		},
	})
}
