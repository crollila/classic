package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerBloodrageCD() {
	if warrior.Level < levelBloodrage {
		return
	}
	actionID := core.ActionID{SpellID: 2687}
	rageMetrics := warrior.NewRageMetrics(actionID)

	instantRage := 10.0 + []float64{0, 2, 5}[warrior.Talents.ImprovedBloodrage]
	ragePerSec := 1.0
	if warrior.Forever != nil {
		// Bloodrage's client rage is in tenths: 2687 effect 0 (energize) the instant rage, and
		// its triggered 29131 effect 0 (periodic energize) the rage a second.
		instantRage = clientRankValue(&warrior.Character, 2687, 0, 100) / 10
		ragePerSec = clientRankValue(&warrior.Character, 29131, 0, 10) / 10
	}
	if warrior.ForeverRank("warrior.talent.improved-bloodrage") > 0 {
		// Client effect 0: rage generated percent (25/50).
		multiplier := 1 + clientTalent(&warrior.Character, "warrior.talent.improved-bloodrage", 0, 1, warrior.ForeverValue("warrior.talent.improved-bloodrage", 0, 0))/100
		instantRage *= multiplier
		ragePerSec *= multiplier
	}

	warrior.BloodrageAura = warrior.RegisterAura(core.Aura{
		Label:    "Bloodrage",
		ActionID: actionID,
		Duration: time.Second * 10,
	})

	warrior.Bloodrage = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ActionID: actionID,
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Minute,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			warrior.BloodrageAura.Activate(sim)
			warrior.AddRage(sim, instantRage, rageMetrics)

			core.StartPeriodicAction(sim, core.PeriodicActionOptions{
				NumTicks: 10,
				Period:   time.Second * 1,
				OnAction: func(sim *core.Simulation) {
					warrior.AddRage(sim, ragePerSec, rageMetrics)
				},
			})
		},
	})

	warrior.AddMajorCooldown(core.MajorCooldown{
		Spell: warrior.Bloodrage.Spell,
		Type:  core.CooldownTypeDPS,
	})
}
