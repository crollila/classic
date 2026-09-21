package forever_test

import (
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
	"google.golang.org/protobuf/encoding/protojson"
	googleproto "google.golang.org/protobuf/proto"
)

// withoutOverrides isolates a test from the generated embedded overrides.json, which the knowledge
// pipeline rewrites; engine semantics are tested against explicit fixtures only.
func withoutOverrides(t *testing.T) {
	t.Helper()
	// These tests are about the override document; the client-data layer is tested on its own.
	t.Cleanup(gamedata.DisableForTesting())
	o, err := foreverdata.ParseOverrides([]byte(`{"version":"forever-overrides-1","generated_at":"2026-01-01T00:00:00Z","source_hash":"` + strings.Repeat("0", 64) + `","spells":{},"items":{},"talents":{},"parameters":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(foreverdata.SetOverridesForTesting(o))
}

func withOverrideFixture(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("../core/foreverdata/testdata/overrides_valid.json")
	if err != nil {
		t.Fatal(err)
	}
	o, err := foreverdata.ParseOverrides(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(foreverdata.SetOverridesForTesting(o))
}

func overrideMage(t *testing.T) *proto.Player {
	return discoveryPlayer(t, "Mage", "Gnome", "mage", map[string]int32{})
}

func findEvent(events []foreverdata.OverrideEvent, id, field string) *foreverdata.OverrideEvent {
	for i := range events {
		if events[i].ID == id && events[i].Field == field {
			return &events[i]
		}
	}
	return nil
}

func nearRel(t *testing.T, actual, want float64) {
	t.Helper()
	if math.Abs(actual-want) > 1e-9*math.Max(1, math.Abs(want)) {
		t.Fatalf("got %.12f want %.12f", actual, want)
	}
}

func TestOverridesFrostboltCoefficientBaseCastAndCost(t *testing.T) {
	withoutOverrides(t)
	classicSim, classicC := simulation(t, overrideMage(t), nil)
	classicSpell := classicC.GetSpell(core.ActionID{SpellID: 116})

	withOverrideFixture(t)
	sim, c := simulation(t, overrideMage(t), nil)
	spell := c.GetSpell(core.ActionID{SpellID: 116})
	if classicSpell == nil || spell == nil {
		t.Fatal("Frostbolt rank 1 not registered")
	}

	if classicSpell.BonusCoefficient != .163 || spell.BonusCoefficient != .5 {
		t.Fatalf("coefficient not overridden: classic %v forever %v", classicSpell.BonusCoefficient, spell.BonusCoefficient)
	}
	if classicSpell.DefaultCast.CastTime != 1500*time.Millisecond || spell.DefaultCast.CastTime != 1000*time.Millisecond {
		t.Fatalf("cast time delta not applied: %v %v", classicSpell.DefaultCast.CastTime, spell.DefaultCast.CastTime)
	}
	if spell.DefaultCast.Cost-classicSpell.DefaultCast.Cost != 5 {
		t.Fatalf("mana cost delta not applied: %v -> %v", classicSpell.DefaultCast.Cost, spell.DefaultCast.Cost)
	}

	// Base ratio avg(40,44)/avg(20,22) = 2 multiplies only the base; the coefficient
	// multiplies the unchanged spell power bonus.
	target, classicTarget := sim.Encounter.TargetUnits[0], classicSim.Encounter.TargetUnits[0]
	bonus := spell.GetBonusDamage(target)
	if bonus <= 0 || bonus != classicSpell.GetBonusDamage(classicTarget) {
		t.Fatal("fixture needs identical positive spell power", bonus)
	}
	forever := spell.CalcDamage(sim, target, 21, spell.OutcomeAlwaysHit).Damage
	classic := classicSpell.CalcDamage(classicSim, classicTarget, 21, classicSpell.OutcomeAlwaysHit).Damage
	if forever <= classic {
		t.Fatal("damage did not increase", classic, forever)
	}
	nearRel(t, forever/classic, (21*2+.5*bonus)/(21+.163*bonus))

	report := c.ForeverOverrideReport()
	for _, field := range []string{"effects.0.coefficient", "effects.0.base", "cast_ms", "cost"} {
		ev := findEvent(report.Applied, "116", field)
		if ev == nil || len(ev.Beliefs) == 0 || ev.Beliefs[0] != "belief:mage.frostbolt.r1" {
			t.Fatalf("missing applied provenance for 116 %s: %+v", field, report)
		}
	}
	// Rank 2 declares classic coefficient 0.5 but the sim registers 0.269: rejected, unchanged.
	rejected := findEvent(report.Rejected, "205", "effects.0.coefficient")
	if rejected == nil || !strings.Contains(rejected.Reason, "does not match classic") || *rejected.Registered != .269 {
		t.Fatalf("mismatch was not rejected and recorded: %+v", report.Rejected)
	}
	if c.GetSpell(core.ActionID{SpellID: 205}).BonusCoefficient != .269 {
		t.Fatal("mismatched coefficient was applied")
	}
}

func TestOverridesStrictRefusesPredicted(t *testing.T) {
	withoutOverrides(t)
	withOverrideFixture(t)
	p := overrideMage(t)
	p.Forever.Mode = proto.ForeverMode_STRICT
	_, c := simulation(t, p, nil)
	ev := findEvent(c.ForeverOverrideReport().Rejected, "205", "*")
	if ev == nil || !strings.Contains(ev.Reason, "STRICT") {
		t.Fatal("PREDICTED override not refused in STRICT")
	}
}

func TestOverridesLeaveClassicUnchanged(t *testing.T) {
	withoutOverrides(t)
	classic := overrideMage(t)
	classic.Forever = nil
	baseSim, base := simulation(t, googleproto.Clone(classic).(*proto.Player), nil)
	withOverrideFixture(t)
	sim, c := simulation(t, classic, nil)
	a, b := base.GetSpell(core.ActionID{SpellID: 116}), c.GetSpell(core.ActionID{SpellID: 116})
	if a.BonusCoefficient != b.BonusCoefficient || a.DefaultCast != b.DefaultCast || base.GetStats() != c.GetStats() {
		t.Fatal("Classic character changed by overrides")
	}
	if a.CalcDamage(baseSim, baseSim.Encounter.TargetUnits[0], 21, a.OutcomeAlwaysHit).Damage != b.CalcDamage(sim, sim.Encounter.TargetUnits[0], 21, b.OutcomeAlwaysHit).Damage {
		t.Fatal("Classic damage changed by overrides")
	}
	if r := c.ForeverOverrideReport(); len(r.Applied)+len(r.Rejected) != 0 {
		t.Fatal("Classic character recorded override events")
	}

	if !core.WITH_DB {
		t.Skip("full-request parity needs -tags=with_db")
	}
	data, err := os.ReadFile("../../examples/mage-classic.json")
	if err != nil {
		t.Fatal(err)
	}
	request := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, request); err != nil {
		t.Fatal(err)
	}
	with := core.RunRaidSim(googleproto.Clone(request).(*proto.RaidSimRequest))
	restore := foreverdata.SetOverridesForTesting(foreverdata.ActiveOverrides())
	defer restore()
	empty, _ := foreverdata.ParseOverrides([]byte(`{"version":"forever-overrides-1","generated_at":"1970-01-01T00:00:00Z","source_hash":"","spells":{},"items":{},"talents":{},"parameters":{}}`))
	foreverdata.SetOverridesForTesting(empty)
	without := core.RunRaidSim(googleproto.Clone(request).(*proto.RaidSimRequest))
	if with.Error != nil || without.Error != nil || with.RaidMetrics.Dps.Avg <= 0 || with.RaidMetrics.Dps.Avg != without.RaidMetrics.Dps.Avg || with.RaidMetrics.Dps.Stdev != without.RaidMetrics.Dps.Stdev {
		t.Fatalf("Classic request changed by overrides: %v vs %v", with.GetRaidMetrics().GetDps(), without.GetRaidMetrics().GetDps())
	}
}

func TestOverridesConsumableSpellValue(t *testing.T) {
	withoutOverrides(t)
	p := overrideMage(t)
	p.Consumes = &proto.Consumes{Flask: proto.Flask_FlaskOfSupremePower}
	_, base := simulation(t, googleproto.Clone(p).(*proto.Player), nil)
	withOverrideFixture(t)
	_, c := simulation(t, p, nil)
	nearRel(t, c.GetStat(stats.SpellPower)-base.GetStat(stats.SpellPower), 150*199.0/149-150)
	ev := findEvent(c.ForeverOverrideReport().Applied, "17628", "effects.0.base")
	if ev == nil || ev.Scope != foreverdata.ScopeSpellValue {
		t.Fatal("flask spell value not recorded")
	}
}

func TestOverridesItemStatsAndNewItem(t *testing.T) {
	withoutOverrides(t)
	core.ItemsByID[990001] = core.Item{ID: 990001, Name: "Classic Test Helm", Type: proto.ItemType_ItemTypeHead, Stats: stats.Stats{stats.Strength: 10}}
	t.Cleanup(func() { delete(core.ItemsByID, 990001) })
	p := overrideMage(t)
	p.Equipment = &proto.EquipmentSpec{Items: []*proto.ItemSpec{{Id: 990001}}}
	classicPlayer := googleproto.Clone(p).(*proto.Player)
	classicPlayer.Forever = nil
	_, base := simulation(t, googleproto.Clone(p).(*proto.Player), nil)

	withOverrideFixture(t)
	_, c := simulation(t, p, nil)
	if c.Head().Stats[stats.Strength] != 15 || base.Head().Stats[stats.Strength] != 10 {
		t.Fatal("item stat delta not applied", c.Head().Stats[stats.Strength])
	}
	near(t, c.GetStat(stats.Strength)-base.GetStat(stats.Strength), 5)
	if c.Head().Stats[stats.Agility] != 0 {
		t.Fatal("mismatched agility override applied")
	}
	report := c.ForeverOverrideReport()
	if findEvent(report.Applied, "990001", "stats.strength") == nil || findEvent(report.Rejected, "990001", "stats.agility") == nil {
		t.Fatalf("item diagnostics missing: %+v", report)
	}
	_, classicC := simulation(t, classicPlayer, nil)
	if classicC.Head().Stats[stats.Strength] != 10 {
		t.Fatal("Classic item changed")
	}

	p = overrideMage(t)
	p.Equipment = &proto.EquipmentSpec{Items: []*proto.ItemSpec{{Id: 990002}}}
	_, c = simulation(t, p, nil)
	if c.Head().Name != "Forever Test Circlet" || c.Head().Stats[stats.SpellPower] != 20 || findEvent(c.ForeverOverrideReport().Applied, "990002", "new_item") == nil {
		t.Fatal("new_item not usable by Forever request", c.Head())
	}
	if _, ok := core.ItemsByID[990002]; ok {
		t.Fatal("new_item leaked into the Classic item database")
	}
	classicPlayer = googleproto.Clone(p).(*proto.Player)
	classicPlayer.Forever = nil
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("Classic request could use a Forever-only item")
			}
		}()
		simulation(t, classicPlayer, nil)
	}()
}

func TestOverridesTalentValues(t *testing.T) {
	withoutOverrides(t)
	p := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{"warrior.talent.anticipation": 1})
	_, base := simulation(t, googleproto.Clone(p).(*proto.Player), nil)
	withOverrideFixture(t)
	_, c := simulation(t, p, nil)
	near(t, c.GetStat(stats.Defense)-base.GetStat(stats.Defense), 1) // rank 1: 4 -> 5
	if findEvent(c.ForeverOverrideReport().Applied, "warrior.talent.anticipation", "values") == nil {
		t.Fatal("talent override not recorded")
	}
}

func TestOverridesParameterDefaultAndRequestPrecedence(t *testing.T) {
	withoutOverrides(t)
	key := foreverdata.ParamSpellCritDamageMultiplier
	_, base := simulation(t, overrideMage(t), nil)
	if base.ForeverParameter(key, 1.5) != 1.5 {
		t.Fatal("unexpected default without overrides")
	}
	withOverrideFixture(t)
	sim, c := simulation(t, overrideMage(t), nil)
	if c.ForeverParameter(key, 1.5) != 2 {
		t.Fatal("override default not used")
	}
	spell := c.GetSpell(core.ActionID{SpellID: 116})
	at := c.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]
	near(t, spell.CritMultiplier(at), 1+(2*at.CritMultiplier-1)*spell.CritDamageBonus)
	if findEvent(c.ForeverOverrideReport().Applied, key, "value") == nil {
		t.Fatal("parameter default not recorded")
	}

	p := overrideMage(t)
	p.Forever.Parameters = map[string]float64{key: 1.75}
	sim, c = simulation(t, p, nil)
	if c.ForeverParameter(key, 1.5) != 1.75 {
		t.Fatal("request value did not win")
	}
	at = c.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]
	near(t, spell.CritDamageBonus, c.GetSpell(core.ActionID{SpellID: 116}).CritDamageBonus)
	near(t, c.GetSpell(core.ActionID{SpellID: 116}).CritMultiplier(at), 1+(1.75*at.CritMultiplier-1)*spell.CritDamageBonus)
	if findEvent(c.ForeverOverrideReport().Applied, key, "value") != nil {
		t.Fatal("override default recorded although the request set the parameter")
	}
	p.Forever.Parameters = map[string]float64{key: 9}
	if foreverdata.Validate(p) == nil {
		t.Fatal("out-of-bounds core parameter accepted")
	}
}

func TestOverridesStatConversionParameter(t *testing.T) {
	withoutOverrides(t)
	p := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{})
	_, base := simulation(t, googleproto.Clone(p).(*proto.Player), nil)
	p.Forever.Parameters = map[string]float64{foreverdata.ConversionParam("warrior", "attack_power_per_strength"): 3}
	_, c := simulation(t, p, nil)
	near(t, c.GetStat(stats.AttackPower)-base.GetStat(stats.AttackPower), c.GetStat(stats.Strength))
}

func TestRunRaidSimWithForeverOverrideReport(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("example request needs -tags=with_db")
	}
	data, err := os.ReadFile("../../examples/mage-discovery.json")
	if err != nil {
		t.Fatal(err)
	}
	request := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, request); err != nil {
		t.Fatal(err)
	}
	withOverrideFixture(t)
	result, report := core.RunRaidSimWithForeverOverrideReport(request)
	if result.Error != nil || report == nil || len(report.Players) != 1 || report.Overrides.Counts.Spells != 3 {
		t.Fatalf("unexpected run report: %v %+v", result.Error, report)
	}
	if findEvent(report.Players[0].Applied, "116", "effects.0.coefficient") == nil {
		t.Fatalf("run report lacks spell diagnostics: %+v", report.Players[0])
	}
}

// The web worker path: SetActiveOverrides swaps the document and the next sim uses it.
func TestSimAfterSetActiveOverridesUsesNewValues(t *testing.T) {
	withoutOverrides(t) // also restores the original document after the swaps below
	_, before := simulation(t, overrideMage(t), nil)

	raw, err := os.ReadFile("../core/foreverdata/testdata/overrides_valid.json")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := foreverdata.SetActiveOverrides(raw)
	if err != nil || summary.Counts.Spells != 3 {
		t.Fatal(err, summary)
	}
	_, after := simulation(t, overrideMage(t), nil)
	if before.GetSpell(core.ActionID{SpellID: 116}).BonusCoefficient != .163 || after.GetSpell(core.ActionID{SpellID: 116}).BonusCoefficient != .5 {
		t.Fatal("sim built after the swap does not use the new document")
	}
	if findEvent(after.ForeverOverrideReport().Applied, "116", "effects.0.coefficient") == nil {
		t.Fatal("swap not visible in diagnostics")
	}

	// A rejected document leaves the swapped-in one active for the next sim.
	if _, err := foreverdata.SetActiveOverrides([]byte(`{"version":"forever-overrides-1"}`)); err == nil {
		t.Fatal("invalid document accepted")
	}
	_, still := simulation(t, overrideMage(t), nil)
	if still.GetSpell(core.ActionID{SpellID: 116}).BonusCoefficient != .5 {
		t.Fatal("rejected document changed the active overrides")
	}

	if !core.WITH_DB {
		return
	}
	data, err := os.ReadFile("../../examples/mage-discovery.json")
	if err != nil {
		t.Fatal(err)
	}
	request := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, request); err != nil {
		t.Fatal(err)
	}
	result, report := core.RunRaidSimWithForeverOverrideReport(request)
	if result.Error != nil || report.Overrides.SourceHash != summary.SourceHash || findEvent(report.Players[0].Applied, "116", "effects.0.coefficient") == nil {
		t.Fatalf("full sim after swap did not use the new document: %v %+v", result.Error, report.Overrides)
	}
}
