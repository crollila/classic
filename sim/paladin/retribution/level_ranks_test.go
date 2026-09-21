package retribution

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

// A retribution paladin of the given level with a two-hand mace (Black Malice).
func levelPaladin(t *testing.T, level int32) *core.Character {
	t.Helper()
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 3194}
	player := &proto.Player{
		Name: "Paladin", Class: proto.Class_ClassPaladin, Race: proto.Race_RaceHuman, Level: level,
		Equipment: equipment,
		Consumes:  &proto.Consumes{},
		Buffs:     &proto.IndividualBuffs{},
		Rotation:  &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec:      &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{}}},
	}
	sim := core.NewSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeUndead, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true},
	}, simsignals.Signals{})
	sim.Reset()
	return sim.Raid.Parties[0].Players[0].GetCharacter()
}

func checkRanks(t *testing.T, c *core.Character, level int32, known, unknown map[string]int32) {
	t.Helper()
	for name, id := range known {
		if c.GetSpell(core.ActionID{SpellID: id}) == nil {
			t.Errorf("level %d: %s (%d) should be registered", level, name, id)
		}
	}
	for name, id := range unknown {
		if c.GetSpell(core.ActionID{SpellID: id}) != nil {
			t.Errorf("level %d: %s (%d) is not learned yet but is registered", level, name, id)
		}
	}
}

func TestPaladinLevel20Ranks(t *testing.T) {
	c := levelPaladin(t, 20)
	checkRanks(t, c, 20, map[string]int32{
		"Seal of Righteousness r3":      20288,
		"Judgement of Righteousness r3": 20281,
		"Seal of Command r1":            20375,
		"Judgement of Command r1":       20467,
		"Seal of the Crusader r2":       20162,
		"Judgement of the Crusader r2":  20188,
		"Exorcism r1":                   879,
		"Judgement":                     20271,
	}, map[string]int32{
		"Seal of Righteousness r4": 20289,
		"Seal of Command r2":       20915,
		"Seal of the Crusader r3":  20305,
		"Exorcism r2":              5614,
		"Hammer of Wrath r1":       24275,
		"Holy Wrath r1":            2812,
	})
}

func TestPaladinLevel60Ranks(t *testing.T) {
	c := levelPaladin(t, 60)
	checkRanks(t, c, 60, map[string]int32{
		"Seal of Righteousness r8": 20293,
		"Seal of Command r5":       20920,
		"Seal of the Crusader r6":  20308,
		"Exorcism r6":              10314,
		"Hammer of Wrath r3":       24239,
		"Holy Wrath r2":            10318,
	}, nil)
}
