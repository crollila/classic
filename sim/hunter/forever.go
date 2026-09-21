package hunter

import (
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (h *Hunter) applyForeverTalents() {
	if h.Forever == nil {
		return
	}
	h.ForeverControlReduction(core.ForeverSnare, h.ForeverValue("hunter.talent.surefooted", 1, 0)/100)
	h.ForeverControlReduction(core.ForeverRoot, h.ForeverValue("hunter.talent.surefooted", 1, 0)/100)
	h.OnSpellRegistered(func(s *core.Spell) {
		if s.Flags.Matches(SpellFlagShot) || s.ProcMask.Matches(core.ProcMaskRangedAuto) {
			old := s.ExtraCastCondition
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
				return h.DistanceFromTarget <= 35+2*float64(h.ForeverRank("hunter.talent.hawk-eye")) && (old == nil || old(sim, t))
			}
		}
		if s.ProcMask.Matches(core.ProcMaskMeleeSpecial) {
			s.BonusCritRating += 2 * float64(h.ForeverRank("hunter.talent.savage-strikes"))
		}
		// Predator's Edge: "melee critical strike damage", white swings included.
		if s.ProcMask.Matches(core.ProcMaskMelee) {
			s.CritDamageBonus += h.ForeverValue("hunter.talent.predator-s-edge", 0, 0) / 100
		}
		if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
			s.DamageMultiplier *= 1 + h.ForeverValue("hunter.talent.predator-s-edge", 1, 0)/100
		}
		if s.Cost != nil && (s.Flags.Matches(SpellFlagTrap) || s.ProcMask.Matches(core.ProcMaskMeleeSpecial)) {
			s.Cost.Multiplier -= 30 * h.ForeverRank("hunter.talent.resourcefulness")
		}
		if s.Flags.Matches(SpellFlagTrap) {
			s.BonusHitRating += 5 * float64(h.ForeverRank("hunter.talent.survival-tactics"))
			s.CD.Duration = time.Duration(float64(s.CD.Duration) * (1 - .2*float64(h.ForeverRank("hunter.talent.survivalist-s-discipline"))))
		}
		// Client values: Improved Stings 6/13/20%, Barrage 3/7/10% (not linear per rank).
		if s.SpellCode == SpellCode_HunterSerpentSting {
			s.DamageMultiplier *= 1 + h.ForeverValue("hunter.talent.improved-stings", 0, 0)/100
		}
		// Barrage names Multi-Shot, Aimed Shot and Volley; Sniper Shot has its own code.
		if s.SpellCode == SpellCode_HunterMultiShot || s.SpellCode == SpellCode_HunterAimedShot || s.SpellCode == SpellCode_HunterVolley {
			s.DamageMultiplier *= 1 + h.ForeverValue("hunter.talent.barrage", 0, 0)/100
		}
	})
	// Focused Fire: "all damage you and your pet deal". The hunter half is the core
	// target multiplier; the pet only deals damage while it is active.
	if h.pet != nil && h.ForeverRank("hunter.talent.focused-fire") > 0 {
		h.pet.PseudoStats.DamageDealtMultiplier *= 1 + h.ForeverValue("hunter.talent.focused-fire", 0, 0)/100
	}
	if n := h.ForeverRank("hunter.talent.improved-tracking"); n > 0 {
		// BEST_GUESS assumes the player selects the appropriate trackable type before combat.
		h.Env.RegisterPostFinalizeEffect(func() {
			for _, target := range h.Env.Encounter.Targets {
				if target.MobType != proto.MobType_MobTypeUnknown && target.MobType != proto.MobType_MobTypeMechanical {
					for _, at := range h.AttackTables[target.UnitIndex] {
						at.DamageDealtMultiplier *= 1 + .01*float64(n)
					}
				}
			}
		})
	}
	if h.pet != nil {
		if distance := h.ForeverParameter("scenario.pet_starting_distance", 0); distance > core.MaxMeleeAttackDistance {
			h.pet.ApplyOnPetEnable(func(sim *core.Simulation) {
				h.pet.DistanceFromTarget = distance
				core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: max(0, sim.CurrentTime) + time.Nanosecond, OnAction: func(sim *core.Simulation) {
					if h.pet.IsEnabled() {
						h.pet.MoveTo(core.MaxMeleeAttackDistance, sim)
					}
				}})
			})
		}
		if h.ForeverRank("hunter.talent.bestial-swiftness") > 0 {
			id := h.ForeverAction("hunter.talent.bestial-swiftness")
			h.Env.RegisterPostFinalizeEffect(func() { h.pet.AddMoveSpeedModifier(&id, 1.3) })
		}
		if n := h.ForeverRank("hunter.talent.spirit-bond"); n > 0 {
			id := h.ForeverAction("hunter.talent.spirit-bond")
			hm := h.NewHealthMetrics(id)
			pm := h.pet.NewHealthMetrics(id)
			h.RegisterResetEffect(func(sim *core.Simulation) {
				core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 10 * time.Second, OnAction: func(sim *core.Simulation) {
					if h.pet.IsEnabled() {
						h.GainHealth(sim, h.MaxHealth()*.01*float64(n), hm)
						h.pet.GainHealth(sim, h.pet.MaxHealth()*.01*float64(n), pm)
					}
				}})
			})
		}
	}
}

