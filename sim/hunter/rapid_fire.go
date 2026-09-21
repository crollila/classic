package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (hunter *Hunter) registerRapidFire() {
	if hunter.Level < 26 {
		return
	}

	actionID := core.ActionID{SpellID: 3045}
	cooldown := time.Minute * 5
	haste, duration := 1.4, time.Second*15
	if hunter.Forever != nil {
		// Forever: effect 0 is the ranged haste percent; duration and cooldown are the client's.
		haste = 1 + hunter.ClientEffectValue(actionID.SpellID, 0, 40)/100
		duration = clientAuraDuration(&hunter.Character, actionID.SpellID, duration)
		cooldown = clientCooldown(&hunter.Character, actionID.SpellID, cooldown)
	}

	hunter.RapidFireAura = hunter.RegisterAura(core.Aura{
		Label:    "Rapid Fire",
		ActionID: actionID,
		Duration: duration,

		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyRangedSpeed(sim, haste)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.MultiplyRangedSpeed(sim, 1/haste)
		},
	})

	hunter.RapidFire = hunter.RegisterSpell(core.SpellConfig{
		ActionID: actionID,

		ManaCost: core.ManaCostOptions{
			FlatCost: 100,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    hunter.NewTimer(),
				Duration: cooldown,
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
			hunter.RapidFireAura.Activate(sim)
		},
	})

	hunter.AddMajorCooldown(core.MajorCooldown{
		Spell: hunter.RapidFire,
		Type:  core.CooldownTypeDPS,
	})
}
