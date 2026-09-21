package rogue

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
)

// Every per-rank value table must have one entry per rank of its trainer spell.
func TestRogueRankTablesMatchTrainerRanks(t *testing.T) {
	tables := map[string]int{
		"Sinister Strike": len(sinisterStrikeBonus),
		"Backstab":        len(backstabBonus),
		"Ambush":          len(ambushBonus),
		"Eviscerate":      len(eviscerateValues),
		"Slice and Dice":  len(sliceAndDiceHaste),
		"Garrote":         len(garroteTick),
		"Rupture":         len(ruptureTick),
		"Expose Armor":    len(exposeArmorPerCombo),
		"Instant Poison":  len(instantPoisonIDs),
		"Deadly Poison":   len(deadlyPoisonIDs),
		"Sprint":          3,
		"Gouge":           5,
		"Kick":            4,
		"Sap":             3,
		"Kidney Shot":     2,
	}
	for family, n := range tables {
		if got := len(core.TrainerRanks("Rogue|" + family)); got != n {
			t.Errorf("%s: table has %d ranks, trainer data %d", family, n, got)
		}
	}
	if len(instantPoisonDamage) != len(instantPoisonIDs) || len(deadlyPoisonTickDamage) != len(deadlyPoisonIDs) {
		t.Error("poison value tables differ in length from their id tables")
	}
}
