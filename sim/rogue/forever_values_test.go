package rogue

import (
	"math"
	"testing"

	"github.com/wowsims/classic/sim/core"
)

// The Forever client's Rupture tooltips: total damage at 1-5 combo points (8-16 sec), by rank.
var ruptureTooltipForever = [][5]float64{
	{25, 37, 51, 68, 87},
	{35, 53, 74, 99, 127},
	{53, 79, 109, 143, 183},
	{76, 110, 149, 195, 246},
	{105, 151, 207, 270, 342},
	{159, 222, 295, 377, 469},
}

func TestForeverRuptureMatchesClientTooltips(t *testing.T) {
	if len(ruptureTickForever) != len(core.TrainerRanks("Rogue|Rupture")) {
		t.Fatal("Forever Rupture table length")
	}
	for rank, totals := range ruptureTooltipForever {
		base, perCP := ruptureTickForever[rank][0], ruptureTickForever[rank][1]
		for cp := int32(1); cp <= 5; cp++ {
			total := (base + perCP*float64(cp)) * float64(3+cp)
			if math.Abs(total-totals[cp-1]) > 0.5 {
				t.Errorf("Rupture rank %d, %d points: %.2f, client %v", rank+1, cp, total, totals[cp-1])
			}
		}
	}
}

func TestForeverEviscerateMatchesClientTooltips(t *testing.T) {
	// {1 point min, 5 points max} from the client tooltip, by rank.
	tooltip := [][2]float64{{6, 30}, {14, 66}, {25, 115}, {41, 185}, {60, 270}, {93, 421}, {144, 652}, {199, 899}, {224, 1012}}
	for rank, want := range tooltip {
		v := eviscerateValuesForever[rank]
		if lo, hi := v[0]+v[1], v[0]+5*v[1]+v[2]; lo != want[0] || hi != want[1] {
			t.Errorf("Eviscerate rank %d: %v-%v, client %v", rank+1, lo, hi, want)
		}
	}
}

func TestForeverPoisonAndArmorTables(t *testing.T) {
	ipTooltip := [][2]float64{{13, 17}, {20, 26}, {29, 37}, {45, 57}, {62, 80}, {76, 100}}
	for i, want := range ipTooltip {
		if v := instantPoisonDamageForever[i]; v[0] != want[0] || v[0]+v[1] != want[1] {
			t.Errorf("Instant Poison %d: %v, client %v", i+1, v, want)
		}
	}
	dpTooltip := []float64{24, 36, 56, 72, 92} // over 12 sec, 4 ticks
	for i, want := range dpTooltip {
		if deadlyPoisonTickDamageForever[i]*4 != want {
			t.Errorf("Deadly Poison %d: %v a tick, client %v over 12 sec", i+1, deadlyPoisonTickDamageForever[i], want)
		}
	}
	if len(mutilateBonusForever) != len(core.TrainerRanks("Rogue|Mutilate")) || mutilateBonusForever[3] != 38 {
		t.Error("Mutilate ranks", mutilateBonusForever)
	}
	if len(exposeArmorPerComboForever) != len(core.TrainerRanks("Rogue|Expose Armor")) || exposeArmorPerComboForever[4]*5 != 2250 {
		t.Error("Expose Armor", exposeArmorPerComboForever)
	}
	if foreverSetupChance[2] != .67 || foreverSetupChance[3] != 1 {
		t.Error("Setup", foreverSetupChance)
	}
}
