package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
)

func TestClassicTreesMatchRepo(t *testing.T) {
	root, err := resolveRoot("")
	if err != nil {
		t.Skip("repo root not found")
	}
	var file struct {
		Classes map[string][]uiTalentTree `json:"classes"`
	}
	if err := json.Unmarshal(classicTreesJSON, &file); err != nil {
		t.Fatal(err)
	}
	for class, trees := range file.Classes {
		raw, err := os.ReadFile(filepath.Join(root, "ui", "core", "talents", "trees", class+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var repo []uiTalentTree
		if err := json.Unmarshal(raw, &repo); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(trees, repo) {
			t.Errorf("presets/classic_talent_trees.json is stale for %s (copy ui/core/talents/trees/%s.json)", class, class)
		}
	}
}

// Every UI Classic preset and every Forever repair is legal under the models,
// and the Classic layout order matches the Forever dataset's classic_layout.
func TestTalentModels(t *testing.T) {
	b := testBench(t)
	for _, v := range variants {
		cm, err := classicTalentModel(v.Class)
		if err != nil {
			t.Fatal(err)
		}
		fm := foreverTalentModel(b.d, v.Class, proto.ForeverMode_BEST_GUESS)
		if _, err := constraintsFor(cm, v); err != nil {
			t.Error(err)
		}
		if _, err := constraintsFor(fm, v); err != nil {
			t.Error(err)
		}
		e, _ := matrixEntry(v.ID, 1)
		if e.TalentsString == "" || v.Role != "dps" {
			// (the feral tank UI preset uses a longer, non-Classic tree layout)
			continue
		}
		s, err := cm.decode(e.TalentsString)
		if err != nil || cm.legal(s) != nil {
			t.Errorf("%s: UI Classic preset %s not legal: %v", v.ID, e.TalentsString, err)
			continue
		}
		if cm.encode(s) != trimTrees(e.TalentsString) {
			t.Errorf("%s: Classic round trip %s -> %s", v.ID, e.TalentsString, cm.encode(s))
		}
		repaired, _ := b.d.repairClassicTalents(v.Class, e.TalentsString, true)
		fs, err := fm.fromMap(repaired)
		if err != nil || fm.validateEngine(fs, v, v.FixedRace, proto.ForeverMode_BEST_GUESS) != nil {
			t.Errorf("%s: repaired Forever build not legal: %v", v.ID, err)
		}
	}
	for class, layout := range b.d.ClassicLayout {
		for _, v := range variants {
			if className := v.Class.String()[5:]; len(className) > 0 && (class == toUpper(className)) {
				cm, _ := classicTalentModel(v.Class)
				for t2, fields := range layout {
					for j, f := range fields {
						if cm.Nodes[cm.treeStart[t2]+j].ID != f {
							t.Fatalf("%s tree %d talent %d: UI tree %s, Forever classic_layout %s", class, t2, j, cm.Nodes[cm.treeStart[t2]+j].ID, f)
						}
					}
				}
				break
			}
		}
	}
}

func toUpper(s string) string {
	out := []byte(s)
	for i, c := range out {
		if c >= 'a' && c <= 'z' {
			out[i] = c - 32
		}
	}
	return string(out)
}

func trimTrees(s string) string {
	out := []byte{}
	parts := []string{}
	cur := []byte{}
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == '-' {
			for len(cur) > 0 && cur[len(cur)-1] == '0' {
				cur = cur[:len(cur)-1]
			}
			parts = append(parts, string(cur))
			cur = []byte{}
			continue
		}
		cur = append(cur, s[i])
	}
	for len(parts) < 3 {
		parts = append(parts, "")
	}
	for i, p := range parts {
		if i > 0 {
			out = append(out, '-')
		}
		out = append(out, p...)
	}
	return string(out)
}

// The optimizer returns legal builds that satisfy the spec identity and is
// deterministic for a fixed seed.
func TestOptimizerLegalAndDeterministic(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	if testing.Short() {
		t.Skip("slow")
	}
	for _, tc := range []struct{ spec, game string }{{"warlock-destruction", "classic"}, {"mage-fire", "forever"}} {
		results := []OptimizeResult{}
		for run := 0; run < 2; run++ {
			fs, c := newFlagSet("test", io.Discard)
			if err := c.parse(fs, []string{"-iterations", "8", "-seed", "11"}); err != nil {
				t.Fatal(err)
			}
			b, _, err := c.newBench()
			if err != nil {
				t.Fatal(err)
			}
			res, err := b.optimize(tc.spec, optimizeOptions{game: tc.game, phase: 1, base: 8, enc: EncounterSpec{Targets: 1, Duration: 60}, parallel: 8, budget: 10 * time.Minute})
			if err != nil || res.Error != "" {
				t.Fatalf("%s %s: %v %s", tc.spec, tc.game, err, res.Error)
			}
			v, _ := findVariant(tc.spec)
			var m *talentModel
			if tc.game == "forever" {
				m = foreverTalentModel(b.d, v.Class, b.mode)
			} else {
				m, _ = classicTalentModel(v.Class)
			}
			s, err := m.decode(res.Talents)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.validateEngine(s, v, v.FixedRace, b.mode); err != nil {
				t.Errorf("%s %s: illegal build %s: %v", tc.spec, tc.game, res.Talents, err)
			}
			cons, _ := constraintsFor(m, v)
			if !cons.satisfied(m, s) || m.total(s) > talentBudget || m.total(s) < 31 {
				t.Errorf("%s %s: build %s (%s) violates constraints %+v", tc.spec, tc.game, res.Talents, res.TreePoints, cons)
			}
			for i, n := range m.Nodes {
				if s[i] > 0 && n.Excluded != "" {
					t.Errorf("%s %s: not-simulated talent %s in build", tc.spec, tc.game, n.Name)
				}
			}
			if res.DPS <= 0 || res.Sims == 0 || len(res.Starts) == 0 {
				t.Errorf("%s %s: incomplete result %+v", tc.spec, tc.game, res)
			}
			results = append(results, res)
		}
		if results[0].Talents != results[1].Talents || results[0].DPS != results[1].DPS {
			t.Errorf("%s %s not deterministic: %s %.2f vs %s %.2f", tc.spec, tc.game, results[0].Talents, results[0].DPS, results[1].Talents, results[1].DPS)
		}
	}
}

// Embedded builds files load, decode, pass validation and satisfy the spec
// identity; phase selection and file merging behave.
func TestBuildsFiles(t *testing.T) {
	b := testBench(t)
	if len(b.builds) == 0 && len(b.classic) == 0 {
		t.Log("no embedded builds yet")
	}
	for _, list := range [][]TalentBuild{b.builds, b.classic} {
		for _, build := range list {
			v, _ := findVariant(build.Spec)
			var m *talentModel
			if build.Game == "forever" {
				m = foreverTalentModel(b.d, v.Class, b.mode)
			} else {
				m, _ = classicTalentModel(v.Class)
			}
			s, err := m.decode(build.Talents)
			if err != nil {
				t.Errorf("%s %s: %v", build.Game, build.Spec, err)
				continue
			}
			if err := m.validateEngine(s, v, v.FixedRace, b.mode); err != nil {
				t.Errorf("%s %s build %s illegal: %v", build.Game, build.Spec, build.Talents, err)
			}
			cons, _ := constraintsFor(m, v)
			if build.optimized() && !cons.satisfied(m, s) {
				t.Errorf("%s %s build %s violates constraints", build.Game, build.Spec, build.Talents)
			}
			if build.optimized() && (build.Date == "" || build.NormalizationHash == "" || build.DPS <= 0 || build.Phase == "") {
				t.Errorf("%s %s optimized build lacks provenance: %+v", build.Game, build.Spec, build)
			}
			if build.Game == "classic" && v.Talents != "" {
				t.Errorf("classic build for %s, which has a UI talent preset", build.Spec)
			}
		}
	}
	list := []TalentBuild{
		{Spec: "warrior-fury", Talents: "a", Phase: "P1", Source: "optimize-talents"},
		{Spec: "warrior-fury", Talents: "b", Phase: "P3", Source: "optimize-talents"},
		{Spec: "warrior-fury", Talents: "c", Phase: "P5", Source: "optimize-talents"},
	}
	for phase, want := range map[Phase]string{0: "a", 1: "a", 2: "a", 3: "b", 4: "b", 6: "c"} {
		if got, ok := pickBuild(list, "warrior-fury", phase, ""); !ok || got.Talents != want {
			t.Errorf("pickBuild phase %d = %q, want %q", phase, got.Talents, want)
		}
	}
	if _, ok := pickBuild(list, "mage-frost", 1, ""); ok {
		t.Error("pickBuild matched another spec")
	}
	path := filepath.Join(t.TempDir(), "builds.json")
	if err := mergeBuildsFile(path, "n", list[:2]); err != nil {
		t.Fatal(err)
	}
	if err := mergeBuildsFile(path, "n", []TalentBuild{{Spec: "warrior-fury", Talents: "z", Phase: "P1", Source: "optimize-talents"}, {Spec: "mage-frost", Talents: "m", Phase: "P1", Source: "manual"}}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	got, err := loadBuilds("forever", raw)
	if err != nil || len(got) != 3 || got[0].Talents != "z" || got[2].Spec != "mage-frost" {
		t.Fatalf("merged builds: %+v %v", got, err)
	}
}

// Borrowed gear is marked and ranked; specs without own or donor gear stay excluded.
func TestBorrowedGear(t *testing.T) {
	ok, excluded, err := selectEntries(1, "dps", "", "none")
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]PresetEntry{}
	for _, e := range ok {
		byID[e.Spec] = e
	}
	arms, found := byID["warrior-arms"]
	if !found || arms.GearStatus != "borrowed:warrior-fury" || arms.BorrowedFrom != "warrior-fury" {
		t.Fatalf("arms P1 must borrow fury gear: %+v", arms)
	}
	var g struct {
		Items []map[string]interface{} `json:"items"`
	}
	if err := json.Unmarshal(arms.gearJSON, &g); err != nil {
		t.Fatal(err)
	}
	if id, _ := g.Items[proto.ItemSlot_ItemSlotMainHand]["id"].(float64); int32(id) != twoHanders[1] {
		t.Errorf("arms main hand %v, want two-hander %d", g.Items[proto.ItemSlot_ItemSlotMainHand], twoHanders[1])
	}
	if id, _ := g.Items[proto.ItemSlot_ItemSlotOffHand]["id"].(float64); id != 0 {
		t.Errorf("arms off hand must be empty: %v", g.Items[proto.ItemSlot_ItemSlotOffHand])
	}
	for _, id := range []string{"mage-fire", "mage-arcane", "warlock-destruction", "hunter-beast-mastery", "hunter-survival", "rogue-assassination", "rogue-subtlety", "paladin-retribution"} {
		if e, found := byID[id]; !found || e.BorrowedFrom == "" {
			t.Errorf("%s P1 must be ranked with borrowed gear: %+v", id, e)
		}
	}
	for _, e := range excluded {
		if e.Spec == "druid-feral-cat" && e.Status != "incomplete_gear" {
			t.Errorf("feral P1: %s", e.Status)
		}
	}
	_, excluded6, _ := selectEntries(6, "dps", "", "none")
	missing := map[string]string{}
	for _, e := range excluded6 {
		missing[e.Spec] = e.GearStatus
	}
	for _, id := range []string{"mage-frost", "mage-fire", "hunter-marksmanship", "rogue-subtlety", "druid-balance"} {
		if missing[id] != "missing" {
			t.Errorf("%s P6 must stay excluded as missing, got %q", id, missing[id])
		}
	}
	if !core.WITH_DB {
		return
	}
	// no_talents: a spec without UI preset and without any build is excluded from both-game rankings.
	b := testBench(t)
	b.classic, b.builds = nil, nil
	kept, ex := b.withTalents([]PresetEntry{arms, byID["warrior-fury"]}, nil, "both")
	if len(kept) != 1 || kept[0].Spec != "warrior-fury" || len(ex) != 1 || ex[0].Status != "no_talents" {
		t.Errorf("withTalents: kept %v excluded %v", kept, ex)
	}
}
