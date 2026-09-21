package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerBloodthirstSpell(cdTimer *core.Timer) {
	if !warrior.Talents.Bloodthirst {
		return
	}
	rank, spellID := warrior.trainerRank("Bloodthirst")
	if rank == 0 {
		return
	}
	var movement *core.Aura
	// Forever Bloodthirst, per rank: effect 0 (school_damage) is the flat damage, effect 1 (dummy)
	// the percent of attack power added to it, effect 2 (mod_increase_speed) the movement bonus for
	// the spell's duration. Classic's single effect 0 is 45 = 45% of attack power.
	bonusDamage := clientRankValue(&warrior.Character, spellID, 0, bloodthirstBonusForever[rank-1])
	apPercent := clientRankValue(&warrior.Character, spellID, 1, 35) / 100
	if warrior.ForeverRank("warrior.talent.bloodthirst") > 0 {
		speed := 1 + clientRankValue(&warrior.Character, spellID, 2, 10)/100
		movement = warrior.RegisterAura(core.Aura{Label: "Forever Bloodthirst", ActionID: core.ActionID{SpellID: spellID}, Duration: clientDuration(&warrior.Character, spellID, "warrior: Bloodthirst", 10*time.Second),
			OnGain:   func(a *core.Aura, sim *core.Simulation) { warrior.AddMoveSpeedModifier(&a.ActionID, speed) },
			OnExpire: func(a *core.Aura, sim *core.Simulation) { warrior.RemoveMoveSpeedModifier(&a.ActionID) }})
	}

	warrior.Bloodthirst = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorBloodthirst,
		ActionID:    core.ActionID{SpellID: spellID},
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
				baseDamage = apPercent*spell.MeleeAttackPower(target) + bonusDamage
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
