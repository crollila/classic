package hunter

import (
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (h *Hunter) applyForeverTalents() {
	if h.Forever == nil {
		return
	}
	// Talent values below are the client's per-rank points (ClientTalentValue), with the research
	// record's value as the fallback for a build without the talent.
	// Surefooted effect 1: movement-impairing duration change (-10/-20/-30%).
	surefooted := -h.clientTalent("hunter.talent.surefooted", 1, -h.ForeverValue("hunter.talent.surefooted", 1, 0)) / 100
	h.ForeverControlReduction(core.ForeverSnare, surefooted)
	h.ForeverControlReduction(core.ForeverRoot, surefooted)
	hawkEye := h.clientTalent("hunter.talent.hawk-eye", 0, 2*float64(h.ForeverRank("hunter.talent.hawk-eye")))
	savageStrikes := h.clientTalent("hunter.talent.savage-strikes", 0, 2*float64(h.ForeverRank("hunter.talent.savage-strikes")))
	predatorsEdgeCrit := h.clientTalent("hunter.talent.predator-s-edge", 0, h.ForeverValue("hunter.talent.predator-s-edge", 0, 0)) / 100
	predatorsEdgeOH := h.clientTalent("hunter.talent.predator-s-edge", 1, h.ForeverValue("hunter.talent.predator-s-edge", 1, 0)) / 100
	// Resourcefulness effect 0: trap and melee mana cost change (-30/-60%).
	resourcefulness := -int32(h.clientTalent("hunter.talent.resourcefulness", 0, -30*float64(h.ForeverRank("hunter.talent.resourcefulness"))))
	survivalTactics := h.clientTalent("hunter.talent.survival-tactics", 0, 5*float64(h.ForeverRank("hunter.talent.survival-tactics")))
	// Survivalist's Discipline effect 0: trap and Deterrence cooldown change (-20/-40%).
	trapCooldown := 1 + h.clientTalent("hunter.talent.survivalist-s-discipline", 0, -20*float64(h.ForeverRank("hunter.talent.survivalist-s-discipline")))/100
	improvedStings := h.clientTalent("hunter.talent.improved-stings", 0, h.ForeverValue("hunter.talent.improved-stings", 0, 0)) / 100
	barrage := h.clientTalent("hunter.talent.barrage", 0, h.ForeverValue("hunter.talent.barrage", 0, 0)) / 100
	h.OnSpellRegistered(func(s *core.Spell) {
		if s.Flags.Matches(SpellFlagShot) || s.ProcMask.Matches(core.ProcMaskRangedAuto) {
			old := s.ExtraCastCondition
			s.ExtraCastCondition = func(sim *core.Simulation, t *core.Unit) bool {
				return h.DistanceFromTarget <= 35+hawkEye && (old == nil || old(sim, t))
			}
		}
		if s.ProcMask.Matches(core.ProcMaskMeleeSpecial) {
			s.BonusCritRating += savageStrikes
		}
		// Predator's Edge: "melee critical strike damage", white swings included.
		if s.ProcMask.Matches(core.ProcMaskMelee) {
			s.CritDamageBonus += predatorsEdgeCrit
		}
		if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
			s.DamageMultiplier *= 1 + predatorsEdgeOH
		}
		if s.Cost != nil && (s.Flags.Matches(SpellFlagTrap) || s.ProcMask.Matches(core.ProcMaskMeleeSpecial)) {
			s.Cost.Multiplier -= resourcefulness
		}
		if s.Flags.Matches(SpellFlagTrap) {
			s.BonusHitRating += survivalTactics
			s.CD.Duration = time.Duration(float64(s.CD.Duration) * trapCooldown)
		}
		// Client values: Improved Stings 6/13/20%, Barrage 3/7/10% (not linear per rank).
		if s.SpellCode == SpellCode_HunterSerpentSting {
			s.DamageMultiplier *= 1 + improvedStings
		}
		// Barrage names Multi-Shot, Aimed Shot and Volley; Sniper Shot has its own code.
		if s.SpellCode == SpellCode_HunterMultiShot || s.SpellCode == SpellCode_HunterAimedShot || s.SpellCode == SpellCode_HunterVolley {
			s.DamageMultiplier *= 1 + barrage
		}
	})
	// Focused Fire: "all damage you and your pet deal". The hunter half is the core
	// target multiplier; the pet only deals damage while it is active.
	if h.pet != nil && h.ForeverRank("hunter.talent.focused-fire") > 0 {
		h.pet.PseudoStats.DamageDealtMultiplier *= 1 + h.clientTalent("hunter.talent.focused-fire", 0, h.ForeverValue("hunter.talent.focused-fire", 0, 0))/100
	}
	if n := h.ForeverRank("hunter.talent.improved-tracking"); n > 0 {
		bonus := h.clientTalent("hunter.talent.improved-tracking", 0, float64(n)) / 100
		// BEST_GUESS assumes the player selects the appropriate trackable type before combat.
		h.Env.RegisterPostFinalizeEffect(func() {
			for _, target := range h.Env.Encounter.Targets {
				if target.MobType != proto.MobType_MobTypeUnknown && target.MobType != proto.MobType_MobTypeMechanical {
					for _, at := range h.AttackTables[target.UnitIndex] {
						at.DamageDealtMultiplier *= 1 + bonus
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
			// Client: effect 0 the health percent, effect 1 the period in seconds (rank 2: 1% every
			// 5 sec; the research record reads 2% every 10 sec, the same rate).
			pct := h.clientTalent("hunter.talent.spirit-bond", 0, float64(n)) / 100
			period := time.Duration(h.clientTalent("hunter.talent.spirit-bond", 1, 10)) * time.Second
			hm := h.NewHealthMetrics(id)
			pm := h.pet.NewHealthMetrics(id)
			h.RegisterResetEffect(func(sim *core.Simulation) {
				core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: period, OnAction: func(sim *core.Simulation) {
					if h.pet.IsEnabled() {
						h.GainHealth(sim, h.MaxHealth()*pct, hm)
						h.pet.GainHealth(sim, h.pet.MaxHealth()*pct, pm)
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
		// Rapid Killing effect 0: Rapid Fire cooldown change in ms (-60000/-120000).
		h.RapidFire.CD.Duration += time.Duration(h.clientTalent("hunter.talent.rapid-killing", 0, -60000*float64(h.ForeverRank("hunter.talent.rapid-killing")))) * time.Millisecond
	}
	if n := h.ForeverRank("hunter.talent.resourcefulness"); n > 0 {
		// The client's rank points (effect 1: -20/-40, effect 2: 50/100) do not map clearly onto
		// the text's 30/60% chance of 50% casting regeneration for 30 sec, so the text's values
		// stay, recorded.
		chance := .3 * float64(n)
		if h.GameData != nil {
			chance = gamedata.Use("hunter: Resourcefulness", "proc chance", chance, "PROVISIONAL",
				"client rank points do not map clearly onto the talent text; text value used")
		}
		a := h.foreverRegen("Resourcefulness", .5, 30*time.Second)
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Resourcefulness Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidCrit() && sim.Proc(chance, "Resourcefulness") {
				a.Activate(sim)
			}
		}}))
	}
	if n := h.ForeverRank("hunter.talent.rapid-recuperation"); n > 0 {
		// Effect 0: casting regeneration percent after Serpent Sting hits (25/50).
		a := h.foreverRegen("Rapid Recuperation", h.clientTalent("hunter.talent.rapid-recuperation", 0, 25*float64(n))/100, 15*time.Second)
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Rapid Recuperation Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if s.SpellCode == SpellCode_HunterSerpentSting && r.Landed() {
				a.Activate(sim)
			}
		}}))
	}
	if n := h.ForeverRank("hunter.talent.expose-prey"); n > 0 {
		chance := h.clientTalent("hunter.talent.expose-prey", 0, 5*float64(n)) / 100
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
			if r.Landed() && s.ProcMask.Matches(core.ProcMaskMeleeOrRanged) && marked && sim.Proc(chance, "Expose Prey") {
				h.DefensiveState.Activate(sim)
			}
		}}))
	}
	if h.ForeverRank("hunter.talent.lacerating-strikes") > 0 {
		// The talent spell's effect 0 is the share of Mongoose Bite's damage (40%); the bleed
		// (client 1310536) runs its duration over its effect 0 period (21 sec, every 3 sec).
		share := h.clientTalent("hunter.talent.lacerating-strikes", 0, h.ForeverValue("hunter.talent.lacerating-strikes", 1, 40)) / 100
		ticks, tick := clientTicks(&h.Character, 1310536, 0, 7, 3*time.Second)
		bleed := h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.lacerating-strikes"), ProcMask: core.ProcMaskEmpty, SpellSchool: core.SpellSchoolPhysical, Flags: core.SpellFlagIgnoreResists | core.SpellFlagIgnoreModifiers, DamageMultiplier: 1, ThreatMultiplier: 1, Dot: core.DotConfig{Aura: core.Aura{Label: "Lacerating Strikes"}, NumberOfTicks: ticks, TickLength: tick, OnTick: func(sim *core.Simulation, t *core.Unit, d *core.Dot) {
			d.CalcAndDealPeriodicSnapshotDamage(sim, t, d.OutcomeTick)
		}}})
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Lacerating Strikes Trigger", OnSpellHitDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			// 40% of the damage Mongoose Bite did, over 7 ticks. r.Damage already carries
			// every attacker and target modifier, so the bleed ignores both.
			if s.SpellCode == SpellCode_HunterMongooseBite && r.Damage > 0 {
				d := bleed.Dot(r.Target)
				d.SnapshotBaseDamage = r.Damage * share / float64(ticks)
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
			// Aspect of the Monkey's effect 0 plus Improved Aspect of the Monkey's effect 0.
			dodge := h.ClientEffectValue(13163, 0, 8) + h.clientTalent("hunter.talent.improved-aspect-of-the-monkey", 0, 2*float64(h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey")))
			// The pet's share (50/75/100%) is not among the client's rank points (effect 1 reads
			// 5/6/7), so the talent text's value stays, recorded.
			petShare := h.ForeverValue("hunter.talent.improved-aspect-of-the-monkey", 1, 0) / 100
			if h.GameData != nil && h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey") > 0 {
				petShare = gamedata.Use("hunter: Improved Aspect of the Monkey", "pet share", petShare, "PROVISIONAL",
					"client rank points hold no pet share (effect 1 reads 5/6/7); talent text value used")
			}
			a.OnGain = func(a *core.Aura, sim *core.Simulation) {
				h.AddStatDynamic(sim, stats.Dodge, dodge)
				if h.pet != nil && h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey") > 0 {
					h.pet.AddStatDynamic(sim, stats.Dodge, dodge*petShare)
				}
			}
			a.OnExpire = func(a *core.Aura, sim *core.Simulation) {
				h.AddStatDynamic(sim, stats.Dodge, -dodge)
				if h.pet != nil && h.ForeverRank("hunter.talent.improved-aspect-of-the-monkey") > 0 {
					h.pet.AddStatDynamic(sim, stats.Dodge, -dodge*petShare)
				}
			}
		}
		if kind == "Cheetah" || kind == "Pack" {
			h.foreverMovementAspect(a, id, kind == "Pack")
		}
		if kind == "Beast" && h.ForeverRank("hunter.talent.deadly-aspects") > 0 {
			quick := h.RegisterAura(core.Aura{Label: "Deadly Beast", Duration: 12 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) { h.MultiplyMeleeSpeed(sim, 1.3) }, OnExpire: func(a *core.Aura, sim *core.Simulation) { h.MultiplyMeleeSpeed(sim, 1/1.3) }})
			a.OnSpellHitDealt = func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
				if s.ProcMask.Matches(core.ProcMaskMeleeWhiteHit) && sim.Proc(h.clientTalent("hunter.talent.deadly-aspects", 1, 2*float64(h.ForeverRank("hunter.talent.deadly-aspects")))/100, "Deadly Beast") {
					quick.Activate(sim)
				}
			}
		}
		h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	if h.ForeverRank("hunter.talent.deterrence") > 0 {
		id := h.ForeverAction("hunter.talent.deterrence")
		a := h.RegisterAura(core.Aura{Label: "Deterrence", ActionID: id, Duration: 10 * time.Second}).AttachStatsBuff(stats.Stats{stats.Dodge: 25, stats.Parry: 25})
		h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: h.NewTimer(), Duration: time.Duration(float64(clientCooldown(&h.Character, 19263, 5*time.Minute)) * (1 + h.clientTalent("hunter.talent.survivalist-s-discipline", 0, -20*float64(h.ForeverRank("hunter.talent.survivalist-s-discipline")))/100))}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { a.Activate(sim) }})
	}
	if h.ForeverRank("hunter.talent.counterattack") > 0 {
		ready := h.RegisterAura(core.Aura{Label: "Counterattack Ready", Duration: 5 * time.Second})
		core.MakePermanent(h.RegisterAura(core.Aura{Label: "Counterattack Trigger", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
			if r.DidParry() {
				ready.Activate(sim)
			}
		}}))
		id, bonus, weapon, mana := h.counterattackClientRank()
		h.counterattackID = id
		rootDuration := clientAuraDuration(&h.Character, id, 5*time.Second)
		h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.counterattack"), SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHSpecial, Flags: core.SpellFlagAPL | SpellFlagStrike, ManaCost: core.ManaCostOptions{FlatCost: mana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: h.NewTimer(), Duration: clientCooldown(&h.Character, id, 5*time.Second)}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			return ready.IsActive() && h.DistanceFromTarget <= core.MaxMeleeAttackDistance
		}, DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			ready.Deactivate(sim)
			r := s.CalcAndDealDamage(sim, t, bonus+weapon*h.MHWeaponDamage(sim, s.MeleeAttackPower(t)), s.OutcomeMeleeSpecialNoBlockDodgeParry)
			if r.Landed() {
				t.ForeverControlAura("Counterattack", s.ActionID, core.ForeverRoot, rootDuration).Activate(sim)
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

// Forever client Summon Hawk ranks: client spell id, learned level, damage per hit, mana. The
// client decides the rank (base level) and its values (effect 0 damage, mana cost); the numbers
// here are the fallback.
var summonHawkRanks = []struct {
	id           int32
	level        int32
	damage, mana float64
}{{1293241, 25, 32, 80}, {1293525, 36, 47, 105}, {1293526, 48, 85, 135}, {1293527, 60, 108, 190}}

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

// Forever client Trueshot Aura ranks: client spell id, learned level, ranged attack power (effect
// 0), mana; read from the client, these are the fallback. The client's rank 5 (level 60) really
// reads 50, below rank 4's 75.
var trueshotAuraRanks = []struct {
	id        int32
	level     int32
	rap, mana float64
}{{1299346, 25, 30, 180}, {1299348, 32, 40, 245}, {19506, 40, 50, 325}, {20905, 50, 75, 425}, {20906, 60, 50, 525}}

func trueshotAuraRank(level int32) (rap, mana float64) {
	rap, mana = trueshotAuraRanks[0].rap, trueshotAuraRanks[0].mana
	for _, r := range trueshotAuraRanks {
		if r.level <= level {
			rap, mana = r.rap, r.mana
		}
	}
	return rap, mana
}

// Forever client Counterattack ranks: client spell id, learned level, flat bonus on 50% weapon
// damage, mana. Read from the client (fallback here): effect 0 is normalized weapon damage plus
// its flat bonus (26/40/70/110) and effect 1 the percent of it dealt (50), so the flat part is
// effect 0 x effect 1 (13/20/35/55).
var counterattackRanks = []struct {
	id          int32
	level       int32
	bonus, mana float64
}{{19306, 30, 13, 30}, {1242634, 30, 20, 45}, {20909, 42, 35, 65}, {20910, 54, 55, 85}}

func counterattackRank(level int32) (bonus, mana float64) {
	bonus, mana = counterattackRanks[0].bonus, counterattackRanks[0].mana
	for _, r := range counterattackRanks {
		if r.level <= level {
			bonus, mana = r.bonus, r.mana
		}
	}
	return bonus, mana
}

func (h *Hunter) summonHawkClientRank() (id int32, damage, mana float64) {
	ids, levels := make([]int32, len(summonHawkRanks)), make([]int32, len(summonHawkRanks))
	for i, r := range summonHawkRanks {
		ids[i], levels[i] = r.id, r.level
	}
	r := summonHawkRanks[talentRank(&h.Character, ids, levels, h.Level)]
	return r.id, h.ClientEffectValue(r.id, 0, r.damage), clientManaCost(&h.Character, r.id, r.mana)
}

func (h *Hunter) trueshotAuraClientRank() (id int32, rap, mana float64) {
	ids, levels := make([]int32, len(trueshotAuraRanks)), make([]int32, len(trueshotAuraRanks))
	for i, r := range trueshotAuraRanks {
		ids[i], levels[i] = r.id, r.level
	}
	r := trueshotAuraRanks[talentRank(&h.Character, ids, levels, h.Level)]
	return r.id, h.ClientEffectValue(r.id, 0, r.rap), clientManaCost(&h.Character, r.id, r.mana)
}

func (h *Hunter) counterattackClientRank() (id int32, bonus, weapon, mana float64) {
	ids, levels := make([]int32, len(counterattackRanks)), make([]int32, len(counterattackRanks))
	for i, r := range counterattackRanks {
		ids[i], levels[i] = r.id, r.level
	}
	r := counterattackRanks[talentRank(&h.Character, ids, levels, h.Level)]
	weapon = h.ClientEffectValue(r.id, 1, 50) / 100
	bonus = h.ClientEffectValue(r.id, 0, r.bonus/weapon) * weapon
	return r.id, bonus, weapon, clientManaCost(&h.Character, r.id, r.mana)
}

// Summon Hawk: per-rank damage (effect 0), mana, the two-hawk cap (effect 2) and the cooldown
// shared with Arcane Shot come from the client. The client's summon has no duration or swing
// timer, so the tooltip's 18 sec and a standard pet's 2 sec swing (PREDICTED) stay, recorded.
func (h *Hunter) registerForeverHawks(timer *core.Timer) {
	id0, hawkDamage, hawkMana := h.summonHawkClientRank()
	h.summonHawkID = id0
	hawkDuration, hawkSwing := 18*time.Second, 2*time.Second
	if h.GameData != nil {
		hawkDuration = time.Duration(gamedata.Use("hunter: Summon Hawk", "duration sec", 18, "PROVISIONAL",
			"client summon spell carries no duration; tooltip value")) * time.Second
		hawkSwing = time.Duration(gamedata.Use("hunter: Summon Hawk", "attack interval sec", 2, "PREDICTED",
			"client has no hawk swing timer; a standard pet's 2 sec swing")) * time.Second
	}
	hawkCount := int(h.ClientEffectValue(id0, 2, 2))
	if hawkCount < 1 {
		hawkCount = 2
	}
	// Unleashed Fury and Ferocity name "pets and hawks": their client effect 0 values. The hawks'
	// base 5% crit is the simulator's pet assumption.
	unleashedFury := h.clientTalent("hunter.talent.unleashed-fury", 0, 3*float64(h.ForeverRank("hunter.talent.unleashed-fury"))) / 100
	ferocity := h.clientTalent("hunter.talent.ferocity", 0, 2*float64(h.ForeverRank("hunter.talent.ferocity")))
	attacks := make([]*core.Spell, hawkCount)
	active := make([]*core.Aura, hawkCount)
	targets := make([]*core.Unit, hawkCount)
	for i := 0; i < hawkCount; i++ {
		slot := i
		id := h.ForeverAction("hunter.talent.summon-hawk")
		active[i] = h.RegisterAura(core.Aura{Label: fmt.Sprintf("Hawk %d", i+1), Duration: hawkDuration})
		childID := id
		childID.Tag = -id.Tag*10 - int32(i)
		attacks[i] = h.RegisterSpell(core.SpellConfig{ActionID: childID, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskEmpty, Flags: core.SpellFlagIgnoreAttackerModifiers, DamageMultiplier: 1 + unleashedFury, BonusCritRating: 5 + ferocity, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			s.CalcAndDealDamage(sim, t, hawkDamage, s.OutcomeMeleeSpecialHitAndCrit)
		}})
		h.RegisterResetEffect(func(sim *core.Simulation) {
			targets[slot] = nil
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: hawkSwing, OnAction: func(sim *core.Simulation) {
				if active[slot].IsActive() && targets[slot] != nil {
					attacks[slot].Cast(sim, targets[slot])
				}
			}})
		})
	}
	h.RegisterSpell(core.SpellConfig{ActionID: h.ForeverAction("hunter.talent.summon-hawk"), Flags: core.SpellFlagAPL, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return h.DistanceFromTarget <= 35 }, ManaCost: core.ManaCostOptions{FlatCost: hawkMana}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: timer, Duration: clientCooldown(&h.Character, id0, 6*time.Second)}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		// A free slot if there is one, else the hawk with the least time left.
		slot := 0
		for i := range active {
			if !active[i].IsActive() {
				slot = i
				break
			}
			if active[i].RemainingDuration(sim) < active[slot].RemainingDuration(sim) {
				slot = i
			}
		}
		targets[slot] = t
		active[slot].Activate(sim)
		attacks[slot].Cast(sim, t)
	}})
}
func (h *Hunter) registerForeverTrueshot() {
	id := h.ForeverAction("hunter.talent.trueshot-aura")
	rankID, rap, mana := h.trueshotAuraClientRank()
	h.trueshotAuraID = rankID
	duration := clientAuraDuration(&h.Character, rankID, 30*time.Minute)
	auras := []*core.Aura{}
	for _, a := range h.Party.Players {
		c := a.GetCharacter()
		aura := c.GetAura("Forever Trueshot Aura")
		if aura == nil {
			aura = c.RegisterAura(core.Aura{Label: "Forever Trueshot Aura", ActionID: id, Duration: duration}).AttachStatsBuff(stats.Stats{stats.RangedAttackPower: rap})
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
	// Improved Revive Pet: effect 0 cast time change (ms), effect 1 mana cost change (%), effect 2
	// extra health (%).
	revive := float64(h.ForeverRank("hunter.talent.improved-revive-pet"))
	reviveCast := time.Duration(h.clientTalent("hunter.talent.improved-revive-pet", 0, -3000*revive)) * time.Millisecond
	reviveCost := int32(h.clientTalent("hunter.talent.improved-revive-pet", 1, -20*revive))
	reviveHealth := h.clientTalent("hunter.talent.improved-revive-pet", 2, 15*revive) / 100
	h.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, ManaCost: core.ManaCostOptions{FlatCost: 1367, Multiplier: 100 + reviveCost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault, CastTime: 10*time.Second + reviveCast}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return !h.pet.IsEnabled() }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		h.pet.Enable(sim, h.pet)
		h.pet.RemoveHealth(sim, h.pet.CurrentHealth())
		h.pet.GainHealth(sim, h.pet.MaxHealth()*(.15+reviveHealth), h.pet.NewHealthMetrics(id))
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
	wingClipChance := h.clientTalent("hunter.talent.improved-wing-clip", 0, 7*float64(h.ForeverRank("hunter.talent.improved-wing-clip"))) / 100
	entrapment := time.Duration(h.clientTalent("hunter.talent.entrapment", 0, 1000*float64(h.ForeverRank("hunter.talent.entrapment")))) * time.Millisecond
	concussiveChance := h.clientTalent("hunter.talent.improved-concussive-shot", 0, 4*float64(h.ForeverRank("hunter.talent.improved-concussive-shot"))) / 100
	core.MakePermanent(h.RegisterAura(core.Aura{Label: "Forever Hunter Control", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, r *core.SpellResult) {
		if !r.Landed() {
			return
		}
		if s.SpellCode == SpellCode_HunterWingClip && sim.Proc(wingClipChance, "Improved Wing Clip") {
			r.Target.ForeverControlAura("Improved Wing Clip", h.ForeverAction("hunter.talent.improved-wing-clip"), core.ForeverRoot, 5*time.Second).Activate(sim)
		}
		if s.Flags.Matches(SpellFlagTrap) && h.ForeverRank("hunter.talent.entrapment") > 0 {
			r.Target.ForeverControlAura("Entrapment", h.ForeverAction("hunter.talent.entrapment"), core.ForeverRoot, entrapment).Activate(sim)
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
		scatterWeapon := .5
		if scatter {
			// Scatter Shot (client 19503): effect 0 weapon percent, the client cooldown.
			id = h.ForeverAction("hunter.talent.scatter-shot")
			cd = clientCooldown(&h.Character, 19503, 30*time.Second)
			scatterWeapon = h.ClientEffectValue(19503, 0, 50) / 100
		}
		h.RegisterSpell(core.SpellConfig{ActionID: id, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeRanged, ProcMask: core.ProcMaskRangedSpecial, Flags: core.SpellFlagAPL | SpellFlagShot, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			if scatter {
				return h.DistanceFromTarget <= 15
			}
			return h.DistanceFromTarget >= 8
		}, ManaCost: core.ManaCostOptions{FlatCost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: core.GCDDefault}, CD: core.Cooldown{Timer: h.NewTimer(), Duration: cd}}, CritDamageBonus: core.TernaryFloat64(scatter, h.mortalShots(), 0), DamageMultiplier: 1, ThreatMultiplier: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			if scatter {
				result := s.CalcAndDealDamage(sim, t, scatterWeapon*h.RangedWeaponDamage(sim, s.RangedAttackPower(t, false)), s.OutcomeRangedHitAndCrit)
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
				if sim.Proc(concussiveChance, "Improved Concussive Shot") {
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
	// Rapid Recuperation effect 1: casting regeneration percent on consuming Rapid Killing
	// (50/100); Rapid Killing effect 1: the next Shot's damage bonus (10/20%).
	regen := h.foreverRegen("Rapid Killing Recuperation", h.clientTalent("hunter.talent.rapid-recuperation", 1, 50*float64(h.ForeverRank("hunter.talent.rapid-recuperation")))/100, 15*time.Second)
	mult := 1 + h.clientTalent("hunter.talent.rapid-killing", 1, 10*float64(n))/100
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
