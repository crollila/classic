package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"math"
	"time"
)

func (w *Warlock) registerForeverDemonCommands() {
	val := func(id string, index int) float64 { return w.ForeverValue("warlock.talent."+id, index, 0) }
	// Pet commands and Hellfire keep their top-rank values but cannot be used before the
	// level their first rank is learned (Consume Shadows 18, Soothing Kiss 22, Suffering 24,
	// Seduction 26, Devour Magic and Hellfire 30, Lesser Invisibility 32, Spell Lock 36).
	// The observed effectiveness multipliers modify real pet commands. Undocumented
	// baseline command ranks use separate Classic analogue constants below.
	tormentDamage := w.Voidwalker.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 11775}, SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskEmpty, ThreatMultiplier: 1, FlatThreatBonus: 315 * (1 + val("improved-voidwalker", 0)/100)})
	sufferingDamage := w.Voidwalker.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 17752}, SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskEmpty, ThreatMultiplier: 1, FlatThreatBonus: 395 * (1 + val("improved-voidwalker", 0)/100)})
	torment := w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 11775}, SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskEmpty, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 5 * time.Second}}, ThreatMultiplier: 1, FlatThreatBonus: 315 * (1 + val("improved-voidwalker", 0)/100), ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Voidwalker }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		tormentDamage.CalcAndDealOutcome(sim, t, tormentDamage.OutcomeAlwaysHit)
	}})
	_ = torment
	w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 17752}, SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskEmpty, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 2 * time.Minute}}, ThreatMultiplier: 1, FlatThreatBonus: 395 * (1 + val("improved-voidwalker", 0)/100), ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Voidwalker && w.Level >= 24 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		for _, t := range sim.Encounter.TargetUnits {
			sufferingDamage.CalcAndDealOutcome(sim, t, sufferingDamage.OutcomeAlwaysHit)
		}
	}})
	shadows := w.Voidwalker.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: core.ActionID{SpellID: 17854}, SpellSchool: core.SpellSchoolShadow, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagHelpful, DamageMultiplier: 1 + val("improved-voidwalker", 0)/100, ThreatMultiplier: 0, Hot: core.DotConfig{SelfOnly: true, Aura: core.Aura{Label: "Forever Consume Shadows"}, NumberOfTicks: 5, TickLength: 2 * time.Second, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
		d.Spell.CalcAndDealHealing(sim, &w.Voidwalker.Unit, 216, d.Spell.OutcomeHealing)
	}}})
	w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 17854}, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Voidwalker && w.Level >= 18 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { shadows.SelfHot().Apply(sim) }})
	seduction := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Seduction-"+w.Label, core.ActionID{SpellID: 6358}, core.ForeverCharm, time.Duration(15*float64(time.Second)*(1+val("improved-sayaad", 0)/100)))
	})
	w.RegisterSpell(core.SpellConfig{ForeverSingleTargetHarmful: true, ActionID: core.ActionID{SpellID: 6358}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 1500 * time.Millisecond}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Succubus && w.Level >= 26 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { seduction.Get(t).Activate(sim) }})
	invis := w.Succubus.RegisterAura(core.Aura{Label: "Forever Lesser Invisibility", ActionID: core.ActionID{SpellID: 7870}, Duration: time.Duration(60 * float64(time.Second) * (1 + val("improved-sayaad", 0)/100)), OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if r.Damage > 0 {
			a.Deactivate(sim)
		}
	}})
	w.Succubus.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if invis.IsActive() {
			r.Damage = 0
		}
	})
	w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 7870}, Flags: core.SpellFlagHelpful | core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 10 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Succubus && w.Level >= 32 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { invis.Activate(sim) }})
	w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 11785}, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 10 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Succubus && w.Level >= 22 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		for _, ps := range w.Succubus.Spellbook {
			ps.SpellMetrics[t.UnitIndex].TotalThreat -= 165 * (1 + val("improved-sayaad", 0)/100) / float64(len(w.Succubus.Spellbook))
		}
	}})
	tainted := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Tainted Blood-" + w.Label, ActionID: w.ForeverAction("warlock.talent.improved-felhunter"), Duration: 10 * time.Second, MaxStacks: 5, OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			t.AddStatDynamic(sim, stats.AttackPower, -20*(1+val("improved-felhunter", 0)/100)*float64(new-old))
		}})
	})
	core.MakePermanent(w.Felhunter.RegisterAura(core.Aura{Label: "Forever Tainted Blood trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) {
			a := tainted.Get(s.Unit)
			a.Activate(sim)
			a.AddStack(sim)
		}
	}}))
	lock := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Spell Lock-"+w.Label, core.ActionID{SpellID: 19647}, core.ForeverSilence, 3*time.Second)
	})
	w.RegisterSpell(core.SpellConfig{ForeverSingleTargetHarmful: true, ActionID: core.ActionID{SpellID: 19647}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 30*time.Second - time.Duration(val("improved-felhunter", 1)*float64(time.Second))}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Felhunter && w.Level >= 36 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		t.ForeverInterruptSchool(sim, 8*time.Second)
		lock.Get(t).Activate(sim)
	}})
	devour := w.Felhunter.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: core.ActionID{SpellID: 19736}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagHelpful, ProcMask: core.ProcMaskSpellHealing, DamageMultiplier: 1 + val("improved-felhunter", 0)/100, ThreatMultiplier: 0})
	w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 19736}, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: w.NewTimer(), Duration: 8 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.ActivePet == w.Felhunter && w.Level >= 30 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		t.ForeverDispel(sim, "magic")
		devour.CalcAndDealHealing(sim, &w.Felhunter.Unit, 195, devour.OutcomeHealing)
	}})
	// Hellfire enables the explicitly named Pyroclasm channel interaction.
	hellfire := w.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 11684}, SpellSchool: core.SpellSchoolFire, ProcMask: core.ProcMaskSpellDamage, DefenseType: core.DefenseTypeMagic, Flags: WarlockFlagDestruction | core.SpellFlagChanneled | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 645}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return w.Level >= 30 }, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 1, Dot: core.DotConfig{IsAOE: true, Aura: core.Aura{Label: "Forever Hellfire-" + w.Label}, NumberOfTicks: 15, TickLength: time.Second, BonusCoefficient: .083, OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, roll bool) { d.Snapshot(t, 208, roll) }, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
		for _, t := range sim.Encounter.TargetUnits {
			d.CalcAndDealPeriodicSnapshotDamage(sim, t, d.OutcomeTick)
		}
		w.RemoveHealth(sim, min(208, w.CurrentHealth()))
	}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { s.AOEDot().Apply(sim) }})
	stuns := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Pyroclasm-"+w.Label, w.ForeverAction("warlock.talent.pyroclasm"), core.ForeverStun, 3*time.Second)
	})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Pyroclasm periodic", OnPeriodicDamageDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if w.ForeverRank("warlock.talent.pyroclasm") == 0 {
			return
		}
		ticks := int32(0)
		if s == hellfire {
			ticks = 15
		} else {
			for _, rain := range w.RainOfFire {
				if s == rain {
					ticks = rain.AOEDot().NumberOfTicks
				}
			}
		}
		if ticks > 0 && sim.Proc(1-math.Pow(1-val("pyroclasm", 0)/100, 1/float64(ticks)), "Forever Pyroclasm channel") {
			stuns.Get(r.Target).Activate(sim)
		}
	}}))
}
