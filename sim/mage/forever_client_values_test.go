package mage

import (
	"math"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/core/stats"
)

// Forever spell and talent values against the beta client tooltips (talentsforever.com
// export of client 1.60.1.69876).

// foreverValueSim builds a legal Forever build at the given level with no crit and certain
// spell hit, so one cast's damage is its base damage times talent multipliers.
func foreverValueSim(t *testing.T, class proto.Class, level int32, goals map[string]int32, mechanics ...string) (*core.Simulation, *core.Character) {
	t.Helper()
	p := foreverCasterBuild(t, class, goals, mechanics...)
	p.Level = level
	p.BonusStats.Stats[stats.SpellCrit] = 0
	p.BonusStats.Stats[stats.SpellHit] = 100 * core.SpellHitRatingPerHitChance
	return foreverCasterSim(t, p)
}

func castDamage(t *testing.T, sim *core.Simulation, s *core.Spell, target *core.Unit, wait time.Duration) float64 {
	t.Helper()
	if s == nil {
		t.Fatal("spell not registered")
	}
	if !s.Cast(sim, target) {
		t.Fatalf("%s did not cast", s.ActionID)
	}
	foreverAdvance(sim, sim.CurrentTime+wait)
	return s.SpellMetrics[target.UnitIndex].TotalDamage
}

// between checks a damage total against a base range, allowing talent multipliers from
// the prerequisite points up to 1.3x (old values sit far outside it).
func between(t *testing.T, name string, got, low, high float64) {
	t.Helper()
	if got < low*.95 || got > high*1.3 {
		t.Errorf("%s: damage %.1f outside client base %.0f-%.0f", name, got, low, high)
	}
}

func TestForeverMageRankedTalentSpells(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassMage, 60, map[string]int32{"mage.talent.arcane-blast": 1, "mage.talent.ice-lance": 1})
	ab := c.GetSpell(c.ForeverAction("mage.talent.arcane-blast"))
	if want := .15 * c.BaseMana; math.Abs(ab.Cost.BaseCost-want) > 1e-6 {
		t.Errorf("Arcane Blast costs 15%% of base mana (%.1f), got %.1f", want, ab.Cost.BaseCost)
	}
	between(t, "Arcane Blast rank 5", castDamage(t, sim, ab, sim.Encounter.TargetUnits[0], 3*time.Second), 364, 424)

	sim, c = foreverValueSim(t, proto.Class_ClassMage, 60, map[string]int32{"mage.talent.ice-lance": 1})
	il := c.GetSpell(c.ForeverAction("mage.talent.ice-lance"))
	if il.Cost.BaseCost != 160 {
		t.Errorf("Ice Lance rank 6 costs 160, got %.0f", il.Cost.BaseCost)
	}
	between(t, "Ice Lance rank 6", castDamage(t, sim, il, sim.Encounter.TargetUnits[0], 2*time.Second), 136, 160)

	// Lower levels learn lower ranks; a talent taken before rank 1's level uses rank 1.
	for _, c := range []struct {
		level     int32
		ranks     []foreverMageRank
		low, mana float64
	}{
		{30, foreverArcaneBlastRanks, 131, 0}, {49, foreverArcaneBlastRanks, 168, 0}, {10, foreverArcaneBlastRanks, 57, 0},
		{41, foreverIceLanceRanks, 44, 70}, {56, foreverIceLanceRanks, 136, 160}, {15, foreverIceLanceRanks, 28, 45},
	} {
		r := foreverTalentRankAt(c.level, c.ranks)
		if r.low != c.low || r.mana != c.mana {
			t.Errorf("level %d: rank %d-%.0f mana %.0f, want low %.0f mana %.0f", c.level, int(r.low), r.high, r.mana, c.low, c.mana)
		}
	}
}

