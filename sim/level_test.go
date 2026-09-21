package sim

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Every DPS spec at level 20, naked but for level-20 Forever dungeon weapons, running the
// rotation the site ships. It fails on a panic or zero damage, and on any cast of a spell
// rank the character is too low to have learned.

type levelCase struct {
	name     string
	class    proto.Class
	race     proto.Race
	spec     interface{}
	apl      string
	weapons  [3]int32 // main hand, off hand, ranged
	talents  string
	skipRank map[int32]bool
}

func levelCases() []levelCase {
	return []levelCase{
		{name: "Rogue", class: proto.Class_ClassRogue, race: proto.Race_RaceHuman, apl: "rogue/apls/leveling_sinister_strike",
			spec: &proto.Player_Rogue{Rogue: &proto.Rogue{Options: &proto.RogueOptions{}}}, weapons: [3]int32{5191, 5192, 0}},
		{name: "Warrior", class: proto.Class_ClassWarrior, race: proto.Race_RaceOrc, apl: "warrior/apls/leveling",
			spec: &proto.Player_Warrior{Warrior: &proto.Warrior{Options: &proto.Warrior_Options{StartingRage: 0}}}, weapons: [3]int32{1318, 0, 0}},
		{name: "Hunter", class: proto.Class_ClassHunter, race: proto.Race_RaceNightElf, apl: "hunter/apls/leveling",
			spec:    &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{Ammo: proto.Hunter_Options_JaggedArrow, PetType: proto.Hunter_Options_Cat, PetUptime: 1}}},
			weapons: [3]int32{1318, 0, 6469}},
		{name: "Mage", class: proto.Class_ClassMage, race: proto.Race_RaceGnome, apl: "mage/apls/leveling_fireball",
			spec: &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "Warlock", class: proto.Class_ClassWarlock, race: proto.Race_RaceUndead, apl: "warlock/apls/rotation",
			spec: &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "ShadowPriest", class: proto.Class_ClassPriest, race: proto.Race_RaceUndead, apl: "shadow_priest/apls/p1",
			spec: &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "BalanceDruid", class: proto.Class_ClassDruid, race: proto.Race_RaceTauren, apl: "balance_druid/apls/balance",
			spec: &proto.Player_BalanceDruid{BalanceDruid: &proto.BalanceDruid{Options: &proto.BalanceDruid_Options{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "FeralDruid", class: proto.Class_ClassDruid, race: proto.Race_RaceTauren, apl: "feral_druid/apls/leveling",
			spec: &proto.Player_FeralDruid{FeralDruid: &proto.FeralDruid{Options: &proto.FeralDruid_Options{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "RetributionPaladin", class: proto.Class_ClassPaladin, race: proto.Race_RaceHuman, apl: "retribution_paladin/apls/basic_ret",
			spec: &proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{}}}, weapons: [3]int32{3194, 0, 0}},
		{name: "ElementalShaman", class: proto.Class_ClassShaman, race: proto.Race_RaceTroll, apl: "elemental_shaman/apls/default",
			spec: &proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}}}, weapons: [3]int32{1484, 0, 0}},
		{name: "EnhancementShaman", class: proto.Class_ClassShaman, race: proto.Race_RaceOrc, apl: "enhancement_shaman/apls/default",
			spec: &proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{}}}, weapons: [3]int32{3194, 0, 0}},
	}
}

func levelPlayer(c levelCase, level int32) *proto.Player {
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	for i, slot := range []proto.ItemSlot{proto.ItemSlot_ItemSlotMainHand, proto.ItemSlot_ItemSlotOffHand, proto.ItemSlot_ItemSlotRanged} {
		equipment.Items[slot] = &proto.ItemSpec{Id: c.weapons[i]}
	}
	path := strings.Split(c.apl, "/")
	player := &proto.Player{
		Name: c.name, Class: c.class, Race: c.race, Level: level,
		Equipment:     equipment,
		TalentsString: c.talents,
		Consumes:      &proto.Consumes{},
		Buffs:         &proto.IndividualBuffs{},
		Rotation:      core.GetAplRotation("../ui/"+path[0]+"/apls", path[2]).Rotation,
	}
	if c.class != proto.Class_ClassRogue && c.class != proto.Class_ClassWarrior && c.name != "FeralDruid" &&
		c.name != "RetributionPaladin" && c.name != "EnhancementShaman" {
		player.DistanceFromTarget = 25
	}
	return core.WithSpec(player, c.spec)
}