func (h *Hunter) foreverRegen(label string, fraction float64, duration time.Duration) *core.Aura {
	return h.RegisterAura(core.Aura{Label: label, Duration: duration, OnGain: func(a *core.Aura, sim *core.Simulation) {
		h.PseudoStats.SpiritRegenRateCasting += fraction
		h.UpdateManaRegenRates()
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		h.PseudoStats.SpiritRegenRateCasting -= fraction
		h.UpdateManaRegenRates()
	}})
}
func (h *Hunter) registerForeverAbilities(arcaneTimer *core.Timer) {
	if h.Forever == nil {
		return
	}
	h.registerForeverRapidKilling()
	if h.RapidFire != nil {
		h.RapidFire.CD.Duration -= time.Duration(h.ForeverRank("hunter.talent.rapid-killing")) * time.Minute
	}
	if n := h.ForeverRank("hunter.talent.resourcefulness"); n > 0 {
		a := h.foreverRegen("Resourcefulness", .5, 30*time.Second)
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Resourcefulness Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && sim.Proc(.3*float64(n), "Resourcefulness") {
				a.Activate(sim)
			}
		}}))
	}
	if n := h.ForeverRank("hunter.talent.rapid-recuperation"); n > 0 {
		a := h.foreverRegen("Rapid Recuperation", .25*float64(n), 15*time.Second)
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Rapid Recuperation Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.SpellCode == SpellCode_HunterSerpentSting && r.Landed() {
				a.Activate(sim)
			}
		}}))
	}
	if n := h.ForeverRank("hunter.talent.expose-prey"); n > 0 {
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Expose Prey", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			marked := false
			for _, a := range r.Target.GetAurasWithTag("HuntersMark") {
				marked = marked || a.IsActive()
			}
			if !marked {
				for _, name := range []string{"Hunter's Mark", "Hunters Mark"} {
					a := r.Target.GetAura(name)
					marked = marked || (a != nil && a.IsActive())
				}
			}
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMeleeOrRanged) && marked && sim.Proc(.05*float64(n), "Expose Prey") {
				h.DefensiveState.Activate(sim)
			}
		}}))
	}
	if h.ForeverRank("hunter.talent.lacerating-strikes") > 0 {
		bleed := h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.lacerating-strikes"), ProcMask: core.ProcMaskEmpty, SpellSchool: core.SpellSchoolPhysical, Flags: core.SpellFlagIgnoreResists | core.SpellFlagIgnoreModifiers, DamageMultiplier: 1, ThreatMultiplier: 1, Dot: core.DotConfig{Aura: core.Aura{Label: "Lacerating Strikes"}, NumberOfTicks: 7, TickLength: 3 * time.Second, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
			d.CalcAndDealPeriodicSnapshotDamage(sim, t, d.OutcomeTick)
		}}})
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Lacerating Strikes Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			// 40% of the damage Mongoose Bite did, over 7 ticks. r.Damage already carries
			// every attacker and target modifier, so the bleed ignores both.
			if s.SpellCode == SpellCode_HunterMongooseBite && r.Damage > 0 {
				d := bleed.Dot(r.Target)
				d.SnapshotBaseDamage = r.Damage * h.ForeverValue("hunter.talent.lacerating-strikes", 1, 40) / 100 / 7
				d.SnapshotAttackerMultiplier = 1
				d.Apply(sim)
			}
		}}))
	}
	if h.ForeverRank("hunter.talent.summon-hawk") > 0 {
		h.registerForeverHawks(arcaneTimer)
	}
	if h.ForeverRank("hunter.talent.trueshot-aura") > 0 {
		h.registerForeverTrueshot()
	}
	// Aspect selection is real and mutually exclusive with Classic Hawk.
	for _, kind := range []string{"Beast", "Monkey", "Cheetah", "Pack"} {
		if h.Level < map[string]int32{"Beast": 30, "Monkey": 4, "Cheetah": 20, "Pack": 40}[kind] {
			continue // not learned yet (Forever client trainer levels)
		}
		id := core.ActionID{SpellID: 13161}
		mana := 50.0
		switch kind {
		case "Monkey":
			id.SpellID = 13163
			mana = 20
		case "Cheetah":
			id.SpellID = 5118
			mana = 40
		case "Pack":
			id.SpellID = 13159
			mana = 100
		}
		a := h.RegisterAura(core.Aura{Label: "Aspect of the " + kind, ActionID: id, Duration: core.NeverExpires})
		a.NewExclusiveEffect("Aspect", true, core.ExclusiveEffect{})
		if kind == "Monkey" {
			dodge := 8 + 2*float64(h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey"))
			a.OnGain = func(a *core.Aura, sim *core.Simulation) {
				h.AddStatDynamic(sim, stats.Dodge, dodge)
				if h.pet != nil && h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey") > 0 {
					h.pet.AddStatDynamic(sim, stats.Dodge, dodge*h.ForeverValue("hunter.talent.improved-aspect-of-the-monkey", 1, 0)/100)
				}
			}
			a.OnExpire = func(a *core.Aura, sim *core.Simulation) {
				h.AddStatDynamic(sim, stats.Dodge, -dodge)
				if h.pet != nil && h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey") > 0 {
					h.pet.AddStatDynamic(sim, stats.Dodge, -dodge*h.ForeverValue("hunter.talent.improved-aspect-of-the-monkey", 1, 0)/100)
				}
			}
		}
		if kind == "Cheetah" || kind == "Pack" {
			h.foreverMovementAspect(a, id, kind == "Pack")
		}
		if kind == "Beast" && h.ForeverRank("hunter.talent.deadly-aspects") > 0 {
			quick := h.RegisterAura(core.Aura{Label: "Deadly Beast", Duration: 12 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) { h.MultiplyMeleeSpeed(sim, 1.3) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { h.MultiplyMeleeSpeed(sim, 1/1.3) }})
			a.OnSpellHitDealt = func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
				if s.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && sim.Proc(.02*float64(h.ForeverRank("hunter.talent.deadly-aspects")), "Deadly Beast") {
					quick.Activate(sim)
				}
			}
		}
		h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	if h.ForeverRank("hunter.talent.deterrence") > 0 {
		id := h.ForeverAction("hunter.talent.deterrence")
		a := h.RegisterAura(core.Aura{Label: "Deterrence", ActionID: id, Duration: 10 * time.Second}).AttachStatsBuff(stats.Stats{stats.Dodge: 25, stats.Parry: 25})
		h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: h.NewTimer(), Duration: time.Duration(float64(5*time.Minute) * (1 - .2*float64(h.ForeverRank("hunter.talent.survivalist-s-discipline"))))}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	if h.ForeverRank("hunter.talent.counterattack") > 0 {
		ready := h.RegisterAura(core.Aura{Label: "Counterattack Ready", Duration: 5 * time.Second})
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Counterattack Trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidParry() {
				ready.Activate(sim)
			}
		}}))
		bonus, mana := counterattackRank(h.Level)
		h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.counterattack"), SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | SpellFlagStrike, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: h.NewTimer(), Duration: 5 * time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return ready.IsActive() && h.DistanceFromTarget <= core.MaxMeleeAttackDistance
		}, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			ready.Deactivate(sim)
			r := s.CalcAndDealDamage(sim, t, bonus+.5*h.MHWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeSpecialNoBlockDodgeParry)
			if r.Landed() {
				t.ForeverControlAura("Counterattack", s.ActionID, core.ForeverRoot, 5*time.Second).Activate(sim)
			}
		}})
	}
	h.registerForeverPetUtility()
	h.registerForeverControls()
	h.registerForeverStings()
	if h.Level >= 30 {
		h.registerForeverFeignDeath()
	}
}

