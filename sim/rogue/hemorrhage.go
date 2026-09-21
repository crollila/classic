package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerHemorrhageSpell() {
	if !rogue.Talents.Hemorrhage {
		return
	}

	// Classic ranks: 16511 (talent, level 30), 17347 (46), 17348 (58).
	spellID := int32(16511)
	if rogue.Level >= 58 {
		spellID = 17348
	} else if rogue.Level >= 46 {
		spellID = 17347
	}

	actionID := core.ActionID{SpellID: spellID}

	var hemoAuras core.AuraArray
	hemoAuras = rogue.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		if rogue.Forever != nil {
			return target.GetOrRegisterAura(core.Aura{Label: "Forever Hemorrhage-" + rogue.Label, ActionID: actionID, Duration: 15 * time.Second})
		}
		return core.HemorrhageAura(target)
	})

	if rogue.Forever != nil {
		rogue.ForeverHemorrhage = hemoAuras
	}
	rogue.Hemorrhage = rogue.RegisterSpell(core.SpellConfig{
		SpellCode:   SpellCode_RogueHemorrhage,
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       rogue.builderFlags(),

		EnergyCost: core.EnergyCostOptions{
			Cost:   35.0,
			Refund: 0.8,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			IgnoreHaste: true,
		},

		CritDamageBonus: rogue.lethality(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			rogue.BreakStealth(sim)
			baseDamage := spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower(target))
			if rogue.Forever != nil && rogue.HasDagger(core.MainHand) {
				baseDamage *= 1.45
			}

			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)

			if result.Landed() {
				rogue.AddComboPoints(sim, 1, target, spell.ComboPointMetrics())
				if len(hemoAuras) > 0 {
					hemoAura := hemoAuras.Get(target)
					hemoAura.Activate(sim)
					if rogue.Forever == nil {
						hemoAura.SetStacks(sim, 30)
					}
				}
			} else {
				spell.IssueRefund(sim)
			}
		},
	})
}
