package warlock

import (
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

const CurseOfAgonyRanks = 6

func (warlock *Warlock) getCurseOfAgonyBaseConfig(rank int) core.SpellConfig {
	numTicks := int32(12)
	tickLength := time.Second * 2

	spellId := [CurseOfAgonyRanks + 1]int32{0, 980, 1014, 6217, 11711, 11712, 11713}[rank]
	spellCoeff := [CurseOfAgonyRanks + 1]float64{0, .046, .077, .083, .083, .083, .083}[rank]
	baseDamage := [CurseOfAgonyRanks + 1]float64{0, 7, 15, 27, 42, 65, 87}[rank] * (1 + .03*float64(warlock.Talents.ImprovedCurseOfAgony))
	manaCost := [CurseOfAgonyRanks + 1]float64{0, 25, 50, 90, 130, 170, 215}[rank]
	level := [CurseOfAgonyRanks + 1]int{0, 8, 18, 28, 38, 48, 58}[rank]

	baseDamage *= 1 + warlock.shadowMasteryBonus()
	snapshotBaseDmgNoBonus := 0.0

	return core.SpellConfig{
		SpellCode:     SpellCode_WarlockCurseOfAgony,
		ActionID:      core.ActionID{SpellID: spellId},
		SpellSchool:   core.SpellSchoolShadow,
		DefenseType:   core.DefenseTypeMagic,
		Flags:         core.SpellFlagAPL | core.SpellFlagResetAttackSwing | core.SpellFlagPureDot | WarlockFlagAffliction,
		ProcMask:      core.ProcMaskSpellDamage,
		RequiredLevel: level,
		Rank:          rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		CritDamageBonus: 0,

		DamageMultiplierAdditive: 1,
		DamageMultiplier:         1,
		ThreatMultiplier:         1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "CurseofAgony-" + warlock.Label + strconv.Itoa(rank),
			},
			NumberOfTicks:    numTicks,
			TickLength:       tickLength,
			BonusCoefficient: spellCoeff,

			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				baseDmg := baseDamage

				if warlock.AmplifyCurseAura.IsActive() {
					baseDmg *= 1.5
					warlock.AmplifyCurseAura.Deactivate(sim)
				}

				// CoA starts with 50% base damage, but bonus from spell power is not changed.
				// Every 4 ticks this base damage is added again, resulting in 150% base damage for the last 4 ticks
				snapshotBaseDmgNoBonus = baseDmg * 0.5

				dot.Snapshot(target, snapshotBaseDmgNoBonus, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, warlock.foreverDotOutcome(dot))
				if dot.TickCount%4 == 0 { // CoA ramp up
					dot.SnapshotBaseDamage += snapshotBaseDmgNoBonus
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				dot := spell.Dot(target)

				if activeCurse := warlock.ActiveCurseAura.Get(target); activeCurse != nil && activeCurse != dot.Aura {
					activeCurse.Deactivate(sim)
				}

				dot.Apply(sim)
				warlock.ActiveCurseAura[target.UnitIndex] = dot.Aura
			}
			spell.DealOutcome(sim, result)
		},
	}
}

func (warlock *Warlock) registerCurseOfAgonySpell() {
	warlock.CurseOfAgony = make([]*core.Spell, 0)
	for rank := 1; rank <= CurseOfAgonyRanks; rank++ {
		config := warlock.getCurseOfAgonyBaseConfig(rank)

		if config.RequiredLevel <= int(warlock.Level) {
			warlock.CurseOfAgony = append(warlock.CurseOfAgony, warlock.GetOrRegisterSpell(config))
		}
	}
}

