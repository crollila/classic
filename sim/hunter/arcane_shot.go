package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

func (hunter *Hunter) getArcaneShotConfig(rank int, timer *core.Timer) core.SpellConfig {
	spellId := [9]int32{0, 3044, 14281, 14282, 14283, 14284, 14285, 14286, 14287}[rank]
	baseDamage := [9]float64{0, 13, 21, 33, 59, 83, 115, 145, 183}[rank]
	spellCoeff := [9]float64{0, .204, .3, .429, .429, .429, .429, .429, .429}[rank]
	manaCost := [9]float64{0, 25, 35, 50, 80, 105, 135, 160, 190}[rank]
	level := [9]int{0, 6, 12, 20, 28, 36, 44, 52, 60}[rank]

	cooldown := time.Second*6 - time.Duration(.2*float64(hunter.Talents.ImprovedArcaneShot)*float64(time.Second))
	if hunter.Forever != nil {
		// Forever: effect 0's damage and spell power coefficient come from the client (1.60.1
		// rank 8: 217 Arcane, coefficient 0 where Classic has 183 and 0.429). The generic
		// client-data layer leaves ranged spells alone, so the read is here. The cooldown is the
		// client's (category) cooldown plus Improved Arcane Shot's client value (effect 0, ms).
		baseDamage = hunter.ClientEffectValue(spellId, 0, baseDamage)
		spellCoeff = clientCoefficient(&hunter.Character, spellId, 0, spellCoeff)
		cooldown = clientCooldown(&hunter.Character, spellId, 6*time.Second) +
			time.Duration(hunter.clientTalent("hunter.talent.improved-arcane-shot", 0, -300*float64(hunter.ForeverRank("hunter.talent.improved-arcane-shot"))))*time.Millisecond
	}

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterArcaneShot,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolArcane,
		DefenseType:   core.DefenseTypeRanged,
		ProcMask:      core.ProcMaskRangedSpecial,
		Flags:         core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagShot,
		CastType:      proto.CastType_CastTypeRanged,
		Rank:          rank,
		RequiredLevel: level,
		MissileSpeed:  24,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    timer,
				Duration: cooldown,
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hunter.DistanceFromTarget >= core.MinRangedAttackDistance
		},

		CritDamageBonus: hunter.mortalShots(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: spellCoeff,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeRangedHitAndCrit)

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	}
}

func (hunter *Hunter) registerArcaneShotSpell(timer *core.Timer) {
	maxRank := 8

	for i := 1; i <= maxRank; i++ {
		config := hunter.getArcaneShotConfig(i, timer)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.ArcaneShot = hunter.GetOrRegisterSpell(config)
		}
	}
}
