package hunter

import (
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

func (hunter *Hunter) getImmolationTrapConfig(rank int, timer *core.Timer) core.SpellConfig {
	spellId := [6]int32{0, 409521, 409524, 409526, 409528, 409530}[rank]
	dotDamage := [6]float64{0, 105, 215, 340, 510, 690}[rank]
	manaCost := [6]float64{0, 50, 90, 135, 190, 245}[rank]
	level := [6]int{0, 16, 26, 36, 46, 56}[rank]

	numTicks, tickLength := int32(5), time.Millisecond*1500
	cooldown := time.Second * 15
	if hunter.Forever != nil {
		// Forever: the trap spell holds the cooldown, its effect spell's effect 0 the damage per
		// tick and duration/period the ticks (1.60.1 rank 5: 138 every 3 sec for 15 sec; 30 sec
		// category cooldown). dotDamage is the total, as for the Classic values.
		trapID := [6]int32{0, 13795, 14302, 14303, 14304, 14305}[rank]
		effectID := [6]int32{0, 13797, 14298, 14299, 14300, 14301}[rank]
		numTicks, tickLength = clientTicks(&hunter.Character, effectID, 0, numTicks, tickLength)
		dotDamage = hunter.ClientEffectValue(effectID, 0, dotDamage/float64(numTicks)) * float64(numTicks)
		cooldown = clientCooldown(&hunter.Character, trapID, cooldown)
	}

	return core.SpellConfig{
		SpellCode:     SpellCode_HunterImmolationTrap,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolFire,
		DefenseType:   core.DefenseTypeMagic,
		ProcMask:      core.ProcMaskSpellDamage,
		Flags:         core.SpellFlagAPL | core.SpellFlagPassiveSpell | SpellFlagTrap,
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
			Aura: core.Aura{
				Label: "ImmolationTrap" + hunter.Label + strconv.Itoa(rank),
				Tag:   "ImmolationTrap",
			},
			NumberOfTicks: numTicks,
			TickLength:    tickLength,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				tickDamage := dotDamage / float64(dot.NumberOfTicks)
				dot.Snapshot(target, tickDamage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if hunter.DistanceFromTarget > 5 {
				return
			}
			// Traps gain no benefit from hit bonuses except for the Trap Mastery talent, since this is a unique interaction this is my workaround
			spellHit := spell.Unit.GetStat(stats.SpellHit) + target.PseudoStats.BonusSpellHitRatingTaken
			spell.Unit.AddStatDynamic(sim, stats.SpellHit, spellHit*-1)
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			spell.Unit.AddStatDynamic(sim, stats.SpellHit, spellHit)
			spell.WaitTravelTime(sim, func(s *core.Simulation) {
				spell.DealOutcome(sim, result)
				if result.Landed() {
					spell.Dot(target).Apply(sim)
				}
			})
		},
	}
}

func (hunter *Hunter) registerImmolationTrapSpell(timer *core.Timer) {
	maxRank := 5
	for i := 1; i <= maxRank; i++ {
		config := hunter.getImmolationTrapConfig(i, timer)

		if config.RequiredLevel <= int(hunter.Level) {
			hunter.ImmolationTrap = hunter.GetOrRegisterSpell(config)
		}
	}
}
