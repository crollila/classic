package optimizer

import (
	"testing"

	"github.com/wowsims/classic/sim"
	"github.com/wowsims/classic/sim/core/proto"
)

func TestRecipesAreNotGearAndDuplicateEnchantsUseRuntimeWinner(t *testing.T) {
	sim.RegisterAll()
	d, err := LoadData("")
	if err != nil {
		t.Fatal(err)
	}
	p, err := NewProblem(d, "warrior", 60)
	if err != nil {
		t.Fatal(err)
	}
	if p.itemFits(d.ItemByID[16244], 6, proto.Race_RaceHuman) {
		t.Fatal("profession formula was admitted as hand armor")
	}
	if got := p.enchantByID[927]; got == nil || got.Type != proto.ItemType_ItemTypeWrist {
		t.Fatalf("effect 927 runtime winner = %v, want wrist enchant", got)
	}
}
