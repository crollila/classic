package mage

import (
	"encoding/json"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
	"github.com/wowsims/classic/sim/priest/healing"
	"github.com/wowsims/classic/sim/priest/shadow"
	"github.com/wowsims/classic/sim/warlock/dps"
	"os"
	"sort"
	"testing"
	"time"
)

func init() { healing.RegisterHealingPriest(); shadow.RegisterShadowPriest(); dps.RegisterDpsWarlock() }
func foreverCasterRecords(t *testing.T) []foreverdata.Record {
	t.Helper()
	raw, e := os.ReadFile("../core/foreverdata/trees.json")
	if e != nil {
		t.Fatal(e)
	}
	var d struct{ Records []foreverdata.Record }
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	return d.Records
}
func foreverCasterBuild(t *testing.T, class proto.Class, goals map[string]int32, mechanics ...string) *proto.Player {
	t.Helper()
	p := &proto.Player{Class: class, Race: proto.Race_RaceHuman, Equipment: &proto.EquipmentSpec{}, Rotation: &proto.APLRotation{Type: proto.APLRotation_TypeAPL}, BonusStats: &proto.UnitStats{Stats: make([]float64, 40)}, Forever: &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS, Talents: map[string]int32{}, Mechanics: mechanics}}
	p.BonusStats.Stats[stats.Mana] = 100000
	p.BonusStats.Stats[stats.Health] = 100000
	switch class {
	case proto.Class_ClassMage:
		p.Spec = &proto.Player_Mage{Mage: &proto.Mage{Options: &proto.Mage_Options{}}}
	case proto.Class_ClassWarlock:
		p.Spec = &proto.Player_Warlock{Warlock: &proto.Warlock{Options: &proto.WarlockOptions{Summon: proto.WarlockOptions_Imp}}}
	case proto.Class_ClassPriest:
		p.Spec = &proto.Player_ShadowPriest{ShadowPriest: &proto.ShadowPriest{Options: &proto.ShadowPriest_Options{}}}
	}
	records := foreverCasterRecords(t)
	byID := map[string]foreverdata.Record{}
	for _, r := range records {
		byID[r.ID] = r
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Row < records[j].Row })
	var selectRank func(string, int32)
	selectRank = func(id string, n int32) {
		r := byID[id]
		for _, req := range r.Prerequisites {
			selectRank(req.ID, req.Rank)
		}
		for {
			points := int32(0)
			for id, n := range p.Forever.Talents {
				other := byID[id]
				if other.Tree == r.Tree && other.Row < r.Row {
					points += n
				}
			}
			if points >= r.RequiredPoints {
				break
			}
			found := false
			for _, candidate := range records {
				if candidate.Class != r.Class || candidate.Tree != r.Tree || candidate.Row >= r.Row || candidate.Mode == "blocked" || p.Forever.Talents[candidate.ID] >= candidate.MaxRank {
					continue
				}
				legal := true
				for _, req := range candidate.Prerequisites {
					if p.Forever.Talents[req.ID] < req.Rank {
						legal = false
					}
				}
				lower := int32(0)
				for id, n := range p.Forever.Talents {
					other := byID[id]
					if other.Tree == candidate.Tree && other.Row < candidate.Row {
						lower += n
					}
				}
				if legal && lower >= candidate.RequiredPoints {
					p.Forever.Talents[candidate.ID]++
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("cannot fill legal prerequisites for %s", id)
			}
		}
		p.Forever.Talents[id] = max(p.Forever.Talents[id], n)
	}
	ids := []string{}
	for id := range goals {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		selectRank(id, goals[id])
	}
	if e := foreverdata.Validate(p); e != nil {
		t.Fatalf("legal build: %v", e)
	}
	return p
}
func foreverCasterSim(t *testing.T, p *proto.Player) (*core.Simulation, *core.Character) {
	t.Helper()
	request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: 60, MobType: proto.MobType_MobTypeHumanoid}, {Level: 60, MobType: proto.MobType_MobTypeHumanoid}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 123, IsTest: true, Interactive: true}}
	s := core.NewSim(request, simsignals.Signals{})
	s.Reset()
	return s, s.Raid.Parties[0].Players[0].GetCharacter()
}
func foreverAdvance(s *core.Simulation, until time.Duration) {
	for s.CurrentTime < until {
		if s.Step() {
			break
		}
	}
}
func TestForeverCasterEveryTalentConstructsAndCasts(t *testing.T) {
	for _, r := range foreverCasterRecords(t) {
		if r.Class != "MAGE" && r.Class != "WARLOCK" && r.Class != "PRIEST" {
			continue
		}
		if r.Mode == "blocked" {
			continue
		}
		r := r
		t.Run(r.ID, func(t *testing.T) {
			class := proto.Class_ClassMage
			if r.Class == "WARLOCK" {
				class = proto.Class_ClassWarlock
			}
			if r.Class == "PRIEST" {
				class = proto.Class_ClassPriest
			}
			p := foreverCasterBuild(t, class, map[string]int32{r.ID: r.MaxRank})
			sim, c := foreverCasterSim(t, p)
			// Exercise each new family, including triggered shields/controls/heals. Every
			// spell gets a fresh reset so cooldowns/stance/resource state cannot hide it.
			for _, s := range c.Spellbook {
				if s.OtherID != proto.OtherAction_OtherActionForever || !s.Flags.Matches(core.SpellFlagAPL) {
					continue
				}
				sim, c = foreverCasterSim(t, p)
				s = c.GetSpell(s.ActionID)
				target := sim.Encounter.TargetUnits[0]
				if s.Flags.Matches(core.SpellFlagHelpful) {
					target = &c.Unit
					c.RemoveHealth(sim, c.MaxHealth()/2)
				}
				if s.CanCast(sim, target) {
					if !s.Cast(sim, target) {
						t.Fatalf("eligible action failed: %s", s.ActionID)
					}
					foreverAdvance(sim, 12*time.Second)
				}
			}
		})
	}
}
func TestForeverArcaneBlastStacksAndConsumes(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{"mage.talent.arcane-blast": 1})
	sim, c := foreverCasterSim(t, p)
	ab := c.GetSpell(c.ForeverAction("mage.talent.arcane-blast"))
	fire := c.GetSpell(core.ActionID{SpellID: 10151})
	before := fire.DamageMultiplierAdditive
	if !ab.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("AB cast")
	}
	foreverAdvance(sim, 3*time.Second)
	if ab.Cost.Multiplier != 275 {
		t.Fatalf("stacked AB cost %d", ab.Cost.Multiplier)
	}
	if fire.DamageMultiplierAdditive != before+.1 {
		t.Fatal("AB other-spell damage bonus")
	}
	if !fire.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Fireball cast")
	}
	foreverAdvance(sim, 7*time.Second)
	if ab.Cost.Multiplier != 100 || fire.DamageMultiplierAdditive != before {
		t.Fatal("AB stacks did not consume")
	}
}
func TestForeverPenanceHealingAndShieldAbsorption(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.penance": 1})
	sim, c := foreverCasterSim(t, p)
	target := &c.Unit
	c.RemoveHealth(sim, 10000)
	before := c.CurrentHealth()
	action := c.ForeverAction("priest.talent.penance")
	spell := c.GetSpell(action.WithTag(-action.Tag))
	if !spell.Cast(sim, target) {
		t.Fatal("Penance heal cast")
	}
	foreverAdvance(sim, 3*time.Second)
	if c.CurrentHealth() <= before+250 {
		t.Fatalf("Penance failed to heal three bolts: %f", c.CurrentHealth()-before)
	}
	shield := c.GetSpell(core.ActionID{SpellID: 10901})
	if !shield.Cast(sim, target) {
		t.Fatal("shield cast")
	}
	r := &core.SpellResult{Target: target, Damage: 200, Outcome: core.OutcomeHit}
	enemy := sim.Encounter.TargetUnits[0]
	incoming := &core.Spell{Unit: enemy, SpellSchool: core.SpellSchoolPhysical}
	for _, mod := range c.DynamicDamageTakenModifiers {
		mod(sim, incoming, r)
	}
	if r.Damage != 0 {
		t.Fatalf("shield failed actual absorption: %f", r.Damage)
	}
}
func TestForeverBaneOfHavocRedirect(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{"warlock.talent.bane-of-havoc": 1})
	sim, c := foreverCasterSim(t, p)
	havoc := c.GetSpell(c.ForeverAction("warlock.talent.bane-of-havoc"))
	marked := sim.Encounter.TargetUnits[1]
	if !havoc.Cast(sim, marked) {
		t.Fatal("Havoc cast")
	}
	foreverAdvance(sim, 2*time.Second)
	sb := c.GetSpell(core.ActionID{SpellID: 11661})
	if !sb.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Shadow Bolt cast")
	}
	foreverAdvance(sim, 7*time.Second)
	copy := c.GetSpell(havoc.ActionID.WithTag(-havoc.Tag))
	if copy.SpellMetrics[marked.UnitIndex].TotalDamage <= 0 {
		t.Fatal("Havoc did not redirect actual damage")
	}
}