// Forever client Summon Hawk ranks: learned level, damage per hit, mana.
var summonHawkRanks = []struct {
	level        int32
	damage, mana float64
}{{25, 32, 80}, {36, 47, 105}, {48, 85, 135}, {60, 108, 190}}

// summonHawkRank is the highest Summon Hawk rank known at the level; the talent grants
// rank 1 whenever it is taken.
func summonHawkRank(level int32) (damage, mana float64) {
	damage, mana = summonHawkRanks[0].damage, summonHawkRanks[0].mana
	for _, r := range summonHawkRanks {
		if r.level <= level {
			damage, mana = r.damage, r.mana
		}
	}
	return damage, mana
}

// Forever client Trueshot Aura ranks: learned level, ranged attack power, mana. The
// client's rank 5 (level 60) really reads 50, below rank 4's 75.
var trueshotAuraRanks = []struct {
	level     int32
	rap, mana float64
}{{25, 30, 180}, {32, 40, 245}, {40, 50, 325}, {50, 75, 425}, {60, 50, 525}}

func trueshotAuraRank(level int32) (rap, mana float64) {
	rap, mana = trueshotAuraRanks[0].rap, trueshotAuraRanks[0].mana
	for _, r := range trueshotAuraRanks {
		if r.level <= level {
			rap, mana = r.rap, r.mana
		}
	}
	return rap, mana
}

