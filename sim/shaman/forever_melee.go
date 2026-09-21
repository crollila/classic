package shaman

import (
	"github.com/wowsims/classic/sim/core"
	"math"
	"time"
)

func (s *Shaman) registerForeverStormstrike() {
	if s.fr("stormstrike") == 0 {
		return
	}
	auras := s.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.RegisterAura(core.Aura{Label: "Forever Stormstrike-" + s.Label, ActionID: s.fa("stormstrike"), Duration: 12 * time.Second})
	})
	// "Your next Lightning Bolt, Chain Lightning, or Earth Shock": +20% to that one spell
	// (not its Lightning Overload copy), which then uses the effect up.
	empowered := func(sp *core.Spell) bool {
		switch sp.SpellCode {
		case SpellCode_ShamanLightningBolt, SpellCode_ShamanChainLightning, SpellCode_ShamanEarthShock:
			return sp.Tag == 0
		}
		return false
	}
	s.ForeverDamageMultiplier(func(sp *core.Spell, at *core.AttackTable) float64 {
		if empowered(sp) && auras.Get(at.Defender).IsActive() {
			return 1.2
		}
		return 1
	})
	core.MakePermanent(s.RegisterAura(core.Aura{Label: "Forever Stormstrike Consume", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
		if empowered(sp) && r.Target != nil {
			if a := auras.Get(r.Target); a.IsActive() {
				a.Deactivate(sim)
			}
		}
	}}))
	regen := s.RegisterAura(core.Aura{Label: "Forever Improved Stormstrike", ActionID: s.fa("improved-stormstrike"), Duration: 15 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
		s.PseudoStats.SpiritRegenRateCasting += .5
		s.UpdateManaRegenRates()
	}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
		s.PseudoStats.SpiritRegenRateCasting -= .5
		s.UpdateManaRegenRates()
	}})
	s.Stormstrike = s.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 17364}, SpellCode: SpellCode_ShamanStormstrike, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: SpellFlagShaman | core.SpellFlagAPL | core.SpellFlagMeleeMetrics, ManaCost: core.ManaCostOptions{FlatCost: 125}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 8 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		r := sp.CalcAndDealDamage(sim, t, s.MHWeaponDamage(sim, sp.MeleeAttackPower(t)), sp.OutcomeMeleeWeaponSpecialHitAndCrit)
		if r.Landed() {
			auras.Get(t).Activate(sim)
		}
		if sim.Proc(.5*s.fr("improved-stormstrike"), "Forever Improved Stormstrike") {
			regen.Activate(sim)
		}
	}})
	if s.fr("improved-stormstrike") > 0 {
		core.MakePermanent(s.RegisterAura(core.Aura{Label: "Forever Stormstrike Reset", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Outcome.Matches(core.OutcomeDodge|core.OutcomeParry) && sim.Proc(.5*s.fr("improved-stormstrike"), "Forever Stormstrike Reset") {
				s.Stormstrike.CD.Reset()
			}
		}}))
	}
}

func (s *Shaman) registerForeverUtility() {
	// Classic Earthbind (15 sec cooldown, 5 sec pulse) is the provisional
	// underlying totem for the explicitly documented Earthbound root.
	if (s.fr("earthbound") > 0 || s.fr("earth-s-grasp") > 0) && s.Level >= 6 {
		auras := s.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Earthbound-"+s.Label, s.fa("earthbound"), core.ForeverRoot, 5*time.Second)
		})
		snares := s.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Earthbind-"+s.Label, core.ActionID{SpellID: 2484}, core.ForeverSnare, 5*time.Second)
		})
		s.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 2484}, Flags: core.SpellFlagAPL | SpellFlagTotem, ManaCost: core.ManaCostOptions{FlatCost: 20, Multiplier: s.totemManaMultiplier()}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: s.NewTimer(), Duration: 15 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			position := s.DistanceFromTarget
			radius := 10 * (1 + .1*s.fr("earth-s-grasp"))
			if s.fr("earthbound") > 0 {
				for _, target := range s.Env.Encounter.Targets {
					if math.Abs(target.DistanceFromTarget-position) <= radius {
						auras.Get(&target.Unit).Activate(sim)
					}
				}
			}
			s.ActiveTotems[EarthTotem] = sp
			s.TotemExpirations[EarthTotem] = sim.CurrentTime + 45*time.Second
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 5 * time.Second, NumTicks: 9, TickImmediately: true, OnAction: func(sim *core.Simulation) {
				if s.ActiveTotems[EarthTotem] == sp {
					for _, target := range s.Env.Encounter.Targets {
						if math.Abs(target.DistanceFromTarget-position) <= radius {
							snares.Get(&target.Unit).Activate(sim)
						}
					}
				}
			}})
		}})
	}
	if s.fr("improved-ghost-wolf") > 0 && s.Level >= 20 {
		a := s.RegisterAura(core.Aura{Label: "Forever Ghost Wolf", ActionID: core.ActionID{SpellID: 2645}, Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) { s.AddMoveSpeedModifier(&a.ActionID, 1.4) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { s.RemoveMoveSpeedModifier(&a.ActionID) }, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellID != 2645 {
				a.Deactivate(sim)
			}
		}})
		s.RegisterSpell(core.SpellConfig{ActionID: a.ActionID, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{BaseCost: .13}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 3*time.Second - time.Duration(s.ForeverValue("shaman.talent.improved-ghost-wolf", 0, 0))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { a.Activate(sim) }})
	}
}
