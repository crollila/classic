package main

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func syntheticDims() []setupDim {
	dim := func(name string, labels ...string) setupDim {
		d := setupDim{Name: name}
		for _, l := range labels {
			d.Options = append(d.Options, setupOption{Label: l, apply: func(*setupConfig) {}})
		}
		return d
	}
	return []setupDim{dim("weapon", "sword", "axe", "mace"), dim("race", "human", "orc", "troll"), dim("food", "none", "stew")}
}

// The axe and the orc each lose 20 DPS alone and win 100 together (racial
// weapon skill); the stew is a plain +10. One-at-a-time ascent stalls at
// sword/human/stew, the pair probes find axe+orc.
func syntheticSetupObjective(s []int32, _ int32) (evalResult, int64) {
	dps := 1000.0
	axe, orc := s[0] == 1, s[1] == 1
	switch {
	case axe && orc:
		dps += 100
	case axe || orc:
		dps -= 20
	}
	if s[0] == 2 {
		dps -= 50
	}
	if s[1] == 2 {
		dps -= 40
	}
	if s[2] == 1 {
		dps += 10
	}
	return evalResult{DPS: dps, StdErr: 0.2}, 1
}

func TestSetupSearchFindsJointOptimum(t *testing.T) {
	stalled := newSetupSearch(syntheticDims(), 10, 1, time.Time{}, 0, syntheticSetupObjective)
	if s := stalled.run([]int32{0, 0, 0}); s[0] != 0 || s[1] != 0 || s[2] != 1 {
		t.Fatalf("coordinate ascent alone should stall at sword/human/stew, got %v", s)
	}
	ss := newSetupSearch(syntheticDims(), 10, 1, time.Time{}, 160, syntheticSetupObjective)
	s := ss.run([]int32{0, 0, 0})
	if s[0] != 1 || s[1] != 1 || s[2] != 1 {
		t.Fatalf("pair probes should find axe/orc/stew, got %v (path %+v)", s, ss.path)
	}
	phases := []string{}
	for _, st := range ss.path {
		phases = append(phases, st.Phase)
	}
	if strings.Join(phases, ",") != "ascent,pairs" {
		t.Fatalf("path phases = %v", phases)
	}
	found := false
	for _, si := range ss.interactions() {
		if si.OptionA == "axe" && si.OptionB == "orc" {
			found = true
			if si.Synergy != 140 || !si.Accepted || si.GainA != -20 || si.GainB != -20 || si.GainAB != 100 || si.SynergySE <= 0 {
				t.Fatalf("axe x orc interaction = %+v", si)
			}
		} else if si.Accepted {
			t.Fatalf("unexpected accepted interaction %+v", si)
		}
	}
	if !found {
		t.Fatalf("axe x orc interaction not reported: %+v", ss.interactions())
	}
	// Deterministic.
	again := newSetupSearch(syntheticDims(), 10, 1, time.Time{}, 160, syntheticSetupObjective)
	if s2 := again.run([]int32{0, 0, 0}); again.eng.m.key(s2) != ss.eng.m.key(s) {
		t.Fatalf("not deterministic: %v vs %v", s, s2)
	}
}

// Noise must not be adopted: differences far below the SE change nothing.
func TestSetupSearchIgnoresNoise(t *testing.T) {
	ss := newSetupSearch(syntheticDims(), 10, 1, time.Time{}, 160, func(s []int32, _ int32) (evalResult, int64) {
		return evalResult{DPS: 1000 + 0.01*float64(s[0]+s[1]+s[2]), StdErr: 5}, 1
	})
	if s := ss.run([]int32{0, 0, 0}); s[0]+s[1]+s[2] != 0 || len(ss.path) != 0 {
		t.Fatalf("noise adopted: %v %+v", s, ss.path)
	}
}

// Every consumable option writes exactly its own exclusive slot, so a setup
// (one option per dimension) cannot hold two flasks, foods or imbues per weapon.
func TestConsumeDimsExclusive(t *testing.T) {
	v, ok := findVariant("warrior-fury")
	if !ok {
		t.Fatal("warrior-fury variant missing")
	}
	dims, rejected, _ := consumeDims(v, "max")
	if len(dims) != len(consumeSlots) {
		t.Fatalf("dims = %d, want %d", len(dims), len(consumeSlots))
	}
	base := consumesFor(v, "max")
	names := map[string]bool{}
	for i, dim := range dims {
		if names[dim.Name] {
			t.Fatalf("duplicate dimension %s", dim.Name)
		}
		names[dim.Name] = true
		labels := map[string]bool{}
		for k, o := range dim.Options {
			if labels[o.Label] {
				t.Fatalf("%s: duplicate option %s", dim.Name, o.Label)
			}
			labels[o.Label] = true
			c := setupConfig{consumes: googleProto.Clone(base).(*proto.Consumes)}
			o.apply(&c)
			if k == 0 && !googleProto.Equal(c.consumes, base) {
				t.Fatalf("%s: option 0 must be the normalized value", dim.Name)
			}
			// Resetting the dimension's own field must give the base back.
			fd := base.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(consumeSlots[i].field))
			c.consumes.ProtoReflect().Set(fd, base.ProtoReflect().Get(fd))
			if !googleProto.Equal(c.consumes, base) {
				t.Fatalf("%s option %s touches other consumable fields", dim.Name, o.Label)
			}
			if dim.Name == "off_hand_imbue" && o.Label == "Windfury" {
				t.Fatal("Windfury offered as an off-hand imbue")
			}
			if strings.Contains(o.Label, "WizardOil") || strings.Contains(o.Label, "Poison") {
				t.Fatalf("%s: %s offered to a fury warrior", dim.Name, o.Label)
			}
		}
	}
	if len(rejected) == 0 {
		t.Fatal("expected rejected consumable options")
	}
	if why := imbueProblem(proto.WeaponImbue_DenseSharpeningStone, weaponInfo{name: "mace", weapon: proto.WeaponType_WeaponTypeMace}, false); why == "" {
		t.Fatal("sharpening stone on a mace must be illegal")
	}
	if why := imbueProblem(proto.WeaponImbue_DenseSharpeningStone, weaponInfo{}, true); why == "" {
		t.Fatal("off-hand imbue without an off-hand weapon must be illegal")
	}
}

func TestOptimizeSetupSmoke(t *testing.T) {
	if !core.WITH_DB {
		t.Skip("requires -tags=with_db")
	}
	if testing.Short() {
		t.Skip("slow")
	}
	fs, c := newFlagSet("test", io.Discard)
	if err := c.parse(fs, []string{"-iterations", "4", "-seed", "11"}); err != nil {
		t.Fatal(err)
	}
	b, _, err := c.newBench()
	if err != nil {
		t.Fatal(err)
	}
	res, err := b.optimizeSetup("warrior-fury", setupOptions{game: "forever", phase: 1, base: 4, enc: EncounterSpec{Targets: 1, Duration: 30},
		parallel: 8, budget: 8 * time.Second, maxProbes: 8, maxReport: 5, dimensions: map[string]bool{"race": true, "consumes": true}}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.DPS <= 0 || res.PresetDPS <= 0 || res.Sims == 0 || res.NormalizationHash == "" || len(res.Setup) < 2 {
		t.Fatalf("bad result: %+v", res)
	}
	for _, d := range res.Setup {
		if d.Name == "talents" || d.Name == "gear" || d.Name == "apl" {
			t.Fatalf("-dimensions not respected: %s", d.Name)
		}
	}
}