func (warlock *Warlock) registerCurseOfRecklessnessSpell() {
	rank := rankAtLevel([]int{0, 14, 28, 42, 56}, warlock.Level)
	if rank == 0 {
		return
	}
	spellID := [5]int32{0, 704, 7658, 7659, 11717}[rank]
	manaCost := [5]float64{0, 35, 60, 90, 115}[rank]
	armor := [5]float64{0, 140, 290, 465, 640}[rank]
	ap := [5]float64{0, 20, 45, 65, 90}[rank]

	if rank == 4 {
		warlock.CurseOfRecklessnessAuras = warlock.NewEnemyAuraArray(core.CurseOfRecklessnessAura)
	} else {
		// Lower ranks: the same debuff with the rank's armor and attack power.
		warlock.CurseOfRecklessnessAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			return target.GetOrRegisterAura(core.Aura{
				Label:    "Curse of Recklessness",
				ActionID: core.ActionID{SpellID: spellID},
				Duration: time.Minute * 2,
				OnGain: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.AddStatDynamic(sim, stats.Armor, -armor)
					aura.Unit.AddStatDynamic(sim, stats.AttackPower, ap)
				},
				OnExpire: func(aura *core.Aura, sim *core.Simulation) {
					aura.Unit.AddStatDynamic(sim, stats.Armor, armor)
					aura.Unit.AddStatDynamic(sim, stats.AttackPower, -ap)
				},
			})
		})
	}

	warlock.CurseOfRecklessness = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL | WarlockFlagAffliction,
		Rank:        rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  156,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				aura := warlock.CurseOfRecklessnessAuras.Get(target)
				if activeCurse := warlock.ActiveCurseAura.Get(target); activeCurse != nil && activeCurse != aura {
					activeCurse.Deactivate(sim)
				}

				warlock.ActiveCurseAura[target.UnitIndex] = aura
				warlock.ActiveCurseAura.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfRecklessnessAuras},
	})
}

func (warlock *Warlock) registerCurseOfElementsSpell() {
	rank := rankAtLevel([]int{0, 32, 46, 60}, warlock.Level)
	if rank == 0 {
		return
	}
	spellID := [4]int32{0, 1490, 11721, 11722}[rank]
	manaCost := [4]float64{0, 100, 150, 200}[rank]

	if rank == 3 {
		warlock.CurseOfElementsAuras = warlock.NewEnemyAuraArray(core.CurseOfElementsAura)
	} else {
		resistance := [4]float64{0, 45, 60, 75}[rank]
		dmgMod := [4]float64{0, 1.06, 1.08, 1.1}[rank]
		warlock.CurseOfElementsAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			return schoolCurseAura(target, "Curse of Elements", spellID, resistance, dmgMod, stats.SchoolIndexFire, stats.SchoolIndexFrost)
		})
	}

	warlock.CurseOfElements = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL | WarlockFlagAffliction,
		Rank:        rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  156,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				aura := warlock.CurseOfElementsAuras.Get(target)
				if activeCurse := warlock.ActiveCurseAura.Get(target); activeCurse != nil && activeCurse != aura {
					activeCurse.Deactivate(sim)
				}

				warlock.ActiveCurseAura[target.UnitIndex] = aura
				warlock.ActiveCurseAura.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfElementsAuras},
	})
}

func (warlock *Warlock) registerCurseOfShadowSpell() {
	rank := rankAtLevel([]int{0, 44, 56}, warlock.Level)
	if rank == 0 {
		return
	}
	spellID := [3]int32{0, 17862, 17937}[rank]
	manaCost := [3]float64{0, 150, 200}[rank]

	if rank == 2 {
		warlock.CurseOfShadowAuras = warlock.NewEnemyAuraArray(core.CurseOfShadowAura)
	} else {
		warlock.CurseOfShadowAuras = warlock.NewEnemyAuraArray(func(target *core.Unit) *core.Aura {
			return schoolCurseAura(target, "Curse of Shadow", spellID, 60, 1.08, stats.SchoolIndexArcane, stats.SchoolIndexShadow)
		})
	}

	warlock.CurseOfShadow = warlock.RegisterSpell(core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellID},
		SpellSchool: core.SpellSchoolShadow,
		ProcMask:    core.ProcMaskEmpty,
		Flags:       core.SpellFlagAPL | WarlockFlagAffliction,
		Rank:        rank,

		ManaCost: core.ManaCostOptions{
			FlatCost: manaCost,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
		},

		ThreatMultiplier: 1,
		FlatThreatBonus:  156,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				aura := warlock.CurseOfShadowAuras.Get(target)
				if activeCurse := warlock.ActiveCurseAura.Get(target); activeCurse != nil && activeCurse != aura {
					activeCurse.Deactivate(sim)
				}

				warlock.ActiveCurseAura[target.UnitIndex] = aura
				warlock.ActiveCurseAura.Get(target).Activate(sim)
			}
		},

		RelatedAuras: []core.AuraArray{warlock.CurseOfShadowAuras},
	})
}

