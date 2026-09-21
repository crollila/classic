package balance

import (
	"sort"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

const levelRanksAPL = `{"type":"TypeAPL","priorityList":[
 {"action":{"autocastOtherCooldowns":{}}},
 {"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":9835}}}}},"castSpell":{"spellId":{"spellId":9835}}}},
 {"action":{"condition":{"not":{"val":{"auraIsActive":{"sourceUnit":{"type":"CurrentTarget"},"auraId":{"spellId":9907}}}}},"castSpell":{"spellId":{"spellId":9907}}}},
 {"action":{"condition":{"cmp":{"op":"OpLt","lhs":{"currentTime":{}},"rhs":{"const":{"val":"8s"}}}},"castSpell":{"spellId":{"spellId":9912}}}},
 {"action":{"castSpell":{"spellId":{"spellId":25298}}}}
]}`

func balanceCastsAtLevel(t *testing.T, level int32) map[int32]bool {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1484}
	p := core.WithSpec(&proto.Player{
		Name: "Balance", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, Level: level,
		Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
		Rotation: core.APLRotationFromJsonString(levelRanksAPL),
	}, &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}})
	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 7},
	})
	if result.Error != nil {
		t.Fatalf("level %d: %s", level, result.Error.Message)
	}
	ids := map[int32]bool{}
	for _, action := range result.RaidMetrics.Parties[0].Players[0].Actions {
		for _, target := range action.Targets {
			if target.Casts > 0 && action.Id.GetSpellId() != 0 {
				ids[action.Id.GetSpellId()] = true
			}
		}
	}
	return ids
}

func TestBalanceRanksByLevel(t *testing.T) {
	for _, c := range []struct {
		level int32
		casts []int32
	}{
		{20, []int32{8925, 770, 5178, 2912}},   // Moonfire 3, Faerie Fire 1, Wrath 3, Starfire 1
		{60, []int32{9835, 9907, 9912, 25298}}, // top ranks
	} {
		ids := balanceCastsAtLevel(t, c.level)
		for _, id := range c.casts {
			if !ids[id] {
				t.Errorf("level %d: expected a cast of %d, got %v", c.level, id, sortedIDs(ids))
			}
		}
		for id := range ids {
			if learned := core.SpellLearnedLevel(id); learned > c.level {
				t.Errorf("level %d: cast %d, learned at %d", c.level, id, learned)
			}
		}
		if c.level < 40 && ids[29166] {
			t.Errorf("level %d: cast Innervate, learned at 40", c.level)
		}
	}
}

func sortedIDs(ids map[int32]bool) []int32 {
	out := make([]int32, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
