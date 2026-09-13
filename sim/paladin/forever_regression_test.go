package paladin_test

import (
	"encoding/json"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/druid"
	"github.com/wowsims/classic/sim/paladin"
	"github.com/wowsims/classic/sim/shaman"
	"math"
	"os"
	"sort"
	"testing"
	"time"
)

func init() {
	paladin.RegisterForeverHolyPaladin()
	shaman.RegisterForeverRestorationShaman()
	druid.RegisterForeverDruidSpecs()
}

func hybridTalents(t *testing.T, id string, rank int32) map[string]int32 {
	t.Helper()
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Records []foreverdata.Record }
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	goal, ok := foreverdata.Lookup(id)
	if !ok {
		t.Fatal(id)
	}
	var tree []foreverdata.Record
	for _, r := range data.Records {
		if r.Class == goal.Class && r.Tree == goal.Tree {
			tree = append(tree, r)
		}
	}
	sort.SliceStable(tree, func(i, j int) bool { return tree[i].Row < tree[j].Row })
	chosen := map[string]int32{}
	var add func(foreverdata.Record, int32)
	add = func(r foreverdata.Record, n int32) {
		for _, pre := range r.Prerequisites {
			p, _ := foreverdata.Lookup(pre.ID)
			add(p, pre.Rank)
		}
		for {
			below := int32(0)
			for id, n := range chosen {
				p, _ := foreverdata.Lookup(id)
				if p.Row < r.Row {
					below += n
				}
			}
			if below >= r.RequiredPoints {
				break
			}
			found := false
			for _, p := range tree {
				if p.Row < r.Row && chosen[p.ID] < p.MaxRank {
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
	add(goal, rank)
	return chosen
}
func hybridSim(t *testing.T, class string, talents map[string]int32, mode proto.ForeverMode) (*core.Simulation, *core.Character) {
	t.Helper()
	p := &proto.Player{Equipment: &proto.EquipmentSpec{}, Consumes: &proto.Consumes{}, BonusStats: &proto.UnitStats{Stats: stats.Stats{stats.Mana: 20000, stats.Health: 10000, stats.AttackPower: 1000, stats.SpellPower: 100, stats.SpellHit: 20, stats.MeleeHit: 20}.ToFloatArray()}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: talents, Mode: mode, ExperimentalEstimatedRanks: true}}
	switch class {
	case "PALADIN":
		p.Class = proto.Class_ClassPaladin
		p.Race = proto.Race_RaceHuman
		p.Spec = &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.PaladinOptions{RighteousFury: true}}}
		p.Forever.Mechanics = []string{"paladin.baseline.holy-strike", "paladin.baseline.seal-of-fury"}
	case "SHAMAN":
		p.Class = proto.Class_ClassShaman
		p.Race = proto.Race_RaceTroll
		p.Spec = &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{}}}
		p.Forever.Mechanics = []string{"shaman.baseline.fire-nova"}
	case "DRUID":
		p.Class = proto.Class_ClassDruid
		p.Race = proto.Race_RaceNightElf
		p.Spec = &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}}
	}
	sim := core.NewSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Health: 100000}.ToFloatArray()}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 1, Interactive: true, IsTest: true}}, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}
