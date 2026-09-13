package rogue

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"time"
)

func (r *Rogue) applyForeverTalents() {
	if r.Forever == nil {
		return
	}
	r.ForeverRangedMissChance += .02 * float64(r.ForeverRank("rogue.talent.heightened-senses"))
	r.ForeverMagicMissChance += .02 * float64(r.ForeverRank("rogue.talent.heightened-senses"))
	r.AddStat(stats.Expertise, float64(r.ForeverRank("rogue.talent.weapon-expertise")))
	r.OnSpellRegistered(func(s *core.Spell) {
		switch s.SpellCode {
		case SpellCode_RogueBackstab:
			s.BonusCritRating += 10 * float64(r.ForeverRank("rogue.talent.puncturing-wounds"))
		}
		switch s.SpellCode {
		case SpellCode_RogueBackstab, SpellCode_RogueGarrote, SpellCode_RogueAmbush:
			s.DamageMultiplier *= 1 + .05*float64(r.ForeverRank("rogue.talent.opportunity"))
		}
		switch s.SpellCode {
		case SpellCode_RogueSinisterStrike, SpellCode_RogueBackstab, SpellCode_RogueEviscerate:
			s.DamageMultiplier *= 1 + .02*float64(r.ForeverRank("rogue.talent.aggression"))
		}
		if s.SpellCode == SpellCode_RogueRupture {
			s.DamageMultiplier *= 1 + .1*float64(r.ForeverRank("rogue.talent.serrated-blades"))
		}
		if s.ProcMask.Matches(core.ProcMaskMelee) {
			s.BonusArmorPenetration += .03 * float64(r.ForeverRank("rogue.talent.serrated-blades"))
			weapon := r.MainHand().WeaponType
			if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
				weapon = r.OffHand().WeaponType
			}
			n := float64(r.ForeverRank("rogue.talent.hack-and-slash"))
			switch weapon {
			case proto.WeaponType_WeaponTypeDagger, proto.WeaponType_WeaponTypeFist:
				s.BonusCritRating += n
			case proto.WeaponType_WeaponTypeMace:
				s.BonusArmorPenetration += .03 * n
			}
		}
	})
	if n := r.ForeverRank("rogue.talent.hack-and-slash"); n > 0 {
		icd := core.Cooldown{Timer: r.NewTimer(), Duration: 200 * time.Millisecond}
		core.MakePermanent(r.RegisterAura(core.Aura{Label: "Hack and Slash", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			weapon := r.MainHand().WeaponType
			if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
				weapon = r.OffHand().WeaponType
			}
			if result.Landed() && s.ProcMask.Matches(core.ProcMaskMelee) && (weapon == proto.WeaponType_WeaponTypeSword || weapon == proto.WeaponType_WeaponTypeAxe) && icd.IsReady(sim) && sim.Proc(.01*float64(n), "Hack and Slash") {
				icd.Use(sim)
				r.AutoAttacks.ExtraMHAttack(sim, 1, r.ForeverAction("rogue.talent.hack-and-slash"), s.ActionID)
			}
		}}))
	}
	if n := r.ForeverRank("rogue.talent.setup"); n > 0 {
		m := r.NewComboPointMetrics(r.ForeverAction("rogue.talent.setup"))
		core.MakePermanent(r.RegisterAura(core.Aura{Label: "Setup", OnSpellHitTaken: func(a *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			if (result.DidDodge() || (!s.SpellSchool.Matches(core.SpellSchoolPhysical) && result.Damage == 0 && result.Landed())) && sim.Proc(min(1, .33*float64(n)), "Setup") {
				r.AddComboPoints(sim, 1, r.CurrentTarget, m)
			}
		}}))
	}
	if n := r.ForeverRank("rogue.talent.puncturing-wounds"); n > 0 {
		m := r.NewComboPointMetrics(r.ForeverAction("rogue.talent.puncturing-wounds"))
		core.MakePermanent(r.RegisterAura(core.Aura{Label: "Puncturing Wounds", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			if s.SpellCode == SpellCode_RogueBackstab && result.Landed() && sim.Proc(.15*float64(n), "Puncturing Wounds") {
				r.AddComboPoints(sim, 1, result.Target, m)
			}
		}}))
	}
}
func (r *Rogue) registerForeverAbilities() {
	if r.Forever == nil {
		return
	}
	if r.Evasion == nil {
		r.RegisterEvasionSpell()
	}
	if r.ForeverRank("rogue.talent.mutilate") > 0 {
		r.registerForeverMutilate()
	}
	if r.ForeverRank("rogue.talent.venom") > 0 {
		r.registerForeverVenom()
	}
	if n := r.ForeverRank("rogue.talent.cutthroat"); n > 0 {
		r.ForeverCutthroat = r.RegisterAura(core.Aura{Label: "Cutthroat", ActionID: r.ForeverAction("rogue.talent.cutthroat"), Duration: 10 * time.Second})
		core.MakePermanent(r.RegisterAura(core.Aura{Label: "Cutthroat Trigger", OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			if s.SpellCode == SpellCode_RogueBackstab && result.Landed() && sim.Proc(.03*float64(n), "Cutthroat") {
				r.ForeverCutthroat.Activate(sim)
			}
		}}))
	}
	if r.ForeverRank("rogue.talent.thousand-cuts") > 0 {
		a := r.RegisterAura(core.Aura{Label: "Thousand Cuts", ActionID: r.ForeverAction("rogue.talent.thousand-cuts"), Duration: 10 * time.Second, MaxStacks: 5, OnStacksChange: func(a *core.Aura, sim *core.Simulation, old, new int32) {
			for _, s := range []*core.Spell{r.Hemorrhage, r.Backstab} {
				if s != nil {
					s.Cost.FlatModifier -= 3 * (new - old)
				}
			}
		}, OnCastComplete: func(a *core.Aura, sim *core.Simulation, s *core.Spell) {
			if s == r.Hemorrhage || s == r.Backstab {
				a.Deactivate(sim)
			}
		}})
		core.MakePermanent(r.RegisterAura(core.Aura{Label: "Thousand Cuts Trigger", OnPeriodicDamageDealt: func(_ *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
			if s == r.Rupture {
				a.Activate(sim)
				a.AddStack(sim)
			}
		}}))
	}
	if n := r.ForeverRank("rogue.talent.quietus"); n > 0 {
		mult := 1 + .02*float64(n)
		a := r.RegisterAura(core.Aura{Label: "Quietus", Duration: core.NeverExpires, OnGain: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range []*core.Spell{r.SinisterStrike, r.GhostlyStrike, r.Hemorrhage} {
				if s != nil {
					s.DamageMultiplier *= mult
				}
			}
		}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
			for _, s := range []*core.Spell{r.SinisterStrike, r.GhostlyStrike, r.Hemorrhage} {
				if s != nil {
					s.DamageMultiplier /= mult
				}
			}
		}})
		r.RegisterResetEffect(func(sim *core.Simulation) {
			sim.RegisterExecutePhaseCallback(func(sim *core.Simulation, phase int32) {
				if phase == 35 {
					a.Activate(sim)
				}
			})
		})
	}
	if r.ForeverRank("rogue.talent.restless-blades") > 0 {
		r.OnComboPointsSpent(func(sim *core.Simulation, s *core.Spell, cp int32) {
			if s != r.Eviscerate && s != r.Rupture {
				return
			}
			for _, cd := range []*core.Spell{r.AdrenalineRush, r.BladeFlurry, r.Evasion, r.ForeverSprint, r.Vanish} {
				if cd != nil && cd.CD.Timer != nil {
					cd.CD.Set(max(sim.CurrentTime, cd.CD.ReadyAt()-time.Duration(2*cp)*time.Second))
				}
			}
		})
	}
	r.registerForeverUtility()
	r.registerForeverRemorseless()
	r.RegisterResetEffect(func(sim *core.Simulation) {
		r.foreverPoisonCharges = [2]int32{115, 115}
		if r.Consumes != nil {
			if r.Consumes.MainHandImbue == proto.WeaponImbue_DeadlyPoison {
				r.foreverPoisonCharges[0] = 105
			}
			if r.Consumes.OffHandImbue == proto.WeaponImbue_DeadlyPoison {
				r.foreverPoisonCharges[1] = 105
			}
		}
	})
	id := core.ActionID{SpellID: 1787}
	var timer *core.Timer
	cd := 10*time.Second - time.Duration(2*r.ForeverRank("rogue.talent.camouflage"))*time.Second
	if cd > 0 {
		timer = r.NewTimer()
	}
	r.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: timer, Duration: cd}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return sim.CurrentTime <= 0 && !r.IsStealthed() }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { r.StealthAura.Activate(sim) }})
}

