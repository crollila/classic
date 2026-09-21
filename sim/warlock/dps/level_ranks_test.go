package dps

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/warlock"
)

// A level-20 warlock knows the ranks its trainer taught by 20, its demons use their
// level-20 ranks and stats, and demons learned later cannot be out.

func levelWarlock(level int32, summon proto.WarlockOptions_Summon, forever bool) *proto.Player {
	p := &proto.Player{Class: proto.Class_ClassWarlock, Race: proto.Race_RaceUndead, Level: level,
		Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{Summon: summon, Armor: proto.WarlockOptions_DemonArmor}}}}
	if forever {
		p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: map[string]int32{}}
	}
	return p
}

func levelWarlockAgent(t *testing.T, p *proto.Player) *warlock.Warlock {
	t.Helper()
	request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: p.Level + 2, MobType: proto.MobType_MobTypeHumanoid}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, IsTest: true, Interactive: true}}
	s := core.NewSim(request, simsignals.Signals{})
	s.Reset()
	return s.Raid.Parties[0].Players[0].(warlock.WarlockAgent).GetWarlock()
}

func checkKnown(t *testing.T, u *core.Unit, known, unknown []int32) {
	t.Helper()
	for _, id := range known {
		if u.GetSpell(core.ActionID{SpellID: id}) == nil {
			t.Errorf("%s level %d: spell %d should be registered", u.Label, u.Level, id)
		}
	}
	for _, id := range unknown {
		if u.GetSpell(core.ActionID{SpellID: id}) != nil {
			t.Errorf("%s level %d: spell %d (learned at %d) should not be registered", u.Label, u.Level, id, core.SpellLearnedLevel(id))
		}
	}
}

func TestWarlockLevel20Ranks(t *testing.T) {
	w := levelWarlockAgent(t, levelWarlock(20, proto.WarlockOptions_Imp, false))
	checkKnown(t, &w.Unit,
		// Shadow Bolt 4, Immolate 3, Corruption 2, Curse of Agony 2, Life Tap 2, Searing Pain 1,
		// Drain Life 1, Drain Soul 1, Rain of Fire 1, Curse of Recklessness 1, summons.
		[]int32{1088, 1094, 6222, 1014, 1455, 5676, 689, 1120, 5740, 704, 688, 697, 712},
		// Next ranks; Curse of the Elements/Shadow/Doom and Summon Felhunter come later.
		[]int32{1106, 2941, 6223, 6217, 1456, 17919, 699, 8288, 7658, 1490, 17862, 603, 691})
	checkKnown(t, &w.Imp.Unit, []int32{7800}, []int32{7801})
	checkKnown(t, &w.Succubus.Unit, []int32{7814}, []int32{7815})
	if w.ActivePet != w.Imp {
		t.Error("the imp should be out")
	}
	if w.Imp.GetStat(stats.Intellect) <= 0 || w.Imp.GetStat(stats.Intellect) >= 94 {
		t.Errorf("level 20 imp Intellect should be below its level-25 94, got %.1f", w.Imp.GetStat(stats.Intellect))
	}
	if armor := w.GetAura("Demon Armor"); armor == nil || armor.ActionID.SpellID != 706 {
		t.Errorf("level 20 armor should be Demon Armor rank 1 (706), got %v", armor)
	}

	felhunter := levelWarlockAgent(t, levelWarlock(20, proto.WarlockOptions_Felhunter, false))
	if felhunter.ActivePet != nil {
		t.Error("the Felhunter is learned at 30 and cannot be out at 20")
	}
}

func TestWarlockLevel60RanksUnchanged(t *testing.T) {
	w := levelWarlockAgent(t, levelWarlock(60, proto.WarlockOptions_Imp, false))
	checkKnown(t, &w.Unit, []int32{11661, 11668, 25311, 11713, 11689, 17923, 11700, 11675, 11678, 11717, 11722, 17937, 603, 691}, nil)
	checkKnown(t, &w.Imp.Unit, []int32{11763}, nil)
	checkKnown(t, &w.Succubus.Unit, []int32{11780}, nil)
	if armor := w.GetAura("Demon Armor"); armor == nil || armor.ActionID.SpellID != 11735 {
		t.Errorf("level 60 armor should be Demon Armor rank 5 (11735), got %v", armor)
	}
}

func TestWarlockForeverLevel20(t *testing.T) {
	w := levelWarlockAgent(t, levelWarlock(20, proto.WarlockOptions_Imp, true))
	// Curse of Weakness rank 2 (12) instead of rank 6.
	checkKnown(t, &w.Unit, []int32{1108}, []int32{11708})
}

func TestWarlockLevelsRun(t *testing.T) {
	for _, level := range []int32{1, 10, 20, 25, 30, 40, 45, 50, 59, 60} {
		for _, forever := range []bool{false, true} {
			for _, summon := range []proto.WarlockOptions_Summon{proto.WarlockOptions_Imp, proto.WarlockOptions_Succubus, proto.WarlockOptions_Voidwalker, proto.WarlockOptions_Felhunter} {
				p := levelWarlock(level, summon, forever)
				p.Rotation = core.GetAplRotation("../../../ui/warlock/apls", "rotation").Rotation
				result := core.RunRaidSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
					Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid}}},
					SimOptions: &proto.SimOptions{Iterations: 5, RandomSeed: 1}})
				if result.Error != nil {
					t.Fatalf("level=%d forever=%v summon=%v: %s", level, forever, summon, result.Error.Message)
				}
				if dps := result.RaidMetrics.Parties[0].Players[0].Dps.Avg; dps <= 0 {
					t.Errorf("level=%d forever=%v summon=%v: no damage", level, forever, summon)
				}
			}
		}
	}
}