func TestForeverEveryHybridTalentRankConstructs(t *testing.T) {
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Records []foreverdata.Record }
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	for _, r := range data.Records {
		if r.Class != "PALADIN" && r.Class != "SHAMAN" && r.Class != "DRUID" {
			continue
		}
		for n := int32(1); n <= r.MaxRank; n++ {
			t.Run(r.ID+"/"+string(rune('0'+n)), func(t *testing.T) {
				_, c := hybridSim(t, r.Class, hybridTalents(t, r.ID, n), proto.ForeverMode_BEST_GUESS)
				for _, v := range c.GetStats() {
					if math.IsNaN(v) || math.IsInf(v, 0) {
						t.Fatal("non-finite stats")
					}
				}
			})
		}
	}
}
func TestForeverHolyShockVigilAndBulwark(t *testing.T) {
	sim, c := hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.light-s-vigil", 1), proto.ForeverMode_BEST_GUESS)
	target := sim.Encounter.TargetUnits[0]
	vigil := c.GetSpell(c.ForeverAction("paladin.talent.light-s-vigil"))
	shock := c.GetSpell(c.ForeverAction("paladin.talent.holy-shock"))
	if vigil == nil || shock == nil {
		t.Fatal("missing player spells")
	}
	vigil.ApplyEffects(sim, target, vigil)
	mana := c.CurrentMana()
	c.SpendMana(sim, 1000, c.NewManaMetrics(vigil.ActionID))
	shock.ApplyEffects(sim, target, shock)
	if math.Abs(c.CurrentMana()-(mana-1000+547.5)) > .001 {
		t.Fatal("Vigil mana refund", c.CurrentMana())
	}
	if !shock.CD.IsReady(sim) {
		t.Fatal("Vigil failed to reset Holy Shock")
	}
	sim, c = hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.templar-s-bulwark", 1), proto.ForeverMode_BEST_GUESS)
	bulwark := c.GetSpell(c.ForeverAction("paladin.talent.templar-s-bulwark"))
	bulwark.ApplyEffects(sim, &c.Unit, bulwark)
	r := &core.SpellResult{Target: &c.Unit, Damage: 123}
	incoming := c.GetSpell(core.ActionID{SpellID: 20271})
	for _, f := range c.DynamicDamageTakenModifiers {
		f(sim, incoming, r)
	}
	if r.Damage != 0 {
		t.Fatal("Bulwark did not absorb real incoming damage", r.Damage)
	}
}
func TestForeverShamanActualCastsAndMaelstrom(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.lava-burst", 1), proto.ForeverMode_BEST_GUESS)
	lava := c.GetSpell(c.ForeverAction("shaman.talent.lava-burst"))
	if lava == nil || lava.CD.Duration != 10*time.Second {
		t.Fatal("missing Lava Burst")
	}
	lava.ApplyEffects(sim, sim.Encounter.TargetUnits[0], lava)
	sim, c = hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.maelstrom-weapon", 5), proto.ForeverMode_BEST_GUESS)
	a := c.GetAura("Forever Maelstrom Weapon")
	lb := c.GetSpell(core.ActionID{SpellID: 15208})
	if a == nil || lb == nil {
		t.Fatal("missing maelstrom or lightning bolt")
	}
	a.Activate(sim)
	a.SetStacks(sim, 5)
	if math.Abs(lb.CastTimeMultiplier) > .00001 || lb.Cost.GetCurrentCost() != 0 {
		t.Fatal("five stacks must make Lightning Bolt instant and free", lb.CastTimeMultiplier, lb.Cost.GetCurrentCost())
	}
	a.Deactivate(sim)
	if lb.CastTimeMultiplier != 1 {
		t.Fatal("maelstrom reset leaked")
	}
}
func TestForeverDruidEclipseAndMangle(t *testing.T) {
	sim, c := hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.eclipse", 3), proto.ForeverMode_BEST_GUESS)
	wrath := c.GetSpell(core.ActionID{SpellID: 9912})
	star := c.GetSpell(core.ActionID{SpellID: 9876})
	before := star.DefaultCast.CastTime
	c.OnCastComplete(sim, wrath)
	a := c.GetAura("Forever Eclipse")
	if a.GetStacks() != 2 || star.DefaultCast.CastTime != before-500*time.Millisecond {
		t.Fatal("Eclipse did not add two charges/reduce Starfire", a.GetStacks(), star.DefaultCast.CastTime)
	}
	c.OnCastComplete(sim, star)
	if a.GetStacks() != 1 {
		t.Fatal("Starfire must consume one charge")
	}
	sim, c = hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.berserk", 1), proto.ForeverMode_BEST_GUESS)
	bear := c.GetSpell(core.ActionID{SpellID: 9634})
	bear.ApplyEffects(sim, &c.Unit, bear)
	mangle := c.GetSpell(c.ForeverAction("druid.talent.mangle"))
	berserk := c.GetSpell(c.ForeverAction("druid.talent.berserk"))
	berserk.ApplyEffects(sim, &c.Unit, berserk)
	if mangle.CD.Duration != 0 {
		t.Fatal("Berserk did not remove Mangle cooldown")
	}
	mangle.ApplyEffects(sim, sim.Encounter.TargetUnits[0], mangle)
	c.GetAura("Forever Berserk").Deactivate(sim)
	if mangle.CD.Duration != 6*time.Second {
		t.Fatal("Berserk expiration must restore cooldown")
	}
}

