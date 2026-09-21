package warrior

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerThunderClapSpell() {
	rank, spellID := warrior.trainerRank("Thunder Clap")
	if rank == 0 {
		return
	}
	// The damage stays the rank table's Classic value: Thunder Clap is a magic-defense spell, so
	// the client-data layer (core/forever_clientdata.go) scales it by the client's Forever/Classic
	// effect 0 ratio; reading the client value here too would apply a change twice.
	baseDamage := thunderClapDamage[rank-1]
	duration := thunderClapDuration[rank-1] * time.Second
	has5pcConq := warrior.HasSetBonus(ItemSetConquerorsBattleGear, 5)
	attackSpeedReduction := core.TernaryInt32(has5pcConq, 15, 10)
	stanceMask := BattleStance
	cooldown := time.Second * 4
	if warrior.Forever != nil {
		// Forever client: 20% slower attacks (the set's +5 kept), 6 sec cooldown, usable in
		// Battle or Defensive Stance.
		// Effect 1 (mod_attack_speed, stored negative) and the spell's duration from the client.
		attackSpeedReduction += int32(-clientRankValue(&warrior.Character, spellID, 1, -20)) - 10
		duration = clientDuration(&warrior.Character, spellID, "warrior: Thunder Clap", duration)
		stanceMask |= DefensiveStance
		cooldown = time.Second * 6
	}

	warrior.ThunderClapAuras = warrior.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		aura := core.ThunderClapAura(target, spellID, attackSpeedReduction)
		aura.Duration = duration
		return aura
	})

	results := make([]*core.SpellResult, min(4, warrior.Env.GetNumTargets()))

	warrior.ThunderClap = warrior.RegisterSpell(stanceMask, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			// Improved Thunder Clap: client effect 0 is the rage cost modifier in tenths.
			Cost: 20 - clientTalent(&warrior.Character, "warrior.talent.improved-thunder-clap", 0, -0.1, warrior.ForeverValue("warrior.talent.improved-thunder-clap", 0, []float64{0, 1, 2, 4}[warrior.Talents.ImprovedThunderClap])),
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    warrior.NewTimer(),
				Duration: cooldown,
			},
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: core.TernaryFloat64(has5pcConq, 1.5, 1),
		ThreatMultiplier: 2.5,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for idx := range results {
				results[idx] = spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
				target = sim.Environment.NextTargetUnit(target)
			}

			for _, result := range results {
				spell.DealDamage(sim, result)
				if result.Landed() {
					warrior.ThunderClapAuras.Get(result.Target).Activate(sim)
				}
			}
		},

		RelatedAuras: []core.AuraArray{warrior.ThunderClapAuras},
	})
}
