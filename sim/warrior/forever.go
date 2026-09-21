package warrior

import (
	"math"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

// Every ID is a record in forever_changes.json. Secondary interactions retain
// Classic behavior; the 10-rage Tactical Mastery baseline is explicitly predicted.
func (w *Warrior) applyForeverTalents() {
	if w.Forever == nil {
		return
	}
	w.registerForeverRageDecayScenario()
	// Iron Will: client effects 0 (stun) and 1 (fear) are duration modifiers (-3..-15%).
	w.ForeverControlReduction(core.ForeverStun, clientTalent(&w.Character, "warrior.talent.iron-will", 0, -1, w.ForeverValue("warrior.talent.iron-will", 0, 0))/100)
	w.ForeverControlReduction(core.ForeverFear, clientTalent(&w.Character, "warrior.talent.iron-will", 1, -1, w.ForeverValue("warrior.talent.iron-will", 0, 0))/100)
	// Focused Rage: client effect 0 is the rage cost modifier in tenths (-10/-20/-30).
	focusedRage := int32(math.Round(clientTalent(&w.Character, "warrior.talent.focused-rage", 0, -0.1, float64(w.ForeverRank("warrior.talent.focused-rage")))))
	w.OnSpellRegistered(func(s *core.Spell) {
		if s.Cost != nil && s.Flags.Matches(SpellFlagOffensive) {
			s.Cost.FlatModifier -= focusedRage
		}
	})
	if n := w.ForeverRank("warrior.talent.weaponmaster"); n > 0 {
		// Weaponmaster, client per rank: effect 0 the crit chance with axes and polearms, effect 1
		// the armor ignored with maces and staves, effect 2 the extra-attack chance with swords.
		crit := clientTalent(&w.Character, "warrior.talent.weaponmaster", 0, 1, float64(n))
		armorPen := clientTalent(&w.Character, "warrior.talent.weaponmaster", 1, 0.01, .03*float64(n))
		chance := clientTalent(&w.Character, "warrior.talent.weaponmaster", 2, 0.01, float64(n)*.01)
		icdDuration := time.Duration(codeValue(&w.Character, "warrior: Weaponmaster", "extra attack icd_ms", 200, "PREDICTED",
			"the client talent states no internal cooldown; Sword Specialization's 200 ms is used") * float64(time.Millisecond))
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
				s.BonusCritRating += crit * core.CritRatingPerCritChance
			case proto.WeaponType_WeaponTypeMace, proto.WeaponType_WeaponTypeStaff:
				s.BonusArmorPenetration += armorPen
			}
		})
		icd := core.Cooldown{Timer: w.NewTimer(), Duration: icdDuration}
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

// Anger Management's known 30% reduction can be evaluated without inventing
// a Forever base decay rate. Both waiting time and base rage loss are explicit
// scenario inputs, and the loss occurs only before combat starts.
func (w *Warrior) registerForeverRageDecayScenario() {
	wait := w.ForeverParameter("scenario.precombat_rage_wait_seconds", 0)
	rate := w.ForeverParameter("scenario.precombat_rage_loss_per_second", 0)
	if wait <= 0 || rate <= 0 {
		return
	}
	mult := 1.0
	if w.ForeverRank("warrior.talent.anger-management") > 0 {
		mult = .7
	}
	metrics := w.NewRageMetrics(w.ForeverAction("warrior.talent.anger-management"))
	w.RegisterPrepullAction(-time.Duration(wait*float64(time.Second)), func(sim *core.Simulation) {
		var tick func(*core.Simulation)
		last := sim.CurrentTime
		tick = func(sim *core.Simulation) {
			elapsed := (sim.CurrentTime - last).Seconds()
			w.SpendRage(sim, min(w.CurrentRage(), rate*elapsed*mult), metrics)
			last = sim.CurrentTime
			if sim.CurrentTime < 0 {
				core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: min(0, sim.CurrentTime+time.Second), OnAction: tick})
			}
		}
		core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: min(0, sim.CurrentTime+time.Second), OnAction: tick})
	})
}

