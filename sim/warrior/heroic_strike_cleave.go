package warrior

import (
	"github.com/wowsims/classic/sim/core"
)

func (warrior *Warrior) registerHeroicStrikeSpell(realismICD *core.Cooldown) {
	rank, spellID := warrior.trainerRank("Heroic Strike")
	if rank == 0 {
		return
	}
	// Effect 0 (weapon_damage, type 17): the flat bonus added to the swing's weapon damage.
	flatDamageBonus := clientRankValue(&warrior.Character, spellID, 0, heroicStrikeBonus[rank-1])
	// No known equation; the client does not store threat.
	threat := codeValue(&warrior.Character, "warrior: Heroic Strike", "bonus threat", heroicStrikeThreat[rank-1], "PROVISIONAL",
		"threat is not client data; the Classic value is used")

	warrior.HeroicStrike = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial | core.ProcMaskMeleeMHAuto,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagNoOnCastComplete | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			// Improved Heroic Strike: client effect 0 is the rage cost modifier in tenths.
			Cost:   15 - clientTalent(&warrior.Character, "warrior.talent.improved-heroic-strike", 0, -0.1, float64(warrior.Talents.ImprovedHeroicStrike)),
			Refund: 0.8,
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		FlatThreatBonus:  threat,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := flatDamageBonus + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower(target))
			result := spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)

			if !result.Landed() {
				spell.IssueRefund(sim)
			}

			spell.DealDamage(sim, result)
			if warrior.curQueueAura != nil {
				warrior.curQueueAura.Deactivate(sim)
			}
		},
	})
	warrior.HeroicStrikeQueue = warrior.makeQueueSpellsAndAura(warrior.HeroicStrike, realismICD)
}

func (warrior *Warrior) registerCleaveSpell(realismICD *core.Cooldown) {
	rank, spellID := warrior.trainerRank("Cleave")
	if rank == 0 {
		return
	}
	// Effect 0 (weapon_damage, type 17): the flat bonus added to each hit's weapon damage.
	rankBonus := clientRankValue(&warrior.Character, spellID, 0, cleaveBonus[rank-1])
	flatDamageBonus := rankBonus
	threat := codeValue(&warrior.Character, "warrior: Cleave", "bonus threat", cleaveThreat[rank-1], "PROVISIONAL",
		"threat is not client data; the Classic value is used")

	flatDamageBonus *= []float64{1, 1.4, 1.8, 2.2}[warrior.Talents.ImprovedCleave]
	if warrior.ForeverRank("warrior.talent.improved-cleave") > 0 {
		flatDamageBonus = rankBonus
	}

	results := make([]*core.SpellResult, min(int32(2), warrior.Env.GetNumTargets()))

	warrior.Cleave = warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial | core.ProcMaskMeleeMHAuto,
		Flags:       core.SpellFlagMeleeMetrics | SpellFlagOffensive,

		RageCost: core.RageCostOptions{
			// Raging Blows effect 1 and Improved Cleave effect 0 are rage cost modifiers in tenths.
			Cost: 20 - clientTalent(&warrior.Character, "warrior.talent.raging-blows", 1, -0.1, 2*float64(warrior.ForeverRank("warrior.talent.raging-blows"))) -
				clientTalent(&warrior.Character, "warrior.talent.improved-cleave", 0, -0.1, warrior.ForeverValue("warrior.talent.improved-cleave", 0, 0)),
		},

		CritDamageBonus: warrior.impale(),

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		FlatThreatBonus:  threat,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			for idx := range results {
				baseDamage := flatDamageBonus + spell.Unit.MHWeaponDamage(sim, spell.MeleeAttackPower(target))
				results[idx] = spell.CalcDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
				target = sim.Environment.NextTargetUnit(target)
			}

			for _, result := range results {
				spell.DealDamage(sim, result)
			}

			if warrior.curQueueAura != nil {
				warrior.curQueueAura.Deactivate(sim)
			}
		},
	})
	warrior.CleaveQueue = warrior.makeQueueSpellsAndAura(warrior.Cleave, realismICD)
}

func (warrior *Warrior) makeQueueSpellsAndAura(srcSpell *WarriorSpell, realismICD *core.Cooldown) *WarriorSpell {
	isQueueQueued := false

	queueAura := warrior.RegisterAura(core.Aura{
		Label:    "HS/Cleave Queue Aura-" + srcSpell.ActionID.String(),
		ActionID: srcSpell.ActionID.WithTag(1),
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			isQueueQueued = false
		},
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if warrior.curQueueAura != nil {
				warrior.curQueueAura.Deactivate(sim)
			}
			warrior.PseudoStats.DisableDWMissPenalty = true
			warrior.curQueueAura = aura
			warrior.curQueuedAutoSpell = srcSpell
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			warrior.PseudoStats.DisableDWMissPenalty = false
			warrior.curQueueAura = nil
			warrior.curQueuedAutoSpell = nil
		},
	})

	queueSpell := warrior.RegisterSpell(AnyStance, core.SpellConfig{
		ActionID: srcSpell.ActionID.WithTag(1),
		Flags:    core.SpellFlagMeleeMetrics | core.SpellFlagAPL | core.SpellFlagCastTimeNoGCD,

		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
			return warrior.curQueueAura == nil &&
				!isQueueQueued &&
				warrior.CurrentRage() >= srcSpell.DefaultCast.Cost &&
				!warrior.IsCasting(sim) &&
				realismICD.IsReady(sim)
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			if realismICD.IsReady(sim) {
				isQueueQueued = true
				realismICD.Use(sim)
				sim.AddPendingAction(&core.PendingAction{
					NextActionAt: sim.CurrentTime + realismICD.Duration,
					OnAction: func(sim *core.Simulation) {
						queueAura.Activate(sim)
						isQueueQueued = false
					},
				})
			}
		},
	})

	return queueSpell
}

func (warrior *Warrior) TryHSOrCleave(sim *core.Simulation, mhSwingSpell *core.Spell) *core.Spell {
	if !warrior.curQueueAura.IsActive() {
		return mhSwingSpell
	}

	if !warrior.curQueuedAutoSpell.CanCast(sim, warrior.CurrentTarget) {
		warrior.curQueueAura.Deactivate(sim)
		return mhSwingSpell
	}

	return warrior.curQueuedAutoSpell.Spell
}
