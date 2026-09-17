// foreverbench ranks and compares WoW Classic and WoW Forever (discovery
// ruleset) DPS across specs using the UI presets, with every non-spec input
// normalized. All output is JSON on stdout; errors are JSON {"error": ...} on
// stdout with exit code 1.
//
//	foreverbench list | presets [-phase P1] | version
//	foreverbench rank    -phase P1 [-game forever|classic|both] [-role dps] [-targets 1 -duration 180 -iterations 1000]
//	foreverbench delta   -phase P1 [-role dps]
//	foreverbench meta    -phase P1 [-encounters first-raid|single-target|cleave|aoe|<json>|<file>]
//	foreverbench compare -phase P1 -spec warrior-fury | -request file.json
//	foreverbench optimize-talents -spec warrior-arms -phase P1 -game forever|classic
//	foreverbench optimize-all -phase P1 -game forever,classic
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
)

const usage = `usage: foreverbench <command> [flags]
  list                                  spec variants (UI talent/APL presets, race, consumables archetype)
  presets [-phase P1] [-role dps|tank|all]   preset matrix spec x phase with gear status and item level
  rank -phase P1 [-game forever|classic|both] [-role dps|tank|all]   sorted DPS ranking with 95% CI
  delta -phase P1 [-role dps]           specs sorted by Forever-vs-Classic % change, with ability changes
  meta -phase P1 [-encounters first-raid]    weighted encounter-mix score and S/A/B/C tiers
  compare -phase P1 -spec ID | -request FILE   Classic vs Forever for one spec (or a RaidSimRequest)
  optimize-talents -spec ID -phase P1 -game forever|classic [-iterations 200 -budget-seconds S -seed X -targets N -out FILE]
                                        deterministic talent search under the rank normalization
  optimize-setup -spec ID -phase P1 [-game forever|classic -dimensions talents,race,gear,consumes,apl -budget-seconds S -reoptimize-talents]
                                        joint setup search (talents x race x gear x consumables x APL) with interactions
  optimize-all -phase P1 [-game forever,classic] [-parallel 2]   optimize every DPS spec; writes presets/*_builds.json
  version                               ruleset / manifest / build info
Sim flags: -phase P0..P6 -iterations N -duration S -targets K -target-level L -seed X
           -world-buffs off|on -consumes standard|max|none -race-policy fixed|best -race-iterations N
           -forever-auto-rotation conservative|ui|off (-no-forever-auto-rotation) -ab-iterations N
           -forever-talent-policy repair|repair-nofill|migrate|sample -forever-builds FILE -classic-builds FILE
           -mode best_guess|strict -parallel P -specs a,b -top N -full-provenance -compact-json -root DIR
Run "foreverbench <command> -h" for details.`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	var (
		out    interface{}
		err    error
		pretty = true
	)
	cmd, rest := args[0], args[1:]
	for _, a := range rest {
		if a == "-compact-json" || a == "--compact-json" {
			pretty = false
		}
	}
	switch cmd {
	case "list":
		out, err = cmdList(rest, stderr)
	case "presets":
		out, err = cmdPresets(rest, stderr)
	case "compare":
		out, err = cmdCompare(rest, stderr)
	case "overrides":
		out, err = cmdOverrides(rest, stderr)
	case "rank":
		out, err = cmdRank(rest, stderr)
	case "delta":
		out, err = cmdDelta(rest, stderr)
	case "meta":
		out, err = cmdMeta(rest, stderr)
	case "optimize-talents":
		out, err = cmdOptimizeTalents(rest, stderr)
	case "optimize-setup":
		out, err = cmdOptimizeSetup(rest, stderr)
	case "optimize-all":
		out, err = cmdOptimizeAll(rest, stderr)
	case "version":
		out = map[string]interface{}{"forever_ruleset_id": foreverdata.RulesetID, "manifest_sha256": foreverdata.ManifestSHA256(),
			"classic_ruleset_id": game.ClassicRulesetID, "upstream_commit": forever.UpstreamCommit, "with_db": core.WITH_DB, "go": runtime.Version(),
			"preset_snapshot_sha256": presetSnapshotSHA()}
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, usage)
		return 0
	default:
		fmt.Fprintln(stderr, usage)
		return 2
	}
	if err == flag.ErrHelp {
		return 0
	}
	if err != nil {
		writeJSON(stdout, map[string]string{"error": err.Error()}, pretty)
		return 1
	}
	writeJSON(stdout, out, pretty)
	return 0
}

func writeJSON(w io.Writer, v interface{}, pretty bool) {
	var data []byte
	if pretty {
		data, _ = json.MarshalIndent(v, "", "  ")
	} else {
		data, _ = json.Marshal(v)
	}
	w.Write(append(data, '\n'))
}

func nowMS() int64 { return time.Now().UnixMilli() }

// ---------------------------------------------------------------- flags

type commonFlags struct {
	root           string
	phase          string
	iterations     int
	duration       float64
	targets        int
	targetLevel    int
	seed           int64
	mode           string
	parallel       int
	top            int
	fullProvenance bool
	noAutoRotation bool
	autoRotation   string
	abIterations   int
	talentPolicy   string
	buildsFile     string
	classicBuilds  string
	worldBuffs     string
	consumes       string
	racePolicy     string
	raceIterations int
	role           string
	specs          string
	gearFill       string
	set            map[string]bool

	phaseValue Phase
}

