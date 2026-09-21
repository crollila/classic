package druid

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

type DruidForm uint8

const (
	Humanoid DruidForm = 1 << iota
	Bear
	Cat
	Moonkin
	Any = Humanoid | Bear | Cat | Moonkin
)

func (form DruidForm) Matches(other DruidForm) bool {
	return (form & other) != 0
}

func (druid *Druid) GetForm() DruidForm {
	return druid.form
}

func (druid *Druid) InForm(form DruidForm) bool {
	return druid.form.Matches(form)
}

func (druid *Druid) GetCatWeapon() core.Weapon {
	return core.Weapon{
		BaseDamageMin:        43.84,
		BaseDamageMax:        65.76,
		SwingSpeed:           1.0,
		NormalizedSwingSpeed: 1.0,
		AttackPowerPerDPS:    core.DefaultAttackPowerPerDPS,
	}
}

// Func (druid *Druid) GetBearWeapon() core.Weapon {
// 	return core.Weapon{
// 		BaseDamageMin:        109,
// 		BaseDamageMax:        165,
// 		SwingSpeed:           2.5,
// 		NormalizedSwingSpeed: 2.5,
// 		AttackPowerPerDPS:    core.DefaultAttackPowerPerDPS,
// 	}
// }

// TODO: Class bonus stats for both cat and bear.
func (druid *Druid) GetFormShiftStats() stats.Stats {
	s := stats.Stats{
		stats.AttackPower: float64(druid.Talents.PredatoryStrikes) * 0.5 * float64(druid.Level),
		stats.MeleeCrit:   float64(druid.Talents.SharpenedClaws) * 2 * core.CritRatingPerCritChance,
	}
	/*
		if weapon := druid.GetMHWeapon(); weapon != nil {
			dps := (weapon.WeaponDamageMax+weapon.WeaponDamageMin)/2.0/weapon.SwingSpeed + druid.PseudoStats.BonusMHDps
			weapAp := weapon.Stats[stats.AttackPower] + weapon.Enchant.Stats[stats.AttackPower]
			fap := math.Floor((dps - 54.8) * 14)

			s[stats.AttackPower] += fap
			s[stats.AttackPower] += (fap + weapAp) * ((0.2 / 3) * float64(druid.Talents.PredatoryStrikes))
		}
	*/

	return s
}

func (druid *Druid) GetDynamicPredStrikeStats() stats.Stats {
	// Accounts for ap bonus for 'dynamic' enchants
	// just scourgebane currently, this is a bit hacky but is needed as the bonus varies based on current target
	// so has to be 'cached' differently
	s := stats.Stats{}
	if weapon := druid.GetMHWeapon(); weapon != nil {
		bonusAp := 0.0
		if weapon.Enchant.EffectID == 3247 && druid.CurrentTarget.MobType == proto.MobType_MobTypeUndead {
			bonusAp += 140
		}
		s[stats.AttackPower] += bonusAp * ((0.2 / 3) * float64(druid.Talents.PredatoryStrikes))
	}
	return s
}

