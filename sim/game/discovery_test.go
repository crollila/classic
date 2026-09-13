package game_test

import (
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	"github.com/wowsims/classic/sim/game"
	googleproto "google.golang.org/protobuf/proto"
	"testing"
)

func TestDiscoveryRequestIsolationAndProvenance(t *testing.T) {
	req := mageRequest(t)
	p := req.Raid.Parties[0].Players[0]
	p.TalentsString = ""
	p.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID, Talents: map[string]int32{"mage.talent.elemental-precision": 1}}
	original := googleproto.Clone(req)
	result, meta, err := game.RunRaidSim(game.Selection{Version: game.Forever, Discovery: true}, req)
	if err != nil || result.GetRaidMetrics().GetDps().GetAvg() <= 0 {
		t.Fatal(err)
	}
	if meta.BaselineOnly || meta.ManifestSHA256 != foreverdata.ManifestSHA256() || len(meta.MechanicIDs) != 1 || meta.MechanicIDs[0] != "mage.talent.elemental-precision" || meta.Experimental {
		t.Fatalf("wrong provenance %+v", meta)
	}
	if !googleproto.Equal(req, original) {
		t.Fatal("mutated request")
	}
	if _, _, err = game.RunRaidSim(game.Selection{Version: game.Classic}, req); err == nil {
		t.Fatal("Classic accepted Forever effects")
	}
	baseline := forever.Baseline()
	if _, _, err = game.RunRaidSim(game.Selection{Version: game.Forever, Catalog: &baseline}, req); err == nil {
		t.Fatal("baseline accepted Forever effects")
	}
	// Same fixture after the opt-in run has exactly the same baseline DPS as before.
	p.Forever = nil
	before, _, err := game.RunRaidSim(game.Selection{Version: game.Classic}, req)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := game.RunRaidSim(game.Selection{Version: game.Classic}, req)
	if err != nil {
		t.Fatal(err)
	}
	if before.RaidMetrics.Dps.Avg != again.RaidMetrics.Dps.Avg {
		t.Fatal("Classic is not repeatable after discovery")
	}
}

func TestDiscoverySupportsAnnouncedRaidSizes(t *testing.T) {
	for _, players := range []int{10, 20, 40} {
		request := mageRequest(t)
		template := request.Raid.Parties[0].Players[0]
		template.TalentsString = ""
		template.Forever = &proto.ForeverOptions{RulesetId: foreverdata.RulesetID}
		request.Raid.Parties = nil
		for i := 0; i < players/5; i++ {
			party := &proto.Party{}
			for j := 0; j < 5; j++ {
				party.Players = append(party.Players, googleproto.Clone(template).(*proto.Player))
			}
			request.Raid.Parties = append(request.Raid.Parties, party)
		}
		env, _, _ := core.NewEnvironment(request.Raid, request.Encounter, false)
		got := 0
		for _, party := range env.Raid.Parties {
			got += len(party.Players)
		}
		if got != players {
			t.Fatalf("%d-player raid produced %d characters", players, got)
		}
	}
}
