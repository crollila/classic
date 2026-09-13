package druid

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

const (
	foreverHealingTouch int32 = 1001 + iota
	foreverRejuvenation
	foreverRegrowth
	foreverTranquility
	foreverSwiftmend
	foreverWildGrowth
	foreverMangle
	foreverMaul
	foreverSwipe
)

func (d *Druid) fr(n string) float64       { return float64(d.ForeverRank("druid.talent." + n)) }
func (d *Druid) fa(n string) core.ActionID { return d.ForeverAction("druid.talent." + n) }

func (d *Druid) applyForeverTalents() {
	if d.Forever == nil {
		return
	}
	d.MultiplyStat(stats.Intellect, 1+.02*d.fr("heart-of-the-wild"))
	d.AddStat(stats.AttackPower, .5*d.fr("predatory-strikes")*float64(d.Level))
	d.PseudoStats.DamageDealtMultiplier *= 1 + .01*d.fr("naturalist")
	d.AddStat(stats.MeleeHit, 2*d.fr("nature-s-reach"))
	d.AddStat(stats.SpellHit, 2*d.fr("nature-s-reach"))
	d.AddStat(stats.Dodge, d.fr("natural-reaction"))
	d.OnSpellRegistered(func(sp *core.Spell) {
		if sp.SpellSchool.Matches(core.SpellSchoolArcane | core.SpellSchoolNature) {
			sp.ThreatMultiplier *= 1 - .1*d.fr("subtlety")
			sp.PushbackReduction += d.fr("nature-s-focus") * .14
			if sp.DefenseType == core.DefenseTypeMagic {
				d.ForeverSpellRange(sp, 30*(1+.1*d.fr("nature-s-reach")))
			}
		}
		if sp.ProcMask.Matches(core.ProcMaskSpellHealing) {
			sp.DamageMultiplier *= 1 + .02*d.fr("gift-of-nature")
			old := sp.ExtraCastCondition
			sp.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
				return !d.InForm(Moonkin) && (old == nil || old(sim, t))
			}
		}
		if sp.ProcMask.Matches(core.ProcMaskMeleeMHSpecial) {
			sp.CritDamageBonus += d.fr("predatory-instincts") * .1
		}
		if sp.SpellCode == SpellCode_DruidClaw || sp.SpellCode == SpellCode_DruidRake || sp.SpellCode == SpellCode_DruidShred || sp.SpellCode == foreverMaul || sp.SpellCode == foreverSwipe {
			sp.DamageMultiplier *= 1 + .05*d.fr("savage-fury")
		}
		if sp.SpellCode == SpellCode_DruidShred && sp.Cost != nil {
			sp.Cost.FlatModifier -= int32(6 * d.fr("shredding-attacks"))
		}
		if sp.SpellCode == SpellCode_DruidStarfire {
			sp.DefaultCast.CastTime -= time.Duration(100*d.fr("improved-starfire")) * time.Millisecond
		}
		if sp.SpellID == 9846 || sp.SpellID == 9845 || sp.SpellID == 6793 || sp.SpellID == 5217 {
			if d.fr("king-of-the-jungle") > 0 {
				metrics := d.NewEnergyMetrics(d.fa("king-of-the-jungle"))
				old := sp.ApplyEffects
				sp.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
					old(sim, t, sp)
					d.AddEnergy(sim, 20*d.fr("king-of-the-jungle"), metrics)
				}
			}
		}
		if d.fr("genesis") > 0 || d.fr("nature-s-splendor") > 0 {
			for _, dot := range sp.Dots() {
				if dot == nil {
					continue
				}
				dot.DamageMultiplier *= 1 + .01*d.fr("genesis")
				if d.fr("nature-s-splendor") > 0 {
					extra := int32(0)
					switch sp.SpellCode {
					case SpellCode_DruidMoonfire, foreverRejuvenation:
						extra = 1
					case foreverRegrowth:
						extra = 2
					case SpellCode_DruidInsectSwarm:
						extra = 1
					}
					dot.NumberOfTicks += extra
					dot.OriginalNumberOfTicks += extra
				}
			}
		}
	})
	if d.fr("primal-fury") > 0 {
		rage := d.NewRageMetrics(d.fa("primal-fury"))
		cp := d.NewComboPointMetrics(d.fa("primal-fury"))
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Primal Fury", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if !r.DidCrit() || !sim.Proc(.5*d.fr("primal-fury"), "Forever Primal Fury") {
				return
			}
			if d.InForm(Bear) {
				d.AddRage(sim, 5, rage)
			}
			if d.InForm(Cat) && sp.Flags.Matches(SpellFlagBuilder) {
				d.AddComboPoints(sim, 1, r.Target, cp)
			}
		}}))
	}
	if d.fr("natural-reaction") > 0 {
		m := d.NewRageMetrics(d.fa("natural-reaction"))
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Natural Reaction", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Outcome.Matches(core.OutcomeDodge) && d.InForm(Bear) && sim.Proc(.2*d.fr("natural-reaction"), "Forever Natural Reaction") {
				d.AddRage(sim, 5, m)
			}
		}}))
	}
	if d.fr("rend-and-tear") > 0 {
		d.ForeverDamageMultiplier(func(sp *core.Spell, at *core.AttackTable) float64 {
			if sp.ProcMask.Matches(core.ProcMaskMeleeMHSpecial) && (d.AssumeBleedActive || at.Defender.ForeverHasDebuff("bleed")) {
				return 1 + .02*d.fr("rend-and-tear")
			}
			return 1
		})
	}
	if d.fr("eclipse") > 0 {
		var stars []*core.Spell
		d.OnSpellRegistered(func(sp *core.Spell) {
			if sp.SpellCode == SpellCode_DruidStarfire {
				stars = append(stars, sp)
			}
		})
		amount := time.Duration(d.ForeverValue("druid.talent.eclipse", 1, 0) * float64(time.Second))
		a := d.RegisterAura(core.Aura{Label: "Forever Eclipse", ActionID: d.fa("eclipse"), Duration: 15 * time.Second, MaxStacks: 4, OnGain: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range stars {
				sp.DefaultCast.CastTime -= amount
			}
		}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range stars {
				sp.DefaultCast.CastTime += amount
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellCode == SpellCode_DruidStarfire {
				a.RemoveStack(sim)
			}
		}})
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Eclipse Trigger", OnCastComplete: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellCode == SpellCode_DruidWrath {
				a.Activate(sim)
				a.SetStacks(sim, min(4, a.GetStacks()+2))
			}
		}}))
	}
	if d.fr("nature-s-grace") > 0 {
		speed := 1 + d.fr("nature-s-grace")*.1
		var spells []*core.Spell
		d.OnSpellRegistered(func(sp *core.Spell) {
			if !sp.ProcMask.Matches(core.ProcMaskMeleeOrRanged) && sp.DefaultCast.GCD > 0 {
				spells = append(spells, sp)
			}
		})
		deltas := map[*core.Spell]time.Duration{}
		a := d.RegisterAura(core.Aura{Label: "Forever Nature's Grace", ActionID: d.fa("nature-s-grace"), Duration: 3 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
			d.MultiplyCastSpeed(speed)
			for _, sp := range spells {
				delta := time.Duration(float64(sp.DefaultCast.GCD) * .01)
				deltas[sp] = delta
				sp.DefaultCast.GCD -= delta
			}
		}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
			d.MultiplyCastSpeed(1 / speed)
			for _, sp := range spells {
				sp.DefaultCast.GCD += deltas[sp]
			}
		}})
		proc := func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && (sp.DefenseType == core.DefenseTypeMagic || sp.ProcMask.Matches(core.ProcMaskSpellHealing)) {
				a.Activate(sim)
			}
		}
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Nature's Grace Trigger", OnSpellHitDealt: proc, OnHealDealt: proc}))
	}
	if d.fr("balance-of-nature") > 0 {
		var nature, arcane []*core.Spell
		d.OnSpellRegistered(func(sp *core.Spell) {
			if sp.DefenseType == core.DefenseTypeMagic {
				if sp.SpellSchool.Matches(core.SpellSchoolNature) {
					nature = append(nature, sp)
				}
				if sp.SpellSchool.Matches(core.SpellSchoolArcane) {
					arcane = append(arcane, sp)
				}
			}
		})
		makeAura := func(name string, list *[]*core.Spell, school core.SpellSchool) *core.Aura {
			return d.RegisterAura(core.Aura{Label: name, ActionID: d.fa("balance-of-nature"), Duration: 10 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
				for _, sp := range *list {
					sp.DamageMultiplier *= 1 + .01*d.fr("balance-of-nature")
				}
			}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
				for _, sp := range *list {
					sp.DamageMultiplier /= 1 + .01*d.fr("balance-of-nature")
				}
			}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
				if sp.SpellSchool.Matches(school) && sp.DefenseType == core.DefenseTypeMagic {
					a.Deactivate(sim)
				}
			}})
		}
		n := makeAura("Forever Balance Nature", &nature, core.SpellSchoolNature)
		a := makeAura("Forever Balance Arcane", &arcane, core.SpellSchoolArcane)
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Balance Trigger", OnCastComplete: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellSchool.Matches(core.SpellSchoolNature) {
				a.Activate(sim)
			}
			if sp.SpellSchool.Matches(core.SpellSchoolArcane) {
				n.Activate(sim)
			}
		}}))
	}
}

