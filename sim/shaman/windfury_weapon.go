package shaman

import (
	"time"

	"github.com/wowsims/classic/sim/core"
)

const WindfuryWeaponRanks = 4

var WindfuryWeaponSpellId = [WindfuryWeaponRanks + 1]int32{0, 8232, 8235, 10486, 16362}
var WindfuryWeaponEnchantId = [WindfuryWeaponRanks + 1]int32{0, 283, 284, 525, 1669}
var WindfuryWeaponBonusAP = [WindfuryWeaponRanks + 1]float64{0, 104, 119, 249, 333}
var WindfuryWeaponLevel = [WindfuryWeaponRanks + 1]int32{0, 30, 40, 50, 60}


func (shaman *Shaman) newWindfuryImbueSpell(isMH bool) *core.Spell {
	rank := rankAtLevel(WindfuryWeaponLevel[:], shaman.Level)

	ewMultiplier := []float64{1, 1.13, 1.27, 1.4}[shaman.Talents.ElementalWeapons]
	bonusAP := WindfuryWeaponBonusAP[rank]
	// Classic deliberately double-dips Elemental Weapons (the in-game Era bug). Forever's
	// tooltip is a single +13/27/40% to the Windfury effect.
	ewApplied := ewMultiplier * ewMultiplier
	if shaman.Forever != nil {
		ewApplied = ewMultiplier
	}

	actionID := core.ActionID{SpellID: WindfuryWeaponSpellId[rank]}.WithTag(core.TernaryInt32(isMH, 1, 2))
	procMask := core.ProcMaskMeleeMHSpecial
	damageMultiplier := 1.0
	weaponDamageFunc := shaman.MHWeaponDamage
	if !isMH {
		procMask = core.ProcMaskMeleeOHSpecial
		damageMultiplier = shaman.AutoAttacks.OHConfig().DamageMultiplier
		weaponDamageFunc = shaman.OHWeaponDamage
	}

	spellConfig := core.SpellConfig{
		ActionID:    actionID,
		SpellSchool: core.SpellSchoolPhysical,
		DefenseType: core.DefenseTypeMelee,
		ProcMask:    procMask,
		Flags:       core.SpellFlagMeleeMetrics | core.SpellFlagNoOnCastComplete | core.SpellFlagPassiveSpell,

		DamageMultiplier: damageMultiplier,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			mAP := spell.MeleeAttackPower(target) + bonusAP*ewApplied
			baseDamage := weaponDamageFunc(sim, mAP)
			spell.CalcAndDealDamage(sim, target, baseDamage, spell.OutcomeMeleeWeaponSpecialHitAndCrit)
		},
	}

	return shaman.RegisterSpell(spellConfig)
}

func (shaman *Shaman) RegisterWindfuryImbue(procMask core.ProcMask) {
	rank := rankAtLevel(WindfuryWeaponLevel[:], shaman.Level)
	if procMask == core.ProcMaskUnknown || rank == 0 {
		return
	}

	enchantId := WindfuryWeaponEnchantId[rank]

	icdDuration := time.Millisecond * 1500

	if procMask.Matches(core.ProcMaskMeleeMH) {
		shaman.MainHand().TempEnchant = enchantId
	}
	if procMask.Matches(core.ProcMaskMeleeOH) {
		shaman.OffHand().TempEnchant = enchantId
	}

	var proc = 0.2
	if procMask == core.ProcMaskMelee {
		proc = 0.36
	}

	icd := core.Cooldown{
		Timer:    shaman.NewTimer(),
		Duration: icdDuration,
	}

	shaman.WindfuryWeaponMH = shaman.newWindfuryImbueSpell(true)

	aura := shaman.RegisterAura(core.Aura{
		Label:    "Windfury Imbue",
		Duration: core.NeverExpires,
		OnReset: func(aura *core.Aura, sim *core.Simulation) {
			aura.Activate(sim)
		},
		OnSpellHitDealt: func(aura *core.Aura, sim *core.Simulation, spell *core.Spell, result *core.SpellResult) {
			if !result.Landed() || !spell.ProcMask.Matches(procMask) || spell.Flags.Matches(core.SpellFlagSuppressEquipProcs) {
				return
			}

			if !icd.IsReady(sim) {
				return
			}

			if sim.RandomFloat("Windfury Imbue") < proc {
				icd.Use(sim)

				// TODO: Vanilla uses two extra attacks but SoD replaced this with yellow hits
				// This needs to be refactored
				shaman.WindfuryWeaponMH.Cast(sim, result.Target)
				shaman.WindfuryWeaponMH.Cast(sim, result.Target)
			}
		},
	})

	shaman.RegisterOnItemSwapWithImbue(enchantId, &procMask, aura)
}

func (shaman *Shaman) ApplyWindfuryImbue(procMask core.ProcMask) {
	if procMask.Matches(core.ProcMaskMeleeMH) && shaman.HasMHWeapon() {
		shaman.ApplyWindfuryImbueToItem(shaman.MainHand())
	}

	if procMask.Matches(core.ProcMaskMeleeOH) && shaman.HasOHWeapon() {
		shaman.ApplyWindfuryImbueToItem(shaman.OffHand())
	}
}

func (shaman *Shaman) ApplyWindfuryImbueToItem(item *core.Item) {
	rank := rankAtLevel(WindfuryWeaponLevel[:], shaman.Level)
	if item == nil || rank == 0 {
		return
	}

	enchantId := WindfuryWeaponEnchantId[rank]

	item.TempEnchant = enchantId
}
