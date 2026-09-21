package warrior

import (
	"math"
	"strconv"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/stats"
)

// Per-rank values of the warrior's trainer spells, lowest rank first, parallel to
// core.TrainerRanks("Warrior|<name>") (spell ids and learned levels from the Forever
// beta client). The values are the Classic ones the sim has always registered at
// level 60: the Forever override layer (core/foreverdata) is keyed by spell id and
// scales from these Classic values, so registering the right rank id with its
// Classic number is what lets a Forever change apply.

// Heroic Strike: flat bonus damage and bonus threat. Threat below rank 8 has no known
// equation; the lower ranks are the commonly quoted Classic values.
var heroicStrikeBonus = []float64{11, 21, 32, 44, 58, 80, 111, 138, 157}
var heroicStrikeThreat = []float64{20, 39, 59, 78, 98, 118, 137, 145, 173}

// Cleave: flat bonus damage; bonus threat (rank 5: 100) scaled with the bonus below rank 5.
var cleaveBonus = []float64{5, 10, 18, 32, 50}
var cleaveThreat = []float64{10, 20, 36, 64, 100}

// Rend: damage per 3 sec tick and tick count (tooltip total over 9/12/15/18/21 sec).
var rendTick = []float64{5, 7, 9, 11, 14, 18, 21}
var rendTicks = []int32{3, 4, 5, 6, 7, 7, 7}

// Execute: {flat damage, damage per extra rage}.
var executeValues = [][2]float64{{125, 3}, {200, 6}, {325, 9}, {450, 12}, {600, 15}}

// Overpower: flat bonus damage.
var overpowerBonus = []float64{5, 15, 25, 35}

// Slam: flat bonus damage. Rank 1 (1240193, level 20) is a Forever-only rank.
var slamBonus = []float64{16, 32, 43, 68, 87}

// Hamstring: damage.
var hamstringDamage = []float64{5, 18, 45}

// Mortal Strike: flat bonus damage (Classic and Forever agree).
var mortalStrikeBonus = []float64{85, 110, 135, 160}

// Bloodthirst (Forever): 35% of attack power plus this, by rank. Classic is 45% AP flat.
var bloodthirstBonusForever = []float64{30, 37, 43, 48}

// Shield Slam: {min, max} damage (Classic) and threat.
var shieldSlamDamage = [][2]float64{{225, 235}, {264, 276}, {303, 317}, {342, 358}}

// Shield Slam (Forever): {min, max} damage, plus Block Value once.
var shieldSlamDamageForever = [][2]float64{{421, 439}, {494, 516}, {567, 593}, {640, 670}}

// Revenge (Forever): {min, max} damage by rank; Classic's are in revenge.go.
var revengeDamageForever = [][2]float64{{20, 24}, {31, 37}, {43, 53}, {73, 91}, {109, 133}, {138, 168}}

// Intercept (Forever client): damage by rank.
var interceptDamage = []float64{25, 45, 65}
var shieldSlamThreat = []float64{160, 190, 220, 254}

// Thunder Clap: damage and slow duration.
var thunderClapDamage = []float64{10, 23, 37, 55, 82, 103}
var thunderClapDuration = []time.Duration{10, 14, 18, 22, 26, 30}

// Sunder Armor: armor per stack.
var sunderArmorPerStack = []float64{90, 180, 270, 360, 450}

// Battle Shout: attack power, from the Classic client (the datamine behind overrides.json).
var battleShoutAP = []float64{15, 35, 55, 85, 130, 185, 232}

// Demoralizing Shout: attack power reduction, from the Classic client (overrides.json).
var demoralizingShoutAP = []float64{35, 55, 70, 105, 140}

// Pummel: damage.
var pummelDamage = []float64{20, 50}

// Charge: rage generated.
var chargeRage = []float64{9, 12, 15}

// trainerRank is the highest rank of a warrior trainer spell the warrior knows at its
// level (1-based) and its spell id; rank 0 when the spell is not learned yet.
func (warrior *Warrior) trainerRank(family string) (int, int32) {
	return core.TrainerRankAt("Warrior|"+family, warrior.Level)
}

