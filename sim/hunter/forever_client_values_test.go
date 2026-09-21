package hunter

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

// Values in these tests are the Forever beta client's (talentsforever.com export of
// client 1.60.1): talent rank text and spellbook tooltips.

// foreverTalentSet returns a legal Forever talent selection holding every goal at its
// rank, filling lower rows of the same tree to meet the point requirements.
func foreverTalentSet(t *testing.T, goals map[string]int32) map[string]int32 {
	t.Helper()
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Records []foreverdata.Record }
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	chosen := map[string]int32{}
	var add func(foreverdata.Record, int32)
	add = func(r foreverdata.Record, n int32) {
		for _, pre := range r.Prerequisites {
			p, _ := foreverdata.Lookup(pre.ID)
			add(p, pre.Rank)
		}
		var tree []foreverdata.Record
		for _, o := range data.Records {
			if o.Class == r.Class && o.Tree == r.Tree && o.Row < r.Row {
				tree = append(tree, o)
			}
		}
		sort.SliceStable(tree, func(i, j int) bool { return tree[i].Row < tree[j].Row })
		for {
			below := int32(0)
			for id, k := range chosen {
				p, _ := foreverdata.Lookup(id)
				if p.Tree == r.Tree && p.Row < r.Row {
					below += k
				}
			}
			if below >= r.RequiredPoints {
				break
			}
			found := false
			for _, p := range tree {
				if _, goal := goals[p.ID]; !goal && chosen[p.ID] < p.MaxRank && len(p.Prerequisites) == 0 {
					add(p, chosen[p.ID]+1)
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("cannot form legal tree for %s", r.ID)
			}
		}
		chosen[r.ID] = max(chosen[r.ID], n)
	}
	ids := make([]string, 0, len(goals))
	for id := range goals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r, ok := foreverdata.Lookup(id)
		if !ok {
			t.Fatal(id)
		}
		add(r, goals[id])
	}
	return chosen
}

func foreverHunterSim(t *testing.T, level int32, goals map[string]int32) (*core.Simulation, *Hunter) {
	t.Helper()
	equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
	for i := range equipment.Items {
		equipment.Items[i] = &proto.ItemSpec{}
	}
	equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1318}
	equipment.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{Id: 1318}
	equipment.Items[proto.ItemSlot_ItemSlotRanged] = &proto.ItemSpec{Id: 6469}
	p := core.WithSpec(&proto.Player{Name: "Hunter", Class: proto.Class_ClassHunter, Race: proto.Race_RaceNightElf, Level: level,
		DistanceFromTarget: 25, Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
		Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: foreverTalentSet(t, goals),
			Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true},
	}, &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{
		Ammo: proto.Hunter_Options_JaggedArrow, PetType: proto.Hunter_Options_Cat, PetUptime: 1}}})
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 3, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Health: 100000}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(HunterAgent).GetHunter()
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func TestForeverHunterRankTables(t *testing.T) {
	for level, want := range map[int32][2]float64{25: {32, 80}, 35: {32, 80}, 36: {47, 105}, 48: {85, 135}, 60: {108, 190}} {
		damage, mana := summonHawkRank(level)
		near(t, "Summon Hawk damage", damage, want[0])
		near(t, "Summon Hawk mana", mana, want[1])
	}
	for level, want := range map[int32]float64{40: 160, 47: 160, 48: 225, 58: 295, 60: 295} {
		_, bonus := sniperShotRank(level)
		near(t, "Sniper Shot bonus", bonus, want)
	}
	for level, want := range map[int32]float64{25: 30, 32: 40, 40: 50, 50: 75, 60: 50} {
		rap, _ := trueshotAuraRank(level)
		near(t, "Trueshot Aura RAP", rap, want)
	}
	for level, want := range map[int32]float64{30: 20, 42: 35, 54: 55, 60: 55} {
		bonus, _ := counterattackRank(level)
		near(t, "Counterattack bonus", bonus, want)
	}
}

