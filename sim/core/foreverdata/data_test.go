package foreverdata

import (
	"github.com/wowsims/classic/sim/core/proto"
	googleproto "google.golang.org/protobuf/proto"
	"strings"
	"testing"
)

func player() *proto.Player {
	return &proto.Player{Class: proto.Class_ClassWarrior, Race: proto.Race_RaceHuman, Spec: &proto.Player_Warrior{Warrior: &proto.Warrior{}}, Forever: &proto.ForeverOptions{RulesetId: RulesetID, Talents: map[string]int32{"warrior.talent.deflection": 5, "warrior.talent.improved-overpower": 1}}}
}
func TestTreeImportAndCorrections(t *testing.T) {
	if len(data.Records) != 470 {
		t.Fatal(len(data.Records))
	}
	trees := map[string]bool{}
	ids := map[string]bool{}
	for _, r := range data.Records {
		if ids[r.ID] {
			t.Fatal(r.ID)
		}
		ids[r.ID] = true
		trees[r.Class+"/"+r.Tree] = true
		if len(r.Ranks) != int(r.MaxRank) || r.RequiredPoints != (r.Row-1)*5 {
			t.Fatal(r.ID)
		}
		for _, pre := range r.Prerequisites {
			p, ok := Lookup(pre.ID)
			if !ok || p.Class != r.Class || pre.Rank != p.MaxRank {
				t.Fatal(r.ID)
			}
		}
	}
	if len(trees) != 27 {
		t.Fatal(len(trees))
	}
	r, _ := Lookup("priest.talent.spiritual-guidance")
	if r.Ranks[3].Estimated || r.Ranks[4].Estimated || r.Ranks[3].Values[1] != 6 || r.Ranks[4].Values[1] != 8 {
		t.Fatal("lost directly observed rank corrections", r.Ranks)
	}
	r, _ = Lookup("druid.talent.eclipse")
	if r.Ranks[2].Values[1] != 0.5 || r.Ranks[2].Estimated {
		t.Fatal("estimated 0.51 replaced a confirmed 0.50", r.Ranks[2])
	}
}
func TestValidateAndTranslate(t *testing.T) {
	p := player()
	before := googleproto.Clone(p)
	translated, err := Prepare(p)
	if err != nil {
		t.Fatal(err)
	}
	if translated.TalentsString == "" || !googleproto.Equal(before, p) {
		t.Fatal("translation mutated caller")
	}
	// Overpower moved to row 2 in Forever but still reaches the old typed slot.
	if !strings.HasPrefix(translated.TalentsString, "0500001") {
		t.Fatal(translated.TalentsString)
	}
	bad := []struct {
		name string
		edit func(*proto.Player)
	}{
		{"classic string", func(p *proto.Player) { p.TalentsString = "5" }},
		{"unknown ruleset", func(p *proto.Player) { p.Forever.RulesetId = "future" }},
		{"wrong spec", func(p *proto.Player) { p.Spec = &proto.Player_Mage{Mage: &proto.Mage{}} }},
		{"removed", func(p *proto.Player) { p.Forever.Talents["warrior.removed-talent.axe-specialization"] = 1 }},
		{"wrong class", func(p *proto.Player) { p.Forever.Talents["rogue.talent.malice"] = 1 }},
		{"negative", func(p *proto.Player) { p.Forever.Talents["warrior.talent.deflection"] = -1 }},
		{"excess rank", func(p *proto.Player) { p.Forever.Talents["warrior.talent.deflection"] = 6 }},
		{"unknown combat", func(p *proto.Player) { p.Forever.Talents["warrior.talent.weaponmaster"] = 1 }},
		{"row gate", func(p *proto.Player) { p.Forever.Talents["warrior.talent.deflection"] = 0 }},
		{"prerequisite", func(p *proto.Player) { p.Forever.Talents["warrior.talent.deep-wounds"] = 1 }},
		{"estimated", func(p *proto.Player) { p.Forever.Talents["warrior.talent.improved-heroic-strike"] = 2 }},
		{"wrong racial", func(p *proto.Player) { p.Forever.Mechanics = []string{"racials.tauren.endurance"} }},
		{"unknown item", func(p *proto.Player) { p.Forever.Mechanics = []string{"items.tier-sets"} }},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			p := player()
			tc.edit(p)
			if err := Validate(p); err == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
	p = player()
	p.Forever.ExperimentalEstimatedRanks = true
	p.Forever.Talents["warrior.talent.improved-heroic-strike"] = 2
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
}
func TestImmutableLookup(t *testing.T) {
	r, _ := Lookup("warrior.talent.deflection")
	r.Ranks[0].Values[0] = 999
	r.Ranks[0].Effect = "corrupt"
	again, _ := Lookup(r.ID)
	if again.Ranks[0].Values[0] != 1 || again.Ranks[0].Effect == "corrupt" {
		t.Fatal("shared dataset mutated")
	}
}

func TestPointBudget(t *testing.T) {
	p := player()
	p.Forever.ExperimentalEstimatedRanks = true
	for id, n := range map[string]int32{"warrior.talent.cruelty": 5, "warrior.talent.unbridled-wrath": 5, "warrior.talent.improved-cleave": 3, "warrior.talent.boundless-rage": 3, "warrior.talent.dual-wield-specialization": 5, "warrior.talent.enrage": 5, "warrior.talent.precision": 3, "warrior.talent.death-wish": 1, "warrior.talent.flurry": 5, "warrior.talent.bloodthirst": 1, "warrior.talent.improved-rend": 3, "warrior.talent.improved-heroic-strike": 3, "warrior.talent.anticipation": 5} {
		p.Forever.Talents[id] = n
	}
	if err := Validate(p); err == nil || !strings.Contains(err.Error(), "budget") {
		t.Fatal("over-budget build was not rejected", err)
	}
}
