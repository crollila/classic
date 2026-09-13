package warrior

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"time"
)

// Every ID is a record in forever_changes.json. Secondary interactions retain
// Classic behavior; the 10-rage Tactical Mastery baseline is explicitly predicted.
func (w *Warrior) applyForeverTalents() {
	if w.Forever == nil {
		return
	}
	w.ForeverControlReduction(core.ForeverStun, w.ForeverValue("warrior.talent.iron-will", 0, 0)/100)
	w.ForeverControlReduction(core.ForeverFear, w.ForeverValue("warrior.talent.iron-will", 0, 0)/100)
	w.OnSpellRegistered(func(s *core.Spell) {
		if s.Cost != nil && s.Flags.Matches(SpellFlagOffensive) {
			s.Cost.FlatModifier -= w.ForeverRank("warrior.talent.focused-rage")
		}
	})
	if n := w.ForeverRank("warrior.talent.weaponmaster"); n > 0 {
		chance := float64(n) * .01
		w.OnSpellRegistered(func(s *core.Spell) {
			if !s.ProcMask.Matches(core.ProcMaskMelee) {
				return
			}
			weapon := w.MainHand().WeaponType
			if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
				weapon = w.OffHand().WeaponType
			}
			switch weapon {
			case proto.WeaponType_WeaponTypeAxe, proto.WeaponType_WeaponTypePolearm:
				s.BonusCritRating += float64(n)
			case proto.WeaponType_WeaponTypeMace, proto.WeaponType_WeaponTypeStaff:
				s.BonusArmorPenetration += .03 * float64(n)
			}
		})
		icd := core.Cooldown{Timer: w.NewTimer(), Duration: 200 * time.Millisecond}
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Weaponmaster", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			weapon := w.MainHand().WeaponType
			if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
				weapon = w.OffHand().WeaponType
			}
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) && weapon == proto.WeaponType_WeaponTypeSword && icd.IsReady(sim) && sim.Proc(chance, "Weaponmaster") {
				icd.Use(sim)
				w.AutoAttacks.ExtraMHAttack(sim, 1, w.ForeverAction("warrior.talent.weaponmaster"), s.ActionID)
			}
		}}))
	}
}

