package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerSlamSpell() {
	rank, spellID := warrior.trainerRank("Slam")
	if rank == 0 {
		return
	}
	requiredLevel := int(core.SpellLearnedLevel(spellID))
	flatDamageBonus := slamBonus[rank-1]

	castTime := time.Millisecond*1500 - time.Millisecond*100*time.Duration(warrior.Talents.ImprovedSlam)
	gcd := core.GCDDefault
	if warrior.ForeverRank("warrior.talent.improved-slam") > 0 {
		reduction := time.Duration(warrior.ForeverValue("warrior.talent.improved-slam", 0, 0) * float64(time.Second))
		castTime = 1500*time.Millisecond - reduction
		gcd -= reduction
	}
	// Forever: every Slam rank has a 15 sec cooldown.
	var cd core.Cooldown
	if warrior.Forever != nil {
		cd = core.Cooldown{Timer: warrior.NewTimer(), Duration: 15 * time.Second}
	}
	warrior.Slam = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorSlam,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RequiredLevel: requiredLevel,

		RageCost: core.RageCostOptions{
			Cost:   15,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      gcd,
				CastTime: castTime,
			},
			CD: cd,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				if spell.CastTime() > 0 && warrior.ForeverRank("warrior.talent.improved-slam") == 0 {
					warrior.AutoAttacks.StopMeleeUntil(sim, sim.CurrentTime+cast.CastTime, true)
				}
			},
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		FlatThreatBonus:  140, // Should this be 54 or the old 140 value from before SoD?
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := flatDamageBonus + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower(target))

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
}
