// foreverref runs the reference simulations against one or more Forever client builds and prints
// the DPS of each, so a build change can be judged before it is published.
//
//	go run --tags=with_db ./cmd/foreverref -load old.json.gz -builds "old-label," -out ref.json
//
// -load registers engine exports (gamedata.engine_export files) for older or candidate builds;
// an empty build label means the embedded current build.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/wowsims/classic/sim"
	"github.com/wowsims/classic/sim/core/gamedata"
	"github.com/wowsims/classic/sim/reference"
)

func main() {
	load := flag.String("load", "", "comma-separated engine export files to register")
	builds := flag.String("builds", ",", "comma-separated build labels to compare; empty label = current")
	only := flag.String("specs", "", "comma-separated spec keys (default: all)")
	iterations := flag.Int("iterations", 2000, "iterations per scenario")
	uiDir := flag.String("ui", "ui", "path to the ui directory")
	out := flag.String("out", "", "write JSON here (default stdout)")
	flag.Parse()
	sim.RegisterAll()

	for _, path := range strings.Split(*load, ",") {
		if path = strings.TrimSpace(path); path == "" {
			continue
		}
		snapshot, err := gamedata.LoadFile(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "registered %s (%s)\n", snapshot.Build, path)
	}
	labels := strings.Split(*builds, ",")
	var specs []string
	if *only != "" {
		specs = strings.Split(*only, ",")
	}
	results := reference.Compare(*uiDir, labels, int32(*iterations), specs)
	body, _ := json.MarshalIndent(map[string]interface{}{"current": gamedata.Current().Build, "builds": labels, "results": results}, "", " ")
	if *out == "" {
		fmt.Println(string(body))
		return
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
