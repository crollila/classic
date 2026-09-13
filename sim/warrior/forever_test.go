package warrior_test

import (
	"encoding/json"
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	_ "github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
	"math"
	"os"
	"testing"
	"time"
)

func physicalBuild(t *testing.T, id string) map[string]int32 {
	t.Helper()
	raw, e := os.ReadFile("../core/foreverdata/trees.json")
	if e != nil {
		t.Fatal(e)
	}
	var d struct{ Records []foreverdata.Record }
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	selected := map[string]int32{}
	var add func(string, int32)
	add = func(id string, n int32) {
		r, ok := foreverdata.Lookup(id)
		if !ok {
			t.Fatal(id)
		}
		if selected[id] >= n {
			return
		}
		for _, p := range r.Prerequisites {
			add(p.ID, p.Rank)
		}
		for {
			below := int32(0)
			for k, v := range selected {
				q, _ := foreverdata.Lookup(k)
				if q.Class == r.Class && q.Tree == r.Tree && q.Row < r.Row {
					below += v
				}
			}
			if below >= r.RequiredPoints {
				break
			}
			progress := false
			for _, q := range d.Records {
				if q.Class == r.Class && q.Tree == r.Tree && q.Row < r.Row && selected[q.ID] < q.MaxRank {
					add(q.ID, min(q.MaxRank, selected[q.ID]+r.RequiredPoints-below))
					progress = true
					break
				}
			}
			if !progress {
				t.Fatalf("No legal fill for %s", id)
			}
		}
		selected[id] = n
	}
	r, _ := foreverdata.Lookup(id)
	add(id, r.MaxRank)
	return selected
}
func physicalSim(t *testing.T, class, spec string, talents map[string]int32) (*core.Simulation, *core.Character) {
	t.Helper()
	p := &proto.Player{}
	race := "Human"
	if class == "Hunter" {
		race = "Troll"
	}
	s := fmt.Sprintf(`{"class":"Class%s","race":"Race%s","%s":{"options":{}},"bonusStats":{"stats":[100,100,100,100,100,100,100]}}`, class, race, spec)
	if e := protojson.Unmarshal([]byte(s), p); e != nil {
		t.Fatal(e)
	}
	p.Equipment = &proto.EquipmentSpec{}
	p.Rotation = &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
	p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: talents, ExperimentalEstimatedRanks: true}
	req := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 60, MobType: proto.MobType_MobTypeGiant}}, ExecuteProportion_35: .35}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 3, IsTest: true, Interactive: true}}
	sim := core.NewSim(req, simsignals.Signals{})
	sim.Reset()
	return sim, sim.Raid.Parties[0].Players[0].GetCharacter()
}
func physicalSpell(t *testing.T, c *core.Character, id string) *core.Spell {
	t.Helper()
	a := c.ForeverAction(id)
	for _, s := range c.Spellbook {
		if s.ActionID == a {
			return s
		}
	}
	t.Fatalf("missing spell %s", id)
	return nil
}
func TestForeverPhysicalEveryTalentBuilds(t *testing.T) {
	raw, e := os.ReadFile("../core/foreverdata/trees.json")
	if e != nil {
		t.Fatal(e)
	}
	var d struct{ Records []foreverdata.Record }
	json.Unmarshal(raw, &d)
	for _, r := range d.Records {
		class, spec := "", ""
		switch r.Class {
		case "WARRIOR":
			class = "Warrior"
			spec = "warrior"
		case "HUNTER":
			class = "Hunter"
			spec = "hunter"
		case "ROGUE":
			class = "Rogue"
			spec = "rogue"
		default:
			continue
		}
		t.Run(r.ID, func(t *testing.T) { physicalSim(t, class, spec, physicalBuild(t, r.ID)) })
	}
}
func TestForeverPhysicalResourceAndCooldownEffects(t *testing.T) {
	_, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.focused-rage"))
	var slam *core.Spell
	for _, s := range c.Spellbook {
		if s.SpellID == 11605 {
			slam = s
		}
	}
	if slam == nil || slam.DefaultCast.Cost != 12 {
		t.Fatalf("Focused Rage Slam cost: %v", slam)
	}
	_, h := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.sniper-shot"))
	sniper := physicalSpell(t, h, "hunter.talent.sniper-shot")
	if sniper.CD.Duration != 15*time.Second || sniper.DefaultCast.CastTime != 4*time.Second {
		t.Fatalf("Sniper timing %v %v", sniper.CD.Duration, sniper.DefaultCast.CastTime)
	}
	_, r := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.weapon-expertise"))
	if math.Abs(r.GetStat(stats.Expertise)-2) > 1e-8 {
		t.Fatal("Weapon Expertise", r.GetStat(stats.Expertise))
	}
}
func TestForeverVenomAndMutilateAreRealSpells(t *testing.T) {
	sim, r := physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.venom"))
	v := physicalSpell(t, r, "rogue.talent.venom")
	r.AddComboPoints(sim, 5, r.CurrentTarget, v.ComboPointMetrics())
	r.AddEnergy(sim, 100, r.NewEnergyMetrics(v.ActionID))
	var poison *core.Spell
	for _, s := range r.Spellbook {
		if s.SpellID == 11340 {
			poison = s
		}
	}
	if poison == nil {
		t.Fatal("No Instant Poison")
	}
	before := poison.DamageMultiplier
	if !v.Cast(sim, r.CurrentTarget) {
		t.Fatal("Venom failed")
	}
	if math.Abs(poison.DamageMultiplier/before-1.3) > 1e-8 || r.ComboPoints() > 1 {
		t.Fatal("Venom did not affect poison or consume combo points")
	}
	a := r.GetAura("Venom")
	if a.RemainingDuration(sim) != 21*time.Second {
		t.Fatal(a.RemainingDuration(sim))
	}
	a.Deactivate(sim)
	if math.Abs(poison.DamageMultiplier-before) > 1e-8 {
		t.Fatal("Venom failed to reverse")
	}
	_, r = physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.mutilate"))
	m := physicalSpell(t, r, "rogue.talent.mutilate")
	if m.DefaultCast.Cost != 60 || !m.Flags.Matches(core.SpellFlagAPL) {
		t.Fatal("Mutilate resolved to child attack")
	}
}
func TestForeverHawkCastAndCap(t *testing.T) {
	sim, h := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.summon-hawk"))
	s := physicalSpell(t, h, "hunter.talent.summon-hawk")
	if s.DefaultCast.Cost != 80 || !s.Cast(sim, h.CurrentTarget) {
		t.Fatal("Hawk was not a payable cast")
	}
	if !h.GetAura("Hawk 1").IsActive() {
		t.Fatal("Hawk did not activate")
	}
	for i := 0; i < 3; i++ {
		h.GCD.Reset()
		s.CD.Reset()
		h.AddMana(sim, 100, h.NewManaMetrics(s.ActionID))
		if !s.Cast(sim, h.CurrentTarget) {
			t.Fatal("Hawk repeat")
		}
	}
	if !h.GetAura("Hawk 1").IsActive() || !h.GetAura("Hawk 2").IsActive() {
		t.Fatal("Two hawks not active")
	}
	count := 0
	for _, a := range []string{"Hawk 1", "Hawk 2", "Hawk 3"} {
		if h.GetAura(a) != nil {
			count++
		}
	}
	if count != 2 {
		t.Fatal("Hawk cap", count)
	}
}
func TestForeverPhysicalControlCasts(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.mortal-strike"))
	var mortal *core.Spell
	for _, s := range c.Spellbook {
		if s.SpellID == 21553 {
			mortal = s
		}
	}
	if mortal == nil {
		t.Fatal("Mortal Strike missing")
	}
	mortal.BonusHitRating = 100
	for i := 0; i < 10 && !c.CurrentTarget.HasActiveAura("Forever Mortal Strike"); i++ {
		c.AddRage(sim, 100, c.NewRageMetrics(mortal.ActionID))
		c.GCD.Reset()
		mortal.CD.Reset()
		mortal.Cast(sim, c.CurrentTarget)
	}
	if c.CurrentTarget.PseudoStats.HealingTakenMultiplier != .5 {
		t.Fatal("Mortal Strike healing reduction absent")
	}
	sim, c = physicalSim(t, "Rogue", "rogue", physicalBuild(t, "rogue.talent.improved-kidney-shot"))
	var kidney *core.Spell
	for _, s := range c.Spellbook {
		if s.SpellID == 8643 {
			kidney = s
		}
	}
	for _, cp := range []int32{1, 5} {
		c.AddComboPoints(sim, cp, c.CurrentTarget, kidney.ComboPointMetrics())
		c.AddEnergy(sim, 100, c.NewEnergyMetrics(kidney.ActionID))
		c.GCD.Reset()
		kidney.CD.Reset()
		if !kidney.Cast(sim, c.CurrentTarget) {
			t.Fatal("Kidney Shot failed")
		}
		if !c.CurrentTarget.ForeverControlled(core.ForeverStun) {
			t.Fatal("Kidney Shot does not control target")
		}
		a := c.CurrentTarget.GetAura("Kidney Shot")
		if a.RemainingDuration(sim) != time.Duration(cp+1)*time.Second {
			t.Fatal("Kidney Shot duration", a.RemainingDuration(sim))
		}
		a.Deactivate(sim)
	}
}
func TestForeverBloodCrazeTerminalTick(t *testing.T) {
	sim, c := physicalSim(t, "Warrior", "warrior", physicalBuild(t, "warrior.talent.blood-craze"))
	c.RemoveHealth(sim, c.MaxHealth()*.5)
	before := c.CurrentHealth()
	spell := c.Spellbook[0]
	c.OnSpellHitTaken(sim, spell, &core.SpellResult{Target: &c.Unit, Outcome: core.OutcomeCrit, Damage: 1})
	for sim.CurrentTime < 7*time.Second {
		if sim.Step() {
			break
		}
	}
	actual := (c.CurrentHealth() - before) / c.MaxHealth()
	if math.Abs(actual-.03) > 1e-8 {
		t.Fatalf("Blood Craze gained %v want .03", actual)
	}
}
func TestForeverImprovedArcaneShotCooldown(t *testing.T) {
	_, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.improved-arcane-shot"))
	found := false
	for _, s := range c.Spellbook {
		if s.SpellID == 14287 {
			found = true
			if s.CD.Duration != 4500*time.Millisecond {
				t.Fatal("Arcane Shot cooldown", s.CD.Duration)
			}
		}
	}
	if !found {
		t.Fatal("Arcane Shot absent")
	}
}
