package game_test

import (
	"github.com/google/go-cmp/cmp"
	"os"
	"testing"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func mageRequest(t *testing.T) *proto.RaidSimRequest {
	t.Helper()
	data, err := os.ReadFile("../../examples/mage-classic.json")
	if err != nil {
		t.Fatal(err)
	}
	request := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, request); err != nil {
		t.Fatal(err)
	}
	return request
}

func TestMageBaselineParityAndIsolation(t *testing.T) {
	request := mageRequest(t)
	original := googleProto.Clone(request)
	expected := core.RunRaidSim(googleProto.Clone(request).(*proto.RaidSimRequest))
	if expected.Error != nil || expected.GetRaidMetrics().GetDps().GetAvg() <= 0 {
		t.Fatalf("upstream baseline failed: %v", expected.Error)
	}
	baseline := forever.Baseline()
	for _, selection := range []game.Selection{
		{Version: game.Classic},
		{Version: game.Forever, Catalog: &baseline},
		{Version: game.Classic},
	} {
		result, provenance, err := game.RunRaidSim(selection, request)
		if err != nil {
			t.Fatal(err)
		}
		// Upstream emits action metrics from a map, so their order is unspecified.
		// Compare every value exactly after ordering only this unordered collection.
		diff := cmp.Diff(expected, result, protocmp.Transform(), protocmp.SortRepeated(func(a, b *proto.ActionMetrics) bool {
			return core.ProtoToActionID(a.Id).String() < core.ProtoToActionID(b.Id).String()
		}))
		if diff != "" {
			t.Fatalf("%s changed upstream metrics: %s", selection.Version, diff)
		}
		if !googleProto.Equal(request, original) {
			t.Fatal("request was mutated")
		}
		if provenance.Game != selection.Version || provenance.UpstreamCommit != forever.UpstreamCommit || len(provenance.MechanicIDs) != 0 {
			t.Fatalf("incorrect provenance: %+v", provenance)
		}
		if provenance.BaselineOnly != (selection.Version == game.Forever) {
			t.Fatal("incorrect baseline label")
		}
	}
}

func TestVersionSelectionFailsClosed(t *testing.T) {
	baseline := forever.Baseline()
	for _, selection := range []game.Selection{
		{}, {Version: "typo"}, {Version: game.Forever}, {Version: game.Classic, Catalog: &baseline},
	} {
		if _, _, err := game.RunRaidSim(selection, mageRequest(t)); err == nil {
			t.Fatalf("accepted %+v", selection)
		}
	}
	if _, _, err := game.RunRaidSim(game.Selection{Version: game.Classic}, nil); err == nil {
		t.Fatal("accepted nil request")
	}
}

func TestForeverCannotContaminateClassicItemDatabase(t *testing.T) {
	request := mageRequest(t)
	const syntheticItemID = 2000000000
	_, existed := core.ItemsByID[syntheticItemID]
	if existed {
		t.Fatal("test requires an unused synthetic ID")
	}
	request.Raid.Parties[0].Players[0].Database = &proto.SimDatabase{Items: []*proto.SimItem{{Id: syntheticItemID}}}
	baseline := forever.Baseline()
	if _, _, err := game.RunRaidSim(game.Selection{Version: game.Forever, Catalog: &baseline}, request); err == nil {
		t.Fatal("accepted a global item database mutation")
	}
	if _, exists := core.ItemsByID[syntheticItemID]; exists {
		t.Fatal("Forever contaminated Classic item database")
	}
}