func (w *Warrior) registerForeverAbilities() {
	if w.Forever == nil {
		return
	}
	w.registerForeverVictoryRush()
	// Bloodthrill uses the existing Overpower window and the caster's own Rend.
	if n := w.ForeverRank("warrior.talent.bloodthrill"); n > 0 {
		// Client effect 0: the proc chance (2-10%). The Overpower window it opens is research data.
		chance := clientTalent(&w.Character, "warrior.talent.bloodthrill", 0, 0.01, .02*float64(n))
		window := time.Duration(codeValue(&w.Character, "warrior: Bloodthrill", "overpower window_s", w.ForeverValue("warrior.talent.bloodthrill", 2, 6), "PROVISIONAL",
			"the client talent states no window; the research value is used") * float64(time.Second))
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Bloodthrill", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.Rend == nil || w.OverpowerAura == nil {
				return
			}
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) && w.Rend.Dot(r.Target).IsActive() && sim.Proc(chance, "Bloodthrill") {
				w.OverpowerAura.Activate(sim)
				w.OverpowerAura.UpdateExpires(sim, sim.CurrentTime+window)
			}
		}}))
	}
	if n := w.ForeverRank("warrior.talent.blood-craze"); n > 0 {
		metric := w.NewHealthMetrics(w.ForeverAction("warrior.talent.blood-craze"))
		// Client: talent effect 0 the health healed (1-3%), talent spell 16487 effect 1 the hit
		// size that triggers it (20% of health), and the heal 16488's duration.
		healed := clientTalent(&w.Character, "warrior.talent.blood-craze", 0, 0.01, .01*float64(n))
		threshold := clientRankValue(&w.Character, 16487, 1, 20) / 100
		hot := w.RegisterAura(core.Aura{Label: "Forever Blood Craze", Duration: clientDuration(&w.Character, 16488, "warrior: Blood Craze", 6*time.Second)})
		generation := 0
		apply := func(sim *core.Simulation) {
			generation++
			current := generation
			hot.Activate(sim)
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 2 * time.Second, NumTicks: 3, OnAction: func(sim *core.Simulation) {
				if generation == current {
					w.GainHealth(sim, w.MaxHealth()*healed/3, metric)
				}
			}})
		}
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Blood Craze Trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidCrit() || r.Damage > w.MaxHealth()*threshold {
				apply(sim)
			}
		}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.SpellCode == SpellCode_WarriorBloodthirst && r.Damage > 0 {
				apply(sim)
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
		mounted := map[*core.Unit]bool{}
		w.RegisterResetEffect(func(sim *core.Simulation) {
			for _, target := range w.Env.Encounter.TargetUnits {
				mounted[target] = w.ForeverParameter("scenario.target_mounted", 0) > 0
			}
		})
		w.RegisterSpell(AnyStance, core.SpellConfig{ActionID: w.ForeverAction("warrior.talent.spearing-strike"), ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			// "Requires Two-Handed Axes, Two-Handed Maces, Polearms, Two-Handed Swords, Staves".
			return w.DistanceFromTarget <= core.MaxMeleeAttackDistance && w.MainHand().HandType == proto.HandType_HandTypeTwoHand
		}, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: SpellFlagOffensive | core.SpellFlagAPL | core.SpellFlagMeleeMetrics, RageCost: core.RageCostOptions{Cost: 15, Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: w.NewTimer(), Duration: 20 * time.Second}}, DamageMultiplier: 1, ThreatMultiplier: 1, BonusCoefficient: 1, CritDamageBonus: w.impale(), ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			mult := .4
			if t.MobType == proto.MobType_MobTypeGiant || t.MobType == proto.MobType_MobTypeDragonkin || mounted[t] {
				mult += .8
			}
			r := s.CalcAndDealDamage(sim, t, mult*w.MHWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeWeaponSpecialHitAndCrit)
			if r.Landed() {
				mounted[t] = false
			}
			if !r.Landed() {
				s.IssueRefund(sim)
			}
		}})
	}
	// Charge/Intercept use their Classic spell IDs and rage/cooldown counterparts.
	for _, charge := range []bool{true, false} {
		// Charge and Intercept at the rank the warrior knows; not registered before it is learned.
		family := "Intercept"
		if charge {
			family = "Charge"
		}
		rank, rankID := w.trainerRank(family)
		if rank == 0 {
			continue
		}
		id := core.ActionID{SpellID: rankID}
		stance := BerserkerStance
		cd := 30*time.Second - time.Duration(w.ForeverValue("warrior.talent.improved-intercept", 0, 0))*time.Second
		cost := 10.
		gain := 0.
		if charge {
			stance = BattleStance
			if w.ForeverRank("warrior.talent.vanguard") > 0 {
				stance |= DefensiveStance
			}
			cd = 15 * time.Second
			cost = 0
			gain = chargeRage[rank-1] + w.ForeverValue("warrior.talent.improved-charge", 0, 0)
		}
		metrics := w.NewRageMetrics(id)
		isCharge := charge
		var timer *core.Timer
		if cd > 0 {
			timer = w.NewTimer()
		}
		config := core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, RageCost: core.RageCostOptions{Cost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: cd}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return (!isCharge || sim.CurrentTime <= 0) && w.DistanceFromTarget >= 8
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			w.DistanceFromTarget = 0
			w.AddRage(sim, gain, metrics)
			t.ForeverControlAura("Charge/Intercept", id, core.ForeverStun, 3*time.Second).Activate(sim)
		}}
		if !charge {
			// Intercept: "Charge an enemy, causing 25/45/65 damage and stunning it for 3 sec."
			// An offensive ability, so Focused Rage reduces its cost; the stun needs the hit.
			damage := interceptDamage[rank-1]
			config.SpellSchool, config.DefenseType, config.ProcMask = core.SpellSchoolPhysical, core.DefenseTypeMelee, core.ProcMaskMeleeMHSpecial
			config.Flags |= SpellFlagOffensive | core.SpellFlagMeleeMetrics
			config.DamageMultiplier, config.ThreatMultiplier, config.CritDamageBonus = 1, 1, w.impale()
			config.ApplyEffects = func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
				w.DistanceFromTarget = 0
				result := s.CalcAndDealDamage(sim, t, damage, s.OutcomeMeleeSpecialHitAndCrit)
				if result.Landed() {
					t.ForeverControlAura("Charge/Intercept", id, core.ForeverStun, 3*time.Second).Activate(sim)
				}
			}
		}
		w.RegisterSpell(stance, config)
	}
	if w.ForeverRank("warrior.talent.concussion-blow") > 0 {
		w.foreverControl("warrior.talent.concussion-blow", core.ActionID{SpellID: 12809}, AnyStance, 10, 45*time.Second, 5*time.Second, core.ForeverStun)
	}
	if w.ForeverRank("warrior.talent.piercing-howl") > 0 {
		w.foreverControl("warrior.talent.piercing-howl", core.ActionID{SpellID: 12323}, AnyStance, 10, 0, 6*time.Second, core.ForeverSnare)
	}
	if w.Level >= levelDisarm {
		w.foreverControl("warrior.talent.improved-disarm", core.ActionID{SpellID: 676}, DefensiveStance, 20, 60*time.Second-time.Duration(w.ForeverValue("warrior.talent.improved-disarm", 0, 0))*time.Second, 10*time.Second, core.ForeverDisarm)
	}
	if w.ForeverRank("warrior.talent.improved-hamstring") > 0 && w.Hamstring != nil {
		core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Improved Hamstring", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if w.Hamstring.IsEqual(s) && r.Landed() && sim.Proc(w.ForeverValue("warrior.talent.improved-hamstring", 0, 0)/100, "Improved Hamstring") {
				r.Target.ForeverControlAura("Improved Hamstring", w.ForeverAction("warrior.talent.improved-hamstring"), core.ForeverRoot, 5*time.Second).Activate(sim)
			}
		}}))
	}
	if _, shieldBashID := w.trainerRank("Shield Bash"); w.ForeverRank("warrior.talent.improved-shield-bash") > 0 && shieldBashID != 0 {
		w.foreverControl("warrior.talent.improved-shield-bash", core.ActionID{SpellID: shieldBashID}, BattleStance|DefensiveStance, 10, 12*time.Second, 3*time.Second, core.ForeverSilence)
	}
}
func (w *Warrior) foreverControl(record string, id core.ActionID, stance Stance, cost float64, cd, duration time.Duration, kind core.ForeverControlKind) {
	var timer *core.Timer
	if cd > 0 {
		timer = w.NewTimer()
	}
	w.RegisterSpell(stance, core.SpellConfig{ActionID: id, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
		if record == "warrior.talent.piercing-howl" {
			return true
		}
		return w.DistanceFromTarget <= core.MaxMeleeAttackDistance && (record != "warrior.talent.improved-shield-bash" || w.OffHand().WeaponType == proto.WeaponType_WeaponTypeShield)
	}, Flags: core.SpellFlagAPL | SpellFlagOffensive, RageCost: core.RageCostOptions{Cost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: cd}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		if record == "warrior.talent.piercing-howl" {
			for _, enemy := range w.Env.Encounter.TargetUnits {
				if absForeverDistance(enemy.DistanceFromTarget-w.DistanceFromTarget) <= 10 {
					enemy.ForeverSnareAura(record, id, duration, .5).Activate(sim)
				}
			}
			return
		}
		if record == "warrior.talent.improved-shield-bash" {
			t.ForeverInterruptSchool(sim, 6*time.Second)
		}
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
	kill := func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if r.Damage > 0 && r.Target.ForeverEnemyDead() {
			a.Activate(sim)
		}
	}
	core.MakePermanent(w.RegisterAura(core.Aura{Label: "Forever Victory Rush Kill Trigger", OnSpellHitDealt: kill, OnPeriodicDamageDealt: kill}))
	w.RegisterResetEffect(func(sim *core.Simulation) {
		if w.ForeverParameter("recent_kill_at_pull", 0) > 0 {
			a.Activate(sim)
		}
		interval := w.ForeverParameter("kill_interval_seconds", 0)
		if interval > 0 {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Duration(interval * float64(time.Second)), OnAction: func(sim *core.Simulation) { a.Activate(sim) }})
		}
	})
	w.RegisterSpell(AnyStance, core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL | SpellFlagOffensive, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
		return a.IsActive() && w.DistanceFromTarget <= core.MaxMeleeAttackDistance
	}, DamageMultiplier: 1, ThreatMultiplier: 1, CritDamageBonus: w.impale(), ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		a.Deactivate(sim)
		// Client (402927): "causing 15% of Attack Power damage".
		s.CalcAndDealDamage(sim, t, .15*s.MeleeAttackPower(t), s.OutcomeMeleeSpecialHitAndCrit)
	}})
}
