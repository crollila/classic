package core

import (
	"testing"

	"github.com/wowsims/classic/sim/core/stats"
)

func TestForeverItemCatalogDoesNotChangeClassicItems(t *testing.T) {
	const id int32 = 2147483000
	classic := Item{ID: id, Name: "Classic", Stats: stats.Stats{stats.Strength: 10}}
	forever := Item{ID: id, Name: "Forever", Stats: stats.Stats{stats.Strength: 25}}
	ItemsByID[id], ForeverItemsByID[id] = classic, forever
	defer delete(ItemsByID, id)
	defer delete(ForeverItemsByID, id)

	if got := NewItem(ItemSpec{ID: id}); got.Name != "Classic" || got.Stats[stats.Strength] != 10 {
		t.Fatalf("Classic item changed by Forever catalog: %+v", got)
	}
	character := &Character{}
	if got := character.foreverNewItem(ItemSpec{ID: id}); got.Name != "Forever" || got.Stats[stats.Strength] != 25 {
		t.Fatalf("Forever item did not use client catalog: %+v", got)
	}
}