func (w *Warrior) registerForeverAbilities() {
	if w.Forever == nil {
		return
	}
	w.registerForeverVictoryRush()
	// Bloodthrill uses the existing Overpower window and the caster's own Rend.
	if n := w.ForeverRank("warrior.talent.bloodthrill"); n > 0 {
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Bloodthrill", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) && w.Rend.Dot(r.Target).IsActive() && sim.Proc(.02*float64(n), "Bloodthrill") {
				w.OverpowerAura.Activate(sim)
				w.OverpowerAura.UpdateExpires(sim, sim.CurrentTime+6*time.Second)
			}
		}}))
	}
	if n := w.ForeverRank("warrior.talent.blood-craze"); n > 0 {
		metric := w.NewHealthMetrics(w.ForeverAction("warrior.talent.blood-craze"))
		hot := w.RegisterAura(core.Aura{Label: "Forever Blood Craze", Duration: 6 * time.Second})
		var next time.Duration
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Blood Craze Trigger", OnReset: func(a *core.Aura, sim *core.Simulation) {
			next = 0
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 2 * time.Second, OnAction: func(sim *core.Simulation) {
				if hot.IsActive() && sim.CurrentTime >= next {
					w.GainHealth(sim, w.MaxHealth()*.01*float64(n)/3, metric)
				}
			}})
		}, OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidCrit() || r.Damage > w.MaxHealth()*.2 {
				hot.Activate(sim)
				next = sim.CurrentTime
			}
		}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.SpellCode == SpellCode_WarriorBloodthirst && r.Damage > 0 {
				hot.Activate(sim)
				next = sim.CurrentTime
			}
		}}))
	}
	if w.ForeverRank("warrior.talent.raging-blows") > 0 {
		oh := w.RegisterSpell(AnyStance, core.SpellConfig{ActionID: w.ForeverAction("warrior.talent.raging-blows"), SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeOHSpecial, Flags: SpellFlagOffensive | core.SpellFlagMeleeMetrics, DamageMultiplier: 1, ThreatMultiplier: 1.25, BonusCoefficient: 1, CritDamageBonus: w.impale(), ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			s.CalcAndDealDamage(sim, t, w.OHNormalizedWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeWeaponSpecialHitAndCrit)
		}})
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Raging Blows", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.SpellCode == SpellCode_WarriorWhirlwind && s.ProcMask.Matches(core.ProcMaskMeleeMH) && w.HasOHWeapon() {
				oh.Cast(sim, r.Target)
			}
		}}))
	}
	if w.ForeverRank("warrior.talent.spearing-strike") > 0 {
		w.RegisterSpell(AnyStance, core.SpellConfig{ActionID: w.ForeverAction("warrior.talent.spearing-strike"), SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: SpellFlagOffensive | core.SpellFlagAPL | core.SpellFlagMeleeMetrics, RageCost: core.RageCostOptions{Cost: 15, Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: w.NewTimer(), Duration: 20 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 1, CritDamageBonus: w.impale(), ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			mult := .4
			if t.MobType == proto.MobType_MobTypeGiant || t.MobType == proto.MobType_MobTypeDragonkin {
				mult += .8
			}
			r := s.CalcAndDealDamage(sim, t, mult*w.MHWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeWeaponSpecialHitAndCrit)
			if !r.Landed() {
				s.IssueRefund(sim)
			}
		}})
	}
	// Charge/Intercept use their Classic spell IDs and rage/cooldown counterparts.
	for _, charge := range []bool{true, false} {
		id := core.ActionID{SpellID: 20252}
		stance := BerserkerStance
		cd := 30*time.Second - time.Duration(w.ForeverValue("warrior.talent.improved-intercept", 0, 0))*time.Second
		cost := 10.
		gain := 0.
		if charge {
			id = core.ActionID{SpellID: 11578}
			stance = BattleStance
			if w.ForeverRank("warrior.talent.vanguard") > 0 {
				stance |= DefensiveStance
			}
			cd = 15 * time.Second
			cost = 0
			gain = 15 + w.ForeverValue("warrior.talent.improved-charge", 0, 0)
		}
		metrics := w.NewRageMetrics(id)
		isCharge := charge
		var timer *core.Timer
		if cd > 0 {
			timer = w.NewTimer()
		}
		w.RegisterSpell(stance, core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, RageCost: core.RageCostOptions{Cost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: cd}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return (!isCharge || sim.CurrentTime <= 0) && w.DistanceFromTarget >= 8
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			w.DistanceFromTarget = 0
			w.AddRage(sim, gain, metrics)
			t.ForeverControlAura("Charge/Intercept", id, core.ForeverStun, 3*time.Second).Activate(sim)
		}})
	}
	if w.ForeverRank("warrior.talent.concussion-blow") > 0 {
		w.foreverControl("warrior.talent.concussion-blow", core.ActionID{SpellID: 12809}, AnyStance, 10, 45*time.Second, 5*time.Second, core.ForeverStun)
	}
	if w.ForeverRank("warrior.talent.piercing-howl") > 0 {
		w.foreverControl("warrior.talent.piercing-howl", core.ActionID{SpellID: 12323}, AnyStance, 10, 0, 6*time.Second, core.ForeverSnare)
	}
	w.foreverControl("warrior.talent.improved-disarm", core.ActionID{SpellID: 676}, DefensiveStance, 20, 60*time.Second-time.Duration(w.ForeverValue("warrior.talent.improved-disarm", 0, 0))*time.Second, 10*time.Second, core.ForeverDisarm)
	if w.ForeverRank("warrior.talent.improved-hamstring") > 0 {
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Improved Hamstring", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.Hamstring.IsEqual(s) && r.Landed() && sim.Proc(w.ForeverValue("warrior.talent.improved-hamstring", 0, 0)/100, "Improved Hamstring") {
				r.Target.ForeverControlAura("Improved Hamstring", w.ForeverAction("warrior.talent.improved-hamstring"), core.ForeverRoot, 5*time.Second).Activate(sim)
			}
		}}))
	}
	if w.ForeverRank("warrior.talent.improved-shield-bash") > 0 {
		w.foreverControl("warrior.talent.improved-shield-bash", core.ActionID{SpellID: 1672}, BattleStance|DefensiveStance, 10, 12*time.Second, 3*time.Second, core.ForeverSilence)
	}
}
func (w *Warrior) foreverControl(record string, id core.ActionID, stance Stance, cost float64, cd, duration time.Duration, kind core.ForeverControlKind) {
	var timer *core.Timer
	if cd > 0 {
		timer = w.NewTimer()
	}
	w.RegisterSpell(stance, core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL | SpellFlagOffensive, RageCost: core.RageCostOptions{Cost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: cd}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		if record == "warrior.talent.improved-shield-bash" && !sim.Proc(w.ForeverValue(record, 0, 0)/100, "Improved Shield Bash") {
			return
		}
		t.ForeverControlAura(record, id, kind, duration).Activate(sim)
	}})
}

func absForeverDistance(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// PREDICTED TBC analogue. Kill events are supplied as a visible scenario, never
// inferred from a boss's virtual remaining health in a duration-based encounter.
func (w *Warrior) registerForeverVictoryRush() {
	if !w.HasForeverMechanic("warrior.baseline.victory-rush") {
		return
	}
	id := core.ActionID{SpellID: 34428}
	a := w.RegisterAura(core.Aura{Label: "Victorious", ActionID: id, Duration: 20 * time.Second})
	w.RegisterResetEffect(func(sim *core.Simulation) {
		if w.ForeverParameter("recent_kill_at_pull", 0) > 0 {
			a.Activate(sim)
		}
		interval := w.ForeverParameter("kill_interval_seconds", 0)
		if interval > 0 {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Duration(interval * float64(time.Second)), OnAction: func(sim *core.Simulation) { a.Activate(sim) }})
		}
	})
	w.RegisterSpell(AnyStance, core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL | SpellFlagOffensive, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return a.IsActive() }, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		a.Deactivate(sim)
		s.CalcAndDealDamage(sim, t, .45*s.MeleeAttackPower(t), s.OutcomeMeleeSpecialHitAndCrit)
	}})
}
