package rogue

import (
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

/**
Instant Poison: 20% proc chance. Deadly Poison: 30% proc chance, 5 stacks.
Wound Poison: 30% proc chance, 5 stacks. Per-rank values and the levels each
rank becomes available are in ranks.go.
*/

// TODO: Add charges to poisons

type PoisonProcSource int

const (
	NormalProc PoisonProcSource = iota
)

func (rogue *Rogue) GetInstantPoisonProcChance() float64 {
	return 0.2 + rogue.improvedPoisons() + rogue.additivePoisonBonusChance
}

func (rogue *Rogue) GetDeadlyPoisonProcChance() float64 {
	return 0.3 + rogue.improvedPoisons() + rogue.additivePoisonBonusChance
}

func (rogue *Rogue) GetWoundPoisonProcChance() float64 {
	return 0.3 + rogue.improvedPoisons() + rogue.additivePoisonBonusChance
}

func (rogue *Rogue) improvedPoisons() float64 {
	return []float64{0, 0.02, 0.04, 0.06, 0.08, 0.1}[rogue.Talents.ImprovedPoisons]
}

func (rogue *Rogue) getPoisonDamageMultiplier() float64 {
	return []float64{1, 1.04, 1.08, 1.12, 1.16, 1.2}[rogue.Talents.VilePoisons]
}

///////////////////////////////////////////////////////////////////////////
//                               Apply Poisons
///////////////////////////////////////////////////////////////////////////

func (rogue *Rogue) applyPoisons() {
	rogue.applyDeadlyPoison()
	rogue.applyInstantPoison()
	rogue.applyWoundPoison()
}

// Apply Instant Poison to weapon and enable procs
func (rogue *Rogue) applyInstantPoison() {
	procMask := rogue.getImbueProcMask(proto.WeaponImbue_InstantPoison)
	if procMask == core.ProcMaskUnknown || rogue.poisonRank("Instant Poison") < 0 {
		return
	}

	rogue.RegisterAura(core.Aura{
		Label:    "Instant Poison",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(procMask) {
				return
			}

			if sim.RandomFloat("Instant Poison") < rogue.GetInstantPoisonProcChance() && rogue.consumeForeverPoisonCharge(sim, spell) {
				rogue.InstantPoison.Cast(sim, result.Target)
			}
		},
	})
}

// Apply Deadly Poison to weapon and enable procs
func (rogue *Rogue) applyDeadlyPoison() {
	procMask := rogue.getImbueProcMask(proto.WeaponImbue_DeadlyPoison)
	if procMask == core.ProcMaskUnknown || rogue.poisonRank("Deadly Poison") < 0 {
		return
	}

	rogue.RegisterAura(core.Aura{
		Label:    "Deadly Poison",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(procMask) {
				return
			}
			if sim.RandomFloat("Deadly Poison") < rogue.GetDeadlyPoisonProcChance() && rogue.consumeForeverPoisonCharge(sim, spell) {
				rogue.DeadlyPoison.Cast(sim, result.Target)
			}
		},
	})
}

// Apply Wound Poison to weapon and enable procs
func (rogue *Rogue) applyWoundPoison() {
	procMask := rogue.getImbueProcMask(proto.WeaponImbue_WoundPoison)
	if procMask == core.ProcMaskUnknown || rogue.poisonRank("Wound Poison") < 0 {
		return
	}

	rogue.RegisterAura(core.Aura{
		Label:    "Wound Poison",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(procMask) {
				return
			}

			if sim.RandomFloat("Wound Poison") < rogue.GetWoundPoisonProcChance() && rogue.consumeForeverPoisonCharge(sim, spell) {
				rogue.WoundPoison.Cast(sim, result.Target)
			}
		},
	})
}

///////////////////////////////////////////////////////////////////////////
//                              Register Poisons
///////////////////////////////////////////////////////////////////////////

func (rogue *Rogue) registerInstantPoisonSpell() {
	rogue.InstantPoison = rogue.makeInstantPoison()
}

