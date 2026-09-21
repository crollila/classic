package mage

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
)

// A level-20 mage knows the ranks its trainer taught by 20 and nothing learned later.

func levelMage(level int32, forever bool, mechanics ...string) *proto.Player {
	p := &proto.Player{Class: proto.Class_ClassMage, Race: proto.Race_RaceGnome, Level: level,
		Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{Armor: proto.Mage_Options_IceArmor}}}}
	if forever {
		p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: map[string]int32{}, Mechanics: mechanics}
	}
	return p
}

func levelMageCharacter(t *testing.T, p *proto.Player) *core.Character {
	t.Helper()
	request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: p.Level + 2, MobType: proto.MobType_MobTypeHumanoid}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true, Interactive: true}}
	s := core.NewSim(request, simsignals.Signals{})
	s.Reset()
	return s.Raid.Parties[0].Players[0].GetCharacter()
}

func checkKnown(t *testing.T, c *core.Character, known, unknown []int32) {
	t.Helper()
	for _, id := range known {
		if c.GetSpell(core.ActionID{SpellID: id}) == nil {
			t.Errorf("level %d: spell %d should be registered", c.Level, id)
		}
	}
	for _, id := range unknown {
		if c.GetSpell(core.ActionID{SpellID: id}) != nil {
			t.Errorf("level %d: spell %d (learned at %d) should not be registered", c.Level, id, core.SpellLearnedLevel(id))
		}
	}
}

func TestMageLevel20Ranks(t *testing.T) {
	c := levelMageCharacter(t, levelMage(20, false))
	checkKnown(t, c,
		// Fireball 4, Frostbolt 4, Fire Blast 2, Arcane Missiles 2, Arcane Explosion 1,
		// Flamestrike 1, Blizzard 1, Evocation.
		[]int32{3140, 7322, 2137, 5144, 1449, 2120, 10, 12051},
		// Next ranks, and Scorch (22) and Counterspell (24).
		[]int32{8400, 8406, 2138, 5145, 8437, 2121, 6141, 2948, 2139})
	if armor := c.GetAura("Ice Armor"); armor == nil || armor.ActionID.SpellID != 7301 {
		t.Errorf("level 20 armor should be Frost Armor rank 3 (7301), got %v", armor)
	}
}

func TestMageLevel60RanksUnchanged(t *testing.T) {
	c := levelMageCharacter(t, levelMage(60, false))
	checkKnown(t, c, []int32{10151, 25304, 10199, 10212, 10202, 10216, 10187, 10207, 12051, 2139}, nil)
	if armor := c.GetAura("Ice Armor"); armor == nil || armor.ActionID.SpellID != 10220 {
		t.Errorf("level 60 armor should be Ice Armor rank 4 (10220), got %v", armor)
	}
}

func TestMageForeverLevel20Ranks(t *testing.T) {
	c := levelMageCharacter(t, levelMage(20, true, "mage.baseline.frostfire-bolt"))
	checkKnown(t, c,
		// Frost Nova 1, Fire Ward 1, Mana Shield 1.
		[]int32{122, 543, 1463},
		// Top ranks, Cone of Cold (26), Frost Ward (22) and Frostfire Bolt (40).
		[]int32{10230, 10161, 120, 10223, 10225, 28609, 6143, 10193, 401502})
	if c.GetSpell(c.ForeverAction("mage.baseline.frostfire-bolt")) != nil {
		t.Error("Frostfire Bolt is learned at 40 and must not be castable at 20")
	}

	c = levelMageCharacter(t, levelMage(60, true, "mage.baseline.frostfire-bolt"))
	checkKnown(t, c, []int32{10230, 10161, 10225, 28609, 10193}, []int32{122, 543, 1463, 10223})
	if c.GetSpell(c.ForeverAction("mage.baseline.frostfire-bolt")) == nil {
		t.Error("level 60 Frostfire Bolt keeps its Forever action")
	}
}

func TestMageLevelsRun(t *testing.T) {
	// From 4: the shipped rotation has nothing to cast before Frostbolt / Shadow Word: Pain.
	for _, level := range []int32{4, 10, 20, 25, 30, 40, 45, 50, 59, 60} {
		for _, forever := range []bool{false, true} {
			p := levelMage(level, forever)
			p.Rotation = core.GetAplRotation("../../ui/mage/apls", "p1").Rotation
			result := core.RunRaidSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
				Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid}}},
				SimOptions: &proto.SimOptions{Iterations: 5, RandomSeed: 1}})
			if result.Error != nil {
				t.Fatalf("level=%d forever=%v: %s", level, forever, result.Error.Message)
			}
			if dps := result.RaidMetrics.Parties[0].Players[0].Dps.Avg; dps <= 0 {
				t.Errorf("level=%d forever=%v: no damage", level, forever)
			}
		}
	}
}
