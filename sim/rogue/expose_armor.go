package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerExposeArmorSpell() {
	rank, spellID := rogue.trainerRank("Expose Armor")
	if rank == 0 {
		return
	}

	rogue.ExposeArmorAuras = rogue.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return core.ExposeArmorAura(target, rogue.Talents.ImprovedExposeArmor)
	})

	arpenPerCombo := exposeArmorPerCombo[rank-1]

	arpenPerCombo *= []float64{1, 1.25, 1.5}[rogue.Talents.ImprovedExposeArmor]
	if rogue.Forever != nil {
		// Forever: the spell itself carries the armor; Improved Expose Armor no longer scales it.
		arpenPerCombo = exposeArmorPerComboForever[rank-1]
	}
	// Improved Expose Armor (Forever) refunds 1/2 combo points when cast with 5.
	refund := rogue.ForeverRank("rogue.talent.improved-expose-armor")

	// share ExtraCastCondition() state with ApplyEffects()
	var arpen float64
	var eaAura *core.Aura

	rogue.ExposeArmor = rogue.RegisterSpell(core.SpellConfig{
		SpellCode:    SpellCode_RogueExposeArmor,
		ActionID:     core.ActionID{SpellID: spellID},
		SpellSchool:  core.SpellSchoolPhysical,
		DefenseType:  core.DefenseTypeMelee,
		ProcMask:     core.ProcMaskMeleeMHSpecial,
		Flags:        rogue.finisherFlags(),
		MetricSplits: 6,

		EnergyCost: core.EnergyCostOptions{
			Cost:   25 - 5*float64(rogue.ForeverRank("rogue.talent.improved-expose-armor")),
			Refund: 0,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				spell.SetMetricsSplit(spell.Unit.ComboPoints())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			if rogue.ComboPoints() == 0 {
				return false
			}

			eaAura = rogue.ExposeArmorAuras.Get(target)
			arpen = float64(rogue.ComboPoints()) * arpenPerCombo

			if curActive := eaAura.ExclusiveEffects[0].Category.GetActiveEffect(); curActive != nil {
				return arpen >= curActive.Priority
			}
			return true
		},

		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)

			result := spell.CalcOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if result.Landed() {
				eaAura.ExclusiveEffects[0].Priority = arpen
				eaAura.Activate(sim)
				spent := rogue.ComboPoints()
				rogue.SpendComboPoints(sim, spell)
				if spent == 5 && refund > 0 {
					rogue.AddComboPoints(sim, refund, target, spell.ComboPointMetrics())
				}
			} else {
				spell.IssueRefund(sim)
			}
			spell.DealOutcome(sim, result)
		},

		RelatedAuras: []core.AuraArray{rogue.ExposeArmorAuras},
	})
	rogue.Finishers = append(rogue.Finishers, rogue.ExposeArmor)
}
