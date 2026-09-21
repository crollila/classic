package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

// Baselines retained from assets/db_inputs/wowhead_spell_tooltips.csv:
// Drain Mana11704, Curse of Weakness11708. Forever interactions are provisional.
func (w *Warlock) registerForeverMissingAffliction() {
	for _, t := range w.Env.Encounter.Targets {
		t.ForeverEnableManaPool(max(t.GetStat(stats.Mana), w.ForeverParameter("scenario.enemy_mana", 0)))
	}
	action := core.ActionID{SpellID: 11704}
	gain := w.NewManaMetrics(action)
	loss := map[int32]*core.ResourceMetrics{}
	for _, t := range w.Env.Encounter.Targets {
		if t.HasManaBar() {
			loss[t.UnitIndex] = t.NewManaMetrics(action)
		}
	}
	drain := w.RegisterSpell(core.SpellConfig{ActionID: action, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: WarlockFlagAffliction | core.SpellFlagAPL | core.SpellFlagChanneled,
		ManaCost: core.ManaCostOptions{FlatCost: 310}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}},
		ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			// Drain Mana is learned at 24; lower levels cannot cast it.
			return w.Level >= 24 && t.HasManaBar() && t.CurrentMana() > 0 && w.DistanceFromTarget <= 20
		},
		Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Drain Mana-" + w.Label}, NumberOfTicks: 5, TickLength: time.Second,
			OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				amount := min(140, t.CurrentMana())
				t.SpendMana(sim, amount, loss[t.UnitIndex])
				w.AddMana(sim, amount, gain)
				if t.CurrentMana() <= 0 {
					d.Deactivate(sim)
				}
			},
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcAndDealOutcome(sim, t, s.OutcomeMagicHit)
			if r.Landed() {
				s.Dot(t).Apply(sim)
			}
		},
	})
	drain.PushbackReduction += w.ForeverValue("warlock.talent.fel-concentration", 0, 0) / 100
	// Curse of Weakness at the rank learned by the warlock's level (Classic values).
	weakRank := rankAtLevel([]int{0, 4, 12, 22, 32, 42, 52}, w.Level)
	if weakRank == 0 {
		return
	}
	weakID := [7]int32{0, 702, 1108, 6205, 7646, 11707, 11708}[weakRank]
	weakValue := [7]float64{0, 3, 6, 10, 15, 22, 31}[weakRank]
	weakCost := [7]float64{0, 20, 35, 70, 95, 130, 175}[weakRank]
	makeWeakness := func(factor float64) core.AuraArray {
		return w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			label := "Forever Curse of Weakness-" + w.Label
			if factor > 1 {
				label += " amplified"
			}
			a := t.GetOrRegisterAura(core.Aura{Label: label, ActionID: core.ActionID{SpellID: weakID}, Tag: "forever-debuff-curse", Duration: 2 * time.Minute})
			a.NewExclusiveEffect("Forever Curse of Weakness", false, core.ExclusiveEffect{Priority: weakValue * factor,
				OnGain:   func(e *core.ExclusiveEffect, sim *core.Simulation) { t.PseudoStats.BonusPhysicalDamage -= e.Priority },
				OnExpire: func(e *core.ExclusiveEffect, sim *core.Simulation) { t.PseudoStats.BonusPhysicalDamage += e.Priority },
			})
			return a
		})
	}
	weak, amplified := makeWeakness(1), makeWeakness(1.5)
	w.CurseOfWeakness = w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: weakID}, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskEmpty, Flags: WarlockFlagAffliction | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: weakCost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ThreatMultiplier: 1,
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcAndDealOutcome(sim, t, s.OutcomeMagicHit)
			if !r.Landed() {
				return
			}
			a := weak.Get(t)
			if w.AmplifyCurseAura != nil && w.AmplifyCurseAura.IsActive() {
				a = amplified.Get(t)
				w.AmplifyCurseAura.Deactivate(sim)
			}
			if old := w.ActiveCurseAura.Get(t); old != nil {
				old.Deactivate(sim)
			}
			w.ActiveCurseAura[t.UnitIndex] = a
			a.Activate(sim)
		},
	})
}
