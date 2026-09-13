package paladin

import (
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/stats"
	"strings"
	"time"
)

const (
	foreverHolyLight int32 = 1001 + iota
	foreverFlashOfLight
	foreverVigil
	foreverHolyStrike
)

func (p *Paladin) fr(name string) float64       { return float64(p.ForeverRank("paladin.talent." + name)) }
func (p *Paladin) fa(name string) core.ActionID { return p.ForeverAction("paladin.talent." + name) }
func (p *Paladin) applyForeverTalents() {
	if p.Forever == nil {
		return
	}
	p.MultiplyStat(stats.Stamina, 1+.02*p.fr("sacred-duty"))
	if p.Options.RighteousFury {
		p.PseudoStats.DamageTakenMultiplier *= 1 - .02*p.fr("improved-righteous-fury")
	} else {
		p.PseudoStats.ThreatMultiplier *= 1 - .1*p.fr("instrument-of-law")
	}
	p.ForeverControlReduction(core.ForeverFear, .15*p.fr("unyielding-faith"))
	if p.fr("pursuit-of-justice") > 0 {
		id := p.fa("pursuit-of-justice")
		p.RegisterAura(core.Aura{Label: "Forever Pursuit of Justice", ActionID: id, OnInit: func(a *core.Aura, sim *core.Simulation) {
			p.AddMoveSpeedModifier(&a.ActionID, 1+.08*p.fr("pursuit-of-justice"))
		}})
	}
	p.OnSpellRegistered(func(sp *core.Spell) {
		if sp.DefaultCast.CastTime == 0 && sp.Cost != nil && sp.Cost.CostType() == core.CostTypeMana {
			sp.Cost.Multiplier -= int32(2 * p.fr("benediction"))
		}
		if sp.ProcMask.Matches(core.ProcMaskSpellHealing) && sp.SpellCode != SpellCode_PaladinHolyShock {
			sp.DamageMultiplier *= 1 + .04*p.fr("healing-light")
		}
		if sp.SpellCode == foreverHolyLight || sp.SpellCode == foreverFlashOfLight || sp.SpellCode == foreverVigil {
			sp.PushbackReduction += .35 * p.fr("spiritual-focus")
		}
		if sp.DefenseType == core.DefenseTypeMagic || sp.ProcMask.Matches(core.ProcMaskSpellHealing) {
			sp.BonusCritRating += p.fr("holy-power") * core.SpellCritRatingPerCritChance
			if sp.SpellCode == SpellCode_PaladinHolyShock {
				sp.BonusCritRating += 2 * p.fr("holy-power") * core.SpellCritRatingPerCritChance
			}
		}
		if sp.SpellCode == SpellCode_PaladinExorcism || sp.SpellCode == SpellCode_PaladinHolyWrath {
			sp.CD.Duration = time.Duration(float64(sp.CD.Duration) * (1 - .165*p.fr("purifying-power")))
		}
		if sp.SpellCode == SpellCode_PaladinHammerOfWrath {
			sp.DefaultCast.CastTime -= time.Duration(500*p.fr("instrument-of-law")) * time.Millisecond
		}
		if sp.SpellCode == SpellCode_PaladinExorcism || sp.SpellCode == SpellCode_PaladinHolyWrath || sp.SpellCode == SpellCode_PaladinHammerOfWrath || sp.SpellCode == SpellCode_PaladinConsecration {
			if sp.Cost != nil {
				sp.Cost.Multiplier -= int32(20 * p.fr("holy-conduit"))
			}
		}
		// Seal procs and Judgements share Classic triggered spell handling.
		if sp.SpellCode == SpellCode_PaladinJudgementOfCommand || sp.SpellCode == SpellCode_PaladinJudgementOfRighteousness || (sp.SpellSchool.Matches(core.SpellSchoolHoly) && sp.ProcMask.Matches(core.ProcMaskMeleeMHSpecial) && !sp.Flags.Matches(core.SpellFlagAPL)) {
			sp.DamageMultiplier *= 1 + .05*p.fr("improved-seals")
		}
	})
	if p.fr("illumination") > 0 {
		metrics := p.NewManaMetrics(p.fa("illumination"))
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Illumination", OnHealDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && sim.Proc(.2*p.fr("illumination"), "Forever Illumination") {
				p.AddMana(sim, .5*sp.DefaultCast.Cost, metrics)
			}
		}}))
	}
	if p.fr("shield-specialization") > 0 {
		p.PseudoStats.BlockValueMultiplier += p.ForeverValue("paladin.talent.shield-specialization", 0, 0) / 100
		metrics := p.NewManaMetrics(p.fa("shield-specialization"))
		icd := core.Cooldown{Timer: p.NewTimer(), Duration: 3 * time.Second}
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Shield Specialization", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidBlock() && icd.IsReady(sim) && sim.Proc(p.ForeverValue("paladin.talent.shield-specialization", 1, 0)/100, "Forever Shield Specialization") {
				icd.Use(sim)
				p.AddMana(sim, .06*p.MaxMana(), metrics)
			}
		}}))
	}
	if p.fr("redoubt") > 0 {
		bonus := 6 * p.fr("redoubt")
		a := p.RegisterAura(core.Aura{Label: "Forever Redoubt", ActionID: p.fa("redoubt"), Duration: 10 * time.Second, MaxStacks: 5, OnGain: func(_ *core.Aura, sim *core.Simulation) { p.AddStatDynamic(sim, stats.Block, bonus) }, OnExpire: func(_ *core.Aura, sim *core.Simulation) { p.AddStatDynamic(sim, stats.Block, -bonus) }, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidBlock() {
				a.RemoveStack(sim)
			}
		}})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Redoubt Trigger", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Landed() && sp.ProcMask.Matches(core.ProcMaskMelee) && sim.Proc(.1, "Forever Redoubt") {
				a.Activate(sim)
				a.SetStacks(sim, 5)
			}
		}}))
	}
	if p.fr("reckoning") > 0 {
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Reckoning", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			chance := 0.0
			if r.DidBlock() && sp.ProcMask.Matches(core.ProcMaskMelee) {
				chance = .08 * p.fr("reckoning")
			}
			if r.DidCrit() {
				chance = .2 * p.fr("reckoning")
			}
			if sim.Proc(chance, "Forever Reckoning") {
				p.AutoAttacks.ExtraMHAttack(sim, 1, p.fa("reckoning"), sp.ActionID)
			}
		}}))
	}
	if p.fr("vengeance") > 0 {
		a := p.RegisterAura(core.Aura{Label: "Forever Vengeance", ActionID: p.fa("vengeance"), Duration: 30 * time.Second, MaxStacks: 5, OnStacksChange: func(_ *core.Aura, sim *core.Simulation, old, new int32) {
			m := (1 + .01*p.fr("vengeance")*float64(new)) / (1 + .01*p.fr("vengeance")*float64(old))
			p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= m
			p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexHoly] *= m
		}})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Vengeance Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() {
				a.Activate(sim)
				a.AddStack(sim)
			}
		}}))
	}
	if p.fr("vindication") > 0 {
		dep := p.NewDynamicMultiplyStat(stats.AttackPower, 1+.01*p.fr("vindication"))
		a := p.RegisterAura(core.Aura{Label: "Forever Vindication", ActionID: p.fa("vindication"), Duration: 30 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) { p.EnableDynamicStatDep(sim, dep) }, OnExpire: func(_ *core.Aura, sim *core.Simulation) { p.DisableDynamicStatDep(sim, dep) }})
		enemies := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			v := 42 * p.fr("vindication")
			return t.RegisterAura(core.Aura{Label: "Forever Vindication-" + p.Label, Duration: 30 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) { t.AddStatDynamic(sim, stats.AttackPower, -v) }, OnExpire: func(_ *core.Aura, sim *core.Simulation) { t.AddStatDynamic(sim, stats.AttackPower, v) }})
		})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Vindication Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.Landed() && sp.ProcMask.Matches(core.ProcMaskMelee) {
				a.Activate(sim)
				enemies.Get(r.Target).Activate(sim)
			}
		}}))
	}
	if p.fr("eye-for-an-eye") > 0 {
		reflect := p.RegisterSpell(core.SpellConfig{ActionID: p.fa("eye-for-an-eye"), SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskEmpty, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 1})
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Eye for an Eye", OnSpellHitTaken: func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() {
				reflect.CalcAndDealDamage(sim, sp.Unit, min(r.Damage*.05*p.fr("eye-for-an-eye"), p.MaxHealth()*.5), reflect.OutcomeAlwaysHit)
			}
		}}))
	}
	if p.fr("consecrated-ground") > 0 {
		var consecrations []*core.Spell
		p.OnSpellRegistered(func(sp *core.Spell) {
			if sp.SpellCode == SpellCode_PaladinConsecration {
				consecrations = append(consecrations, sp)
			}
		})
		p.ForeverDamageMultiplier(func(sp *core.Spell, at *core.AttackTable) float64 {
			if !sp.SpellSchool.Matches(core.SpellSchoolHoly) {
				return 1
			}
			for i, t := range p.Env.Encounter.Targets {
				if i < 4 && &t.Unit == at.Defender {
					for _, c := range consecrations {
						if c.AOEDot().IsActive() {
							return 1 + .05*p.fr("consecrated-ground")
						}
					}
				}
			}
			return 1
		})
	}
	if p.fr("twist-of-light") > 0 {
		p.foreverEcho = p.RegisterAura(core.Aura{Label: "Forever Seal Echo", ActionID: p.fa("twist-of-light"), Duration: core.NeverExpires, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if !sp.ProcMask.Matches(core.ProcMaskMelee) {
				return
			}
			old := p.foreverEchoSeal
			a.Deactivate(sim)
			if old != nil && old.OnSpellHitDealt != nil {
				old.OnSpellHitDealt(old, sim, sp, r)
			}
		}})
	}
}

