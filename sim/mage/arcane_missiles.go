package mage

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

const ArcaneMissilesRanks = 8

var ArcaneMissilesSpellId = [ArcaneMissilesRanks + 1]int32{0, 5143, 5144, 5145, 8416, 8417, 10211, 10212, 25345}
var ArcaneMissilesBaseTickDamage = [ArcaneMissilesRanks + 1]float64{0, 26, 38, 57, 86, 115, 153, 196, 230}
var ArcaneMissilesSpellCoeff = [ArcaneMissilesRanks + 1]float64{0, .132, .204, .24, .24, .24, .24, .24, .24}
var ArcaneMissilesCastTime = [ArcaneMissilesRanks + 1]int32{0, 3, 4, 5, 5, 5, 5, 5, 5}
var ArcaneMissilesManaCost = [ArcaneMissilesRanks + 1]float64{0, 85, 140, 235, 320, 410, 500, 595, 655}
var ArcaneMissilesLevel = [ArcaneMissilesRanks + 1]int{0, 8, 16, 24, 32, 40, 48, 56, 56}

func (mage *Mage) registerArcaneMissilesSpell() {
	mage.ArcaneMissiles = make([]*core.Spell, ArcaneMissilesRanks+1)
	mage.ArcaneMissilesTickSpell = make([]*core.Spell, ArcaneMissilesRanks+1)

	// TODO AQ <=
	// Forever's trainer teaches rank 8 at 56 (Classic keeps it as the AQ book).
	maxRank := ArcaneMissilesRanks - 1
	if mage.Forever != nil {
		maxRank = ArcaneMissilesRanks
	}
	for rank := 1; rank <= maxRank; rank++ {
		config := mage.getArcaneMissilesSpellConfig(rank)

		if config.RequiredLevel <= int(mage.Level) {
			mage.ArcaneMissiles[rank] = mage.GetOrRegisterSpell(config)
		}
	}
}

// arcaneMissilesTick is one missile's base damage and coefficient; Forever uses the
// client's per-rank values.
func (mage *Mage) arcaneMissilesTick(rank int) (float64, float64) {
	if mage.Forever != nil {
		// The channel's periodic effect (0) triggers the missile spell, whose damage effect
		// (0) holds the per-missile value and coefficient; the table is the fallback.
		tick, coeff := foreverArcaneMissilesTick[rank], foreverArcaneMissilesCoeff
		if missile := mage.ClientSpell(ArcaneMissilesSpellId[rank]).Effect(0); missile != nil && missile.TriggerSpell != 0 {
			tick, _ = foreverClientRange(mage.GetCharacter(), missile.TriggerSpell, 0, tick, tick)
			if c := mage.ClientSpell(missile.TriggerSpell).Effect(0).Coefficient(); c > 0 {
				coeff = c
			}
		} else {
			gamedata.Use("mage: Arcane Missiles rank "+itoa(int32(rank)), "missile damage", tick, "UNKNOWN", "client build has no missile trigger for this rank; tooltip value used")
		}
		return tick, coeff
	}
	return ArcaneMissilesBaseTickDamage[rank], ArcaneMissilesSpellCoeff[rank]
}

func (mage *Mage) getArcaneMissilesSpellConfig(rank int) core.SpellConfig {
	spellId := ArcaneMissilesSpellId[rank]
	baseTickDamage, _ := mage.arcaneMissilesTick(rank)
	castTime := ArcaneMissilesCastTime[rank]
	manaCost := ArcaneMissilesManaCost[rank]
	level := ArcaneMissilesLevel[rank]

	numTicks := castTime
	tickLength := time.Second
	// Forever: the channel's duration and missile period come from the client.
	if client := mage.ClientSpell(spellId); client != nil && client.DurationMs > 0 {
		if e := client.Effect(0); e != nil && e.PeriodMs > 0 {
			numTicks, tickLength = int32(client.DurationMs/e.PeriodMs), time.Duration(e.PeriodMs)*time.Millisecond
		}
	}

	tickSpell := mage.getArcaneMissilesTickSpell(rank)
	mage.ArcaneMissilesTickSpell[rank] = tickSpell

	return core.SpellConfig{
		SpellCode:   SpellCode_MageArcaneMissiles,
		ActionID:    core.ActionID{SpellID: spellId},
		SpellSchool: core.SpellSchoolArcane,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagAPL | core.SpellFlagChanneled | core.SpellFlagNoMetrics,

		RequiredLevel: level,
		Rank:          rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: fmt.Sprintf("ArcaneMissiles-%d-%d", +rank, numTicks),
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					// TODO: This check is necessary to ensure the final tick occurs before
					// Arcane Blast stacks are dropped. To fix this, ticks need to reliably
					// occur before aura expirations.

					//TODO: Test interaction in classic code without aura
					dot := mage.ArcaneMissiles[rank].Dot(aura.Unit)
					if dot.TickCount < dot.NumberOfTicks {
						dot.TickCount++
						dot.TickOnce(sim)
					}
				},
			},
			NumberOfTicks: numTicks,
			TickLength:    tickLength,
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				tickSpell.Cast(sim, target)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.Dot(target).Apply(sim)
		},
		ExpectedTickDamage: func(sim *core.Simulation, target *core.Unit, spell *core.Spell, _ bool) *core.SpellResult {
			return tickSpell.CalcDamage(sim, target, baseTickDamage, spell.OutcomeExpectedMagicHitAndCrit)
		},
	}
}

func (mage *Mage) getArcaneMissilesTickSpell(rank int) *core.Spell {
	spellId := ArcaneMissilesSpellId[rank]
	baseTickDamage, spellCoeff := mage.arcaneMissilesTick(rank)

	return mage.RegisterSpell(core.SpellConfig{
		SpellCode:    SpellCode_MageArcaneMissilesTick,
		ActionID:     core.ActionID{SpellID: spellId}.WithTag(1),
		SpellSchool:  core.SpellSchoolArcane,
		DefenseType:  core.DefenseTypeMagic,
		ProcMask:     core.ProcMaskSpellDamage,
		Flags:        SpellFlagMage,
		MissileSpeed: 20,

		Rank: 1,

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: spellCoeff,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcDamage(sim, target, baseTickDamage, spell.OutcomeMagicHitAndCrit)

			spell.WaitTravelTime(sim, func(sim *core.Simulation) {
				spell.DealDamage(sim, result)
			})
		},
	})
}
