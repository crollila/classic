// foreversim is a version-aware CLI for the unchanged WoWSims engine.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
)

func main() {
	version := flag.String("game", "classic", "classic or forever")
	catalogPath := flag.String("catalog", "", "Forever catalog JSON, or 'discovery' for reviewed manifest adapters; required for -game forever")
	inputPath := flag.String("infile", "", "existing WoWSims RaidSimRequest JSON")
	outputPath := flag.String("outfile", "", "result envelope JSON; defaults to stdout")
	flag.Parse()
	if err := run(*version, *catalogPath, *inputPath, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(version, catalogPath, inputPath, outputPath string) error {
	selection := game.Selection{Version: game.Version(version)}
	if catalogPath == "discovery" {
		selection.Discovery = true
	} else if catalogPath != "" {
		file, err := os.Open(catalogPath)
		if err != nil {
			return err
		}
		defer file.Close()
		catalog, err := forever.DecodeCatalog(file)
		if err != nil {
			return err
		}
		selection.Catalog = &catalog
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	request := &proto.RaidSimRequest{}
	// Unknown input fields must not silently disappear (especially Forever fields).
	if err := protojson.Unmarshal(data, request); err != nil {
		return err
	}
	result, provenance, err := game.RunRaidSim(selection, request)
	if err != nil {
		return err
	}
	resultJSON, err := protojson.Marshal(result)
	if err != nil {
		return err
	}
	output, err := json.MarshalIndent(struct {
		Provenance game.Provenance `json:"provenance"`
		Result     json.RawMessage `json:"result"`
	}{provenance, resultJSON}, "", "  ")
	if err != nil {
		return err
	}
	output = append(output, '\n')
	if outputPath != "" {
		return os.WriteFile(outputPath, output, 0644)
	}
	_, err = os.Stdout.Write(output)
	return err
}
