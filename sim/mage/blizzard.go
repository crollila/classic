package mage

import (
	"fmt"
	"time"

	"github.com/wowsims/classic/sim/core"
)

const BlizzardRanks = 6

var BlizzardSpellId = [BlizzardRanks + 1]int32{0, 10, 6141, 8427, 10185, 10186, 10187}
var BlizzardBaseDamage = [BlizzardRanks + 1]float64{0, 200, 352, 520, 720, 936, 1192}
var BlizzardManaCost = [BlizzardRanks + 1]float64{0, 320, 520, 720, 935, 1160, 1400}
var BlizzardLevel = [BlizzardRanks + 1]int{0, 20, 28, 36, 44, 52, 60}

func (mage *Mage) registerBlizzardSpell() {
	mage.Blizzard = make([]*core.Spell, BlizzardRanks+1)

	for rank := 1; rank <= BlizzardRanks; rank++ {
		config := mage.newBlizzardSpellConfig(rank)

		if config.RequiredLevel <= int(mage.Level) {
			mage.Blizzard[rank] = mage.GetOrRegisterSpell(config)
		}
	}
}

func (mage *Mage) newBlizzardSpellConfig(rank int) core.SpellConfig {
	numTicks := int32(8)
	tickLength := time.Second * 1

	spellId := BlizzardSpellId[rank]
	baseDamage := BlizzardBaseDamage[rank] / float64(numTicks)
	manaCost := BlizzardManaCost[rank]
	level := BlizzardLevel[rank]

	spellCoeff := .042

	var foreverChill core.AuraArray
	if mage.ForeverRank("mage.talent.improved-blizzard") > 0 {
		foreverChill = mage.NewEnemyAuraArray(func(t *core.Unit) *core.Aura {
			// Client: Improved Blizzard effect 0 and Permafrost effect 1 are negative slow
			// points; Permafrost effect 0 lengthens the chill (%).
			return t.ForeverSnareAura("Forever Blizzard chill-"+mage.Label, mage.ForeverAction("mage.talent.improved-blizzard"), time.Duration(1.5*float64(time.Second)*(1+mage.clientTalent("permafrost", 0, 0)/100)), -(mage.clientTalent("improved-blizzard", 0, 0)+mage.clientTalent("permafrost", 1, 0))/100)
		})
	}
	winterChance := 0.0
	if mage.ForeverRank("mage.talent.winter-s-chill") > 0 {
		winterChance = min(1, mage.clientTalent("winter-s-chill", 1, 0)/100)
	}
	var improvedBlizzardProcApplication *core.Spell
	if mage.Talents.ImprovedBlizzard > 0 {
		impId := []int32{0, 11185, 12487, 12488}[mage.Talents.ImprovedBlizzard]
		auras := mage.NewEnemyAuraArray(func(unit *core.Unit) *core.Aura {
			return unit.GetOrRegisterAura(core.Aura{
				ActionID: core.ActionID{SpellID: impId},
				Label:    "Improved Blizzard",
				Duration: time.Millisecond * 1500,
			})
		})
		improvedBlizzardProcApplication = mage.RegisterSpell(core.SpellConfig{
			ActionID: core.ActionID{SpellID: impId},
			ProcMask: core.ProcMaskSpellProc,
			Flags:    SpellFlagMage | core.SpellFlagNoLogs | SpellFlagChillSpell,
			ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
				auras.Get(target).Activate(sim)
			},
		})
	}

	return core.SpellConfig{
		ActionID:    core.ActionID{SpellID: spellId},
		SpellSchool: core.SpellSchoolFrost,
		ProcMask:    core.ProcMaskSpellDamage,
		Flags:       SpellFlagMage | core.SpellFlagChanneled | core.SpellFlagAPL,

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

		Dot: core.DotConfig{
			IsAOE: true,
			Aura: core.Aura{
				Label: fmt.Sprintf("Blizzard (Rank %d)", rank),
			},
			NumberOfTicks:    numTicks,
			TickLength:       tickLength,
			BonusCoefficient: spellCoeff,
			OnSnapshot: func(sim *core.Simulation, target *core.Unit, dot *core.Dot, isRollover bool) {
				dot.Snapshot(target, baseDamage, isRollover)
			},
			OnTick: func(sim *core.Simulation, target *core.Unit, dot *core.Dot) {
				for _, aoeTarget := range sim.Encounter.TargetUnits {
					dot.CalcAndDealPeriodicSnapshotDamage(sim, aoeTarget, dot.OutcomeTick)
					// Forever Winter's Chill: "your Frost damage spells" include Blizzard's ticks.
					if winterChance > 0 && sim.Proc(winterChance, "Forever Winter's Chill") {
						a := mage.foreverState.winter.Get(aoeTarget)
						a.Activate(sim)
						a.AddStack(sim)
					}
					if foreverChill != nil {
						foreverChill.Get(aoeTarget).Activate(sim)
						if sim.Proc(mage.clientTalent("frostbite", 0, 0)/100, "Forever Blizzard Frostbite") {
							mage.foreverState.frozen.Get(aoeTarget).Activate(sim)
						}
						if sim.Proc(mage.clientTalent("fingers-of-frost", 1, 0)/100, "Forever Blizzard Fingers of Frost") {
							a := mage.foreverState.fingers
							a.Activate(sim)
							a.SetStacks(sim, a.MaxStacks)
						}
					}

					if improvedBlizzardProcApplication != nil {
						improvedBlizzardProcApplication.Cast(sim, aoeTarget)
					}
				}
			},
		},

		DamageMultiplier: 1,
		ThreatMultiplier: 1,

		ApplyEffects: func(sim *core.Simulation, target *core.Unit, spell *core.Spell) {
			spell.AOEDot().Apply(sim)
		},
	}
}
