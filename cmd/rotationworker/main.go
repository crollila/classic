// Local, line-delimited JSON bridge for the simulation optimizer. No network listener.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
	"os"
)

func handle(data []byte) (out any) {
	defer func() {
		if r := recover(); r != nil {
			out = map[string]any{"error": fmt.Sprint(r)}
		}
	}()
	var in struct {
		Op      string          `json:"op"`
		Request json.RawMessage `json:"request"`
	}
	if err := json.Unmarshal(data, &in); err != nil {
		return map[string]any{"error": err.Error()}
	}
	r := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(in.Request, r); err != nil {
		return map[string]any{"error": err.Error()}
	}
	if r.Raid == nil || len(r.Raid.Parties) != 1 || len(r.Raid.Parties[0].Players) != 1 || r.Encounter == nil {
		return map[string]any{"error": "One player and encounter required"}
	}
	selection := game.Selection{Version: game.Forever, Discovery: true}
	if in.Op == "run" {
		result, provenance, err := game.RunRaidSim(selection, r)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		b, err := protojson.Marshal(result)
		if err != nil {
			return map[string]any{"error": err.Error()}
		}
		return map[string]any{"result": json.RawMessage(b), "provenance": provenance}
	}
	if in.Op != "validate" {
		return map[string]any{"error": "Unknown operation"}
	}
	// The game boundary checks scope and modes and supplies authoritative provenance.
	r.SimOptions = &proto.SimOptions{Iterations: 1, RandomSeed: 1}
	_, provenance, err := game.RunRaidSim(selection, r)
	if err != nil {
		return map[string]any{"valid": false, "warnings": []string{err.Error()}}
	}
	result := core.ComputeStats(&proto.ComputeStatsRequest{Raid: r.Raid, Encounter: r.Encounter})
	p := result.RaidStats.Parties[0].Players[0]
	warnings := []string{}
	if result.ErrorResult != "" {
		warnings = append(warnings, result.ErrorResult)
	}
	for _, s := range append(p.RotationStats.GetPrepullActions(), p.RotationStats.GetPriorityList()...) {
		warnings = append(warnings, s.Warnings...)
	}
	b, _ := protojson.Marshal(p.Metadata)
	return map[string]any{"valid": len(warnings) == 0, "warnings": warnings, "metadata": json.RawMessage(b), "provenance": provenance}
}
func main() {
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 65536), 8<<20)
	w := json.NewEncoder(os.Stdout)
	for s.Scan() {
		if err := w.Encode(handle(s.Bytes())); err != nil {
			os.Exit(1)
		}
	}
	if s.Err() != nil {
		fmt.Fprintln(os.Stderr, s.Err())
		os.Exit(1)
	}
}
