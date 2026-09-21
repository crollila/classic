package warlock

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

type foreverWarlockState struct {
	sacrificed                          *WarlockPet
	execute20                           bool
	banes                               core.AuraArray
	havoc                               *core.Unit
	havocAuras, hope, shadowBolt, brand core.AuraArray
	decimation, shadowFlame, fireFlame  *core.Aura
}

// foreverDotOutcome lets Forever damage over time crit at its snapshot crit chance with or
// without Pandemic: the client's Pandemic "increases the critical strike damage bonus" of
// DoTs, which implies they already crit (ElliotWood/Forever crits every DoT). Needs
// in-game confirmation. Classic ticks never crit.
func (w *Warlock) foreverDotOutcome(dot *core.Dot) core.OutcomeApplier {
	if w.Forever != nil {
		return dot.OutcomeSnapshotCrit
	}
	return dot.OutcomeTick
}
func (w *Warlock) applyForeverCasterTalents() {
	if w.Forever == nil {
		return
	}
	f := &foreverWarlockState{}
	w.foreverState = f
	val := func(id string, index int) float64 { return w.ForeverValue("warlock.talent."+id, index, 0) }
	rank := func(id string) int32 { return w.ForeverRank("warlock.talent." + id) }
	f.banes = make(core.AuraArray, len(w.Env.AllUnits))
	f.hope = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Drain Hope-" + w.Label, ActionID: w.ForeverAction("warlock.talent.drain-hope"), Duration: 6 * time.Second})
	})
	f.shadowBolt = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Improved Shadow Bolt-" + w.Label, ActionID: w.ForeverAction("warlock.talent.improved-shadow-bolt"), Duration: 12 * time.Second})
	})
	f.havocAuras = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Bane of Havoc-" + w.Label, Tag: "forever-debuff-bane", ActionID: w.ForeverAction("warlock.talent.bane-of-havoc"), Duration: 5 * time.Minute})
	})
	f.brand = w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Demonic Brand-" + w.Label, ActionID: w.ForeverAction("warlock.talent.demonic-brand"), Duration: 10 * time.Second, MaxStacks: 6})
	})
	f.decimation = w.RegisterAura(core.Aura{Label: "Forever Decimation", ActionID: w.ForeverAction("warlock.talent.decimation"), Duration: 10 * time.Second,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range w.SoulFire {
				s.CastTimeMultiplier -= val("decimation", 4) / 100
			}
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range w.SoulFire {
				s.CastTimeMultiplier += val("decimation", 4) / 100
			}
		},
	})
	f.shadowFlame = w.RegisterAura(core.Aura{Label: "Forever Shadow and Flame Shadow", ActionID: w.ForeverAction("warlock.talent.shadow-and-flame"), Duration: 20 * time.Second,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + val("shadow-and-flame", 0)/100
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1 + val("shadow-and-flame", 0)/100
		},
	})
	f.fireFlame = w.RegisterAura(core.Aura{Label: "Forever Shadow and Flame Fire", ActionID: w.ForeverAction("warlock.talent.shadow-and-flame"), Duration: 20 * time.Second,
		OnGain: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] *= 1 + val("shadow-and-flame", 0)/100
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			w.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexFire] /= 1 + val("shadow-and-flame", 0)/100
		},
	})
	w.ForeverDamageMultiplier(func(s *core.Spell, at *core.AttackTable) float64 {
		mult := 1.0
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && f.shadowBolt.Get(at.Defender).IsActive() {
			mult *= 1 + val("improved-shadow-bolt", 0)/100
		}
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && s.SpellCode != SpellCode_WarlockDrainHope && len(s.Dots()) > 0 && f.hope.Get(at.Defender).IsActive() {
			mult *= 1.1
		}
		if isForeverDrain(s) {
			mult *= w.foreverDrainMultiplier(s, at.Defender)
		}
		return mult
	})
	w.OnSpellRegistered(func(s *core.Spell) {
		if !s.Flags.Matches(SpellFlagWarlock) {
			return
		}
		if s.DefenseType == core.DefenseTypeMagic {
			// The client's Improved Drains no longer extends Drain Life's range.
			w.ForeverSpellRange(s, 30*(1+val("destructive-reach", 0)/100))
		}
		if s.Flags.Matches(WarlockFlagDestruction) {
			if s.Cost != nil {
				s.Cost.Multiplier -= int32(val("cataclysm", 0))
			}
			s.CritDamageBonus += val("ruin", 0) / 100
			s.DamageMultiplierAdditive += val("agonizing-flames", 1) / 100
			s.PushbackReduction += val("intensity", 0) / 100
		}
		if s.SpellCode == SpellCode_WarlockCorruption {
			s.DefaultCast.CastTime -= time.Duration(val("improved-corruption", 0) * float64(time.Second))
			s.DamageMultiplierAdditive += val("improved-corruption", 1) / 100
		}
		if s.SpellCode == SpellCode_WarlockCurseOfAgony {
			s.DamageMultiplierAdditive += val("improved-bane-of-agony", 0) / 100
		}
		if s.SpellCode == SpellCode_WarlockSearingPain {
			s.BonusCritRating += val("agonizing-flames", 0) * core.SpellCritRatingPerCritChance
			// Client: 17/33/50% less threat (the demo record reads 17/34/51).
			s.ThreatMultiplier *= 1 - [4]float64{0, .17, .33, .50}[min(3, rank("demonic-brand"))]
		}
		if s.SpellCode == SpellCode_WarlockConflagrate {
			s.BonusCritRating += val("fire-and-brimstone", 0) * core.SpellCritRatingPerCritChance
		}
		if s.SpellCode == SpellCode_WarlockShadowBolt || s.SpellCode == SpellCode_WarlockImmolate || s.SpellCode == SpellCode_WarlockIncinerate {
			s.DefaultCast.CastTime -= time.Duration(val("bane", 0) * float64(time.Second))
		}
		if s.SpellCode == SpellCode_WarlockSoulFire {
			s.DefaultCast.CastTime -= time.Duration(val("bane", 1) * float64(time.Second))
			s.CD.Duration = time.Duration(float64(s.CD.Duration) * (1 - val("decimation", 0)/100))
		}
		if isForeverDrain(s) {
			s.PushbackReduction += val("fel-concentration", 0) / 100
		}
		if d := s.AOEDot(); d != nil {
			d.DamageMultiplier *= 1 + val("malediction", 0)/100
		}
		for _, d := range s.Dots() {
			if d == nil {
				continue
			}
			d.DamageMultiplier *= 1 + val("malediction", 0)/100
		}
		if s.SpellSchool.Matches(core.SpellSchoolShadow) && len(s.Dots()) > 0 {
			s.CritDamageBonus += val("pandemic", 0) / 100
		}
		if s.SpellCode == SpellCode_WarlockCurseOfAgony || s.SpellCode == SpellCode_WarlockCurseOfDoom {
			for _, d := range s.Dots() {
				if d != nil {
					d.Tag = "forever-debuff-bane"
				}
			}
			old := s.ApplyEffects
			s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				oldCurse := w.ActiveCurseAura[t.UnitIndex]
				w.ActiveCurseAura[t.UnitIndex] = f.banes[t.UnitIndex]
				old(sim, t, sp)
				f.banes[t.UnitIndex] = w.ActiveCurseAura[t.UnitIndex]
				w.ActiveCurseAura[t.UnitIndex] = oldCurse
			}
		}
		old := s.ApplyEffects
		s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			execute := (sp.SpellCode == SpellCode_WarlockSearingPain || sp.SpellCode == SpellCode_WarlockShadowBolt) && rank("decimation") > 0 && (sim.IsExecutePhase35() || (t.HasHealthBar() && t.CurrentHealthPercent() < .35))
			mult := 1.0
			if execute {
				mult += val("decimation", 2) / 100
				f.decimation.Activate(sim)
			}
			sp.DamageMultiplier *= mult
			old(sim, t, sp)
			sp.DamageMultiplier /= mult
		}
	})
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Warlock triggers", OnReset: func(a *core.Aura, sim *core.Simulation) {
		f.havoc = nil
		f.sacrificed = nil
		f.execute20 = false
		sim.RegisterExecutePhaseCallback(func(sim *core.Simulation, phase int32) {
			if phase == 20 {
				f.execute20 = true
			}
		})
		for i := range f.banes {
			f.banes[i] = nil
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() {
			return
		}
		if s.SpellCode == SpellCode_WarlockShadowBolt && r.DidCrit() && rank("improved-shadow-bolt") > 0 {
			f.shadowBolt.Get(r.Target).Activate(sim)
		}
		if s.SpellCode == SpellCode_WarlockSearingPain && rank("demonic-brand") > 0 {
			a := f.brand.Get(r.Target)
			a.Activate(sim)
			// Client: the brand arms 2/4/6 pet attacks (the demo record reads 2 at every rank).
			a.SetStacks(sim, 2*min(3, rank("demonic-brand")))
		}
		if rank("shadow-and-flame") > 0 {
			if s.SpellCode == SpellCode_WarlockConflagrate {
				f.shadowFlame.Activate(sim)
			}
			if s.SpellCode == SpellCode_WarlockShadowburn {
				f.fireFlame.Activate(sim)
			}
		}
	}}))
	w.applyForeverPetTalents()
}
func (w *Warlock) registerForeverSpells() {
	if w.Forever == nil {
		return
	}
	f := w.foreverState
	if w.ForeverRank("warlock.talent.incinerate") > 0 {
		// The talent keeps its Forever action at every rank; the rank the level has learned
		// (client tooltips) supplies damage and mana.
		inc := foreverIncinerateRanks[0]
		for _, r := range foreverIncinerateRanks {
			if r.level <= w.Level {
				inc = r
			}
		}
		w.RegisterSpell(core.SpellConfig{ActionID: w.ForeverAction("warlock.talent.incinerate"), SpellCode: SpellCode_WarlockIncinerate, SpellSchool: core.SpellSchoolFire, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: WarlockFlagDestruction | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: inc.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 2500 * time.Millisecond}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 2.5 / 3.5,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				mult := 1.0
				if w.getActiveImmolateSpell(t) != nil {
					mult = 1.25
				}
				s.DamageMultiplier *= mult
				s.CalcAndDealDamage(sim, t, sim.Roll(inc.min, inc.max), s.OutcomeMagicHitAndCrit)
				s.DamageMultiplier /= mult
			},
		})
	}
	if w.ForeverRank("warlock.talent.drain-hope") > 0 {
		w.RegisterSpell(core.SpellConfig{ActionID: w.ForeverAction("warlock.talent.drain-hope"), SpellCode: SpellCode_WarlockDrainHope, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: WarlockFlagAffliction | core.SpellFlagAPL | core.SpellFlagChanneled, ManaCost: core.ManaCostOptions{FlatCost: 200}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 1,
			// Wrack (1316697) in the beta client: 36 Shadow a second for 6 sec at 14.3% a tick,
			// 200 mana. The talent record still carries the demo's "Drain Hope" 52 and 10%.
			Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Drain Hope damage-" + w.Label}, NumberOfTicks: 6, TickLength: time.Second, BonusCoefficient: .143, OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, roll bool) { d.Snapshot(t, 36, roll) }, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				d.CalcAndDealPeriodicSnapshotDamage(sim, t, w.foreverDotOutcome(d))
			}},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				r := s.CalcAndDealOutcome(sim, t, s.OutcomeMagicHit)
				if r.Landed() {
					s.Dot(t).Apply(sim)
					f.hope.Get(t).Activate(sim)
				}
			},
		})
	}
	if w.ForeverRank("warlock.talent.bane-of-havoc") > 0 {
		action := w.ForeverAction("warlock.talent.bane-of-havoc")
		copy := w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action.WithTag(-action.Tag), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagPassiveSpell | core.SpellFlagIgnoreAttackerModifiers | core.SpellFlagIgnoreTargetModifiers, DamageMultiplier: 1, ThreatMultiplier: 1})
		trigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.IsOpponent(r.Target) && r.Damage > 0 && s != copy && f.havoc != nil && r.Target != f.havoc && f.havocAuras.Get(f.havoc).IsActive() {
				copy.CalcAndDealDamage(sim, f.havoc, r.Damage*.15, copy.OutcomeAlwaysHit)
			}
		}
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Bane of Havoc trigger", OnSpellHitDealt: trigger, OnPeriodicDamageDealt: trigger}))
		w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: core.SpellSchoolShadow, Flags: WarlockFlagDestruction | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 43}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if f.havoc != nil {
				f.havocAuras.Get(f.havoc).Deactivate(sim)
			}
			if old := f.banes.Get(t); old != nil {
				old.Deactivate(sim)
			}
			f.havoc = t
			f.banes[t.UnitIndex] = f.havocAuras.Get(t)
			f.banes.Get(t).Activate(sim)
		}})
	}
	if w.ForeverRank("warlock.talent.curse-of-exhaustion") > 0 {
		auras := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverSnareAura("Forever Curse of Exhaustion-"+w.Label, w.ForeverAction("warlock.talent.curse-of-exhaustion"), 12*time.Second, .3)
		})
		amplified := w.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverSnareAura("Forever Amplified Exhaustion-"+w.Label, w.ForeverAction("warlock.talent.curse-of-exhaustion"), 12*time.Second, .5)
		})
		w.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: w.ForeverAction("warlock.talent.curse-of-exhaustion"), SpellSchool: core.SpellSchoolShadow, Flags: WarlockFlagAffliction | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 69}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if old := w.ActiveCurseAura.Get(t); old != nil {
				old.Deactivate(sim)
			}
			a := auras.Get(t)
			if w.AmplifyCurseAura != nil && w.AmplifyCurseAura.IsActive() {
				w.AmplifyCurseAura.Deactivate(sim)
				a = amplified.Get(t)
			}
			w.ActiveCurseAura[t.UnitIndex] = a
			a.Activate(sim)
		}})
	}
	// Health Funnel is a channeled health transfer. The Classic 10-second channel
	// and 50-per-second transfer are explicit best-guess baseline parameters.
	w.registerForeverMissingAffliction()
	w.registerForeverPetUtilities()
	w.registerForeverSoulShards()
}

