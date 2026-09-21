package shadow

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
)

// A level-20 shadow priest knows the ranks its trainer taught by 20 and nothing learned later.

func levelPriest(level int32, forever bool, mechanics ...string) *proto.Player {
	p := &proto.Player{Class: proto.Class_ClassPriest, Race: proto.Race_RaceUndead, Level: level,
		Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}}
	if forever {
		p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: map[string]int32{}, Mechanics: mechanics}
	}
	return p
}

func levelPriestCharacter(t *testing.T, p *proto.Player) *core.Character {
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

func TestShadowPriestLevel20Ranks(t *testing.T) {
	c := levelPriestCharacter(t, levelPriest(20, false))
	checkKnown(t, c,
		// Mind Blast 2, Shadow Word: Pain 3, Smite 3, Holy Fire 1, Devouring Plague 1 (Undead).
		[]int32{8102, 970, 598, 14914, 2944},
		// Next ranks; Mind Flay is a talent this build does not take.
		[]int32{8103, 992, 984, 15262, 19276, 15407})
}

func TestShadowPriestLevel60RanksUnchanged(t *testing.T) {
	c := levelPriestCharacter(t, levelPriest(60, false))
	checkKnown(t, c, []int32{10947, 10894, 10934, 15261, 19279}, nil)
}

func TestShadowPriestForeverLevel20(t *testing.T) {
	c := levelPriestCharacter(t, levelPriest(20, true, "priest.baseline.shadow-word-death"))
	if c.GetSpell(c.ForeverAction("priest.baseline.shadow-word-death")) != nil {
		t.Error("Shadow Word: Death is learned at 32 and must not be castable at 20")
	}
	c = levelPriestCharacter(t, levelPriest(60, true, "priest.baseline.shadow-word-death"))
	if c.GetSpell(c.ForeverAction("priest.baseline.shadow-word-death")) == nil {
		t.Error("level 60 keeps Shadow Word: Death")
	}
}

func TestShadowPriestLevelsRun(t *testing.T) {
	// From 4: the shipped rotation has nothing to cast before Frostbolt / Shadow Word: Pain.
	for _, level := range []int32{4, 10, 20, 25, 30, 40, 45, 50, 59, 60} {
		for _, forever := range []bool{false, true} {
			p := levelPriest(level, forever)
			p.Rotation = core.GetAplRotation("../../../ui/shadow_priest/apls", "p1").Rotation
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
