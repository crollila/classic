package priest

import (
	"math"
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever client-data readers for the priest. Forever-only spells, Forever rank spells, talent
// values and proc numbers are read from the client build the character simulates
// (character.GameData, keyed by client spell ids and talent records); the typed numbers in this
// package are the Classic/fallback source only. In Forever mode every number the client does
// not state is recorded with gamedata.Use, so no fallback is silent. In Classic mode every helper
// returns the given value unchanged.

func itoa(id int32) string { return strconv.Itoa(int(id)) }

// foreverClientRange is effect idx of client spell id at the character's level: the client's
// range plus its per-level growth, trunc(perLevel * (level clamped to the spell's base..max
// level - spell level)), which is what the game applies and what the tooltip tables typed into
// this package show. Without the spell it returns lo..hi and records it.
func foreverClientRange(c *core.Character, id int32, idx int, lo, hi float64) (float64, float64) {
	sp := c.ClientSpell(id)
	e := sp.Effect(idx)
	if e == nil {
		return c.ClientEffectRange(id, idx, lo, hi)
	}
	a, b := e.Range()
	bonus := foreverLevelBonus(c.Level, sp, e)
	return a + bonus, b + bonus
}

func foreverLevelBonus(level int32, sp *gamedata.Spell, e *gamedata.Effect) float64 {
	if e.PerLevel == 0 {
		return 0
	}
	if sp.MaxLevel > 0 && level > sp.MaxLevel {
		level = sp.MaxLevel
	}
	if level < sp.BaseLevel {
		level = sp.BaseLevel
	}
	return math.Trunc(e.PerLevel * float64(max(0, level-sp.SpellLevel)))
}

// foreverClientValue is foreverClientRange's low end (single-valued effects have min == max).
func foreverClientValue(c *core.Character, id int32, idx int, classic float64) float64 {
	v, _ := foreverClientRange(c, id, idx, classic, classic)
	return v
}

// foreverRankAt is the highest of the rank ids the character's level has learned (the client's
// spell level, else the listed level). Below rank 1's level it is rank 1 with learned false (a
// talent taken early still casts its first rank).
func foreverRankAt(c *core.Character, ids []int32, levels []int32) (id int32, learned bool) {
	id = ids[0]
	for i, rid := range ids {
		level := levels[i]
		if sp := c.ClientSpell(rid); sp != nil && sp.SpellLevel > 0 {
			level = sp.SpellLevel
		}
		if level <= c.Level {
			id, learned = rid, true
		}
	}
	return id, learned
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
	return core.ManaCostOptions{FlatCost: foreverUse(c, owner, "mana cost", flat, "PROVISIONAL", "client spell "+itoa(id)+" has no mana cost")}
}

// foreverCastTime is the client cast time of a spell (0 for instant client spells).
func foreverCastTime(c *core.Character, id int32, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil {
		return time.Duration(sp.CastMs) * time.Millisecond
	}
	return foreverUseMs(c, owner, "cast ms", classic, "client build has no spell "+itoa(id))
}

// foreverCooldown is the client cooldown of a spell (its own, else its category cooldown).
func foreverCooldown(c *core.Character, id int32, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil {
		if sp.CooldownMs > 0 {
			return time.Duration(sp.CooldownMs) * time.Millisecond
		}
		if sp.CategoryCooldownMs > 0 {
			return time.Duration(sp.CategoryCooldownMs) * time.Millisecond
		}
	}
	return foreverUseMs(c, owner, "cooldown ms", classic, "client spell "+itoa(id)+" has no cooldown")
}

// foreverDuration is the client duration of a spell or aura.
func foreverDuration(c *core.Character, id int32, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil && sp.DurationMs > 0 {
		return time.Duration(sp.DurationMs) * time.Millisecond
	}
	return foreverUseMs(c, owner, "duration ms", classic, "client spell "+itoa(id)+" has no duration; the tooltip value is kept")
}

// foreverCoefficient is the client spell power coefficient of an effect.
func foreverCoefficient(c *core.Character, id int32, idx int, classic float64, owner string) float64 {
	if c.Forever == nil {
		return classic
	}
	if e := c.ClientSpell(id).Effect(idx); e != nil && e.SPCoefficient != nil && *e.SPCoefficient > 0 {
		return *e.SPCoefficient
	}
	return foreverUse(c, owner, "sp coefficient", classic, "PROVISIONAL", "client effect lists no spell power coefficient")
}

// foreverPeriod is the client tick period of a periodic effect.
func foreverPeriod(c *core.Character, id int32, idx int, classic time.Duration, owner string) time.Duration {
	if c.Forever == nil {
		return classic
	}
	if e := c.ClientSpell(id).Effect(idx); e != nil && e.PeriodMs > 0 {
		return time.Duration(e.PeriodMs) * time.Millisecond
	}
	return foreverUseMs(c, owner, "period ms", classic, "client effect has no period")
}

// foreverTrigger is the spell an effect triggers (a proc's buff), or fallback (recorded).
func foreverTrigger(c *core.Character, id int32, idx int, fallback int32, owner string) int32 {
	if c.Forever == nil {
		return fallback
	}
	if e := c.ClientSpell(id).Effect(idx); e != nil && e.TriggerSpell != 0 {
		return e.TriggerSpell
	}
	return int32(foreverUse(c, owner, "trigger spell", float64(fallback), "PROVISIONAL", "client effect names no trigger spell"))
}

// foreverMaxStacks is a client aura's stack limit.
func foreverMaxStacks(c *core.Character, id int32, classic int32, owner string) int32 {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil && sp.MaxStacks > 0 {
		return int32(sp.MaxStacks)
	}
	return int32(foreverUse(c, owner, "max stacks", float64(classic), "PROVISIONAL", "client spell "+itoa(id)+" has no stack limit"))
}

// foreverCharges is a client aura's proc charges.
func foreverCharges(c *core.Character, id int32, classic int32, owner string) int32 {
	if c.Forever == nil {
		return classic
	}
	if sp := c.ClientSpell(id); sp != nil && sp.ProcCharges > 0 {
		return int32(sp.ProcCharges)
	}
	return int32(foreverUse(c, owner, "charges", float64(classic), "PROVISIONAL", "client spell "+itoa(id)+" has no charges"))
}

// talent is a Forever priest talent's client value for client effect `effect` at the chosen
// rank, times sign (the client stores reductions as negative numbers and times in ms). Without
// a client value the research record's value at recordIndex is used and recorded. Rank 0 is 0.
func (p *Priest) talent(id string, effect int, sign float64, recordIndex int) float64 {
	record := "priest.talent." + id
	if p.ForeverRank(record) == 0 {
		return 0
	}
	return sign * p.ClientTalentValue(record, effect, p.ForeverValue(record, recordIndex, 0)/sign)
}

// talentOr is talent for numbers the research record does not carry.
func (p *Priest) talentOr(id string, effect int, sign float64, fallback float64) float64 {
	record := "priest.talent." + id
	if p.ForeverRank(record) == 0 {
		return 0
	}
	return sign * p.ClientTalentValue(record, effect, fallback/sign)
}

// foreverUse records a Forever number with no client source (Forever mode only) and returns it.
func foreverUse(c *core.Character, owner, what string, value float64, confidence, reason string) float64 {
	if c.Forever == nil {
		return value
	}
	return gamedata.Use("priest: "+owner, what, value, confidence, reason)
}

func foreverUseMs(c *core.Character, owner, what string, value time.Duration, reason string) time.Duration {
	return time.Duration(foreverUse(c, owner, what, float64(value/time.Millisecond), "PROVISIONAL", reason)) * time.Millisecond
}
