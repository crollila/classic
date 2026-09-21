package sim

import (
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

// These tests prove that ordinary Forever values reach the simulator from the game data alone:
// each one changes a value in a copy of the client build's data, selects it by build label, and
// checks the simulation follows -- with no engine or class code change.

func foreverPlayer(c levelCase, level int32, build string, talents map[string]int32) *proto.Player {
	player := levelPlayer(c, level)
	if talents == nil {
		talents = map[string]int32{}
	}
	player.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Mode: proto.ForeverMode_BEST_GUESS,
		Talents: talents, GameDataBuild: build}
	return player
}

func caseNamed(t *testing.T, name string) levelCase {
	for _, c := range levelCases() {
		if c.name == name {
			return c
		}
	}
	t.Fatalf("no level case %s", name)
	return levelCase{}
}

func runForever(t *testing.T, player *proto.Player, level int32, iterations int32) (*proto.RaidSimResult, *core.Character) {
	t.Helper()
	target := &proto.Target{Level: level + 2, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}
	request := &proto.RaidSimRequest{
		Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
		Encounter:  &proto.Encounter{Duration: 120, Targets: []*proto.Target{target}},
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: 101},
	}
	result := core.RunRaidSim(request)
	if result.Error != nil {
		t.Fatalf("%s", result.Error.Message)
	}
	env, _, _ := core.NewEnvironment(request.Raid, request.Encounter, false)
	return result, env.Raid.Parties[0].Players[0].GetCharacter()
}

func dpsOf(r *proto.RaidSimResult) float64 { return r.RaidMetrics.Parties[0].Players[0].Dps.Avg }

// Verification 5: a spell's cast time and damage come from data.
func TestDataDrivenSpellCastTimeAndDamage(t *testing.T) {
	shaman := caseNamed(t, "ElementalShaman")
	base, baseChar := runForever(t, foreverPlayer(shaman, 20, "", nil), 20, 300)
	bolt := baseChar.GetSpell(core.ActionID{SpellID: 915}) // Lightning Bolt rank 4, learned at 20
	if bolt == nil {
		t.Fatal("Lightning Bolt rank 4 not registered at level 20")
	}
	want := time.Duration(gamedata.Current().Spell(915).CastMs) * time.Millisecond
	if bolt.DefaultCast.CastTime != want {
		t.Fatalf("Lightning Bolt cast %v, client says %v", bolt.DefaultCast.CastTime, want)
	}

	gamedata.RegisterVariant("test-lightning-bolt", func(s *gamedata.Snapshot) {
		s.Spell(915).CastMs = 1500
		e := s.Spell(915).Effect(0)
		e.Min, e.Max, e.Base = e.Min*2, e.Max*2, e.Base*2
	})
	changed, changedChar := runForever(t, foreverPlayer(shaman, 20, "test-lightning-bolt", nil), 20, 300)
	if cast := changedChar.GetSpell(core.ActionID{SpellID: 915}).DefaultCast.CastTime; cast != 1500*time.Millisecond {
		t.Fatalf("changed data: Lightning Bolt cast %v, want 1.5s", cast)
	}
	if dpsOf(changed) < dpsOf(base)*1.3 {
		t.Fatalf("doubling Lightning Bolt damage and cutting its cast time moved DPS only %.1f -> %.1f", dpsOf(base), dpsOf(changed))
	}
	t.Logf("Lightning Bolt from data: %.1f -> %.1f DPS", dpsOf(base), dpsOf(changed))
}

// Verification 8: an older build's data stays selectable after a newer one is current.
func TestHistoricalBuildSelectable(t *testing.T) {
	gamedata.RegisterVariant("test-older-build", func(s *gamedata.Snapshot) { s.Spell(915).CastMs = 3000 })
	_, older := runForever(t, foreverPlayer(caseNamed(t, "ElementalShaman"), 20, "test-older-build", nil), 20, 10)
	_, current := runForever(t, foreverPlayer(caseNamed(t, "ElementalShaman"), 20, "", nil), 20, 10)
	if older.GetSpell(core.ActionID{SpellID: 915}).DefaultCast.CastTime == current.GetSpell(core.ActionID{SpellID: 915}).DefaultCast.CastTime {
		t.Fatal("selecting the older build did not change the simulated data")
	}
	if older.GameData.Build != "test-older-build" || current.GameData.Build != gamedata.Current().Build {
		t.Fatalf("builds: %s / %s", older.GameData.Build, current.GameData.Build)
	}
}

// Every DPS spec simulates in Forever mode at level 20 with client data.
func TestLevel20SpecsForever(t *testing.T) {
	for _, c := range levelCases() {
		t.Run(c.name, func(t *testing.T) {
			result, _ := runForever(t, foreverPlayer(c, 20, "", nil), 20, 100)
			if dpsOf(result) <= 0 {
				t.Fatalf("%s: no damage", c.name)
			}
		})
	}
}

// Verification 8 (reproducibility): a result names the game data and seed it came from, and the
// same request with that seed and build reproduces it exactly.
func TestResultProvenanceReproduces(t *testing.T) {
	player := foreverPlayer(caseNamed(t, "Rogue"), 20, "", nil)
	request := func(seed int64) *proto.RaidSimRequest {
		return &proto.RaidSimRequest{
			Raid:       &proto.Raid{Parties: []*proto.Party{{Players: []*proto.Player{player}}}, Buffs: &proto.RaidBuffs{}, Debuffs: &proto.Debuffs{}},
			Encounter:  &proto.Encounter{Duration: 60, Targets: []*proto.Target{{Level: 22, MobType: proto.MobType_MobTypeHumanoid, Stats: stats.Stats{stats.Armor: 850}.ToFloatArray()}}},
			SimOptions: &proto.SimOptions{Iterations: 200, RandomSeed: seed},
		}
	}
	first := core.RunRaidSim(request(0))
	p := first.Provenance
	if p == nil || p.GameDataBuild != gamedata.Current().Build || p.GameDataSha256 == "" || p.RandomSeed == 0 {
		t.Fatalf("provenance incomplete: %+v", p)
	}
	player.Forever.GameDataBuild = p.GameDataBuild
	again := core.RunRaidSim(request(p.RandomSeed))
	if again.RaidMetrics.Dps.Avg != first.RaidMetrics.Dps.Avg {
		t.Fatalf("same build + seed gave %.6f then %.6f", first.RaidMetrics.Dps.Avg, again.RaidMetrics.Dps.Avg)
	}
}