func TestForeverCasterStrictPredictionPolicy(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{}, "mage.baseline.frostfire-bolt")
	_, best := foreverCasterSim(t, p)
	action := best.ForeverAction("mage.baseline.frostfire-bolt")
	if best.GetSpell(action) == nil {
		t.Fatal("BEST_GUESS missing predicted Frostfire Bolt")
	}
	p.Forever.Mode = proto.ForeverMode_STRICT
	_, strict := foreverCasterSim(t, p)
	if strict.GetSpell(action) != nil {
		t.Fatal("STRICT enabled predicted Frostfire Bolt")
	}
}

func TestForeverSoulShardInventoryAndStrictPreparation(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassWarlock, map[string]int32{})
	p.Forever.Parameters = map[string]float64{"warlock.soul_shards": 0}
	sim, c := foreverCasterSim(t, p)
	soulFire := c.GetSpell(core.ActionID{SpellID: 17924})
	if soulFire.CanCast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Soul Fire ignored empty shard inventory")
	}
	p.Forever.Parameters["warlock.soul_shards"] = 1
	sim, c = foreverCasterSim(t, p)
	soulFire = c.GetSpell(core.ActionID{SpellID: 17924})
	if !soulFire.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Soul Fire rejected available shard")
	}
	foreverAdvance(sim, 7*time.Second)
	if c.GetAura("Forever Soul Shards").GetStacks() != 0 {
		t.Fatal("Soul Fire did not consume its shard")
	}
	p.Forever.Mode = proto.ForeverMode_STRICT
	p.Forever.Parameters["warlock.soul_shards"] = 0
	sim, c = foreverCasterSim(t, p)
	soulFire = c.GetSpell(core.ActionID{SpellID: 17924})
	if !soulFire.CanCast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("STRICT lost Classic sufficient-shard policy")
	}
}

