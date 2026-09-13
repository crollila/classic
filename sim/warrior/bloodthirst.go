package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerBloodthirstSpell(cdTimer *core.Timer) {
	if !warrior.Talents.Bloodthirst {
		return
	}
	var movement *core.Aura
	if warrior.ForeverRank("warrior.talent.bloodthirst") > 0 {
		movement = warrior.RegisterAura(core.Aura{Label: "Forever Bloodthirst", ActionID: core.ActionID{SpellID: 23894}, Duration: 10 * time.Second,
			OnGain:   func(a *core.Aura, sim *core.Simulation) { warrior.AddMoveSpeedModifier(&a.ActionID, 1.1) },
			OnExpire: func(a *core.Aura, sim *core.Simulation) { warrior.RemoveMoveSpeedModifier(&a.ActionID) }})
	}

	warrior.Bloodthirst = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorBloodthirst,
		ActionID:    core.ActionID{SpellID: 23894},
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
			baseDamage := 0.45 * spell.MeleeAttackPower(target)
			if movement != nil {
				baseDamage = 0.35*spell.MeleeAttackPower(target) + 30
			}
			result := spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
			if !result.Landed() {
				spell.IssueRefund(sim)
			} else if movement != nil {
				movement.Activate(sim)
			}
		},
	})
}
