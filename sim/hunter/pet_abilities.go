package hunter

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

type PetAbilityType int

// Pet AI doesn't use abilities immediately, so model this with a 1.6s GCD.
const PetGCD = time.Millisecond * 1600

const (
	Unknown PetAbilityType = iota
	Bite
	Claw
	Screech
	FuriousHowl
	LightningBreath
	ScorpidPoison
)

// petAbilityRank is one trainer rank of a pet ability: spell id, learned level and
// base damage (min/max; for Scorpid Poison min is the damage per tick per stack).
type petAbilityRank struct {
	id       int32
	level    int32
	min, max float64
}

// Ranks and learned levels are the Forever beta client's pet-trainer ranks
// (core.TrainerRanks("Hunter|Bite") etc.). Damage is the Classic base value the
// Forever overrides scale from; ranks with no Forever override use the client
// tooltip value, which then equals the Classic one. The top ranks keep the values
// the simulator has always used at level 60.
var petBiteRanks = []petAbilityRank{
	{17253, 1, 7, 9}, {17255, 8, 16, 18}, {17256, 16, 24, 28}, {17257, 24, 31, 37},
	{17258, 32, 40, 48}, {17259, 40, 49, 59}, {17260, 48, 66, 80}, {17261, 56, 81, 91},
}
var petClawRanks = []petAbilityRank{
	{16827, 1, 4, 6}, {16828, 8, 8, 12}, {16829, 16, 12, 16}, {16830, 24, 16, 22},
	{16831, 32, 21, 29}, {16832, 40, 26, 36}, {3010, 48, 35, 49}, {3009, 56, 43, 59},
}
var petLightningBreathRanks = []petAbilityRank{
	{24844, 1, 8, 10}, {25008, 12, 20, 22}, {25009, 24, 36, 41}, {25010, 36, 46, 54},
	{25011, 48, 78, 91}, {25012, 60, 99, 113},
}

// Demoralizing Screech. The level-60 rank keeps the effect id (24582) the simulator
// has always reported.
var petScreechRanks = []petAbilityRank{
	{24423, 8, 7, 9}, {24577, 24, 9, 13}, {24578, 48, 21, 27}, {24582, 56, 26, 46},
}
var petScorpidPoisonRanks = []petAbilityRank{
	{24640, 8, 1, 1}, {24583, 24, 3, 3}, {24586, 40, 6, 6}, {24587, 56, 8, 8},
}

// petRankAt returns the highest rank a pet of the given level knows.
func petRankAt(ranks []petAbilityRank, level int32) (petAbilityRank, bool) {
	best, ok := petAbilityRank{}, false
	for _, r := range ranks {
		if r.level <= level {
			best, ok = r, true
		}
	}
	return best, ok
}

func (hp *HunterPet) NewPetAbility(abilityType PetAbilityType, isPrimary bool) *core.Spell {
	switch abilityType {
	case Bite:
		return hp.newBite()
	case Claw:
		return hp.newClaw()
	case Screech:
		return hp.newScreech()
	// case FuriousHowl:
	// 	return hp.newFuriousHowl()
	case LightningBreath:
		return hp.newLightningBreath()
	case ScorpidPoison:
		return hp.newScorpidPoison()
	// case Swipe:
	// 	return hp.newSwipe()
	case Unknown:
		return nil
	default:
		panic("Invalid pet ability type")
	}
}

func (hp *HunterPet) newClaw() *core.Spell {
	rank, ok := petRankAt(petClawRanks, hp.Level)
	if !ok {
		return nil
	}
	spellID, baseDamageMin, baseDamageMax := rank.id, rank.min, rank.max

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellCode:   SpellCode_HunterPetClaw,
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics,

		FocusCost: core.FocusCostOptions{
			Cost: 25,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: PetGCD,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageMin, baseDamageMax)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}

func (hp *HunterPet) newBite() *core.Spell {
	rank, ok := petRankAt(petBiteRanks, hp.Level)
	if !ok {
		return nil
	}
	spellID, baseDamageMin, baseDamageMax := rank.id, rank.min, rank.max

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellCode:   SpellCode_HunterPetBite,
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagMeleeMetrics,

		FocusCost: core.FocusCostOptions{
			Cost: 35,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: PetGCD,
			},
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: 10 * time.Second,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageMin, baseDamageMax)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}

