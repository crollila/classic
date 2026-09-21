package rogue_test

import (
	"fmt"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	_ "github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
)

// levelRogue builds a rogue of the given level with Instant Poison on both weapons.
func levelRogue(t *testing.T, level int32, edits ...func(*proto.Player)) *core.Character {
	t.Helper()
	p := &proto.Player{}
	if err := protojson.Unmarshal([]byte(`{"class":"ClassRogue","race":"RaceHuman","rogue":{"options":{}}}`), p); err != nil {
		t.Fatal(err)
	}
	p.Level = level
	p.Equipment = &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range p.Equipment.Items {
		p.Equipment.Items[i] = &proto.ItemSpec{}
	}
	p.Equipment.Items[proto.ItemSlot_ItemSlotMainHand].Id = 5191 // level-20 dungeon weapons
	p.Equipment.Items[proto.ItemSlot_ItemSlotOffHand].Id = 5192
	p.Consumes = &proto.Consumes{MainHandImbue: proto.WeaponImbue_InstantPoison, OffHandImbue: proto.WeaponImbue_DeadlyPoison}
	p.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	for _, edit := range edits {
		edit(p)
	}
	req := &proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 3, IsTest: true},
	}
	sim := core.NewSim(req, simsignals.Signals{})
	sim.Reset()
	return sim.Raid.Parties[0].Players[0].GetCharacter()
}

func spellIDs(c *core.Character) map[int32]bool {
	ids := map[int32]bool{}
	for _, s := range c.Spellbook {
		ids[s.SpellID] = true
	}
	for _, a := range c.GetAuras() {
		ids[a.ActionID.SpellID] = true
	}
	return ids
}

func TestRogueRanksByLevel(t *testing.T) {
	cases := []struct {
		level   int32
		want    []int32 // registered
		absent  []int32 // must not be registered
		poisons []string
	}{
		// Sinister Strike r3, Eviscerate r3, Slice and Dice r1, Backstab r3, Rupture r1,
		// Garrote r1, Ambush r1, Expose Armor r1, Feint r1, Stealth r2, Instant Poison I.
		{20, []int32{1758, 6761, 5171, 2590, 1943, 703, 8676, 8647, 1966, 1785, 8679},
			[]int32{1759, 6762, 6774, 2591, 8639, 1856}, []string{"Instant Poison"}},
		{10, []int32{1757, 6760, 5171, 53}, []int32{2589, 1943, 703, 8676, 8647, 1966}, nil},
		{1, []int32{1752, 2098, 1784}, []int32{53, 5171}, nil},
		{40, []int32{8621, 8624, 5171, 8721, 8640, 8633, 8725, 8650, 8637, 1786, 8688, 2824, 1856}, []int32{6774},
			[]string{"Instant Poison", "Deadly Poison"}},
		// Level 60 keeps the ranks the sim has always used.
		{60, []int32{11294, 31016, 6774, 25300, 11275, 11290, 11269, 11198, 1787, 11340, 25347}, nil,
			[]string{"Instant Poison", "Deadly Poison"}},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.level), func(t *testing.T) {
			c := levelRogue(t, tc.level)
			ids := spellIDs(c)
			for _, id := range tc.want {
				if !ids[id] {
					t.Errorf("level %d: spell %d not registered", tc.level, id)
				}
			}
			for _, id := range tc.absent {
				if ids[id] {
					t.Errorf("level %d: spell %d registered", tc.level, id)
				}
			}
			// The weapon imbues only proc once the rogue has learned that poison.
			for _, label := range []string{"Instant Poison", "Deadly Poison"} {
				want := false
				for _, l := range tc.poisons {
					want = want || l == label
				}
				if got := c.GetAura(label) != nil; got != want {
					t.Errorf("level %d: %s imbue active = %v, want %v", tc.level, label, got, want)
				}
			}
			for id := range ids {
				if learned := core.SpellLearnedLevel(id); learned > tc.level {
					t.Errorf("level %d: registered spell %d is learned at %d", tc.level, id, learned)
				}
			}
		})
	}
}

// The Forever-only rogue utility abilities (Sprint, Gouge, Kick, Sap, ...) follow the same ranks.
func TestForeverRogueUtilityRanksByLevel(t *testing.T) {
	forever := func(p *proto.Player) {
		p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: map[string]int32{}, ExperimentalEstimatedRanks: true}
	}
	cases := []struct {
		level  int32
		want   []int32
		absent []int32
	}{
		// Sprint r1, Gouge r2, Kick r1, Sap r1, Stealth r2; no Kidney Shot, Cheap Shot or Blind yet.
		{20, []int32{2983, 1777, 1766, 6770, 1785}, []int32{408, 8643, 1833, 2094, 11305, 11286, 1769}},
		{60, []int32{11305, 11286, 1769, 11297, 8643, 1833, 2094, 1787}, nil},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.level), func(t *testing.T) {
			ids := spellIDs(levelRogue(t, tc.level, forever))
			for _, id := range tc.want {
				if !ids[id] {
					t.Errorf("level %d: spell %d not registered", tc.level, id)
				}
			}
			for _, id := range tc.absent {
				if ids[id] {
					t.Errorf("level %d: spell %d registered", tc.level, id)
				}
			}
			for id := range ids {
				if learned := core.SpellLearnedLevel(id); learned > tc.level {
					t.Errorf("level %d: registered spell %d is learned at %d", tc.level, id, learned)
				}
			}
		})
	}
}
