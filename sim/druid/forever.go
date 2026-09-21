package druid

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
	"math"
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
	d.PseudoStats.DamageDealtMultiplier *= 1 + .01*d.fr("naturalist")
	d.AddStat(stats.MeleeHit, 2*d.fr("nature-s-reach"))
	d.AddStat(stats.SpellHit, 2*d.fr("nature-s-reach"))
	d.AddStat(stats.Dodge, d.fr("natural-reaction")+2*d.fr("feral-swiftness"))
	d.OnSpellRegistered(func(sp *core.Spell) {
		switch sp.SpellCode {
		case SpellCode_DruidWrath, SpellCode_DruidStarfire, SpellCode_DruidMoonfire, SpellCode_DruidInsectSwarm, SpellCode_DruidFaerieFire, SpellCode_DruidFaerieFireFeral:
			sp.ForeverSingleTargetHarmful = true
		}
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
			dots := append([]*core.Dot{}, sp.Dots()...)
			if sp.AOEDot() != nil {
				dots = append(dots, sp.AOEDot())
			}
			for _, dot := range dots {
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
			if r.Outcome.Matches(core.OutcomeDodge) && sim.Proc(.2*d.fr("natural-reaction"), "Forever Natural Reaction") {
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
		// "increasing your spellcasting speed and reducing your global cooldown by 10%".
		fraction := d.ForeverValue("druid.talent.nature-s-grace", 0, 10) / 100
		speed := 1 + fraction
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
				delta := time.Duration(float64(sp.DefaultCast.GCD) * fraction)
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
	// Balance of Nature is selectable in trees.json but is not in the Forever client's
	// talent list (talentsforever.com export of the beta client), so a rank does nothing.
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
	d.RegisterAura(core.Aura{Label: "Forever Form Aura Range", OnReset: func(_ *core.Aura, sim *core.Simulation) {
		core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 200 * time.Millisecond, OnAction: d.foreverFormCrit})
	}})
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
		if active && math.Abs(d.DistanceFromTarget-a.Unit.DistanceFromTarget) <= 45 {
			a.Activate(sim)
		} else {
			a.Deactivate(sim)
		}
	}
}

// foreverPredatoryStrikesAP is Predatory Strikes' attack power, "in Cat Form, Bear Form,
// and Dire Bear Form" only: 50/100/150% of the druid's level.
func (d *Druid) foreverPredatoryStrikesAP() float64 {
	return d.ForeverValue("druid.talent.predatory-strikes", 0, 0) / 100 * float64(d.Level)
}

// foreverMangleBonus is the flat bonus of the Bear Mangle rank known at the level
// (client ranks: 26 at 25, 38 at 36, 59 at 48, 77 at 60). The talent grants rank 1.
func foreverMangleBonus(level int32) float64 {
	bonus := 26.0
	for _, r := range []struct {
		level int32
		bonus float64
	}{{36, 38}, {48, 59}, {60, 77}} {
		if r.level <= level {
			bonus = r.bonus
		}
	}
	return bonus
}

// foreverFurorEnergy is the Energy Furor restores on entering Cat Form: 20/40/60/80/100%
// of the Energy held when last in Cat Form plus 2/4/6/8/10 per second spent outside
// Bear, Dire Bear and Cat Form, capped at 20/40/60/80/100.
func (d *Druid) foreverFurorEnergy(outside time.Duration) float64 {
	retained := d.ForeverValue("druid.talent.furor", 2, 0) / 100
	perSecond := d.ForeverValue("druid.talent.furor", 3, 0)
	limit := d.ForeverValue("druid.talent.furor", 4, 0)
	return min(limit, d.foreverLastCatEnergy*retained+perSecond*outside.Seconds())
}