// Forever client Counterattack ranks: learned level, flat bonus on 50% weapon damage, mana.
var counterattackRanks = []struct {
	level       int32
	bonus, mana float64
}{{30, 13, 30}, {30, 20, 45}, {42, 35, 65}, {54, 55, 85}}

func counterattackRank(level int32) (bonus, mana float64) {
	bonus, mana = counterattackRanks[0].bonus, counterattackRanks[0].mana
	for _, r := range counterattackRanks {
		if r.level <= level {
			bonus, mana = r.bonus, r.mana
		}
	}
	return bonus, mana
}

// PREDICTED repeated hawk attacks use a standard pet's 2s swing. Per-rank damage and
// mana, 18s duration, two-hawk cap, cooldown sharing and pet-talent scaling are sourced.
func (h *Hunter) registerForeverHawks(timer *core.Timer) {
	hawkDamage, hawkMana := summonHawkRank(h.Level)
	attacks := make([]*core.Spell, 2)
	active := make([]*core.Aura, 2)
	targets := make([]*core.Unit, 2)
	for i := 0; i < 2; i++ {
		slot := i
		id := h.ForeverAction("hunter.talent.summon-hawk")
		active[i] = h.RegisterAura(core.Aura{Label: fmt.Sprintf("Hawk %d", i+1), Duration: 18 * time.Second})
		childID := id
		childID.Tag = -id.Tag*10 - int32(i)
		attacks[i] = h.RegisterSpell(core.SpellConfig{ActionID: childID, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskEmpty, Flags: core.SpellFlagIgnoreAttackerModifiers, DamageMultiplier: 1 + .03*float64(h.ForeverRank("hunter.talent.unleashed-fury")), BonusCritRating: 5 + 2*float64(h.ForeverRank("hunter.talent.ferocity")), ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			s.CalcAndDealDamage(sim, t, hawkDamage, s.OutcomeMeleeSpecialHitAndCrit)
		}})
		h.RegisterResetEffect(func(sim *core.Simulation) {
			targets[slot] = nil
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: 2 * time.Second, OnAction: func(sim *core.Simulation) {
				if active[slot].IsActive() && targets[slot] != nil {
					attacks[slot].Cast(sim, targets[slot])
				}
			}})
		})
	}
	h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.summon-hawk"), Flags: core.SpellFlagAPL, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return h.DistanceFromTarget <= 35 }, ManaCost: core.ManaCostOptions{FlatCost: hawkMana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: timer, Duration: 6 * time.Second}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		slot := 0
		if active[0].IsActive() && (!active[1].IsActive() || active[0].RemainingDuration(sim) > active[1].RemainingDuration(sim)) {
			slot = 1
		}
		targets[slot] = t
		active[slot].Activate(sim)
		attacks[slot].Cast(sim, t)
	}})
}
func (h *Hunter) registerForeverTrueshot() {
	id := h.ForeverAction("hunter.talent.trueshot-aura")
	rap, mana := trueshotAuraRank(h.Level)
	auras := []*core.Aura{}
	for _, a := range h.Party.Players {
		c := a.GetCharacter()
		aura := c.GetAura("Forever Trueshot Aura")
		if aura == nil {
			aura = c.RegisterAura(core.Aura{Label: "Forever Trueshot Aura", ActionID: id, Duration: 30 * time.Minute}).AttachStatsBuff(stats.Stats{stats.RangedAttackPower: rap})
		}
		auras = append(auras, aura)
	}
	h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		for _, a := range auras {
			if a.Unit.DistanceFromTarget-h.DistanceFromTarget <= 45 && h.DistanceFromTarget-a.Unit.DistanceFromTarget <= 45 {
				a.Activate(sim)
			}
		}
	}})
}
func (h *Hunter) registerForeverPetUtility() {
	if h.pet == nil {
		return
	}
	id := core.ActionID{SpellID: 982}
	if h.Level < 10 {
		return
	}
	h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 1367, Multiplier: 100 - 20*h.ForeverRank("hunter.talent.improved-revive-pet")}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 10*time.Second - time.Duration(3*h.ForeverRank("hunter.talent.improved-revive-pet"))*time.Second}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return !h.pet.IsEnabled() }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		h.pet.Enable(sim, h.pet)
		h.pet.RemoveHealth(sim, h.pet.CurrentHealth())
		h.pet.GainHealth(sim, h.pet.MaxHealth()*(.15+.15*float64(h.ForeverRank("hunter.talent.improved-revive-pet"))), h.pet.NewHealthMetrics(id))
	}})
	if h.ForeverRank("hunter.talent.intimidation") > 0 {
		id := h.ForeverAction("hunter.talent.intimidation")
		// Classic client threat value: https://www.wowhead.com/classic/spell=24394/intimidation
		threat := h.pet.RegisterSpell(core.SpellConfig{ActionID: core.ActionID{SpellID: 24394}, Flags: core.SpellFlagPassiveSpell, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			s.SpellMetrics[t.UnitIndex].TotalThreat += 580 * h.pet.PseudoStats.ThreatMultiplier
		}})
		a := h.pet.RegisterAura(core.Aura{Label: "Intimidation", Duration: 30 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) { h.pet.AddStatDynamic(sim, stats.MeleeCrit, 100) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { h.pet.AddStatDynamic(sim, stats.MeleeCrit, -100) }, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) {
				threat.Cast(sim, r.Target)
				r.Target.ForeverControlAura("Intimidation", id, core.ForeverStun, 3*time.Second).Activate(sim)
				a.Deactivate(sim)
			}
		}})
		h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 84}, Cast: core.CastConfig{CD: core.Cooldown{Timer: h.NewTimer(), Duration: time.Minute}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	if h.Level >= 12 {
		h.registerForeverMendPet()
	}
}

