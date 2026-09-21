package feral

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// A Forever cat (Tiger's Fury rework, King of the Jungle, Furor, baseline Omen of
// Clarity) runs the shipped leveling rotation at 20, 40 and 60 without errors, with
// damage, and without casting a rank above its level.
func TestForeverFeralLevelingSmoke(t *testing.T) {
	talents := map[string]int32{
		"druid.talent.ferocity": 5, "druid.talent.heart-of-the-wild": 5, "druid.talent.feral-instinct": 3,
		"druid.talent.savage-fury": 2, "druid.talent.sharpened-claws": 2, "druid.talent.predatory-strikes": 3,
		"druid.talent.king-of-the-jungle": 3, "druid.talent.furor": 5,
	}
	for _, level := range []int32{20, 40, 60} {
		equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
		for i := range equipment.Items {
			equipment.Items[i] = &proto.ItemSpec{}
		}
		equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1484}
		p := core.WithSpec(&proto.Player{
			Name: "Feral", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, Level: level,
			Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
			Rotation: core.GetAplRotation("../../../ui/feral_druid/apls", "leveling").Rotation,
			Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: talents,
				Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true},
		}, &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{}}})
		result := core.RunRaidSim(&proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
			Encounter:  &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
			SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 3},
		})
		if result.Error != nil {
			t.Fatalf("level %d: %s", level, result.Error.Message)
		}
		unit := result.RaidMetrics.Parties[0].Players[0]
		if unit.Dps.Avg <= 0 {
			t.Errorf("level %d: no damage", level)
		}
		casts := map[int32]int32{}
		for _, action := range unit.Actions {
			for _, target := range action.Targets {
				casts[action.Id.GetSpellId()] += target.Casts
			}
		}
		for id, n := range casts {
			if learned := core.SpellLearnedLevel(id); n > 0 && learned > level {
				t.Errorf("level %d: cast %d learned at %d", level, id, learned)
			}
		}
		// The Forever Tiger's Fury has a 30 sec cooldown: at most 4 casts in 90 sec.
		if casts[5217] > 4 {
			t.Errorf("level %d: Tiger's Fury cast %d times in 90 sec", level, casts[5217])
		}
		t.Logf("level %d: %.1f dps, casts %v", level, unit.Dps.Avg, casts)
	}
}
