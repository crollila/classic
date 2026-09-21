package sim

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/stats"
)

func TestDataDrivenSetBonusReachesSimulation(t *testing.T) {
	// Synthetic set/spells only: existing test weapons are assigned to a
	// manufactured set to test activation, not to assert a real game mechanic.
	var set gamedata.ItemSet
	if err := json.Unmarshal([]byte(`{"name":"Synthetic integration set","items":[5191,5192],"bonuses":[{"pieces":2,"spell_id":2147483001},{"pieces":2,"spell_id":2147483002}]}`), &set); err != nil {
		t.Fatal(err)
	}
	variant := gamedata.RegisterVariant("synthetic-set-integration", func(s *gamedata.Snapshot) {
		s.RawItemSets["2147483000"] = &set
		s.RawSpells["2147483001"] = &gamedata.Spell{Effects: []*gamedata.Effect{{Effect: 6, Aura: 99, Base: 100, Targets: []int{1}}}}
		// The second spell is intentionally missing to exercise result warnings.
	})
	ro := caseNamed(t, "Rogue")
	_, base := runForever(t, foreverPlayer(ro, 20, "", nil), 20, 20)
	result, equipped := runForever(t, foreverPlayer(ro, 20, variant.Build, nil), 20, 20)
	if got := equipped.GetStat(stats.AttackPower) - base.GetStat(stats.AttackPower); got != 100 {
		t.Fatalf("set AP applied incorrectly: delta %g", got)
	}
	if result.Provenance == nil || !strings.Contains(strings.Join(result.Provenance.NonClientValues, "\n"), "Synthetic integration set") {
		t.Fatal("missing set spell was omitted from result provenance")
	}
	player := foreverPlayer(ro, 20, variant.Build, nil)
	player.Equipment.Items[proto.ItemSlot_ItemSlotOffHand] = &proto.ItemSpec{}
	without, single := runForever(t, player, 20, 20)
	for _, name := range single.GetActiveSetBonusNames() {
		if strings.Contains(name, "Synthetic integration set") {
			t.Fatal("one equipped piece activated synthetic two-piece set")
		}
	}
	if strings.Contains(strings.Join(without.Provenance.NonClientValues, "\n"), "Synthetic integration set") {
		t.Fatal("set warning leaked from another simulation")
	}
}