func TestForeverPenanceSharesCooldown(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.penance": 1})
	sim, c := foreverCasterSim(t, p)
	action := c.ForeverAction("priest.talent.penance")
	heal := c.GetSpell(action.WithTag(-action.Tag))
	damage := c.GetSpell(action)
	if !heal.Cast(sim, &c.Unit) {
		t.Fatal("Penance heal cast")
	}
	foreverAdvance(sim, 3*time.Second)
	if damage.CanCast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Penance variants have separate cooldowns")
	}
}

func TestForeverClassicIdentityUtilityCasts(t *testing.T) {
	cases := []struct {
		class  proto.Class
		spell  int32
		goal   string
		summon proto.WarlockOptions_Summon
	}{
		{proto.Class_ClassMage, 10230, "", 0}, {proto.Class_ClassMage, 10161, "", 0}, {proto.Class_ClassMage, 10223, "", 0}, {proto.Class_ClassMage, 10225, "", 0}, {proto.Class_ClassMage, 10193, "", 0},
		{proto.Class_ClassPriest, 10965, "", 0}, {proto.Class_ClassPriest, 10929, "", 0}, {proto.Class_ClassPriest, 10961, "", 0}, {proto.Class_ClassPriest, 10890, "", 0}, {proto.Class_ClassPriest, 10942, "", 0}, {proto.Class_ClassPriest, 10952, "", 0},
		{proto.Class_ClassWarlock, 11775, "warlock.talent.improved-voidwalker", proto.WarlockOptions_Voidwalker}, {proto.Class_ClassWarlock, 17752, "warlock.talent.improved-voidwalker", proto.WarlockOptions_Voidwalker}, {proto.Class_ClassWarlock, 17854, "warlock.talent.improved-voidwalker", proto.WarlockOptions_Voidwalker}, {proto.Class_ClassWarlock, 19443, "warlock.talent.improved-voidwalker", proto.WarlockOptions_Voidwalker},
		{proto.Class_ClassWarlock, 6358, "warlock.talent.improved-sayaad", proto.WarlockOptions_Succubus}, {proto.Class_ClassWarlock, 7870, "warlock.talent.improved-sayaad", proto.WarlockOptions_Succubus}, {proto.Class_ClassWarlock, 11785, "warlock.talent.improved-sayaad", proto.WarlockOptions_Succubus},
		{proto.Class_ClassWarlock, 19647, "warlock.talent.improved-felhunter", proto.WarlockOptions_Felhunter}, {proto.Class_ClassWarlock, 19736, "warlock.talent.improved-felhunter", proto.WarlockOptions_Felhunter}, {proto.Class_ClassWarlock, 11684, "warlock.talent.pyroclasm", proto.WarlockOptions_Imp},
	}
	for _, tc := range cases {
		goals := map[string]int32{}
		if tc.goal != "" {
			goals[tc.goal] = 1
		}
		p := foreverCasterBuild(t, tc.class, goals)
		if tc.class == proto.Class_ClassWarlock {
			p.GetWarlock().Options.Summon = tc.summon
		}
		sim, c := foreverCasterSim(t, p)
		s := c.GetSpell(core.ActionID{SpellID: tc.spell})
		if s == nil {
			t.Fatalf("utility %d not registered", tc.spell)
		}
		target := sim.Encounter.TargetUnits[0]
		if s.Flags.Matches(core.SpellFlagHelpful) {
			target = &c.Unit
		}
		if !s.Cast(sim, target) {
			t.Fatalf("utility %d did not cast", tc.spell)
		}
		foreverAdvance(sim, 12*time.Second)
	}
}

