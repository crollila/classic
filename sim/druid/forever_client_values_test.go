package druid

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"strings"
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

func init() {
	RegisterForeverDruidSpecs()
}

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
			if o.Class == r.Class && o.Tree == r.Tree && o.Row < r.Row && o.ID != "druid.talent.balance-of-nature" {
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
				if chosen[p.ID] < p.MaxRank && len(p.Prerequisites) == 0 {
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

func foreverDruidSim(t *testing.T, level int32, goals map[string]int32) (*core.Simulation, *Druid) {
	t.Helper()
	p := &proto.Player{Name: "Druid", Class: proto.Class_ClassDruid, Race: proto.Race_RaceNightElf, Level: level,
		Equipment: &proto.EquipmentSpec{}, Consumes: &proto.Consumes{},
		BonusStats: &proto.UnitStats{Stats: stats.Stats{stats.Mana: 20000, stats.Health: 10000}.ToFloatArray()},
		Rotation:   &proto.APLRotation{Type: proto.APLRotation_TypeAPL},
		Spec:       &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}},
		Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: foreverTalentSet(t, goals),
			Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true}}
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}},
		Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: level + 3, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Health: 100000}.ToFloatArray()}}},
		SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].(DruidAgent).GetDruid()
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

func spellByCode(d *Druid, code int32) *core.Spell {
	var found *core.Spell
	for _, sp := range d.Spellbook {
		if sp.SpellCode == code {
			found = sp // highest registered rank
		}
	}
	return found
}

// Insect Swarm R5 (level 60): 186 Nature damage over 12 sec for 160 mana; R1 48 for 45.
func TestForeverInsectSwarmRanks(t *testing.T) {
	for _, c := range []struct {
		level       int32
		id          int32
		total, mana float64
	}{{60, 24977, 186, 160}, {20, 5570, 48, 45}, {30, 24974, 90, 85}} {
		sim, d := foreverDruidSim(t, c.level, map[string]int32{"druid.talent.insect-swarm": 1})
		sp := d.GetSpell(core.ActionID{SpellID: c.id})
		if sp == nil {
			t.Fatalf("level %d: Insect Swarm %d not registered", c.level, c.id)
		}
		near(t, "Insect Swarm base mana", sp.Cost.BaseCost, c.mana)
		target := sim.Encounter.TargetUnits[0]
		dot := sp.Dot(target)
		dot.OnSnapshot(sim, target, dot, false)
		near(t, "Insect Swarm damage over 12 sec", math.Round(dot.SnapshotBaseDamage*float64(dot.NumberOfTicks)), c.total)
	}
}

// Nature's Grace: spellcasting speed and global cooldown -10% for 3 sec.
func TestForeverNaturesGraceGCD(t *testing.T) {
	sim, d := foreverDruidSim(t, 60, map[string]int32{"druid.talent.nature-s-grace": 1})
	moonfire := spellByCode(d, SpellCode_DruidMoonfire)
	d.GetAura("Forever Nature's Grace").Activate(sim)
	if got := moonfire.DefaultCast.GCD; got != 1350*time.Millisecond {
		t.Errorf("Moonfire GCD under Nature's Grace = %v, want 1.35s", got)
	}
}

// Improved Moonfire: "Increases the damage and critical strike chance of your Moonfire
// spell by 10%" - the spell power part too, not only the base damage.
func TestForeverImprovedMoonfireWholeSpell(t *testing.T) {
	_, plain := foreverDruidSim(t, 60, map[string]int32{"druid.talent.nature-s-reach": 1})
	_, d := foreverDruidSim(t, 60, map[string]int32{"druid.talent.improved-moonfire": 2})
	before, after := spellByCode(plain, SpellCode_DruidMoonfire), spellByCode(d, SpellCode_DruidMoonfire)
	near(t, "Moonfire BaseDamageMultiplierAdditive", after.BaseDamageMultiplierAdditive, 1)
	near(t, "Moonfire DamageMultiplierAdditive gain", after.DamageMultiplierAdditive-before.DamageMultiplierAdditive, .10)
}

