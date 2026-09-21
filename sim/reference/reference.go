// Package reference is the fixed set of simulations every Forever build is measured with: each
// DPS spec at level 20 (levelling rotation, a level-20 dungeon weapon, no talents) and at level
// 60 (the spec's Classic gear preset, the most popular Forever talent build, its default
// rotation). The build pipeline runs them against the previous and the new client data and
// reports the change, so a Blizzard change is seen in DPS before anything is published.
package reference

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

type Scenario struct {
	Spec        string // app spec key
	Class       proto.Class
	Race        proto.Race
	Level       int32
	APL         string   // ui/<spec>/apls/<name>
	Gear        string   // ui/<spec>/gear_sets/<name> (level 60), empty = weapons only
	Weapons     [3]int32 // main hand, off hand, ranged (level 20)
	Preset      bool     // use the first popular talent build (level 60)
	SpecOptions interface{}
	Ranged      bool
}

func (s Scenario) Name() string { return fmt.Sprintf("%s L%d", s.Spec, s.Level) }

func specs() []Scenario {
	return []Scenario{
		{Spec: "warrior", Class: proto.Class_ClassWarrior, Race: proto.Race_RaceOrc, APL: "warrior/apls/leveling", Gear: "warrior/gear_sets/phase_1", Weapons: [3]int32{1318, 0, 0},
			SpecOptions: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{}}}},
		{Spec: "rogue", Class: proto.Class_ClassRogue, Race: proto.Race_RaceHuman, APL: "rogue/apls/leveling_sinister_strike", Gear: "rogue/gear_sets/combat_sinister_strike_prebis", Weapons: [3]int32{5191, 5192, 0},
			SpecOptions: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}}},
		{Spec: "hunter", Class: proto.Class_ClassHunter, Race: proto.Race_RaceNightElf, APL: "hunter/apls/leveling", Gear: "hunter/gear_sets/p1.bis", Weapons: [3]int32{1318, 0, 6469}, Ranged: true,
			SpecOptions: &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{PetType: proto.Hunter_Options_Cat, PetUptime: 1}}}},
		{Spec: "mage", Class: proto.Class_ClassMage, Race: proto.Race_RaceGnome, APL: "mage/apls/leveling_fireball", Gear: "mage/gear_sets/p1.bis", Weapons: [3]int32{1484, 0, 0}, Ranged: true,
			SpecOptions: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}},
		{Spec: "warlock", Class: proto.Class_ClassWarlock, Race: proto.Race_RaceUndead, APL: "warlock/apls/rotation", Gear: "warlock/gear_sets/prebis", Weapons: [3]int32{1484, 0, 0}, Ranged: true,
			SpecOptions: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{}}}},
		{Spec: "shadow_priest", Class: proto.Class_ClassPriest, Race: proto.Race_RaceUndead, APL: "shadow_priest/apls/p1", Gear: "shadow_priest/gear_sets/p1.bis", Weapons: [3]int32{1484, 0, 0}, Ranged: true,
			SpecOptions: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}},
		{Spec: "balance_druid", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, APL: "balance_druid/apls/balance", Gear: "balance_druid/gear_sets/p1.bis", Weapons: [3]int32{1484, 0, 0}, Ranged: true,
			SpecOptions: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}}},
		{Spec: "feral_druid", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, APL: "feral_druid/apls/leveling", Gear: "feral_druid/gear_sets/p1.bis", Weapons: [3]int32{1484, 0, 0},
			SpecOptions: &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{}}}},
		{Spec: "retribution_paladin", Class: proto.Class_ClassPaladin, Race: proto.Race_RaceHuman, APL: "retribution_paladin/apls/basic_ret", Weapons: [3]int32{3194, 0, 0},
			SpecOptions: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{}}}},
		{Spec: "elemental_shaman", Class: proto.Class_ClassShaman, Race: proto.Race_RaceTroll, APL: "elemental_shaman/apls/default", Gear: "elemental_shaman/gear_sets/phase_1", Weapons: [3]int32{1484, 0, 0}, Ranged: true,
			SpecOptions: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}}}},
		{Spec: "enhancement_shaman", Class: proto.Class_ClassShaman, Race: proto.Race_RaceOrc, APL: "enhancement_shaman/apls/default", Gear: "enhancement_shaman/gear_sets/phase_1", Weapons: [3]int32{3194, 0, 0},
			SpecOptions: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{}}}},
	}
}

