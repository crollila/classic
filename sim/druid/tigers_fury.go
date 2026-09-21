package druid

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

// Classic Tiger's Fury ranks (+10/20/30/40 damage). The Forever client has a single
// rank, 5217 at level 24, reworked to +15% physical damage on a 30 sec cooldown.
var tigersFuryRanks = []catRank{{5217, 24, 10}, {6793, 36, 20}, {9845, 48, 30}, {9846, 60, 40}}

func (druid *Druid) registerTigersFurySpell() {
	if druid.Forever != nil {
		druid.registerForeverTigersFurySpell()
		return
	}
	rank, ok := catRankAt(tigersFuryRanks, druid.Level)
	if !ok {
		return
	}
	actionID := core.ActionID{SpellID: rank.id}
	dmgBonus := rank.value

	druid.TigersFuryAura = druid.RegisterAura(core.Aura{
		Label:    "Tiger's Fury Aura",
		ActionID: actionID,
		Duration: 6 * time.Second,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.BonusPhysicalDamage += dmgBonus
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.BonusPhysicalDamage -= dmgBonus
		},
	})

	spell := druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,

		EnergyCost: core.EnergyCostOptions{
			Cost: 30,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			druid.TigersFuryAura.Activate(sim)
		},
	})

	druid.TigersFury = spell
}

// Forever client Tiger's Fury (5217, level 24, Cat Form): "Increases Physical damage done
// by 15% for 6 sec", instant, 30 sec cooldown, no cost listed.
func (druid *Druid) registerForeverTigersFurySpell() {
	if druid.Level < tigersFuryRanks[0].level {
		return
	}
	actionID := core.ActionID{SpellID: tigersFuryRanks[0].id}
	const multiplier = 1.15

	druid.TigersFuryAura = druid.RegisterAura(core.Aura{
		Label:    "Tiger's Fury Aura",
		ActionID: actionID,
		Duration: 6 * time.Second,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= multiplier
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= multiplier
		},
	})

	spell := druid.RegisterSpell(Cat, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagAPL,
		Cast: core.CastConfig{
			// Registered with the Classic 1 sec so the spell override (1000 -> 30000 ms)
			// matches; the 30 sec is then set outright so it does not depend on it.
			CD: core.Cooldown{
				Timer:    druid.NewTimer(),
				Duration: time.Second,
			},
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			druid.TigersFuryAura.Activate(sim)
		},
	})
	spell.CD.Duration = 30 * time.Second

	druid.TigersFury = spell
}
