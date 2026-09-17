package druid

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (d *Druid) registerForeverBear() {
	id := core.ActionID{SpellID: 9634}
	stam := d.NewDynamicMultiplyStat(stats.Stamina, 1.25*(1+.04*d.fr("heart-of-the-wild")))
	base := stats.Stats{stats.AttackPower: 3 * float64(d.Level), stats.MeleeCrit: 3 * d.fr("sharpened-claws")}
	armor := 0.0
	d.BearFormAura = d.RegisterAura(core.Aura{Label: "Forever Dire Bear Form", ActionID: id, Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) {
		d.CancelShapeshift(sim)
		d.form = Bear
		d.foreverFormCrit(sim)
		d.SetCurrentPowerBar(core.RageBar)
		d.SetShapeshift(a)
		d.foreverShiftDispel(sim)
		d.EnableDynamicStatDep(sim, stam)
		d.AddStatsDynamic(sim, base)
		d.ApplyDynamicEquipScaling(sim, stats.Armor, 4.6)
		if d.fr("thick-hide") > 0 {
			armor = float64(d.Level) + d.ForeverValue("druid.talent.thick-hide", 1, .67)*max(0, d.GetStat(stats.Defense))
			d.AddStatDynamic(sim, stats.Armor, armor*4.6)
		}
		d.PseudoStats.ThreatMultiplier *= 1.3
		d.AutoAttacks.SetMH(core.Weapon{BaseDamageMin: 109, BaseDamageMax: 165, SwingSpeed: 2.5, NormalizedSwingSpeed: 2.5, AttackPowerPerDPS: core.DefaultAttackPowerPerDPS})
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		d.form = Humanoid
		d.foreverFormCrit(sim)
		d.foreverHumanoidSince = sim.CurrentTime
		d.SetCurrentPowerBar(core.ManaBar)
		d.SetShapeshift(nil)
		d.DisableDynamicStatDep(sim, stam)
		d.AddStatsDynamic(sim, base.Invert())
		d.RemoveDynamicEquipScaling(sim, stats.Armor, 4.6)
		d.AddStatDynamic(sim, stats.Armor, -armor*4.6)
		d.PseudoStats.ThreatMultiplier /= 1.3
		d.AutoAttacks.SetMH(d.WeaponFromMainHand())
	}})
	rage := d.NewRageMetrics(d.fa("furor"))
	d.BearForm = d.RegisterSpell(Any, core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL | core.SpellFlagNoOnCastComplete, ManaCost: core.ManaCostOptions{BaseCost: .55, Multiplier: 100 - 10*d.Talents.NaturalShapeshifter}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		if d.InForm(Humanoid | Moonkin) {
			d.foreverHumanoidTime += sim.CurrentTime - d.foreverHumanoidSince
		}
		if d.InForm(Cat) {
			d.foreverLastCatEnergy = d.CurrentEnergy()
		}
		d.BearFormAura.Activate(sim)
		d.SpendRage(sim, d.CurrentRage(), rage)
		if sim.Proc(.2*d.fr("furor"), "Forever Furor Bear") {
			d.AddRage(sim, 10, rage)
		}
	}})
	if d.fr("mangle") > 0 {
		mangle := d.RegisterSpell(Bear, core.SpellConfig{ActionID: d.fa("mangle"), SpellCode: foreverMangle, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | core.SpellFlagMeleeMetrics, RageCost: core.RageCostOptions{Cost: 20 - d.fr("ferocity"), Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: d.NewTimer(), Duration: 6 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			targets := []*core.Unit{t}
			if d.BerserkAura != nil && d.BerserkAura.IsActive() {
				for _, target := range d.Env.Encounter.Targets {
					if &target.Unit != t && len(targets) < 3 {
						targets = append(targets, &target.Unit)
					}
				}
			}
			for _, target := range targets {
				r := sp.CalcAndDealDamage(sim, target, d.MHWeaponDamage(sim, sp.MeleeAttackPower(target))+26, sp.OutcomeMeleeSpecialHitAndCrit)
				if !r.Landed() && target == t {
					sp.IssueRefund(sim)
				}
			}
		}})
		if d.fr("berserk") > 0 {
			var builders []*core.Spell
			d.OnSpellRegistered(func(sp *core.Spell) {
				if sp.Flags.Matches(SpellFlagBuilder) {
					builders = append(builders, sp)
				}
			})
			immune := d.ForeverControlImmunityAura("Forever Berserk Fear Immunity", d.fa("berserk"), []core.ForeverControlKind{core.ForeverFear}, 15*time.Second)
			d.BerserkAura = d.RegisterAura(core.Aura{Label: "Forever Berserk", ActionID: d.fa("berserk"), Duration: 15 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
				mangle.CD.Duration = 0
				mangle.CD.Reset()
				for _, sp := range builders {
					sp.BonusCritRating += 100 * core.CritRatingPerCritChance
				}
				immune.Activate(sim)
			}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
				mangle.CD.Duration = 6 * time.Second
				for _, sp := range builders {
					sp.BonusCritRating -= 100 * core.CritRatingPerCritChance
				}
				immune.Deactivate(sim)
			}})
			sp := d.RegisterSpell(Any, core.SpellConfig{ActionID: d.fa("berserk"), Flags: core.SpellFlagAPL | core.SpellFlagHelpful, Cast: core.CastConfig{CD: core.Cooldown{Timer: d.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { d.BerserkAura.Activate(sim) }})
			d.AddMajorCooldown(core.MajorCooldown{Spell: sp.Spell, Type: core.CooldownTypeDPS})
		}
	}
	for _, v := range []struct {
		id           int32
		code         int32
		cost, damage float64
	}{{9881, foreverMaul, 15, 128}, {9908, foreverSwipe, 20, 83}} {
		v := v
		sp := d.RegisterSpell(Bear, core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, SpellCode: v.code, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | core.SpellFlagMeleeMetrics, RageCost: core.RageCostOptions{Cost: v.cost - d.fr("ferocity"), Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true}, DamageMultiplier: 1, ThreatMultiplier: 1.75, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if v.code == foreverSwipe {
				for i, target := range d.Env.Encounter.Targets {
					if i < 3 {
						sp.CalcAndDealDamage(sim, &target.Unit, v.damage*(1+.1*d.fr("feral-instinct")), sp.OutcomeMeleeSpecialHitAndCrit)
					}
				}
			} else {
				sp.CalcAndDealDamage(sim, t, v.damage+d.MHWeaponDamage(sim, sp.MeleeAttackPower(t)), sp.OutcomeMeleeSpecialHitAndCrit)
			}
		}})
		if v.code == foreverMaul {
			d.Maul = sp
		} else {
			d.SwipeBear = sp
		}
	}

	if !foreverdata.IsStrict(d.Forever) && d.fr("shredding-attacks") > 0 {
		// PREDICTED Forever baseline using the explicit TBC rank-1 tooltip:
		// https://www.wowhead.com/tbc/spell=33745/lacerate . This is an analog,
		// not a claim that the level-66 client rank or ID exists in Forever.
		action := d.fa("shredding-attacks")
		action.Tag = -action.Tag
		d.RegisterSpell(Bear, core.SpellConfig{ActionID: action, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | core.SpellFlagMeleeMetrics | core.SpellFlagIgnoreResists, RageCost: core.RageCostOptions{Cost: 15 - d.fr("shredding-attacks"), Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true}, DamageMultiplier: 1, ThreatMultiplier: 1, Dot: core.DotConfig{Aura: core.Aura{Label: "Forever Predicted Lacerate", MaxStacks: 5}, NumberOfTicks: 5, TickLength: 3 * time.Second, DamageMultiplier: 1, OnSnapshot: func(sim *core.Simulation, t *core.Unit, dot *core.Dot, b bool) {
			dot.Snapshot(t, 31*float64(dot.GetStacks()), false)
		}, OnTick: func(sim *core.Simulation, t *core.Unit, dot *core.Dot) {
			dot.CalcAndDealPeriodicSnapshotDamage(sim, t, dot.OutcomeTick)
		}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			r := sp.CalcAndDealDamage(sim, t, 31, sp.OutcomeMeleeSpecialHitAndCrit)
			if r.Landed() {
				dot := sp.Dot(t)
				dot.ApplyOrRefresh(sim)
				dot.AddStack(sim)
				dot.TakeSnapshot(sim, false)
			} else {
				sp.IssueRefund(sim)
			}
		}})
	}
	d.Enrage = d.RegisterSpell(Bear, core.SpellConfig{ActionID: core.ActionID{SpellID: 5229}, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: d.NewTimer(), Duration: time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
		core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Second, NumTicks: 10, OnAction: func(sim *core.Simulation) { d.AddRage(sim, 2, rage) }})
	}})
}
