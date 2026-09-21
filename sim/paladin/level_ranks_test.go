package paladin_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

// hybridSimAtLevel is hybridSim for a character below the level cap.
func hybridSimAtLevel(t *testing.T, class string, talents map[string]int32, level int32) *core.Character {
	t.Helper()
	p := &proto.Player{Level: level, Equipment: &proto.EquipmentSpec{}, Consumes: &proto.Consumes{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: talents, Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true}}
	if class == "PALADIN" {
		p.Class, p.Race = proto.Class_ClassPaladin, proto.Race_RaceHuman
		p.Spec = &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.PaladinOptions{RighteousFury: true}}}
		p.Forever.Mechanics = []string{"paladin.baseline.holy-strike", "paladin.baseline.seal-of-fury"}
	} else {
		p.Class, p.Race = proto.Class_ClassShaman, proto.Race_RaceTroll
		p.Spec = &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{}}}
		p.Forever.Mechanics = []string{"shaman.baseline.fire-nova"}
	}
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Health: 100000}.ToFloatArray()}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	return sim.Raid.Parties[0].Players[0].GetCharacter()
}

// Every Forever paladin and shaman talent rank still constructs below the level cap,
// where many baseline spells are not learned yet.
func TestForeverHybridTalentsConstructAtLowLevel(t *testing.T) {
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Records []foreverdata.Record }
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, level := range []int32{1, 10, 20} {
		for _, r := range data.Records {
			if (r.Class != "PALADIN" && r.Class != "SHAMAN") || r.MaxRank == 0 {
				continue
			}
			hybridSimAtLevel(t, r.Class, hybridTalents(t, r.ID, r.MaxRank), level)
		}
	}
}

// Forever-only baseline spells use the rank the character has learned.
func TestForeverLevel20Ranks(t *testing.T) {
	pal := hybridSimAtLevel(t, "PALADIN", map[string]int32{}, 20)
	// Holy Light r3 yes / r4 no, Flash of Light r1, Divine Protection r2, Purify yes;
	// Divine Shield (34) and Cleanse (42) not yet.
	for id, want := range map[int32]bool{647: true, 1026: false, 19750: true, 5573: true, 1152: true, 642: false, 1020: false, 4987: false} {
		if got := pal.GetSpell(core.ActionID{SpellID: id}) != nil; got != want {
			t.Errorf("paladin level 20: spell %d registered=%v, want %v", id, got, want)
		}
	}
	sham := hybridSimAtLevel(t, "SHAMAN", map[string]int32{}, 20)
	for id, want := range map[int32]bool{913: true, 939: false, 8004: true, 8008: false, 1064: false} {
		if got := sham.GetSpell(core.ActionID{SpellID: id}) != nil; got != want {
			t.Errorf("shaman level 20: heal %d registered=%v, want %v", id, got, want)
		}
	}
	nova := sham.GetSpell(sham.ForeverAction("shaman.baseline.fire-nova"))
	if nova == nil || nova.DefaultCast.Cost != 95 {
		t.Errorf("shaman level 20: Fire Nova should be rank 1 (95 Mana), got %v", nova)
	}
}
