package core

import (
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/stats"
)

func foreverTestUnit(t *testing.T, doc string) *Unit {
	t.Helper()
	o, err := foreverdata.ParseOverrides([]byte(`{"version":"forever-overrides-1","generated_at":"2026-09-16T00:00:00Z","source_hash":"",` + doc + `}`))
	if err != nil {
		t.Fatal(err)
	}
	return &Unit{Label: "test", auraTracker: newAuraTracker(), PseudoStats: stats.NewPseudoStats(),
		foreverOverrides: &foreverOverrideState{overrides: o, diag: foreverdata.NewDiagnostics()}}
}

func TestForeverSpellOverrideDurationCooldownCost(t *testing.T) {
	unit := foreverTestUnit(t, `"spells":{
		"900":{"duration_ms":{"classic":18000,"forever":24000},"cooldown_ms":{"classic":10000,"forever":8000},"cost":{"classic":15,"forever":10,"power":"rage"},
		       "effects":{"0":{"base":{"classic":100,"forever":150},"kind":"periodic_damage"},"1":{"coefficient":{"classic":0.1,"forever":0.2},"kind":"periodic_damage"}}},
		"901":{"duration_ms":{"classic":18000,"forever":20000}},
		"902":{"effects":{"0":{"base":{"classic":10,"forever":20},"kind":"school_damage"}}},
		"903":{"duration_ms":{"classic":10000,"forever":12000}}}`)
	dotConfig := func(id int32) SpellConfig {
		return SpellConfig{ActionID: ActionID{SpellID: id}, SpellSchool: SpellSchoolShadow, DefenseType: DefenseTypeMagic, ProcMask: ProcMaskSpellDamage,
			RageCost: RageCostOptions{Cost: 15}, Cast: CastConfig{DefaultCast: Cast{GCD: GCDDefault}, CD: Cooldown{Timer: unit.NewTimer(), Duration: 10 * time.Second}},
			DamageMultiplier: 1, ThreatMultiplier: 1,
			Dot: DotConfig{IsAOE: true, Aura: Aura{Label: "dot" + ActionID{SpellID: id}.String()}, NumberOfTicks: 6, TickLength: 3 * time.Second, BonusCoefficient: 0.1}}
	}
	spell := unit.RegisterSpell(dotConfig(900))
	if dot := spell.AOEDot(); dot.NumberOfTicks != 8 || dot.TickLength != 3*time.Second || dot.BonusCoefficient != 0.2 {
		t.Fatalf("dot duration/coefficient not applied: ticks %d coef %v", dot.NumberOfTicks, dot.BonusCoefficient)
	}
	if spell.CD.Duration != 8*time.Second || spell.DefaultCast.Cost != 10 || spell.foreverPeriodicBaseRatio != 1.5 || spell.foreverImpactBaseRatio != 0 {
		t.Fatalf("cooldown/cost/base ratio not applied: %v %v %v", spell.CD.Duration, spell.DefaultCast.Cost, spell.foreverPeriodicBaseRatio)
	}

	// 20s is not a multiple of the 3s tick: rejected, ticks unchanged.
	if dot := unit.RegisterSpell(dotConfig(901)).AOEDot(); dot.NumberOfTicks != 6 {
		t.Fatal("non-divisible duration applied")
	}
	// Physical spells cannot separate base damage from weapon damage.
	melee := unit.RegisterSpell(SpellConfig{ActionID: ActionID{SpellID: 902}, SpellSchool: SpellSchoolPhysical, DefenseType: DefenseTypeMelee,
		ProcMask: ProcMaskMeleeMHSpecial, DamageMultiplier: 1, ThreatMultiplier: 1})
	if melee.foreverImpactBaseRatio != 0 {
		t.Fatal("base ratio applied to a melee spell")
	}
	// Aura durations use the same key for spells without a Dot.
	aura := unit.RegisterAura(Aura{Label: "buff", ActionID: ActionID{SpellID: 903}, Duration: 10 * time.Second})
	if aura.Duration != 12*time.Second {
		t.Fatal("aura duration not applied", aura.Duration)
	}

	report := unit.foreverOverrides.diag.Report()
	for _, want := range []string{"900 duration_ms", "900 cooldown_ms", "900 cost", "900 effects.0.base", "900 effects.1.coefficient", "903 duration_ms"} {
		parts := strings.SplitN(want, " ", 2)
		found := false
		for _, e := range report.Applied {
			found = found || (e.ID == parts[0] && e.Field == parts[1])
		}
		if !found {
			t.Errorf("missing applied event %s in %+v", want, report.Applied)
		}
	}
	rejected := map[string]bool{}
	for _, e := range report.Rejected {
		rejected[e.ID+" "+e.Field] = e.Reason != ""
	}
	if !rejected["901 duration_ms"] || !rejected["902 effects.0.base"] {
		t.Fatalf("expected rejections with reasons: %+v", report.Rejected)
	}
}

func TestForeverSpellValueRatio(t *testing.T) {
	unit := foreverTestUnit(t, `"spells":{
		"17628":{"effects":{"0":{"base":{"classic":149,"forever":199},"kind":"apply_aura"}}},
		"20217":{"effects":{"0":{"base":{"classic":9,"forever":14},"kind":"apply_aura"}}},
		"16609":{"effects":{"0":{"base":{"classic":null,"forever":400},"kind":"apply_aura"}}}}`)
	if v := unit.ForeverSpellValue(17628, 0, 150); v != 150*199.0/149 {
		t.Fatal(v)
	}
	if v := foreverPercentMultiplier(unit, 20217, 0, 1.10); v != 1+10*14.0/9/100 {
		t.Fatal(v)
	}
	if v := unit.ForeverSpellValue(16609, 0, 300); v != 300 {
		t.Fatal("null classic applied", v)
	}
	classic := &Unit{}
	if classic.ForeverSpellValue(17628, 0, 150) != 150 || foreverPercentMultiplier(classic, 20217, 0, 1.10) != 1.10 {
		t.Fatal("Classic unit changed")
	}
	if ForeverBuffStats(classic, MarkOfTheWild) != BuffSpellValues[MarkOfTheWild] {
		t.Fatal("Classic buff table changed")
	}
	r := unit.foreverOverrides.diag.Report()
	if len(r.Applied) != 2 || len(r.Rejected) != 1 {
		t.Fatalf("%+v", r)
	}
}