func TestForeverFiniteEnemyHealthKillEffects(t *testing.T) {
	for _, tc := range []struct {
		class  proto.Class
		talent string
		rank   int32
		spell  int32
		aura   string
	}{
		{proto.Class_ClassMage, "mage.talent.wake-of-fire", 1, 10151, "Forever Wake of Fire"},
		{proto.Class_ClassPriest, "priest.talent.spirit-tap", 5, 10947, "Spirit Tap"},
		{proto.Class_ClassWarlock, "warlock.talent.soul-harvesting", 1, 11675, "Forever Soul Harvest"},
	} {
		t.Run(tc.talent, func(t *testing.T) {
			p := foreverCasterBuild(t, tc.class, map[string]int32{tc.talent: tc.rank})
			p.Forever.Parameters = map[string]float64{"scenario.enemy_health.0": 1, "scenario.enemy_health.1": 100000}
			if tc.class == proto.Class_ClassWarlock {
				p.GetWarlock().Options.Summon = proto.WarlockOptions_NoSummon
			}
			sim, c := foreverCasterSim(t, p)
			target := sim.Encounter.TargetUnits[0]
			s := c.GetSpell(core.ActionID{SpellID: tc.spell})
			if !s.Cast(sim, target) {
				t.Fatal("killing cast failed")
			}
			foreverAdvance(sim, 4*time.Second)
			if !target.ForeverEnemyDead() {
				t.Fatalf("finite target did not die: hp=%f", target.CurrentHealth())
			}
			buff := c.GetAura(tc.aura)
			if buff == nil || !buff.IsActive() {
				t.Fatalf("kill did not activate %s", tc.aura)
			}
			if c.CurrentTarget != sim.Encounter.TargetUnits[1] {
				t.Fatal("caster did not switch to surviving target")
			}
			if s.CanCast(sim, target) || s.Cast(sim, target) {
				t.Fatal("cast allowed a dead target")
			}
			before, expires := s.SpellMetrics[target.UnitIndex].TotalDamage, buff.ExpiresAt()
			s.CalcAndDealDamage(sim, target, 100, s.OutcomeAlwaysHit)
			if s.SpellMetrics[target.UnitIndex].TotalDamage != before || buff.ExpiresAt() != expires {
				t.Fatal("corpse generated damage or another kill proc")
			}
		})
	}
}