func TestForeverFrostfireBoltRank3AndDot(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassMage, 60, map[string]int32{}, "mage.baseline.frostfire-bolt")
	ffb := c.GetSpell(c.ForeverAction("mage.baseline.frostfire-bolt"))
	if ffb.Cost.BaseCost != 370 {
		t.Errorf("Frostfire Bolt rank 3 costs 370, got %.0f", ffb.Cost.BaseCost)
	}
	target := sim.Encounter.TargetUnits[0]
	direct := castDamage(t, sim, ffb, target, 3100*time.Millisecond)
	between(t, "Frostfire Bolt rank 3", direct, 270, 314)
	if !ffb.Dot(target).IsActive() {
		t.Fatal("Frostfire Bolt applies its 9 sec periodic damage")
	}
	foreverAdvance(sim, sim.CurrentTime+10*time.Second)
	if dot := ffb.SpellMetrics[target.UnitIndex].TotalDamage - direct; math.Abs(dot-57) > 1 {
		t.Errorf("Frostfire Bolt rank 3 periodic damage is 57 over 9 sec, got %.1f", dot)
	}
}

func TestForeverArcaneMissilesRank8(t *testing.T) {
	_, c := foreverValueSim(t, proto.Class_ClassMage, 60, map[string]int32{})
	mage := c.Env.Raid.Parties[0].Players[0].(MageAgent).GetMage()
	if mage.ArcaneMissiles[8] == nil {
		t.Fatal("Forever teaches Arcane Missiles rank 8 at 56")
	}
	if tick, coeff := mage.arcaneMissilesTick(8); tick != 209 || coeff != foreverArcaneMissilesCoeff {
		t.Errorf("rank 8 missile %.0f coefficient %.3f", tick, coeff)
	}
	if tick, _ := mage.arcaneMissilesTick(7); tick != 174 {
		t.Errorf("rank 7 missile is 174, got %.0f", tick)
	}
}

func TestForeverWarlockClientSpellValues(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassWarlock, 60, map[string]int32{"warlock.talent.incinerate": 1})
	inc := c.GetSpell(c.ForeverAction("warlock.talent.incinerate"))
	if inc.Cost.BaseCost != 325 {
		t.Errorf("Incinerate rank 3 costs 325, got %.0f", inc.Cost.BaseCost)
	}
	between(t, "Incinerate rank 3", castDamage(t, sim, inc, sim.Encounter.TargetUnits[0], 3*time.Second), 201, 233)

	// Periodic spells: the snapshot base per tick (no spell power) is the client's number.
	for _, c := range []struct {
		name  string
		goal  string
		spell func(*core.Character) *core.Spell
		tick  float64
		coeff float64
	}{
		{"Wrack", "warlock.talent.drain-hope", func(c *core.Character) *core.Spell { return c.GetSpell(c.ForeverAction("warlock.talent.drain-hope")) }, 36, .143},
		{"Drain Life rank 6", "", func(c *core.Character) *core.Spell { return c.GetSpell(core.ActionID{SpellID: 11700}) }, 51, .1},
		{"Siphon Life rank 4", "warlock.talent.siphon-life", func(c *core.Character) *core.Spell { return c.GetSpell(core.ActionID{SpellID: 18881}) }, 41, .05},
	} {
		goals := map[string]int32{}
		if c.goal != "" {
			goals[c.goal] = 1
		}
		sim, ch := foreverValueSim(t, proto.Class_ClassWarlock, 60, goals)
		s, target := c.spell(ch), sim.Encounter.TargetUnits[0]
		if s == nil {
			t.Fatalf("%s not registered", c.name)
		}
		if !s.Cast(sim, target) {
			t.Fatalf("%s cast", c.name)
		}
		d := s.Dot(target)
		if math.Abs(d.SnapshotBaseDamage-c.tick) > 1e-9 || math.Abs(d.BonusCoefficient-c.coeff) > 1e-9 {
			t.Errorf("%s: %.2f a tick at %.3f, want %.0f at %.3f", c.name, d.SnapshotBaseDamage, d.BonusCoefficient, c.tick, c.coeff)
		}
	}
}

