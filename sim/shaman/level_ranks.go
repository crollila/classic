package shaman

import (
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
)

// rankAtLevel is the highest rank whose learned level is at most level, from a per-rank
// learned level table indexed by rank (index 0 unused). 0 when no rank is learned yet.
func rankAtLevel[T int | int32](levels []T, level int32) int {
	rank := 0
	for r := 1; r < len(levels); r++ {
		if int32(levels[r]) <= level {
			rank = r
		}
	}
	return rank
}

// Classic buff values of every Strength of Earth / Grace of Air rank. The core party buff
// auras only model the top rank; a shaman below that rank's level gets a class-local aura
// scaled from it, so Forever buff overrides carry over as a ratio.
var StrengthOfEarthTotemStrength = [StrengthOfEarthTotemRanks + 1]float64{0, 10, 20, 36, 61, 77}
var GraceOfAirTotemAgility = [GraceOfAirTotemRanks + 1]float64{0, 43, 67, 77}
var StoneskinTotemReduction = [StoneskinTotemRanks + 1]float64{0, 4, 7, 11, 16, 22, 30}

// lowRankStatTotemAura is the self buff of a stat totem rank below the one core models.
func (shaman *Shaman) lowRankStatTotemAura(label string, spellID int32, buff core.BuffName, ratio, multiplier float64) *core.Aura {
	updateStats := core.ForeverBuffStats(&shaman.Unit, buff).Multiply(ratio * multiplier)
	for i := range updateStats {
		updateStats[i] = math.Floor(updateStats[i] + 1e-6) // 77 * 10/77 is 10, not 9.999
	}
	return shaman.RegisterAura(core.Aura{
		Label:      label,
		ActionID:   core.ActionID{SpellID: spellID},
		Duration:   time.Minute * 2,
		BuildPhase: core.CharacterBuildPhaseBuffs,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				aura.Unit.AddStats(updateStats)
			} else {
				aura.Unit.AddStatsDynamic(sim, updateStats)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				aura.Unit.AddStats(updateStats.Multiply(-1))
			} else {
				aura.Unit.AddStatsDynamic(sim, updateStats.Multiply(-1))
			}
		},
	})
}

// LowLevelCoefficientPenalty is Classic's spell power coefficient reduction for spells
// learned below level 20: 3.75% per level short of 20.
func LowLevelCoefficientPenalty(learnedLevel int32) float64 {
	if learnedLevel <= 0 || learnedLevel >= 20 {
		return 1
	}
	return 1 - float64(20-learnedLevel)*0.0375
}

type foreverHeal = struct {
	id              int32
	code            int32
	mana, low, high float64
	cast            time.Duration
}

// Classic heal ranks: spell id, mana, heal range, cast time. Learned levels come from the
// Forever trainer data (core.SpellLearnedLevel).
var healingWaveRanks = []foreverHeal{
	{331, SpellCode_ShamanHealingWave, 25, 36, 47, 1500 * time.Millisecond},
	{332, SpellCode_ShamanHealingWave, 45, 69, 83, 2 * time.Second},
	{547, SpellCode_ShamanHealingWave, 80, 136, 163, 2500 * time.Millisecond},
	{913, SpellCode_ShamanHealingWave, 155, 279, 328, 3 * time.Second},
	{939, SpellCode_ShamanHealingWave, 200, 389, 454, 3 * time.Second},
	{959, SpellCode_ShamanHealingWave, 265, 552, 639, 3 * time.Second},
	{8005, SpellCode_ShamanHealingWave, 340, 759, 874, 3 * time.Second},
	{10395, SpellCode_ShamanHealingWave, 440, 1040, 1191, 3 * time.Second},
	{10396, SpellCode_ShamanHealingWave, 560, 1389, 1583, 3 * time.Second},
	{25357, SpellCode_ShamanHealingWave, 620, 1620, 1850, 3 * time.Second},
}
var lesserHealingWaveRanks = []foreverHeal{
	{8004, SpellCode_ShamanLesserHealingWave, 105, 170, 195, 1500 * time.Millisecond},
	{8008, SpellCode_ShamanLesserHealingWave, 145, 257, 292, 1500 * time.Millisecond},
	{8010, SpellCode_ShamanLesserHealingWave, 185, 347, 391, 1500 * time.Millisecond},
	{10466, SpellCode_ShamanLesserHealingWave, 235, 473, 529, 1500 * time.Millisecond},
	{10467, SpellCode_ShamanLesserHealingWave, 305, 649, 723, 1500 * time.Millisecond},
	{10468, SpellCode_ShamanLesserHealingWave, 380, 832, 928, 1500 * time.Millisecond},
}
var chainHealRanks = []foreverHeal{
	{1064, SpellCode_ShamanChainHeal, 260, 332, 381, 2500 * time.Millisecond},
	{10622, SpellCode_ShamanChainHeal, 315, 419, 479, 2500 * time.Millisecond},
	{10623, SpellCode_ShamanChainHeal, 405, 567, 646, 2500 * time.Millisecond},
}

// foreverHealRank is the highest learned rank, or a zero value when none is learned yet.
func foreverHealRank(ranks []foreverHeal, level int32) foreverHeal {
	var best foreverHeal
	for _, r := range ranks {
		if core.SpellLearnedLevel(r.id) <= level {
			best = r
		}
	}
	return best
}

// Lava Burst per rank in the Forever client (tf/data.json spell_desc): mana, damage range.
var foreverLavaBurstRanks = [...]struct{ mana, low, high float64 }{{165, 106, 134}, {230, 164, 210}, {265, 192, 248}}