// schoolCurseAura is a lower rank of Curse of the Elements or Curse of Shadow, built with
// the same exclusive effects as the core's max-rank debuff so the strongest one applies.
func schoolCurseAura(target *core.Unit, label string, spellID int32, resistance float64, dmgMod float64, schools ...stats.SchoolIndex) *core.Aura {
	aura := target.GetOrRegisterAura(core.Aura{
		Label:    label,
		ActionID: core.ActionID{SpellID: spellID},
		Duration: time.Minute * 5,
	})
	for _, school := range schools {
		aura.NewExclusiveEffect("spellDamage"+strconv.Itoa(int(school)), false, core.ExclusiveEffect{
			Priority: dmgMod,
			OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				ee.Aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[school] *= dmgMod
			},
			OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				ee.Aura.Unit.PseudoStats.SchoolDamageTakenMultiplier[school] /= dmgMod
			},
		})
	}
	for _, school := range schools {
		aura.NewExclusiveEffect("resistance"+strconv.Itoa(int(school)), false, core.ExclusiveEffect{
			Priority: resistance,
			OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				aura.Unit.AddResistancesDynamic(sim, -resistance)
			},
			OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
				aura.Unit.AddResistancesDynamic(sim, resistance)
			},
		})
	}
	return aura
}

func (warlock *Warlock) registerAmplifyCurseSpell() {
	if !warlock.Talents.AmplifyCurse {
		return
	}

	actionID := core.ActionID{SpellID: 18288}

	warlock.AmplifyCurseAura = warlock.GetOrRegisterAura(core.Aura{
		Label:    "Amplify Curse",
		ActionID: actionID,
		Duration: time.Second * 30,
	})

	warlock.AmplifyCurse = warlock.GetOrRegisterSpell(core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolShadow,
		Flags:       core.SpellFlagAPL | WarlockFlagAffliction,

		Cast: core.CastConfig{
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: 3 * time.Minute,
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			warlock.AmplifyCurseAura.Activate(sim)
		},
	})
}

func (warlock *Warlock) registerCurseOfDoomSpell() {
	if warlock.Level < 60 {
		return
	}

	warlock.CurseOfDoom = warlock.RegisterSpell(core.SpellConfig{
		SpellCode:   SpellCode_WarlockCurseOfDoom,
		ActionID:    core.ActionID{SpellID: 603},
		SpellSchool: core.SpellSchoolShadow,
		DefenseType: core.DefenseTypeMagic,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       core.SpellFlagAPL | WarlockFlagAffliction,

		RequiredLevel: 60,

		ManaCost: core.ManaCostOptions{
			FlatCost: 300,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			CD: core.Cooldown{
				Timer:    warlock.NewTimer(),
				Duration: time.Second * 60,
			},
		},

		CritDamageBonus: 0,

		DamageMultiplier: 1,
		ThreatMultiplier: 1 - 0.1*float64(warlock.Talents.ImprovedDrainSoul),
		FlatThreatBonus:  160,
		BonusCoefficient: 1,

		Dot: core.DotConfig{
			Aura: core.Aura{
				Label: "CurseofDoom",
			},
			NumberOfTicks: 1,
			TickLength:    time.Minute,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, 3200, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				dot.CalcAndDealPeriodicSnapshotDamage(sim, target, warlock.foreverDotOutcome(dot))
			},
		},

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			result := spell.CalcOutcome(sim, target, spell.OutcomeMagicHitNoHitCounter)
			if result.Landed() {
				dot := spell.Dot(target)
				if activeCurse := warlock.ActiveCurseAura.Get(target); activeCurse != nil && activeCurse != dot.Aura {
					activeCurse.Deactivate(sim)
				}

				dot.Apply(sim)
				warlock.ActiveCurseAura[target.UnitIndex] = dot.Aura
			}
		},
	})
}