func (p *Paladin) registerForeverSpells() {
	if p.Forever == nil {
		return
	}
	p.registerForeverHealing()
	p.registerForeverDefenses()
	// Baseline Holy Strike is only named in direct tooltips. Normal Holy weapon
	// damage, 6 sec cooldown and 6% base mana are explicitly PREDICTED analogs.
	iron := p.RegisterAura(core.Aura{Label: "Forever Iron Creed", ActionID: p.fa("iron-creed"), Duration: 6 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
		p.PseudoStats.DamageTakenMultiplier *= 1 - .02*p.fr("iron-creed")
	}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
		p.PseudoStats.DamageTakenMultiplier /= 1 - .02*p.fr("iron-creed")
	}})
	if !foreverdata.IsStrict(p.Forever) && (p.HasForeverMechanic("paladin.baseline.holy-strike") || p.fr("improved-holy-strike")+p.fr("sacred-arbiter")+p.fr("iron-creed") > 0) {
		p.foreverHolyStrike = p.RegisterSpell(core.SpellConfig{ActionID: p.ForeverAction("paladin.baseline.holy-strike"), SpellCode: foreverHolyStrike, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | core.SpellFlagMeleeMetrics, ManaCost: core.ManaCostOptions{BaseCost: .06}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 6*time.Second - time.Duration(p.fr("improved-holy-strike"))*time.Second}}, DamageMultiplier: 1 + .1*p.fr("sacred-arbiter"), ThreatMultiplier: 1 + .05*p.fr("iron-creed"), BonusCoefficient: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			r := sp.CalcAndDealDamage(sim, t, p.MHWeaponDamage(sim, sp.MeleeAttackPower(t)), sp.OutcomeMeleeSpecialHitAndCrit)
			if r.Landed() {
				if p.Options.RighteousFury {
					iron.Activate(sim)
				}
				if p.fr("sacred-arbiter") > 0 {
					for _, a := range t.GetAuras() {
						if a.IsActive() && strings.Contains(a.Label, "Judgement") {
							a.Refresh(sim)
						}
					}
				}
			}
		}})
	}
	if p.fr("swift-judgement") > 0 {
		a := p.RegisterAura(core.Aura{Label: "Forever Swift Judgement", ActionID: p.fa("swift-judgement"), Duration: core.NeverExpires, OnGain: func(_ *core.Aura, sim *core.Simulation) { p.judgement.Cost.Multiplier -= 100 }, OnExpire: func(_ *core.Aura, sim *core.Simulation) { p.judgement.Cost.Multiplier += 100 }})
		old := p.judgement.ApplyEffects
		p.judgement.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { old(sim, t, sp); a.Deactivate(sim) }
		p.RegisterSpell(core.SpellConfig{ActionID: p.fa("swift-judgement"), Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) { p.judgement.CD.Reset(); a.Activate(sim) }})
	}
	if p.fr("sanctified-judgement") > 0 {
		metrics := p.NewManaMetrics(p.fa("sanctified-judgement"))
		old := p.judgement.ApplyEffects
		p.judgement.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			seal := p.currentSeal
			cost := 0.0
			for _, spell := range p.Spellbook {
				if seal != nil && spell.ActionID == seal.ActionID {
					cost = spell.DefaultCast.Cost
				}
			}
			old(sim, t, sp)
			if sim.Proc(p.ForeverValue("paladin.talent.sanctified-judgement", 0, 0)/100, "Forever Sanctified Judgement") {
				p.AddMana(sim, .2*cost, metrics)
			}
		}
	}
}

