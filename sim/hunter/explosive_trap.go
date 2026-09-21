package hunter

import (
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

func (hunter *Hunter) getExplosiveTrapConfig(rank int, timer *core.Timer) core.SpellConfig {
	spellId := [4]int32{0, 409532, 409534, 409535}[rank]
	dotDamage := [4]float64{0, 15, 24, 33}[rank]
	minDamage := [4]float64{0, 104, 145, 208}[rank]
	maxDamage := [4]float64{0, 135, 193, 265}[rank]
	manaCost := [4]float64{0, 275, 395, 520}[rank]
	level := [4]int{0, 34, 44, 54}[rank]

	numHits := hunter.Env.GetNumTargets()

	numTicks, tickLength := int32(10), time.Second*2
	cooldown := time.Second * 15
	if hunter.Forever != nil {
		// Forever: the trap spell (13813/14316/14317) holds the cooldown; its effect spell
		// (13812/14314/14315) the damage: effect 0 the explosion's range, effect 1 the periodic
		// damage per tick, and the duration over effect 1's period the ticks (1.60.1 rank 3:
		// 201-257 then 33 every 2 sec for 20 sec; 30 sec category cooldown).
		trapID := [4]int32{0, 13813, 14316, 14317}[rank]
		effectID := [4]int32{0, 13812, 14314, 14315}[rank]
		minDamage, maxDamage = hunter.ClientEffectRange(effectID, 0, minDamage, maxDamage)
		dotDamage = hunter.ClientEffectValue(effectID, 1, dotDamage)
		numTicks, tickLength = clientTicks(&hunter.Character, effectID, 1, numTicks, tickLength)
		cooldown = clientCooldown(&hunter.Character, trapID, cooldown)
	}

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterExplosiveTrap,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolFire,
		DefenseType:   core.DefenseTypeMagic,
		ProcMask:      core.ProcMaskSpellDamage,
		Flags:         core.SpellFlagAPL | SpellFlagTrap,
		Rank:          rank,
		RequiredLevel: level,
		MissileSpeed:  24,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    timer,
				Duration: cooldown,
			},
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true, // Hunter GCD is locked at 1.5s
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: "ExplosiveTrap" + hunter.Label + strconv.Itoa(rank),
				Tag:   "ExplosiveTrap",
			},
			NumberOfTicks: numTicks,
			TickLength:    tickLength,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, dotDamage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					// Explosive Trap DoT only does damage if the target does not have an immolation trap ticking on them
					if !aoeTarget.HasActiveAuraWithTag("ImmolationTrap") {
						dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeTick)
					}
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if hunter.DistanceFromTarget > 5 {
				return
			}

			spell.WaitTravelTime(sim, func(s *core.Simulation) {
				curTarget := target
				// Traps gain no benefit from hit bonuses except for the Trap Mastery talent, since this is a unique interaction this is my workaround
				spellHit := spell.Unit.GetStat(stats.SpellHit) + target.PseudoStats.BonusSpellHitRatingTaken
				spell.Unit.AddStatDynamic(sim, stats.SpellHit, spellHit*-1)
				for hitIndex := int32(0); hitIndex < numHits; hitIndex++ {
					baseDamage := sim.Roll(minDamage, maxDamage)
					baseDamage *= sim.Encounter.AOECapMultiplier()
					spell.CalcAndDealDamage(sim, curTarget, baseDamage, spell.OutcomeMagicHitAndCrit)
					curTarget = sim.Environment.NextTargetUnit(curTarget)
				}
				spell.Unit.AddStatDynamic(sim, stats.SpellHit, spellHit)
				spell.AOEDot().ApplyOrReset(sim)
			})
		},
	}
}

func (hunter *Hunter) registerExplosiveTrapSpell(timer *core.Timer) {
	maxRank := 3
	for i := 1; i <= maxRank; i++ {
		config := hunter.getExplosiveTrapConfig(i, timer)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.ExplosiveTrap = hunter.GetOrRegisterSpell(config)
		}
	}
}