func (r *Rogue) registerForeverMutilate() {
	id := r.ForeverAction("rogue.talent.mutilate")
	attacks := make([]*core.Spell, 2)
	landed := false
	for i := 0; i < 2; i++ {
		offhand := i == 1
		mask := core.ProcMaskMeleeMHSpecial
		if offhand {
			mask = core.ProcMaskMeleeOHSpecial
		}
		childID := id
		childID.Tag = -id.Tag*10 - int32(i)
		attacks[i] = r.RegisterSpell(core.SpellConfig{ActionID: childID, SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: mask, Flags: SpellFlagBuilder | SpellFlagColdBlooded | core.SpellFlagMeleeMetrics, DamageMultiplier: 1 + .05*float64(r.ForeverRank("rogue.talent.opportunity")), ThreatMultiplier: 1, CritDamageBonus: r.lethality(), BonusCritRating: 5 * float64(r.ForeverRank("rogue.talent.puncturing-wounds")), BonusCoefficient: 1, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			d := r.MHNormalizedWeaponDamage(sim, s.MeleeAttackPower(t))
			if offhand {
				d = r.OHNormalizedWeaponDamage(sim, s.MeleeAttackPower(t)) * r.dwsMultiplier()
			}
			d = .75*d + 13
			poisoned := r.DeadlyPoison.Dot(t).IsActive()
			for _, a := range t.GetAurasWithTag("Poison") {
				poisoned = poisoned || a.IsActive()
			}
			if poisoned {
				d *= 1.2
			}
			result := s.CalcAndDealDamage(sim, t, d, s.OutcomeMeleeWeaponSpecialHitAndCrit)
			landed = landed || result.Landed()
		}})
	}
	r.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, EnergyCost: core.EnergyCostOptions{Cost: 60, Refund: .8}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: time.Second}, IgnoreHaste: true}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
		return r.HasDagger(core.MainHand) && r.HasDagger(core.OffHand)
	}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		r.BreakStealth(sim)
		landed = false
		attacks[0].Cast(sim, t)
		attacks[1].Cast(sim, t)
		if landed {
			r.AddComboPoints(sim, 2, t, s.ComboPointMetrics())
		} else {
			s.IssueRefund(sim)
		}
	}})
}
func (r *Rogue) registerForeverVenom() {
	id := r.ForeverAction("rogue.talent.venom")
	a := r.RegisterAura(core.Aura{Label: "Venom", ActionID: id, Duration: 21 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		r.additivePoisonBonusChance += .1
		for _, s := range r.Spellbook {
			if s.Flags.Matches(SpellFlagRoguePoison) {
				s.DamageMultiplier *= 1.3
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		r.additivePoisonBonusChance -= .1
		for _, s := range r.Spellbook {
			if s.Flags.Matches(SpellFlagRoguePoison) {
				s.DamageMultiplier /= 1.3
			}
		}
	}})
	spell := r.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, EnergyCost: core.EnergyCostOptions{Cost: 25}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: time.Second}, IgnoreHaste: true}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool { return r.ComboPoints() > 0 }, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
		a.Duration = time.Duration(6+3*r.ComboPoints()) * time.Second
		r.SpendComboPoints(sim, s)
		a.Activate(sim)
	}})
	r.Finishers = append(r.Finishers, spell)
}

