package core

import (
	"math"
	"testing"
)

func TestRageConversion(t *testing.T) {
	// Literal expected values catch accidental XOR instead of level squared.
	for _, tc := range []struct {
		level int32
		want  float64
	}{
		{20, 62.69}, {25, 82.25}, {40, 140.5},
		{45, 167.866543875}, {60, 230.60000004},
	} {
		if got := GetRageConversion(tc.level); math.Abs(got-tc.want) > 1e-7 {
			t.Errorf("level %d: got %.12f, want %.12f", tc.level, got, tc.want)
		}
	}
}

func TestForeverRageCorrectionPreservesClassicReplay(t *testing.T) {
	classic := &Unit{}
	forever := &Unit{foreverOverrides: &foreverOverrideState{}}
	if math.Abs(forever.rageConversion(60)-230.60000004) > 1e-7 {
		t.Fatal("Forever did not use corrected conversion")
	}
	if math.Abs(classic.rageConversion(60)-198.3660476632) > 1e-7 {
		t.Fatal("frozen Classic behavior changed")
	}
	if classic.rageConversion(20) != forever.rageConversion(20) {
		t.Fatal("low-level fit changed")
	}
}
