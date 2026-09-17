package paladin

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

func (paladin *Paladin) registerForbearance() {
	if paladin.Forever != nil {
		paladin.registerForeverForbearance()
		return
	}

	actionID := core.ActionID{SpellID: 25771}

	forbearanceAura := paladin.RegisterAura(core.Aura{
		Label:    "Forbearance",
		ActionID: actionID,
		Duration: time.Minute * 1,
	})

	paladin.OnSpellRegistered(func(spell *core.Spell) {

		if spell.Flags.Matches(SpellFlag_Forbearance) {
			oldEffect := spell.ApplyEffects

			spell.ApplyEffects = func(sim *core.Simulation, unit *core.Unit, spell *core.Spell) {
				oldEffect(sim, unit, spell)
				forbearanceAura.Activate(sim)
			}

			if spell.ExtraCastCondition != nil {
				oldCondition := spell.ExtraCastCondition
				spell.ExtraCastCondition = func(sim *core.Simulation, target *core.Unit) bool {
					return (!forbearanceAura.IsActive()) && oldCondition(sim, target)
				}
			} else {
				spell.ExtraCastCondition = func(sim *core.Simulation, target *core.Unit) bool {
					return !forbearanceAura.IsActive()
				}
			}
		}
	})
}

func (p *Paladin) registerForeverForbearance() {
	auras := map[*core.Unit]*core.Aura{}
	for _, t := range p.Env.AllUnits {
		if !p.IsOpponent(t) {
			auras[t] = t.GetOrRegisterAura(core.Aura{Label: "Forbearance", ActionID: core.ActionID{SpellID: 25771}, Duration: time.Minute})
		}
	}
	targetFor := func(sp *core.Spell, t *core.Unit) *core.Unit {
		if sp.SpellID != 10278 || t == nil || p.IsOpponent(t) {
			return &p.Unit
		}
		return t
	}
	p.OnSpellRegistered(func(sp *core.Spell) {
		if !sp.Flags.Matches(SpellFlag_Forbearance) {
			return
		}
		apply, cond := sp.ApplyEffects, sp.ExtraCastCondition
		sp.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
			return !auras[targetFor(sp, t)].IsActive() && (cond == nil || cond(sim, t))
		}
		sp.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			apply(sim, t, sp)
			auras[targetFor(sp, t)].Activate(sim)
		}
	})
}