func newFlagSet(name string, stderr io.Writer) (*flag.FlagSet, *commonFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &commonFlags{}
	fs.StringVar(&c.root, "root", "", "repo root containing ui/forever/data/talents.json (default: auto-detect from cwd/executable, or $FOREVERBENCH_ROOT)")
	fs.StringVar(&c.phase, "phase", "P1", "content phase P0 (pre-raid BiS) .. P6; every spec uses its UI BiS gear set for this phase")
	fs.IntVar(&c.iterations, "iterations", 1000, "iterations per sim")
	fs.Float64Var(&c.duration, "duration", 180, "encounter duration in seconds")
	fs.IntVar(&c.targets, "targets", 1, "number of targets")
	fs.IntVar(&c.targetLevel, "target-level", 63, "target level")
	fs.Int64Var(&c.seed, "seed", 101, "random seed, reused for every spec and both games")
	fs.StringVar(&c.mode, "mode", "best_guess", "Forever mode: best_guess (UI default) or strict")
	fs.IntVar(&c.parallel, "parallel", min(2, runtime.NumCPU()), "sims to run concurrently")
	fs.IntVar(&c.top, "top", 8, "number of abilities to report")
	fs.BoolVar(&c.fullProvenance, "full-provenance", false, "include full per-game provenance (per-talent evidence) per spec")
	fs.StringVar(&c.autoRotation, "forever-auto-rotation", "conservative", "new Forever actions in the APL: conservative (keep only if a quick A/B shows no DPS loss), ui (add all, like the web UI), off")
	fs.BoolVar(&c.noAutoRotation, "no-forever-auto-rotation", false, "same as -forever-auto-rotation off")
	fs.IntVar(&c.abIterations, "ab-iterations", 300, "iterations per auto-rotation A/B sim")
	fs.StringVar(&c.talentPolicy, "forever-talent-policy", "repair", "Forever talents when no genuine Forever build exists: repair, repair-nofill, migrate (literal UI migration), sample (UI sample build)")
	fs.StringVar(&c.buildsFile, "forever-builds", "", `JSON {"builds":[{"spec","name","talents":"F1:...","source","phase"}]} of Forever builds (added to the embedded presets/forever_builds.json; later entries win)`)
	fs.StringVar(&c.classicBuilds, "classic-builds", "", `JSON {"builds":[{"spec","name","talents":"<classic string>","source","phase"}]} of Classic builds for specs without a UI talent preset (added to presets/classic_builds.json)`)
	fs.StringVar(&c.worldBuffs, "world-buffs", "off", "off: no world/Dire Maul buffs for anyone; on: all of them for everyone")
	fs.StringVar(&c.consumes, "consumes", "standard", "consumables tier for every spec: standard, max (+Zanza), none")
	fs.StringVar(&c.racePolicy, "race-policy", "fixed", "fixed: documented best race per spec (see list); best: sim every legal race and use the best")
	fs.IntVar(&c.raceIterations, "race-iterations", 200, "iterations per race trial with -race-policy best")
	fs.StringVar(&c.role, "role", "dps", "dps, tank, or all")
	fs.StringVar(&c.specs, "specs", "", "optional comma-separated spec ids to restrict to")
	fs.StringVar(&c.gearFill, "gear-fill", "none", "incomplete phase gear sets (e.g. a lone tier set): none = exclude the spec; previous = fill empty slots from the spec's previous complete phase set (gear_status patched)")
	fs.Bool("compact-json", false, "single-line JSON output")
	return fs, c
}

func (c *commonFlags) parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	c.set = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { c.set[f.Name] = true })
	var err error
	if c.phaseValue, err = parsePhase(c.phase); err != nil {
		return err
	}
	switch {
	case c.iterations < 1 || c.iterations > 1000000:
		return fmt.Errorf("-iterations must be 1..1000000")
	case c.duration <= 0 || c.duration > 3600:
		return fmt.Errorf("-duration must be in (0, 3600]")
	case c.targets < 1 || c.targets > 40:
		return fmt.Errorf("-targets must be 1..40")
	case c.abIterations < 10 || c.abIterations > 100000:
		return fmt.Errorf("-ab-iterations must be 10..100000")
	case c.raceIterations < 10 || c.raceIterations > 100000:
		return fmt.Errorf("-race-iterations must be 10..100000")
	}
	if c.parallel < 1 {
		c.parallel = 1
	}
	if _, err := c.foreverMode(); err != nil {
		return err
	}
	if c.noAutoRotation {
		c.autoRotation = "off"
	}
	checks := []struct {
		name, value string
		allowed     []string
	}{
		{"forever-auto-rotation", c.autoRotation, []string{"conservative", "ui", "off"}},
		{"forever-talent-policy", c.talentPolicy, []string{"repair", "repair-nofill", "migrate", "sample"}},
		{"world-buffs", c.worldBuffs, []string{"off", "on"}},
		{"consumes", c.consumes, []string{"standard", "max", "none"}},
		{"race-policy", c.racePolicy, []string{"fixed", "best"}},
		{"role", c.role, []string{"dps", "tank", "all"}},
		{"gear-fill", c.gearFill, []string{"none", "previous"}},
	}
	for _, ch := range checks {
		ok := false
		for _, a := range ch.allowed {
			ok = ok || a == ch.value
		}
		if !ok {
			return fmt.Errorf("-%s must be one of %s", ch.name, strings.Join(ch.allowed, ", "))
		}
	}
	if !core.WITH_DB {
		return fmt.Errorf("binary was built without -tags=with_db; item database is empty and results would be meaningless")
	}
	return nil
}

func (c *commonFlags) foreverMode() (proto.ForeverMode, error) {
	switch strings.ToLower(c.mode) {
	case "best_guess", "best-guess", "bestguess":
		return proto.ForeverMode_BEST_GUESS, nil
	case "strict":
		return proto.ForeverMode_STRICT, nil
	}
	return 0, fmt.Errorf("-mode must be best_guess or strict")
}

func (c *commonFlags) normalization() Normalization {
	m, _ := c.foreverMode()
	n := Normalization{WorldBuffs: c.worldBuffs == "on", ConsumesTier: c.consumes, RacePolicy: c.racePolicy, TargetLevel: int32(c.targetLevel),
		Seed: c.seed, Iterations: int32(c.iterations), AutoRotation: c.autoRotation, ABIterations: int32(c.abIterations),
		TalentPolicy: c.talentPolicy, ForeverMode: m.String(), GearFill: c.gearFill, ReactionMs: reactionTimeMs, ChannelClipMs: channelClipDelayMs}
	if c.racePolicy == "best" {
		n.RaceIters = int32(c.raceIterations)
	}
	return n
}

