package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
)

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	if code := run(args, &out, io.Discard); code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, out.String())
	}
	return out.Bytes()
}

func testBench(t *testing.T) *bench {
	t.Helper()
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	fs, c := newFlagSet("test", io.Discard)
	if err := c.parse(fs, []string{"-iterations", "20", "-ab-iterations", "10"}); err != nil {
		t.Fatal(err)
	}
	b, _, err := c.newBench()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPresetMatrixAllPhases(t *testing.T) {
	m, err := presetMatrix()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != len(variants)*len(allPhases()) {
		t.Fatalf("matrix has %d entries, want %d", len(m), len(variants)*len(allPhases()))
	}
	okPerPhase := map[string]int{}
	for _, e := range m {
		switch e.Status {
		case "ok":
			okPerPhase[e.Phase]++
			if (e.GearStatus != "ok" && !strings.HasPrefix(e.GearStatus, "borrowed:")) || e.GearFile == "" || strings.Contains(strings.ToLower(e.GearFile), "blank") {
				t.Errorf("%s %s: ok entry with bad gear %q (%s)", e.Spec, e.Phase, e.GearFile, e.GearStatus)
			}
			if _, found := presetFile(e.GearFile); !found {
				t.Errorf("%s %s: gear file %s not in snapshot", e.Spec, e.Phase, e.GearFile)
			}
			if e.FilledSlots < 15 || e.AvgIlvl < 50 {
				t.Errorf("%s %s: gear looks empty (%d slots, ilvl %.1f)", e.Spec, e.Phase, e.FilledSlots, e.AvgIlvl)
			}
			if (e.TalentsString == "" && e.variant.Talents != "") || e.APLFile == "" {
				t.Errorf("%s %s: missing talents or APL", e.Spec, e.Phase)
			}
			if _, found := presetFile(e.APLFile); !found {
				t.Errorf("%s %s: APL file %s not found", e.Spec, e.Phase, e.APLFile)
			}
		case "missing_gear", "incomplete_gear":
			if (e.GearStatus != "missing" && e.GearStatus != "incomplete") || e.GearFile != "" {
				t.Errorf("%s %s: missing entry carries gear %q", e.Spec, e.Phase, e.GearFile)
			}
		case "unavailable":
		default:
			t.Errorf("%s %s: unknown status %q", e.Spec, e.Phase, e.Status)
		}
	}
	for _, p := range allPhases() {
		if okPerPhase[p.String()] == 0 {
			t.Errorf("phase %s has no simmable spec", p)
		}
	}
	// Spot checks against the UI presets.
	for spec, phase := range map[string]string{"warrior-fury": "P6", "shaman-elemental": "P3", "mage-frost": "P1", "rogue-combat-daggers": "P2"} {
		p, _ := parsePhase(phase)
		e, err := matrixEntry(spec, p)
		if err != nil || e.Status != "ok" {
			t.Errorf("%s %s should have gear: %+v %v", spec, phase, e, err)
		}
	}
	if e, _ := matrixEntry("paladin-retribution", 1); e.Status != "ok" || e.GearStatus != "borrowed:warrior-fury" || e.OwnGearStatus != "missing" {
		t.Errorf("retribution only has blank gear and must borrow the fury set, got %s %s", e.Status, e.GearStatus)
	}
	if e, _ := matrixEntry("priest-shadow", 1); e.Status != "ok" || e.GearStatus != "borrowed:warlock-affliction" || e.OwnGearStatus != "incomplete" {
		t.Errorf("shadow priest P1 is a lone tier set and must borrow the warlock set, got %s %s", e.Status, e.GearStatus)
	}
	if e, _ := matrixEntry("druid-feral-cat", 1); e.Status != "incomplete_gear" {
		t.Errorf("feral P1 has no donor and must stay incomplete_gear, got %s", e.Status)
	}
	if e, _ := matrixEntry("mage-fire", 6); e.Status != "missing_gear" {
		t.Errorf("fire mage P6 has no donor set and must be missing_gear, got %s", e.Status)
	}
}

func TestPresetSnapshotMatchesRepo(t *testing.T) {
	root, err := resolveRoot("")
	if err != nil {
		t.Skip("repo root not found")
	}
	snap, err := uiPresets()
	if err != nil {
		t.Fatal(err)
	}
	for rel, raw := range snap.Files {
		data, err := os.ReadFile(filepath.Join(root, "ui", filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("snapshot file %s: %v (rerun node cmd/foreverbench/extract_presets.mjs)", rel, err)
			continue
		}
		var a, b interface{}
		if json.Unmarshal(raw, &a) != nil || json.Unmarshal(data, &b) != nil || !reflect.DeepEqual(a, b) {
			t.Errorf("snapshot of ui/%s is stale; rerun node cmd/foreverbench/extract_presets.mjs", rel)
		}
	}
	for dir, spec := range snap.Specs {
		entries, _ := os.ReadDir(filepath.Join(root, "ui", dir, "gear_sets"))
		if len(entries) != len(spec.GearSetFiles) {
			t.Errorf("ui/%s/gear_sets changed (%d files, snapshot %d); rerun extract_presets.mjs", dir, len(entries), len(spec.GearSetFiles))
		}
	}
}

func TestClassifyGear(t *testing.T) {
	cases := map[string]struct {
		phase   Phase
		ok, alt bool
	}{
		"P1 BiS": {1, true, false}, "Phase 4": {4, true, false}, "Pre-BiS": {0, true, false}, "Pre-BIS": {0, true, false},
		"MC": {1, true, false}, "Blank": {0, false, false}, "P2 Pre-BiS": {0, false, true}, "Backstab P2 BiS": {2, true, false},
		"Sinister Strike Pre-BiS": {0, true, false}, "feral_druid/gear_sets/p1.bis.gear.json": {1, true, false},
		"rogue/gear_sets/combat_backstab_p1_bis.gear.json": {1, true, false}, "warrior/gear_sets/phase_6.gear.json": {6, true, false},
	}
	for name, want := range cases {
		p, ok, alt := classifyGear(name)
		if ok != want.ok || alt != want.alt || (ok && p != want.phase) {
			t.Errorf("classifyGear(%q) = %v %v %v, want %+v", name, p, ok, alt, want)
		}
	}
}

// Everything except the spec (gear, talents, APL, spec options, archetype
// slot consumables, race, distance) must be identical across specs.
func TestNormalizationIdenticalAcrossSpecs(t *testing.T) {
	b := testBench(t)
	entries, _, err := selectEntries(1, "dps", "", "none")
	if err != nil {
		t.Fatal(err)
	}
	enc := EncounterSpec{Targets: 3, Duration: 120}
	var ref *proto.RaidSimRequest
	elixirs := func(c *proto.Consumes) []interface{} {
		return []interface{}{c.AgilityElixir, c.StrengthBuff, c.AttackPowerBuff, c.SpellPowerBuff, c.FirePowerBuff, c.FrostPowerBuff, c.ShadowPowerBuff}
	}
	for _, e := range entries {
		req, err := b.classicRequest(e, e.variant.FixedRace, enc, 20)
		if err != nil {
			t.Fatalf("%s: %v", e.Spec, err)
		}
		p := req.Raid.Parties[0].Players[0]
		if ref == nil {
			ref = req
			continue
		}
		rp := ref.Raid.Parties[0].Players[0]
		checks := []struct {
			name string
			a, b googleProto.Message
		}{
			{"raid buffs", req.Raid.Buffs, ref.Raid.Buffs},
			{"debuffs", req.Raid.Debuffs, ref.Raid.Debuffs},
			{"party buffs", req.Raid.Parties[0].Buffs, ref.Raid.Parties[0].Buffs},
			{"individual buffs", p.Buffs, rp.Buffs},
			{"encounter", req.Encounter, ref.Encounter},
			{"sim options", req.SimOptions, ref.SimOptions},
		}
		for _, ch := range checks {
			if !googleProto.Equal(ch.a, ch.b) {
				t.Errorf("%s: %s differ from %s", e.Spec, ch.name, ref.Raid.Parties[0].Players[0].Name)
			}
		}
		if p.Profession1 != rp.Profession1 || p.Profession2 != rp.Profession2 || p.ReactionTimeMs != rp.ReactionTimeMs || p.ChannelClipDelayMs != rp.ChannelClipDelayMs {
			t.Errorf("%s: player normalization differs", e.Spec)
		}
		if !reflect.DeepEqual(elixirs(p.Consumes), elixirs(rp.Consumes)) {
			t.Errorf("%s: elixir set differs", e.Spec)
		}
		if p.Consumes.Flask == proto.Flask_FlaskUnknown || p.Consumes.Food == proto.Food_FoodUnknown || p.Consumes.MainHandImbue == proto.WeaponImbue_WeaponImbueUnknown {
			t.Errorf("%s: empty consumable slot %v", e.Spec, p.Consumes)
		}
		if p.Buffs.SongflowerSerenade || p.Buffs.RallyingCryOfTheDragonslayer {
			t.Errorf("%s: world buffs present with -world-buffs off", e.Spec)
		}
		if len(req.Encounter.Targets) != 3 || req.Encounter.Targets[0].Level != 63 {
			t.Errorf("%s: encounter targets %v", e.Spec, req.Encounter.Targets)
		}
	}
}

func TestParseEncounterMix(t *testing.T) {
	name, mix, err := parseEncounterMix("first-raid", os.ReadFile)
	if err != nil || name != "first-raid" || len(mix) != 3 {
		t.Fatalf("first-raid: %v %v %v", name, mix, err)
	}
	_, mix, err = parseEncounterMix(`[{"name":"a","weight":3,"targets":1,"duration_s":100},{"weight":1,"targets":4,"duration_s":50}]`, os.ReadFile)
	if err != nil || mix[0].Weight != 0.75 || mix[1].Name == "" {
		t.Fatalf("inline mix: %+v %v", mix, err)
	}
	if _, _, err := parseEncounterMix(`[{"weight":1,"targets":0,"duration_s":50}]`, os.ReadFile); err == nil {
		t.Fatal("invalid mix accepted")
	}
}

func TestList(t *testing.T) {
	var specs []VariantListing
	if err := json.Unmarshal(runCLI(t, "list"), &specs); err != nil {
		t.Fatal(err)
	}
	if len(specs) != len(variants) {
		t.Fatalf("listed %d specs, want %d", len(specs), len(variants))
	}
	for _, s := range specs {
		v, _ := findVariant(s.ID)
		if s.Status == "available" && ((s.ClassicTalents == "" && v.Talents != "") || s.FixedRace == "" || len(s.GearPhases) == 0) {
			t.Fatalf("bad listing %+v", s)
		}
	}
}

// Every derived Forever build must pass the engine's own validation, and F1
// strings must round-trip; fixed races must be legal in both games.
func TestForeverTalentPoliciesAndRaces(t *testing.T) {
	b := testBench(t)
	for _, v := range variants {
		if v.Unavailable != "" {
			continue
		}
		e, err := matrixEntry(v.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if !b.d.raceAllowed(v.Class, v.FixedRace, true) {
			t.Errorf("%s: fixed race %s not legal in both games", v.ID, raceName(v.FixedRace))
		}
		if e = b.resolveTalents(e); e.TalentsString == "" {
			continue
		}
		for _, name := range []string{"repair", "repair-nofill", "migrate", "sample"} {
			p := &proto.Player{Class: v.Class, Race: v.FixedRace, TalentsString: e.TalentsString}
			if err := setSpecOptions(p, v.OneofField, e.specOptions); err != nil {
				t.Fatal(err)
			}
			info := &ForeverPlayerInfo{}
			talents := b.d.talentsFor(p, TalentPolicy{Name: name}, true, info)
			f := b.d.defaultOptions(v.Class, v.FixedRace, proto.ForeverMode_BEST_GUESS)
			f.Talents = talents
			fp := &proto.Player{Class: v.Class, Race: v.FixedRace, Spec: p.Spec, Forever: f}
			if err := foreverdata.Validate(fp); err != nil {
				t.Fatalf("%s/%s: %v", v.ID, name, err)
			}
			if name == "repair" && sumPoints(talents) != 51 {
				t.Errorf("%s/repair placed %d points", v.ID, sumPoints(talents))
			}
			decoded, err := b.d.decodeTalents(v.Class, b.d.encodeTalents(v.Class, talents))
			if err != nil || sumPoints(decoded) != sumPoints(talents) {
				t.Fatalf("%s/%s: F1 round trip failed: %v", v.ID, name, err)
			}
		}
	}
}

type rankOut struct {
	Ranking  []RankEntry     `json:"ranking"`
	Excluded []ExcludedEntry `json:"excluded"`
	Failed   int             `json:"failed"`
	Norm     json.RawMessage `json:"normalization"`
	Prov     map[string]any  `json:"provenance"`
	Elapsed  int64           `json:"elapsed_ms"`
}

func TestRankP1Both(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	var out rankOut
	raw := runCLI(t, "rank", "-phase", "P1", "-game", "both", "-iterations", "30", "-ab-iterations", "20", "-compact-json")
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Failed != 0 {
		t.Fatalf("failed specs: %s", raw)
	}
	if len(out.Ranking) < 10 || out.Prov["manifest_sha256"] == "" || len(out.Norm) == 0 {
		t.Fatalf("unexpected output: %s", raw)
	}
	for i, e := range out.Ranking {
		if (e.GearStatus != "ok" && !strings.HasPrefix(e.GearStatus, "borrowed:")) || strings.Contains(e.GearFile, "blank") {
			t.Errorf("%s ranked with gear %s (%s)", e.Spec, e.GearFile, e.GearStatus)
		}
		if e.DPS <= 0 || e.ClassicDPS == nil || e.DeltaPct == nil || e.TalentSource == "" || e.AutoRotation == nil || e.Rank != i+1 {
			t.Errorf("incomplete entry %+v", e)
		}
		if e.CI95[0] > e.DPS || e.CI95[1] < e.DPS {
			t.Errorf("%s: ci95 %v does not contain %v", e.Spec, e.CI95, e.DPS)
		}
		if i > 0 && e.DPS > out.Ranking[i-1].DPS {
			t.Errorf("ranking not sorted at %d", i)
		}
	}
	ids := map[string]string{}
	for _, e := range out.Excluded {
		ids[e.Spec] = e.Status
		if e.Status == "ok" {
			t.Errorf("%s excluded with status ok", e.Spec)
		}
	}
	if ids["druid-feral-cat"] != "incomplete_gear" {
		t.Errorf("feral cat (incomplete P1 gear, no donor) must be excluded: %+v", out.Excluded)
	}
	ranked := map[string]RankEntry{}
	for _, e := range out.Ranking {
		ranked[e.Spec] = e
	}
	if r, ok := ranked["paladin-retribution"]; !ok || r.GearStatus != "borrowed:warrior-fury" || r.GearBorrowed != "warrior-fury" {
		t.Errorf("retribution must be ranked with borrowed fury gear: %+v", r)
	}
}

func TestDeltaAndMeta(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	specs := "mage-frost,warrior-fury,warlock-affliction"
	var delta struct {
		Ranking    []RankEntry      `json:"ranking"`
		MostBuffed []map[string]any `json:"most_buffed"`
		Failed     int              `json:"failed"`
	}
	raw := runCLI(t, "delta", "-phase", "P1", "-specs", specs, "-iterations", "20", "-ab-iterations", "10", "-compact-json")
	if err := json.Unmarshal(raw, &delta); err != nil || delta.Failed != 0 || len(delta.Ranking) != 3 || len(delta.MostBuffed) != 3 {
		t.Fatalf("delta: %v %s", err, raw)
	}
	for i, e := range delta.Ranking {
		if len(e.AbilityDelta) == 0 || (i > 0 && *e.DeltaPct > *delta.Ranking[i-1].DeltaPct) {
			t.Errorf("delta entry %d: %+v", i, e)
		}
	}
	var meta struct {
		Ranking []MetaEntry         `json:"ranking"`
		Tiers   map[string][]string `json:"tiers"`
		Caveats []string            `json:"caveats"`
		Mix     struct {
			Encounters []EncounterSpec `json:"encounters"`
		} `json:"encounter_mix"`
	}
	raw = runCLI(t, "meta", "-phase", "P1", "-specs", specs, "-iterations", "10", "-ab-iterations", "10", "-compact-json")
	if err := json.Unmarshal(raw, &meta); err != nil || len(meta.Ranking) != 3 || len(meta.Mix.Encounters) != 3 || len(meta.Caveats) == 0 {
		t.Fatalf("meta: %v %s", err, raw)
	}
	if meta.Ranking[0].Tier != "S" || meta.Ranking[0].RelScore != 100 || len(meta.Ranking[0].Encounters) != 3 {
		t.Fatalf("meta top entry: %+v", meta.Ranking[0])
	}
}

func TestPhaseWithoutGearIsRejectedForCompare(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	var out bytes.Buffer
	if code := run([]string{"compare", "-phase", "P6", "-spec", "mage-frost", "-iterations", "10"}, &out, io.Discard); code != 1 || !strings.Contains(out.String(), "no simmable preset") {
		t.Fatalf("expected missing-gear error, got %d %s", code, out.String())
	}
}

func TestTierBandsAndEmptyRole(t *testing.T) {
	bands, err := parseTierBands("S:90,A:75,B:60")
	if err != nil || len(bands) != 4 || bands[3].Tier != "C" || bands[1].Min != 75 {
		t.Fatalf("bands: %+v %v", bands, err)
	}
	if _, err := parseTierBands("S:60,A:75"); err == nil {
		t.Fatal("ascending bands accepted")
	}
	if !core.WITH_DB {
		return
	}
	// No tank has complete P1 gear: the command must still answer with the exclusions.
	var out rankOut
	if err := json.Unmarshal(runCLI(t, "rank", "-phase", "P1", "-role", "tank", "-iterations", "10", "-compact-json"), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Ranking) != 0 || len(out.Excluded) == 0 {
		t.Fatalf("tank P1: %+v", out)
	}
}
