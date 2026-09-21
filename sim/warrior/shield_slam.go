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
	// Forever: effect 1 (school_damage) is the rank's damage range, plus Block Value once.
	foreverLow, foreverHigh := warrior.ClientEffectRange(spellID, 1, shieldSlamDamageForever[rank-1][0], shieldSlamDamageForever[rank-1][1])
	threat := shieldSlamThreat[rank-1]
	// Forever: the chance to dispel a magic effect is the talent research value (the client
	// states the dispel as an effect without a chance).
	dispelChance := 0.5
	if warrior.Forever != nil {
		threat = codeValue(&warrior.Character, "warrior: Shield Slam", "bonus threat", threat, "PROVISIONAL", "threat is not client data; the Classic value is used")
		dispelChance = codeValue(&warrior.Character, "warrior: Shield Slam", "dispel chance", warrior.ForeverValue("warrior.talent.shield-slam", 2, 50)/100, "PROVISIONAL",
			"the client dispel effect (type 38) states no chance; the talent research value is used")
	}

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

			if warrior.Forever != nil && result.Landed() && sim.Proc(dispelChance, "Forever Shield Slam Dispel") {
				target.ForeverDispelOne(sim, "magic-buff")
			}
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