func TestForeverWarlockConflagrateAndShadowburnRanks(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassWarlock, 60, map[string]int32{"warlock.talent.conflagrate": 1, "warlock.talent.shadowburn": 1})
	for _, id := range []int32{1293817, 1293818, 17962, 18930, 18931, 18932} {
		if c.GetSpell(core.ActionID{SpellID: id}) == nil {
			t.Errorf("Conflagrate %d should be known at 60", id)
		}
	}
	target := sim.Encounter.TargetUnits[0]
	between(t, "Shadowburn rank 6", castDamage(t, sim, c.GetSpell(core.ActionID{SpellID: 18871}), target, time.Second), 258, 288)
	immolate := c.GetSpell(core.ActionID{SpellID: 25309})
	if immolate == nil {
		immolate = c.GetSpell(core.ActionID{SpellID: 11668})
	}
	if !immolate.Cast(sim, target) {
		t.Fatal("Immolate cast")
	}
	foreverAdvance(sim, sim.CurrentTime+3*time.Second)
	between(t, "Conflagrate rank 4 (18930)", castDamage(t, sim, c.GetSpell(core.ActionID{SpellID: 18930}), target, time.Second), 178, 222)

	sim, c = foreverValueSim(t, proto.Class_ClassWarlock, 60, map[string]int32{"warlock.talent.shadowburn": 1})
	between(t, "Shadowburn rank 1", castDamage(t, sim, c.GetSpell(core.ActionID{SpellID: 17877}), sim.Encounter.TargetUnits[0], time.Second), 65, 73)
}

func TestForeverWarlockDrainTalents(t *testing.T) {
	_, c := foreverValueSim(t, proto.Class_ClassWarlock, 60, map[string]int32{"warlock.talent.improved-drains": 3})
	drain := c.GetSpell(core.ActionID{SpellID: 11700})
	if m := drain.TargetDamageMultiplier(c.AttackTables[c.CurrentTarget.UnitIndex][drain.CastType], true); math.Abs(m-1.20) > 1e-9 {
		t.Errorf("Improved Drains rank 3 is a flat 20%%, got %.4f", m)
	}
	// Soul Siphon no longer speeds drains up.
	_, c = foreverValueSim(t, proto.Class_ClassWarlock, 60, map[string]int32{"warlock.talent.soul-siphon": 3})
	if d := c.GetSpell(core.ActionID{SpellID: 11700}).Dot(c.CurrentTarget); d.TickLength != time.Second {
		t.Errorf("Drain Life ticks every 1 sec, got %s", d.TickLength)
	}
}

func TestForeverPriestClientSpellValues(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassPriest, 60, map[string]int32{"priest.talent.holy-nova": 1})
	nova := c.GetSpell(c.ForeverAction("priest.talent.holy-nova"))
	if nova.Cost.BaseCost != 750 {
		t.Errorf("Holy Nova rank 6 costs 750, got %.0f", nova.Cost.BaseCost)
	}
	between(t, "Holy Nova rank 6", castDamage(t, sim, nova, sim.Encounter.TargetUnits[0], time.Second), 174, 200)

	sim, c = foreverValueSim(t, proto.Class_ClassPriest, 60, map[string]int32{"priest.talent.penance": 1})
	penance := c.GetSpell(c.ForeverAction("priest.talent.penance"))
	if penance.Cost.BaseCost != 355 {
		t.Errorf("Penance rank 4 costs 355, got %.0f", penance.Cost.BaseCost)
	}
	between(t, "Penance rank 4 (three bolts)", castDamage(t, sim, penance, sim.Encounter.TargetUnits[0], 3*time.Second), 3*131, 3*131)

	sim, c = foreverValueSim(t, proto.Class_ClassPriest, 60, map[string]int32{}, "priest.baseline.shadow-word-death")
	swd := c.GetSpell(c.ForeverAction("priest.baseline.shadow-word-death"))
	if swd.Cost.BaseCost != 340 || swd.CD.Duration != 15*time.Second {
		t.Errorf("Shadow Word: Death rank 4: 340 mana, 15 sec; got %.0f, %s", swd.Cost.BaseCost, swd.CD.Duration)
	}
	health := c.CurrentHealth()
	between(t, "Shadow Word: Death rank 4", castDamage(t, sim, swd, sim.Encounter.TargetUnits[0], time.Second), 444, 472)
	if lost := health - c.CurrentHealth(); math.Abs(lost-.1*c.MaxHealth()) > 1 {
		t.Errorf("Shadow Word: Death backlash is 10%% of max health (%.0f), lost %.0f", .1*c.MaxHealth(), lost)
	}
}