// trainerRankCount is the number of ranks of a warrior trainer spell.
func trainerRankCount(family string) int {
	return len(core.TrainerRanks("Warrior|" + family))
}

// Single-rank warrior abilities and the level they are learned at (Forever client).
const (
	levelDefensiveStance = 10
	levelBerserkerStance = 30
	levelBloodrage       = 10
	levelShieldBlock     = 16
	levelShieldWall      = 28
	levelWhirlwind       = 36
	levelDisarm          = 18
)

// The core buff/debuff auras are the level-60 ranks. Below them the warrior's own ranks
// are built here, on the same labels and exclusive-effect categories so they interact
// with other sources of the same effect exactly as the level-60 auras do.

func battleShoutAuraAtRank(unit *core.Unit, rank int32, impBattleShout int32, boomingVoicePts int32, has3pcWrath bool) *core.Aura {
	spellID := core.BattleShoutSpellId[rank]
	baseAP := unit.ForeverSpellValue(spellID, 0, battleShoutAP[rank-1])
	bonus := stats.Stats{stats.AttackPower: math.Floor(baseAP*(1+0.05*float64(impBattleShout)) + core.TernaryFloat64(has3pcWrath, 30, 0))}
	return unit.GetOrRegisterAura(core.Aura{
		Label:      "Battle Shout",
		ActionID:   core.ActionID{SpellID: spellID},
		Duration:   time.Duration(float64(time.Minute*2) * (1 + 0.1*float64(boomingVoicePts))),
		BuildPhase: core.CharacterBuildPhaseBuffs,
		OnGain: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.AddStatsDynamic(sim, bonus)
		},
		OnExpire: func(aura *core.Aura, sim *core.Simulation) {
			aura.Unit.AddStatsDynamic(sim, bonus.Invert())
		},
	})
}

func demoralizingShoutAuraAtRank(target *core.Unit, rank int32, boomingVoicePts int32, impDemoShoutPts int32) *core.Aura {
	spellID := core.DemoralizingShoutSpellId[rank]
	apReduction := math.Floor(demoralizingShoutAP[rank-1] * (1 + 0.08*float64(impDemoShoutPts)))
	reduction := stats.Stats{stats.AttackPower: -apReduction}
	aura := target.GetOrRegisterAura(core.Aura{
		Label:    "DemoralizingShout-" + strconv.Itoa(int(impDemoShoutPts)) + "-rank" + strconv.Itoa(int(rank)),
		ActionID: core.ActionID{SpellID: spellID},
		Duration: time.Duration(float64(time.Second*30) * (1 + 0.1*float64(boomingVoicePts))),
	})
	aura.NewExclusiveEffect("APReduction", false, core.ExclusiveEffect{
		Priority: apReduction,
		OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatsDynamic(sim, reduction)
		},
		OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatsDynamic(sim, reduction.Invert())
		},
	})
	return aura
}

func sunderArmorAuraAtRank(target *core.Unit, rank int, spellID int32) *core.Aura {
	arpen := sunderArmorPerStack[rank-1]
	var effect *core.ExclusiveEffect
	aura := target.GetOrRegisterAura(core.Aura{
		Label:     "Sunder Armor-rank" + strconv.Itoa(rank),
		ActionID:  core.ActionID{SpellID: spellID},
		Duration:  time.Second * 30,
		MaxStacks: 5,
		OnStacksChange: func(aura *core.Aura, sim *core.Simulation, oldStacks int32, newStacks int32) {
			effect.SetPriority(sim, arpen*float64(newStacks))
		},
	})
	effect = aura.NewExclusiveEffect("MajorArmorReduction", true, core.ExclusiveEffect{
		Priority: 0,
		OnGain: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, -ee.Priority)
		},
		OnExpire: func(ee *core.ExclusiveEffect, sim *core.Simulation) {
			ee.Aura.Unit.AddStatDynamic(sim, stats.Armor, ee.Priority)
		},
	})
	return aura
}
