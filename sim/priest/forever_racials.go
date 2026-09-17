package priest

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (p *Priest) registerForeverRacials() {
	if p.HasForeverMechanic("priest.baseline.fear-ward") {
		wards := p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlImmunityChargesAura("Forever Fear Ward", p.ForeverAction("priest.baseline.fear-ward"), []core.ForeverControlKind{core.ForeverFear}, 3*time.Minute, 1)
		})
		p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: p.ForeverAction("priest.baseline.fear-ward"), SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if p.IsOpponent(t) {
				t = &p.Unit
			}
			wards.Get(t).Activate(sim)
		}})
	}
	if p.HasForeverMechanic("priest.racial.dark-sacrifice") {
		action := p.ForeverAction("priest.racial.dark-sacrifice")
		metrics := p.NewManaMetrics(action)
		p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 10 * time.Minute}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return p.CurrentHealth() > 720 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Second, NumTicks: 15, OnAction: func(sim *core.Simulation) {
				amount := min(48.0, max(0, p.CurrentHealth()-1))
				p.RemoveHealth(sim, amount)
				p.AddMana(sim, amount, metrics)
			}})
		}})
	}
	if p.HasForeverMechanic("priest.racial.elunes-grace") {
		a := p.RegisterAura(core.Aura{Label: "Forever Elune's Grace", ActionID: p.ForeverAction("priest.racial.elunes-grace"), Duration: 15 * time.Second, MaxStacks: 3, OnGain: func(a *core.Aura, sim *core.Simulation) {
			p.PseudoStats.BonusMeleeHitRatingTaken -= 50 * core.MeleeHitRatingPerHitChance
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			p.PseudoStats.BonusMeleeHitRatingTaken += 50 * core.MeleeHitRatingPerHitChance
		}, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.ProcMask.Matches(core.ProcMaskMeleeOrRanged) && r.Outcome.Matches(core.OutcomeMiss) {
				a.RemoveStack(sim)
			}
		}})
		p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: p.ForeverAction("priest.racial.elunes-grace"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 26}, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 5 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim); a.SetStacks(sim, 3) }})
	}
	if p.HasForeverMechanic("priest.racial.touch-of-weakness") {
		action := p.ForeverAction("priest.racial.touch-of-weakness")
		damage := p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action.WithTag(-action.Tag), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 1})
		debuff := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.GetOrRegisterAura(core.Aura{Label: "Forever Touch of Weakness", ActionID: action, Duration: 2 * time.Minute, OnGain: func(a *core.Aura, sim *core.Simulation) { t.AddStatDynamic(sim, stats.AttackPower, -20) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { t.AddStatDynamic(sim, stats.AttackPower, 20) }})
		})
		a := p.RegisterAura(core.Aura{Label: "Forever Touch of Weakness", ActionID: action, Duration: 10 * time.Minute, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) {
				a.Deactivate(sim)
				damage.CalcAndDealDamage(sim, s.Unit, 48, damage.OutcomeAlwaysHit)
				debuff.Get(s.Unit).Activate(sim)
			}
		}})
		p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 90}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	for _, id := range []string{"priest.racial.dwarf.chastise", "priest.racial.gnome.confounding-flash"} {
		if !p.HasForeverMechanic(id) {
			continue
		}
		id := id
		kind := core.ForeverRoot
		dur := 2 * time.Second
		cd := 30 * time.Second
		if id == "priest.racial.gnome.confounding-flash" {
			kind = core.ForeverStun
			dur = 3 * time.Second
			cd = 3 * time.Minute
		}
		auras := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura(id+"-"+p.Label, p.ForeverAction(id), kind, dur)
		})
		p.RegisterSpell(core.SpellConfig{ActionID: p.ForeverAction(id), SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 50}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: cd}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .107,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				if id == "priest.racial.dwarf.chastise" {
					s.CalcAndDealDamage(sim, t, 50, s.OutcomeMagicHitAndCrit)
					auras.Get(t).Activate(sim)
				} else {
					for _, t := range sim.Encounter.TargetUnits {
						auras.Get(t).Activate(sim)
					}
				}
			},
		})
	}
	for _, id := range []string{"priest.racial.dwarf.desperate-prayer", "priest.racial.human.divine-grace"} {
		if !p.HasForeverMechanic(id) {
			continue
		}
		id := id
		p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: p.ForeverAction(id), SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 10 * time.Minute}}, DamageMultiplier: 1, ThreatMultiplier: 0, BonusCoefficient: .429, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if id == "priest.racial.dwarf.desperate-prayer" {
				t = &p.Unit
			} else if a := p.WeakenedSouls.Get(t); a != nil {
				a.Deactivate(sim)
			}
			s.CalcAndDealHealing(sim, t, sim.Roll(1324, 1563), s.OutcomeHealingCrit)
		}})
	}
	if p.HasForeverMechanic("priest.racial.gnome.contingency-plan") {
		action := p.ForeverAction("priest.racial.gnome.contingency-plan")
		heal := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: action.WithTag(-action.Tag), SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagHelpful | core.SpellFlagPassiveSpell | core.SpellFlagIgnoreAttackerModifiers, DamageMultiplier: 1, ThreatMultiplier: 0})
		auras := p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
			a := t.GetOrRegisterAura(core.Aura{Label: "Forever Contingency Plan", ActionID: action, Duration: 10 * time.Second})
			t.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
				if a.IsActive() && r.Damage >= t.CurrentHealth() {
					a.Deactivate(sim)
					r.Damage = 0
					heal.CalcAndDealHealing(sim, t, .5*t.MaxHealth(), heal.OutcomeHealing)
				}
			})
			return a
		})
		p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagHelpful | core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { auras.Get(t).Activate(sim) }})
	}
}
