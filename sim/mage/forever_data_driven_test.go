package mage

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// Forever mage numbers come from the client build the character simulates: a changed value
// in a copy of the build, selected by label, must reach the simulation with no code change.

// foreverMageOnBuild is foreverValueSim on a selected client build.
func foreverMageOnBuild(t *testing.T, build string, level int32, goals map[string]int32, mechanics ...string) (*core.Simulation, *Mage) {
	t.Helper()
	p := foreverCasterBuild(t, proto.Class_ClassMage, goals, mechanics...)
	p.Level = level
	p.Forever.GameDataBuild = build
	p.BonusStats.Stats[stats.SpellCrit] = 0
	p.BonusStats.Stats[stats.SpellHit] = 100 * core.SpellHitRatingPerHitChance
	sim, c := foreverCasterSim(t, p)
	return sim, c.Env.Raid.Parties[0].Players[0].(MageAgent).GetMage()
}

// (a) A Forever-only spell's damage and cost follow the client data.
func TestForeverMageArcaneBlastFollowsClientData(t *testing.T) {
	gamedata.RegisterVariant("test-mage-arcane-blast", func(s *gamedata.Snapshot) {
		ab := s.Spell(1239700) // Arcane Blast rank 5
		e := ab.Effect(0)
		e.Min, e.Max, e.Base = e.Min*2, e.Max*2, e.Base*2
		ab.Costs[0].CostPct = 30
		ab.CastMs = 2000
	})
	goals := map[string]int32{"mage.talent.arcane-blast": 1}
	sim, m := foreverMageOnBuild(t, "", 60, goals)
	ab := m.GetSpell(m.ForeverAction("mage.talent.arcane-blast"))
	base := castDamage(t, sim, ab, sim.Encounter.TargetUnits[0], 3*time.Second)

	sim, m = foreverMageOnBuild(t, "test-mage-arcane-blast", 60, goals)
	ab = m.GetSpell(m.ForeverAction("mage.talent.arcane-blast"))
	if want := .30 * m.BaseMana; math.Abs(ab.Cost.BaseCost-want) > 1e-6 {
		t.Errorf("changed data: Arcane Blast costs 30%% of base mana (%.1f), got %.1f", want, ab.Cost.BaseCost)
	}
	if ab.DefaultCast.CastTime != 2*time.Second {
		t.Errorf("changed data: Arcane Blast cast %v, want 2s", ab.DefaultCast.CastTime)
	}
	changed := castDamage(t, sim, ab, sim.Encounter.TargetUnits[0], 3*time.Second)
	if changed < base*1.6 {
		t.Errorf("doubling Arcane Blast's client damage moved one cast only %.0f -> %.0f", base, changed)
	}

	// Arcane Missiles' per-missile damage is the client missile spell's (25346 for rank 8).
	gamedata.RegisterVariant("test-mage-missile", func(s *gamedata.Snapshot) {
		e := s.Spell(25346).Effect(0)
		e.Min, e.Max, e.Base = 300, 300, 300
	})
	_, m = foreverMageOnBuild(t, "test-mage-missile", 60, nil)
	if tick, _ := m.arcaneMissilesTick(8); tick != 300 {
		t.Errorf("changed data: rank 8 missile %.0f, want 300", tick)
	}
}

