package paladin_test

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/paladin/retribution"
	"github.com/wowsims/classic/sim/shaman/elemental"
	"github.com/wowsims/classic/sim/shaman/enhancement"
)

// Forever Retribution / Elemental / Enhancement sims driven by the client build. The damage
// specs are registered here beside the hybrid factories this package already uses.

func init() {
	retribution.RegisterRetributionPaladin()
	elemental.RegisterElementalShaman()
	enhancement.RegisterEnhancementShaman()
}

type foreverSpec struct {
	name    string
	talents []struct {
		id   string
		rank int32
	}
	player func(level int32) *proto.Player
	apl    func(level int32) string
}

func foreverTag(id string) int32 { return foreverdata.ActionTag(id) }

func castTag(id string) string {
	return fmt.Sprintf(`{"action":{"castSpell":{"spellId":{"otherId":"OtherActionForever","tag":%d}}}}`, foreverTag(id))
}

func foreverSpecs() []foreverSpec {
	type tr = struct {
		id   string
		rank int32
	}
	equip := func(mh int32) *proto.EquipmentSpec {
		e := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
		for i := range e.Items {
			e.Items[i] = &proto.ItemSpec{}
		}
		e.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: mh}
		return e
	}
	bonus := &proto.UnitStats{Stats: stats.Stats{stats.AttackPower: 600, stats.SpellPower: 200, stats.MeleeCrit: 10, stats.SpellCrit: 5, stats.Mana: 4000}.ToFloatArray()}
	return []foreverSpec{
		{name: "RetributionPaladin",
			talents: []tr{{"paladin.talent.seal-of-command", 1}, {"paladin.talent.vengeance", 3}, {"paladin.talent.improved-seals", 3},
				{"paladin.talent.sanctified-judgement", 3}, {"paladin.talent.improved-holy-strike", 2}, {"paladin.talent.sacred-arbiter", 1}},
			player: func(level int32) *proto.Player {
				return core.WithSpec(&proto.Player{Name: "ret", Class: proto.Class_ClassPaladin, Race: proto.Race_RaceHuman, Level: level, Equipment: equip(3194), Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{}, BonusStats: bonus},
					&proto.Player_RetributionPaladin{RetributionPaladin: &proto.RetributionPaladin{Options: &proto.PaladinOptions{PrimarySeal: core.Ternary(level == 60, proto.PaladinSeal_Command, proto.PaladinSeal_Righteousness)}}})
			},
			apl: func(int32) string {
				return `{"type":"TypeAPL","prepullActions":[{"action":{"castPaladinPrimarySeal":{}},"doAtValue":{"const":{"val":"-1.5s"}}}],"priorityList":[
				{"action":{"condition":{"cmp":{"op":"OpLe","lhs":{"currentSealRemainingTime":{}},"rhs":{"const":{"val":"1"}}}},"castPaladinPrimarySeal":{}}},
				{"action":{"castSpell":{"spellId":{"spellId":20271}}}},` + castTag("paladin.baseline.holy-strike") + `]}`
			}},
		{name: "ElementalShaman",
			talents: []tr{{"shaman.talent.lightning-overload", 3}, {"shaman.talent.lava-burst", 1}, {"shaman.talent.elemental-fury", 5},
				{"shaman.talent.call-of-flame", 3}, {"shaman.talent.improved-fire-nova", 2}},
			player: func(level int32) *proto.Player {
				return core.WithSpec(&proto.Player{Name: "ele", Class: proto.Class_ClassShaman, Race: proto.Race_RaceTroll, Level: level, Equipment: equip(1484), Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{}, BonusStats: bonus, DistanceFromTarget: 8},
					&proto.Player_ElementalShaman{ElementalShaman: &proto.ElementalShaman{Options: &proto.ElementalShaman_Options{}}})
			},
			apl: func(int32) string {
				return `{"type":"TypeAPL","priorityList":[` + castTag("shaman.talent.lava-burst") + `,` + castTag("shaman.baseline.fire-nova") + `,
				{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":29228}}}}},"castSpell":{"spellId":{"spellId":29228}}}},
				{"action":{"castSpell":{"spellId":{"spellId":15208,"rank":10}}}}]}`
			}},
		{name: "EnhancementShaman",
			talents: []tr{{"shaman.talent.stormstrike", 1}, {"shaman.talent.maelstrom-weapon", 5}, {"shaman.talent.flurry", 5},
				{"shaman.talent.elemental-weapons", 3}, {"shaman.talent.improved-stormstrike", 2}},
			player: func(level int32) *proto.Player {
				imbue := proto.WeaponImbue_WindfuryWeapon
				if level < 30 {
					imbue = proto.WeaponImbue_RockbiterWeapon
				}
				return core.WithSpec(&proto.Player{Name: "enh", Class: proto.Class_ClassShaman, Race: proto.Race_RaceOrc, Level: level, Equipment: equip(3194), Consumes: &proto.Consumes{MainHandImbue: imbue}, Buffs: &proto.IndividualBuffs{}, BonusStats: bonus},
					&proto.Player_EnhancementShaman{EnhancementShaman: &proto.EnhancementShaman{Options: &proto.EnhancementShaman_Options{}}})
			},
			apl: func(level int32) string {
				maelstrom := ""
				if level == 60 {
					maelstrom = `{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"auraNumStacks":{"auraId":{"otherId":"OtherActionForever","tag":` + fmt.Sprint(foreverTag("shaman.talent.maelstrom-weapon")) + `}}},"rhs":{"const":{"val":"5"}}}},"castSpell":{"spellId":{"spellId":15208,"rank":10}}}},`
				}
				return `{"type":"TypeAPL","prepullActions":[
				{"action":{"castSpell":{"spellId":{"spellId":25361,"rank":5}}},"doAtValue":{"const":{"val":"-3s"}}},
				{"action":{"castSpell":{"spellId":{"spellId":10438,"rank":6}}},"doAtValue":{"const":{"val":"-1.5s"}}}],"priorityList":[
				{"action":{"castSpell":{"spellId":{"spellId":17364,"rank":1}}}},
				` + maelstrom + `
				{"action":{"condition":{"not":{"val":{"dotIsActive":{"spellId":{"spellId":10438,"rank":6}}}}},"castSpell":{"spellId":{"spellId":10438,"rank":6}}}},
				{"action":{"condition":{"cmp":{"op":"OpGe","lhs":{"currentManaPercent":{}},"rhs":{"const":{"val":"50%"}}}},"castSpell":{"spellId":{"spellId":10414,"rank":7}}}}]}`
			}},
	}
}

