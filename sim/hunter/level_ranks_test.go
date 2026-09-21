package hunter

import (
	"sort"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// The rotation names the level-60 ranks; below 60 the APL falls back to the rank the
// hunter actually learned. Multi-Shot has a single rank in the Forever client.
const levelRanksAPL = `{"type":"TypeAPL",
 "prepullActions":[{"action":{"castSpell":{"spellId":{"spellId":25296}}},"doAtValue":{"const":{"val":"-5s"}}}],
 "priorityList":[
  {"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":25295}}}}},"castSpell":{"spellId":{"spellId":25295}}}},
  {"action":{"castSpell":{"spellId":{"spellId":14287}}}},
  {"action":{"castSpell":{"spellId":{"spellId":2643}}}}
 ]}`

func castIDs(unit *proto.UnitMetrics) map[int32]bool {
	ids := map[int32]bool{}
	for _, action := range unit.Actions {
		for _, target := range action.Targets {
			if target.Casts > 0 && action.Id.GetSpellId() != 0 {
				ids[action.Id.GetSpellId()] = true
			}
		}
	}
	return ids
}

func runHunterAtLevel(t *testing.T, level int32, petType proto.Hunter_Options_PetType) (player, pet map[int32]bool) {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1318}
	equipment.Items[proto.ItemSlot_ItemSlotRanged] = &proto.ItemSpec{Id: 6469}
	p := core.WithSpec(&proto.Player{
		Name: "Hunter", Class: proto.Class_ClassHunter, Race: proto.Race_RaceNightElf, Level: level, DistanceFromTarget: 30,
		Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
		Rotation: core.APLRotationFromJsonString(levelRanksAPL),
	}, &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
		Ammo: proto.Hunter_Options_JaggedArrow, PetType: petType, PetUptime: 1}}})
	result := core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 7},
	})
	if result.Error != nil {
		t.Fatalf("level %d: %s", level, result.Error.Message)
	}
	unit := result.RaidMetrics.Parties[0].Players[0]
	pet = map[int32]bool{}
	for _, p := range unit.Pets {
		for id := range castIDs(p) {
			pet[id] = true
		}
	}
	return castIDs(unit), pet
}

func TestHunterRanksByLevel(t *testing.T) {
	cases := []struct {
		level       int32
		petType     proto.Hunter_Options_PetType
		player, pet []int32
	}{
		// Aspect of the Hawk 2, Serpent Sting 3, Arcane Shot 3, Multi-Shot; Claw 3.
		{20, proto.Hunter_Options_Cat, []int32{14318, 13550, 14282, 2643}, []int32{16829}},
		// Bite 3, Lightning Breath 2.
		{20, proto.Hunter_Options_WindSerpent, nil, []int32{17256, 25008}},
		// Aspect of the Hawk 1, Serpent Sting 2, Arcane Shot 2; Claw 2.
		{12, proto.Hunter_Options_Cat, []int32{13165, 13549, 14281}, []int32{16828}},
		// Bite 2, Lightning Breath 2.
		{12, proto.Hunter_Options_WindSerpent, nil, []int32{17255, 25008}},
		// Level 60 keeps the top ranks.
		{60, proto.Hunter_Options_Cat, []int32{25296, 25295, 14287}, []int32{3009}},
	}
	for _, c := range cases {
		player, pet := runHunterAtLevel(t, c.level, c.petType)
		for _, id := range c.player {
			if !player[id] {
				t.Errorf("level %d: expected hunter cast of %d, got %v", c.level, id, sorted(player))
			}
		}
		for _, id := range c.pet {
			if !pet[id] {
				t.Errorf("level %d: expected pet cast of %d, got %v", c.level, id, sorted(pet))
			}
		}
		for _, ids := range []map[int32]bool{player, pet} {
			for id := range ids {
				if learned := core.SpellLearnedLevel(id); learned > c.level {
					t.Errorf("level %d: cast %d, learned at %d", c.level, id, learned)
				}
			}
		}
	}
}

func TestHunterNoPetBelowTen(t *testing.T) {
	_, pet := runHunterAtLevel(t, 8, proto.Hunter_Options_Cat)
	if len(pet) != 0 {
		t.Errorf("level 8 hunter has a pet casting %v", sorted(pet))
	}
}

func TestHunterMeleeRanksByLevel(t *testing.T) {
	for _, c := range []struct {
		level                      int32
		raptor, mongoose, wingClip int
	}{{1, 1, 0, 0}, {20, 3, 1, 1}, {60, 8, 4, 3}} {
		if got := rankForLevel(RaptorStrikeLevel[:], c.level); got != c.raptor {
			t.Errorf("level %d Raptor Strike rank %d, want %d", c.level, got, c.raptor)
		}
		if got := rankForLevel(mongooseBiteLevel[:], c.level); got != c.mongoose {
			t.Errorf("level %d Mongoose Bite rank %d, want %d", c.level, got, c.mongoose)
		}
		if got := rankForLevel(wingClipLevel[:], c.level); got != c.wingClip {
			t.Errorf("level %d Wing Clip rank %d, want %d", c.level, got, c.wingClip)
		}
	}
	if id := RaptorStrikeSpellId[rankForLevel(RaptorStrikeLevel[:], 20)]; id != 14261 {
		t.Errorf("level 20 Raptor Strike id %d, want 14261", id)
	}
}

func sorted(ids map[int32]bool) []int32 {
	out := make([]int32, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