// TODO: Classic feral and bear
func (druid *Druid) registerCatFormSpell() {
	actionID := core.ActionID{SpellID: 768}

	statBonus := druid.GetFormShiftStats().Add(stats.Stats{
		stats.AttackPower: float64(druid.Level) * 2,
	})
	if druid.Forever != nil {
		statBonus[stats.AttackPower] += druid.foreverPredatoryStrikesAP()
	}

	agiApDep := druid.NewDynamicStatDependency(stats.Agility, stats.AttackPower, 1)
	feralApDep := druid.NewDynamicStatDependency(stats.FeralAttackPower, stats.AttackPower, 1)

	var hotwDep *stats.StatDependency
	if druid.Talents.HeartOfTheWild > 0 {
		hotwDep = druid.NewDynamicMultiplyStat(stats.Strength, 1.0+0.04*float64(druid.Talents.HeartOfTheWild))
	}
	if druid.Forever != nil && druid.fr("heart-of-the-wild") > 0 {
		hotwDep = druid.NewDynamicMultiplyStat(stats.Strength, 1+.02*druid.fr("heart-of-the-wild"))
	}

	clawWeapon := druid.GetCatWeapon()

	predBonus := stats.Stats{}
	foreverArmor := 0.0

	druid.CatFormAura = druid.RegisterAura(core.Aura{
		Label:      "Cat Form",
		ActionID:   actionID,
		Duration:   core.NeverExpires,
		BuildPhase: core.Ternary(druid.StartingForm.Matches(Cat), core.CharacterBuildPhaseBase, core.CharacterBuildPhaseNone),
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if !druid.Env.MeasuringStats && druid.form != Humanoid {
				druid.CancelShapeshift(sim)
			}
			druid.form = Cat
			if druid.Forever != nil {
				druid.AddStatDynamic(sim, stats.MeleeCrit, 3*druid.fr("sharpened-claws"))
				if druid.fr("thick-hide") > 0 {
					foreverArmor = float64(druid.Level) + druid.ForeverValue("druid.talent.thick-hide", 1, .67)*max(0, druid.GetStat(stats.Defense))
					druid.AddStatDynamic(sim, stats.Armor, foreverArmor)
				}
				if druid.MovementHandler != nil {
					druid.AddMoveSpeedModifier(&aura.ActionID, 1+.15*druid.fr("feral-swiftness"))
				}
				druid.foreverFormCrit(sim)
			}
			druid.SetCurrentPowerBar(core.EnergyBar)

			druid.AutoAttacks.SetMH(clawWeapon)

			druid.PseudoStats.ThreatMultiplier *= 0.71
			druid.AddStatDynamic(sim, stats.Dodge, 2*float64(druid.Talents.FelineSwiftness))
			druid.SetShapeshift(aura)
			if druid.Forever != nil {
				druid.foreverShiftDispel(sim)
			}

			predBonus = druid.GetDynamicPredStrikeStats()
			druid.AddStatsDynamic(sim, predBonus)
			druid.AddStatsDynamic(sim, statBonus)
			druid.EnableDynamicStatDep(sim, agiApDep)
			druid.EnableDynamicStatDep(sim, feralApDep)
			if hotwDep != nil {
				druid.EnableDynamicStatDep(sim, hotwDep)
			}

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(nil)
				druid.AutoAttacks.EnableAutoSwing(sim)
				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			if druid.Forever != nil {
				if prowl := druid.GetAura("Forever Prowl"); prowl != nil {
					prowl.Deactivate(sim)
				}
				druid.foreverLastCatEnergy = druid.CurrentEnergy()
				druid.foreverHumanoidSince = sim.CurrentTime
				druid.foreverHumanoidTime = 0
				druid.AddStatDynamic(sim, stats.MeleeCrit, -3*druid.fr("sharpened-claws"))
				druid.AddStatDynamic(sim, stats.Armor, -foreverArmor)
				if druid.MovementHandler != nil {
					druid.RemoveMoveSpeedModifier(&aura.ActionID)
				}
			}
			druid.form = Humanoid
			if druid.Forever != nil {
				druid.foreverFormCrit(sim)
			}
			druid.SetCurrentPowerBar(core.ManaBar)

			if druid.TigersFuryAura != nil {
				druid.TigersFuryAura.Deactivate(sim)
			}

			druid.AutoAttacks.SetMH(druid.WeaponFromMainHand())

			druid.PseudoStats.ThreatMultiplier /= 0.71
			druid.AddStatDynamic(sim, stats.Dodge, -2*float64(druid.Talents.FelineSwiftness))
			druid.SetShapeshift(nil)

			druid.AddStatsDynamic(sim, predBonus.Invert())
			druid.AddStatsDynamic(sim, statBonus.Invert())
			druid.DisableDynamicStatDep(sim, agiApDep)
			druid.DisableDynamicStatDep(sim, feralApDep)
			if hotwDep != nil {
				druid.DisableDynamicStatDep(sim, hotwDep)
			}

			if !druid.Env.MeasuringStats {
				druid.AutoAttacks.SetReplaceMHSwing(nil)
				druid.AutoAttacks.EnableAutoSwing(sim)
				druid.manageCooldownsEnabled()
				druid.UpdateManaRegenRates()

				//druid.TigersFuryAura.Deactivate(sim)
			}
		},
	})

	energyMetrics := druid.NewEnergyMetrics(actionID)

	furorProcChance := 0.2 * float64(druid.Talents.Furor)

	hasWolfheadBonus := false
	if head := druid.Equipment.Head(); head != nil && (head.ID == WolfsheadHelm) {
		hasWolfheadBonus = true
	}

	druid.CatForm = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   0.55,
			Multiplier: 100 - 10*druid.Talents.NaturalShapeshifter,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
			ModifyCast: func(sim *core.Simulation, spell *core.Spell, cast *core.Cast) {
				if druid.CatFormAura.IsActive() {
					cast.GCD = 0
					spell.Cost.Multiplier -= 100
				}
			},
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			if druid.CatFormAura.IsActive() {
				druid.CancelShapeshift(sim)
				spell.Cost.Multiplier += 100
			} else {
				maxShiftEnergy := core.TernaryFloat64(sim.RandomFloat("Furor") < furorProcChance, 40, 0)
				if druid.Forever != nil {
					outside := druid.foreverHumanoidTime
					if druid.InForm(Humanoid | Moonkin) {
						outside += sim.CurrentTime - druid.foreverHumanoidSince
					}
					maxShiftEnergy = druid.foreverFurorEnergy(outside)
					druid.foreverHumanoidTime = 0
				}
				maxShiftEnergy = core.TernaryFloat64(hasWolfheadBonus, maxShiftEnergy+20, maxShiftEnergy)
				energyDelta := maxShiftEnergy - druid.CurrentEnergy()

				if energyDelta > 0 {
					druid.AddEnergy(sim, energyDelta, energyMetrics)
				} else {
					druid.SpendEnergy(sim, -energyDelta, energyMetrics)
				}

				druid.CatFormAura.Activate(sim)
			}
		},
	})
}

