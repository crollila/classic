package mage

import (
	"math"
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
)

// Forever client-data readers for the mage. Forever-only spells, talent values and proc
// numbers are read from the client build the character simulates (keyed by client spell
// ids and talent records); the typed tables in this package remain the Classic/fallback
// source only. Every value the client does not state is recorded with gamedata.Use.

// foreverClientRange is effect idx of client spell id at the character's level: the
// client's range plus its per-level growth, trunc(perLevel * (level clamped to the spell's
// base..max level - spell level)), which is what the game applies and what the tooltip
// tables typed into this package show. Without the spell it returns lo..hi and records it.
func foreverClientRange(c *core.Character, id int32, idx int, lo, hi float64) (float64, float64) {
	spell := c.ClientSpell(id)
	effect := spell.Effect(idx)
	if effect == nil {
		return c.ClientEffectRange(id, idx, lo, hi)
	}
	a, b := effect.Range()
	return a + foreverLevelBonus(c.Level, spell, effect), b + foreverLevelBonus(c.Level, spell, effect)
}

func foreverLevelBonus(level int32, spell *gamedata.Spell, effect *gamedata.Effect) float64 {
	if effect.PerLevel == 0 {
		return 0
	}
	if spell.MaxLevel > 0 && level > spell.MaxLevel {
		level = spell.MaxLevel
	}
	if level < spell.BaseLevel {
		level = spell.BaseLevel
	}
	return max(0, math.Trunc(effect.PerLevel*float64(level-spell.SpellLevel)))
}

// clientTalent is a mage talent's client value for a client effect index at the
// character's rank (Forever), or the Classic value.
func (m *Mage) clientTalent(id string, effect int, classic float64) float64 {
	if m.Forever == nil {
		return classic
	}
	return m.ClientTalentValue("mage.talent."+id, effect, classic)
}

// talentPct is a talent's client value as a fraction (Forever), or the Classic fraction
// unchanged (so Classic arithmetic stays bit-identical).
func (m *Mage) talentPct(id string, effect int, classic float64) float64 {
	if m.Forever == nil {
		return classic
	}
	return m.ClientTalentValue("mage.talent."+id, effect, classic*100) / 100
}

// clientEffect is effect idx's base value of a client spell (an aura or trigger spell),
// or the given value, recorded as a fallback when the build lacks it.
func (m *Mage) clientEffect(id int32, idx int, fallback float64) float64 {
	if m.Forever == nil {
		return fallback
	}
	return m.ClientEffectValue(id, idx, fallback)
}

// clientDuration is a client spell's duration, or the given one (recorded) when the build
// has none.
func (m *Mage) clientDuration(id int32, fallback time.Duration) time.Duration {
	if m.Forever == nil {
		return fallback
	}
	if s := m.ClientSpell(id); s != nil && s.DurationMs > 0 {
		return time.Duration(s.DurationMs) * time.Millisecond
	}
	gamedata.Use("mage: spell "+itoa(id), "duration_ms", float64(fallback/time.Millisecond), "UNKNOWN", "client build has no duration for this spell; Classic/tooltip value used")
	return fallback
}

// clientMaxStacks is a client aura's stack limit, or the given one (recorded).
func (m *Mage) clientMaxStacks(id int32, fallback int32) int32 {
	if m.Forever == nil {
		return fallback
	}
	if s := m.ClientSpell(id); s != nil && s.MaxStacks > 0 {
		return int32(s.MaxStacks)
	}
	gamedata.Use("mage: spell "+itoa(id), "max_stacks", float64(fallback), "UNKNOWN", "client build has no stack limit for this spell; tooltip value used")
	return fallback
}

// foreverRankSpell is a Forever rank's client values at the character's level: damage
// effect range, mana (flat, or percent of base mana), cast time and coefficient. Values the
// client lacks keep the rank table's.
type foreverRankSpell struct {
	id                 int32
	low, high          float64
	flatMana, pctMana  float64
	cast               time.Duration
	coefficient        float64
	periodic           float64 // periodic effect value per tick, if any
	period, duration   time.Duration
	client             *gamedata.Spell
	hasCoefficientData bool
}

// clientRank reads rank r (data id r.id) with its damage in effect `damage` and an optional
// periodic effect (`periodic` < 0 for none).
func (m *Mage) clientRank(r foreverMageRank, damage, periodic int, cast time.Duration, coefficient float64) foreverRankSpell {
	out := foreverRankSpell{id: r.id, low: r.low, high: r.high, flatMana: r.mana, cast: cast, coefficient: coefficient}
	if m.Forever == nil {
		return out
	}
	s := m.ClientSpell(r.id)
	out.client = s
	out.low, out.high = foreverClientRange(m.GetCharacter(), r.id, damage, r.low, r.high)
	if s == nil {
		return out
	}
	if c := s.Cost("mana"); c != nil {
		if c.CostPct > 0 {
			out.flatMana, out.pctMana = 0, c.CostPct/100
		} else if c.Cost > 0 {
			out.flatMana = c.Cost
		}
	}
	if s.CastMs > 0 {
		out.cast = time.Duration(s.CastMs) * time.Millisecond
	}
	if e := s.Effect(damage); e != nil && e.Coefficient() > 0 {
		out.coefficient, out.hasCoefficientData = e.Coefficient(), true
	}
	if periodic >= 0 {
		if e := s.Effect(periodic); e != nil {
			out.periodic, _ = foreverClientRange(m.GetCharacter(), r.id, periodic, e.Base, e.Base)
			out.period = time.Duration(e.PeriodMs) * time.Millisecond
			out.duration = time.Duration(s.DurationMs) * time.Millisecond
		}
	}
	return out
}

func (r foreverRankSpell) manaCost() core.ManaCostOptions {
	if r.pctMana > 0 {
		return core.ManaCostOptions{BaseCost: r.pctMana}
	}
	return core.ManaCostOptions{FlatCost: r.flatMana}
}

func itoa(id int32) string { return strconv.Itoa(int(id)) }
