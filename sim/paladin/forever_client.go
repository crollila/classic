package paladin

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever client values the generic client-data layer cannot reach: numbers inside effect
// closures (weapon-% strikes, seal procs, judgements), Forever-only spells registered under
// simulator action ids, and talent values. Every helper returns the client's value for the
// build the character simulates; the typed Classic/fallback value is used only in Classic mode
// or when the build lacks the spell, and then it is recorded (gamedata.Use), never silent.

// clientLevelRange is effect `index` of client spell `id` at the paladin's level: the client's
// base range plus RealPointsPerLevel for every level above the spell level, capped at the
// spell's max level (the server's CalculateSpellDamage rule). When the client has no max level
// the Classic cap `classicCap` is used (0: none). ok is false without client data.
func clientLevelRange(c *core.Character, id int32, index int, classicCap int32) (lo, hi float64, ok bool) {
	spell := c.ClientSpell(id)
	e := spell.Effect(index)
	if e == nil {
		return 0, 0, false
	}
	lo, hi = e.Range()
	if e.PerLevel != 0 {
		base := spell.SpellLevel
		if base == 0 {
			base = spell.BaseLevel
		}
		level := c.Level
		if spell.MaxLevel > 0 {
			level = min(level, spell.MaxLevel)
		} else if classicCap > 0 {
			level = min(level, classicCap)
		}
		if level > base {
			lo += e.PerLevel * float64(level-base)
			hi += e.PerLevel * float64(level-base)
		}
	}
	return lo, hi, true
}

// clientRange is clientLevelRange with the Classic/fallback range: Classic mode returns the
// fallback unchanged; a Forever build without the spell returns it and records it.
func (paladin *Paladin) clientRange(id int32, index int, classicCap int32, fallbackLo, fallbackHi float64) (float64, float64) {
	if paladin.GameData == nil {
		return fallbackLo, fallbackHi
	}
	if lo, hi, ok := clientLevelRange(&paladin.Character, id, index, classicCap); ok {
		return lo, hi
	}
	return paladin.ClientEffectRange(id, index, fallbackLo, fallbackHi) // records the fallback
}

// clientValue is clientRange's low end, for single-valued effects.
func (paladin *Paladin) clientValue(id int32, index int, fallback float64) float64 {
	lo, _ := paladin.clientRange(id, index, 0, fallback, fallback)
	return lo
}

// clientCoefficient is effect `index`'s spell power coefficient, or fallback.
func (paladin *Paladin) clientCoefficient(id int32, index int, fallback float64) float64 {
	if paladin.GameData == nil {
		return fallback
	}
	if e := paladin.ClientSpell(id).Effect(index); e != nil && e.SPCoefficient != nil {
		return *e.SPCoefficient
	}
	return gamedata.Use(fmt.Sprintf("spell %d effect %d", id, index), "sp coefficient", fallback, "UNKNOWN",
		"client build has no such spell/effect; fallback coefficient used")
}

// clientMana is the spell's flat mana cost, or fallback.
func (paladin *Paladin) clientMana(id int32, fallback float64) float64 {
	if paladin.GameData == nil {
		return fallback
	}
	if cost := paladin.ClientSpell(id).Cost("mana"); cost != nil && cost.Cost > 0 {
		return cost.Cost
	}
	return gamedata.Use(fmt.Sprintf("spell %d", id), "mana cost", fallback, "UNKNOWN", "client build has no mana cost for this spell")
}

// clientCooldown is the spell's cooldown (or category cooldown), or fallback.
func (paladin *Paladin) clientCooldown(id int32, fallback time.Duration) time.Duration {
	if paladin.GameData == nil {
		return fallback
	}
	if spell := paladin.ClientSpell(id); spell != nil {
		if ms := max(spell.CooldownMs, spell.CategoryCooldownMs); ms > 0 {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return time.Duration(gamedata.Use(fmt.Sprintf("spell %d", id), "cooldown ms", float64(fallback.Milliseconds()), "UNKNOWN",
		"client build has no cooldown for this spell")) * time.Millisecond
}

// clientCastTime is the spell's cast time, or fallback.
func (paladin *Paladin) clientCastTime(id int32, fallback time.Duration) time.Duration {
	if paladin.GameData == nil {
		return fallback
	}
	if spell := paladin.ClientSpell(id); spell != nil && spell.CastMs > 0 {
		return time.Duration(spell.CastMs) * time.Millisecond
	}
	return fallback
}

// ft is a Forever talent's client value for effect `index` at the chosen rank (the Trait node's
// per-rank points, else the talent spell's effect). fallback, in the client's units, is used only
// without client data for the record (recorded by ClientTalentValue).
func (paladin *Paladin) ft(name string, index int, fallback float64) float64 {
	return paladin.ClientTalentValue("paladin.talent."+name, index, fallback)
}

// ftPct is ft as a fraction: +5 -> .05, -20 -> -.2.
func (paladin *Paladin) ftPct(name string, index int, fallbackPct float64) float64 {
	return paladin.ft(name, index, fallbackPct) / 100
}

// ftMs is ft for a client millisecond value, as a duration.
func (paladin *Paladin) ftMs(name string, index int, fallbackMs float64) time.Duration {
	return time.Duration(paladin.ft(name, index, fallbackMs)) * time.Millisecond
}

// foreverUse records a value the client does not state (a hidden proc rate, a server-side
// rule) and returns it. Owner names start with "paladin:" so reports can group them.
func foreverUse(what string, value float64, confidence, reason string) float64 {
	return gamedata.Use("paladin: "+what, "value", value, confidence, reason)
}