func (d *Druid) registerForeverSpells() {
	if d.Forever == nil {
		return
	}
	if d.fr("leader-of-the-pack") > 0 || d.fr("moonkin-form") > 0 {
		for _, agent := range d.Party.Players {
			c := agent.GetCharacter()
			a := c.GetOrRegisterAura(core.Aura{Label: "Forever Form Crit-" + d.Label, Duration: core.NeverExpires})
			a.NewExclusiveEffect("Forever Form Critical Strike", false, core.ExclusiveEffect{Priority: 3, OnGain: func(_ *core.ExclusiveEffect, sim *core.Simulation) {
				c.AddStatDynamic(sim, stats.MeleeCrit, 3)
				c.AddStatDynamic(sim, stats.SpellCrit, 3)
			}, OnExpire: func(_ *core.ExclusiveEffect, sim *core.Simulation) {
				c.AddStatDynamic(sim, stats.MeleeCrit, -3)
				c.AddStatDynamic(sim, stats.SpellCrit, -3)
			}})
			d.foreverPartyCrit = append(d.foreverPartyCrit, a)
		}
	}
	d.registerForeverBear()
	d.registerForeverHealing()
	d.registerForeverUtility()
	if d.fr("improved-starfire") > 0 {
		auras := d.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			return t.ForeverControlAura("Forever Starfire Stun-"+d.Label, d.fa("improved-starfire"), core.ForeverStun, 3*time.Second)
		})
		core.MakePermanent(d.RegisterAura(core.Aura{Label: "Forever Starfire Stun Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if sp.SpellCode == SpellCode_DruidStarfire && r.Landed() && sim.Proc(.03*d.fr("improved-starfire"), "Forever Starfire Stun") {
				auras.Get(r.Target).Activate(sim)
			}
		}}))
	}
}

func (d *Druid) foreverFormCrit(sim *core.Simulation) {
	active := (d.InForm(Moonkin) && d.fr("moonkin-form") > 0) || (d.InForm(Bear|Cat) && d.fr("leader-of-the-pack") > 0)
	for _, a := range d.foreverPartyCrit {
		if active {
			a.Activate(sim)
		} else {
			a.Deactivate(sim)
		}
	}
}
