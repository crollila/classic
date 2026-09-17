package core

import (
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (c *Character) foreverCooldown(id string, duration time.Duration, kind CooldownType, effect func(*Simulation, *Unit, *Spell)) *Spell {
	s := c.RegisterSpell(SpellConfig{ActionID: c.ForeverAction(id), Flags: SpellFlagAPL | SpellFlagHelpful, Cast: CastConfig{CD: Cooldown{Timer: c.NewTimer(), Duration: duration}}, ApplyEffects: effect})
	c.AddMajorCooldown(MajorCooldown{Spell: s, Type: kind})
	return s
}
func (c *Character) foreverRecovery(id string, percent float64, duration time.Duration, mana bool, corpse bool) {
	if corpse {
		c.foreverCorpseRecovery(id, percent, duration, mana)
		return
	}
	action := c.ForeverAction(id)
	hm := c.NewHealthMetrics(action)
	mm := c.NewManaMetrics(action)
	var end time.Duration
	interrupted := false
	aura := c.RegisterAura(Aura{Label: id, ActionID: action, Duration: duration, OnExpire: func(a *Aura, sim *Simulation) {
		if sim.CurrentTime < end {
			interrupted = true
		}
	}})
	s := c.foreverCooldown(id, 3*time.Minute, CooldownTypeSurvival, func(sim *Simulation, t *Unit, s *Spell) {
		interrupted = false
		end = sim.CurrentTime + duration
		aura.Activate(sim)
		StartPeriodicAction(sim, PeriodicActionOptions{Period: time.Second, NumTicks: int(duration / time.Second), OnAction: func(sim *Simulation) {
			// Aura expiry precedes pending actions at the same timestamp. Its final
			// tick still heals unless the channel was interrupted before that time.
			if interrupted {
				return
			}
			if corpse && c.IsMoving() {
				aura.Deactivate(sim)
				return
			}
			c.GainHealth(sim, c.MaxHealth()*percent/duration.Seconds(), hm)
			if mana && c.HasManaBar() {
				c.AddMana(sim, c.MaxMana()*percent/duration.Seconds(), mm)
			}
		}})
	})
	if corpse {
		// PREDICTED preparation scenario: a usable corpse must be explicitly supplied.
		s.ExtraCastCondition = func(sim *Simulation, t *Unit) bool {
			return c.ForeverParameter("scenario.corpse_available", 0) > 0 && !c.IsMoving()
		}
		aura.OnSpellHitTaken = func(a *Aura, sim *Simulation, s *Spell, r *SpellResult) {
			if r.Damage > 0 {
				a.Deactivate(sim)
			}
		}
	}
}
func (c *Character) applyExpandedForeverRacials() {
	if c.Forever == nil {
		return
	}
	if c.HasForeverMechanic("racials.orc.hardiness") {
		c.ForeverControlReduction(ForeverStun, .2)
	}
	for _, escape := range []struct {
		id       string
		kinds    []ForeverControlKind
		duration time.Duration
		cd       time.Duration
	}{
		{"racials.human.will-to-survive", []ForeverControlKind{ForeverStun}, time.Nanosecond, 2 * time.Minute},
		{"racials.undead.will-of-the-forsaken", []ForeverControlKind{ForeverFear, ForeverCharm, ForeverSleep}, time.Nanosecond, 2 * time.Minute},
		{"racials.gnome.escape-artist", []ForeverControlKind{ForeverRoot, ForeverSnare}, 5 * time.Second, time.Minute},
	} {
		if !c.HasForeverMechanic(escape.id) {
			continue
		}
		aura := c.ForeverControlImmunityAura(escape.id, c.ForeverAction(escape.id), escape.kinds, escape.duration)
		s := c.foreverCooldown(escape.id, escape.cd, CooldownTypeSurvival, func(sim *Simulation, t *Unit, s *Spell) { aura.Activate(sim) })
		s.ForeverIgnoreControl = true
		s.ExtraCastCondition = func(sim *Simulation, t *Unit) bool {
			for _, k := range escape.kinds {
				if c.ForeverControlled(k) {
					return true
				}
			}
			return false
		}
	}
	if c.HasForeverMechanic("racials.tauren.war-stomp") {
		id := "racials.tauren.war-stomp"
		auras := make([]*Aura, len(c.Env.Encounter.TargetUnits))
		for i, t := range c.Env.Encounter.TargetUnits {
			auras[i] = t.ForeverControlAura(id, c.ForeverAction(id), ForeverStun, 2*time.Second)
		}
		c.RegisterSpell(SpellConfig{ActionID: c.ForeverAction(id), Flags: SpellFlagAPL, SpellSchool: SpellSchoolPhysical, Cast: CastConfig{DefaultCast: Cast{CastTime: 500 * time.Millisecond}, CD: Cooldown{Timer: c.NewTimer(), Duration: 2 * time.Minute}}, ExtraCastCondition: func(sim *Simulation, t *Unit) bool { return c.DistanceFromTarget <= 8 }, ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) {
			for _, a := range auras {
				a.Activate(sim)
			}
		}})
	}
	if c.HasForeverMechanic("racials.night-elf.elune-s-light") {
		id := "racials.night-elf.elune-s-light"
		a := c.NewTemporaryStatsAura(id, c.ForeverAction(id), stats.Stats{stats.MeleeCrit: 10, stats.SpellCrit: 10}, 15*time.Second)
		c.foreverCooldown(id, 3*time.Minute, CooldownTypeDPS, func(sim *Simulation, t *Unit, s *Spell) { a.Activate(sim) })
	}
	if c.HasForeverMechanic("racials.gnome.eureka") {
		id := "racials.gnome.eureka"
		eligible := []*Spell{}
		c.OnSpellRegistered(func(s *Spell) {
			if s.ProcMask.Matches(ProcMaskMeleeSpecial|ProcMaskRangedSpecial|ProcMaskSpellDamage|ProcMaskSpellHealing) && s.DefaultCast.GCD > 0 {
				eligible = append(eligible, s)
			}
		})
		aura := c.RegisterAura(Aura{Label: id, ActionID: c.ForeverAction(id), Duration: 30 * time.Second, MaxStacks: 3,
			OnGain: func(a *Aura, sim *Simulation) {
				for _, s := range eligible {
					s.DamageMultiplier *= 1.1
					if s.Cost != nil {
						s.Cost.Multiplier -= 20
					}
				}
			},
			OnExpire: func(a *Aura, sim *Simulation) {
				for _, s := range eligible {
					s.DamageMultiplier /= 1.1
					if s.Cost != nil {
						s.Cost.Multiplier += 20
					}
				}
			},
			OnCastComplete: func(a *Aura, sim *Simulation, s *Spell) {
				for _, candidate := range eligible {
					if candidate == s {
						a.RemoveStack(sim)
						break
					}
				}
			},
		})
		c.foreverCooldown(id, 3*time.Minute, CooldownTypeDPS, func(sim *Simulation, t *Unit, s *Spell) { aura.Activate(sim); aura.SetStacks(sim, 3) })
	}
	if c.HasForeverMechanic("racials.undead.touch-of-the-grave") {
		id := "racials.undead.touch-of-the-grave"
		action := c.ForeverAction(id)
		metrics := c.NewHealthMetrics(action)
		icd := Cooldown{Timer: c.NewTimer(), Duration: 10 * time.Second}
		drain := c.RegisterSpell(SpellConfig{ActionID: action, SpellSchool: SpellSchoolShadow, DefenseType: DefenseTypeMagic, ProcMask: ProcMaskSpellProc, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) {
			r := s.CalcAndDealDamage(sim, t, c.MaxHealth()*.05, s.OutcomeMagicHit)
			c.GainHealth(sim, r.Damage, metrics)
		}})
		MakePermanent(c.RegisterAura(Aura{Label: id, OnSpellHitDealt: func(a *Aura, sim *Simulation, s *Spell, r *SpellResult) {
			if s != drain && r.Landed() && s.ProcMask.Matches(ProcMaskMeleeOrRanged|ProcMaskSpellDamage) && icd.IsReady(sim) && sim.Proc(.05, id) {
				icd.Use(sim)
				drain.Cast(sim, r.Target)
			}
		}}))
	}
	if c.HasForeverMechanic("racials.undead.cannibalize") {
		c.foreverRecovery("racials.undead.cannibalize", .35, 10*time.Second, true, true)
	}
	if c.HasForeverMechanic("racials.troll.rapid-regeneration") {
		c.foreverRecovery("racials.troll.rapid-regeneration", .5, 10*time.Second, false, false)
	}
	if c.HasForeverMechanic("racials.troll.regeneration") {
		id := "racials.troll.regeneration"
		m := c.NewHealthMetrics(c.ForeverAction(id))
		MakePermanent(c.RegisterAura(Aura{Label: id, OnReset: func(a *Aura, sim *Simulation) {
			a.Activate(sim)
			StartPeriodicAction(sim, PeriodicActionOptions{Period: 2 * time.Second, OnAction: func(sim *Simulation) { c.GainHealth(sim, .1*(c.GetStat(stats.Spirit)*.25+6), m) }})
		}}))
	}
	if c.HasForeverMechanic("racials.night-elf.quickness") {
		action := c.ForeverAction("racials.night-elf.quickness")
		MakePermanent(c.RegisterAura(Aura{Label: "Forever Quickness movement", ActionID: action,
			OnGain:   func(a *Aura, sim *Simulation) { c.AddMoveSpeedModifier(&action, 1.02) },
			OnExpire: func(a *Aura, sim *Simulation) { c.RemoveMoveSpeedModifier(&action) },
		}))
	}
	if c.HasForeverMechanic("racials.tauren.plainsrunning") {
		id := "racials.tauren.plainsrunning"
		action := c.ForeverAction(id)
		moving := 0
		MakePermanent(c.RegisterAura(Aura{Label: id, OnReset: func(a *Aura, sim *Simulation) {
			a.Activate(sim)
			moving = 0
			StartPeriodicAction(sim, PeriodicActionOptions{Period: time.Second, OnAction: func(sim *Simulation) {
				c.RemoveMoveSpeedModifier(&action)
				if c.IsMoving() {
					moving++
					c.AddMoveSpeedModifier(&action, 1+.05*float64(min(6, moving)))
				} else {
					moving = 0
				}
			}})
		}}))
		c.initForeverControl() // use travel integration that observes changing speed.
	}
	for _, prefix := range []string{"racials.skyborne-windshaper.", "racials.skyborne-high-order."} {
		if c.HasForeverMechanic(prefix + "wind-blessed") {
			c.PseudoStats.MeleeSpeedMultiplier *= 1.01
			c.PseudoStats.RangedSpeedMultiplier *= 1.01
			c.PseudoStats.CastSpeedMultiplier *= 1.01
		}
		if c.HasForeverMechanic(prefix + "elemental-insight") {
			c.foreverMobDamage(proto.MobType_MobTypeElemental, 1.05)
		}
	}
	if c.HasForeverMechanic("racials.skyborne-windshaper.skysight") {
		action := c.ForeverAction("racials.skyborne-windshaper.skysight")
		MakePermanent(c.RegisterAura(Aura{Label: "Forever Skysight movement", ActionID: action,
			OnGain:   func(a *Aura, sim *Simulation) { c.AddMoveSpeedModifier(&action, 1.1) },
			OnExpire: func(a *Aura, sim *Simulation) { c.RemoveMoveSpeedModifier(&action) },
		}))
	}
	if c.HasForeverMechanic("racials.skyborne-high-order.read-ley-line") {
		id := "racials.skyborne-high-order.read-ley-line"
		hm := c.NewHealthMetrics(c.ForeverAction(id))
		mm := c.NewManaMetrics(c.ForeverAction(id))
		a := c.RegisterAura(Aura{Label: id, Duration: 15 * time.Second, ActionID: c.ForeverAction(id)})
		c.foreverCooldown(id, 3*time.Minute, CooldownTypeMana, func(sim *Simulation, t *Unit, s *Spell) {
			a.Activate(sim)
			StartPeriodicAction(sim, PeriodicActionOptions{Period: 2 * time.Second, NumTicks: 7, OnAction: func(sim *Simulation) {
				if !a.IsActive() {
					return
				}
				c.GainHealth(sim, c.GetStat(stats.Spirit)*.25+6, hm)
				if c.HasManaBar() {
					rate := c.ManaRegenPerSecondWhileCasting()
					if sim.CurrentTime >= c.PseudoStats.FiveSecondRuleRefreshTime {
						rate = c.ManaRegenPerSecondWhileNotCasting()
					}
					c.AddMana(sim, rate*2, mm)
				}
			}})
		})
	}
	for _, d := range []struct {
		id     string
		kinds  []string
		school SpellSchool
	}{
		{"racials.orc.shatter-curse", []string{"curse", "bane"}, SpellSchoolHoly | SpellSchoolFire | SpellSchoolNature | SpellSchoolFrost | SpellSchoolShadow | SpellSchoolArcane},
		{"racials.dwarf.stoneform", []string{"bleed", "poison", "disease"}, SpellSchoolPhysical},
	} {
		if c.HasForeverMechanic(d.id) {
			a := c.ForeverDebuffImmunity(d.id, c.ForeverAction(d.id), d.kinds, 8*time.Second)
			c.DynamicDamageTakenModifiers = append(c.DynamicDamageTakenModifiers, func(sim *Simulation, s *Spell, r *SpellResult) {
				if a.IsActive() && s.SpellSchool.Matches(d.school) {
					r.Damage *= .9
				}
			})
			if d.id == "racials.dwarf.stoneform" {
				c.OnSpellRegistered(func(s *Spell) {
					if s.SpellID == 20594 {
						old := s.ApplyEffects
						s.ApplyEffects = func(sim *Simulation, t *Unit, s *Spell) { old(sim, t, s); a.Activate(sim) }
					}
				})
			} else {
				c.foreverCooldown(d.id, 3*time.Minute, CooldownTypeSurvival, func(sim *Simulation, t *Unit, s *Spell) { a.Activate(sim) })
			}
		}
	}
}
func (c *Character) foreverMobDamage(kind proto.MobType, multiplier float64) {
	c.Env.RegisterPostFinalizeEffect(func() {
		for _, t := range c.Env.Encounter.TargetUnits {
			if t.MobType == kind {
				for _, at := range c.AttackTables[t.UnitIndex] {
					at.DamageDealtMultiplier *= multiplier
				}
			}
		}
	})
}