func (c *commonFlags) newBench() (*bench, string, error) {
	root, err := resolveRoot(c.root)
	if err != nil {
		return nil, "", err
	}
	d, err := loadForeverData(root)
	if err != nil {
		return nil, "", err
	}
	read := func(path string) ([]byte, error) {
		if path == "" {
			return nil, nil
		}
		return os.ReadFile(path)
	}
	extra, err := read(c.buildsFile)
	if err != nil {
		return nil, "", err
	}
	builds, err := loadForeverBuilds(extra)
	if err != nil {
		return nil, "", err
	}
	if extra, err = read(c.classicBuilds); err != nil {
		return nil, "", err
	}
	classic, err := loadClassicBuilds(extra)
	if err != nil {
		return nil, "", err
	}
	if _, err := presetMatrix(); err != nil {
		return nil, "", err
	}
	mode, _ := c.foreverMode()
	return &bench{d: d, norm: c.normalization(), mode: mode, builds: builds, classic: classic, parallel: c.parallel, fullProv: c.fullProvenance, top: c.top}, root, nil
}

// ---------------------------------------------------------------- list

type VariantListing struct {
	ID             string                 `json:"id"`
	Class          string                 `json:"class"`
	Spec           string                 `json:"spec"`
	Role           string                 `json:"role"`
	Status         string                 `json:"status"`
	Reason         string                 `json:"reason,omitempty"`
	UIDir          string                 `json:"ui_dir"`
	TalentPreset   string                 `json:"talent_preset,omitempty"`
	ClassicTalents string                 `json:"classic_talents,omitempty"`
	ClassicSource  string                 `json:"classic_talent_source,omitempty"`
	APLPreset      string                 `json:"apl_preset,omitempty"`
	APLFile        string                 `json:"apl_file,omitempty"`
	SpecOptions    json.RawMessage        `json:"ui_spec_options,omitempty"`
	Archetype      string                 `json:"consumes_archetype"`
	FixedRace      string                 `json:"fixed_race,omitempty"`
	FixedRaceWhy   string                 `json:"fixed_race_reason,omitempty"`
	ForeverRaces   []string               `json:"forever_legal_races,omitempty"`
	BothGameRaces  []string               `json:"races_legal_in_both_games,omitempty"`
	GearPhases     []string               `json:"phases_with_gear"`
	GearByPhase    map[string]string      `json:"gear_status_by_phase,omitempty"`
	Forever        map[string]interface{} `json:"forever_talents,omitempty"`
	Notes          []string               `json:"notes,omitempty"`
}

