package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

func (hunter *Hunter) getMultiShotConfig(rank int, timer *core.Timer) core.SpellConfig {
	spellId := [6]int32{0, 2643, 14288, 14289, 14290, 25294}[rank]
	baseDamage := [6]float64{0, 0, 40, 80, 120, 150}[rank]
	manaCost := [6]float64{0, 100, 140, 175, 210, 230}[rank]
	level := [6]int{0, 18, 30, 42, 54, 60}[rank]

	numHits := min(3, hunter.Env.GetNumTargets())
	results := make([]*core.SpellResult, numHits)

	// Forever: one rank (2643) in the client (1.60.1: no flat bonus, 13.9% of base mana, 0.5 sec
	// cast, 6 sec category cooldown shared with Aimed Shot), all read from it.
	cooldown := time.Second * 10
	manaCostOptions := core.ManaCostOptions{FlatCost: manaCost}
	if hunter.Forever != nil {
		baseDamage = hunter.ClientEffectValue(spellId, 0, baseDamage)
		cooldown = clientCooldown(&hunter.Character, spellId, 6*time.Second)
		pct := .139
		if hunter.GameData != nil {
			if cost := hunter.GameData.Spell(spellId).Cost("mana"); cost != nil && cost.CostPct > 0 {
				pct = cost.CostPct / 100
			} else {
				pct = clientMissing(spellId, "mana cost percent", pct)
			}
		}
		manaCostOptions = core.ManaCostOptions{BaseCost: pct}
	}

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterMultiShot,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolPhysical,
		DefenseType:   core.DefenseTypeRanged,
		ProcMask:      core.ProcMaskRangedSpecial,
		Flags:         core.SpellFlagMeleeMetrics | core.SpellFlagAPL | SpellFlagShot,
		CastType:      proto.CastType_CastTypeRanged,
		Rank:          rank,
		RequiredLevel: level,
		MissileSpeed:  24,

		ManaCost: manaCostOptions,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD:      core.GCDDefault,
				CastTime: time.Millisecond * 500,
			},
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				cast.CastTime = spell.CastTime()
				hunter.Unit.AutoAttacks.CancelAutoSwing(sim)
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
			CD: core.Cooldown{
				Timer:    timer,
				Duration: cooldown,
			},
			CastTime: func(spell *core.Spell) time.Duration {
				return time.Duration(float64(spell.DefaultCast.CastTime) / hunter.RangedSwingSpeed())
			},
		},
		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return hunter.DistanceFromTarget >= core.MinRangedAttackDistance
		},

		CritDamageBonus: hunter.mortalShots(),

		DamageMultiplier: 1 + .05*float64(hunter.Talents.Barrage),
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			curTarget := target

			for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
				baseDamage := baseDamage +
					hunter.AutoAttacks.Ranged().CalculateNormalizedWeaponDamage(sim, spell.RangedAttackPower(target, false)) +
					hunter.AmmoDamageBonus

				results[hitIndex] = spell.CalcDamage(sim, curTarget, baseDamage, spell.OutcomeRangedHitAndCrit)

				curTarget = sim.Environment.NextTargetUnit(curTarget)
			}
			hunter.Unit.AutoAttacks.EnableAutoSwing(sim)
			spell.WaitTravelTime(sim, func(s *core.Simulation) {
				for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
					spell.DealDamage(sim, results[hitIndex])

					curTarget = sim.Environment.NextTargetUnit(curTarget)
				}
			})

		},
	}
}

func (hunter *Hunter) registerMultiShotSpell(timer *core.Timer) {
	maxRank := core.TernaryInt(core.IncludeAQ, 5, 4)
	if hunter.Forever != nil {
		// The ranks the client build has decide: 1.60.1 has only rank 1.
		ids := []int32{2643, 14288, 14289, 14290, 25294}
		levels := []int32{18, 30, 42, 54, 60}
		if rank := clientRank(&hunter.Character, ids, levels, hunter.Level); rank >= 0 {
			hunter.MultiShot = hunter.GetOrRegisterSpell(hunter.getMultiShotConfig(rank+1, timer))
		}
		return
	}
	for rank := 1; rank <= maxRank; rank++ {
		config := hunter.getMultiShotConfig(rank, timer)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.MultiShot = hunter.GetOrRegisterSpell(config)
		}
	}
}