// Forever Aimed Shot is baseline (2 sec cast, shares Multi-Shot's 6 sec cooldown); Sniper
// Shot is its own spell, and Barrage (3/7/10%) buffs Aimed Shot but not Sniper Shot.
func TestForeverAimedShotSniperShotBarrage(t *testing.T) {
	_, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.sniper-shot": 1, "hunter.talent.barrage": 3})
	if h.AimedShot == nil || h.AimedShot.ActionID.SpellID != 20904 {
		t.Fatalf("Aimed Shot rank 6 not registered: %v", h.AimedShot)
	}
	if h.SniperShot == nil || h.SniperShot == h.AimedShot {
		t.Fatal("Sniper Shot must be a separate spell")
	}
	if h.AimedShot.DefaultCast.CastTime != 2*time.Second || h.AimedShot.CD.Timer != h.MultiShot.CD.Timer || h.MultiShot.CD.Duration != 6*time.Second {
		t.Errorf("Aimed Shot cast %v, Multi-Shot cooldown %v, shared %v", h.AimedShot.DefaultCast.CastTime, h.MultiShot.CD.Duration, h.AimedShot.CD.Timer == h.MultiShot.CD.Timer)
	}
	if h.MultiShot.ActionID.SpellID != 2643 {
		t.Errorf("Forever Multi-Shot has one rank, got %d", h.MultiShot.ActionID.SpellID)
	}
	near(t, "Aimed Shot Barrage", h.AimedShot.DamageMultiplier, 1.10)
	near(t, "Multi-Shot Barrage", h.MultiShot.DamageMultiplier, 1.10)
	near(t, "Sniper Shot Barrage", h.SniperShot.DamageMultiplier, 1)
	if h.SniperShot.CD.Duration != 15*time.Second || h.SniperShot.DefaultCast.CastTime != 4*time.Second {
		t.Errorf("Sniper Shot cast %v cooldown %v", h.SniperShot.DefaultCast.CastTime, h.SniperShot.CD.Duration)
	}

	_, low := foreverHunterSim(t, 20, nil)
	if low.AimedShot == nil || low.AimedShot.ActionID.SpellID != 19434 {
		t.Errorf("level 20 Aimed Shot rank 1 missing")
	}
}

// Improved Stings: Serpent Sting +6/13/20%.
func TestForeverImprovedStings(t *testing.T) {
	for rank, want := range map[int32]float64{2: 1.13, 3: 1.20} {
		_, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.improved-stings": rank})
		near(t, "Serpent Sting multiplier", h.SerpentSting.DamageMultiplier, want)
	}
}

// Mortal Shots is ranged only; Predator's Edge covers every melee crit, white swings too.
func TestForeverMortalShotsAndPredatorsEdge(t *testing.T) {
	_, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.mortal-shots": 5, "hunter.talent.predator-s-edge": 5})
	_, plain := foreverHunterSim(t, 60, nil)
	bonus := func(h *Hunter, s *core.Spell) float64 { return s.CritDamageBonus }
	near(t, "Raptor Strike crit bonus", bonus(h, h.RaptorStrike)-bonus(plain, plain.RaptorStrike), .30)
	near(t, "Wing Clip crit bonus", bonus(h, h.WingClip)-bonus(plain, plain.WingClip), .30)
	near(t, "Arcane Shot crit bonus", bonus(h, h.ArcaneShot)-bonus(plain, plain.ArcaneShot), .30)
	near(t, "melee auto crit bonus", h.AutoAttacks.MHAuto().CritDamageBonus-plain.AutoAttacks.MHAuto().CritDamageBonus, .30)
}

// Volley: 112 Arcane damage per second at rank 3, and Forever Efficiency does not cover it.
func TestForeverVolley(t *testing.T) {
	_, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.efficiency": 5})
	if h.Volley == nil {
		t.Fatal("no Volley")
	}
	if h.Volley.Cost.Multiplier != 100 {
		t.Errorf("Volley cost multiplier %d", h.Volley.Cost.Multiplier)
	}
	if h.MultiShot.Cost.Multiplier != 85 {
		t.Errorf("Multi-Shot cost multiplier %d, want 85", h.MultiShot.Cost.Multiplier)
	}
}

