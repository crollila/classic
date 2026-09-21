package warrior

import (
	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerSunderArmorSpell() {
	rank, spellID := warrior.trainerRank("Sunder Armor")
	if rank == 0 {
		return
	}
	if rank == trainerRankCount("Sunder Armor") {
		warrior.SunderArmorAuras = warrior.NewEnemyAuraArray(core.SunderArmorAura)
	} else {
		warrior.SunderArmorAuras = warrior.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			// Effect 0 (mod_resistance): the armor removed per stack, stored negative.
			return sunderArmorAuraAtRank(target, rank, spellID, -clientRankValue(&warrior.Character, spellID, 0, -sunderArmorPerStack[rank-1]))
		})
	}

	spell_level := core.SpellLearnedLevel(spellID)

	var canApplySunder bool

	warrior.SunderArmor = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost:   15 - float64(warrior.Talents.ImprovedSunderArmor),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			sa := warrior.SunderArmorAuras.Get(target)
			if sa.IsActive() {
				canApplySunder = true
			} else if sa.ExclusiveEffects[0].Category.AnyActive() {
				canApplySunder = false
			} else {
				canApplySunder = true
			}
			return canApplySunder
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  2.25 * 2 * float64(spell_level),

		RelatedAuras: []core.AuraArray{warrior.SunderArmorAuras},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMeleeWeaponSpecialNoCrit) // Cannot be blocked
			if !result.Landed() {
				spell.IssueRefund(sim)
				return
			}

			if canApplySunder {
				sa := warrior.SunderArmorAuras.Get(target)
				sa.Activate(sim)
				sa.AddStack(sim)
			}
		},
	})
}
