package foreverdata

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/wowsims/classic/sim/core/proto"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestEmbeddedOverridesAreValid(t *testing.T) {
	raw, err := files.ReadFile("overrides.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseOverrides(raw); err != nil {
		t.Fatal(err)
	}
	if info := OverridesInfo(); info.Version != OverridesVersion {
		t.Fatal(info)
	}
}

func TestValidOverridesFixture(t *testing.T) {
	o, err := ParseOverrides(loadFixture(t, "overrides_valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	s := o.Summary()
	if s.Counts.Spells != 3 || s.Counts.SpellEffects != 3 || s.Counts.Items != 2 || s.Counts.NewItems != 1 || s.Counts.Talents != 1 || s.Counts.Parameters != 1 {
		t.Fatalf("unexpected counts %+v", s.Counts)
	}
	if s.Rejected.Talents != 1 || s.Rejected.Parameters != 1 || len(s.InitRejects) != 2 {
		t.Fatalf("expected talent shape and parameter bound rejections: %+v", s.InitRejects)
	}
	if s.SourceHash == "" || s.GeneratedAt != "2026-09-16T12:00:00Z" {
		t.Fatal(s)
	}
	spell, ok := o.Spell(116)
	if !ok || *spell.Effects["0"].Coefficient.Forever != 0.5 || spell.Cost.Power != "mana" {
		t.Fatal("spell 116 not indexed")
	}
	item, ok := o.Item(990002)
	if !ok || item.NewItemProto() == nil || item.NewItemProto().Name != "Forever Test Circlet" || item.NewItemProto().Type != proto.ItemType_ItemTypeHead {
		t.Fatal("new_item not parsed")
	}
	if _, ok := o.Parameter("maelstrom_proc_chance"); ok {
		t.Fatal("out-of-bounds parameter accepted")
	}
}

func TestInvalidOverridesRejected(t *testing.T) {
	if _, err := ParseOverrides(loadFixture(t, "overrides_invalid.json")); err == nil || !strings.Contains(err.Error(), `unknown top-level key "classes"`) {
		t.Fatalf("invalid fixture accepted or unclear error: %v", err)
	}
	valid := string(loadFixture(t, "overrides_valid.json"))
	cases := map[string]string{
		"version":        strings.Replace(valid, `"forever-overrides-1"`, `"forever-overrides-2"`, 1),
		"generated_at":   strings.Replace(valid, `2026-09-16T12:00:00Z`, `yesterday`, 1),
		"kind":           strings.Replace(valid, `"kind": "school_damage"`, `"kind": "fire_damage"`, 1),
		"power":          strings.Replace(valid, `"power": "mana"`, `"power": "focus"`, 1),
		"stat key":       strings.Replace(valid, `"strength": {"classic": 10`, `"might": {"classic": 10`, 1),
		"nested field":   strings.Replace(valid, `"cast_ms":`, `"gcd_ms": {"classic": 1, "forever": 1}, "cast_ms":`, 1),
		"missing value":  strings.Replace(valid, `"forever": 1000`, `"forever": null`, 1),
		"spell id":       strings.Replace(valid, `"116": {`, `"frostbolt": {`, 1),
		"new_item field": strings.Replace(valid, `"quality": 4`, `"quality": 4, "bogus": 1`, 1),
	}
	for name, doc := range cases {
		if doc == valid {
			t.Fatalf("%s: fixture replacement failed", name)
		}
		if _, err := ParseOverrides([]byte(doc)); err == nil {
			t.Errorf("%s: invalid document accepted", name)
		}
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "invalid embedded overrides.json") {
			t.Fatalf("expected init-style panic, got %v", r)
		}
	}()
	mustParseOverrides(loadFixture(t, "overrides_invalid.json"))
}

func TestTalentOverrideLookup(t *testing.T) {
	o, err := ParseOverrides(loadFixture(t, "overrides_valid.json"))
	if err != nil {
		t.Fatal(err)
	}
	before, _ := Lookup("warrior.talent.anticipation")
	restore := SetOverridesForTesting(o)
	after, _ := Lookup("warrior.talent.anticipation")
	focus, _ := Lookup("mage.talent.arcane-focus")
	restore()
	again, _ := Lookup("warrior.talent.anticipation")
	if before.Ranks[0].Values[0] != 4 || after.Ranks[0].Values[0] != 5 || after.Ranks[4].Values[0] != 25 || again.Ranks[0].Values[0] != 4 {
		t.Fatal("talent override not applied/restored", before.Ranks[0], after.Ranks[0], again.Ranks[0])
	}
	if focus.Ranks[0].Values[0] != 1 || len(focus.Ranks) != 5 {
		t.Fatal("mis-shaped talent override was applied")
	}
}

func TestParameterCatalogAndStatKeys(t *testing.T) {
	if len(StatKeys) != len(proto.Stat_name) {
		t.Fatalf("StatKeys has %d entries, proto.Stat has %d", len(StatKeys), len(proto.Stat_name))
	}
	for i, key := range StatKeys {
		name := strings.ToLower(strings.TrimPrefix(proto.Stat_name[int32(i)], "Stat"))
		if strings.ReplaceAll(key, "_", "") != name {
			t.Errorf("stat key %s does not match %s", key, proto.Stat_name[int32(i)])
		}
	}
	for _, key := range []string{ParamSpellCritDamageMultiplier, ParamMeleeCritDamageMultiplier, ParamGlancingDamageMultiplier,
		ConversionParam("warrior", "agility_per_melee_crit"), ConversionParam("mage", "intellect_per_spell_crit"),
		ConversionParam("rogue", "attack_power_per_agility"), ConversionParam("druid", "attack_power_per_strength")} {
		found := false
		for _, p := range ParameterCatalog() {
			found = found || p.Key == key
		}
		if !found {
			t.Error("missing catalog key", key)
		}
	}
	if ParameterInBounds(ConversionParam("mage", "agility_per_melee_crit"), 20) {
		t.Error("mage does not register an agility->crit conversion")
	}
	if len(SupportedOverrideKeys()) == 0 {
		t.Fatal("empty supported key catalog")
	}
}

// Every Unit.ForeverSpellValue call site with literal ids must be listed in SpellValueHooks,
// so the exporter's SupportedOverrideKeys routing matches the simulator.
func TestSpellValueHookCatalogMatchesCallSites(t *testing.T) {
	call := regexp.MustCompile(`ForeverSpellValue\((\d+), (\d+),`)
	percent := regexp.MustCompile(`foreverPercentMultiplier\([^,]+, (\d+), (\d+),`)
	buffTable := regexp.MustCompile(`return (?:TernaryInt32\(IncludeAQ, )?(\d+)(?:, (\d+)\))? \}, map\[stats\.Stat\]int\{([^}]*)`)
	found := 0
	for _, file := range []string{"../consumes.go", "../buffs.go"} {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		for _, re := range []*regexp.Regexp{call, percent} {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				id, _ := strconv.Atoi(m[1])
				idx, _ := strconv.Atoi(m[2])
				found++
				if !HasSpellValueHook(int32(id), idx) {
					t.Errorf("%s: ForeverSpellValue(%d, %d) missing from SpellValueHooks", file, id, idx)
				}
			}
		}
		for _, m := range buffTable.FindAllStringSubmatch(src, -1) {
			for _, idText := range []string{m[1], m[2]} {
				if idText == "" {
					continue
				}
				id, _ := strconv.Atoi(idText)
				for _, idx := range regexp.MustCompile(`: (\d+)`).FindAllStringSubmatch(m[3], -1) {
					i, _ := strconv.Atoi(idx[1])
					found++
					if !HasSpellValueHook(int32(id), i) {
						t.Errorf("%s: buff table (%d, %d) missing from SpellValueHooks", file, id, i)
					}
				}
			}
		}
	}
	if found < 80 {
		t.Fatalf("only %d call sites found; regex out of date", found)
	}
}

