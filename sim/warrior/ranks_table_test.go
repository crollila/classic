package warrior

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
)

// Every per-rank value table must have one entry per rank of its trainer spell.
func TestWarriorRankTablesMatchTrainerRanks(t *testing.T) {
	tables := map[string][]int{
		"Heroic Strike":      {len(heroicStrikeBonus), len(heroicStrikeThreat)},
		"Cleave":             {len(cleaveBonus), len(cleaveThreat)},
		"Rend":               {len(rendTick), len(rendTicks)},
		"Execute":            {len(executeValues)},
		"Overpower":          {len(overpowerBonus)},
		"Slam":               {len(slamBonus)},
		"Hamstring":          {len(hamstringDamage)},
		"Mortal Strike":      {len(mortalStrikeBonus)},
		"Shield Slam":        {len(shieldSlamDamage), len(shieldSlamThreat), len(shieldSlamDamageForever)},
		"Thunder Clap":       {len(thunderClapDamage), len(thunderClapDuration)},
		"Sunder Armor":       {len(sunderArmorPerStack)},
		"Battle Shout":       {len(battleShoutAP), core.BattleShoutRanks},
		"Demoralizing Shout": {len(demoralizingShoutAP), core.DemoralizingShoutRanks},
		"Revenge":            {RevengeRanks, len(revengeDamageForever)},
		"Intercept":          {len(interceptDamage)},
		"Pummel":             {len(pummelDamage)},
		"Charge":             {len(chargeRage)},
		"Bloodthirst":        {4, len(bloodthirstBonusForever)},
	}
	for family, lengths := range tables {
		ranks := core.TrainerRanks("Warrior|" + family)
		for _, n := range lengths {
			if n != len(ranks) {
				t.Errorf("%s: table has %d ranks, trainer data %d", family, n, len(ranks))
			}
		}
	}
	// The core rank tables the shouts and Revenge index must name the same spells.
	for i, r := range core.TrainerRanks("Warrior|Battle Shout") {
		if core.BattleShoutSpellId[i+1] != r[0] {
			t.Errorf("Battle Shout rank %d: core id %d, trainer id %d", i+1, core.BattleShoutSpellId[i+1], r[0])
		}
	}
	for i, r := range core.TrainerRanks("Warrior|Demoralizing Shout") {
		if core.DemoralizingShoutSpellId[i+1] != r[0] {
			t.Errorf("Demoralizing Shout rank %d: core id %d, trainer id %d", i+1, core.DemoralizingShoutSpellId[i+1], r[0])
		}
	}
	for i, r := range core.TrainerRanks("Warrior|Revenge") {
		if RevengeSpellId[i+1] != r[0] || int32(RevengeLevel[i+1]) != r[1] {
			t.Errorf("Revenge rank %d: id/level %d/%d, trainer %d/%d", i+1, RevengeSpellId[i+1], RevengeLevel[i+1], r[0], r[1])
		}
	}
}
