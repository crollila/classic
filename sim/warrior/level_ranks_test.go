package warrior_test

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"google.golang.org/protobuf/encoding/protojson"
)

func levelWarrior(t *testing.T, level int32, edits ...func(*proto.Player)) *core.Character {
	t.Helper()
	p := &proto.Player{}
	if err := protojson.Unmarshal([]byte(`{"class":"ClassWarrior","race":"RaceOrc","warrior":{"options":{}}}`), p); err != nil {
		t.Fatal(err)
	}
	p.Level = level
	p.Equipment = &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range p.Equipment.Items {
		p.Equipment.Items[i] = &proto.ItemSpec{}
	}
	p.Equipment.Items[proto.ItemSlot_ItemSlotMainHand].Id = 1318 // level-20 dungeon weapon
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

func warriorSpellIDs(c *core.Character) map[int32]bool {
	ids := map[int32]bool{}
	for _, s := range c.Spellbook {
		ids[s.SpellID] = true
	}
	return ids
}

func TestWarriorRanksByLevel(t *testing.T) {
	forever := func(p *proto.Player) {
		p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: map[string]int32{}, ExperimentalEstimatedRanks: true}
	}
	cases := []struct {
		name   string
		level  int32
		edits  []func(*proto.Player)
		want   []int32
		absent []int32
	}{
		// Heroic Strike r3, Rend r3, Battle Shout r2, Sunder Armor r1, Thunder Clap r2, Hamstring r1,
		// Overpower r1, Revenge r1, Demoralizing Shout r1, Cleave r1, Slam r1 (Forever), Bloodrage,
		// Battle and Defensive Stance. No Execute, Berserker Stance, Whirlwind, Pummel, Berserker Rage.
		{"20", 20, nil, []int32{285, 6547, 5242, 7386, 8198, 1715, 7384, 6572, 1160, 845, 1240193, 2687, 2457, 71},
			[]int32{5308, 2458, 1680, 6552, 18499, 1719}},
		{"10", 10, nil, []int32{284, 6546, 6673, 7386, 6343, 1715, 2687, 71}, []int32{7384, 6572, 1160, 845, 1240193}},
		{"1", 1, nil, []int32{78, 6673, 2457}, []int32{772, 71, 2687, 7386}},
		{"40", 40, nil, []int32{11565, 11572, 11549, 8380, 8205, 7372, 7887, 7379, 11554, 11608, 8820, 20660, 2458, 1680, 6552, 18499},
			[]int32{1719}},
		// Level 60 keeps the ranks the sim has always used.
		{"60", 60, nil, []int32{25286, 11574, 25289, 11597, 11581, 7373, 11585, 25288, 11556, 20569, 11605, 20662, 2458, 1680, 6554, 18499, 1719}, nil},
		// Forever Charge r1 and Disarm at 20; Intercept not before 30.
		{"20-forever", 20, []func(*proto.Player){forever}, []int32{100, 676}, []int32{20252, 6178, 11578}},
		{"60-forever", 60, []func(*proto.Player){forever}, []int32{11578, 20617, 676}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ids := warriorSpellIDs(levelWarrior(t, tc.level, tc.edits...))
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

// A warrior below Berserker Stance's level whose talents point to Fury starts in Battle Stance.
func TestWarriorUnlearnedStanceFallsBackToBattle(t *testing.T) {
	c := levelWarrior(t, 20, func(p *proto.Player) {
		p.GetWarrior().Options.Stance = proto.WarriorStance_WarriorStanceBerserker
	})
	if !c.GetAura("Battle Stance").IsActive() || c.GetAura("Berserker Stance").IsActive() {
		t.Fatal("level 20 warrior did not start in Battle Stance")
	}
}
