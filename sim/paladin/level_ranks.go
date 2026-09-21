package paladin

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

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
var holyLightRanks = []foreverHeal{
	{635, foreverHolyLight, 35, 42, 51, 2500 * time.Millisecond},
	{639, foreverHolyLight, 60, 81, 96, 2500 * time.Millisecond},
	{647, foreverHolyLight, 110, 167, 196, 2500 * time.Millisecond},
	{1026, foreverHolyLight, 190, 322, 368, 2500 * time.Millisecond},
	{1042, foreverHolyLight, 275, 506, 569, 2500 * time.Millisecond},
	{3472, foreverHolyLight, 365, 717, 799, 2500 * time.Millisecond},
	{10328, foreverHolyLight, 465, 968, 1076, 2500 * time.Millisecond},
	{10329, foreverHolyLight, 580, 1272, 1414, 2500 * time.Millisecond},
	{25292, foreverHolyLight, 660, 1590, 1770, 2500 * time.Millisecond},
}
var flashOfLightRanks = []foreverHeal{
	{19750, foreverFlashOfLight, 35, 67, 77, 1500 * time.Millisecond},
	{19939, foreverFlashOfLight, 50, 102, 117, 1500 * time.Millisecond},
	{19940, foreverFlashOfLight, 70, 153, 171, 1500 * time.Millisecond},
	{19941, foreverFlashOfLight, 90, 206, 231, 1500 * time.Millisecond},
	{19942, foreverFlashOfLight, 115, 278, 310, 1500 * time.Millisecond},
	{19943, foreverFlashOfLight, 140, 348, 389, 1500 * time.Millisecond},
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

// foreverRankIndex is the 0-based index of the highest rank of a Forever trainer family the
// paladin knows at its level, or -1 when none is learned yet. talentGranted is the rank 1 a
// talent hands out: it is known from the talent alone, the later ranks still need the level.
func (paladin *Paladin) foreverRankIndex(family string, talentGranted bool) int {
	rank, _ := core.TrainerRankAt("Paladin|"+family, paladin.Level)
	if rank == 0 && talentGranted {
		rank = 1
	}
	return rank - 1
}

// Forever beta client 1.60.1.69876 tooltips (tf/data.json spell_desc), one row per rank.
// weapon is the share of normalized main hand weapon damage, low/high the flat Holy part.
type foreverHolyStrikeRank struct {
	mana, weapon, low, high float64
}

var foreverHolyStrikeRanks = []foreverHolyStrikeRank{
	{5, .25, 2, 4}, {9, .25, 3, 5}, {12, .30, 5, 7}, {14, .30, 6, 9},
	{16, .35, 11, 14}, {17, .35, 18, 24}, {19, .40, 28, 36}, {20, .40, 32, 42},
}

// Holy Shock: damage to an enemy, healing to an ally, mana. Rank 1 is the talent's.
type foreverShockRank struct {
	mana, low, high, healLow, healHigh float64
}

var foreverHolyShockRanks = []foreverShockRank{
	{160, 129, 139, 110, 118},
	{225, 175, 189, 150, 162},
	{275, 248, 268, 221, 239},
	{325, 334, 362, 307, 333},
}

// Light's Vigil: the triggered Holy Shock damage to an enemy, party heal, and the mana cost
// (75% of which comes back when it is spent on an enemy).
var foreverLightsVigilRanks = []foreverShockRank{
	{730, 175, 189, 325, 343},
	{1000, 268, 288, 486, 514},
	{1340, 380, 410, 684, 724},
}

// Hammer of Wrath per rank in the Forever client (Classic 316-348 / 412-455 / 504-566).
// Core's spell overrides cannot rescale it: it rolls on the ranged table.
var foreverHammerOfWrathDamage = [][2]float64{{286, 314}, {382, 420}, {474, 522}}