func (h *Hunter) registerForeverControls() {
	core.MakePermanent(h.RegisterAura(core.Aura{Label: "Forever Hunter Control", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() {
			return
		}
		if s.SpellCode == SpellCode_HunterWingClip && sim.Proc(.07*float64(h.ForeverRank("hunter.talent.improved-wing-clip")), "Improved Wing Clip") {
			r.Target.ForeverControlAura("Improved Wing Clip", h.ForeverAction("hunter.talent.improved-wing-clip"), core.ForeverRoot, 5*time.Second).Activate(sim)
		}
		if s.Flags.Matches(SpellFlagTrap) && h.ForeverRank("hunter.talent.entrapment") > 0 {
			r.Target.ForeverControlAura("Entrapment", h.ForeverAction("hunter.talent.entrapment"), core.ForeverRoot, time.Duration(h.ForeverRank("hunter.talent.entrapment"))*time.Second).Activate(sim)
		}
	}}))
	for _, scatter := range []bool{false, true} {
		if scatter && h.ForeverRank("hunter.talent.scatter-shot") == 0 {
			continue
		}
		if !scatter && h.Level < 8 {
			continue // Concussive Shot is learned at level 8
		}
		id := core.ActionID{SpellID: 5116}
		cost := 84.
		cd := 12 * time.Second
		if scatter {
			id = h.ForeverAction("hunter.talent.scatter-shot")
			cd = 30 * time.Second
		}
		h.RegisterSpell(core.SpellConfig{ActionID: id, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeRanged, ProcMask: core.ProcMaskRangedSpecial, Flags: core.SpellFlagAPL | SpellFlagShot, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			if scatter {
				return h.DistanceFromTarget <= 15
			}
			return h.DistanceFromTarget >= 8
		}, ManaCost: core.ManaCostOptions{FlatCost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: h.NewTimer(), Duration: cd}}, CritDamageBonus: core.TernaryFloat64(scatter, h.mortalShots(), 0), DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if scatter {
				result := s.CalcAndDealDamage(sim, t, .5*h.RangedWeaponDamage(sim, s.RangedAttackPower(t, false)), s.OutcomeRangedHitAndCrit)
				h.AutoAttacks.CancelAutoSwing(sim)
				if !result.Landed() {
					return
				}
				t.ForeverControlAura("Scatter Shot", id, core.ForeverIncapacitate, 4*time.Second).Activate(sim)
			} else {
				result := s.CalcOutcome(sim, t, s.OutcomeRangedHitNoHitCounter)
				s.DealOutcome(sim, result)
				if !result.Landed() {
					return
				}
				t.ForeverControlAura("Concussive Shot", id, core.ForeverSnare, 4*time.Second).Activate(sim)
				if sim.Proc(.04*float64(h.ForeverRank("hunter.talent.improved-concussive-shot")), "Improved Concussive Shot") {
					t.ForeverControlAura("Improved Concussive Shot", id, core.ForeverStun, 3*time.Second).Activate(sim)
				}
			}
		}})
	}
}