// Tiger's Fury: "Increases Physical damage done by 15% for 6 sec", 30 sec cooldown, no cost.
func TestForeverTigersFury(t *testing.T) {
	for _, level := range []int32{24, 60} {
		sim, d := foreverDruidSim(t, level, nil)
		tf := d.GetSpell(core.ActionID{SpellID: 5217})
		if tf == nil || d.GetSpell(core.ActionID{SpellID: 9846}) != nil {
			t.Fatalf("level %d: want the single Forever rank 5217", level)
		}
		if tf.CD.Duration != 30*time.Second || tf.Cost != nil {
			t.Errorf("level %d: Tiger's Fury cooldown %v, cost %v; want 30s and none", level, tf.CD.Duration, tf.Cost)
		}
		before := d.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]
		d.TigersFuryAura.Activate(sim)
		near(t, "physical multiplier", d.PseudoStats.SchoolDamageDealtMultiplier[stats.SchoolIndexPhysical]/before, 1.15)
		if d.TigersFuryAura.Duration != 6*time.Second {
			t.Errorf("Tiger's Fury lasts %v", d.TigersFuryAura.Duration)
		}
	}
}

// Omen of Clarity is baseline from level 20 and is not consumed by Wrath.
func TestForeverOmenOfClarityBaseline(t *testing.T) {
	_, low := foreverDruidSim(t, 19, nil)
	if low.ClearcastingAura != nil {
		t.Error("Omen of Clarity before level 20")
	}
	sim, d := foreverDruidSim(t, 60, nil)
	if d.ClearcastingAura == nil {
		t.Fatal("no baseline Omen of Clarity")
	}
	wrath, starfire := spellByCode(d, SpellCode_DruidWrath), spellByCode(d, SpellCode_DruidStarfire)
	d.ClearcastingAura.Activate(sim)
	sim.CurrentTime += time.Second
	if starfire.Cost.GetCurrentCost() != 0 {
		t.Errorf("Starfire costs %v under Clearcasting", starfire.Cost.GetCurrentCost())
	}
	if wrath.Cost.GetCurrentCost() == 0 {
		t.Error("Wrath is free under Clearcasting")
	}
	d.OnCastComplete(sim, wrath)
	if !d.ClearcastingAura.IsActive() {
		t.Error("Wrath consumed Clearcasting")
	}
	d.OnCastComplete(sim, starfire)
	if d.ClearcastingAura.IsActive() {
		t.Error("Starfire did not consume Clearcasting")
	}
}

// Furor 5/5: 100% of the last Cat Form Energy plus 10 per second out of form, cap 100;
// 1/5: 20% plus 2 per second, cap 20.
func TestForeverFurorEnergy(t *testing.T) {
	_, d := foreverDruidSim(t, 60, map[string]int32{"druid.talent.furor": 5})
	d.foreverLastCatEnergy = 0
	near(t, "Furor 5/5 after 3s", d.foreverFurorEnergy(3*time.Second), 30)
	d.foreverLastCatEnergy = 50
	near(t, "Furor 5/5 capped", d.foreverFurorEnergy(9*time.Second), 100)
	_, d = foreverDruidSim(t, 60, map[string]int32{"druid.talent.furor": 1})
	d.foreverLastCatEnergy = 40
	near(t, "Furor 1/5", d.foreverFurorEnergy(2*time.Second), 12)
}

