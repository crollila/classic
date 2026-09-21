package warrior

import (
	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerExecuteSpell() {

	rank, spellID := warrior.trainerRank("Execute")
	if rank == 0 {
		return
	}
	// Effect 0 (dummy): the flat damage. The damage per extra rage point is not stored on the
	// client spell (it is the tooltip's formula), so the Classic value is used and recorded.
	flatDamage := clientRankValue(&warrior.Character, spellID, 0, executeValues[rank-1][0])
	convertedRageDamage := codeValue(&warrior.Character, "warrior: Execute", "damage per extra rage", executeValues[rank-1][1], "PROVISIONAL",
		"not stored on the client spell; the Classic per-rank value is used")

	var rageMetrics *core.ResourceMetrics
	warrior.Execute = warrior.RegisterSpell(BattleStance|BerserkerStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorExecute,
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | core.SpellFlagPassiveSpell | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			// Improved Execute: client effect 0 is the rage cost modifier in tenths (-30/-50).
			Cost:   15 - clientTalent(&warrior.Character, "warrior.talent.improved-execute", 0, -0.1, warrior.ForeverValue("warrior.talent.improved-execute", 0, []float64{0, 2, 5}[warrior.Talents.ImprovedExecute])),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return sim.IsExecutePhase20()
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1.25,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			extraRage := spell.Unit.CurrentRage()
			warrior.SpendRage(sim, extraRage, rageMetrics)
			// We must count this rage event if the spell itself cost 0,
			// otherwise we could end up with 0 events even though rage was spent.
			if spell.Cost.GetCurrentCost() > 0 {
				rageMetrics.Events--
			}

			baseDamage := flatDamage + convertedRageDamage*(extraRage)

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if !result.Landed() {
				spell.IssueRefund(sim)
			}
		},
	})
	rageMetrics = warrior.Execute.Cost.SpellCostFunctions.(*core.RageCost).ResourceMetrics
}
