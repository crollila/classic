package warlock

import (
	"github.com/wowsims/classic/sim/core/stats"
)

// Level each demon is learned at (Summon Imp/Voidwalker/Succubus/Felhunter).
const (
	summonImpLevel        = 1
	summonVoidwalkerLevel = 10
	summonSuccubusLevel   = 20
	summonFelhunterLevel  = 30
)

// petConfigAtLevel returns a demon's stats and melee weapon at the owner's level. The
// tables were measured at a few levels (anchors); an anchor level returns its table
// unchanged, other levels interpolate linearly between the nearest anchors. Below the
// first anchor the first two anchors are extrapolated, floored at the first anchor
// scaled by level, so values never go negative for a low-level warlock.
func petConfigAtLevel(cfg PetConfig, level int32, anchors []int32, at func(int32) PetConfig) PetConfig {
	withName := func(c PetConfig) PetConfig {
		c.Name, c.PowerModifier = cfg.Name, cfg.PowerModifier
		return c
	}
	for _, a := range anchors {
		if a == level {
			return withName(at(a))
		}
	}
	if len(anchors) == 1 {
		return withName(at(anchors[0]))
	}

	i := 1
	for i < len(anchors)-1 && level > anchors[i] {
		i++
	}
	loLevel, hiLevel := anchors[i-1], anchors[i]
	lo, hi := at(loLevel), at(hiLevel)
	t := float64(level-loLevel) / float64(hiLevel-loLevel)
	floor := 1.0
	if level < loLevel {
		floor = float64(level) / float64(loLevel)
	}
	lerp := func(a, b float64) float64 {
		v := a + (b-a)*t
		if level < loLevel {
			v = max(v, a*floor)
		}
		return v
	}

	result := lo
	result.Stats = stats.Stats{}
	for s := range lo.Stats {
		result.Stats[s] = lerp(lo.Stats[s], hi.Stats[s])
	}
	result.AutoAttacks.MainHand.BaseDamageMin = lerp(lo.AutoAttacks.MainHand.BaseDamageMin, hi.AutoAttacks.MainHand.BaseDamageMin)
	result.AutoAttacks.MainHand.BaseDamageMax = lerp(lo.AutoAttacks.MainHand.BaseDamageMax, hi.AutoAttacks.MainHand.BaseDamageMax)
	return withName(result)
}

// rankAtLevel is the highest 1-based rank whose learned level is at most level, 0 when none.
// learned is indexed by rank with an unused 0th entry, as the rank tables in this package are.
func rankAtLevel(learned []int, level int32) int {
	rank := 0
	for r := 1; r < len(learned); r++ {
		if learned[r] <= int(level) {
			rank = r
		}
	}
	return rank
}