func (rogue *Rogue) registerDeadlyPoisonSpell() {
	// Below the level Deadly Poison is learned the imbue never procs (applyDeadlyPoison),
	// but the spells stay registered at rank 1 for the code that inspects the dot.
	rank := max(0, rogue.poisonRank("Deadly Poison"))
	baseDamageTick := deadlyPoisonTickDamage[rank]
	if rogue.Forever != nil {
		baseDamageTick = deadlyPoisonTickDamageForever[rank]
	}
	spellID := deadlyPoisonIDs[rank]

	rogue.deadlyPoisonTick = rogue.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID, Tag: 100},
		SpellSchool: core.SpellSchoolNature,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamageProc,
		Flags:       core.SpellFlagPoison | core.SpellFlagPassiveSpell | SpellFlagRoguePoison,

		DamageMultiplier: rogue.getPoisonDamageMultiplier(),
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label:     "DeadlyPoison",
				MaxStacks: 5,
				Duration:  time.Second * 12,
			},
			NumberOfTicks: 4,
			TickLength:    time.Second * 3,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, applyStack bool) {
				if !applyStack {
					return
				}

				// only the first stack snapshots the multiplier
				if dot.GetStacks() == 1 {
					attackTable := dot.Spell.Unit.AttackTables[target.UnitIndex][dot.Spell.CastType]
					dot.SnapshotAttackerMultiplier = dot.Spell.AttackerDamageMultiplier(attackTable, true)
					dot.SnapshotBaseDamage = 0
				}

				dot.SnapshotBaseDamage += baseDamageTick
			},

			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				// Venom raises poison damage while it is up. The stack snapshots its multiplier
				// only on the first application, so Venom is read at tick time instead of being
				// folded into that snapshot (which would miss a running stack, or outlive Venom).
				if venom := rogue.foreverVenomAura; venom != nil && venom.IsActive() {
					snapshot := dot.SnapshotAttackerMultiplier
					dot.SnapshotAttackerMultiplier *= foreverVenomPoisonMultiplier
					dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
					dot.SnapshotAttackerMultiplier = snapshot
					return
				}
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},
	})

	rogue.DeadlyPoison = rogue.makeDeadlyPoison()
}

func (rogue *Rogue) registerWoundPoisonSpell() {
	woundPoisonDebuffAura := core.Aura{
		Label:     "WoundPoison-" + strconv.Itoa(int(rogue.Index)),
		ActionID:  core.ActionID{SpellID: 13219},
		MaxStacks: 5,
		Duration:  time.Second * 15,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			// all healing effects used on target reduced by x, stacks 5 times
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			// undo reduced healing effects used on targets
		},
	}

	rogue.woundPoisonDebuffAuras = rogue.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
		return target.RegisterAura(woundPoisonDebuffAura)
	})
	rogue.WoundPoison = rogue.makeWoundPoison()
}

///////////////////////////////////////////////////////////////////////////
//                              Make Poisons
///////////////////////////////////////////////////////////////////////////

// Make a source based variant of Instant Poison
func (rogue *Rogue) makeInstantPoison() *core.Spell {
	// Below the level Instant Poison is learned the imbue never procs (applyInstantPoison).
	rank := max(0, rogue.poisonRank("Instant Poison"))
	damage := instantPoisonDamage[rank]
	if rogue.Forever != nil {
		damage = instantPoisonDamageForever[rank]
	}
	baseDamageByLevel := damage[0]
	damageVariance := damage[1]
	spellID := instantPoisonIDs[rank]

	return rogue.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolNature,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamageProc,
		Flags:       core.SpellFlagPoison | core.SpellFlagPassiveSpell | SpellFlagRoguePoison,

		DamageMultiplier: rogue.getPoisonDamageMultiplier(),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageByLevel, baseDamageByLevel+damageVariance)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}

func (rogue *Rogue) makeDeadlyPoison() *core.Spell {
	return rogue.RegisterSpell(core.SpellConfig{
		ActionID: core.ActionID{SpellID: rogue.deadlyPoisonTick.SpellID},
		Flags:    core.SpellFlagPoison | core.SpellFlagPassiveSpell | SpellFlagRoguePoison,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)

			if !result.Landed() {
				return
			}

			dot := rogue.deadlyPoisonTick.Dot(target)

			dot.ApplyOrRefresh(sim)
			if dot.GetStacks() < dot.MaxStacks {
				dot.AddStack(sim)
				// snapshotting only takes place when adding a stack
				dot.TakeSnapshot(sim, true)
			}
		},
	})
}

// Make a source based variant of Wound Poison
func (rogue *Rogue) makeWoundPoison() *core.Spell {
	return rogue.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: 13219},
		SpellSchool: core.SpellSchoolNature,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamageProc,
		Flags:       core.SpellFlagPoison | core.SpellFlagPassiveSpell | SpellFlagRoguePoison,

		DamageMultiplier: rogue.getPoisonDamageMultiplier(),
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMagicHit)

			if !result.Landed() {
				return
			}

			aura := rogue.woundPoisonDebuffAuras.Get(target)
			if !aura.IsActive() {
				aura.Activate(sim)
				aura.SetStacks(sim, 1)
				return
			}

			if aura.GetStacks() < 5 {
				aura.Refresh(sim)
				aura.AddStack(sim)
				return
			}
			aura.Refresh(sim)
		},
	})
}
