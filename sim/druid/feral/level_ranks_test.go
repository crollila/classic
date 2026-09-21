package feral

import (
	"sort"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// The rotation names the level-60 ranks; below 60 the APL falls back to the rank the
// druid learned, and abilities not learned yet are skipped. Tiger's Fury has a single
// rank in the Forever client, so there is no rank family to fall back through and
// the rotation names rank 1 separately.
const levelRanksAPL = `{"type":"TypeAPL","priorityList":[
 {"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":9846}}}}},"castSpell":{"spellId":{"spellId":9846}}}},
 {"action":{"condition":{"not":{"val":{"auraIsActive":{"auraId":{"spellId":5217}}}}},"castSpell":{"spellId":{"spellId":5217}}}},
 {"action":{"condition":{"cmp":{"op":"OpEq","lhs":{"currentComboPoints":{}},"rhs":{"const":{"val":"5"}}}},"castSpell":{"spellId":{"spellId":9896}}}},
 {"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":9904}}}}},"castSpell":{"spellId":{"spellId":9904}}}},
 {"action":{"castSpell":{"spellId":{"spellId":9830}}}},
 {"action":{"castSpell":{"spellId":{"spellId":9850}}}}
]}`

func feralCastsAtLevel(t *testing.T, level int32) map[int32]bool {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1484}
	p := core.WithSpec(&proto.Player{
		Name: "Feral", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, Level: level,
		Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
		Rotation: core.APLRotationFromJsonString(levelRanksAPL),
	}, &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{}}})
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

func TestFeralRanksByLevel(t *testing.T) {
	for _, c := range []struct {
		level int32
		casts []int32
	}{
		{20, []int32{1079, 1082}},             // Rip 1, Claw 1; no Shred/Rake/Tiger's Fury yet
		{30, []int32{5217, 9492, 1822, 6800}}, // Tiger's Fury 1, Rip 2, Rake 1, Shred 2
		{60, []int32{9846, 9896, 9904, 9830}}, // top ranks
	} {
		ids := feralCastsAtLevel(t, c.level)
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
