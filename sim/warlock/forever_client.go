package warlock

import (
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever values from the client build the unit simulates (character.GameData). Every helper
// returns the given Classic/fallback value in Classic mode, and in Forever mode records it with
// gamedata.Use when the build lacks the number, so no fallback is silent.

// foreverClientRange is effect idx of client spell id at the character's level (per-level growth
// included, truncated as the game does); Classic/missing data returns lo,hi and records the
// fallback through ClientEffectRange.
func foreverClientRange(c *core.Character, id int32, idx int, lo, hi float64) (float64, float64) {
	sp := c.ClientSpell(id)
	e := sp.Effect(idx)
	if e == nil {
		return c.ClientEffectRange(id, idx, lo, hi)
	}
	a, b := e.Range()
	lvl := c.Level
	if sp.MaxLevel > 0 && lvl > sp.MaxLevel {
		lvl = sp.MaxLevel
	}
	if lvl < sp.BaseLevel {
		lvl = sp.BaseLevel
	}
	bonus := math.Trunc(e.PerLevel * float64(lvl-sp.SpellLevel))
	if bonus < 0 {
		bonus = 0
	}
	return a + bonus, b + bonus
}

// foreverClientValue is foreverClientRange's single value (the low end: client effects with a
// single value have min == max).
func foreverClientValue(c *core.Character, id int32, idx int, classic float64) float64 {
	v, _ := foreverClientRange(c, id, idx, classic, classic)
	return v
}

// foreverRankAt is the highest of the rank ids the character's level has learned: the client's
// spell level, else the listed fallback level. Below rank 1's level it is rank 1 (a talent taken
// early still casts its first rank). The index into ids is returned too.
func foreverRankAt(c *core.Character, ids []int32, levels []int32) (int, int32) {
	best := 0
	for i, id := range ids {
		level := levels[i]
		if sp := c.ClientSpell(id); sp != nil && sp.SpellLevel > 0 {
			level = sp.SpellLevel
		}
		if level <= c.Level {
			best = i
		}
	}
	return best, ids[best]
}

// foreverManaCost is the client cost of a spell: a flat amount, or a percent of base mana.
func foreverManaCost(c *core.Character, id int32, flat float64, owner string) core.ManaCostOptions {
	if c.Forever == nil {
		return core.ManaCostOptions{FlatCost: flat}
	}
	if cost := c.ClientSpell(id).Cost("mana"); cost != nil {
		if cost.Cost > 0 {
			return core.ManaCostOptions{FlatCost: cost.Cost}
		}
		if cost.CostPct > 0 {
			return core.ManaCostOptions{BaseCost: cost.CostPct / 100}
		}
	}
	return core.ManaCostOptions{FlatCost: gamedata.Use("warlock: "+owner, "mana cost", flat, "PROVISIONAL", "client spell has no mana cost")}
}

// foreverCastTime is the client cast time of a spell (0 for instant client spells).
func foreverCastTime(c *core.Character, id int32, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil {
		return time.Duration(sp.CastMs) * time.Millisecond
	}
	return time.Duration(gamedata.Use("warlock: "+owner, "cast ms", float64(classic/time.Millisecond), "PROVISIONAL", "client build has no such spell")) * time.Millisecond
}

// foreverDuration is the client duration of a spell or aura.
func foreverDuration(c *core.Character, id int32, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil && sp.DurationMs > 0 {
		return time.Duration(sp.DurationMs) * time.Millisecond
	}
	return time.Duration(gamedata.Use("warlock: "+owner, "duration ms", float64(classic/time.Millisecond), "PROVISIONAL",
		"client build has no duration for this spell/aura; the tooltip value is kept")) * time.Millisecond
}

// foreverCoefficient is the client spell power coefficient of an effect.
func foreverCoefficient(c *core.Character, id int32, idx int, classic float64, owner string) float64 {
	if c.Forever == nil {
		return classic
	}
	if e := c.ClientSpell(id).Effect(idx); e != nil && e.SPCoefficient != nil {
		return *e.SPCoefficient
	}
	return gamedata.Use("warlock: "+owner, "sp coefficient", classic, "PROVISIONAL", "client effect has no coefficient")
}

// foreverPeriod is the client tick period of a periodic effect.
func foreverPeriod(c *core.Character, id int32, idx int, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if e := c.ClientSpell(id).Effect(idx); e != nil && e.PeriodMs > 0 {
		return time.Duration(e.PeriodMs) * time.Millisecond
	}
	return time.Duration(gamedata.Use("warlock: "+owner, "period ms", float64(classic/time.Millisecond), "PROVISIONAL", "client effect has no period")) * time.Millisecond
}

// talentValue is a Forever warlock talent's client value for client effect `effect` at the
// chosen rank, times sign (the client stores reductions as negative numbers). Without a client
// value the talent's research record value at recordIndex is used and recorded. Rank 0 is 0.
func (w *Warlock) talentValue(id string, effect int, sign float64, recordIndex int) float64 {
	record := "warlock.talent." + id
	if w.ForeverRank(record) == 0 {
		return 0
	}
	return sign * w.ClientTalentValue(record, effect, sign*w.ForeverValue(record, recordIndex, 0))
}

// talentValueOr is talentValue for numbers the research record does not carry.
func (w *Warlock) talentValueOr(id string, effect int, sign float64, fallback float64) float64 {
	record := "warlock.talent." + id
	if w.ForeverRank(record) == 0 {
		return 0
	}
	return sign * w.ClientTalentValue(record, effect, sign*fallback)
}

// foreverProvisional records a Forever number with no client source and returns it.
func (w *Warlock) foreverProvisional(owner, what string, value float64, confidence, reason string) float64 {
	if w.Forever == nil {
		return value
	}
	return gamedata.Use("warlock: "+owner, what, value, confidence, reason)
}