// foreverIncinerateRanks: 412758 (40), 1293812 (50), 1293813 (60) from the beta client.
var foreverIncinerateRanks = []struct {
	level          int32
	mana, min, max float64
}{{40, 205, 99, 113}, {50, 265, 145, 167}, {60, 325, 201, 233}}

// isForeverDrain is Drain Life, Drain Soul or Wrack, the spells Improved Drains, Soul
// Siphon, Fel Concentration and Nightfall name in the Forever client.
func isForeverDrain(s *core.Spell) bool {
	return s.SpellCode == SpellCode_WarlockDrainLife || s.SpellCode == SpellCode_WarlockDrainSoul || s.SpellCode == SpellCode_WarlockDrainHope
}

// foreverImprovedDrains is the client's flat Improved Drains bonus per rank.
var foreverImprovedDrains = [4]float64{0, .07, .13, .20}

// foreverDrainMultiplier is Improved Drains (flat 7/13/20%) times Soul Siphon (4/8/12% for
// each of the warlock's other Affliction effects on the target, up to three effects, so
// 12/24/36%). The two talents are separate multipliers, as ElliotWood/Forever has them.
func (w *Warlock) foreverDrainMultiplier(s *core.Spell, target *core.Unit) float64 {
	mult := 1 + foreverImprovedDrains[min(3, w.ForeverRank("warlock.talent.improved-drains"))]
	if w.ForeverRank("warlock.talent.soul-siphon") == 0 {
		return mult
	}
	count := 0
	for _, other := range w.Spellbook {
		if other != s && other.Flags.Matches(WarlockFlagAffliction) && len(other.Dots()) > 0 {
			if d := other.Dot(target); d != nil && d.IsActive() {
				count++
			}
		}
	}
	if curse := w.ActiveCurseAura.Get(target); curse != nil && curse.IsActive() {
		count++
	}
	perEffect := w.ForeverValue("warlock.talent.soul-siphon", 0, 0) / 100
	limit := w.ForeverValue("warlock.talent.soul-siphon", 1, 0) / 100
	return mult * (1 + min(float64(count)*perEffect, limit))
}
