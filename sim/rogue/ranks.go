package rogue

import (
	"github.com/wowsims/classic/sim/core"
)

// Per-rank values of the rogue's trainer spells, lowest rank first, parallel to
// core.TrainerRanks("Rogue|<name>") (spell ids and learned levels from the Forever
// beta client). The values are the Classic ones the sim has always registered at
// level 60: the Forever override layer (core/foreverdata) is keyed by spell id and
// scales from these Classic values, so registering the right rank id with its
// Classic number is what lets a Forever change apply. Ranks whose Forever tooltip
// differs are noted.

// Sinister Strike: flat bonus damage.
var sinisterStrikeBonus = []float64{3, 6, 10, 15, 22, 33, 52, 68}

// Backstab: tooltip bonus / 1.5 (the 150% multiplier also scales the bonus).
var backstabBonus = []float64{10, 20, 32, 46, 60, 90, 110, 140, 150}

// Ambush: tooltip bonus / 2.5 (the 250% multiplier also scales the bonus).
var ambushBonus = []float64{28, 40, 50, 74, 92, 116}

// Eviscerate: {flat, per combo point, damage range}, i.e. 1 point is flat+cp to flat+cp+range.
// Rank 6 is Classic 99-143 per first point; the Forever client lists 93-137 (see
// eviscerateValuesForever).
var eviscerateValues = [][3]float64{
	{1, 5, 4},
	{3, 11, 8},
	{6, 19, 14},
	{10, 31, 20},
	{15, 45, 30},
	{22, 77, 44},
	{34, 110, 68},
	{48, 151, 96},
	{54, 170, 108},
}

// eviscerateValuesForever is the Forever client's table: rank 6 (8624) is 93-137 at 1 point
// and 377-421 at 5, i.e. 71 a combo point instead of 77; every other rank matches Classic.
var eviscerateValuesForever = func() [][3]float64 {
	v := append([][3]float64(nil), eviscerateValues...)
	v[5] = [3]float64{22, 71, 44}
	return v
}()

// Slice and Dice: melee haste.
var sliceAndDiceHaste = []float64{0.20, 0.30}

// Garrote: damage per tick (tooltip total / 6).
var garroteTick = []float64{24, 34, 47, 59, 74, 92}

// Rupture: {base, per combo point} damage per 2 sec tick. Classic values; Forever
// reduces every rank's base by ~40% (e.g. rank 1 8 -> 5 per overrides.json), which the
// override layer does not apply to melee-defense spells.
var ruptureTick = [][2]float64{{8, 2}, {12, 3}, {18, 4}, {27, 5}, {37, 7}, {60, 8}}

// ruptureTickForever is the Forever client's Rupture: the same base + per combo point tick
// over points+3 ticks, cut ~40%. Ranks 1/3/4/6 are the client's stored numbers (5+1.18,
// 11+2.37, 16+2.96, 35+4.73); ranks 2 and 5 are fitted to their tooltips (1 point 35 / 105
// ... 5 points 127 / 342 over 8-16 sec), which they reproduce to the damage point.
var ruptureTickForever = [][2]float64{{5, 1.18}, {7, 1.78}, {11, 2.37}, {16, 2.96}, {22, 4.14}, {35, 4.73}}

// Expose Armor: armor per combo point (Classic; the Forever client lists 90/180/270/360/450).
var exposeArmorPerCombo = []float64{80, 145, 210, 275, 340}

// exposeArmorPerComboForever is the Forever client's armor per combo point. Improved Expose
// Armor no longer scales it (the talent is now a cost reduction and combo point refund).
var exposeArmorPerComboForever = []float64{90, 180, 270, 360, 450}

// mutilateBonusForever is Mutilate's "additional N with each weapon" by rank (1310707,
// 399956, 1241582, 1241584; learned at 30/40/50/60), added after the 75% weapon damage.
var mutilateBonusForever = []float64{13, 19, 27, 38}

// trainerRank is the highest rank of a rogue trainer spell the rogue knows at its level
// (1-based) and its spell id; rank 0 when the spell is not learned yet.
func (rogue *Rogue) trainerRank(family string) (int, int32) {
	return core.TrainerRankAt("Rogue|"+family, rogue.Level)
}

// trainerSpellID is the spell id of the highest known rank, 0 when not learned yet.
func (rogue *Rogue) trainerSpellID(family string) int32 {
	_, id := rogue.trainerRank(family)
	return id
}

// levelRank picks the highest entry whose learned level is <= level; -1 when none.
func levelRank(levels []int32, level int32) int {
	rank := -1
	for i, l := range levels {
		if l <= level {
			rank = i
		}
	}
	return rank
}

// Poisons are weapon-imbue consumables: the enchant spell ids below are the ones the
// sim has always used, each rank's poison is usable from the level the rogue learns
// to make it (the Forever client's "Rogue|<Poison>" trainer levels).

// Instant Poison I-VI: {enchant spell id}, {min damage, range}. Classic values; the
// Forever client lists 13-17, 20-26, 29-37, 45-57, 62-80 and 76-100.
var instantPoisonIDs = []int32{8679, 8686, 8688, 11338, 11339, 11340}
var instantPoisonDamage = [][2]float64{{19, 6}, {30, 8}, {44, 12}, {67, 18}, {92, 26}, {112, 36}}
var instantPoisonDamageForever = [][2]float64{{13, 4}, {20, 6}, {29, 8}, {45, 12}, {62, 18}, {76, 24}}

// Deadly Poison I-V: damage per 3 sec tick of one stack. Classic values; the Forever
// client lists 24/36/56/72/92 over 12 sec.
var deadlyPoisonIDs = []int32{2823, 2824, 11355, 11356, 25347}
var deadlyPoisonTickDamage = []float64{9, 13, 20, 27, 34}
var deadlyPoisonTickDamageForever = []float64{6, 9, 14, 18, 23} // tooltip total / 4 ticks

func (rogue *Rogue) poisonRank(family string) int {
	ranks := core.TrainerRanks("Rogue|" + family)
	levels := make([]int32, len(ranks))
	for i, r := range ranks {
		levels[i] = r[1]
	}
	return levelRank(levels, rogue.Level)
}
