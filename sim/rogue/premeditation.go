package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerPremeditation() {
	if !rogue.Talents.Premeditation {
		return
	}

	comboMetrics := rogue.NewComboPointMetrics(core.ActionID{SpellID: 14183})

	generation := 0
	if rogue.Forever != nil {
		rogue.OnComboPointsGained(func(sim *core.Simulation) { generation++ })
		rogue.OnComboPointsSpent(func(sim *core.Simulation, s *core.Spell, n int32) { generation++ })
	}
	rogue.Premeditation = rogue.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 14183},
		Flags:    core.SpellFlagAPL,

		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				Cost: 0,
				GCD:  0,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    rogue.NewTimer(),
				Duration: time.Minute * 2,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return rogue.IsStealthed()
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.AddComboPoints(sim, 2, target, comboMetrics)
			if rogue.Forever != nil {
				snapshot := generation
				core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: sim.CurrentTime + 20*time.Second, OnAction: func(sim *core.Simulation) {
					if snapshot == generation {
						rogue.AddComboPointsIgnoreTarget(sim, -min(2, rogue.ComboPoints()), comboMetrics)
					}
				}})
			}
		},
	})

	rogue.AddMajorCooldown(core.MajorCooldown{
		Spell:    rogue.Premeditation,
		Type:     core.CooldownTypeDPS,
		Priority: core.CooldownPriorityLow,
	})
}
