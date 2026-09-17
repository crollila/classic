package main

import (
	"fmt"
	"testing"
)

// syntheticOptimizer builds an optimizer over a one-tree model (12 filler
// talents of 5 ranks with decreasing value, plus the named extra talents) whose
// objective is the injected function instead of a simulation.
func syntheticOptimizer(extra []talentNode, objective func(m *talentModel, s []int32) float64) *optimizer {
	m := &talentModel{Game: "classic", Trees: []string{"tree"}, treeStart: []int{0}, byID: map[string]int{}}
	for i := 0; i < 12; i++ {
		m.Nodes = append(m.Nodes, talentNode{ID: fmt.Sprintf("filler%d", i), Name: fmt.Sprintf("filler%d", i), Max: 5})
	}
	m.Nodes = append(m.Nodes, extra...)
	for i, n := range m.Nodes {
		m.byID[n.ID] = i
	}
	o := &optimizer{m: m, cons: buildConstraints{forbid: map[int]bool{}}, levels: []int32{10, 50, 150}, parallel: 1, pairCap: defaultPairCap,
		cache: map[string]evalResult{}, values: map[int]float64{}}
	o.evalFn = func(s []int32, iterations int32) (evalResult, int64) {
		return evalResult{DPS: objective(m, s), StdErr: 0.2}, 1
	}
	return o
}

func fillerValue(m *talentModel, s []int32) float64 {
	dps := 1000.0
	for i := 0; i < 12; i++ {
		dps += float64(s[i]) * float64(20-i)
	}
	return dps
}

// Talents A and B are worthless alone and worth 100 DPS together: greedy,
// swaps and fill never take them (the build is full of positive filler), the
// pair phase swaps the two cheapest points for A+B and reports the synergy.
func TestPairPhaseFindsSynergy(t *testing.T) {
	o := syntheticOptimizer([]talentNode{{ID: "A", Name: "A", Max: 1}, {ID: "B", Name: "B", Max: 1}}, func(m *talentModel, s []int32) float64 {
		dps := fillerValue(m, s)
		if s[m.byID["A"]] > 0 && s[m.byID["B"]] > 0 {
			dps += 100
		}
		return dps
	})
	a, b := o.m.byID["A"], o.m.byID["B"]
	s, _ := o.greedy()
	s, _ = o.swaps(s)
	s, _ = o.fill(s)
	if o.m.total(s) != talentBudget || s[a] != 0 || s[b] != 0 {
		t.Fatalf("marginal search must fill the build without A and B: %v", s)
	}
	before := fillerValue(o.m, s)
	s, steps := o.pairs(s)
	if s[a] != 1 || s[b] != 1 || o.m.total(s) != talentBudget || o.m.legal(s) != nil {
		t.Fatalf("pair phase did not find A+B: %v (steps %+v)", s, steps)
	}
	if len(steps) == 0 || steps[0].Phase != "pairs" || steps[0].DPSAfter <= before {
		t.Errorf("pair step not recorded: %+v", steps)
	}
	found := false
	for _, ti := range o.interactions {
		if ti.A == "A" && ti.B == "B" && ti.Accepted {
			found = true
			if ti.Synergy == nil || *ti.Synergy < 99 || *ti.GainA != 0 || *ti.GainB != 0 || len(ti.Removed) != 2 {
				t.Errorf("A+B interaction: %+v", ti)
			}
		}
	}
	if !found {
		t.Errorf("accepted A+B interaction not reported: %+v", o.interactions)
	}
}

// An anti-synergy is never taken, and a talent that only pays off with the
// talent it unlocks (unlock pair: no single-point gain for the second talent).
func TestPairPhaseUnlockAndNoFalsePositive(t *testing.T) {
	extra := []talentNode{{ID: "A", Name: "A", Max: 1}, {ID: "B", Name: "B", Max: 1, Prereqs: []nodePrereq{{idx: 12, rank: 1}}},
		{ID: "C", Name: "C", Max: 1}, {ID: "D", Name: "D", Max: 1}}
	o := syntheticOptimizer(extra, func(m *talentModel, s []int32) float64 {
		dps := fillerValue(m, s)
		if s[m.byID["B"]] > 0 {
			dps += 80
		}
		// C and D are worth 1 DPS each and lose 40 together.
		c, d := float64(s[m.byID["C"]]), float64(s[m.byID["D"]])
		return dps + 1*c + 1*d - 40*c*d
	})
	s := make([]int32, len(o.m.Nodes))
	for i := 0; i < 10; i++ {
		s[i] = 5
	}
	s[10] = 1
	if err := o.m.legal(s); err != nil {
		t.Fatal(err)
	}
	s, steps := o.pairs(s)
	if s[o.m.byID["A"]] != 1 || s[o.m.byID["B"]] != 1 {
		t.Fatalf("unlock pair A+B not found: %v %+v", s, steps)
	}
	if s[o.m.byID["C"]]+s[o.m.byID["D"]] > 1 {
		t.Errorf("anti-synergy pair C+D taken: %v", s)
	}
	if o.m.legal(s) != nil || o.m.total(s) > talentBudget {
		t.Errorf("illegal build %v", s)
	}
}
