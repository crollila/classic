package core

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/stats"
)

// Client set membership and thresholds are build-scoped. Database display names
// and old Classic thresholds are deliberately not used for Forever activation.
func (c *Character) clientSetBonuses() []ActiveSetBonus {
	equipped := map[int32]bool{}
	for _, item := range c.Equipment {
		if item.ID != 0 {
			equipped[item.ID] = true
		}
	}
	keys := make([]string, 0, len(c.GameData.RawItemSets))
	for key := range c.GameData.RawItemSets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out []ActiveSetBonus
	for _, key := range keys {
		set := c.GameData.RawItemSets[key]
		count, seen := 0, map[int32]bool{}
		for _, id := range set.Items {
			if equipped[id] && !seen[id] {
				count++
				seen[id] = true
			}
		}
		seenBonus := map[string]bool{}
		for _, bonus := range set.Bonuses {
			identity := fmt.Sprintf("%d:%d", bonus.Pieces, bonus.SpellID)
			if bonus.Pieces <= 0 || count < bonus.Pieces || seenBonus[identity] {
				continue
			}
			seenBonus[identity] = true
			spell := c.GameData.Spell(bonus.SpellID)
			apply, gaps := c.planClientSetSpell(spell)
			name := fmt.Sprintf("%s · spell %d", set.Name, bonus.SpellID)
			if len(gaps) > 0 {
				name += " [incomplete: " + strings.Join(gaps, "; ") + "]"
			}
			out = append(out, ActiveSetBonus{Name: name, NumPieces: int32(bonus.Pieces), BonusEffect: func(agent Agent) {
				for _, effect := range apply {
					effect(agent.GetCharacter())
				}
			}})
		}
	}
	return out
}

type clientSetEffect func(*Character)

// Effect IDs follow the client's aura enums. Only reviewed semantic handlers are
// accepted. Scripted/dummy effects remain explicit gaps, never guessed numbers.
func (c *Character) planClientSetSpell(spell *gamedata.Spell) ([]clientSetEffect, []string) {
	if spell == nil {
		return nil, []string{"missing client spell"}
	}
	if len(spell.Effects) == 0 {
		return nil, []string{"missing client effects"}
	}
	if spell.RequiresItemClass != nil && *spell.RequiresItemClass >= 0 {
		return nil, []string{"equipment-restricted bonus needs an adapter"}
	}
	if spell.DurationMs > 0 {
		return nil, []string{"temporary set spell needs an activation adapter"}
	}
	var actions []clientSetEffect
	var gaps []string
	for index, e := range spell.Effects {
		action, reason := c.planClientSetEffect(spell, e)
		if reason != "" {
			if e != nil {
				index = e.Index
			}
			gaps = append(gaps, fmt.Sprintf("effect %d: %s", index, reason))
		} else if action != nil {
			actions = append(actions, action)
		}
	}
	return actions, gaps
}

