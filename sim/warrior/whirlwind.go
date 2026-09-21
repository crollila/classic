package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerWhirlwindSpell() {
	if warrior.Level < levelWhirlwind {
		return
	}
	// Effect 0 (normalized_weapon_damage) carries the flat bonus (0 in every build so far); the
	// spell's max targets bounds the hits.
	bonus := clientRankValue(&warrior.Character, 1680, 0, 0)
	maxTargets := int32(4)
	if client := warrior.ClientSpell(1680); client != nil && client.MaxTargets > 0 {
		maxTargets = int32(client.MaxTargets)
	}
	results := make([]*core.SpellResult, min(maxTargets, warrior.Env.GetNumTargets()))

	warrior.Whirlwind = warrior.RegisterSpell(BerserkerStance, core.SpellConfig{
		SpellCode:   SpellCode_WarriorWhirlwind,
		ActionID:    core.ActionID{SpellID: 1680},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			Cost: 25,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: time.Second * 10,
			},
		},
		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1.25,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for idx := range results {
				baseDamage := bonus + spell.Unit.MHNormalizedWeaponDamage(sim, spell.MeleeAttackPower(target))
				results[idx] = spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
				target = sim.Environment.NextTargetUnit(target)
			}

			for _, result := range results {
				spell.DealDamage(sim, result)
			}
		},
	})
}
