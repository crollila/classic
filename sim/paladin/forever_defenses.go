package paladin

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"time"
)

func (p *Paladin) registerForeverDefenses() {
	for _, id := range []int32{4987, 1152} {
		p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: id}, SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{BaseCost: .06, Multiplier: 100 - int32(10*p.fr("purifying-power"))}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if p.IsOpponent(t) {
				t = &p.Unit
			}
			t.ForeverDispel(sim, "poison")
			t.ForeverDispel(sim, "disease")
			if id == 4987 {
				t.ForeverDispel(sim, "magic")
			}
		}})
	}
	for _, v := range []struct {
		id       int32
		duration time.Duration
		cost     float64
		school   core.SpellSchool
	}{{1020, 12 * time.Second, 110, 0}, {5573, 8 * time.Second, 35, core.SpellSchoolPhysical}} {
		v := v
		shield := core.NewForeverAbsorb(&p.Unit, "Forever Divine Defense-"+core.ActionID{SpellID: v.id}.String(), core.ActionID{SpellID: v.id}, v.duration, v.school)
		// Preserve the verified Classic defensive penalties while Sacred Duty
		// changes both cooldowns. This does not invent new Forever immunities.
		shield.Aura.OnGain = func(_ *core.Aura, sim *core.Simulation) {
			if v.id == 1020 {
				p.MultiplyMeleeSpeed(sim, .5)
			} else {
				p.AutoAttacks.CancelAutoSwing(sim)
			}
		}
		originalExpire := shield.Aura.OnExpire
		shield.Aura.OnExpire = func(a *core.Aura, sim *core.Simulation) {
			if originalExpire != nil {
				originalExpire(a, sim)
			}
			if v.id == 1020 {
				p.MultiplyMeleeSpeed(sim, 2)
			} else {
				p.AutoAttacks.EnableAutoSwing(sim)
			}
		}
		if v.id == 5573 {
			p.OnSpellRegistered(func(sp *core.Spell) {
				if sp.SpellSchool.Matches(core.SpellSchoolPhysical) {
					old := sp.ExtraCastCondition
					sp.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
						return !shield.Aura.IsActive() && (old == nil || old(sim, t))
					}
				}
			})
		}
		p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, Flags: core.SpellFlagAPL | core.SpellFlagHelpful | SpellFlag_Forbearance, ManaCost: core.ManaCostOptions{FlatCost: v.cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 5*time.Minute - time.Duration(30*p.fr("sacred-duty"))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { shield.Apply(sim, 1e15, false) }})
	}
	if p.fr("templar-s-bulwark") > 0 {
		shield := core.NewForeverAbsorb(&p.Unit, "Forever Templar's Bulwark", p.fa("templar-s-bulwark"), 8*time.Second, 0)
		p.RegisterSpell(core.SpellConfig{ActionID: p.fa("templar-s-bulwark"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful | SpellFlag_Forbearance, ManaCost: core.ManaCostOptions{FlatCost: 110}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 5*time.Minute - time.Duration(30*p.fr("sacred-duty"))*time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { shield.Apply(sim, p.MaxHealth(), false) }})
	}
	if p.fr("voice-of-truth") > 0 {
		a := p.ForeverControlImmunityAura("Forever Voice of Truth", p.fa("voice-of-truth"), []core.ForeverControlKind{core.ForeverSilence}, 6*time.Second)
		p.RegisterSpell(core.SpellConfig{ActionID: p.fa("voice-of-truth"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { a.Activate(sim) }})
	}
	for _, v := range []struct {
		id           int32
		name         string
		duration, cd time.Duration
		kind         core.ForeverControlKind
		enabled      bool
	}{{853, "improved-hammer-of-justice", 6 * time.Second, 60*time.Second - time.Duration(5*p.fr("improved-hammer-of-justice"))*time.Second, core.ForeverStun, true}, {20066, "repentance", 6 * time.Second, time.Minute, core.ForeverFear, p.fr("repentance") > 0}} {
		if !v.enabled {
			continue
		}
		v := v
		auras := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			a := t.ForeverControlAura("Forever "+v.name+"-"+p.Label, core.ActionID{SpellID: v.id}, v.kind, v.duration)
			if v.id == 20066 {
				onDamage := func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
					if r.Damage > 0 {
						a.Deactivate(sim)
					}
				}
				a.OnSpellHitTaken = onDamage
				a.OnPeriodicDamageTaken = onDamage
			}
			return a
		})
		p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: core.TernaryFloat64(v.id == 20066, 60, p.BaseMana*.03)}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: v.cd}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return v.id != 20066 || t.MobType == proto.MobType_MobTypeHumanoid
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if sp.CalcOutcome(sim, t, sp.OutcomeMagicHit).Landed() {
				auras.Get(t).Activate(sim)
			}
		}})
	}
	if p.fr("guardian-s-favor") > 0 {
		a := p.ForeverControlImmunityAura("Forever Blessing of Freedom", core.ActionID{SpellID: 1044}, []core.ForeverControlKind{core.ForeverRoot, core.ForeverSnare}, 10*time.Second+time.Duration(3*p.fr("guardian-s-favor"))*time.Second)
		p.RegisterSpell(core.SpellConfig{ActionID: a.ActionID, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 20 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { a.Activate(sim) }})
		shield := core.NewForeverAbsorb(&p.Unit, "Forever Blessing of Protection", core.ActionID{SpellID: 10278}, 10*time.Second, core.SpellSchoolPhysical)
		p.RegisterSpell(core.SpellConfig{ActionID: shield.Aura.ActionID, Flags: core.SpellFlagAPL | core.SpellFlagHelpful | SpellFlag_Forbearance, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 5*time.Minute - time.Duration(p.fr("guardian-s-favor"))*time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { shield.Apply(sim, 1e15, false) }})
	}
	if p.fr("divine-favor") > 0 {
		var affected []*core.Spell
		for _, sp := range p.Spellbook {
			if sp.SpellCode == foreverHolyLight || sp.SpellCode == foreverFlashOfLight || sp.SpellCode == SpellCode_PaladinHolyShock {
				affected = append(affected, sp)
			}
		}
		a := p.RegisterAura(core.Aura{Label: "Forever Divine Favor", ActionID: core.ActionID{SpellID: 20216}, Duration: core.NeverExpires, OnGain: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range affected {
				sp.BonusCritRating += 100 * core.SpellCritRatingPerCritChance
			}
		}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range affected {
				sp.BonusCritRating -= 100 * core.SpellCritRatingPerCritChance
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			for _, s := range affected {
				if s == sp {
					a.Deactivate(sim)
				}
			}
		}})
		p.RegisterSpell(core.SpellConfig{ActionID: a.ActionID, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 37}, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 2 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { a.Activate(sim) }})
	}
	// The shield itself lacks a visible numerical tooltip; use a small 60-point
	// per-swing shield as a PREDICTED scenario. Its known fully-absorbed mana
	// return and attacker-level scaling use the observed numbers exactly.
	if p.HasForeverMechanic("paladin.baseline.seal-of-fury") || p.fr("improved-seal-of-fury") > 0 {
		action := p.ForeverAction("paladin.baseline.seal-of-fury")
		shield := core.NewForeverAbsorb(&p.Unit, "Forever Seal of Fury Shield", action, 10*time.Second, 0)
		metrics := p.NewManaMetrics(p.fa("improved-seal-of-fury"))
		// The absorption hook runs before OnSpellHitTaken; record the active pool
		// before each incoming hit so expiration cannot cause spurious mana gains.
		hadPool := false
		p.DynamicDamageTakenModifiers = append([]core.DynamicDamageTakenModifier{func(sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			hadPool = shield.Aura.IsActive() && shield.Remaining > 0
		}}, p.DynamicDamageTakenModifiers...)
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Seal of Fury Depletion", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if hadPool && shield.Remaining == 0 && p.fr("improved-seal-of-fury") > 0 {
				p.AddMana(sim, 38*(1+min(.45, max(0, float64(sp.Unit.Level-p.Level))*.15)), metrics)
			}
			hadPool = false
		}}))
		a := p.RegisterAura(core.Aura{Label: "Forever Seal of Fury", ActionID: action, Duration: 30 * time.Second, OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if sp.ProcMask.Matches(core.ProcMaskMelee) && r.Landed() {
				shield.Apply(sim, 60, false)
			}
		}})
		judge := p.RegisterSpell(core.SpellConfig{ActionID: action.WithTag(-action.Tag), SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, DamageMultiplier: 1, ThreatMultiplier: 2, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			sp.CalcAndDealDamage(sim, t, 60, sp.OutcomeMagicHitAndCrit)
		}})
		p.RegisterSpell(core.SpellConfig{ActionID: action, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 60}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { p.applySeal(a, judge, sim) }})
	}
}
