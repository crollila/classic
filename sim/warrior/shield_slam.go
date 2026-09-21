package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerShieldSlamSpell() {
	if !warrior.Talents.ShieldSlam {
		return
	}

	rank, spellID := warrior.trainerRank("Shield Slam")
	if rank == 0 {
		return
	}
	damageLow := shieldSlamDamage[rank-1][0]
	damageHigh := shieldSlamDamage[rank-1][1]
	foreverLow, foreverHigh := shieldSlamDamageForever[rank-1][0], shieldSlamDamageForever[rank-1][1]
	threat := shieldSlamThreat[rank-1]

	apCoef := 0.15

	warrior.ShieldSlam = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorShieldSlam,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial, // TODO really?
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   20,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Second * 6,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.PseudoStats.CanBlock
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		FlatThreatBonus:  threat * 2,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			damage := sim.Roll(damageLow, damageHigh) + warrior.BlockValue()*2 + apCoef*spell.MeleeAttackPower(target)
			if warrior.Forever != nil {
				damage = sim.Roll(foreverLow, foreverHigh) + warrior.BlockValue()
			}
			result := spell.CalcAndDealDamage(sim, target, damage, spell.OutcomeMeleeSpecialHitAndCrit)

			if warrior.Forever != nil && result.Landed() && sim.Proc(.5, "Forever Shield Slam Dispel") {
				target.ForeverDispelOne(sim, "magic-buff")
			}
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
