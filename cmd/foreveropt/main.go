// foreveropt exposes the full Forever gear/talent/enchant/race/rotation optimizer.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/wowsims/classic/sim"
	"github.com/wowsims/classic/sim/optimizer"
)

func main() {
	spec := flag.String("spec", "", "Forever app spec key (required)")
	level := flag.Int("level", 60, "character level (10..60)")
	build := flag.String("build", "", "registered game-data build; empty uses embedded current")
	seed := flag.Int64("seed", 20260920, "reproducible search seed")
	parallel := flag.Int("parallel", 0, "parallel simulator workers; zero uses CPU count")
	base := flag.Int("base-iterations", 100, "initial racing iterations")
	final := flag.Int("final-iterations", 5000, "fresh final-report iterations")
	weights := flag.Int("weight-iterations", 1500, "stat-weight iterations")
	topK := flag.Int("top-k", 5, "EP-ranked items retained per slot")
	rounds := flag.Int("rounds", 4, "maximum complete search rounds")
	budget := flag.Duration("budget", 0, "optional wall-clock search budget, for example 20m")
	root := flag.String("root", "", "classic repository root; normally auto-detected")
	out := flag.String("out", "", "write JSON here (default stdout)")
	flag.Parse()
	if *spec == "" {
		fmt.Fprintln(os.Stderr, "-spec is required")
		os.Exit(2)
	}
	sim.RegisterAll()
	data, err := optimizer.LoadData(*root)
	if err != nil {
		fail(err)
	}
	problem, err := optimizer.NewProblem(data, *spec, int32(*level))
	if err != nil {
		fail(err)
	}
	problem.Build = *build
	result, err := optimizer.Optimize(problem, optimizer.Options{
		Seed: *seed, Parallel: *parallel, BaseIterations: int32(*base), FinalIterations: int32(*final),
		WeightIters: int32(*weights), TopK: *topK, MaxRounds: *rounds, Budget: time.Duration(*budget),
		Progress: func(stage string, sims int64, best float64) {
			fmt.Fprintf(os.Stderr, "%s: %d sims, %.2f DPS\n", stage, sims, best)
		},
	})
	if err != nil {
		fail(err)
	}
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fail(err)
	}
	if *out == "" {
		fmt.Println(string(body))
		return
	}
	if err := os.WriteFile(*out, body, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
