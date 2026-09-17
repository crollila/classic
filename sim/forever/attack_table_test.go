package forever_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	googleproto "google.golang.org/protobuf/proto"
)

// attackTableFields flattens every numeric attack table field so tests can assert
// that a parameter changes exactly one of them.
func attackTableFields(at *core.AttackTable) map[string]float64 {
	return map[string]float64{
		"BaseMissChance": at.BaseMissChance, "HitSuppression": at.HitSuppression, "BaseSpellMissChance": at.BaseSpellMissChance,
		"BaseBlockChance": at.BaseBlockChance, "BaseDodgeChance": at.BaseDodgeChance, "BaseParryChance": at.BaseParryChance,
		"BaseGlanceChance": at.BaseGlanceChance, "BaseCritChance": at.BaseCritChance, "BaseCrushChance": at.BaseCrushChance,
		"DualWieldMissPenalty": at.DualWieldMissPenalty, "GlanceMultiplierMin": at.GlanceMultiplierMin, "GlanceMultiplierMax": at.GlanceMultiplierMax,
		"MeleeCritSuppression": at.MeleeCritSuppression, "SpellCritSuppression": at.SpellCritSuppression, "CritMultiplier": at.CritMultiplier,
		"DamageDealtMultiplier": at.DamageDealtMultiplier, "DamageTakenMultiplier": at.DamageTakenMultiplier,
	}
}

func warriorAttackTable(t *testing.T, p *proto.Player) (map[string]float64, *core.Character) {
	t.Helper()
	sim, c := simulation(t, p, nil)
	return attackTableFields(c.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]), c
}

func attackTableOverrides(t *testing.T, parameters string) *foreverdata.Overrides {
	t.Helper()
	o, err := foreverdata.ParseOverrides([]byte(`{"version":"forever-overrides-1","generated_at":"2026-01-01T00:00:00Z","source_hash":"` + strings.Repeat("0", 64) + `","spells":{},"items":{},"talents":{},"parameters":{` + parameters + `}}`))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func TestAttackTableParametersDefaultToClassic(t *testing.T) {
	withoutOverrides(t)
	forever := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{})
	classic := googleproto.Clone(forever).(*proto.Player)
	classic.Forever = nil
	want, _ := warriorAttackTable(t, classic)
	got, _ := warriorAttackTable(t, forever)
	for field, value := range want {
		if got[field] != value {
			t.Errorf("%s: Forever %v differs from Classic %v without parameters", field, got[field], value)
		}
	}
	if want["DualWieldMissPenalty"] != 0.19 || want["BaseMissChance"] <= 0 || want["BaseGlanceChance"] != 0.4 {
		t.Fatalf("unexpected Classic table: %v", want)
	}

	// Explicit Classic defaults are no-ops too.
	forever.Forever.Parameters = map[string]float64{foreverdata.ParamDualWieldMissPenalty: 0.19, foreverdata.ParamDodgeOffset: 0}
	got, _ = warriorAttackTable(t, forever)
	for field, value := range want {
		if got[field] != value {
			t.Errorf("%s: %v differs from Classic %v with default-valued parameters", field, got[field], value)
		}
	}
}

func TestAttackTableParametersChangeExactlyOneField(t *testing.T) {
	withoutOverrides(t)
	base, _ := warriorAttackTable(t, discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{}))
	tests := []struct {
		key, field  string
		value, want float64
	}{
		{foreverdata.ParamDualWieldMissPenalty, "DualWieldMissPenalty", 0.24, 0.24},
		{foreverdata.ParamDualWieldMissPenalty, "DualWieldMissPenalty", 0, 0},
		{foreverdata.ParamMeleeMissOffset, "BaseMissChance", 0.01, base["BaseMissChance"] + 0.01},
		{foreverdata.ParamMeleeMissOffset, "BaseMissChance", -0.25, 0},
		{foreverdata.ParamDodgeOffset, "BaseDodgeChance", -0.02, base["BaseDodgeChance"] - 0.02},
		{foreverdata.ParamParryOffset, "BaseParryChance", 0.03, base["BaseParryChance"] + 0.03},
		{foreverdata.ParamGlancingChanceOffset, "BaseGlanceChance", -0.15, base["BaseGlanceChance"] - 0.15},
		{foreverdata.ParamMeleeCritSuppressionOffset, "MeleeCritSuppression", -0.018, base["MeleeCritSuppression"] - 0.018},
		{foreverdata.ParamMeleeCritSuppressionOffset, "MeleeCritSuppression", -0.25, 0},
		{foreverdata.ParamSpellMissOffset, "BaseSpellMissChance", 0.05, base["BaseSpellMissChance"] + 0.05},
		{foreverdata.ParamSpellMissOffset, "BaseSpellMissChance", -0.25, 0},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s=%v", tc.key, tc.value), func(t *testing.T) {
			p := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{})
			p.Forever.Parameters = map[string]float64{tc.key: tc.value}
			if err := foreverdata.Validate(p); err != nil {
				t.Fatal(err)
			}
			got, _ := warriorAttackTable(t, p)
			for field, value := range base {
				if field == tc.field {
					near(t, got[field], tc.want)
				} else if got[field] != value {
					t.Errorf("%s changed %s: %v -> %v", tc.key, field, value, got[field])
				}
			}
		})
	}
}

