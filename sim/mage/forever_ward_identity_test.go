package mage

import (
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/core/proto"
)

func TestForeverWardRankIdentityMatchesClient(t *testing.T) {
	for name, ranks := range map[string][]foreverMageRank{"Fire Ward": foreverFireWardRanks, "Frost Ward": foreverFrostWardRanks} {
		seen := map[int32]bool{}
		for _, rank := range ranks {
			spell := gamedata.Current().Spell(rank.id)
			if seen[rank.id] || spell == nil || spell.Name != name || spell.BaseLevel != rank.level {
				t.Fatalf("%s level %d has incorrect/duplicate client identity %d", name, rank.level, rank.id)
			}
			seen[rank.id] = true
		}
	}
	_, character := foreverValueSim(t, proto.Class_ClassMage, 60, nil)
	for _, tc := range []struct {
		id     int32
		school core.SpellSchool
	}{{10225, core.SpellSchoolFire}, {28609, core.SpellSchoolFrost}} {
		spell := character.GetSpell(core.ActionID{SpellID: tc.id})
		if spell == nil || spell.SpellSchool != tc.school {
			t.Fatalf("ward %d missing or registered in wrong school", tc.id)
		}
	}
}
