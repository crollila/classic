package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (hunter *Hunter) getFreezingTrapConfig(timer *core.Timer) core.SpellConfig {

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterFreezingTrap,
		ActionID:      core.ActionID{SpellID: 409510},
		SpellSchool:   core.SpellSchoolFrost,
		DefenseType:   core.DefenseTypeMagic,
		ProcMask:      core.ProcMaskSpellDamage,
		Flags:         core.SpellFlagAPL | SpellFlagTrap,
		RequiredLevel: 20,
		MissileSpeed:  24,

		ManaCost: core.ManaCostOptions{
			FlatCost: 50,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    timer,
				Duration: time.Second * 15,
			},
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
		},
	}
}

func (hunter *Hunter) registerFreezingTrapSpell(timer *core.Timer) {
	config := hunter.getFreezingTrapConfig(timer)
	if hunter.Forever != nil {
		config.ActionID = core.ActionID{SpellID: 14311}
		config.ManaCost.FlatCost = 100
		config.RequiredLevel = 60
		config.ApplyEffects = func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcOutcome(sim, t, s.OutcomeMagicHit)
			s.DealOutcome(sim, r)
			if r.Landed() {
				t.ForeverControlAura("Freezing Trap", s.ActionID, core.ForeverIncapacitate, time.Duration(float64(20*time.Second)*(1+.15*float64(hunter.ForeverRank("hunter.talent.clever-traps"))))).Activate(sim)
			}
		}
	}

	if config.RequiredLevel <= int(hunter.Level) {
		hunter.FreezingTrap = hunter.GetOrRegisterSpell(config)
	}
}
