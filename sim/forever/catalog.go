// Package forever owns Forever-specific mechanics and their provenance.
// The initial catalog is deliberately empty: no Forever mechanics are verified yet.
package forever

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
)

const UpstreamCommit = "7779ebbf79dc7f1341e6ab939b28a3402c9a730a"
const BaselineRulesetID = "forever-classic-baseline-v1"

//go:embed baseline.json
var baselineJSON []byte

// Catalog is the simulator-side import contract, not an assumed database schema.
// A database exporter must translate its records to this versioned contract.
type Catalog struct {
	SchemaVersion    int        `json:"schema_version"`
	RulesetID        string     `json:"ruleset_id"`
	UpstreamCommit   string     `json:"upstream_commit"`
	DatabaseRevision string     `json:"database_revision"`
	Mechanics        []Mechanic `json:"mechanics"`
}

type Mechanic struct {
	MechanicID     string     `json:"mechanic_id"`
	Kind           string     `json:"kind"`
	Change         string     `json:"change"`
	EntityID       int64      `json:"entity_id,omitempty"`
	ClassID        int32      `json:"class_id,omitempty"` // WoW class ID, NOT proto.Class's enum value.
	RaceID         int32      `json:"race_id,omitempty"`  // WoW race ID, NOT proto.Race's enum value.
	RequiredPieces int32      `json:"required_pieces,omitempty"`
	Confidence     string     `json:"confidence"`
	Evidence       []Evidence `json:"evidence"`
	Implementation string     `json:"implementation"`
	Tests          []TestRef  `json:"tests"`
}

type Evidence struct {
	SourceID string `json:"source_id"`
	Locator  string `json:"locator"` // URL, database record, or captured experiment artifact.
	Note     string `json:"note"`
}

type TestRef struct {
	File     string `json:"file"`     // Repository-relative Go test file.
	Function string `json:"function"` // Top-level Test function; CI verifies this exists.
}

func Baseline() Catalog {
	catalog, err := DecodeCatalog(bytes.NewReader(baselineJSON))
	if err != nil {
		panic(err) // A malformed embedded catalog is a build defect.
	}
	return catalog
}

func DecodeCatalog(reader io.Reader) (Catalog, error) {
	var catalog Catalog
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, fmt.Errorf("decode Forever catalog: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Catalog{}, fmt.Errorf("Forever catalog must contain exactly one JSON object")
	}
	return catalog, catalog.Validate()
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != 1 || !present(catalog.RulesetID) || catalog.UpstreamCommit != UpstreamCommit || catalog.Mechanics == nil {
		return fmt.Errorf("catalog requires schema_version 1, ruleset_id, the pinned upstream_commit, and a mechanics array")
	}
	if len(catalog.Mechanics) > 0 && !present(catalog.DatabaseRevision) {
		return fmt.Errorf("a nonempty catalog requires database_revision")
	}
	seen := make(map[string]bool)
	for _, mechanic := range catalog.Mechanics {
		if !present(mechanic.MechanicID) || seen[mechanic.MechanicID] {
			return fmt.Errorf("missing or duplicate mechanic_id %q", mechanic.MechanicID)
		}
		seen[mechanic.MechanicID] = true
		if err := mechanic.validate(); err != nil {
			return fmt.Errorf("mechanic %s: %w", mechanic.MechanicID, err)
		}
	}
	return nil
}

func (m Mechanic) validate() error {
	switch m.Kind {
	case "talent", "ability", "racial", "item", "set_bonus":
		if m.EntityID <= 0 {
			return fmt.Errorf("%s requires a positive entity_id", m.Kind)
		}
	case "combat_mechanic":
		// Some engine rules have no game entity ID; mechanic_id remains mandatory.
	default:
		return fmt.Errorf("unsupported kind %q", m.Kind)
	}
	if m.EntityID < 0 || m.ClassID < 0 || m.RaceID < 0 || m.RequiredPieces < 0 {
		return fmt.Errorf("IDs and piece counts cannot be negative")
	}
	if m.Change != "new" && m.Change != "changed" {
		return fmt.Errorf("change must be new or changed")
	}
	if m.Kind == "talent" && m.ClassID == 0 {
		return fmt.Errorf("talent requires class_id")
	}
	if m.Kind == "racial" && m.RaceID == 0 {
		return fmt.Errorf("racial requires race_id")
	}
	if m.Kind == "set_bonus" && m.RequiredPieces == 0 {
		return fmt.Errorf("set_bonus requires required_pieces")
	}
	switch m.Confidence {
	case "low", "medium", "high", "verified":
	default:
		return fmt.Errorf("confidence must be low, medium, high, or verified")
	}
	if len(m.Evidence) == 0 || len(m.Tests) == 0 || !foreverPath(m.Implementation, ".go") || strings.HasSuffix(m.Implementation, "_test.go") {
		return fmt.Errorf("evidence, automated tests, and a sim/forever implementation file are required")
	}
	for _, evidence := range m.Evidence {
		if !present(evidence.SourceID) || !present(evidence.Locator) || !present(evidence.Note) {
			return fmt.Errorf("each evidence entry requires source_id, locator, and note")
		}
	}
	for _, test := range m.Tests {
		if !foreverPath(test.File, "_test.go") || !strings.HasPrefix(test.Function, "Test") || len(test.Function) <= 4 {
			return fmt.Errorf("invalid automated test reference")
		}
	}
	return nil
}

func present(s string) bool { return strings.TrimSpace(s) != "" && strings.TrimSpace(s) == s }

func foreverPath(file, suffix string) bool {
	return strings.HasPrefix(file, "sim/forever/") && path.Clean(file) == file && !strings.Contains(file, "\\") && strings.HasSuffix(file, suffix)
}

// ValidateExecutable fails closed until a class adapter and its mechanics tests
// are implemented. Merely importing records must never imply mechanics support.
func (catalog Catalog) ValidateExecutable() error {
	if err := catalog.Validate(); err != nil {
		return err
	}
	if len(catalog.Mechanics) > 0 {
		return fmt.Errorf("Forever mechanic %s has no executable adapter in this baseline", catalog.Mechanics[0].MechanicID)
	}
	if catalog.RulesetID != BaselineRulesetID || catalog.DatabaseRevision != "" {
		return fmt.Errorf("only the explicit empty Forever baseline is executable")
	}
	return nil
}