func (hp *HunterPet) newLightningBreath() *core.Spell {
	rank, ok := petRankAt(petLightningBreathRanks, hp.Level)
	if !ok {
		return nil
	}
	spellID, baseDamageMin, baseDamageMax := rank.id, rank.min, rank.max

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellCode:   SpellCode_HunterPetLightningBreath,
		SpellSchool: core.SpellSchoolNature,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamage,

		FocusCost: core.FocusCostOptions{
			Cost: 50,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: PetGCD,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,
		BonusCoefficient: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageMin, baseDamageMax)

			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMagicHitAndCrit)
		},
	})
}

func (hp *HunterPet) newScreech() *core.Spell {
	rank, ok := petRankAt(petScreechRanks, hp.Level)
	if !ok {
		return nil
	}
	spellID, baseDamageMin, baseDamageMax := rank.id, rank.min, rank.max

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellCode:   SpellCode_HunterPetScreech,
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeSpecial,
		Flags:       core.SpellFlagMeleeMetrics,

		FocusCost: core.FocusCostOptions{
			Cost: 20,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: PetGCD,
			},
			IgnoreHaste: true,
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			baseDamage := sim.Roll(baseDamageMin, baseDamageMax)
			// This ability also applies a melee attack power reduction similar to demoralizing shout - left it out for now
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeSpecialHitAndCrit)
		},
	})
}

// func (hp *HunterPet) newFuriousHowl() *core.Spell {
// 	actionID := core.ActionID{SpellID: 64495}

// 	petAura := hp.NewTemporaryStatsAura("FuriousHowl", actionID, stats.Stats{stats.AttackPower: 320, stats.RangedAttackPower: 320}, time.Second*20)
// 	ownerAura := hp.hunterOwner.NewTemporaryStatsAura("FuriousHowl", actionID, stats.Stats{stats.AttackPower: 320, stats.RangedAttackPower: 320}, time.Second*20)

// 	howlSpell := hp.RegisterSpell(core.SpellConfig{
// 		ActionID: actionID,

// 		FocusCost: core.FocusCostOptions{
// 			Cost: 20,
// 		},
// 		Cast: core.CastConfig{
// 			CD: core.Cooldown{
// 				Timer:    hp.NewTimer(),
// 				Duration: time.Second * 40,
// 			},
// 		},
// 		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
// 			return hp.IsEnabled()
// 		},
// 		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, _ *core.Spell) {
// 			petAura.Activate(sim)
// 			ownerAura.Activate(sim)
// 		},
// 	})

// 	hp.hunterOwner.RegisterSpell(core.SpellConfig{
// 		ActionID: actionID,
// 		Flags:    core.SpellFlagAPL | core.SpellFlagMCD,
// 		ExtraCastCondition: func(sim *core.Simulation, target *core.Unit) bool {
// 			return howlSpell.CanCast(sim, target)
// 		},
// 		ApplyEffects: func(sim *core.Simulation, target *core.Unit, _ *core.Spell) {
// 			howlSpell.Cast(sim, target)
// 		},
// 	})

// 	hp.hunterOwner.AddMajorCooldown(core.MajorCooldown{
// 		Spell: howlSpell,
// 		Type:  core.CooldownTypeDPS,
// 	})

// 	return nil
// }

func (hp *HunterPet) newScorpidPoison() *core.Spell {
	rank, ok := petRankAt(petScorpidPoisonRanks, hp.Level)
	if !ok {
		return nil
	}
	spellID, baseDamageTick := rank.id, rank.min

	return hp.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellCode:   SpellCode_HunterPetScorpidPoison,
		SpellSchool: core.SpellSchoolNature,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    core.ProcMaskMeleeMHSpecial,
		Flags:       core.SpellFlagPassiveSpell | core.SpellFlagPoison,

		FocusCost: core.FocusCostOptions{
			Cost: 30,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: PetGCD,
			},
			IgnoreHaste: true,
			CD: core.Cooldown{
				Timer:    hp.NewTimer(),
				Duration: time.Second * 4,
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label:     "ScorpidPoison",
				MaxStacks: 5,
				Duration:  time.Second * 10,
			},
			NumberOfTicks: 5,
			TickLength:    time.Second * 2,

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
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, dot.OutcomeTick)
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcAndDealOutcome(sim, target, spell.OutcomeMeleeSpecialHit)
			if !result.Landed() {
				return
			}

			dot := spell.Dot(target)
			dot.ApplyOrRefresh(sim)
			if dot.GetStacks() < dot.MaxStacks {
				dot.AddStack(sim)
				dot.TakeSnapshot(sim, true)
			}
		},
	})
}
