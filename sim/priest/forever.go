package priest

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

type foreverPriestState struct {
	holyNova         *core.Spell
	freeNova         *core.Aura
	weaving, embrace core.AuraArray
	shields, aegis   map[int32]*core.ForeverAbsorb
	healsRegistered  bool
}

func (p *Priest) applyForeverCasterTalents() {
	if p.Forever == nil {
		return
	}
	f := &foreverPriestState{shields: map[int32]*core.ForeverAbsorb{}, aegis: map[int32]*core.ForeverAbsorb{}}
	p.foreverState = f
	val := func(id string, index int) float64 { return p.ForeverValue("priest.talent."+id, index, 0) }
	p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1 + val("darkness", 0)/100
	for _, kind := range []core.ForeverControlKind{core.ForeverStun, core.ForeverFear, core.ForeverSilence} {
		p.ForeverControlReduction(kind, val("silent-resolve", 1)/100)
	}
	p.WeakenedSouls = p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Weakened Soul", ActionID: core.ActionID{SpellID: 6788}, Duration: 15 * time.Second})
	})
	f.weaving = p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Shadow Weaving-" + p.Label, ActionID: p.ForeverAction("priest.talent.shadow-weaving"), Duration: 15 * time.Second, MaxStacks: 5})
	})
	f.embrace = p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Vampiric Embrace-" + p.Label, ActionID: p.ForeverAction("priest.talent.vampiric-embrace"), Duration: 30 * time.Second})
	})
	f.freeNova = p.RegisterAura(core.Aura{Label: "Forever Searing Light", ActionID: p.ForeverAction("priest.talent.searing-light"), Duration: 15 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		if f.holyNova != nil {
			f.holyNova.Cost.Multiplier -= 100
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		if f.holyNova != nil {
			f.holyNova.Cost.Multiplier += 100
		}
	}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
		if s == f.holyNova {
			a.Deactivate(sim)
		}
	}})
	p.ForeverDamageMultiplier(func(s *core.Spell, at *core.AttackTable) float64 {
		mult := 1.0
		if s.SpellSchool.Matches(core.SpellSchoolShadow) {
			mult *= 1 + .02*float64(f.weaving.Get(at.Defender).GetStacks())
		}
		if (s.SpellCode == SpellCode_PriestSmite || s.SpellCode == SpellCode_PriestPenance) && p.ForeverRank("priest.talent.power-in-light") > 0 {
			for _, hf := range p.HolyFire {
				if hf != nil && hf.Dot(at.Defender).IsActive() {
					mult *= 1 + val("power-in-light", 0)/100
					break
				}
			}
		}
		return mult
	})
	p.OnSpellRegistered(func(s *core.Spell) {
		if s.ProcMask.Matches(core.ProcMaskRanged) && s.OtherID == proto.OtherAction_OtherActionShoot {
			s.DamageMultiplier *= 1 + val("wand-specialization", 0)/100
		}
		if !s.Flags.Matches(SpellFlagPriest) {
			return
		}
		s.PushbackReduction += val("twilight-focus", 0) / 100
		if s.SpellSchool.Matches(core.SpellSchoolHoly) {
			s.ThreatMultiplier *= 1 - val("silent-resolve", 0)/100
			if !s.Flags.Matches(core.SpellFlagHelpful) {
				s.DamageMultiplierAdditive += val("searing-light", 0) / 100
			}
		}
		if s.DefaultCast.CastTime == 0 && !s.Flags.Matches(core.SpellFlagChanneled) {
			s.DamageMultiplierAdditive += val("twin-disciplines", 0) / 100
		}
		if s.Cost != nil && (s.DefaultCast.CastTime == 0 || s.SpellCode == SpellCode_PriestSmite || s.SpellCode == SpellCode_PriestHolyFire) {
			s.Cost.Multiplier -= int32(val("mental-agility", 0))
		}
		if s.Flags.Matches(core.SpellFlagHelpful) {
			s.DamageMultiplierAdditive += val("spiritual-healing", 0) / 100
		}
		if s.SpellCode == SpellCode_PriestGreaterHeal || s.SpellCode == SpellCode_PriestHeal || s.SpellCode == SpellCode_PriestPenance || s.SpellCode == SpellCode_PriestPrayerOfMending {
			if s.Cost != nil {
				s.Cost.Multiplier -= int32(val("improved-healing", 0))
			}
		}
		if s.SpellCode == SpellCode_PriestRenew {
			s.DamageMultiplierAdditive += val("improved-renew", 0) / 100
		}
		if s.SpellCode == SpellCode_PriestDevouringPlague && s.Cost != nil {
			s.Cost.Multiplier -= int32(val("devouring-contagion", 0))
		}
		yards := 30.0
		if s.Flags.Matches(core.SpellFlagHelpful) {
			yards = 40
		}
		if s.SpellCode == SpellCode_PriestMindFlay {
			yards = 20 + val("improved-mind-flay", 1)
		}
		if s.SpellCode == SpellCode_PriestPenance && !s.Flags.Matches(core.SpellFlagHelpful) {
			yards = 36
		}
		if s.SpellSchool.Matches(core.SpellSchoolShadow) {
			yards *= 1 + val("shadow-reach", 0)/100
		}
		if s.SpellCode == SpellCode_PriestSmite || s.SpellCode == SpellCode_PriestHolyFire {
			yards *= 1 + val("holy-reach", 0)/100
		}
		p.ForeverSpellRange(s, yards)
		oldCondition := s.ExtraCastCondition
		if s.Flags.Matches(core.SpellFlagHelpful) {
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
				return (p.ShadowformAura == nil || !p.ShadowformAura.IsActive()) && (oldCondition == nil || oldCondition(sim, t))
			}
		}
		old := s.ApplyEffects
		s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
			renewed := (sp.SpellCode == SpellCode_PriestFlashHeal || sp.SpellCode == SpellCode_PriestHeal || sp.SpellCode == SpellCode_PriestGreaterHeal || sp.SpellCode == SpellCode_PriestBindingHeal || sp.SpellCode == SpellCode_PriestPenance) && !p.IsOpponent(t) && p.WeakenedSouls.Get(t).IsActive()
			crit := 0.0
			if renewed {
				crit = val("renewed-hope", 0) * core.SpellCritRatingPerCritChance
			}
			sp.BonusCritRating += crit
			old(sim, t, sp)
			sp.BonusCritRating -= crit
			if renewed && p.ForeverRank("priest.talent.renewed-hope") > 0 {
				a := p.WeakenedSouls.Get(t)
				a.UpdateExpires(sim, max(sim.CurrentTime, a.ExpiresAt()-time.Duration(val("renewed-hope", 1)*float64(time.Second))))
			}
		}
	})
	// Non-periodic critical heals only; each healer owns its Divine Aegis pool.
	inspiration := p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
		dep := t.NewDynamicMultiplyStat(stats.Armor, 1+val("inspiration", 0)/100)
		a := t.GetOrRegisterAura(core.Aura{Label: "Forever Inspiration-" + p.Label, ActionID: p.ForeverAction("priest.talent.inspiration"), Duration: 15 * time.Second})
		a.NewExclusiveEffect("Forever Inspiration Armor", false, core.ExclusiveEffect{Priority: val("inspiration", 0), OnGain: func(e *core.ExclusiveEffect, sim *core.Simulation) { t.EnableDynamicStatDep(sim, dep) }, OnExpire: func(e *core.ExclusiveEffect, sim *core.Simulation) { t.DisableDynamicStatDep(sim, dep) }})
		return a
	})
	for _, t := range p.Env.AllUnits {
		if !p.IsOpponent(t) {
			f.aegis[t.UnitIndex] = core.NewForeverAbsorb(t, "Forever Divine Aegis-"+p.Label, p.ForeverAction("priest.talent.divine-aegis"), 12*time.Second, 0)
		}
	}
	lastHeal := core.ActionID{}
	mana := p.NewManaMetrics(p.ForeverAction("priest.talent.litany-of-light"))
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Priest healing triggers", OnReset: func(a *core.Aura, sim *core.Simulation) { lastHeal = core.ActionID{} }, OnHealDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.DidCrit() {
			return
		}
		if p.ForeverRank("priest.talent.inspiration") > 0 {
			inspiration.Get(r.Target).Activate(sim)
		}
		if p.ForeverRank("priest.talent.divine-aegis") > 0 {
			f.aegis[r.Target.UnitIndex].Apply(sim, r.Damage*val("divine-aegis", 0)/100, true)
		}
	}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
		if !s.Flags.Matches(core.SpellFlagHelpful) || !s.ProcMask.Matches(core.ProcMaskSpellHealing) {
			return
		}
		if !lastHeal.IsEmptyAction() && !lastHeal.SameAction(s.ActionID) && s.Cost != nil {
			p.AddMana(sim, s.Cost.BaseCost*val("litany-of-light", 0)/100, mana)
		}
		lastHeal = s.ActionID
	}}))
	// Reactive damage and healing use actual event callbacks, never assumed uptime.
	blackouts := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Blackout-"+p.Label, p.ForeverAction("priest.talent.blackout"), core.ForeverStun, 3*time.Second)
	})
	flaySlows := p.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverSnareAura("Forever Mind Flay slow-"+p.Label, p.ForeverAction("priest.talent.improved-mind-flay"), 3*time.Second, core.TernaryFloat64(p.ForeverRank("priest.talent.improved-mind-flay") > 0, val("improved-mind-flay", 2)/100, .5))
	})
	heal := p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: p.ForeverAction("priest.talent.vampiric-embrace"), SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagPassiveSpell | core.SpellFlagHelpful | core.SpellFlagIgnoreAttackerModifiers, DamageMultiplier: 1, ThreatMultiplier: 0})
	damageTrigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() || !s.SpellSchool.Matches(core.SpellSchoolShadow) {
			return
		}
		if p.ForeverRank("priest.talent.shadow-weaving") > 0 && sim.Proc(min(1, val("shadow-weaving", 0)/100), "Forever Shadow Weaving") {
			a := f.weaving.Get(r.Target)
			a.Activate(sim)
			a.AddStack(sim)
		}
		if p.ForeverRank("priest.talent.blackout") > 0 && sim.Proc(val("blackout", 0)/100, "Forever Blackout") {
			blackouts.Get(r.Target).Activate(sim)
		}
		if s.SpellCode == SpellCode_PriestMindFlay {
			flaySlows.Get(r.Target).Activate(sim)
		}
		if f.embrace.Get(r.Target).IsActive() && r.Damage > 0 {
			for _, member := range p.Party.Players {
				heal.CalcAndDealHealing(sim, &member.GetCharacter().Unit, r.Damage*.2, heal.OutcomeHealing)
			}
		}
	}
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Priest damage triggers", OnSpellHitDealt: damageTrigger, OnPeriodicDamageDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		damageTrigger(a, sim, s, r)
		if s.SpellCode == SpellCode_PriestHolyFire && p.ForeverRank("priest.talent.searing-light") > 0 && sim.Proc(val("searing-light", 1)/100, "Forever Searing Light") {
			f.freeNova.Activate(sim)
		}
	}}))
	p.registerForeverReactiveTalents()
}
func (p *Priest) registerForeverShadowform() {
	if p.ForeverRank("priest.talent.shadowform") == 0 {
		return
	}
	p.ShadowformAura = p.RegisterAura(core.Aura{Label: "Shadowform", ActionID: core.ActionID{SpellID: 15473}, Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) {
		p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] *= 1.1
		p.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] *= .85
		for _, s := range p.Spellbook {
			if s.SpellSchool.Matches(core.SpellSchoolShadow) {
				if s.Cost != nil {
					s.Cost.Multiplier -= 50
				}
				s.CritDamageBonus += 1
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		p.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexShadow] /= 1.1
		p.PseudoStats.SchoolDamageTakenMultiplier[stats.SchoolIndexPhysical] /= .85
		for _, s := range p.Spellbook {
			if s.SpellSchool.Matches(core.SpellSchoolShadow) {
				if s.Cost != nil {
					s.Cost.Multiplier += 50
				}
				s.CritDamageBonus -= 1
			}
		}
	}})
	p.Shadowform = p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 15473}, SpellSchool: core.SpellSchoolShadow, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 345}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { p.ShadowformAura.Activate(sim) }})
}
func (p *Priest) registerForeverSpells() {
	if p.Forever == nil {
		return
	}
	p.RegisterForeverWand()
	p.registerForeverHealing()
	p.registerForeverBaseline()
	p.registerForeverRacials()
}
