package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerMortalStrikeSpell(cdTimer *core.Timer) {
	if !warrior.Talents.MortalStrike {
		return
	}

	rank, spellID := warrior.trainerRank("Mortal Strike")
	if rank == 0 {
		return
	}
	// The learned rank's bonus. The Forever talent record's value is the talent tooltip,
	// i.e. rank 1 (85), not the rank the warrior has learned (rank 4: 160).
	bonusDamage := mortalStrikeBonus[rank-1]

	var mortalAuras core.AuraArray
	if warrior.Forever != nil {
		mortalAuras = warrior.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.GetOrRegisterAura(core.Aura{Label: "Forever Mortal Strike", ActionID: core.ActionID{SpellID: spellID}, Duration: 10 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) { t.PseudoStats.HealingTakenMultiplier *= .5 }, OnExpire: func(a *core.Aura, sim *core.Simulation) { t.PseudoStats.HealingTakenMultiplier /= .5 }})
		})
	}
	warrior.MortalStrike = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorMortalStrike,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   30,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    cdTimer,
				Duration: time.Second * 6,
			},
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := bonusDamage + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower(target))

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)

			if warrior.Forever != nil && result.Landed() {
				mortalAuras.Get(target).Activate(sim)
			}
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
