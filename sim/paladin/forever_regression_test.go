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
	c.SpendMana(sim, 2000, c.NewManaMetrics(vigil.ActionID))
	shock.ApplyEffects(sim, target, shock)
	if math.Abs(c.CurrentMana()-(mana-2000+1005)) > .001 {
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
				// Holy Strike is a trainer spell with client-read ranks now, not a prediction.
				if v.class == "PALADIN" && c.GetSpell(c.ForeverAction("paladin.baseline.holy-strike")) == nil {
					t.Fatal("Holy Strike missing")
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

func TestForeverHybridFactoryPlanningRotations(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		for _, spec := range []string{"HOLY", "RESTO_SHAMAN", "RESTO_DRUID", "BEAR"} {
			t.Run(spec+mode.String(), func(t *testing.T) {
				p := &proto.Player{Equipment: &proto.EquipmentSpec{}, Consumes: &proto.Consumes{}, DistanceFromTarget: 0, BonusStats: &proto.UnitStats{Stats: stats.Stats{stats.Health: 10000, stats.Mana: 20000, stats.AttackPower: 1000, stats.SpellPower: 200, stats.SpellCrit: 100 * core.SpellCritRatingPerCritChance, stats.MeleeHit: 20, stats.SpellHit: 20}.ToFloatArray()}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: mode, ExperimentalEstimatedRanks: true}}
				ids := []int32{}
				switch spec {
				case "HOLY":
					p.Class = proto.Class_ClassPaladin
					p.Race = proto.Race_RaceHuman
					p.Spec = &proto.Player_HolyPaladin{HolyPaladin: &proto.HolyPaladin{Options: &proto.PaladinOptions{}}}
					ids = []int32{25292, 19943}
				case "RESTO_SHAMAN":
					p.Class = proto.Class_ClassShaman
					p.Race = proto.Race_RaceTroll
					p.Spec = &proto.Player_RestorationShaman{RestorationShaman: &proto.RestorationShaman{Options: &proto.RestorationShaman_Options{}}}
					ids = []int32{25357, 10468}
				case "RESTO_DRUID":
					p.Class = proto.Class_ClassDruid
					p.Race = proto.Race_RaceNightElf
					p.Spec = &proto.Player_RestorationDruid{RestorationDruid: &proto.RestorationDruid{Options: &proto.RestorationDruid_Options{}}}
					ids = []int32{25297}
				case "BEAR":
					p.Class = proto.Class_ClassDruid
					p.Race = proto.Race_RaceNightElf
					p.Spec = &proto.Player_FeralTankDruid{FeralTankDruid: &proto.FeralTankDruid{Options: &proto.FeralTankDruid_Options{}}}
					ids = []int32{5229, 9908, 9881}
				}
				p.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
				for _, id := range ids {
					target := &proto.UnitReference{Type: proto.UnitReference_Self}
					if spec == "BEAR" {
						target = nil
					}
					p.Rotation.PriorityList = append(p.Rotation.PriorityList, &proto.APLListItem{Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{SpellId: &proto.ActionID{RawId: &proto.ActionID_SpellId{SpellId: id}}, Target: target}}}})
				}
				result := core.RunSim(&proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}, Tanks: []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}}, Encounter: &proto.Encounter{Duration: 30, Targets: []*proto.Target{{Level: 63, MinBaseDamage: 400, SwingSpeed: 2, TankIndex: 0, Stats: stats.Stats{stats.Health: 1000000}.ToFloatArray()}}}, SimOptions: &proto.SimOptions{Iterations: 2, RandomSeed: 5, IsTest: true}}, nil, simsignals.Signals{})
				if result.Error != nil {
					t.Fatal(result.Error)
				}
				output := result.RaidMetrics.Hps.Avg
				if spec == "BEAR" {
					output = result.RaidMetrics.Dps.Avg
				}
				if output <= 0 || math.IsNaN(output) || math.IsInf(output, 0) {
					t.Fatal("default rotation produced no finite output", output)
				}
				t.Logf("%s %s output=%.2f", spec, mode.String(), output)
			})
		}
	}
}

