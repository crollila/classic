package rogue

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

func (rogue *Rogue) registerPreparationCD() {
	if !rogue.Talents.Preparation {
		return
	}

	rogue.Preparation = rogue.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: 14185},
		Flags:    core.SpellFlagAPL,
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: time.Second,
			},
			CD: core.Cooldown{
				Timer:    rogue.NewTimer(),
				Duration: time.Minute * 10,
			},
			IgnoreHaste: true,
		},
		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			// Spells affected by Preparation are: Cold Blood, Shadowstep, Vanish (Overkill/Master of Subtlety), Evasion, Sprint
			var affectedSpells = []*core.Spell{rogue.ColdBlood, rogue.Shadowstep, rogue.Vanish}
			if rogue.Forever != nil {
				affectedSpells = nil
				for _, s := range rogue.Spellbook {
					allowed := false
					switch s.SpellID {
					case 14177, 1856, 1857, 5277, 13877, 13750, 14183, 14278, 14251, 2094,
						2983, 8696, 11305, // Sprint
						1776, 1777, 8629, 11285, 11286, // Gouge
						1766, 1767, 1768, 1769, // Kick
						408, 8643, // Kidney Shot
						1784, 1785, 1786, 1787: // Stealth
						allowed = true
					}
					if allowed && s != spell && s.CD.Timer != nil {
						affectedSpells = append(affectedSpells, s)
					}
				}
			}
			// Reset Cooldown on affected spells
			for _, affectedSpell := range affectedSpells {
				if affectedSpell != nil {
					affectedSpell.CD.Reset()
				}
			}
		},
	})

	rogue.AddMajorCooldown(core.MajorCooldown{
		Spell:    rogue.Preparation,
		Type:     core.CooldownTypeDPS,
		Priority: core.CooldownPriorityDefault,
		ShouldActivate: func(sim *core.Simulation, character *core.Character) bool {
			return rogue.Vanish == nil || !rogue.Vanish.CD.IsReady(sim)
		},
	})
}