// (b) A talent rank's client value reaches the simulation: Arcane Concentration's chance.
func TestForeverMageArcaneConcentrationChanceFollowsClientData(t *testing.T) {
	record := "mage.talent.arcane-concentration"
	gamedata.RegisterVariant("test-mage-ac-always", func(s *gamedata.Snapshot) {
		s.RawTalents[s.RecordTalents[record]].RankPoints[0]["0"] = 100
	})
	gamedata.RegisterVariant("test-mage-ac-never", func(s *gamedata.Snapshot) {
		s.RawTalents[s.RecordTalents[record]].RankPoints[0]["0"] = 0
	})
	procs := func(build string) int {
		n := 0
		for i := 0; i < 10; i++ {
			sim, m := foreverMageOnBuild(t, build, 60, map[string]int32{record: 1})
			if m.ClearcastingAura == nil {
				t.Fatal("Arcane Concentration not applied")
			}
			fireball := m.GetSpell(core.ActionID{SpellID: 25306})
			if fireball == nil {
				fireball = m.GetSpell(core.ActionID{SpellID: 10151})
			}
			castDamage(t, sim, fireball, sim.Encounter.TargetUnits[0], 4*time.Second)
			if m.ClearcastingAura.IsActive() {
				n++
			}
		}
		return n
	}
	if n := procs("test-mage-ac-always"); n != 10 {
		t.Errorf("client chance 100%%: Clearcasting after %d of 10 hits", n)
	}
	if n := procs("test-mage-ac-never"); n != 0 {
		t.Errorf("client chance 0%%: Clearcasting after %d of 10 hits", n)
	}

	// Fingers of Frost's charges are its client effect 0 at the chosen rank.
	gamedata.RegisterVariant("test-mage-fingers", func(s *gamedata.Snapshot) {
		s.RawTalents[s.RecordTalents["mage.talent.fingers-of-frost"]].RankPoints[1]["0"] = 3
	})
	_, m := foreverMageOnBuild(t, "test-mage-fingers", 60, map[string]int32{"mage.talent.fingers-of-frost": 2})
	if got := m.foreverState.fingers.MaxStacks; got != 3 {
		t.Errorf("changed data: Fingers of Frost covers %d spells, want 3", got)
	}
}

// (c) Talent-granted and new multi-rank Forever spells use the highest rank learned at the
// character's level (not rank 1), with that rank's client values at that level.
func TestForeverMageRanksByLevel(t *testing.T) {
	data := gamedata.Current()
	rangeAt := func(id int32, idx int, level int32) (float64, float64) {
		s := data.Spell(id)
		lo, hi := s.Effect(idx).Range()
		b := foreverLevelBonus(level, s, s.Effect(idx))
		return lo + b, hi + b
	}
	for _, c := range []struct {
		level                int32
		blast, lance, fireFB int32
		missiles             int
	}{
		{20, 400574, 1312002, 0, 2},
		{40, 1239697, 1240044, 401502, 5},
		{60, 1239700, 1240047, 1237313, 8},
	} {
		sim, m := foreverMageOnBuild(t, "", c.level, map[string]int32{"mage.talent.arcane-blast": 1, "mage.talent.ice-lance": 1}, "mage.baseline.frostfire-bolt")
		f := m.foreverState
		if f.arcaneBlast.id != c.blast || f.iceLance.id != c.lance || f.frostfireBolt.id != c.fireFB {
			t.Errorf("level %d: Arcane Blast %d Ice Lance %d Frostfire Bolt %d, want %d %d %d", c.level, f.arcaneBlast.id, f.iceLance.id, f.frostfireBolt.id, c.blast, c.lance, c.fireFB)
		}
		for _, r := range []struct {
			name string
			got  foreverRankSpell
			id   int32
			idx  int
		}{{"Arcane Blast", f.arcaneBlast, c.blast, 0}, {"Ice Lance", f.iceLance, c.lance, 0}, {"Frostfire Bolt", f.frostfireBolt, c.fireFB, 1}} {
			if r.id == 0 {
				continue
			}
			if lo, hi := rangeAt(r.id, r.idx, c.level); r.got.low != lo || r.got.high != hi {
				t.Errorf("level %d: %s %.0f-%.0f, client rank at this level %.0f-%.0f", c.level, r.name, r.got.low, r.got.high, lo, hi)
			}
		}
		if c.level == 60 {
			between(t, "Arcane Blast", castDamage(t, sim, m.GetSpell(m.ForeverAction("mage.talent.arcane-blast")), sim.Encounter.TargetUnits[0], 3*time.Second), 364, 424)
		}
		il := m.GetSpell(m.ForeverAction("mage.talent.ice-lance"))
		if want := data.Spell(c.lance).Cost("mana").Cost; il.Cost.BaseCost != want {
			t.Errorf("level %d: Ice Lance costs %.0f, client rank says %.0f", c.level, il.Cost.BaseCost, want)
		}
		if c.fireFB != 0 {
			action := core.ActionID{SpellID: c.fireFB}
			if c.fireFB == 1237313 {
				action = m.ForeverAction("mage.baseline.frostfire-bolt")
			}
			ffb := m.GetSpell(action)
			if ffb == nil {
				t.Fatalf("level %d: Frostfire Bolt %d not registered", c.level, c.fireFB)
			}
			if want := data.Spell(c.fireFB).Cost("mana").Cost; ffb.Cost.BaseCost != want {
				t.Errorf("level %d: Frostfire Bolt costs %.0f, client rank says %.0f", c.level, ffb.Cost.BaseCost, want)
			}
		}
		for rank := c.missiles + 1; rank <= ArcaneMissilesRanks; rank++ {
			if m.ArcaneMissiles[rank] != nil {
				t.Errorf("level %d: Arcane Missiles rank %d registered before it is learned", c.level, rank)
			}
		}
		if m.ArcaneMissiles[c.missiles] == nil {
			t.Fatalf("level %d: Arcane Missiles rank %d not registered", c.level, c.missiles)
		}
		missile := data.Spell(ArcaneMissilesSpellId[c.missiles]).Effect(0).TriggerSpell
		want, _ := rangeAt(missile, 0, c.level)
		if tick, _ := m.arcaneMissilesTick(c.missiles); tick != want {
			t.Errorf("level %d: Arcane Missiles rank %d missile %.0f, client says %.0f", c.level, c.missiles, tick, want)
		}
	}
	// The per-level growth reproduces the client tooltip: Arcane Blast rank 1 at its top
	// level (28) is 57-65, at 20 the base 50-58.
	if lo, hi := rangeAt(400574, 0, 28); lo != 57 || hi != 65 {
		t.Errorf("Arcane Blast rank 1 at 28: %.0f-%.0f, tooltip 57-65", lo, hi)
	}
}

