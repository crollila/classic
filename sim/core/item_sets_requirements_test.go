package core

import "testing"

func TestSetBonusRequiresEquippedPieces(t *testing.T) {
	// Manufactured set and item IDs; no claims about live game mechanics.
	set := &ItemSet{ID: 987654, Name: "Synthetic set", Bonuses: map[int32]ApplyEffect{2: func(Agent) {}, 4: func(Agent) {}}}
	previous := sets
	sets = []*ItemSet{set}
	t.Cleanup(func() { sets = previous })
	c := &Character{}
	for count := 0; count <= 4; count++ {
		if count > 0 {
			c.Equipment[count-1] = Item{ID: int32(900000 + count), SetID: set.ID}
		}
		want := 0
		if count >= 2 {
			want++
		}
		if count >= 4 {
			want++
		}
		if got := len(c.GetActiveSetBonuses()); got != want {
			t.Fatalf("%d equipped: %d bonuses, want %d", count, got, want)
		}
		if c.HasSetBonus(set, 2) != (count >= 2) || c.HasSetBonus(set, 4) != (count >= 4) {
			t.Fatal("HasSetBonus disagrees with equipped requirements")
		}
	}
	// A candidate swap must remove the 4-piece, then the 2-piece, immediately.
	c.Equipment[0] = Item{ID: 900010, SetID: set.ID + 1, SetName: set.Name}
	if c.HasSetBonus(set, 4) || len(c.GetActiveSetBonuses()) != 1 {
		t.Fatal("wrong set ID activated same-name bonus")
	}
	c.Equipment[1] = Item{}
	c.Equipment[2] = Item{}
	if c.HasSetBonus(set, 2) || len(c.GetActiveSetBonuses()) != 0 {
		t.Fatal("bonus persisted after gear removal")
	}
}