// Cannibalize uses the engine's channel path, so casts, movement and incoming
// damage interrupt it, and its last tick is delivered by Dot cleanup ordering.
func (c *Character) foreverCorpseRecovery(id string, percent float64, duration time.Duration, mana bool) {
	action := c.ForeverAction(id)
	hm := c.NewHealthMetrics(action)
	mm := c.NewManaMetrics(action)
	var recovery *Spell
	recovery = c.RegisterSpell(SpellConfig{ActionID: action, Flags: SpellFlagAPL | SpellFlagHelpful | SpellFlagChanneled,
		Cast: CastConfig{DefaultCast: Cast{GCD: GCDDefault}, CD: Cooldown{Timer: c.NewTimer(), Duration: 3 * time.Minute}},
		ExtraCastCondition: func(sim *Simulation, t *Unit) bool {
			return c.ForeverParameter("scenario.corpse_available", 0) > 0 && !c.IsMoving()
		},
		Dot: DotConfig{SelfOnly: true, Aura: Aura{Label: id,
			OnSpellHitTaken: func(a *Aura, sim *Simulation, s *Spell, r *SpellResult) {
				if r.Damage > 0 {
					a.Deactivate(sim)
				}
			},
			OnPeriodicDamageTaken: func(a *Aura, sim *Simulation, s *Spell, r *SpellResult) {
				if r.Damage > 0 {
					a.Deactivate(sim)
				}
			},
			OnExpire: func(a *Aura, sim *Simulation) {
				if !c.foreverAutosSuppressed() && !c.IsMoving() {
					c.AutoAttacks.EnableAutoSwing(sim)
				}
			},
		}, NumberOfTicks: int32(duration / time.Second), TickLength: time.Second,
			OnTick: func(sim *Simulation, t *Unit, d *Dot) {
				c.GainHealth(sim, c.MaxHealth()*percent/duration.Seconds(), hm)
				if mana && c.HasManaBar() {
					c.AddMana(sim, c.MaxMana()*percent/duration.Seconds(), mm)
				}
			},
		},
		ApplyEffects: func(sim *Simulation, t *Unit, s *Spell) { c.AutoAttacks.CancelAutoSwing(sim); s.AOEDot().Apply(sim) },
	})
	c.AddMajorCooldown(MajorCooldown{Spell: recovery, Type: CooldownTypeSurvival})
}
