package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wowsims/classic/sim/game"
)

func TestCLIResultProvenance(t *testing.T) {
	output := filepath.Join(t.TempDir(), "result.json")
	if err := run("forever", "../../sim/forever/baseline.json", "../../examples/mage-classic.json", output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance game.Provenance `json:"provenance"`
		Result     json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Provenance.Game != game.Forever || !envelope.Provenance.BaselineOnly || len(envelope.Result) == 0 {
		t.Fatal("missing result provenance")
	}
}

func TestCLIRejectsUnknownFieldsAndMissingCatalog(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.json")
	output := filepath.Join(dir, "output.json")
	if err := os.WriteFile(input, []byte(`{"unimplementedForeverTalent":123}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run("classic", "", input, output); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := run("forever", "", "../../examples/mage-classic.json", output); err == nil {
		t.Fatal("silently fell back to Classic")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("failed request wrote an output file")
	}
}

func TestCLIDiscoveryFixtures(t *testing.T) {
	output := filepath.Join(t.TempDir(), "discovery.json")
	if err := run("forever", "discovery", "../../examples/mage-discovery.json", output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		Provenance game.Provenance `json:"provenance"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	if !envelope.Provenance.PartialRuleset || envelope.Provenance.BaselineOnly || len(envelope.Provenance.TalentEvidence) != 3 {
		t.Fatal("missing per-record discovery provenance")
	}
	if err = run("forever", "discovery", "../../examples/mage-discovery-invalid-rank.json", filepath.Join(t.TempDir(), "invalid.json")); err == nil || !strings.Contains(err.Error(), "invalid rank") {
		t.Fatal("accepted out-of-range rank", err)
	}
	if err = run("forever", "discovery", "../../examples/mage-discovery-strict.json", filepath.Join(t.TempDir(), "strict.json")); err != nil {
		t.Fatal(err)
	}
}