func (c *Character) planClientSetEffect(parent *gamedata.Spell, e *gamedata.Effect) (clientSetEffect, string) {
	if e == nil {
		return nil, "missing effect definition"
	}
	if e.PerLevel != 0 || e.PointsPerCombo != 0 || e.Variance != 0 || e.APCoefficient != 0 || e.Coefficient() != 0 {
		return nil, "scaled or variable set effect needs an adapter"
	}
	if e.Effect != 6 {
		return nil, fmt.Sprintf("unmodeled effect %d", e.Effect)
	}
	if len(e.Targets) != 1 || e.Targets[0] != 1 {
		return nil, "non-self target needs an adapter"
	}
	v := e.Base
	misc := int32(0)
	if len(e.Misc) > 0 {
		misc = e.Misc[0]
	}
	add := func(stat stats.Stat) (clientSetEffect, string) { return func(c *Character) { c.AddStat(stat, v) }, "" }
	switch e.Aura {
	case 29:
		if misc >= 0 && misc <= 4 {
			return add(stats.Stat(misc))
		}
		if misc == -1 {
			return func(c *Character) {
				for stat := stats.Strength; stat <= stats.Spirit; stat++ {
					c.AddStat(stat, v)
				}
			}, ""
		}
	case 99:
		return add(stats.AttackPower)
	case 124:
		return add(stats.RangedAttackPower)
	case 54:
		return add(stats.MeleeHit)
	case 55:
		return add(stats.SpellHit)
	case 52:
		return add(stats.MeleeCrit)
	case 57:
		return add(stats.SpellCrit)
	case 290:
		return func(c *Character) { c.AddStat(stats.MeleeCrit, v); c.AddStat(stats.SpellCrit, v) }, ""
	case 47:
		return add(stats.Parry)
	case 49:
		return add(stats.Dodge)
	case 51:
		return add(stats.Block)
	case 274:
		return add(stats.BlockValue)
	case 30:
		if misc == 95 {
			return add(stats.Defense)
		}
	case 135:
		if misc == 126 || misc == 127 {
			return add(stats.HealingPower)
		}
	case 13:
		if misc == 126 {
			return add(stats.SpellDamage)
		}
		if misc > 0 && misc & ^int32(126) == 0 {
			return func(c *Character) {
				for bit, stat := range map[int32]stats.Stat{2: stats.HolyPower, 4: stats.FirePower, 8: stats.NaturePower, 16: stats.FrostPower, 32: stats.ShadowPower, 64: stats.ArcanePower} {
					if misc&bit != 0 {
						c.AddStat(stat, v)
					}
				}
			}, ""
		}
	case 22:
		if misc > 0 && misc & ^int32(125) == 0 {
			return func(c *Character) {
				for bit, stat := range map[int32]stats.Stat{1: stats.Armor, 4: stats.FireResistance, 8: stats.NatureResistance, 16: stats.FrostResistance, 32: stats.ShadowResistance, 64: stats.ArcaneResistance} {
					if misc&bit != 0 {
						c.AddStat(stat, v)
					}
				}
			}, ""
		}
	case 85:
		if misc == 0 {
			return add(stats.MP5)
		}
	case 342:
		return func(c *Character) {
			c.PseudoStats.MeleeSpeedMultiplier *= 1 + v/100
			c.PseudoStats.RangedSpeedMultiplier *= 1 + v/100
		}, ""
	case 65:
		return nil, "non-stacking cast haste needs a shared stacking adapter"
	case 107, 108:
		if misc == 7 && e.Aura == 108 {
			return nil, "percent crit modifier needs an adapter"
		}
		if misc != 7 && misc != 10 && misc != 11 {
			return nil, fmt.Sprintf("spell modifier operation %d needs an adapter", misc)
		}
		if parent.SpellClass == "" || len(e.ClassMask) == 0 {
			return nil, "missing spell-family selector"
		}
		return func(c *Character) {
			c.OnSpellRegistered(func(s *Spell) {
				data := c.ClientSpell(s.SpellID)
				if data == nil || data.SpellClass != parent.SpellClass || !clientMaskOverlap(e.ClassMask, data.SpellClassMask) {
					return
				}
				switch misc {
				case 7:
					if e.Aura == 107 {
						s.BonusCritRating += v
					}
				case 10:
					if e.Aura == 107 {
						s.DefaultCast.CastTime = max(0, s.DefaultCast.CastTime+time.Duration(v*float64(time.Millisecond)))
					} else {
						s.CastTimeMultiplier *= max(0, 1+v/100)
					}
				case 11:
					if e.Aura == 107 {
						s.CD.Duration = max(0, s.CD.Duration+time.Duration(v*float64(time.Millisecond)))
					} else {
						s.CD.Duration = time.Duration(float64(s.CD.Duration) * max(0, 1+v/100))
					}
				}
			})
		}, ""
	case 189:
		// The UI/core use percent units for hit/crit. Convert rating using the
		// selected client build and character level, not a fixed level-60 divisor.
		if misc <= 0 || misc & ^int32(224|1792) != 0 {
			break
		}
		if (misc&96 != 0 && misc&96 != 96) || (misc&768 != 0 && misc&768 != 768) {
			return nil, "melee-only or ranged-only rating needs separate stats"
		}
		delta := stats.Stats{}
		for _, r := range []struct {
			mask   int32
			stat   stats.Stat
			column string
		}{{96, stats.MeleeHit, "Hit - Melee"}, {128, stats.SpellHit, "Hit - Spell"}, {768, stats.MeleeCrit, "Crit - Melee"}, {1024, stats.SpellCrit, "Crit - Spell"}} {
			if misc&r.mask == 0 {
				continue
			}
			conversion, ok := c.GameData.Value("combatratings", strconv.Itoa(int(c.Level)), r.column)
			if !ok || conversion <= 0 {
				return nil, "missing rating conversion"
			}
			delta[r.stat] += v / conversion
		}
		return func(c *Character) { c.AddStats(delta) }, ""
	}
	return nil, fmt.Sprintf("unmodeled aura %d", e.Aura)
}

func clientMaskOverlap(a, b []int64) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if uint32(a[i])&uint32(b[i]) != 0 {
			return true
		}
	}
	return false
}
