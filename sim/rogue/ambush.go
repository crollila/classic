package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerAmbushSpell() {
	rank, spellID := rogue.trainerRank("Ambush")
	if rank == 0 {
		return
	}
	// The tooltip bonus is applied through the 250% multiplier, so the table holds bonus/2.5.
	flatDamageBonus := ambushBonus[rank-1]

	damageMultiplier := 2.5 * []float64{1, 1.04, 1.08, 1.12, 1.16, 1.2}[rogue.Talents.Opportunity]

	rogue.Ambush = rogue.RegisterSpell(core.SpellConfig{
		SpellCode:   SpellCode_RogueAmbush,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       rogue.builderFlags(),

		EnergyCost: core.EnergyCostOptions{
			Cost:   60,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			if !rogue.HasDagger(core.MainHand) {
				return false
			}
			// "Must be stealthed and behind the target"; Cutthroat lifts only the stealth requirement.
			if rogue.PseudoStats.InFrontOfTarget {
				return false
			}
			return rogue.IsStealthed() || (rogue.ForeverCutthroat != nil && rogue.ForeverCutthroat.IsActive())
		},

		BonusCritRating:  15 * core.CritRatingPerCritChance * float64(rogue.Talents.ImprovedAmbush),
		DamageMultiplier: damageMultiplier,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			if rogue.ForeverCutthroat != nil {
				rogue.ForeverCutthroat.Deactivate(sim)
			}
			baseDamage := (flatDamageBonus + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower(target)))

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialNoBlockDodgeParry)

			if result.Landed() {
				rogue.AddComboPoints(sim, 1, target, spell.ComboPointMetrics())
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