// func (druid *Druid) registerBearFormSpell() {
// 	actionID := core.ActionID{SpellID: 9634}
// 	healthMetrics := druid.NewHealthMetrics(actionID)

// 	statBonus := druid.GetFormShiftStats().Add(stats.Stats{
// 		stats.AttackPower: 3 * float64(druid.Level),
// 	})

// 	stamDep := druid.NewDynamicMultiplyStat(stats.Stamina, 1.25)

// 	var potpDep *stats.StatDependency
// 	if druid.Talents.ProtectorOfThePack > 0 {
// 		potpDep = druid.NewDynamicMultiplyStat(stats.AttackPower, 1.0+0.02*float64(druid.Talents.ProtectorOfThePack))
// 	}

// 	var hotwDep *stats.StatDependency
// 	if druid.Talents.HeartOfTheWild > 0 {
// 		hotwDep = druid.NewDynamicMultiplyStat(stats.Stamina, 1.0+0.02*float64(druid.Talents.HeartOfTheWild))
// 	}

// 	potpdtm := 1 - 0.04*float64(druid.Talents.ProtectorOfThePack)

// 	clawWeapon := druid.GetBearWeapon()
// 	predBonus := stats.Stats{}

// 	druid.BearFormAura = druid.RegisterAura(core.Aura{
// 		Label:      "Bear Form",
// 		ActionID:   actionID,
// 		Duration:   core.NeverExpires,
// 		BuildPhase: core.Ternary(druid.StartingForm.Matches(Bear), core.CharacterBuildPhaseBase, core.CharacterBuildPhaseNone),
// 		OnGain: func(aura *core.Aura, sim *core.Simulation) {
// 			if !druid.Env.MeasuringStats && druid.form != Humanoid {
// 				druid.CancelShapeshift(sim)
// 			}
// 			druid.form = Bear
// 			druid.SetCurrentPowerBar(core.RageBar)

// 			druid.AutoAttacks.SetMH(clawWeapon)