func (r *Rogue) registerForeverUtility() {
	id := core.ActionID{SpellID: 11305}
	sprint := r.RegisterAura(core.Aura{Label: "Sprint", ActionID: id, Duration: 15 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		r.AddMoveSpeedModifier(&id, 1.7)
		if sim.Proc(.5*float64(r.ForeverRank("rogue.talent.improved-sprint")), "Improved Sprint") {
			for _, kind := range []core.ForeverControlKind{core.ForeverRoot, core.ForeverSnare} {
				for _, a := range r.GetAurasWithTag("forever-control-" + string(kind)) {
					a.Deactivate(sim)
				}
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) { r.RemoveMoveSpeedModifier(&id) }})
	r.ForeverSprint = r.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, Cast: core.CastConfig{CD: core.Cooldown{Timer: r.NewTimer(), Duration: time.Duration(float64(5*time.Minute) * (1 - .3*float64(r.ForeverRank("rogue.talent.endurance"))))}}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) { sprint.Activate(sim) }})
	for _, kind := range []string{"Gouge", "Blind", "Sap", "Kick", "Cheap Shot", "Kidney Shot"} {
		id := core.ActionID{}
		cost := 0.
		cd := time.Duration(0)
		duration := time.Duration(0)
		control := core.ForeverStun
		switch kind {
		case "Gouge":
			id.SpellID = 11286
			cost = 45
			cd = 10 * time.Second
			duration = 4*time.Second + time.Duration(.5*float64(r.ForeverRank("rogue.talent.improved-gouge"))*float64(time.Second))
		case "Blind":
			id.SpellID = 2094
			cost = 30 * (1 - .25*float64(r.ForeverRank("rogue.talent.dirty-tricks")))
			cd = 5*time.Minute - time.Duration(45*r.ForeverRank("rogue.talent.elusiveness"))*time.Second
			duration = 10 * time.Second
		case "Sap":
			id.SpellID = 11297
			cost = 65 * (1 - .25*float64(r.ForeverRank("rogue.talent.dirty-tricks")))
			duration = 45 * time.Second
		case "Kick":
			id.SpellID = 1769
			cost = 25
			cd = 10 * time.Second
			duration = 2 * time.Second
			control = core.ForeverSilence
		case "Cheap Shot":
			id.SpellID = 1833
			cost = 60 - 10*float64(r.ForeverRank("rogue.talent.dirty-deeds"))
			duration = 4 * time.Second
		case "Kidney Shot":
			id.SpellID = 8643
			cost = 25
			cd = 20 * time.Second
			duration = 6 * time.Second
		}
		var kidneyAuras core.AuraArray
		if kind == "Kidney Shot" && r.ForeverRank("rogue.talent.improved-kidney-shot") > 0 {
			kidneyAuras = r.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
				return t.GetOrRegisterAura(core.Aura{Label: "Improved Kidney Shot-" + r.Label, Duration: 6 * time.Second})
			})
		}
		var timer *core.Timer
		if cd > 0 {
			timer = r.NewTimer()
		}
		spell := r.RegisterSpell(core.SpellConfig{ActionID: id, Flags: core.SpellFlagAPL, EnergyCost: core.EnergyCostOptions{Cost: cost}, Cast: core.CastConfig{DefaultCast: core.Cast{GCD: time.Second}, IgnoreHaste: true, CD: core.Cooldown{Timer: timer, Duration: cd}}, ExtraCastCondition: func(sim *core.Simulation, t *core.Unit) bool {
			if kind == "Sap" || kind == "Cheap Shot" {
				return r.IsStealthed()
			}
			return kind != "Kidney Shot" || r.ComboPoints() > 0
		}, ApplyEffects: func(sim *core.Simulation, t *core.Unit, s *core.Spell) {
			r.BreakStealth(sim)
			d := duration
			if kind == "Kidney Shot" {
				d = time.Duration(r.ComboPoints()+1) * time.Second
				r.SpendComboPoints(sim, s)
			}
			if kind == "Kick" {
				t.ForeverInterrupt(sim)
				if !sim.Proc(.5*float64(r.ForeverRank("rogue.talent.improved-kick")), "Improved Kick") {
					return
				}
			}
			a := t.ForeverControlAura(kind, id, control, d)
			a.Activate(sim)
			if kind == "Cheap Shot" {
				r.AddComboPoints(sim, 2, t, s.ComboPointMetrics())
			}
			if kind == "Gouge" {
				r.AddComboPoints(sim, 1, t, s.ComboPointMetrics())
			}
			if kind == "Kidney Shot" && a.IsActive() && r.ForeverRank("rogue.talent.improved-kidney-shot") > 0 {
				buff := kidneyAuras.Get(t)
				buff.Duration = d
				buff.Activate(sim)
			}
		}})
		if kind == "Kidney Shot" {
			r.Finishers = append(r.Finishers, spell)
		}
	}
	if n := r.ForeverRank("rogue.talent.improved-kidney-shot"); n > 0 {
		r.Env.RegisterPostFinalizeEffect(func() {
			for _, t := range r.Env.Encounter.Targets {
				for _, at := range r.AttackTables[t.UnitIndex] {
					old := at.DamageDoneByCasterMultiplier
					at.DamageDoneByCasterMultiplier = func(s *core.Spell, table *core.AttackTable) float64 {
						m := 1.
						if old != nil {
							m = old(s, table)
						}
						a := t.GetAura("Improved Kidney Shot-" + r.Label)
						if a != nil && a.IsActive() {
							m *= 1 + .05*float64(n)
						}
						return m
					}
				}
			}
		})
	}
}