func (p *Paladin) registerForeverHealing() {
	for _, v := range []struct {
		id              int32
		code            int32
		mana, low, high float64
		cast            time.Duration
	}{{25292, foreverHolyLight, 660, 1590, 1770, 2500 * time.Millisecond}, {19943, foreverFlashOfLight, 140, 348, 389, 1500 * time.Millisecond}} {
		v := v
		p.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: v.id}, SpellCode: v.code, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: v.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: v.cast}}, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: v.cast.Seconds() / 3.5, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if p.IsOpponent(t) {
				t = &p.Unit
			}
			sp.CalcAndDealHealing(sim, t, sim.Roll(v.low, v.high), sp.OutcomeHealingCrit)
		}})
	}
	var shock *core.Spell
	vigils := map[*core.Unit]*core.Aura{}
	if p.fr("light-s-vigil") > 0 {
		for _, t := range p.Env.AllUnits {
			vigils[t] = t.RegisterAura(core.Aura{Label: fmt.Sprintf("Forever Light's Vigil-%d", p.UnitIndex), ActionID: p.fa("light-s-vigil"), Duration: 30 * time.Second})
		}
		p.RegisterSpell(core.SpellConfig{ActionID: p.fa("light-s-vigil"), SpellCode: foreverVigil, SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 730}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 1500 * time.Millisecond}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 6 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			for u, a := range vigils {
				if p.IsOpponent(t) && p.IsOpponent(u) || !p.IsOpponent(t) && p.Env.Raid.GetPlayerParty(u) == p.Env.Raid.GetPlayerParty(t) {
					a.Deactivate(sim)
				}
			}
			vigils[t].Activate(sim)
		}})
	}
	if p.fr("holy-shock") > 0 {
		metrics := p.NewManaMetrics(p.fa("light-s-vigil"))
		shock = p.RegisterSpell(core.SpellConfig{ActionID: p.fa("holy-shock"), SpellCode: SpellCode_PaladinHolyShock, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage | core.ProcMaskSpellHealing, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 160}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 10 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .429, ApplyEffects: func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			if p.IsOpponent(t) {
				sp.CalcAndDealDamage(sim, t, sim.Roll(129, 139), sp.OutcomeMagicHitAndCrit)
			} else {
				old := sp.DamageMultiplier
				sp.DamageMultiplier *= 1 + .04*p.fr("healing-light")
				sp.CalcAndDealHealing(sim, t, sim.Roll(110, 118), sp.OutcomeHealingCrit)
				sp.DamageMultiplier = old
			}
			if a := vigils[t]; a != nil && a.IsActive() {
				a.Deactivate(sim)
				sp.CD.Reset()
				if p.IsOpponent(t) {
					sp.CalcAndDealDamage(sim, t, sim.Roll(175, 189), sp.OutcomeMagicHitAndCrit)
					p.AddMana(sim, 730*.75, metrics)
				} else {
					party := p.Env.Raid.GetPlayerParty(t)
					if party == nil || len(party.Players) == 0 {
						party = p.Party
					}
					for _, a := range party.Players {
						sp.CalcAndDealHealing(sim, &a.GetCharacter().Unit, sim.Roll(315, 333), sp.OutcomeHealingCrit)
					}
				}
			}
		}})
	}
	if shock != nil {
		p.ForeverSpellRange(shock, 20)
	}
	if p.fr("infusion-of-light") > 0 {
		var spells []*core.Spell
		for _, sp := range p.Spellbook {
			if sp.SpellCode == foreverHolyLight {
				spells = append(spells, sp)
			}
		}
		amount := time.Duration(500*p.fr("infusion-of-light")) * time.Millisecond
		a := p.RegisterAura(core.Aura{Label: "Forever Infusion of Light", ActionID: p.fa("infusion-of-light"), Duration: 15 * time.Second, OnGain: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range spells {
				sp.DefaultCast.CastTime -= amount
			}
		}, OnExpire: func(_ *core.Aura, sim *core.Simulation) {
			for _, sp := range spells {
				sp.DefaultCast.CastTime += amount
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, sp *core.Spell) {
			if sp.SpellCode == foreverHolyLight {
				a.Deactivate(sim)
			}
		}})
		proc := func(_ *core.Aura, sim *core.Simulation, sp *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && (sp.SpellCode == foreverFlashOfLight || sp.SpellCode == SpellCode_PaladinHolyShock) {
				a.Activate(sim)
			}
		}
		core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Infusion Trigger", OnHealDealt: proc, OnSpellHitDealt: proc}))
	}
}