func runLevel(t *testing.T, player *proto.Player, level int32) *proto.RaidSimResult {
	target := &proto.Target{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid,
		Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}
	return core.RunRaidSim(&proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 120, Targets: []*proto.Target{target}},
		SimOptions: &proto.SimOptions{Iterations: 200, RandomSeed: 101},
	})
}

func TestLevel20Specs(t *testing.T) {
	var report []string
	for _, c := range levelCases() {
		t.Run(c.name, func(t *testing.T) {
			result := runLevel(t, levelPlayer(c, 20), 20)
			if result.Error != nil {
				t.Fatalf("%s: %s", c.name, result.Error.Message)
			}
			unit := result.RaidMetrics.Parties[0].Players[0]
			dps := unit.Dps.Avg
			var cast []string
			var tooHigh []string
			for _, action := range unit.Actions {
				id := action.Id.GetSpellId()
				if id == 0 {
					continue
				}
				casts := int32(0)
				for _, target := range action.Targets {
					casts += target.Casts
				}
				if casts == 0 {
					continue
				}
				cast = append(cast, fmt.Sprintf("%d", id))
				if learned := core.SpellLearnedLevel(id); learned > 20 && !c.skipRank[id] {
					tooHigh = append(tooHigh, fmt.Sprintf("%d (learned at %d)", id, learned))
				}
			}
			sort.Strings(cast)
			report = append(report, fmt.Sprintf("%-20s %8.1f dps  casts %s", c.name, dps, strings.Join(cast, ",")))
			if dps <= 0 {
				t.Errorf("%s: no damage at level 20", c.name)
			}
			if len(tooHigh) > 0 {
				t.Errorf("%s: cast spell ranks above level 20: %s", c.name, strings.Join(tooHigh, ", "))
			}
		})
	}
	if os.Getenv("LEVEL_REPORT") != "" {
		fmt.Println(strings.Join(report, "\n"))
	}
}

// TestClientDataAudit lists, per spec at level 60 in Forever mode, what the client-data layer did
// with each registered spell's cost, cooldown and cast time. Run with CLIENT_AUDIT=1 to print.
func TestClientDataAudit(t *testing.T) {
	for _, c := range levelCases() {
		player := levelPlayer(c, 60)
		player.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: map[string]int32{}}
		target := &proto.Target{Level: 63, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 3731}.ToFloatArray()}
		result, report := core.RunRaidSimWithForeverOverrideReport(&proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
			Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{target}},
			SimOptions: &proto.SimOptions{Iterations: 10, RandomSeed: 7},
		})
		if result.Error != nil {
			t.Fatalf("%s: %s", c.name, result.Error.Message)
		}
		applied, disagreements := 0, 0
		var lines []string
		for _, p := range report.Players {
			for _, ev := range append(append([]foreverdata.OverrideEvent{}, p.Applied...), p.Rejected...) {
				if ev.Scope != "client_data" {
					continue
				}
				if ev.Applied {
					applied++
				} else {
					disagreements++
				}
				lines = append(lines, fmt.Sprintf("    %s %s applied=%v registered=%v forever=%v %s", ev.ID, ev.Field, ev.Applied, deref(ev.Registered), deref(ev.Forever), ev.Reason))
			}
		}
		if os.Getenv("CLIENT_AUDIT") != "" {
			fmt.Printf("%s: %d applied, %d disagreements\n%s\n", c.name, applied, disagreements, strings.Join(lines, "\n"))
		}
	}
}

func deref(p *float64) float64 {
	if p == nil {
		return math.NaN()
	}
	return *p
}