func TestForeverReincarnationUsesObservedRestoration(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.improved-reincarnation", 2), proto.ForeverMode_BEST_GUESS)
	sp := c.GetSpell(core.ActionID{SpellID: 20608})
	if sp == nil || sp.CD.Duration != 40*time.Minute {
		t.Fatal("missing reduced Reincarnation cooldown")
	}
	c.RemoveHealth(sim, c.MaxHealth())
	if !sp.Cast(sim, &c.Unit) {
		t.Fatal("Reincarnation unavailable at zero health")
	}
	if math.Abs(c.CurrentHealthPercent()-.4) > .0001 || math.Abs(c.CurrentManaPercent()-.4) > .0001 {
		t.Fatal("Reincarnation did not restore40%health andMana")
	}
}
func TestForeverLaceratePredictionModeAndRage(t *testing.T) {
	for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
		sim, c := hybridSim(t, "DRUID", hybridTalents(t, "druid.talent.shredding-attacks", 3), mode)
		id := c.ForeverAction("druid.talent.shredding-attacks")
		id.Tag = -id.Tag
		sp := c.GetSpell(id)
		if mode == proto.ForeverMode_STRICT {
			if sp != nil {
				t.Fatal("Lacerate prediction active in STRICT")
			}
			continue
		}
		if sp == nil || sp.Cost.GetCurrentCost() != 12 {
			t.Fatal("Lacerate Rage reduction missing")
		}
		bear := c.GetSpell(core.ActionID{SpellID: 9634})
		bear.ApplyEffects(sim, &c.Unit, bear)
		for range 5 {
			sp.ApplyEffects(sim, sim.Encounter.TargetUnits[0], sp)
		}
		dot := sp.Dot(sim.Encounter.TargetUnits[0])
		if dot.GetStacks() != 5 || dot.SnapshotBaseDamage != 155 {
			t.Fatal("Lacerate stacking baseline failed", dot.GetStacks(), dot.SnapshotBaseDamage)
		}
	}
}

func TestForeverStoneskinReplacementAndVoiceImmunity(t *testing.T) {
	sim, c := hybridSim(t, "SHAMAN", hybridTalents(t, "shaman.talent.guardian-totems", 2), proto.ForeverMode_BEST_GUESS)
	skin := c.GetSpell(core.ActionID{SpellID: 10408})
	skin.ApplyEffects(sim, &c.Unit, skin)
	incoming := &core.Spell{SpellSchool: core.SpellSchoolPhysical, DefenseType: core.DefenseTypeMelee, ProcMask: core.ProcMaskMeleeMHAuto}
	damage := func() float64 {
		r := &core.SpellResult{Target: &c.Unit, Damage: 100}
		for _, f := range c.DynamicDamageTakenModifiers {
			f(sim, incoming, r)
		}
		return r.Damage
	}
	if math.Abs(damage()-64) > .001 {
		t.Fatal("Stoneskin did not apply36 reduction", damage())
	}
	strength := c.GetSpell(core.ActionID{SpellID: 25361})
	strength.ApplyEffects(sim, &c.Unit, strength)
	if damage() != 100 {
		t.Fatal("replaced Stoneskin still reduces damage", damage())
	}
	sim, c = hybridSim(t, "PALADIN", hybridTalents(t, "paladin.talent.voice-of-truth", 1), proto.ForeverMode_BEST_GUESS)
	voice := c.GetSpell(c.ForeverAction("paladin.talent.voice-of-truth"))
	voice.ApplyEffects(sim, &c.Unit, voice)
	heal := c.GetSpell(core.ActionID{SpellID: 25292})
	if !heal.Cast(sim, &c.Unit) {
		t.Fatal("healing cast failed")
	}
	ends := c.Hardcast.Expires
	c.ForeverInterrupt(sim)
	if c.Hardcast.Expires != ends {
		t.Fatal("Voice of Truth did not prevent interruption")
	}
}