func TestForeverHybridStrictAndBestGuessHealing(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		for _, v := range []struct {
			class string
			id    int32
		}{{"PALADIN", 25292}, {"SHAMAN", 25357}, {"DRUID", 25297}} {
			t.Run(v.class+mode.String(), func(t *testing.T) {
				sim, c := hybridSim(t, v.class, map[string]int32{}, mode)
				heal := c.GetSpell(core.ActionID{SpellID: v.id})
				if heal == nil {
					t.Fatal("baseline healing spell unavailable")
				}
				c.RemoveHealth(sim, 5000)
				before := c.CurrentHealth()
				mana := c.CurrentMana()
				if !heal.Cast(sim, &c.Unit) {
					t.Fatal("healing spell cannot cast")
				}
				for sim.CurrentTime < 5*time.Second {
					if sim.Step() {
						break
					}
				}
				if c.CurrentHealth() <= before || c.CurrentMana() >= mana {
					t.Fatalf("cast did not heal/spend mana: health %v -> %v", before, c.CurrentHealth())
				}
				if mode == proto.ForeverMode_STRICT && v.class == "PALADIN" && c.GetSpell(c.ForeverAction("paladin.baseline.holy-strike")) != nil {
					t.Fatal("prediction enabled in STRICT")
				}
			})
		}
	}
}
func TestForeverStoneclawHealthAndControl(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.earth-s-grasp", 2), proto.ForeverMode_BEST_GUESS)
	c.DistanceFromTarget = 0
	sp := c.GetSpell(core.ActionID{SpellID: 10428})
	if sp == nil || !sp.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Stoneclaw unavailable")
	}
	target := sim.Encounter.TargetUnits[0]
	if target.CurrentTarget == nil || target.CurrentTarget.Type != core.PetUnit || math.Abs(target.CurrentTarget.MaxHealth()-720) > .001 {
		t.Fatal("Stoneclaw must taunt and expose actual 720 HP")
	}
	for sim.CurrentTime < 16*time.Second {
		if sim.Step() {
			break
		}
	}
	if target.CurrentTarget != nil && target.CurrentTarget.Type == core.PetUnit {
		t.Fatal("Stoneclaw did not restore target at expiration")
	}
}
func TestForeverLightningOverloadUsesSeparateSpell(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.lightning-overload", 3), proto.ForeverMode_BEST_GUESS)
	original := c.GetSpell(core.ActionID{SpellID: 10605})
	extra := c.GetSpell(core.ActionID{SpellID: 10605, Tag: 1})
	if original == nil || extra == nil || extra == original {
		t.Fatal("missing independent overload spell")
	}
	for range 100 {
		original.ApplyEffects(sim, sim.Encounter.TargetUnits[0], original)
	}
	if extra.SpellMetrics[sim.Encounter.TargetUnits[0].UnitIndex].TotalDamage <= 0 {
		t.Fatal("Overload never dealt damage")
	}
	if extra.ThreatMultiplier != 0 || original.ThreatMultiplier == 0 {
		t.Fatal("Overload threat modifier leaked")
	}
}