func TestForeverPriestMentalAgilitySkipsChannels(t *testing.T) {
	_, c := foreverValueSim(t, proto.Class_ClassPriest, 60, map[string]int32{"priest.talent.mental-agility": 3, "priest.talent.mind-flay": 1})
	if flay := c.GetSpell(core.ActionID{SpellID: 18807}); flay.Cost.Multiplier != 100 {
		t.Errorf("Mind Flay is channeled, not instant: cost multiplier %d", flay.Cost.Multiplier)
	}
	if swp := c.GetSpell(core.ActionID{SpellID: 10894}); swp.Cost.Multiplier != 90 {
		t.Errorf("Shadow Word: Pain is instant: cost multiplier %d", swp.Cost.Multiplier)
	}
}

func TestForeverShadowWeavingFollowsThePriest(t *testing.T) {
	sim, c := foreverValueSim(t, proto.Class_ClassPriest, 60, map[string]int32{"priest.talent.shadow-weaving": 3})
	swp := c.GetSpell(core.ActionID{SpellID: 10894})
	if !swp.Cast(sim, sim.Encounter.TargetUnits[0]) {
		t.Fatal("Shadow Word: Pain cast")
	}
	foreverAdvance(sim, sim.CurrentTime+2*time.Second)
	weaving := c.GetAura("Forever Shadow Weaving")
	if weaving == nil || weaving.GetStacks() != 1 {
		t.Fatalf("one landed Shadow spell at 3/3 gives one self stack; got %v", weaving)
	}
	// The stack is the priest's: a Shadow spell on the second target benefits too.
	other := sim.Encounter.TargetUnits[1]
	if m := swp.TargetDamageMultiplier(c.AttackTables[other.UnitIndex][swp.CastType], false); math.Abs(m-1.02) > 1e-9 {
		t.Errorf("Shadow Weaving applies on another target: multiplier %.4f", m)
	}
	// Periodic ticks do not roll: after 8 sec of ticks the stack count is still one.
	foreverAdvance(sim, sim.CurrentTime+8*time.Second)
	if weaving.GetStacks() != 1 {
		t.Errorf("Shadow Word: Pain ticks rolled Shadow Weaving: %d stacks", weaving.GetStacks())
	}
}

func TestForeverMageArcaneConcentrationOncePerMissiles(t *testing.T) {
	// 5/5 is 10% "after any damage spell hits". Rolling on each of five missiles would give
	// about 41% a channel; one roll a channel gives 10%.
	channels, procs := 0, 0
	for seed := int64(1); seed <= 40; seed++ {
		p := foreverCasterBuild(t, proto.Class_ClassMage, map[string]int32{"mage.talent.arcane-concentration": 5})
		p.BonusStats.Stats[stats.SpellCrit] = 0
		p.BonusStats.Stats[stats.SpellHit] = 100 * core.SpellHitRatingPerHitChance
		request := &proto.RaidSimRequest{Raid: &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{p}}}}, Encounter: &proto.Encounter{Duration: 90, Targets: []*proto.Target{{Level: 60, MobType: proto.MobType_MobTypeHumanoid}}}, SimOptions: &proto.SimOptions{Iterations: 1, RandomSeed: seed, IsTest: true, Interactive: true}}
		sim := core.NewSim(request, simsignals.Signals{})
		sim.Reset()
		mage := sim.Raid.Parties[0].Players[0].(MageAgent).GetMage()
		cc := mage.ClearcastingAura
		cc.ApplyOnGain(func(a *core.Aura, sim *core.Simulation) { procs++ })
		missiles := mage.ArcaneMissiles[8]
		for i := 0; i < 12; i++ {
			if !missiles.Cast(sim, sim.Encounter.TargetUnits[0]) {
				t.Fatal("Arcane Missiles cast")
			}
			foreverAdvance(sim, sim.CurrentTime+6*time.Second)
			cc.Deactivate(sim)
			channels++
		}
	}
	if rate := float64(procs) / float64(channels); rate > .2 {
		t.Errorf("Clearcasting on %.0f%% of Arcane Missiles channels; one 10%% roll a channel expected", 100*rate)
	}
}