// runForeverSpec simulates one spec at a level; talents apply at 60 only (the builds need 51
// points). build "" is the embedded client build.
func runForeverSpec(t *testing.T, spec foreverSpec, level int32, build string, iterations int32) (float64, *core.Character) {
	t.Helper()
	talents := map[string]int32{}
	if level == 60 {
		for _, tr := range spec.talents {
			talents = mergeTalents(talents, hybridTalents(t, tr.id, tr.rank))
		}
	}
	player := spec.player(level)
	player.Rotation = core.APLRotationFromJsonString(spec.apl(level))
	player.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: talents, GameDataBuild: build}
	target := &proto.Target{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850 + 45*float64(level)}.ToFloatArray()}
	request := &proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 120, Targets: []*proto.Target{target}},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 101},
	}
	result := core.RunRaidSim(request)
	if result.Error != nil {
		t.Fatalf("%s level %d: %s", spec.name, level, result.Error.Message)
	}
	env, _, _ := core.NewEnvironment(request.Raid, request.Encounter, false)
	if os.Getenv("FOREVER_DPS_REPORT") == "casts" {
		for _, a := range result.RaidMetrics.Parties[0].Players[0].Actions {
			casts, dmg := int32(0), 0.0
			for _, tg := range a.Targets {
				casts += tg.Casts
				dmg += tg.Damage
			}
			fmt.Printf("  %s L%d %v casts %d dmg %.0f\n", spec.name, level, a.Id, casts, dmg/float64(iterations))
		}
	}
	return result.RaidMetrics.Parties[0].Players[0].Dps.Avg, env.Raid.Parties[0].Players[0].GetCharacter()
}

// FOREVER_DPS_REPORT=1 prints Forever DPS per spec and level (the baseline the data-driven
// change is compared against).
func TestForeverClientValuesDPSReport(t *testing.T) {
	if os.Getenv("FOREVER_DPS_REPORT") == "" {
		t.Skip("set FOREVER_DPS_REPORT=1")
	}
	var lines []string
	for _, spec := range foreverSpecs() {
		for _, level := range []int32{20, 40, 60} {
			dps, _ := runForeverSpec(t, spec, level, "", 400)
			lines = append(lines, fmt.Sprintf("%-20s L%d %9.2f", spec.name, level, dps))
		}
	}
	fmt.Println(strings.Join(lines, "\n"))
}

// The Forever Ret/Ele/Enh sims at 60 use client values everywhere except the documented,
// recorded fallbacks: proc rates and internal cooldowns the client does not state.
func TestForeverDamageSpecsRecordOnlyDocumentedFallbacks(t *testing.T) {
	documented := map[string]bool{}
	for _, owner := range documentedFallbacks {
		documented[owner] = true
	}
	for _, spec := range foreverSpecs() {
		if dps, _ := runForeverSpec(t, spec, 60, "", 20); dps <= 0 {
			t.Fatalf("%s: no damage", spec.name)
		}
	}
	var unexpected []string
	for _, f := range gamedata.Fallbacks() {
		key := f.Owner + "|" + f.What
		if strings.HasPrefix(f.Owner, "paladin") || strings.HasPrefix(f.Owner, "shaman") || strings.HasPrefix(f.Owner, "spell ") || strings.HasPrefix(f.Owner, "talent paladin") || strings.HasPrefix(f.Owner, "talent shaman") {
			if !documented[key] {
				unexpected = append(unexpected, fmt.Sprintf("%s = %v (%s: %s)", key, f.Value, f.Confidence, f.Reason))
			}
		}
	}
	sort.Strings(unexpected)
	if len(unexpected) > 0 {
		t.Errorf("undocumented fallbacks:\n%s", strings.Join(unexpected, "\n"))
	}
}

// Every non-client value the Forever damage specs are allowed to use, by owner|what.
var documentedFallbacks = []string{}