// Predatory Strikes adds attack power in Cat/Bear only; Vengeance covers Hurricane.
func TestForeverPredatoryStrikesAndVengeance(t *testing.T) {
	_, plain := foreverDruidSim(t, 60, map[string]int32{"druid.talent.heart-of-the-wild": 1})
	_, d := foreverDruidSim(t, 60, map[string]int32{"druid.talent.predatory-strikes": 3})
	near(t, "caster attack power difference", d.GetStat(stats.AttackPower)-plain.GetStat(stats.AttackPower), 0)
	near(t, "Predatory Strikes 3/3 at 60", d.foreverPredatoryStrikesAP(), 90)

	_, v := foreverDruidSim(t, 60, map[string]int32{"druid.talent.vengeance": 5})
	var hurricane *core.Spell
	for _, h := range v.Hurricane {
		if h != nil {
			hurricane = h.Spell
		}
	}
	if hurricane == nil {
		t.Fatal("no Hurricane at 60")
	}
	near(t, "Hurricane Vengeance bonus", hurricane.CritDamageBonus-spellByCode(plain, SpellCode_DruidMoonfire).CritDamageBonus, 1)
	near(t, "Starfire Vengeance bonus", spellByCode(v, SpellCode_DruidStarfire).CritDamageBonus-spellByCode(plain, SpellCode_DruidStarfire).CritDamageBonus, 1)
}

// Balance of Nature is not a Forever client talent: a selected rank changes nothing.
func TestForeverBalanceOfNatureInert(t *testing.T) {
	_, d := foreverDruidSim(t, 60, map[string]int32{"druid.talent.balance-of-nature": 5})
	if d.GetAura("Forever Balance Nature") != nil || d.GetAura("Forever Balance Arcane") != nil {
		t.Error("phantom Balance of Nature auras registered")
	}
}

func TestForeverMangleRanks(t *testing.T) {
	for level, want := range map[int32]float64{25: 26, 36: 38, 47: 38, 48: 59, 60: 77} {
		near(t, "Mangle bonus", foreverMangleBonus(level), want)
	}
}

// A Forever druid runs the shipped Balance and feral leveling rotations at 20, 40 and 60
// without errors, with damage, and without casting a rank above its level.
func TestForeverDruidLevelingSmoke(t *testing.T) {
	builds := []struct {
		apl   string
		goals map[string]int32
	}{
		{"balance_druid/apls/balance", map[string]int32{"druid.talent.insect-swarm": 1, "druid.talent.nature-s-grace": 1, "druid.talent.improved-moonfire": 2, "druid.talent.moonglow": 3}},
		{"feral_druid/apls/leveling", map[string]int32{"druid.talent.furor": 5, "druid.talent.king-of-the-jungle": 3, "druid.talent.predatory-strikes": 3}},
	}
	for _, level := range []int32{20, 40, 60} {
		for _, b := range builds {
			path := strings.Split(b.apl, "/")
			p := &proto.Player{Name: "Druid", Class: proto.Class_ClassDruid, Race: proto.Race_RaceTauren, Level: level,
				Equipment: &proto.EquipmentSpec{Items: []*proto.ItemSpec{}}, Consumes: &proto.Consumes{}, Buffs: &proto.IndividualBuffs{},
				Rotation: core.GetAplRotation("../../ui/"+path[0]+"/apls", path[2]).Rotation,
				Spec:     &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}},
				Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: foreverTalentSet(t, b.goals),
					Mode: proto.ForeverMode_BEST_GUESS, ExperimentalEstimatedRanks: true}}
			result := core.RunRaidSim(&proto.RaidSimRequest{
				Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
				Encounter:  &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
				SimOptions: &proto.SimOptions{Iterations: 20, RandomSeed: 3},
			})
			if result.Error != nil {
				t.Fatalf("level %d %s: %s", level, b.apl, result.Error.Message)
			}
			unit := result.RaidMetrics.Parties[0].Players[0]
			if unit.Dps.Avg <= 0 {
				t.Errorf("level %d %s: no damage", level, b.apl)
			}
			for _, action := range unit.Actions {
				id := action.Id.GetSpellId()
				for _, target := range action.Targets {
					if learned := core.SpellLearnedLevel(id); target.Casts > 0 && learned > level {
						t.Errorf("level %d %s: cast %d learned at %d", level, b.apl, id, learned)
					}
				}
			}
			t.Logf("level %d %s: %.1f dps", level, b.apl, unit.Dps.Avg)
		}
	}
}