// 			druid.PseudoStats.ThreatMultiplier *= 2.1021
// 			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] *= 1.0 + 0.02*float64(druid.Talents.MasterShapeshifter)
// 			druid.PseudoStats.DamageTakenMultiplier *= potpdtm
//			Switch to using AddStat as PseudoStat is being removed
// 			druid.PseudoStats.BaseDodge += 0.02 * float64(druid.Talents.FeralSwiftness+druid.Talents.NaturalReaction)

// 			predBonus = druid.GetDynamicPredStrikeStats()
// 			druid.AddStatsDynamic(sim, predBonus)
// 			druid.AddStatsDynamic(sim, statBonus)
// 			druid.ApplyDynamicEquipScaling(sim, stats.Armor, druid.BearArmorMultiplier())
// 			if potpDep != nil {
// 				druid.EnableDynamicStatDep(sim, potpDep)
// 			}

// 			// Preserve fraction of max health when shifting
// 			healthFrac := druid.CurrentHealth() / druid.MaxHealth()
// 			druid.EnableDynamicStatDep(sim, stamDep)
// 			if hotwDep != nil {
// 				druid.EnableDynamicStatDep(sim, hotwDep)
// 			}
// 			druid.GainHealth(sim, healthFrac*druid.MaxHealth()-druid.CurrentHealth(), healthMetrics)

// 			if !druid.Env.MeasuringStats {
// 				druid.AutoAttacks.SetReplaceMHSwing(druid.ReplaceBearMHFunc)
// 				druid.AutoAttacks.EnableAutoSwing(sim)

// 				druid.manageCooldownsEnabled()
// 				druid.UpdateManaRegenRates()
// 			}
// 		},
// 		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
// 			druid.form = Humanoid
// 			druid.AutoAttacks.SetMH(druid.WeaponFromMainHand())

// 			druid.PseudoStats.ThreatMultiplier /= 2.1021
// 			druid.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical] /= 1.0 + 0.02*float64(druid.Talents.MasterShapeshifter)
// 			druid.PseudoStats.DamageTakenMultiplier /= potpdtm
//			Switch to using AddStat as PseudoStat is being removed
// 			druid.PseudoStats.BaseDodge -= 0.02 * float64(druid.Talents.FeralSwiftness+druid.Talents.NaturalReaction)

// 			druid.AddStatsDynamic(sim, predBonus.Invert())
// 			druid.AddStatsDynamic(sim, statBonus.Invert())
// 			druid.RemoveDynamicEquipScaling(sim, stats.Armor, druid.BearArmorMultiplier())
// 			if potpDep != nil {
// 				druid.DisableDynamicStatDep(sim, potpDep)
// 			}

// 			healthFrac := druid.CurrentHealth() / druid.MaxHealth()
// 			druid.DisableDynamicStatDep(sim, stamDep)
// 			if hotwDep != nil {
// 				druid.DisableDynamicStatDep(sim, hotwDep)
// 			}
// 			druid.RemoveHealth(sim, druid.CurrentHealth()-healthFrac*druid.MaxHealth())

// 			if !druid.Env.MeasuringStats {
// 				druid.AutoAttacks.SetReplaceMHSwing(nil)
// 				druid.AutoAttacks.EnableAutoSwing(sim)

// 				druid.manageCooldownsEnabled()
// 				druid.UpdateManaRegenRates()
// 				druid.EnrageAura.Deactivate(sim)
// 				druid.MaulQueueAura.Deactivate(sim)
// 			}
// 		},
// 	})

// 	rageMetrics := druid.NewRageMetrics(actionID)

// 	furorProcChance := []float64{0, 0.2, 0.4, 0.6, 0.8, 1}[druid.Talents.Furor]

// 	druid.BearForm = druid.RegisterSpell(Any, core.SpellConfig{
// 		ActionID: actionID,
// 		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

// 		ManaCost: core.ManaCostOptions{
// 			BaseCost:   0.55,
// 			Multiplier: (1 - 0.2*float64(druid.Talents.KingOfTheJungle)) * (1 - 0.1*float64(druid.Talents.NaturalShapeshifter)),
// 		},
// 		Cast: core.CastConfig{
// 			DefaultCast: core.Cast{
// 				GCD: core.GCDDefault,
// 			},
// 			IgnoreHaste: true,
// 		},