func (h *Hunter) registerForeverRapidKilling() {
	n := h.ForeverRank("hunter.talent.rapid-killing")
	if n == 0 {
		return
	}
	regen := h.foreverRegen("Rapid Killing Recuperation", .5*float64(h.ForeverRank("hunter.talent.rapid-recuperation")), 15*time.Second)
	mult := 1 + .1*float64(n)
	a := h.RegisterAura(core.Aura{Label: "Rapid Killing", Duration: 20 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range h.Shots {
			s.DamageMultiplier *= mult
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range h.Shots {
			s.DamageMultiplier /= mult
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		// "the damage of your next Shot": a Shot that deals no damage (Concussive Shot)
		// does not use it up.
		if s.Flags.Matches(SpellFlagShot) && s.SpellID != 5116 {
			a.Deactivate(sim)
			if h.ForeverRank("hunter.talent.rapid-recuperation") > 0 {
				regen.Activate(sim)
			}
		}
	}})
	for _, target := range h.Env.Encounter.TargetUnits {
		kill := func(_ *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			if result.Damage > 0 && result.Target.ForeverEnemyDead() &&
				(s.Unit == &h.Unit || (h.pet != nil && s.Unit == &h.pet.Unit) || (h.SerpentSting != nil && h.SerpentSting.Dot(result.Target).IsActive())) {
				a.Activate(sim)
			}
		}
		core.MakePermanent(target.GetOrRegisterAura(core.Aura{Label: "Rapid Killing Watch-" + h.Label, OnSpellHitTaken: kill, OnPeriodicDamageTaken: kill}))
	}
	h.RegisterResetEffect(func(sim *core.Simulation) {
		if h.ForeverParameter("recent_kill_at_pull", 0) > 0 {
			a.Activate(sim)
		}
		interval := h.ForeverParameter("kill_interval_seconds", 0)
		if interval > 0 {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Duration(interval * float64(time.Second)), OnAction: func(sim *core.Simulation) { a.Activate(sim) }})
		}
	})
}
