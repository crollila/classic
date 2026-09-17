// Package game selects a ruleset before calling the existing simulator.
// It has no combat math, replacement engine, or mutable global game selection.
package game

import (
	"fmt"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"sort"
	"strings"

	"github.com/wowsims/classic/sim"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	googleProto "google.golang.org/protobuf/proto"
)

type Version string

const (
	Classic          Version = "classic"
	Forever          Version = "forever"
	ClassicRulesetID         = "classic-7779ebbf"
)

// Register once during package initialization, before callers can run concurrently.
func init() { sim.RegisterAll() }

type Selection struct {
	Version Version
	// Required for Forever, rejected for Classic. There is no implicit fallback.
	Catalog *forever.Catalog
	// Explicit built-in discovery adapter. Mutually exclusive with a v1 catalog.
	Discovery bool
}

type Provenance struct {
	Game           Version          `json:"game"`
	RulesetID      string           `json:"ruleset_id"`
	UpstreamCommit string           `json:"upstream_commit"`
	MechanicIDs    []string         `json:"mechanic_ids"`
	BaselineOnly   bool             `json:"baseline_only"`
	ManifestSHA256 string           `json:"manifest_sha256,omitempty"`
	Experimental   bool             `json:"experimental_estimated_ranks,omitempty"`
	PartialRuleset bool             `json:"partial_ruleset,omitempty"`
	TalentEvidence []TalentEvidence `json:"talent_evidence,omitempty"`
	Modes          []string         `json:"modes,omitempty"`
}

type TalentEvidence struct {
	PlayerIndex           int      `json:"player_index"`
	RecordID              string   `json:"record_id"`
	Rank                  int32    `json:"rank"`
	ValueConfidence       string   `json:"value_confidence"`
	Estimated             bool     `json:"estimated"`
	SourceURL             string   `json:"source_url"`
	InteractionConfidence string   `json:"interaction_confidence"`
	SelectedRank          int32    `json:"selected_rank"`
	ImplementationState   string   `json:"implementation_state"`
	PredictionReason      string   `json:"prediction_reason,omitempty"`
	ConfidenceValue       float64  `json:"confidence_value,omitempty"`
	PredictedComponents   []string `json:"predicted_components,omitempty"`
}

func (s Selection) provenance() (Provenance, error) {
	metadata := Provenance{Game: s.Version, UpstreamCommit: forever.UpstreamCommit, MechanicIDs: []string{}}
	switch s.Version {
	case Classic:
		if s.Catalog != nil || s.Discovery {
			return Provenance{}, fmt.Errorf("Classic cannot accept a Forever mechanics catalog")
		}
		metadata.RulesetID = ClassicRulesetID
	case Forever:
		if s.Discovery {
			if s.Catalog != nil {
				return Provenance{}, fmt.Errorf("discovery cannot accept an external catalog")
			}
			metadata.RulesetID = foreverdata.RulesetID
			metadata.ManifestSHA256 = foreverdata.ManifestSHA256()
			metadata.PartialRuleset = true
			return metadata, nil
		}
		if s.Catalog == nil {
			return Provenance{}, fmt.Errorf("Forever requires an explicit mechanics catalog; use the baseline catalog only for compatibility testing")
		}
		if err := s.Catalog.ValidateExecutable(); err != nil {
			return Provenance{}, err
		}
		metadata.RulesetID = s.Catalog.RulesetID
		metadata.BaselineOnly = true
	default:
		return Provenance{}, fmt.Errorf("unknown game version %q", s.Version)
	}
	return metadata, nil
}

