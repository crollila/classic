package priest

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (p *Priest) registerForeverReactiveTalents() {
	val := func(id string, index int) float64 { return p.ForeverValue("priest.talent."+id, index, 0) }
	if p.ForeverRank("priest.talent.martyrdom") > 0 {
		resistance := p.ForeverInterruptResistanceAura("Forever Focused Casting interrupt resistance", p.ForeverAction("priest.talent.martyrdom"), 6*time.Second, val("martyrdom", 2)/100)
		focused := p.RegisterAura(core.Aura{Label: "Forever Focused Casting", ActionID: p.ForeverAction("priest.talent.martyrdom"), Duration: 6 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range p.Spellbook {
				s.PushbackReduction += 1
			}
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range p.Spellbook {
				s.PushbackReduction -= 1
			}
		}})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Martyrdom trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && s.ProcMask.Matches(core.ProcMaskMeleeOrRanged) && sim.Proc(val("martyrdom", 0)/100, "Forever Martyrdom") {
				focused.Activate(sim)
				resistance.Activate(sim)
			}
		}}))
	}
	if p.ForeverRank("priest.talent.blessed-recovery") > 0 {
		pending := 0.0
		recovery := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: p.ForeverAction("priest.talent.blessed-recovery"), SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagHelpful | core.SpellFlagPassiveSpell | core.SpellFlagIgnoreAttackerModifiers, ProcMask: core.ProcMaskSpellHealing, DamageMultiplier: 1, ThreatMultiplier: 0, Hot: core.DotConfig{SelfOnly: true, Aura: core.Aura{Label: "Forever Blessed Recovery"}, NumberOfTicks: 3, TickLength: 2 * time.Second, OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, roll bool) {
			d.SnapshotBaseDamage = pending / 3
			d.SnapshotAttackerMultiplier = 1
		}, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
			d.CalcAndDealPeriodicSnapshotHealing(sim, &p.Unit, d.OutcomeTick)
		}}})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Blessed Recovery trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if (r.DidCrit() && s.ProcMask.Matches(core.ProcMaskMeleeOrRanged)) || r.Damage > .3*p.MaxHealth() {
				d := recovery.SelfHot()
				pending = r.Damage * val("blessed-recovery", 1) / 100
				if d.IsActive() {
					pending += d.SnapshotBaseDamage * float64(d.MaxTicksRemaining())
				}
				d.ApplyOrReset(sim)
			}
		}}))
	}
	if p.ForeverRank("priest.talent.spirit-tap") > 0 {
		// The existing aura already implements Spirit and casting regeneration.
		onDamage := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if p.SpiritTapAura != nil && r.Damage > 0 && r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Target.Level >= p.Level-8 && sim.Proc(val("spirit-tap", 0)/100, "Forever Spirit Tap") {
				p.SpiritTapAura.Activate(sim)
			}
		}
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Spirit Tap trigger", OnSpellHitDealt: onDamage, OnPeriodicDamageDealt: onDamage}))
	}
	if p.ForeverRank("priest.talent.spirit-of-redemption") > 0 {
		used := false
		immobile := p.ForeverImmobileAura("Forever Redemption immobility", p.ForeverAction("priest.talent.spirit-of-redemption"), 15*time.Second)
		immunity := p.ForeverControlImmunityAura("Forever Redemption control immunity", p.ForeverAction("priest.talent.spirit-of-redemption"), []core.ForeverControlKind{core.ForeverRoot, core.ForeverStun, core.ForeverFear, core.ForeverSilence, core.ForeverCharm, core.ForeverSleep, core.ForeverIncapacitate}, 15*time.Second)
		spirit := p.RegisterAura(core.Aura{Label: "Forever Spirit of Redemption", ActionID: p.ForeverAction("priest.talent.spirit-of-redemption"), Duration: 15 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
			immobile.Activate(sim)
			immunity.Activate(sim)
			for _, s := range p.Spellbook {
				if s.Flags.Matches(core.SpellFlagHelpful) && s.Cost != nil {
					s.Cost.Multiplier -= 100
				}
			}
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range p.Spellbook {
				if s.Flags.Matches(core.SpellFlagHelpful) && s.Cost != nil {
					s.Cost.Multiplier += 100
				}
			}
			immobile.Deactivate(sim)
			immunity.Deactivate(sim)
			p.RemoveHealth(sim, p.CurrentHealth())
		}})
		p.Env.RegisterPreFinalizeEffect(func() {
			for _, u := range p.Env.AllUnits {
				for _, s := range u.Spellbook {
					old := s.ExtraCastCondition
					s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
						return (t != &p.Unit || !spirit.IsActive()) && (old == nil || old(sim, t))
					}
				}
			}
		})
		p.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if spirit.IsActive() {
				r.Damage = 0
				return
			}
			if !used && r.Damage >= p.CurrentHealth() {
				used = true
				r.Damage = max(0, p.CurrentHealth()-1)
				spirit.Activate(sim)
			}
		})
		p.OnSpellRegistered(func(s *core.Spell) {
			old := s.ExtraCastCondition
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
				return (!used || p.CurrentHealth() > 0) && (!spirit.IsActive() || s.Flags.Matches(core.SpellFlagHelpful)) && (old == nil || old(sim, t))
			}
		})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Spirit of Redemption state", OnReset: func(a *core.Aura, sim *core.Simulation) { used = false }}))
	}
}
func (p *Priest) registerForeverBaseline() {
	val := func(id string, index int) float64 { return p.ForeverValue("priest.talent."+id, index, 0) }
	for _, t := range p.Env.Encounter.Targets {
		t.ForeverEnableManaPool(max(t.GetStat(stats.Mana), p.ForeverParameter("scenario.enemy_mana", 0)))
	}
	// Baseline spells below cannot be used before their first rank is learned: Shadow Word:
	// Death 32, Psychic Scream 14, Fade 8, Mana Burn 24, Inner Fire 12. Shadow Word: Death
	// uses the rank the level has learned (client tooltips); the others stay the top rank's.
	if swd, ok := foreverPriestRankAt(p.Level, foreverShadowWordDeathRanks); ok && p.HasForeverMechanic("priest.baseline.shadow-word-death") {
		p.RegisterSpell(core.SpellConfig{ActionID: p.ForeverAction("priest.baseline.shadow-word-death"), SpellCode: SpellCode_PriestShadowWordDeath, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: swd.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 15 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .429,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				crit := 0.0
				if sim.IsExecutePhase20() || (t.HasHealthBar() && t.CurrentHealthPercent() <= .2) {
					crit = val("early-demise", 1) * core.SpellCritRatingPerCritChance
				}
				s.BonusCritRating += crit
				s.CalcAndDealDamage(sim, t, sim.Roll(swd.low, swd.high), s.OutcomeMagicHitAndCrit)
				s.BonusCritRating -= crit
				// "If your target is not killed ... backlash damage equal to 10% of your
				// maximum health."
				if !t.HasHealthBar() || t.CurrentHealth() > 0 {
					p.RemoveHealth(sim, min(.1*p.MaxHealth(), p.CurrentHealth()))
				}
			},
		})
	}
	if p.ForeverRank("priest.talent.vampiric-embrace") > 0 {
		p.VampiricEmbrace = p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 15286}, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 40}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r := s.CalcAndDealOutcome(sim, t, s.OutcomeMagicHit)
			if r.Landed() {
				p.foreverState.embrace.Get(t).Activate(sim)
			}
		}})
	}
	if p.ForeverRank("priest.talent.silence") > 0 {
		silence := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Silence-"+p.Label, p.ForeverAction("priest.talent.silence"), core.ForeverSilence, 5*time.Second)
		})
		p.RegisterSpell(core.SpellConfig{ForeverSingleTargetHarmful: true, ProcMask: core.ProcMaskEmpty, ActionID: p.ForeverAction("priest.talent.silence"), SpellSchool: core.SpellSchoolShadow, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 225}, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 45 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return p.DistanceFromTarget <= 20*(1+val("shadow-reach", 0)/100)
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			t.ForeverInterruptSchool(sim, 3*time.Second)
			silence.Get(t).Activate(sim)
		}})
	}
	fears := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Psychic Scream-"+p.Label, core.ActionID{SpellID: 10890}, core.ForeverFear, 8*time.Second)
	})
	p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 10890}, SpellSchool: core.SpellSchoolShadow, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 210}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 30*time.Second - time.Duration(val("improved-psychic-scream", 0)*float64(time.Second))}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return p.Level >= 14 && p.DistanceFromTarget <= 8 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		for i, t := range sim.Encounter.TargetUnits {
			if i == 5 {
				break
			}
			fears.Get(t).Activate(sim)
		}
	}})
	fade := p.RegisterAura(core.Aura{Label: "Forever Fade", ActionID: core.ActionID{SpellID: 10942}, Duration: 10 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range p.Spellbook {
			for i := range s.SpellMetrics {
				s.SpellMetrics[i].TotalThreat -= 820 / float64(len(p.Spellbook))
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range p.Spellbook {
			for i := range s.SpellMetrics {
				s.SpellMetrics[i].TotalThreat += 820 / float64(len(p.Spellbook))
			}
		}
	}})
	p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 10942}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 250}, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 30*time.Second - time.Duration(val("improved-fade", 0)*float64(time.Second))}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return p.Level >= 8 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { fade.Activate(sim) }})
	burnMetrics := map[int32]*core.ResourceMetrics{}
	for _, target := range p.Env.Encounter.Targets {
		if target.HasManaBar() {
			burnMetrics[target.UnitIndex] = target.NewManaMetrics(core.ActionID{SpellID: 10876})
		}
	}
	p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 10876}, SpellSchool: core.SpellSchoolShadow, DefenseType: core.DefenseTypeMagic, Flags: SpellFlagPriest | core.SpellFlagAPL, ProcMask: core.ProcMaskSpellDamage, ManaCost: core.ManaCostOptions{FlatCost: 165}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 3*time.Second - time.Duration(val("improved-mana-burn", 0)*float64(time.Second))}}, DamageMultiplier: 1, ThreatMultiplier: 1, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return p.Level >= 24 && t.HasManaBar() }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		amount := min(t.CurrentMana(), 600.0)
		result := s.CalcDamage(sim, t, amount*.5, s.OutcomeMagicHitAndCrit)
		if result.Landed() {
			t.SpendMana(sim, amount, burnMetrics[t.UnitIndex])
		}
		s.DealDamage(sim, result)
	}})
	// Inner Fire's baseline armor and finite charges now participate in tank tests.
	armor := 1395 * (1 + val("improved-inner-fire", 0)/100)
	inner := p.RegisterAura(core.Aura{Label: "Forever Inner Fire", ActionID: core.ActionID{SpellID: 10952}, Duration: 10 * time.Minute, MaxStacks: 20 + int32(val("improved-inner-fire", 1)), OnGain: func(a *core.Aura, sim *core.Simulation) { p.AddStatDynamic(sim, stats.Armor, armor) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { p.AddStatDynamic(sim, stats.Armor, -armor) }, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if r.Landed() && s.ProcMask.Matches(core.ProcMaskMeleeOrRanged) {
			a.RemoveStack(sim)
		}
	}})
	p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 10952}, SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 315}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return p.Level >= 12 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		inner.Activate(sim)
		inner.SetStacks(sim, inner.MaxStacks)
	}})
	// Devouring Contagion also triggers when another attacker kills the afflicted target.
	if p.ForeverRank("priest.talent.devouring-contagion") > 0 {
		p.OnSpellRegistered(func(spell *core.Spell) {
			if spell.SpellCode != SpellCode_PriestDevouringPlague {
				return
			}
			for _, d := range spell.Dots() {
				if d == nil {
					continue
				}
				d := d
				kill := func(a *core.Aura, sim *core.Simulation, damage *core.Spell, r *core.SpellResult) {
					if !r.Target.HasHealthBar() || r.Target.CurrentHealth() > 0 || r.Damage <= 0 {
						return
					}
					for _, next := range sim.Encounter.TargetUnits {
						if next == r.Target || next.ForeverEnemyDead() || !next.IsActive() {
							continue
						}
						copy := spell.Dot(next)
						copy.NumberOfTicks = d.MaxTicksRemaining()
						if copy.NumberOfTicks > 0 {
							copy.Apply(sim)
							copy.SnapshotBaseDamage = d.SnapshotBaseDamage
							copy.SnapshotAttackerMultiplier = d.SnapshotAttackerMultiplier
							copy.SnapshotCritChance = d.SnapshotCritChance
						}
						d.Deactivate(sim)
						break
					}
				}
				d.OnSpellHitTaken = kill
				d.OnPeriodicDamageTaken = kill
			}
		})
	}
}
