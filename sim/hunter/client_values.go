package hunter

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever client reads for hunter abilities. In Forever mode a hunter's numbers come from the
// client build it simulates (character.GameData), keyed by the rank's client spell id, so a
// Blizzard change reaches the simulation with no code edit. The per-rank tables in the ability
// files stay as the Classic values and as the fallback when a build has no such spell; every
// fallback in Forever mode is recorded through gamedata.Use, never silent.

// clientRank is the index (0-based) of the highest rank in ids a unit of `level` has learned, or
// -1 for none. Forever: a rank's learned level is its client spell's base level, and a rank the
// build does not have is skipped. Without client data it is levels[i].
func clientRank(c *core.Character, ids []int32, levels []int32, level int32) int {
	best := -1
	for i, id := range ids {
		learned := levels[i]
		if c.GameData != nil {
			spell := c.GameData.Spell(id)
			if spell == nil {
				continue
			}
			if spell.BaseLevel > 0 {
				learned = spell.BaseLevel
			} else if spell.SpellLevel > 0 {
				learned = spell.SpellLevel
			}
		}
		if learned <= level {
			best = i
		}
	}
	return best
}

// talentRank is clientRank for a spell a talent grants: the talent teaches rank 1 whatever the
// level, so the result is at least 0.
func talentRank(c *core.Character, ids []int32, levels []int32, level int32) int {
	return max(0, clientRank(c, ids, levels, level))
}

func clientMissing(id int32, field string, value float64) float64 {
	return gamedata.Use(fmt.Sprintf("hunter: spell %d", id), field, value, "UNKNOWN",
		"client build has no value for this spell; simulator value used")
}

// clientManaCost is the spell's flat mana cost in the client build, else fallback.
func clientManaCost(c *core.Character, id int32, fallback float64) float64 {
	if c.GameData == nil {
		return fallback
	}
	if cost := c.GameData.Spell(id).Cost("mana"); cost != nil && cost.Cost > 0 {
		return cost.Cost
	}
	return clientMissing(id, "mana cost", fallback)
}

// clientDuration reads a millisecond field of the client spell; a zero or negative value is
// "none" and falls back.
func clientDuration(c *core.Character, id int32, field string, get func(*gamedata.Spell) int, fallback time.Duration) time.Duration {
	if c.GameData == nil {
		return fallback
	}
	if spell := c.GameData.Spell(id); spell != nil {
		if ms := get(spell); ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return time.Duration(clientMissing(id, field, float64(fallback/time.Millisecond))) * time.Millisecond
}

func clientCastTime(c *core.Character, id int32, fallback time.Duration) time.Duration {
	return clientDuration(c, id, "cast time", func(s *gamedata.Spell) int { return s.CastMs }, fallback)
}

// clientCooldown is the spell's cooldown, or its category cooldown when it has only that.
func clientCooldown(c *core.Character, id int32, fallback time.Duration) time.Duration {
	return clientDuration(c, id, "cooldown", func(s *gamedata.Spell) int { return max(s.CooldownMs, s.CategoryCooldownMs) }, fallback)
}

func clientAuraDuration(c *core.Character, id int32, fallback time.Duration) time.Duration {
	return clientDuration(c, id, "duration", func(s *gamedata.Spell) int { return s.DurationMs }, fallback)
}

// clientTicks is a periodic effect's tick count and length: the spell's duration over the
// effect's period.
func clientTicks(c *core.Character, id int32, effect int, fallbackTicks int32, fallbackPeriod time.Duration) (int32, time.Duration) {
	if c.GameData == nil {
		return fallbackTicks, fallbackPeriod
	}
	spell := c.GameData.Spell(id)
	if e := spell.Effect(effect); e != nil && e.PeriodMs > 0 && spell.DurationMs > 0 {
		return int32(spell.DurationMs / e.PeriodMs), time.Duration(e.PeriodMs) * time.Millisecond
	}
	clientMissing(id, fmt.Sprintf("effect %d period", effect), float64(fallbackPeriod/time.Millisecond))
	return fallbackTicks, fallbackPeriod
}

// clientCoefficient is an effect's spell power coefficient in the client build, else fallback.
func clientCoefficient(c *core.Character, id int32, effect int, fallback float64) float64 {
	if c.GameData == nil {
		return fallback
	}
	if e := c.GameData.Spell(id).Effect(effect); e != nil && e.SPCoefficient != nil {
		return *e.SPCoefficient
	}
	return clientMissing(id, fmt.Sprintf("effect %d spell power coefficient", effect), fallback)
}

// clientTalent is a Forever talent's client value at the hunter's rank for `effect`; fallback is
// the simulator's own value and is what Classic mode (or a build without the talent) uses.
func (hunter *Hunter) clientTalent(record string, effect int, fallback float64) float64 {
	return hunter.ClientTalentValue(record, effect, fallback)
}
