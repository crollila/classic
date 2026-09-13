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
func physicalSim(t *testing.T, class, spec string, talents map[string]int32, edits ...func(*proto.Player)) (*core.Simulation, *core.Character) {
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
	for _, edit := range edits {
		edit(p)
	}
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

func physicalID(t *testing.T, c *core.Character, id int32) *core.Spell {
	t.Helper()
	for _, s := range c.Spellbook {
		if s.SpellID == id {
			return s
		}
	}
	t.Fatal("missing", id)
	return nil
}
func physicalAdvance(t *testing.T, sim *core.Simulation, to time.Duration) {
	t.Helper()
	to += time.Nanosecond
	core.StartDelayedAction(sim, core.DelayedActionOptions{DoAt: to, OnAction: func(*core.Simulation) {}})
	for sim.CurrentTime < to {
		if sim.Step() {
			t.Fatal("ended early")
		}
	}
}
func TestForeverHunterStingSecondaryEffects(t *testing.T) {
	sim, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.improved-stings"), func(p *proto.Player) {
		p.DistanceFromTarget = 10
		p.Forever.Parameters = map[string]float64{"scenario.enemy_mana": 2000}
	})
	target := c.CurrentTarget
	scorpid := physicalID(t, c, 14277)
	scorpid.BonusHitRating = 100
	before := target.GetStat(stats.Strength)
	if !scorpid.Cast(sim, target) {
		t.Fatalf("Scorpid failed range=%v mana=%v cost=%v gcd=%v time=%v condition=%v", c.DistanceFromTarget, c.CurrentMana(), scorpid.DefaultCast.Cost, c.GCD.ReadyAt(), sim.CurrentTime, scorpid.ExtraCastCondition(sim, target))
	}
	aura := target.GetAura("Scorpid Sting-" + c.Label)
	if !aura.IsActive() || aura.Duration != 65*time.Second || target.GetStat(stats.Strength) != before-68 {
		t.Fatal("Scorpid duration/stats", aura.IsActive(), aura.Duration, target.GetStat(stats.Strength))
	}
	viper := physicalID(t, c, 14280)
	viper.BonusHitRating = 100
	c.GCD.Reset()
	if viper.CD.Duration != 9*time.Second || !viper.Cast(sim, target) {
		t.Fatal("Viper cooldown/cast", viper.CD.Duration)
	}
	if aura.IsActive() || target.GetStat(stats.Strength) != before {
		t.Fatal("Stings not exclusive")
	}
	physicalAdvance(t, sim, 8*time.Second)
	if math.Abs(target.CurrentMana()-892) > 1e-8 {
		t.Fatal("Viper drain", target.CurrentMana())
	}
	sim.Cleanup()
	sim.Reset()
	if target.CurrentMana() != 2000 {
		t.Fatal("enemy mana not reset", target.CurrentMana())
	}
	_, strict := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.improved-stings"), func(p *proto.Player) { p.Forever.Mode = proto.ForeverMode_STRICT })
	for _, s := range strict.Spellbook {
		if s.SpellID == 14280 {
			t.Fatal("predicted Viper leaked into STRICT")
		}
	}
}
func TestForeverHunterTrapDurationAndBreak(t *testing.T) {
	sim, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.clever-traps"))
	s := physicalID(t, c, 14311)
	s.BonusHitRating = 100
	if !s.Cast(sim, c.CurrentTarget) {
		t.Fatal("trap failed")
	}
	a := c.CurrentTarget.GetAura("Freezing Trap")
	if a == nil || !a.IsActive() || a.Duration != 26*time.Second {
		t.Fatal("trap duration", a)
	}
	c.CurrentTarget.OnPeriodicDamageTaken(sim, s, &core.SpellResult{Target: c.CurrentTarget, Damage: 1, Outcome: core.OutcomeHit})
	if a.IsActive() {
		t.Fatal("damage did not break freezing")
	}
}
func TestForeverHunterMendPetChannel(t *testing.T) {
	sim, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.improved-mend-pet"), func(p *proto.Player) {
		p.GetHunter().Options.PetType = proto.Hunter_Options_Cat
		p.GetHunter().Options.PetUptime = 1
	})
	pet := c.PetAgents[0].GetPet()
	pet.RemoveHealth(sim, pet.MaxHealth()*.8)
	before := pet.CurrentHealth()
	s := physicalID(t, c, 13544)
	if !s.Cast(sim, c.CurrentTarget) || !c.IsChanneling(sim) {
		t.Fatal("Mend Pet not channeling")
	}
	physicalAdvance(t, sim, 2*time.Second)
	if pet.CurrentHealth()-before != 490 {
		t.Fatal("Mend Pet does not heal each second", pet.CurrentHealth()-before)
	}
	physicalAdvance(t, sim, 5*time.Second)
	if pet.CurrentHealth()-before != 1225 || c.IsChanneling(sim) {
		t.Fatal("Mend Pet total", pet.CurrentHealth()-before)
	}
}
func TestForeverHunterFeignDeathThreatReset(t *testing.T) {
	sim, c := physicalSim(t, "Hunter", "hunter", physicalBuild(t, "hunter.talent.survival-tactics"))
	c.Spellbook[0].SpellMetrics[c.CurrentTarget.UnitIndex].TotalThreat = 1000
	s := physicalID(t, c, 5384)
	if s.BonusHitRating != 10 {
		t.Fatal("Survival Tactics absent on Feign Death")
	}
	s.BonusHitRating = 100
	if !s.Cast(sim, c.CurrentTarget) {
		t.Fatal("Feign failed")
	}
	total := 0.
	for _, spell := range c.Spellbook {
		total += spell.SpellMetrics[c.CurrentTarget.UnitIndex].TotalThreat
	}
	if math.Abs(total) > 1e-8 {
		t.Fatal("Feign did not reset threat", total)
	}
}

func TestForeverMutilateConsumesColdBloodAfterBothWeapons(t *testing.T) {
	build := physicalBuild(t, "rogue.talent.mutilate")
	build["rogue.talent.cold-blood"] = 1
	sim, c := physicalSim(t, "Rogue", "rogue", build, func(p *proto.Player) {
		p.Equipment = core.GetGearSet("../../ui/rogue/gear_sets", "combat_backstab_prebis").GearSet
	})
	physicalID(t, c, 14177).Cast(sim, c.CurrentTarget)
	parent := physicalSpell(t, c, "rogue.talent.mutilate")
	for _, s := range c.Spellbook {
		if s.OtherID == proto.OtherAction_OtherActionForever && s.Tag < 0 {
			s.BonusHitRating = 100
		}
	}
	if !parent.Cast(sim, c.CurrentTarget) {
		t.Fatal("Mutilate failed")
	}
	crits := int32(0)
	for _, s := range c.Spellbook {
		if s.OtherID == proto.OtherAction_OtherActionForever && s.Tag < 0 {
			crits += s.SpellMetrics[c.CurrentTarget.UnitIndex].Crits
		}
	}
	if crits != 2 {
		t.Fatal("Cold Blood did not affect both Mutilate weapons", crits)
	}
	if c.GetAura("Cold Blood").IsActive() {
		t.Fatal("Cold Blood not consumed")
	}
}