func TestAttackTableSpellMissKeepsFloorAndDualWieldPenaltyIsUsed(t *testing.T) {
	withoutOverrides(t)
	p := overrideMage(t)
	p.Forever.Parameters = map[string]float64{foreverdata.ParamSpellMissOffset: -0.25}
	sim, c := simulation(t, p, nil)
	at := c.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]
	near(t, c.GetSpell(core.ActionID{SpellID: 116}).SpellChanceToMiss(at), 0.01)

	p = overrideMage(t)
	p.Forever.Parameters = map[string]float64{foreverdata.ParamSpellMissOffset: 0.02}
	sim, c = simulation(t, p, nil)
	at = c.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]
	_, base := simulation(t, overrideMage(t), nil)
	baseAt := base.AttackTables[sim.Encounter.TargetUnits[0].UnitIndex][proto.CastType_CastTypeMainHand]
	near(t, c.GetSpell(core.ActionID{SpellID: 116}).SpellChanceToMiss(at)-base.GetSpell(core.ActionID{SpellID: 116}).SpellChanceToMiss(baseAt), 0.02)
}

func TestAttackTableParameterOverrideDefaultsAndClassicIgnoresThem(t *testing.T) {
	t.Cleanup(foreverdata.SetOverridesForTesting(attackTableOverrides(t, "")))
	classic := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{})
	classic.Forever = nil
	want, _ := warriorAttackTable(t, googleproto.Clone(classic).(*proto.Player))

	doc := attackTableOverrides(t, `"core.combat.dual_wield_miss_penalty":{"value":0.25,"provenance":{"beliefs":["belief:dw"]}},`+
		`"core.combat.dodge_offset":{"value":0.01,"provenance":{"beliefs":["belief:dodge"]}},`+
		`"core.combat.parry_offset":{"value":0.5,"provenance":{"beliefs":["belief:oob"]}}`)
	t.Cleanup(foreverdata.SetOverridesForTesting(doc))

	got, c := warriorAttackTable(t, classic)
	for field, value := range want {
		if got[field] != value {
			t.Errorf("Classic %s changed by attack table parameters: %v -> %v", field, value, got[field])
		}
	}
	if r := c.ForeverOverrideReport(); len(r.Applied)+len(r.Rejected) != 0 {
		t.Fatal("Classic character recorded override events")
	}

	got, c = warriorAttackTable(t, discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{}))
	near(t, got["DualWieldMissPenalty"], 0.25)
	near(t, got["BaseDodgeChance"], want["BaseDodgeChance"]+0.01)
	near(t, got["BaseParryChance"], want["BaseParryChance"])
	applied := c.ForeverOverrideReport().Applied
	if findEvent(applied, foreverdata.ParamDualWieldMissPenalty, "value") == nil || findEvent(applied, foreverdata.ParamDodgeOffset, "value") == nil {
		t.Fatalf("attack table parameter defaults not recorded: %+v", applied)
	}
	if findEvent(doc.Rejected(), foreverdata.ParamParryOffset, "value") == nil {
		t.Fatalf("out-of-bounds override default not rejected at load: %+v", doc.Rejected())
	}
}

func TestAttackTableParameterBounds(t *testing.T) {
	keys := append([]string{foreverdata.ParamDualWieldMissPenalty}, foreverdata.AttackTableOffsetParams...)
	for _, key := range keys {
		found := false
		for _, spec := range foreverdata.ParameterCatalog() {
			found = found || spec.Key == key
		}
		if !found {
			t.Error("missing catalog key", key)
		}
	}
	for _, tc := range []struct {
		key   string
		value float64
		ok    bool
	}{
		{foreverdata.ParamDualWieldMissPenalty, 0, true}, {foreverdata.ParamDualWieldMissPenalty, 0.5, true},
		{foreverdata.ParamDualWieldMissPenalty, -0.01, false}, {foreverdata.ParamDualWieldMissPenalty, 0.51, false},
	} {
		if foreverdata.ParameterInBounds(tc.key, tc.value) != tc.ok {
			t.Errorf("%s=%v in bounds != %v", tc.key, tc.value, tc.ok)
		}
	}
	for _, key := range foreverdata.AttackTableOffsetParams {
		if !foreverdata.ParameterInBounds(key, -0.25) || !foreverdata.ParameterInBounds(key, 0.25) || !foreverdata.ParameterInBounds(key, 0) {
			t.Errorf("%s rejects values inside bounds", key)
		}
		if foreverdata.ParameterInBounds(key, -0.26) || foreverdata.ParameterInBounds(key, 0.26) {
			t.Errorf("%s accepts values outside bounds", key)
		}
		p := discoveryPlayer(t, "Warrior", "Human", "warrior", map[string]int32{})
		p.Forever.Parameters = map[string]float64{key: 0.3}
		if foreverdata.Validate(p) == nil {
			t.Errorf("request validation accepted %s=0.3", key)
		}
	}
}
