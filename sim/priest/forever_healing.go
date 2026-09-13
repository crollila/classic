package priest

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

// Healing baselines preserve the level60 Classic spell identities. Parameters
// absent from the discovery evidence are explicitly provisional Classic analogues.
func (p *Priest) registerForeverHealing() {
	f := p.foreverState
	if f.healsRegistered {
		return
	}
	f.healsRegistered = true
	val := func(id string, index int) float64 { return p.ForeverValue("priest.talent."+id, index, 0) }
	for _, h := range []struct {
		id              int32
		code            int32
		low, high, mana float64
		cast            time.Duration
	}{
		{10917, SpellCode_PriestFlashHeal, 885, 1028, 380, 1500 * time.Millisecond},
		{10965, SpellCode_PriestGreaterHeal, 1966, 2194, 655, 3 * time.Second},
		{6064, SpellCode_PriestHeal, 712, 804, 305, 3 * time.Second},
		{2053, SpellCode_PriestHeal, 135, 157, 75, 2500 * time.Millisecond},
	} {
		h := h
		cast := h.cast
		if h.id == 10965 || h.id == 6064 {
			cast -= time.Duration(float64(time.Second) * .1 * float64(p.ForeverRank("priest.talent.divine-fury")))
		}
		s := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: core.ActionID{SpellID: h.id}, SpellCode: h.code, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: h.mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: cast}}, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: h.cast.Seconds() / 3.5, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			p.foreverRenewedHopeHeal(sim, t, s, sim.Roll(h.low, h.high))
		}})
		if h.code == SpellCode_PriestFlashHeal {
			p.FlashHeal = append(p.FlashHeal, s)
		} else if h.code == SpellCode_PriestGreaterHeal {
			p.GreaterHeal = append(p.GreaterHeal, s)
		}
	}
	renew := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: core.ActionID{SpellID: 10929}, SpellCode: SpellCode_PriestRenew, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 365}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: .5,
		Hot: core.DotConfig{Aura: core.Aura{Label: "Forever Renew-" + p.Label}, NumberOfTicks: 5, TickLength: 3 * time.Second, BonusCoefficient: .2, OnSnapshot: func(sim *core.Simulation, t *core.Unit, d *core.Dot, roll bool) { d.SnapshotHeal(t, 162, roll) }, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
			d.CalcAndDealPeriodicSnapshotHealing(sim, t, d.OutcomeTick)
		}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { s.Hot(t).Apply(sim) },
	})
	p.Renew = append(p.Renew, renew)
	for _, t := range p.Env.AllUnits {
		if !p.IsOpponent(t) {
			f.shields[t.UnitIndex] = core.NewForeverAbsorb(t, "Forever Power Word Shield-"+p.Label, core.ActionID{SpellID: 10901}, 30*time.Second, 0)
		}
	}
	shieldCD := core.Cooldown{}
	if duration := max(0, 4*time.Second-time.Duration(val("soul-warding", 0)*float64(time.Second))); duration > 0 {
		shieldCD = core.Cooldown{Timer: p.NewTimer(), Duration: duration}
	}
	shield := p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 10901}, SpellCode: SpellCode_PriestPowerWordShield, SpellSchool: core.SpellSchoolHoly, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 500, Multiplier: 100 - int32(val("soul-warding", 1))}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: shieldCD}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
		return !p.IsOpponent(t) && !p.WeakenedSouls.Get(t).IsActive()
	}, DamageMultiplier: 1, ThreatMultiplier: 0,
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			f.shields[t.UnitIndex].Apply(sim, (942+.1*s.HealingPower(t))*(1+val("improved-power-word-shield", 0)/100)*p.PseudoStats.ShieldDealtMultiplier, false)
			p.WeakenedSouls.Get(t).Activate(sim)
		},
	})
	p.PowerWordShield = append(p.PowerWordShield, shield)
	poh := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: core.ActionID{SpellID: 10961}, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 1030}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 3 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: .286, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		for _, member := range p.Party.Players {
			s.CalcAndDealHealing(sim, &member.GetCharacter().Unit, sim.Roll(939, 991), s.OutcomeHealingCrit)
		}
	}})
	p.PrayerOfHealing = append(p.PrayerOfHealing, poh)
	if p.ForeverRank("priest.talent.binding-heal") > 0 {
		p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: p.ForeverAction("priest.talent.binding-heal"), SpellCode: SpellCode_PriestBindingHeal, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 155}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 1500 * time.Millisecond}}, DamageMultiplier: 1, ThreatMultiplier: .25, BonusCoefficient: 1.5 / 3.5 / 2,
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				p.foreverRenewedHopeHeal(sim, t, s, sim.Roll(382, 443))
				if t != &p.Unit {
					p.foreverRenewedHopeHeal(sim, &p.Unit, s, sim.Roll(382, 443))
				}
			},
		})
	}
	if p.ForeverRank("priest.talent.holy-nova") > 0 {
		heal := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: p.ForeverAction("priest.talent.holy-nova").WithTag(-p.ForeverAction("priest.talent.holy-nova").Tag), SpellCode: SpellCode_PriestHolyNova, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 0, BonusCoefficient: .107})
		f.holyNova = p.RegisterSpell(core.SpellConfig{ActionID: p.ForeverAction("priest.talent.holy-nova"), SpellCode: SpellCode_PriestHolyNova, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagPriest | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 185}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, DamageMultiplier: 1, ThreatMultiplier: 0, BonusCoefficient: .107,
			ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
				return p.DistanceFromTarget <= 10*(1+val("holy-reach", 0)/100) && (p.ShadowformAura == nil || !p.ShadowformAura.IsActive())
			},
			ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				for _, t := range sim.Encounter.TargetUnits {
					s.CalcAndDealDamage(sim, t, sim.Roll(31, 36), s.OutcomeMagicHitAndCrit)
				}
				for _, member := range p.Party.Players {
					heal.CalcAndDealHealing(sim, &member.GetCharacter().Unit, sim.Roll(68, 77), heal.OutcomeHealingCrit)
				}
			},
		})
	}
	if p.ForeverRank("priest.talent.penance") > 0 {
		penanceTimer := p.NewTimer()
		for _, helpful := range []bool{false, true} {
			helpful := helpful
			action := p.ForeverAction("priest.talent.penance")
			flags := SpellFlagPriest | core.SpellFlagAPL | core.SpellFlagChanneled
			mask := core.ProcMaskSpellDamage
			if helpful {
				action = action.WithTag(-action.Tag)
				flags |= core.SpellFlagHelpful
				mask = core.ProcMaskSpellHealing
			}
			cfg := core.SpellConfig{ActionID: action, SpellCode: SpellCode_PriestPenance, SpellSchool: core.SpellSchoolHoly, DefenseType: core.DefenseTypeMagic, ProcMask: mask, Flags: flags, ManaCost: core.ManaCostOptions{FlatCost: 85}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: penanceTimer, Duration: 12 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 1.0 / 3.5}
			dc := core.DotConfig{Aura: core.Aura{Label: "Forever Penance-" + p.Label}, NumberOfTicks: 2, TickLength: time.Second, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
				if helpful {
					p.foreverRenewedHopeHeal(sim, t, d.Spell, 98)
				} else {
					d.Spell.CalcAndDealDamage(sim, t, 19, d.Spell.OutcomeMagicHitAndCrit)
				}
			}}
			if helpful {
				cfg.Hot = dc
			} else {
				cfg.Dot = dc
			}
			cfg.ApplyEffects = func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				if helpful {
					s.Hot(t).Apply(sim)
					p.foreverRenewedHopeHeal(sim, t, s, 98)
				} else {
					s.Dot(t).Apply(sim)
					s.CalcAndDealDamage(sim, t, 19, s.OutcomeMagicHitAndCrit)
				}
			}
			p.RegisterSpell(cfg)
		}
	}
	if p.ForeverRank("priest.talent.prayer-of-mending") > 0 {
		p.registerForeverPrayerOfMending()
	}
	if p.ForeverRank("priest.talent.power-infusion") > 0 {
		auras := p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
			return t.GetOrRegisterAura(core.Aura{Label: "Forever Power Infusion", ActionID: p.ForeverAction("priest.talent.power-infusion"), Duration: 15 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
				t.PseudoStats.HealingDealtMultiplier *= 1.2
				t.PseudoStats.SchoolDamageDealtMultiplier.MultiplyMagicSchools(1.2)
			}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
				t.PseudoStats.HealingDealtMultiplier /= 1.2
				t.PseudoStats.SchoolDamageDealtMultiplier.MultiplyMagicSchools(1 / 1.2)
			}})
		})
		s := p.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: p.ForeverAction("priest.talent.power-infusion"), SpellSchool: core.SpellSchoolHoly, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 173}, Cast: core.CastConfig{CD: core.Cooldown{Timer: p.NewTimer(), Duration: 3 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if p.IsOpponent(t) {
				t = &p.Unit
			}
			auras.Get(t).Activate(sim)
		}})
		p.AddMajorCooldown(core.MajorCooldown{Spell: s, Type: core.CooldownTypeDPS})
	}
}
func (p *Priest) registerForeverPrayerOfMending() {
	action := p.ForeverAction("priest.talent.prayer-of-mending")
	heal := p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: action.WithTag(-action.Tag), SpellCode: SpellCode_PriestPrayerOfMending, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: .5, BonusCoefficient: .429})
	charges := int32(0)
	var current *core.Unit
	busy := false
	var auras core.AuraArray
	trigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if busy || s == heal || charges == 0 || r.Damage <= 0 {
			return
		}
		busy = true
		defer func() { busy = false }()
		target := a.Unit
		a.Deactivate(sim)
		heal.CalcAndDealHealing(sim, target, 263, heal.OutcomeHealingCrit)
		charges--
		if charges == 0 {
			return
		}
		var next *core.Unit
		for _, unit := range p.Env.AllUnits {
			if p.IsOpponent(unit) || unit == target || !unit.IsActive() {
				continue
			}
			if next == nil || unit.CurrentHealthPercent() < next.CurrentHealthPercent() {
				next = unit
			}
		}
		if next != nil {
			current = next
			auras.Get(next).Activate(sim)
		}
	}
	auras = p.NewRaidAuraArray(func(t *core.Unit) *core.Aura {
		return t.GetOrRegisterAura(core.Aura{Label: "Forever Prayer of Mending-" + p.Label, ActionID: action, Duration: 30 * time.Second, OnSpellHitTaken: trigger, OnPeriodicDamageTaken: trigger, OnHealTaken: trigger})
	})
	p.PrayerOfMending = p.RegisterSpell(core.SpellConfig{DefenseType: core.DefenseTypeMagic, ActionID: action, SpellCode: SpellCode_PriestPrayerOfMending, SpellSchool: core.SpellSchoolHoly, ProcMask: core.ProcMaskSpellHealing, Flags: SpellFlagPriest | core.SpellFlagHelpful | core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 178}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: p.NewTimer(), Duration: 10 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		if current != nil {
			auras.Get(current).Deactivate(sim)
		}
		current = t
		// Literal observed "up to 5 jumps": initial heal plus five transfers.
		charges = 6
		auras.Get(t).Activate(sim)
	}})
	core.MakePermanent(p.RegisterAura(core.Aura{Label: "Forever Prayer of Mending state", OnReset: func(a *core.Aura, sim *core.Simulation) { current = nil; charges = 0; busy = false }}))
}