// RunRaidSim selects validated per-character adapters while preserving Classic
// requests, factories and the shared item database. There is no global game flag.
func RunRaidSim(s Selection, request *proto.RaidSimRequest) (*proto.RaidSimResult, Provenance, error) {
	metadata, err := s.provenance()
	if err != nil {
		return nil, Provenance{}, err
	}
	if request == nil || request.Raid == nil || request.Encounter == nil || request.SimOptions == nil || request.SimOptions.Iterations <= 0 {
		return nil, Provenance{}, fmt.Errorf("raid, encounter, and positive simulation iterations are required")
	}
	if s.Version == Forever {
		for _, party := range request.Raid.Parties {
			for _, player := range party.GetPlayers() {
				if player.GetDatabase() != nil {
					return nil, Provenance{}, fmt.Errorf("Forever baseline cannot load custom items into the global Classic database")
				}
			}
		}
	}
	seen := map[string]bool{}
	for partyIndex, party := range request.Raid.Parties {
		for playerIndex, player := range party.GetPlayers() {
			if player == nil || player.Class == proto.Class_ClassUnknown {
				continue
			}
			if !s.Discovery && player.Forever != nil {
				return nil, metadata, fmt.Errorf("Forever options require the discovery ruleset; cannot run as Classic/baseline")
			}
			if s.Discovery {
				if player.Forever == nil {
					return nil, metadata, fmt.Errorf("discovery requires explicit Forever options on every player")
				}
				if err := foreverdata.Validate(player); err != nil {
					return nil, metadata, err
				}
				for id, n := range player.Forever.Talents {
					if n > 0 {
						r, _ := foreverdata.Lookup(id)
						effective := foreverdata.EffectiveRank(player.Forever, id)
						evidence := TalentEvidence{PlayerIndex: partyIndex*5 + playerIndex, RecordID: id, SelectedRank: n, Rank: effective, ImplementationState: "INACTIVE", ValueConfidence: "UNKNOWN", InteractionConfidence: "PROVISIONAL"}
						if effective > 0 {
							seen[id] = true
							rank := r.Ranks[effective-1]
							evidence.ImplementationState = "ACTIVE"
							evidence.ValueConfidence = rank.Confidence
							evidence.Estimated = rank.Estimated
							evidence.SourceURL = rank.SourceURL
							evidence.PredictionReason = rank.PredictionReason
							evidence.ConfidenceValue = rank.ConfidenceValue
							if r.Confidence == "PREDICTED" {
								evidence.ValueConfidence = "PREDICTED"
							}
							if !foreverdata.IsStrict(player.Forever) && len(r.Adapter.PredictedComponents) > 0 {
								evidence.PredictedComponents = r.Adapter.PredictedComponents
								evidence.InteractionConfidence = "PREDICTED"
							}
							metadata.Experimental = metadata.Experimental || evidence.ValueConfidence == "PREDICTED" || len(evidence.PredictedComponents) > 0
						}
						metadata.TalentEvidence = append(metadata.TalentEvidence, evidence)
					}
				}
				for _, id := range player.Forever.Mechanics {
					if foreverdata.MechanicEnabled(player.Forever, id) {
						seen[id] = true
						m, _ := foreverdata.LookupMechanic(id)
						metadata.Experimental = metadata.Experimental || m.Confidence == "PREDICTED"
						if !foreverdata.IsStrict(player.Forever) && len(m.Adapter.PredictedComponents) > 0 && (!strings.HasPrefix(id, "legacy.") || player.Forever.MechanicRanks[id] > 1) {
							metadata.Experimental = true
						}
					}
				}
				mode := "BEST_GUESS"
				if foreverdata.IsStrict(player.Forever) {
					mode = "STRICT"
				}
				metadata.Modes = append(metadata.Modes, mode)
			}
		}
	}
	for id := range seen {
		metadata.MechanicIDs = append(metadata.MechanicIDs, id)
	}
	sort.Strings(metadata.MechanicIDs)
	sort.Slice(metadata.TalentEvidence, func(i, j int) bool {
		a, b := metadata.TalentEvidence[i], metadata.TalentEvidence[j]
		if a.PlayerIndex != b.PlayerIndex {
			return a.PlayerIndex < b.PlayerIndex
		}
		return a.RecordID < b.RecordID
	})
	result := core.RunRaidSim(googleProto.Clone(request).(*proto.RaidSimRequest))
	if result.Error != nil {
		return nil, metadata, fmt.Errorf("simulation failed: %s", result.Error.Message)
	}
	return result, metadata, nil
}
