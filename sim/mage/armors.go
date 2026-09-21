package mage

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

type mageArmorRank struct {
	spellID         int32
	level           int32
	armor, frostRes float64
	resistance      float64
}

// Frost Armor ranks 1-3 then Ice Armor ranks 1-4, as learned from the trainer.
var frostIceArmorRanks = []mageArmorRank{
	{spellID: 168, level: 1, armor: 30},
	{spellID: 7300, level: 10, armor: 110},
	{spellID: 7301, level: 20, armor: 200},
	{spellID: 7302, level: 30, armor: 290, frostRes: 6},
	{spellID: 7320, level: 40, armor: 380, frostRes: 9},
	{spellID: 10219, level: 50, armor: 470, frostRes: 12},
	{spellID: 10220, level: 60, armor: 560, frostRes: 15},
}

var mageArmorRanks = []mageArmorRank{
	{spellID: 6117, level: 34, resistance: 5},
	{spellID: 22782, level: 46, resistance: 10},
	{spellID: 22783, level: 58, resistance: 15},
}

func (mage *Mage) applyFrostIceArmor() {
	// Frost Armor (ranks 1-3) until Ice Armor is learned at 30; both share one aura.
	var spellID int32
	var armor, frostRes float64
	for _, r := range frostIceArmorRanks {
		if r.level <= mage.Level {
			spellID, armor, frostRes = r.spellID, r.armor, r.frostRes
		}
	}
	if spellID == 0 {
		return
	}

	armor *= 1 + mage.clientTalent("frost-warding", 0, 0)/100
	frostRes *= 1 + mage.clientTalent("frost-warding", 0, 0)/100
	mage.IceArmorAura = core.MakePermanent(mage.RegisterAura(core.Aura{
		Label:    "Ice Armor",
		ActionID: core.ActionID{SpellID: spellID},
		// BuildPhase: core.CharacterBuildPhaseBuffs,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				mage.AddStat(stats.BonusArmor, armor)
				mage.AddStat(stats.FrostResistance, frostRes)
			} else {
				mage.AddStatDynamic(sim, stats.Armor, armor)
				mage.AddStatDynamic(sim, stats.FrostResistance, frostRes)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				mage.AddStat(stats.BonusArmor, -1*armor)
				mage.AddStat(stats.FrostResistance, -1*frostRes)
			} else {
				mage.AddStatDynamic(sim, stats.Armor, -1*armor)
				mage.AddStatDynamic(sim, stats.FrostResistance, -1*frostRes)
			}
		},
	}))
}

func (mage *Mage) applyMageArmor() {
	var spellID int32
	var spellRes float64
	for _, r := range mageArmorRanks {
		if r.level <= mage.Level {
			spellID, spellRes = r.spellID, r.resistance
		}
	}
	if spellID == 0 {
		return
	}

	spellRes *= 1 + mage.clientTalent("arcane-shielding", 1, 0)/100
	mage.MageArmorAura = core.MakePermanent(mage.RegisterAura(core.Aura{
		Label:      "Mage Armor",
		ActionID:   core.ActionID{SpellID: spellID},
		BuildPhase: core.CharacterBuildPhaseBuffs,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			mage.PseudoStats.SpiritRegenRateCasting += .3

			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				mage.AddResistances(spellRes)
			} else {
				mage.AddResistancesDynamic(sim, spellRes)
			}
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			mage.PseudoStats.SpiritRegenRateCasting -= .3

			if aura.Unit.Env.MeasuringStats && aura.Unit.Env.State != core.Finalized {
				mage.AddResistances(-1 * spellRes)
			} else {
				mage.AddResistancesDynamic(sim, -1*spellRes)
			}
		},
	}))
}