// Focused Fire: +2% on the pet's damage too.
func TestForeverFocusedFirePet(t *testing.T) {
	_, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.focused-fire": 2})
	_, plain := foreverHunterSim(t, 60, nil)
	near(t, "pet damage multiplier ratio", h.pet.PseudoStats.DamageDealtMultiplier/plain.pet.PseudoStats.DamageDealtMultiplier, 1.02)
}

// Trueshot Aura at 60 is client rank 5: 50 ranged attack power for 525 mana.
func TestForeverTrueshotAura(t *testing.T) {
	sim, h := foreverHunterSim(t, 60, map[string]int32{"hunter.talent.trueshot-aura": 1})
	spell := h.GetSpell(h.ForeverAction("hunter.talent.trueshot-aura"))
	near(t, "Trueshot mana", spell.Cost.BaseCost, 525)
	before := h.GetStat(stats.RangedAttackPower)
	h.GetAura("Forever Trueshot Aura").Activate(sim)
	near(t, "Trueshot RAP", h.GetStat(stats.RangedAttackPower)-before, 50)
}

// Forever hunters of every tree run the shipped leveling rotation at 20, 40 and 60
// without errors, with damage, and without casting a rank above their level.
func TestForeverHunterLevelingSmoke(t *testing.T) {
	builds := []map[string]int32{
		{"hunter.talent.summon-hawk": 1, "hunter.talent.focused-fire": 2},
		{"hunter.talent.sniper-shot": 1, "hunter.talent.barrage": 3, "hunter.talent.trueshot-aura": 1, "hunter.talent.mortal-shots": 5, "hunter.talent.rapid-killing": 2},
		{"hunter.talent.counterattack": 1, "hunter.talent.lacerating-strikes": 1, "hunter.talent.predator-s-edge": 5},
	}
	rotation := core.GetAplRotation("../../ui/hunter/apls", "leveling").Rotation
	for _, level := range []int32{20, 40, 60} {
		for i, goals := range builds {
			equipment := &proto.EquipmentSpec{Items: make([]*proto.ItemSpec, 17)}
			for j := range equipment.Items {
				equipment.Items[j] = &proto.ItemSpec{}
			}
			equipment.Items[proto.ItemSlot_ItemSlotMainHand] = &proto.ItemSpec{Id: 1318}
			equipment.Items[proto.ItemSlot_ItemSlotRanged] = &proto.ItemSpec{Id: 6469}
			p := core.WithSpec(&proto.Player{Name: "Hunter", Class: proto.Class_ClassHunter, Race: proto.Race_RaceNightElf, Level: level,
				DistanceFromTarget: 25, Equipment: equipment, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{}, Rotation: rotation,
				Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: foreverTalentSet(t, goals), Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true},
			}, &proto.Player_Hunter{Hunter: &proto.Hunter{Options: &proto.Hunter_Options{Ammo: proto.Hunter_Options_JaggedArrow, PetType: proto.Hunter_Options_Cat, PetUptime: 1}}})
			result := core.RunRaidSim(&proto.RaidSimRequest{
				Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
				Encounter:  &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
				SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 3},
			})
			if result.Error != nil {
				t.Fatalf("level %d build %d: %s", level, i, result.Error.Message)
			}
			unit := result.RaidMetrics.Parties[0].Players[0]
			if unit.Dps.Avg <= 0 {
				t.Errorf("level %d build %d: no damage", level, i)
			}
			for id := range castIDs(unit) {
				if learned := core.SpellLearnedLevel(id); learned > level {
					t.Errorf("level %d build %d: cast %d learned at %d", level, i, id, learned)
				}
			}
		}
	}
}