func cmdList(args []string, stderr io.Writer) (interface{}, error) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "repo root (used for Forever race/talent data)")
	role := fs.String("role", "all", "filter: dps, tank, all")
	fs.Bool("compact-json", false, "single-line JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	m, err := presetMatrix()
	if err != nil {
		return nil, err
	}
	var d *fvDataset
	if r, err := resolveRoot(*root); err == nil {
		d, _ = loadForeverData(r)
	}
	builds, _ := loadForeverBuilds(nil)
	classicBuilds, _ := loadClassicBuilds(nil)
	lb := &bench{builds: builds, classic: classicBuilds}
	out := []VariantListing{}
	for _, v := range variants {
		if *role != "all" && v.Role != *role {
			continue
		}
		l := VariantListing{ID: v.ID, Class: strings.TrimPrefix(v.Class.String(), "Class"), Spec: v.Spec, Role: v.Role, UIDir: v.UIDir,
			Archetype: v.Archetype, Notes: v.Notes, GearPhases: []string{}, GearByPhase: map[string]string{}, Status: "available"}
		if v.Unavailable != "" {
			l.Status, l.Reason = "unavailable", v.Unavailable
			out = append(out, l)
			continue
		}
		l.FixedRace, l.FixedRaceWhy = raceName(v.FixedRace), v.RaceWhy
		for _, e := range m {
			if e.Spec != v.ID {
				continue
			}
			if e.Phase == "P1" || l.ClassicTalents == "" {
				r := lb.resolveTalents(e)
				l.TalentPreset, l.ClassicTalents, l.ClassicSource = r.TalentPreset, r.TalentsString, r.TalentSource
			}
			l.APLPreset, l.APLFile, l.SpecOptions = e.APLPreset, e.APLFile, normalizeJSON(e.specOptions)
			l.GearByPhase[e.Phase] = e.GearStatus
			if e.Status == "ok" {
				l.GearPhases = append(l.GearPhases, e.Phase)
			}
		}
		if len(l.GearPhases) == 0 {
			l.Status, l.Reason = "no_gear", "no phase gear set in the UI presets (only blank gear)"
		}
		if d != nil {
			for _, r := range d.raceCandidates(v.Class, false) {
				l.ForeverRaces = append(l.ForeverRaces, raceName(r))
			}
			for _, r := range d.raceCandidates(v.Class, true) {
				l.BothGameRaces = append(l.BothGameRaces, raceName(r))
			}
			if b, ok := pickBuild(builds, v.ID, 1, d.RulesetID); ok {
				l.Forever = map[string]interface{}{"source": b.label(), "f1": b.Talents, "phase": b.Phase, "optimizer_dps": b.DPS}
			} else if l.ClassicTalents != "" {
				r, filler := d.repairClassicTalents(v.Class, l.ClassicTalents, true)
				l.Forever = map[string]interface{}{
					"source":          "derived:repair (no genuine Forever build available)",
					"f1":              d.encodeTalents(v.Class, r),
					"points":          sumPoints(r),
					"filler_points":   sumPoints(filler),
					"dropped_classic": droppedClassicFields(d, v.Class, l.ClassicTalents, r),
				}
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// ---------------------------------------------------------------- presets

type PhaseSummary struct {
	Phase       string   `json:"phase"`
	Label       string   `json:"label"`
	Ranked      []string `json:"ranked_specs"`
	MissingGear []string `json:"missing_or_incomplete_gear"`
	IlvlMin     float64  `json:"avg_ilvl_min,omitempty"`
	IlvlMax     float64  `json:"avg_ilvl_max,omitempty"`
	IlvlMean    float64  `json:"avg_ilvl_mean,omitempty"`
	Note        string   `json:"note,omitempty"`
}

func cmdPresets(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs := flag.NewFlagSet("presets", flag.ContinueOnError)
	fs.SetOutput(stderr)
	phaseFlag := fs.String("phase", "", "only this phase (default: all)")
	role := fs.String("role", "all", "dps, tank, all")
	fs.Bool("compact-json", false, "single-line JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	var only *Phase
	if *phaseFlag != "" {
		p, err := parsePhase(*phaseFlag)
		if err != nil {
			return nil, err
		}
		only = &p
	}
	m, err := presetMatrix()
	if err != nil {
		return nil, err
	}
	entries := []PresetEntry{}
	coverage := map[string]map[string]string{}
	summaries := []PhaseSummary{}
	for _, p := range allPhases() {
		if only != nil && *only != p {
			continue
		}
		s := PhaseSummary{Phase: p.String(), Label: p.Label(), Ranked: []string{}, MissingGear: []string{}}
		sum, n := 0.0, 0
		for _, e := range m {
			if e.Phase != p.String() || (*role != "all" && e.Role != *role) {
				continue
			}
			entries = append(entries, e)
			if coverage[e.Spec] == nil {
				coverage[e.Spec] = map[string]string{}
			}
			switch e.Status {
			case "ok":
				coverage[e.Spec][e.Phase] = fmt.Sprintf("%s (%s, ilvl %.1f)", e.GearStatus, e.GearLabel, e.AvgIlvl)
				s.Ranked = append(s.Ranked, e.Spec)
				if e.Role == "dps" {
					if n == 0 || e.AvgIlvl < s.IlvlMin {
						s.IlvlMin = e.AvgIlvl
					}
					if e.AvgIlvl > s.IlvlMax {
						s.IlvlMax = e.AvgIlvl
					}
					sum += e.AvgIlvl
					n++
				}
			case "missing_gear", "incomplete_gear":
				coverage[e.Spec][e.Phase] = e.GearStatus
				s.MissingGear = append(s.MissingGear, e.Spec)
			default:
				coverage[e.Spec][e.Phase] = "unavailable"
			}
		}
		if n > 0 {
			s.IlvlMean = round2(sum / float64(n))
			if s.IlvlMax-s.IlvlMin > 6 {
				s.Note = fmt.Sprintf("DPS gear average item level spans %.1f..%.1f in this phase; UI BiS lists are not equally up to date", s.IlvlMin, s.IlvlMax)
			}
		}
		summaries = append(summaries, s)
	}
	return map[string]interface{}{
		"command":    "presets",
		"phases":     summaries,
		"coverage":   coverage,
		"entries":    entries,
		"provenance": provenanceBlock("", nil),
		"notes": []string{
			"Phase is assigned from the UI gear preset name (e.g. 'P3 BiS', 'Phase 3', 'Pre-BiS' = P0, 'MC' = P1) or, for gear files not registered in presets.ts, the file name.",
			"Sets named 'Pn Pre-BiS' are phase-internal alternatives and are not used; 'blank' gear is never used.",
			"gear_status incomplete: the only set for the phase leaves a required slot (head..main hand, ranged for hunters) empty, e.g. a lone 8-piece tier set; such entries are excluded from rankings.",
			"gear_status borrowed:<spec>: the spec has no (complete) UI set for the phase and uses the named same-armor, same-role spec's set (two-handed specs get a phase two-hander instead of the donor's weapons); borrowed entries are ranked.",
			"avg_ilvl is the mean item level of the equipped items (from the embedded item database), a rough cross-spec gear tier check.",
		},
		"elapsed_ms": time.Since(start).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------- rank

type RankEntry struct {
	Rank           int                  `json:"rank"`
	Spec           string               `json:"spec"`
	Class          string               `json:"class"`
	SpecLabel      string               `json:"spec_label"`
	Role           string               `json:"role"`
	DPS            float64              `json:"dps"`
	Stdev          float64              `json:"stdev"`
	StdErr         float64              `json:"stderr"`
	CI95           [2]float64           `json:"ci95"`
	ClassicDPS     *float64             `json:"classic_dps,omitempty"`
	ClassicStdErr  *float64             `json:"classic_stderr,omitempty"`
	ForeverDPS     *float64             `json:"forever_dps,omitempty"`
	DeltaDPS       *float64             `json:"delta_dps,omitempty"`
	DeltaPct       *float64             `json:"delta_pct,omitempty"`
	DeltaPctCI95   *[2]float64          `json:"delta_pct_ci95,omitempty"`
	Race           string               `json:"race"`
	RaceChoice     *RaceChoice          `json:"race_choice"`
	GearStatus     string               `json:"gear_status"`
	GearLabel      string               `json:"gear_label"`
	GearFile       string               `json:"gear_file"`
	AvgIlvl        float64              `json:"avg_ilvl"`
	TalentPreset   string               `json:"talent_preset"`
	APLPreset      string               `json:"apl_preset"`
	TalentSource   string               `json:"talent_source,omitempty"`
	ClassicSource  string               `json:"classic_talent_source,omitempty"`
	TalentBuild    *TalentBuildRef      `json:"talent_build,omitempty"`
	GearBorrowed   string               `json:"gear_borrowed_from,omitempty"`
	ForeverF1      string               `json:"forever_talents,omitempty"`
	ForeverPoints  *int32               `json:"forever_talent_points,omitempty"`
	DroppedTalents []string             `json:"classic_talents_without_forever_node,omitempty"`
	FillerPoints   *int32               `json:"forever_filler_points,omitempty"`
	AutoRotation   *AutoRotationReport  `json:"forever_auto_rotation,omitempty"`
	AbilityDelta   []AbilityChange      `json:"top_ability_changes,omitempty"`
	TopAbilities   map[string][]Ability `json:"top_abilities,omitempty"`
	Provenance     interface{}          `json:"provenance,omitempty"`
	Notes          []string             `json:"notes,omitempty"`
	Error          string               `json:"error,omitempty"`
	ElapsedMS      int64                `json:"elapsed_ms"`
}

func (b *bench) rankEntry(r SpecRun, gameName string) RankEntry {
	e := r.Entry
	out := RankEntry{Spec: e.Spec, Class: e.Class, SpecLabel: e.SpecLabel, Role: e.Role, Race: r.Race.Race, RaceChoice: r.Race,
		GearStatus: e.GearStatus, GearLabel: e.GearLabel, GearFile: e.GearFile, AvgIlvl: e.AvgIlvl, TalentPreset: e.TalentPreset, APLPreset: e.APLPreset,
		TalentSource: r.TalentSource, ClassicSource: e.TalentSource, TalentBuild: r.TalentBuild, GearBorrowed: e.BorrowedFrom,
		AutoRotation: r.Auto, Notes: e.variant.Notes, Error: r.Err, ElapsedMS: r.ElapsedMS}
	if gameName == "classic" {
		out.TalentSource = e.TalentSource
	}
	if e.PatchedFrom != "" {
		out.Notes = append(append([]string{}, out.Notes...), fmt.Sprintf("gear patched: %s filled from %s; lower tier than a full %s BiS set", strings.Join(e.PatchedSlots, ", "), e.PatchedFrom, e.Phase))
	} else if e.GearSource != "" && e.GearSource != "ui presets.ts" && e.BorrowedFrom == "" {
		out.Notes = append(append([]string{}, out.Notes...), "gear: "+e.GearSource)
	}
	if r.ForeverInfo != nil {
		pts := r.ForeverInfo.ForeverPoints
		out.ForeverPoints, out.ForeverF1 = &pts, r.ForeverInfo.ForeverF1
		out.DroppedTalents = r.ForeverInfo.DroppedClassic
		if len(r.ForeverInfo.FillerPoints) > 0 {
			filler := sumPoints(r.ForeverInfo.FillerPoints)
			out.FillerPoints = &filler
		}
	}
	if p := r.primary(); p != nil && p.Error == "" {
		out.DPS, out.Stdev, out.StdErr, out.CI95 = p.DPS, p.Stdev, p.StdErr, p.CI95
	}
	prov := map[string]interface{}{}
	if r.Classic != nil {
		prov["classic"] = compactOrFull(r.Classic.Provenance, b.fullProv)
	}
	if r.Forever != nil {
		prov["forever"] = compactOrFull(r.Forever.Provenance, b.fullProv)
	}
	if b.fullProv {
		out.Provenance = prov
	}
	if gameName == "both" && r.Classic != nil && r.Forever != nil {
		if r.Classic.Error == "" {
			v, se := r.Classic.DPS, r.Classic.StdErr
			out.ClassicDPS, out.ClassicStdErr = &v, &se
		}
		if r.Forever.Error == "" {
			v := r.Forever.DPS
			out.ForeverDPS = &v
		}
		if r.Classic.Error == "" && r.Forever.Error == "" && r.Classic.DPS > 0 {
			dd := round2(r.Forever.DPS - r.Classic.DPS)
			pct := round2(dd / r.Classic.DPS * 100)
			se := sqrt(r.Classic.StdErr*r.Classic.StdErr+r.Forever.StdErr*r.Forever.StdErr) / r.Classic.DPS * 100
			ci := [2]float64{round2(pct - 1.96*se), round2(pct + 1.96*se)}
			out.DeltaDPS, out.DeltaPct, out.DeltaPctCI95 = &dd, &pct, &ci
		}
	}
	return out
}

func sortRanked(entries []RankEntry, less func(a, b RankEntry) bool) {
	sort.SliceStable(entries, func(i, j int) bool {
		if (entries[i].Error == "") != (entries[j].Error == "") {
			return entries[i].Error == ""
		}
		return less(entries[i], entries[j])
	})
	rank := 0
	for i := range entries {
		if entries[i].Error == "" {
			rank++
			entries[i].Rank = rank
		}
	}
}

func cmdRank(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("rank", stderr)
	gameName := fs.String("game", "forever", "forever, classic, or both (sorted by forever; includes classic dps and delta)")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if *gameName != "forever" && *gameName != "classic" && *gameName != "both" {
		return nil, fmt.Errorf("-game must be forever, classic or both")
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	entries, excluded, err := selectEntries(c.phaseValue, c.role, c.specs, c.gearFill)
	if err != nil {
		return nil, err
	}
	entries, excluded = b.withTalents(entries, excluded, *gameName)
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	runs := b.runAll(entries, []EncounterSpec{enc}, *gameName)
	ranking := []RankEntry{}
	failed := 0
	for _, rr := range runs {
		re := b.rankEntry(rr[0], *gameName)
		if re.Error != "" {
			failed++
		}
		ranking = append(ranking, re)
	}
	sortRanked(ranking, func(a, b RankEntry) bool { return a.DPS > b.DPS })
	notes := []string{
		fmt.Sprintf("Ranked by %s DPS. Gear is each spec's UI BiS set for %s (or a same-armor/role donor's set, gear_status borrowed:<spec>); specs without one are listed under excluded.", map[string]string{"forever": "Forever", "classic": "Classic", "both": "Forever"}[*gameName], c.phaseValue.Label()),
		"Adjacent specs whose ci95 ranges overlap are not meaningfully different at this iteration count.",
	}
	if *gameName != "classic" {
		notes = append(notes, "Forever uses the partial discovery ruleset "+foreverdata.RulesetID+" ("+b.norm.ForeverMode+"); mechanics outside the manifest behave like Classic.")
	}
	return map[string]interface{}{
		"command":       "rank",
		"phase":         c.phaseValue.String(),
		"phase_label":   c.phaseValue.Label(),
		"game":          *gameName,
		"role":          c.role,
		"encounter":     enc,
		"ranking":       ranking,
		"failed":        failed,
		"excluded":      excludedList(excluded),
		"normalization": normalizationDoc(b.norm, c.role),
		"provenance":    provenanceBlock(root, map[string]interface{}{"upstream_commit": forever.UpstreamCommit}),
		"notes":         notes,
		"elapsed_ms":    time.Since(start).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------- delta

func cmdDelta(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("delta", stderr)
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	entries, excluded, err := selectEntries(c.phaseValue, c.role, c.specs, c.gearFill)
	if err != nil {
		return nil, err
	}
	entries, excluded = b.withTalents(entries, excluded, "both")
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	runs := b.runAll(entries, []EncounterSpec{enc}, "both")
	ranking := []RankEntry{}
	failed := 0
	for _, rr := range runs {
		r := rr[0]
		re := b.rankEntry(r, "both")
		if re.Error == "" && re.DeltaPct == nil {
			re.Error = "no delta (classic dps is zero)"
		}
		if re.Error != "" {
			failed++
		} else {
			re.AbilityDelta = abilityChanges(b.d, *r.Classic, *r.Forever, min(c.top, 5))
		}
		ranking = append(ranking, re)
	}
	sortRanked(ranking, func(a, b RankEntry) bool { return *a.DeltaPct > *b.DeltaPct })
	pick := func(from []RankEntry) []map[string]interface{} {
		out := []map[string]interface{}{}
		for _, e := range from {
			out = append(out, map[string]interface{}{"spec": e.Spec, "delta_pct": e.DeltaPct, "classic_dps": e.ClassicDPS, "forever_dps": e.ForeverDPS})
		}
		return out
	}
	if len(entries) == 0 {
		failed = 0
	}
	ok := ranking[:len(ranking)-failed]
	n := min(5, len(ok))
	bottom := append([]RankEntry{}, ok[len(ok)-n:]...)
	for i, j := 0, len(bottom)-1; i < j; i, j = i+1, j-1 {
		bottom[i], bottom[j] = bottom[j], bottom[i]
	}
	return map[string]interface{}{
		"command":       "delta",
		"phase":         c.phaseValue.String(),
		"phase_label":   c.phaseValue.Label(),
		"role":          c.role,
		"encounter":     enc,
		"ranking":       ranking,
		"most_buffed":   pick(ok[:n]),
		"most_nerfed":   pick(bottom),
		"failed":        failed,
		"excluded":      excludedList(excluded),
		"normalization": normalizationDoc(b.norm, c.role),
		"provenance":    provenanceBlock(root, map[string]interface{}{"upstream_commit": forever.UpstreamCommit}),
		"notes": []string{
			"delta_pct = (Forever - Classic) / Classic with identical gear, consumables, buffs, race, APL and seed; talents differ per game: Classic uses classic_talent_source (UI preset, else optimized build), Forever uses talent_source (optimized:<date> build, else derived from the Classic talents), and Forever may add new actions (forever_auto_rotation).",
			"delta_pct_ci95 treats the two runs as independent, so it is conservative (common random numbers usually make the true interval narrower).",
			"top_ability_changes lists the largest per-ability DPS changes; status new_in_forever marks actions that do not exist in Classic.",
		},
		"elapsed_ms": time.Since(start).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------- meta

type MetaEncounterResult struct {
	Encounter string   `json:"encounter"`
	DPS       float64  `json:"dps"`
	StdErr    float64  `json:"stderr"`
	Relative  float64  `json:"relative_pct"`
	Added     []string `json:"forever_auto_rotation_added,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type MetaEntry struct {
	Rank         int                   `json:"rank"`
	Spec         string                `json:"spec"`
	Class        string                `json:"class"`
	SpecLabel    string                `json:"spec_label"`
	Score        float64               `json:"score"`
	RelScore     float64               `json:"relative_score_pct"`
	Tier         string                `json:"tier"`
	Borderline   bool                  `json:"borderline,omitempty"`
	Race         string                `json:"race"`
	GearLabel    string                `json:"gear_label"`
	AvgIlvl      float64               `json:"avg_ilvl"`
	TalentSource string                `json:"talent_source,omitempty"`
	GearStatus   string                `json:"gear_status"`
	Encounters   []MetaEncounterResult `json:"encounters"`
	Error        string                `json:"error,omitempty"`
}

type tierBand struct {
	Tier string  `json:"tier"`
	Min  float64 `json:"min_relative_score_pct"`
}

// parseTierBands parses "S:90,A:75,B:60" (descending); C is everything below.
func parseTierBands(s string) ([]tierBand, error) {
	out := []tierBand{}
	last := 101.0
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		var v float64
		if len(kv) != 2 || kv[0] == "" {
			return nil, fmt.Errorf("-tier-bands: bad entry %q (want e.g. S:90,A:75,B:60)", part)
		}
		if _, err := fmt.Sscanf(kv[1], "%g", &v); err != nil || v <= 0 || v >= last {
			return nil, fmt.Errorf("-tier-bands: %q must be a descending percentage in (0,100]", part)
		}
		last = v
		out = append(out, tierBand{kv[0], v})
	}
	return append(out, tierBand{"C", 0}), nil
}

func cmdMeta(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("meta", stderr)
	gameName := fs.String("game", "forever", "forever or classic")
	bandsFlag := fs.String("tier-bands", "S:90,A:75,B:60", "tier thresholds as % of the top score; the rest is C")
	encFlag := fs.String("encounters", "first-raid", "encounter mix: preset ("+strings.Join(mixNames(), ", ")+"), inline JSON [{name,weight,targets,duration_s}], or a JSON file")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if *gameName != "forever" && *gameName != "classic" {
		return nil, fmt.Errorf("-game must be forever or classic for meta")
	}
	mixName, mix, err := parseEncounterMix(*encFlag, os.ReadFile)
	if err != nil {
		return nil, err
	}
	tierBands, err := parseTierBands(*bandsFlag)
	if err != nil {
		return nil, err
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	entries, excluded, err := selectEntries(c.phaseValue, c.role, c.specs, c.gearFill)
	if err != nil {
		return nil, err
	}
	entries, excluded = b.withTalents(entries, excluded, *gameName)
	runs := b.runAll(entries, mix, *gameName)
	best := make([]float64, len(mix))
	for _, rr := range runs {
		for j, r := range rr {
			if p := r.primary(); r.Err == "" && p != nil && p.DPS > best[j] {
				best[j] = p.DPS
			}
		}
	}
	metas := []MetaEntry{}
	for _, rr := range runs {
		e := rr[0].Entry
		m := MetaEntry{Spec: e.Spec, Class: e.Class, SpecLabel: e.SpecLabel, Race: rr[0].Race.Race, GearLabel: e.GearLabel, AvgIlvl: e.AvgIlvl, TalentSource: rr[0].TalentSource, GearStatus: e.GearStatus}
		if *gameName == "classic" {
			m.TalentSource = e.TalentSource
		}
		errs := []string{}
		for j, r := range rr {
			er := MetaEncounterResult{Encounter: mix[j].Name, Error: r.Err}
			if r.Auto != nil {
				er.Added = r.Auto.Added
			}
			if p := r.primary(); r.Err == "" && p != nil && best[j] > 0 {
				er.DPS, er.StdErr = p.DPS, p.StdErr
				er.Relative = round2(p.DPS / best[j] * 100)
				m.Score += mix[j].Weight * p.DPS / best[j] * 100
			} else if r.Err != "" {
				errs = append(errs, mix[j].Name+": "+r.Err)
			}
			m.Encounters = append(m.Encounters, er)
		}
		m.Score = round2(m.Score)
		m.Error = strings.Join(errs, "; ")
		metas = append(metas, m)
	}
	sort.SliceStable(metas, func(i, j int) bool {
		if (metas[i].Error == "") != (metas[j].Error == "") {
			return metas[i].Error == ""
		}
		return metas[i].Score > metas[j].Score
	})
	top := 0.0
	if len(metas) > 0 && metas[0].Error == "" {
		top = metas[0].Score
	}
	tiers := map[string][]string{}
	for _, band := range tierBands {
		tiers[band.Tier] = []string{}
	}
	rank := 0
	for i := range metas {
		m := &metas[i]
		if m.Error != "" || top <= 0 {
			continue
		}
		rank++
		m.Rank = rank
		m.RelScore = round2(m.Score / top * 100)
		for _, band := range tierBands {
			if m.RelScore >= band.Min {
				m.Tier = band.Tier
				break
			}
		}
		for _, band := range tierBands[:len(tierBands)-1] {
			if abs(m.RelScore-band.Min) < 1.5 {
				m.Borderline = true
			}
		}
		tiers[m.Tier] = append(tiers[m.Tier], m.Spec)
	}
	caveats := []string{
		"Scores are relative: for each encounter a spec's DPS is divided by the best spec's DPS, then weighted by the mix; tiers compare the score to the top score.",
		"The UI APLs are mostly single-target rotations; multi-target encounters reflect passive cleave/AoE the APL already uses (plus kept Forever auto-rotation actions), not hand-tuned AoE play.",
		"borderline=true: the relative score is within 1.5 points of a tier boundary, which is inside typical sim noise at these iteration counts.",
		"Every spec uses its UI BiS gear for " + c.phaseValue.Label() + "; gear lists differ in how well they are optimized (see avg_ilvl and `foreverbench presets`).",
	}
	if len(excluded) > 0 {
		ids := []string{}
		for _, e := range excluded {
			ids = append(ids, e.Spec)
		}
		caveats = append(caveats, "Not scored (no phase gear or no UI preset): "+strings.Join(ids, ", ")+".")
	}
	if *gameName == "forever" {
		caveats = append(caveats, "Forever uses the partial discovery ruleset "+foreverdata.RulesetID+" ("+b.norm.ForeverMode+"); talents are the optimize-all builds (talent_source optimized:<date>) where present, otherwise derived from Classic presets; treat the meta as a projection, not a measurement.")
	}
	if len(metas) > 1 && metas[1].Error == "" && metas[1].Score > 0 && metas[0].Score/metas[1].Score > 1.15 {
		caveats = append(caveats, fmt.Sprintf("%s scores %.0f%% above the second spec; because tiers are relative to the top score, the remaining specs are compressed into lower tiers. Compare relative_score_pct values rather than tier letters.", metas[0].Spec, (metas[0].Score/metas[1].Score-1)*100))
	}
	mixDoc := encounterMixDocs[mixName]
	return map[string]interface{}{
		"command":       "meta",
		"phase":         c.phaseValue.String(),
		"phase_label":   c.phaseValue.Label(),
		"game":          *gameName,
		"role":          c.role,
		"encounter_mix": map[string]interface{}{"name": mixName, "description": mixDoc, "encounters": mix},
		"tier_bands":    tierBands,
		"tiers":         tiers,
		"ranking":       metas,
		"excluded":      excludedList(excluded),
		"caveats":       caveats,
		"normalization": normalizationDoc(b.norm, c.role),
		"provenance":    provenanceBlock(root, map[string]interface{}{"upstream_commit": forever.UpstreamCommit}),
		"elapsed_ms":    time.Since(start).Milliseconds(),
	}, nil
}

// ---------------------------------------------------------------- compare

func cmdCompare(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("compare", stderr)
	specID := fs.String("spec", "", "spec id from `list`")
	requestPath := fs.String("request", "", "RaidSimRequest JSON (Classic form, or with forever options) to run under both games; bypasses presets and normalization")
	foreverTalents := fs.String("forever-talents", "", "Forever talent string F1:<CLASS>:a-b-c for player 0")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if (*specID == "") == (*requestPath == "") {
		return nil, fmt.Errorf("compare needs exactly one of -spec or -request")
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	if *requestPath != "" {
		return compareRequest(b, c, root, *requestPath, *foreverTalents, start)
	}
	e, err := matrixEntry(*specID, c.phaseValue)
	if err != nil {
		return nil, err
	}
	e = e.withGearFill(c.gearFill)
	if e.Status != "ok" {
		return nil, fmt.Errorf("%s has no simmable preset for %s: %s", e.Spec, e.Phase, e.Reason)
	}
	if *foreverTalents != "" {
		b.builds = append(b.builds, TalentBuild{Spec: e.Spec, Name: "command line", Talents: *foreverTalents, Source: "-forever-talents", Game: "forever", Phase: e.Phase})
	}
	if ok, _ := b.withTalents([]PresetEntry{e}, nil, "both"); len(ok) == 0 {
		return nil, fmt.Errorf("%s has no Classic talents (no UI preset and no optimized Classic build)", e.Spec)
	}
	e = b.resolveTalents(e)
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	r := b.runAll([]PresetEntry{e}, []EncounterSpec{enc}, "both")[0][0]
	re := b.rankEntry(r, "both")
	if r.Err == "" {
		re.AbilityDelta = abilityChanges(b.d, *r.Classic, *r.Forever, c.top)
		re.TopAbilities = map[string][]Ability{"classic": topAbilities(b.d, *r.Classic, c.top), "forever": topAbilities(b.d, *r.Forever, c.top)}
	}
	re.Provenance = map[string]interface{}{}
	if r.Classic != nil {
		re.Provenance.(map[string]interface{})["classic"] = compactOrFull(r.Classic.Provenance, c.fullProvenance)
	}
	if r.Forever != nil {
		re.Provenance.(map[string]interface{})["forever"] = compactOrFull(r.Forever.Provenance, c.fullProvenance)
	}
	var setup interface{}
	if r.ForeverInfo != nil {
		setup = r.ForeverInfo
	}
	return map[string]interface{}{
		"command":       "compare",
		"phase":         c.phaseValue.String(),
		"phase_label":   c.phaseValue.Label(),
		"encounter":     enc,
		"result":        re,
		"forever_setup": setup,
		"normalization": normalizationDoc(b.norm, e.Role),
		"provenance":    provenanceBlock(root, map[string]interface{}{"upstream_commit": forever.UpstreamCommit}),
		"notes": []string{
			"Both games use the same gear, consumables, buffs, race, APL and seed; delta is Forever minus Classic.",
			"Forever uses the partial discovery ruleset " + foreverdata.RulesetID + " (" + b.norm.ForeverMode + "); mechanics outside the manifest behave like Classic.",
		},
		"elapsed_ms": time.Since(start).Milliseconds(),
	}, nil
}

// compareRequest runs a user RaidSimRequest under both games (no presets or
// normalization; Forever auto-rotation follows the web UI unless off).
func compareRequest(b *bench, c *commonFlags, root, path, foreverTalents string, start time.Time) (interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	userReq := &proto.RaidSimRequest{}
	if err := protojson.Unmarshal(data, userReq); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if userReq.Raid == nil || len(userReq.Raid.Parties) == 0 {
		return nil, fmt.Errorf("%s: request has no raid", path)
	}
	if userReq.SimOptions == nil {
		userReq.SimOptions = &proto.SimOptions{}
	}
	if userReq.Encounter == nil {
		userReq.Encounter = makeEncounter(c.targets, c.duration, int32(c.targetLevel))
	}
	if c.set["iterations"] || userReq.SimOptions.Iterations <= 0 {
		userReq.SimOptions.Iterations = int32(c.iterations)
	}
	if c.set["seed"] || userReq.SimOptions.RandomSeed == 0 {
		userReq.SimOptions.RandomSeed = c.seed
	}
	userReq.SimOptions.IsTest, userReq.SimOptions.Debug = false, false
	if c.set["duration"] {
		userReq.Encounter.Duration = c.duration
	}
	if c.set["targets"] || c.set["target-level"] || len(userReq.Encounter.Targets) == 0 {
		n, level := max(1, len(userReq.Encounter.Targets)), int32(c.targetLevel)
		if c.set["targets"] {
			n = c.targets
		}
		if !c.set["target-level"] && len(userReq.Encounter.Targets) > 0 {
			level = userReq.Encounter.Targets[0].Level
		}
		userReq.Encounter.Targets = makeEncounter(n, 1, level).Targets
	}
	classicReq, notes, err := toClassic(userReq)
	if err != nil {
		return nil, err
	}
	var class proto.Class
	for _, party := range userReq.Raid.Parties {
		for _, p := range party.Players {
			if p != nil && p.Class != proto.Class_ClassUnknown && class == proto.Class_ClassUnknown {
				class = p.Class
			}
		}
	}
	classicReq = mergeForeverBack(classicReq, userReq)
	policy := TalentPolicy{Name: c.talentPolicy}
	if foreverTalents != "" {
		policy.Name = "override"
		if policy.Override, err = b.d.decodeTalents(class, foreverTalents); err != nil {
			return nil, err
		}
	}
	foreverReq, infos, err := toForever(b.d, classicReq, b.mode, policy)
	if err != nil {
		return nil, err
	}
	if b.norm.AutoRotation != "off" {
		if err := applyUIAutoRotation(b.d, foreverReq, infos); err != nil {
			return nil, err
		}
		notes = append(notes, "-request runs add every Forever auto-rotation action (web UI behaviour); the conservative A/B policy only applies to preset specs.")
	}
	if classicReq, _, err = toClassic(classicReq); err != nil {
		return nil, err
	}
	results := make([]RunResult, 2)
	runJobs([]func(){
		func() { results[0] = runGame(game.Classic, classicReq) },
		func() { results[1] = runGame(game.Forever, foreverReq) },
	}, c.parallel)
	cr, fr := results[0], results[1]
	out := map[string]interface{}{
		"command":       "compare",
		"request":       path,
		"classic":       cr,
		"forever":       fr,
		"forever_setup": infos,
		"top_abilities": map[string][]Ability{"classic": topAbilities(b.d, cr, c.top), "forever": topAbilities(b.d, fr, c.top)},
		"provenance": provenanceBlock(root, map[string]interface{}{"upstream_commit": forever.UpstreamCommit,
			"classic": compactOrFull(cr.Provenance, c.fullProvenance), "forever": compactOrFull(fr.Provenance, c.fullProvenance),
			"settings": map[string]interface{}{"iterations": userReq.SimOptions.Iterations, "seed": userReq.SimOptions.RandomSeed, "duration_s": userReq.Encounter.Duration, "targets": len(userReq.Encounter.Targets), "forever_mode": b.norm.ForeverMode, "forever_talent_policy": policy.Name}}),
	}
	if cr.Error == "" && fr.Error == "" {
		out["delta_dps"] = round2(fr.DPS - cr.DPS)
		if cr.DPS != 0 {
			out["delta_pct"] = round2((fr.DPS - cr.DPS) / cr.DPS * 100)
			out["top_ability_changes"] = abilityChanges(b.d, cr, fr, c.top)
		}
	}
	out["notes"] = append(notes, "Request files bypass the preset matrix and normalization: gear, buffs and consumables are taken from the file.")
	out["elapsed_ms"] = time.Since(start).Milliseconds()
	return out, nil
}

// mergeForeverBack re-attaches user-provided Forever options (removed by toClassic)
// so toForever keeps them rather than migrating the bridged Classic string.
func mergeForeverBack(classic, user *proto.RaidSimRequest) *proto.RaidSimRequest {
	for pi, party := range user.Raid.Parties {
		for pj, p := range party.Players {
			if p != nil && p.Forever != nil {
				cp := classic.Raid.Parties[pi].Players[pj]
				cp.Forever = p.Forever
				cp.TalentsString = ""
			}
		}
	}
	return classic
}

func runJobs(jobs []func(), parallel int) {
	sem := make(chan struct{}, max(1, parallel))
	done := make(chan struct{})
	for _, job := range jobs {
		sem <- struct{}{}
		go func(j func()) {
			defer func() { <-sem; done <- struct{}{} }()
			j()
		}(job)
	}
	for range jobs {
		<-done
	}
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
