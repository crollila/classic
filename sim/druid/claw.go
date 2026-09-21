package druid

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

// Claw's flat bonus per rank (Classic). The Forever client shows these times 1.1
// ("110% normal damage plus 29/42/62/96/126").
var clawRanks = []catRank{{1082, 20, 27}, {3029, 28, 39}, {5201, 38, 57}, {9849, 48, 88}, {9850, 58, 115}}

func (druid *Druid) registerClawSpell() {
	rank, ok := catRankAt(clawRanks, druid.Level)
	if !ok {
		return
	}
	flatDamageBonus := rank.value

	druid.Claw = druid.RegisterSpell(Cat, core.SpellConfig{
		SpellCode:   SpellCode_DruidClaw,
		ActionID:    core.ActionID{SpellID: rank.id},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagOmen | SpellFlagBuilder,

		EnergyCost: core.EnergyCostOptions{
			Cost:   45 - 1*float64(druid.Talents.Ferocity),
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		DamageMultiplierAdditive: 1 + 0.1*float64(druid.Talents.SavageFury),
		DamageMultiplier:         1,
		ThreatMultiplier:         1,
		BonusCoefficient:         1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := flatDamageBonus + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower(target))

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)

			if result.Landed() {
				druid.AddComboPoints(sim, 1, target, spell.ComboPointMetrics())
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