// Exercise the actual critical healing branch, not just low-crit construction.
// Use legal 51-point builds and both mode filters; every registered hybrid heal
// is invoked, including triggered Healing Stream and periodic/channel spells.
func TestForeverHybridEveryHealingSpellForcedCrit(t *testing.T) {
	raw, err := os.ReadFile("../core/foreverdata/trees.json")
	if err != nil {
		t.Fatal(err)
	}
	var data struct{ Records []foreverdata.Record }
	if err = json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	sort.SliceStable(data.Records, func(i, j int) bool { return data.Records[i].Row < data.Records[j].Row })
	for _, v := range []struct{ class, goal string }{{"PALADIN", "paladin.talent.light-s-vigil"}, {"SHAMAN", "shaman.talent.riptide"}, {"DRUID", "druid.talent.wild-growth"}, {"DRUID", "druid.talent.swiftmend"}} {
		for _, mode := range []proto.ForeverMode{proto.ForeverMode_STRICT, proto.ForeverMode_BEST_GUESS} {
			t.Run(v.goal+mode.String(), func(t *testing.T) {
				talents := hybridTalents(t, v.goal, 1)
				goal, _ := foreverdata.Lookup(v.goal)
				total := int32(0)
				for _, n := range talents {
					total += n
				}
				for _, r := range data.Records {
					if r.Class != v.class || r.Tree != goal.Tree {
						continue
					}
					for talents[r.ID] < r.MaxRank && total < 51 {
						below := int32(0)
						for id, n := range talents {
							other, _ := foreverdata.Lookup(id)
							if other.Row < r.Row {
								below += n
							}
						}
						legal := below >= r.RequiredPoints
						for _, pre := range r.Prerequisites {
							legal = legal && talents[pre.ID] >= pre.Rank
						}
						if !legal {
							break
						}
						talents[r.ID]++
						total++
					}
				}
				for _, r := range data.Records {
					if r.Class == v.class && r.RequiredPoints == 0 && len(r.Prerequisites) == 0 {
						for total < 51 && talents[r.ID] < r.MaxRank {
							talents[r.ID]++
							total++
						}
					}
				}
				if total != 51 {
					t.Fatal("test build must use51 points", total)
				}
				sim, c := hybridSim(t, v.class, talents, mode)
				c.AddStatDynamic(sim, stats.SpellCrit, 100*core.SpellCritRatingPerCritChance)
				c.RemoveHealth(sim, c.MaxHealth()*.9)
				if v.class == "PALADIN" {
					if vigil := c.GetSpell(c.ForeverAction("paladin.talent.light-s-vigil")); vigil != nil {
						vigil.ApplyEffects(sim, &c.Unit, vigil)
					}
				}
				tested := 0
				for _, sp := range c.Spellbook {
					if !sp.ProcMask.Matches(core.ProcMaskSpellHealing) || sp.ApplyEffects == nil {
						continue
					}
					if sp.DefenseType != core.DefenseTypeMagic {
						t.Fatalf("healing spell %s lacks Magic defense type", sp.ActionID)
					}
					if sp.SpellID == 18562 {
						rejuv := c.GetSpell(core.ActionID{SpellID: 25299})
						rejuv.ApplyEffects(sim, &c.Unit, rejuv)
					}
					before := sp.SpellMetrics[c.UnitIndex].Crits
					sp.ApplyEffects(sim, &c.Unit, sp)
					switch sp.SpellID {
					case 25292, 19943, 25357, 10468, 10623, 25297, 9858, 18562:
						if sp.SpellMetrics[c.UnitIndex].Crits <= before {
							t.Fatalf("direct heal %s did not execute critical outcome", sp.ActionID)
						}
					}
					if sp.OtherID == proto.OtherAction_OtherActionForever && (sp.ActionID == c.ForeverAction(v.goal) || v.class == "PALADIN") && sp.ActionID != c.ForeverAction("druid.talent.wild-growth") && sp.SpellMetrics[c.UnitIndex].Crits <= before {
						t.Fatalf("talented heal %s did not crit", sp.ActionID)
					}
					tested++
				}
				for sim.CurrentTime < 11*time.Second {
					if sim.Step() {
						break
					}
				}
				if tested < 3 || c.CurrentHealth() <= c.MaxHealth()*.1 {
					t.Fatal("healing paths did not execute", tested)
				}
			})
		}
	}
}