func TestDiagnosticsDeduplicateAndSplit(t *testing.T) {
	d := NewDiagnostics()
	d.Record(OverrideEvent{Scope: ScopeSpell, ID: "1", Field: "cast_ms", Applied: true})
	d.Record(OverrideEvent{Scope: ScopeSpell, ID: "1", Field: "cast_ms", Applied: true})
	d.Record(OverrideEvent{Scope: ScopeSpell, ID: "1", Field: "cost", Reason: "mismatch"})
	r := d.Report()
	if len(r.Applied) != 1 || len(r.Rejected) != 1 || !d.Seen(ScopeSpell, "1", "cost", "", "") {
		t.Fatal(r)
	}
}

func TestSetActiveOverrides(t *testing.T) {
	t.Cleanup(SetOverridesForTesting(ActiveOverrides()))
	embedded := OverridesInfo()
	valid := loadFixture(t, "overrides_valid.json")

	summary, err := SetActiveOverrides(valid)
	if err != nil {
		t.Fatal(err)
	}
	active := OverridesInfo()
	if summary.SourceHash != "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08" || active.SourceHash != summary.SourceHash ||
		active.Counts != summary.Counts || summary.Counts.Spells != 3 || summary.Rejected.Parameters != 1 {
		t.Fatalf("valid document was not swapped in: %+v (embedded %+v)", active, embedded)
	}
	if r, _ := Lookup("warrior.talent.anticipation"); r.Ranks[0].Values[0] != 5 {
		t.Fatal("Lookup does not read the swapped document")
	}

	// Any validation error leaves the previous document active.
	for _, raw := range [][]byte{loadFixture(t, "overrides_invalid.json"), []byte("not json"), nil,
		[]byte(strings.Replace(string(valid), `"forever-overrides-1"`, `"forever-overrides-2"`, 1))} {
		if _, err := SetActiveOverrides(raw); err == nil {
			t.Fatal("invalid document accepted")
		}
		if OverridesInfo().SourceHash != summary.SourceHash {
			t.Fatal("invalid document replaced the active one")
		}
	}
}

func TestSetActiveOverridesConcurrent(t *testing.T) {
	t.Cleanup(SetOverridesForTesting(ActiveOverrides()))
	valid := loadFixture(t, "overrides_valid.json")
	empty := []byte(`{"version":"forever-overrides-1","generated_at":"2026-01-01T00:00:00Z","source_hash":"","spells":{},"items":{},"talents":{},"parameters":{}}`)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				raw := valid
				if (i+j)%2 == 0 {
					raw = empty
				}
				if _, err := SetActiveOverrides(raw); err != nil {
					t.Error(err)
				}
				// Readers always see one complete document.
				if s := OverridesInfo(); s.Counts.Spells != 0 && s.Counts.Spells != 3 {
					t.Errorf("torn document: %+v", s.Counts)
				}
				Lookup("warrior.talent.anticipation")
			}
		}(i)
	}
	wg.Wait()
}