// 		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
// 			rageDelta := 0 - druid.CurrentRage()
// 			if sim.Proc(furorProcChance, "Furor") {
// 				rageDelta += 10
// 			}
// 			if rageDelta > 0 {
// 				druid.AddRage(sim, rageDelta, rageMetrics)
// 			} else if rageDelta < 0 {
// 				druid.SpendRage(sim, -rageDelta, rageMetrics)
// 			}
// 			druid.BearFormAura.Activate(sim)
// 		},
// 	})
// }

func (druid *Druid) manageCooldownsEnabled() {
	// Disable cooldowns not usable in form and/or delay others
	if druid.StartingForm.Matches(Cat | Bear) {
		for _, mcd := range druid.disabledMCDs {
			mcd.Enable()
		}
		druid.disabledMCDs = nil

		if druid.InForm(Humanoid) {
			// Disable cooldown that incurs a gcd, so we dont get stuck out of form when we dont need to (Greater Drums)
			for _, mcd := range druid.GetMajorCooldowns() {
				if mcd.Spell.DefaultCast.GCD > 0 {
					mcd.Disable()
					druid.disabledMCDs = append(druid.disabledMCDs, mcd)
				}
			}
		}
	}
}

// https://www.wowhead.com/classic/spell=24858/moonkin-form
// - Moonfire costs 50% less mana and deals 50% more damage over time
// - Sunfire costs 50% less mana and deals 50% more damage over time
// - Your periodic damage spells can deal critical periodic damage (handled in individual dot snapshots)
// - You gain (2 * Level) spell damage
func (druid *Druid) registerMoonkinFormSpell() {
	if !druid.Talents.MoonkinForm {
		return
	}

	actionID := core.ActionID{SpellID: 24858}

	druid.MoonkinFormAura = druid.RegisterAura(core.Aura{
		Label:    "Moonkin Form",
		ActionID: actionID,
		Duration: core.NeverExpires,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if !druid.Env.MeasuringStats && druid.form != Humanoid {
				druid.CancelShapeshift(sim)
			}
			druid.form = Moonkin
			if druid.Forever != nil {
				druid.foreverShiftDispel(sim)
				druid.SetShapeshift(aura)
				druid.ApplyDynamicEquipScaling(sim, stats.Armor, 4.6)
				if druid.fr("thick-hide") > 0 {
					druid.AddStatDynamic(sim, stats.Armor, (float64(druid.Level)+druid.ForeverValue("druid.talent.thick-hide", 1, .67)*druid.GetStat(stats.Defense))*4.6)
				}
				druid.foreverFormCrit(sim)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			druid.form = Humanoid
			if druid.Forever != nil {
				druid.SetShapeshift(nil)
				druid.RemoveDynamicEquipScaling(sim, stats.Armor, 4.6)
				if druid.fr("thick-hide") > 0 {
					druid.AddStatDynamic(sim, stats.Armor, -(float64(druid.Level)+druid.ForeverValue("druid.talent.thick-hide", 1, .67)*druid.GetStat(stats.Defense))*4.6)
				}
				druid.foreverFormCrit(sim)
			}
		},
	})

	druid.MoonkinForm = druid.RegisterSpell(Any, core.SpellConfig{
		ActionID: actionID,
		Flags:    core.SpellFlagNoOnCastComplete | core.SpellFlagAPL,

		ManaCost: core.ManaCostOptions{
			BaseCost:   core.TernaryFloat64(druid.Forever == nil, 0.35, 0),
			FlatCost:   core.TernaryFloat64(druid.Forever != nil, 283, 0),
			Multiplier: 100 - 10*druid.Talents.NaturalShapeshifter,
		},
		Cast: core.CastConfig{
			DefaultCast: core.Cast{
				GCD: core.GCDDefault,
			},
			IgnoreHaste: true,
		},

		ApplyEffects: func(sim *core.Simulation, _ *core.Unit, spell *core.Spell) {
			druid.MoonkinFormAura.Activate(sim)
		},
	})
}