// (d) A level-60 Forever mage with the Forever talents records no fallback beyond the
// documented ones.
func TestForeverMageFallbacksDocumented(t *testing.T) {
	allowed := map[string]bool{
		"mage: Ice Lance|spell power coefficient":               true, // client lists 0
		"mage: Frostfire Bolt|periodic spell power coefficient": true, // client lists 0
	}
	goals := map[string]int32{"mage.talent.arcane-blast": 1, "mage.talent.ice-lance": 1, "mage.talent.missile-barrage": 1,
		"mage.talent.hot-streak": 1, "mage.talent.fingers-of-frost": 2, "mage.talent.winter-s-chill": 5, "mage.talent.combustion": 1,
		"mage.talent.arcane-concentration": 5, "mage.talent.improved-scorch": 3, "mage.talent.magic-absorption": 2}
	for id, n := range goals {
		sim, m := foreverMageOnBuild(t, "", 60, map[string]int32{id: n}, "mage.baseline.frostfire-bolt")
		for _, s := range m.Spellbook {
			if !s.Flags.Matches(core.SpellFlagAPL) || s.Flags.Matches(core.SpellFlagHelpful) {
				continue
			}
			if s.CanCast(sim, sim.Encounter.TargetUnits[0]) {
				s.Cast(sim, sim.Encounter.TargetUnits[0])
				foreverAdvance(sim, sim.CurrentTime+3*time.Second)
			}
		}
	}
	for _, f := range gamedata.Fallbacks() {
		key := f.Owner + "|" + f.What
		mageSpell := false
		for _, id := range []string{"12494", "22959", "12579", "400625", "400669", "400573", "400588", "400589", "12355", "12042", "28682", "11129", "11213", "12536", "29441",
			"400574", "1239696", "1239697", "1239699", "1239700", "1312002", "400640", "1240044", "1240045", "1240046", "1240047", "401502", "1237312", "1237313"} {
			mageSpell = mageSpell || strings.HasPrefix(f.Owner, "spell "+id+" ")
		}
		if !(mageSpell || strings.HasPrefix(f.Owner, "mage") || strings.HasPrefix(f.Owner, "talent mage.")) {
			continue
		}
		if !allowed[key] {
			t.Errorf("undocumented mage fallback: %s = %v (%s, %s)", key, f.Value, f.Confidence, f.Reason)
		}
	}
}