// Scenarios is every spec at level 20 and 60.
func Scenarios() []Scenario {
	var out []Scenario
	for _, s := range specs() {
		l20 := s
		l20.Level, l20.Gear, l20.Preset = 20, "", false
		l60 := s
		l60.Level, l60.Preset = 60, true
		out = append(out, l20, l60)
	}
	return out
}

func readJSON(path string, into interface{}) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, into)
}

// presetTalents is the first popular build for a spec (ui/app/data/presets.json), in record ids.
func presetTalents(uiDir, spec string) map[string]int32 {
	var presets map[string][]struct {
		Talents map[string]int32 `json:"talents"`
	}
	if err := readJSON(filepath.Join(uiDir, "app", "data", "presets.json"), &presets); err != nil || len(presets[spec]) == 0 {
		return map[string]int32{}
	}
	return presets[spec][0].Talents
}

// Player builds the scenario's player for a client build label ("" = current).
func (s Scenario) Player(uiDir, build string) *proto.Player {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	if s.Gear != "" {
		parts := strings.Split(s.Gear, "/")
		equipment = core.GetGearSet(filepath.Join(uiDir, parts[0], "gear_sets"), parts[2]).GearSet
	} else {
		for i, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand, proto.ItemSlot_ItemSlotRanged} {
			equipment.Items[slot] = &proto.ItemSpec{Id: s.Weapons[i]}
		}
	}
	apl := strings.Split(s.APL, "/")
	talents := map[string]int32{}
	if s.Preset {
		talents = presetTalents(uiDir, s.Spec)
	}
	player := &proto.Player{
		Name: s.Name(), Class: s.Class, Race: s.Race, Level: s.Level, Equipment: equipment,
		Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
		Rotation: core.GetAplRotation(filepath.Join(uiDir, apl[0], "apls"), apl[2]).Rotation,
		Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS,
			Talents: talents, GameDataBuild: build},
	}
	if s.Ranged {
		player.DistanceFromTarget = 25
	}
	return core.WithSpec(player, s.SpecOptions)
}

type Result struct {
	Scenario string  `json:"scenario"`
	Build    string  `json:"build"`
	DPS      float64 `json:"dps"`
	Stdev    float64 `json:"stdev"`
	Error    string  `json:"error,omitempty"`
}

// Run simulates one scenario against one build.
func (s Scenario) Run(uiDir, build string, iterations int32) Result {
	armor := 850.0
	targetLevel := s.Level + 2
	if s.Level >= 60 {
		armor, targetLevel = 3731, 63
	}
	request := &proto.RaidSimRequest{
		Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{s.Player(uiDir, build)}}},
			Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 180, Targets: []*proto.Target{{Level: targetLevel, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: armor}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 20260920},
	}
	result := core.RunRaidSim(request)
	out := Result{Scenario: s.Name(), Build: build}
	if result.Error != nil {
		out.Error = result.Error.Message
		if i := strings.Index(out.Error, "\nStack Trace"); i > 0 {
			out.Error = out.Error[:i]
		}
		return out
	}
	unit := result.RaidMetrics.Parties[0].Players[0]
	out.DPS, out.Stdev = unit.Dps.Avg, unit.Dps.Stdev
	return out
}

// Compare runs every scenario on each build and returns results keyed by scenario then build.
func Compare(uiDir string, builds []string, iterations int32, only []string) map[string]map[string]Result {
	out := map[string]map[string]Result{}
	for _, s := range Scenarios() {
		if len(only) > 0 && !contains(only, s.Spec) {
			continue
		}
		out[s.Name()] = map[string]Result{}
		for _, build := range builds {
			out[s.Name()][build] = s.Run(uiDir, build, iterations)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// SortedNames lists scenario names in a stable order.
func SortedNames(results map[string]map[string]Result) []string {
	names := make([]string, 0, len(results))
	for name := range results {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