func (r *Rogue) registerForeverRemorseless() {
	n := r.ForeverRank("rogue.talent.remorseless-attacks")
	if n == 0 {
		return
	}
	eligible := func(s *core.Spell) bool {
		return s.SpellCode == SpellCode_RogueSinisterStrike || s.SpellCode == SpellCode_RogueBackstab || s.SpellCode == SpellCode_RogueAmbush || s.SpellCode == SpellCode_RogueGhostlyStrike || (s.OtherID == proto.OtherAction_OtherActionForever && s.Tag < 0)
	}
	bonus := 20 * float64(n)
	a := r.RegisterAura(core.Aura{Label: "Remorseless Attacks", Duration: 20 * time.Second, OnGain: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range r.Spellbook {
			if eligible(s) {
				s.BonusCritRating += bonus
			}
		}
	}, OnExpire: func(a *core.Aura, sim *core.Simulation) {
		for _, s := range r.Spellbook {
			if eligible(s) {
				s.BonusCritRating -= bonus
			}
		}
	}, OnSpellHitDealt: func(a *core.Aura, sim *core.Simulation, s *core.Spell, result *core.SpellResult) {
		if eligible(s) {
			a.Deactivate(sim)
		}
	}})
	r.RegisterResetEffect(func(sim *core.Simulation) {
		if r.ForeverParameter("recent_kill_at_pull", 0) > 0 {
			a.Activate(sim)
		}
		interval := r.ForeverParameter("kill_interval_seconds", 0)
		if interval > 0 {
			core.StartPeriodicAction(sim, core.PeriodicActionOptions{Period: time.Duration(interval * float64(time.Second)), OnAction: func(sim *core.Simulation) { a.Activate(sim) }})
		}
	})
}

// Classical charge budgets are reused provisionally. Conservation is rolled on
// application, once for the attacking hand; Classic profiles keep unlimited charges.
func (r *Rogue) consumeForeverPoisonCharge(sim *core.Simulation, s *core.Spell) bool {
	if r.Forever == nil {
		return true
	}
	hand := 0
	if s.ProcMask.Matches(core.ProcMaskMeleeOH) {
		hand = 1
	}
	if r.foreverPoisonCharges[hand] <= 0 {
		return false
	}
	if !sim.Proc(.1*float64(r.ForeverRank("rogue.talent.improved-poisons")), "Improved Poisons charge conservation") {
		r.foreverPoisonCharges[hand]--
	}
	return true
}
