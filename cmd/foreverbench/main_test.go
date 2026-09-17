package main

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
)

func runCLI(t *testing.T, args ...string) []byte {
	t.Helper()
	var out bytes.Buffer
	if code := run(args, &out, io.Discard); code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, out.String())
	}
	return out.Bytes()
}

func TestList(t *testing.T) {
	var specs []SpecListing
	if err := json.Unmarshal(runCLI(t, "list"), &specs); err != nil {
		t.Fatal(err)
	}
	if len(specs) != len(specTable) {
		t.Fatalf("listed %d specs, table has %d", len(specs), len(specTable))
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.ID] || s.Talents == "" || s.Race == "" || string(s.SpecOptions) == "null" {
			t.Fatalf("bad listing %+v", s)
		}
		seen[s.ID] = true
	}
}

// Every derived Forever build must pass the engine's own validation, and F1
// strings must round-trip.
func TestForeverTalentPolicies(t *testing.T) {
	root, err := resolveRoot("")
	if err != nil {
		t.Fatal(err)
	}
	d, err := loadForeverData(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specTable {
		for _, name := range []string{"repair", "repair-nofill", "migrate", "sample"} {
			p := core.WithSpec(&proto.Player{Class: spec.Class, Race: spec.Race, TalentsString: spec.Talents}, spec.SpecOptions)
			info := &ForeverPlayerInfo{}
			talents := d.talentsFor(p, TalentPolicy{Name: name}, true, info)
			f := d.defaultOptions(spec.Class, spec.Race, proto.ForeverMode_BEST_GUESS)
			f.Talents = talents
			fp := &proto.Player{Class: spec.Class, Race: spec.Race, Spec: p.Spec, Forever: f}
			if err := foreverdata.Validate(fp); err != nil {
				t.Fatalf("%s/%s: %v", spec.ID, name, err)
			}
			if name == "repair" && sumPoints(talents) != 51 {
				t.Errorf("%s/repair placed %d points", spec.ID, sumPoints(talents))
			}
			decoded, err := d.decodeTalents(spec.Class, d.encodeTalents(spec.Class, talents))
			if err != nil || sumPoints(decoded) != sumPoints(talents) {
				t.Fatalf("%s/%s: F1 round trip failed: %v", spec.ID, name, err)
			}
		}
	}
}

func TestCompareCheapSpec(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	var out CompareOutput
	raw := runCLI(t, "compare", "-spec", "mage-frost", "-iterations", "20", "-duration", "60")
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Classic.Error != "" || out.Forever.Error != "" {
		t.Fatalf("errors: classic=%q forever=%q", out.Classic.Error, out.Forever.Error)
	}
	if out.Classic.DPS <= 0 || out.Forever.DPS <= 0 || out.DeltaDPS == nil || out.DeltaPct == nil {
		t.Fatalf("missing dps: %s", raw)
	}
	if len(out.TopAbilities["classic"]) == 0 || len(out.TopAbilities["forever"]) == 0 {
		t.Fatalf("missing top abilities: %s", raw)
	}
	if len(out.ForeverSetup) != 1 || out.ForeverSetup[0].ForeverPoints != 51 {
		t.Fatalf("unexpected forever setup: %+v", out.ForeverSetup)
	}
}
