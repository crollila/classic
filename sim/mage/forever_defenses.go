package mage

import (
	"github.com/wowsims/classic/sim/core"
	"time"
)

func (m *Mage) registerForeverDefenses() {
	if m.Forever == nil {
		return
	}
	f := m.foreverState
	val := func(id string, index int) float64 { return m.ForeverValue("mage.talent."+id, index, 0) }
	// The rank table is Classic; the observed first-rank barrier overrides it.
	amounts := []float64{0, 431, 549, 678, 818}
	if m.ForeverRank("mage.talent.ice-barrier") > 0 {
		shield := core.NewForeverAbsorb(&m.Unit, "Forever Ice Barrier", m.ForeverAction("mage.talent.ice-barrier"), time.Minute, 0)
		for i, s := range m.IceBarrier {
			if s == nil {
				continue
			}
			i, s := i, s
			old := s.ApplyEffects
			s.ApplyEffects = func(sim *core.Simulation, t *core.Unit, sp *core.Spell) {
				old(sim, t, sp)
				shield.Apply(sim, amounts[i], false)
			}
		}
		a := shield.Aura
		oldGain, oldExpire := a.OnGain, a.OnExpire
		a.OnGain = func(a *core.Aura, sim *core.Simulation) {
			if oldGain != nil {
				oldGain(a, sim)
			}
			for _, sp := range m.Spellbook {
				sp.PushbackReduction += 1
			}
		}
		a.OnExpire = func(a *core.Aura, sim *core.Simulation) {
			if oldExpire != nil {
				oldExpire(a, sim)
			}
			for _, sp := range m.Spellbook {
				sp.PushbackReduction -= 1
			}
		}
	}
	if m.ForeverRank("mage.talent.ice-block") > 0 {
		a := m.ForeverImmobileAura("Forever Ice Block", m.ForeverAction("mage.talent.ice-block"), 10*time.Second)
		m.OnSpellRegistered(func(s *core.Spell) {
			if !s.Flags.Matches(core.SpellFlagAPL) {
				return
			}
			old := s.ExtraCastCondition
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool { return !a.IsActive() && (old == nil || old(sim, t)) }
		})
		immunity := m.ForeverControlImmunityAura("Forever Ice Block immunity", m.ForeverAction("mage.talent.ice-block"), []core.ForeverControlKind{core.ForeverRoot, core.ForeverFear, core.ForeverSilence, core.ForeverStun, core.ForeverCharm, core.ForeverSleep, core.ForeverIncapacitate}, 10*time.Second)
		m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if a.IsActive() {
				r.Damage = 0
			}
		})
		m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: m.ForeverAction("mage.talent.ice-block"), SpellSchool: core.SpellSchoolFrost, Flags: core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 15}, Cast: core.CastConfig{CD: core.Cooldown{Timer: m.NewTimer(), Duration: 5 * time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			m.ForeverInterrupt(sim)
			a.Activate(sim)
			immunity.Activate(sim)
		}})
	}
	// Baseline Frost Nova / Cone of Cold make all root and chill talents playable.
	// Their known Classic highest-rank values are retained; prediction concerns the
	// otherwise undocumented Forever baseline, not fabricated spell identifiers.
	nova := m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Frost Nova-"+m.Label, core.ActionID{SpellID: 10230}, core.ForeverRoot, 8*time.Second)
	})
	m.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 10230}, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{FlatCost: 205}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 25*time.Second - time.Duration(val("improved-frost-nova", 0)*float64(time.Second))}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: .135,
		ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return m.DistanceFromTarget <= 10*(1+val("arctic-reach", 0)/100)
		},
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			for _, t := range sim.Encounter.TargetUnits {
				r := s.CalcAndDealDamage(sim, t, sim.Roll(71, 80), s.OutcomeMagicHitAndCrit)
				if r.Landed() {
					nova.Get(t).Activate(sim)
					if nova.Get(t).IsActive() {
						a := f.frozen.Get(t)
						a.Activate(sim)
						a.UpdateExpires(sim, sim.CurrentTime+8*time.Second)
					}
				}
			}
		},
	})
	m.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 10161}, SpellSchool: core.SpellSchoolFrost, DefenseType: core.DefenseTypeMagic, ProcMask: core.ProcMaskSpellDamage, Flags: SpellFlagMage | SpellFlagChillSpell | core.SpellFlagAPL,
		ManaCost: core.ManaCostOptions{FlatCost: 555}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 10 * time.Second}}, DamageMultiplier: 1 + val("improved-cone-of-cold", 0)/100, ThreatMultiplier: 1, BonusCoefficient: .135,
		ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return m.DistanceFromTarget <= 10*(1+val("arctic-reach", 0)/100)
		},
		ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			for _, t := range sim.Encounter.TargetUnits {
				s.CalcAndDealDamage(sim, t, sim.Roll(335, 365), s.OutcomeMagicHitAndCrit)
			}
		},
	})
	// Counterspell's Classic interruption now operates on encounter spellcasts.
	silences := m.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
		return t.ForeverControlAura("Forever Improved Counterspell-"+m.Label, m.ForeverAction("mage.talent.improved-counterspell"), core.ForeverSilence, time.Duration(val("improved-counterspell", 0)*float64(time.Second)))
	})
	oldCounter := m.Counterspell.ApplyEffects
	m.Counterspell.ApplyEffects = func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		oldCounter(sim, t, s)
		t.ForeverInterruptSchool(sim, 10*time.Second)
		if val("improved-counterspell", 0) > 0 {
			silences.Get(t).Activate(sim)
		}
	}
	for _, ward := range []struct {
		id     int32
		name   string
		school core.SpellSchool
		talent string
	}{
		{10223, "Fire Ward", core.SpellSchoolFire, "improved-fire-ward"}, {10225, "Frost Ward", core.SpellSchoolFrost, "frost-warding"},
	} {
		ward := ward
		action := core.ActionID{SpellID: ward.id}
		var shield *core.ForeverAbsorb
		reflect := m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action.WithTag(1), SpellSchool: ward.school, Flags: core.SpellFlagPassiveSpell, DamageMultiplier: 1, ThreatMultiplier: 1})
		m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			chance := val(ward.talent, 0)
			if ward.talent == "frost-warding" {
				chance = val(ward.talent, 1)
			}
			if shield.Aura.IsActive() && s.SpellSchool.Matches(ward.school) && sim.Proc(chance/100, ward.name) {
				damage := r.Damage
				r.Damage = 0
				reflect.CalcAndDealDamage(sim, s.Unit, damage, reflect.OutcomeAlwaysHit)
			}
		})
		shield = core.NewForeverAbsorb(&m.Unit, "Forever "+ward.name, action, 30*time.Second, ward.school)
		m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: action, SpellSchool: ward.school, Flags: SpellFlagMage | core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 320}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: m.NewTimer(), Duration: 30 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { shield.Apply(sim, 920, false) }})
	}
	// Mana Shield converts absorbed Physical damage into mana spending.
	manaShield := m.RegisterAura(core.Aura{Label: "Forever Mana Shield", ActionID: core.ActionID{SpellID: 10193}, Duration: time.Minute})
	remaining := 0.0
	manaMetrics := m.NewManaMetrics(core.ActionID{SpellID: 10193})
	m.AddDynamicDamageTakenModifier(func(sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !manaShield.IsActive() || !s.SpellSchool.Matches(core.SpellSchoolPhysical) {
			return
		}
		ratio := 2 * (1 - val("arcane-shielding", 0)/100)
		absorb := min(r.Damage, min(remaining, m.CurrentMana()/ratio))
		r.Damage -= absorb
		remaining -= absorb
		m.SpendMana(sim, absorb*ratio, manaMetrics)
		if remaining <= 0 || m.CurrentMana() <= 0 {
			manaShield.Deactivate(sim)
		}
	})
	m.RegisterSpell(core.SpellConfig{ProcMask: core.ProcMaskEmpty, ActionID: core.ActionID{SpellID: 10193}, SpellSchool: core.SpellSchoolArcane, Flags: SpellFlagMage | core.SpellFlagAPL | core.SpellFlagHelpful, ManaCost: core.ManaCostOptions{FlatCost: 180}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { remaining = 570; manaShield.Activate(sim) }})
	// A kill is detected only with health-backed targets; fixed-duration bosses do
	// not fabricate kills. The next Fire Blast consumes its own 20-second buff.
	wake := m.RegisterAura(core.Aura{Label: "Forever Wake of Fire", ActionID: m.ForeverAction("mage.talent.wake-of-fire"), Duration: 20 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range m.FireBlast {
			if s != nil {
				s.BonusCritRating += val("wake-of-fire", 2) * core.SpellCritRatingPerCritChance
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range m.FireBlast {
			if s != nil {
				s.BonusCritRating -= val("wake-of-fire", 2) * core.SpellCritRatingPerCritChance
			}
		}
	}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
		if s.SpellCode == SpellCode_MageFireBlast && a.RemainingDuration(sim) != a.Duration {
			a.Deactivate(sim)
		}
	}})
	killTrigger := func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if m.ForeverRank("mage.talent.wake-of-fire") > 0 && r.Target.HasHealthBar() && r.Target.CurrentHealth() <= 0 && r.Damage > 0 && r.Target.Level >= m.Level-8 {
			wake.Activate(sim)
		}
	}
	core.MakePermanent(m.RegisterAura(core.Aura{Label: "Forever Wake of Fire trigger", OnSpellHitDealt: killTrigger, OnPeriodicDamageDealt: killTrigger}))
}