func TestForeverContagionSpreadsOnOtherDamageKill(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassPriest, map[string]int32{"priest.talent.devouring-contagion": 1}, "priest.baseline.devouring-plague")
	p.Forever.Parameters = map[string]float64{"scenario.enemy_health.0": 100, "scenario.enemy_health.1": 100000}
	sim, c := foreverCasterSim(t, p)
	target, next := sim.Encounter.TargetUnits[0], sim.Encounter.TargetUnits[1]
	plague := c.GetSpell(core.ActionID{SpellID: 19279})
	if !plague.Cast(sim, target) {
		t.Fatal("plague cast")
	}
	if !plague.Dot(target).IsActive() {
		t.Fatal("plague missed in deterministic fixture")
	}
	// A distinct spell kills the target before the first plague tick.
	bolt := c.GetSpell(core.ActionID{SpellID: 10947})
	bolt.CalcAndDealDamage(sim, target, 1000, bolt.OutcomeAlwaysHit)
	if !target.ForeverEnemyDead() || !plague.Dot(next).IsActive() {
		t.Fatal("Contagion did not spread on a non-plague killing hit")
	}
	if plague.Dot(next).NumberOfTicks != plague.Dot(target).NumberOfTicks {
		t.Fatal("spread lost remaining duration")
	}
}

func TestForeverFiniteEnemyHealthRequiresScenario(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{})
	_, c := foreverCasterSim(t, p)
	if c.Env.Encounter.TargetUnits[0].HasHealthBar() {
		t.Fatal("default duration encounter gained finite target health")
	}
}

func TestForeverHealthEncounterEndsAfterLastFiniteTarget(t *testing.T) {
	p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{})
	targetStats := make([]float64, 40)
	targetStats[stats.Health] = 2
	r := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{UseHealth: true, Targets: []*proto.Target{{Level: 60, Stats: targetStats}, {Level: 60, Stats: targetStats}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: 123, IsTest: true, Interactive: true}}
	sim := core.NewSim(r, simsignals.Signals{})
	sim.Reset()
	c := sim.Raid.Parties[0].Players[0].GetCharacter()
	spell := c.GetSpell(core.ActionID{SpellID: 10151})
	spell.CalcAndDealDamage(sim, sim.Encounter.TargetUnits[0], 1000, spell.OutcomeAlwaysHit)
	if sim.Encounter.DamageTaken != 2 {
		t.Fatal("overkill advanced aggregate health past a surviving target")
	}
	spell.CalcAndDealDamage(sim, sim.Encounter.TargetUnits[1], 1000, spell.OutcomeAlwaysHit)
	if !sim.Step() {
		t.Fatal("health encounter did not terminate at exact finite health total")
	}
}
