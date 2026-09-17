// foreverbench compares WoW Classic and WoW Forever (discovery ruleset) DPS for
// the spec configurations used by the repository's Go tests, and ranks specs.
// All output is JSON on stdout; errors are JSON on stdout with exit code 1.
//
//	foreverbench list
//	foreverbench compare -spec warrior-fury [-iterations 1000 -duration 180 -targets 1 -target-level 63 -seed 101]
//	foreverbench compare -request my-raid-sim-request.json
//	foreverbench rank [-role dps] [-game forever|classic|both]
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
	"sync"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/forever"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const usage = `usage: foreverbench <list|compare|rank|version> [flags]
  list                         JSON array of built-in spec configs
  compare -spec ID | -request FILE   run Classic and Forever(discovery) and diff
  rank [-role dps|tank|all] [-game forever|classic|both]
Common flags: -root DIR -iterations N -duration S -targets K -target-level L -seed X
              -mode best_guess|strict -parallel P -top N -full-provenance -compact-json
Run "foreverbench <cmd> -h" for details.`

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
	case "compare":
		out, err = cmdCompare(rest, stderr)
	case "rank":
		out, err = cmdRank(rest, stderr)
	case "version":
		out = map[string]interface{}{"forever_ruleset_id": foreverdata.RulesetID, "manifest_sha256": foreverdata.ManifestSHA256(),
			"classic_ruleset_id": game.ClassicRulesetID, "upstream_commit": forever.UpstreamCommit, "with_db": core.WITH_DB, "go": runtime.Version()}
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

type commonFlags struct {
	root           string
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
	talentPolicy   string
	set            map[string]bool
}

func newFlagSet(name string, stderr io.Writer) (*flag.FlagSet, *commonFlags) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &commonFlags{}
	fs.StringVar(&c.root, "root", "", "repo root containing ui/<spec>/gear_sets, ui/<spec>/apls and ui/forever/data/talents.json (default: auto-detect from cwd/executable, or $FOREVERBENCH_ROOT)")
	fs.IntVar(&c.iterations, "iterations", 1000, "iterations per sim")
	fs.Float64Var(&c.duration, "duration", 180, "encounter duration in seconds")
	fs.IntVar(&c.targets, "targets", 1, "number of targets")
	fs.IntVar(&c.targetLevel, "target-level", 63, "target level")
	fs.Int64Var(&c.seed, "seed", 101, "random seed (same seed is used for both games)")
	fs.StringVar(&c.mode, "mode", "best_guess", "Forever mode: best_guess (UI default) or strict")
	fs.IntVar(&c.parallel, "parallel", min(2, runtime.NumCPU()), "sims to run concurrently")
	fs.IntVar(&c.top, "top", 8, "number of top abilities to report (compare)")
	fs.BoolVar(&c.fullProvenance, "full-provenance", false, "include full provenance (per-talent evidence) instead of the compact summary")
	fs.BoolVar(&c.noAutoRotation, "no-forever-auto-rotation", false, "do not prepend new Forever actions to the APL (the Forever UI does this by default)")
	fs.StringVar(&c.talentPolicy, "forever-talent-policy", "repair", "how Forever talents are derived from the Classic build: repair, repair-nofill, migrate (literal UI migration), sample (UI sample build)")
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
	if c.iterations < 1 || c.iterations > 1000000 {
		return fmt.Errorf("-iterations must be 1..1000000")
	}
	if c.duration <= 0 || c.duration > 3600 {
		return fmt.Errorf("-duration must be in (0, 3600]")
	}
	if c.targets < 1 || c.targets > 40 {
		return fmt.Errorf("-targets must be 1..40")
	}
	if c.parallel < 1 {
		c.parallel = 1
	}
	if _, err := c.foreverMode(); err != nil {
		return err
	}
	switch c.talentPolicy {
	case "repair", "repair-nofill", "migrate", "sample":
	default:
		return fmt.Errorf("-forever-talent-policy must be repair, repair-nofill, migrate or sample")
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

func (c *commonFlags) settings() SimSettings {
	m, _ := c.foreverMode()
	return SimSettings{Iterations: int32(c.iterations), Duration: c.duration, Targets: c.targets, TargetLevel: int32(c.targetLevel),
		Seed: c.seed, ForeverMode: m.String(), AutoRotate: !c.noAutoRotation, TalentPolicy: c.talentPolicy}
}

// ---------------------------------------------------------------- list

type SpecListing struct {
	ID          string                 `json:"id"`
	Class       string                 `json:"class"`
	Spec        string                 `json:"spec"`
	Role        string                 `json:"role"`
	Race        string                 `json:"race"`
	Talents     string                 `json:"talents"`
	GearSet     string                 `json:"gear_set"`
	Apl         string                 `json:"apl"`
	Phase       int32                  `json:"phase"`
	Buffs       string                 `json:"buffs"`
	Consumes    json.RawMessage        `json:"consumes"`
	SpecOptions json.RawMessage        `json:"spec_options"`
	Source      string                 `json:"source"`
	Notes       []string               `json:"notes,omitempty"`
	Forever     map[string]interface{} `json:"forever_talents,omitempty"`
}

func specOptionsJSON(spec SpecConfig) json.RawMessage {
	player := core.WithSpec(&proto.Player{}, spec.SpecOptions)
	msg := player.ProtoReflect()
	od := msg.Descriptor().Oneofs().ByName(protoreflect.Name("spec"))
	if od == nil {
		return json.RawMessage("null")
	}
	fd := msg.WhichOneof(od)
	if fd == nil {
		return json.RawMessage("null")
	}
	inner, err := protojson.Marshal(msg.Get(fd).Message().Interface())
	if err != nil {
		return json.RawMessage("null")
	}
	wrapped, _ := json.Marshal(map[string]json.RawMessage{fd.JSONName(): normalizeJSON(inner)})
	return wrapped
}

// protojson output has randomized whitespace; re-encode for stable output.
func normalizeJSON(raw []byte) json.RawMessage {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return json.RawMessage(raw)
	}
	out, _ := json.Marshal(v)
	return out
}

func cmdList(args []string, stderr io.Writer) (interface{}, error) {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "", "repo root (only used to show derived Forever talents)")
	role := fs.String("role", "all", "filter: dps, tank, all")
	fs.Bool("compact-json", false, "single-line JSON output")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	var d *fvDataset
	if r, err := resolveRoot(*root); err == nil {
		d, _ = loadForeverData(r)
	}
	out := []SpecListing{}
	for _, s := range specTable {
		if *role != "all" && s.Role != *role {
			continue
		}
		consumes, _ := protojson.Marshal(s.Consumes)
		l := SpecListing{ID: s.ID, Class: strings.TrimPrefix(s.Class.String(), "Class"), Spec: s.Spec, Role: s.Role,
			Race: strings.TrimPrefix(s.Race.String(), "Race"), Talents: s.Talents,
			GearSet: "ui/" + s.GearDir + "/gear_sets/" + s.GearSet + ".gear.json", Apl: "ui/" + s.AplDir + "/apls/" + s.Apl + ".apl.json",
			Phase: s.Phase, Buffs: "core.FullBuffs", Consumes: normalizeJSON(consumes), SpecOptions: specOptionsJSON(s), Source: s.Source, Notes: s.Notes}
		if d != nil {
			m := d.migrateClassicTalents(s.Class, s.Talents)
			r, filler := d.repairClassicTalents(s.Class, s.Talents, true)
			l.Forever = map[string]interface{}{
				"repair":          map[string]interface{}{"f1": d.encodeTalents(s.Class, r), "points": sumPoints(r), "filler_points": sumPoints(filler)},
				"migrate":         map[string]interface{}{"f1": d.encodeTalents(s.Class, m), "points": sumPoints(m)},
				"dropped_classic": droppedClassicFields(d, s.Class, s.Talents, r),
			}
		}
		out = append(out, l)
	}
	return out, nil
}

// ---------------------------------------------------------------- compare

type CompareOutput struct {
	Spec         interface{}            `json:"spec"`
	Settings     SimSettings            `json:"settings"`
	Classic      RunResult              `json:"classic"`
	Forever      RunResult              `json:"forever"`
	DeltaDPS     *float64               `json:"delta_dps"`
	DeltaPct     *float64               `json:"delta_pct"`
	ForeverSetup []ForeverPlayerInfo    `json:"forever_setup"`
	Provenance   map[string]interface{} `json:"provenance"`
	TopAbilities map[string][]Ability   `json:"top_abilities"`
	Notes        []string               `json:"notes"`
	ElapsedMS    int64                  `json:"elapsed_ms"`
}

func cmdCompare(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("compare", stderr)
	specID := fs.String("spec", "", "spec id from `list`")
	requestPath := fs.String("request", "", "RaidSimRequest JSON (Classic form, or with forever options) to run under both games")
	foreverTalents := fs.String("forever-talents", "", "Forever talent string F1:<CLASS>:a-b-c for player 0 (default: migrate Classic talents)")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if (*specID == "") == (*requestPath == "") {
		return nil, fmt.Errorf("compare needs exactly one of -spec or -request")
	}
	root, err := resolveRoot(c.root)
	if err != nil {
		return nil, err
	}
	d, err := loadForeverData(root)
	if err != nil {
		return nil, err
	}
	s := c.settings()
	notes := []string{}
	var classicReq *proto.RaidSimRequest
	var specInfo interface{}
	var class proto.Class
	if *specID != "" {
		spec, ok := findSpec(*specID)
		if !ok {
			return nil, fmt.Errorf("unknown spec %q (see `foreverbench list`)", *specID)
		}
		classicReq, err = buildSpecRequest(root, spec, s)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", spec.ID, err)
		}
		class = spec.Class
		specInfo = map[string]interface{}{"id": spec.ID, "class": strings.TrimPrefix(spec.Class.String(), "Class"), "spec": spec.Spec, "role": spec.Role,
			"race": strings.TrimPrefix(spec.Race.String(), "Race"), "gear_set": spec.GearSet, "apl": spec.Apl, "source": spec.Source}
		notes = append(notes, spec.Notes...)
	} else {
		data, err := os.ReadFile(*requestPath)
		if err != nil {
			return nil, err
		}
		userReq := &proto.RaidSimRequest{}
		if err := protojson.Unmarshal(data, userReq); err != nil {
			return nil, fmt.Errorf("%s: %w", *requestPath, err)
		}
		if userReq.Raid == nil || len(userReq.Raid.Parties) == 0 {
			return nil, fmt.Errorf("%s: request has no raid", *requestPath)
		}
		if userReq.SimOptions == nil {
			userReq.SimOptions = &proto.SimOptions{}
		}
		if userReq.Encounter == nil {
			userReq.Encounter = makeEncounter(s)
		}
		// Flags only override the file when given explicitly.
		if c.set["iterations"] || userReq.SimOptions.Iterations <= 0 {
			userReq.SimOptions.Iterations = int32(c.iterations)
		}
		if c.set["seed"] || userReq.SimOptions.RandomSeed == 0 {
			userReq.SimOptions.RandomSeed = c.seed
		}
		userReq.SimOptions.IsTest = false
		userReq.SimOptions.Debug = false
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
			userReq.Encounter.Targets = makeEncounter(SimSettings{Targets: n, TargetLevel: level}).Targets
		}
		s.Iterations = userReq.SimOptions.Iterations
		s.Seed = userReq.SimOptions.RandomSeed
		s.Duration = userReq.Encounter.Duration
		s.Targets = len(userReq.Encounter.Targets)
		if s.Targets > 0 {
			s.TargetLevel = userReq.Encounter.Targets[0].Level
		}
		var bridgeNotes []string
		classicReq, bridgeNotes, err = toClassic(userReq)
		if err != nil {
			return nil, err
		}
		notes = append(notes, bridgeNotes...)
		specInfo = map[string]interface{}{"id": "request", "file": *requestPath}
		for _, party := range userReq.Raid.Parties {
			for _, p := range party.Players {
				if p != nil && p.Class != proto.Class_ClassUnknown && class == proto.Class_ClassUnknown {
					class = p.Class
				}
			}
		}
		// Keep the user's Forever options if they had any.
		classicReq = mergeForeverBack(classicReq, userReq)
	}
	mode, _ := c.foreverMode()
	policy := TalentPolicy{Name: c.talentPolicy}
	if *foreverTalents != "" {
		policy.Name = "override"
		policy.Override, err = d.decodeTalents(class, *foreverTalents)
		if err != nil {
			return nil, err
		}
	}
	s.TalentPolicy = policy.Name
	foreverReq, infos, err := toForever(d, classicReq, mode, policy, s.AutoRotate)
	if err != nil {
		return nil, err
	}
	classicReq, _, err = toClassic(classicReq) // strip forever options retained for -request
	if err != nil {
		return nil, err
	}

	results := make([]RunResult, 2)
	jobs := []func(){
		func() { results[0] = runGame(game.Classic, classicReq) },
		func() { results[1] = runGame(game.Forever, foreverReq) },
	}
	runJobs(jobs, c.parallel)
	cr, fr := results[0], results[1]
	out := CompareOutput{Spec: specInfo, Settings: s, Classic: cr, Forever: fr, ForeverSetup: infos,
		TopAbilities: map[string][]Ability{"classic": topAbilities(d, cr, c.top), "forever": topAbilities(d, fr, c.top)}}
	if cr.Error == "" && fr.Error == "" {
		delta := round2(fr.DPS - cr.DPS)
		out.DeltaDPS = &delta
		if cr.DPS != 0 {
			pct := round2((fr.DPS - cr.DPS) / cr.DPS * 100)
			out.DeltaPct = &pct
		}
	}
	if c.fullProvenance {
		out.Provenance = map[string]interface{}{"classic": cr.Provenance, "forever": fr.Provenance}
	} else {
		out.Provenance = map[string]interface{}{"classic": compact(cr.Provenance), "forever": compact(fr.Provenance)}
	}
	for _, info := range infos {
		if info.ClassicPoints > 0 && (info.ForeverPoints < info.ClassicPoints || len(info.DroppedClassic) > 0 || len(info.FillerPoints) > 0) {
			notes = append(notes, fmt.Sprintf("Forever talents for player %d (%s): %d points placed vs %d in the Classic build; %d Classic talents have no legal Forever node; %d filler points. Pass -forever-talents F1:... for a hand-made Forever build.",
				info.PlayerIndex, strings.SplitN(info.TalentSource, ":", 2)[0], info.ForeverPoints, info.ClassicPoints, len(info.DroppedClassic), sumPoints(info.FillerPoints)))
		}
	}
	notes = append(notes,
		"Forever uses the discovery ruleset "+foreverdata.RulesetID+" ("+s.ForeverMode+"): a partial, research-grade ruleset; mechanics not in the manifest behave like Classic.",
		"Both games use the same gear, consumes, buffs, APL and seed; delta is Forever minus Classic.")
	out.Notes = notes
	out.ElapsedMS = time.Since(start).Milliseconds()
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
	var wg sync.WaitGroup
	for _, job := range jobs {
		wg.Add(1)
		sem <- struct{}{}
		go func(j func()) {
			defer func() { <-sem; wg.Done() }()
			j()
		}(job)
	}
	wg.Wait()
}

// ---------------------------------------------------------------- rank

type RankEntry struct {
	Rank          int         `json:"rank"`
	SpecID        string      `json:"spec"`
	Class         string      `json:"class"`
	SpecLabel     string      `json:"spec_label"`
	Role          string      `json:"role"`
	DPS           float64     `json:"dps"`
	Stdev         float64     `json:"stdev"`
	StdErr        float64     `json:"stderr"`
	ClassicDPS    *float64    `json:"classic_dps,omitempty"`
	ForeverDPS    *float64    `json:"forever_dps,omitempty"`
	DeltaPct      *float64    `json:"delta_pct,omitempty"`
	ForeverPoints *int32      `json:"forever_talent_points,omitempty"`
	AutoAdditions []string    `json:"forever_auto_rotation_additions,omitempty"`
	Provenance    interface{} `json:"provenance"`
	Notes         []string    `json:"notes,omitempty"`
	Error         string      `json:"error,omitempty"`
	ElapsedMS     int64       `json:"elapsed_ms"`
}

type RankOutput struct {
	Game      string      `json:"game"`
	Role      string      `json:"role"`
	Settings  SimSettings `json:"settings"`
	Ranking   []RankEntry `json:"ranking"`
	Failed    int         `json:"failed"`
	Notes     []string    `json:"notes"`
	ElapsedMS int64       `json:"elapsed_ms"`
}

func cmdRank(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("rank", stderr)
	role := fs.String("role", "dps", "dps, tank, or all")
	gameName := fs.String("game", "forever", "forever, classic, or both (sorted by forever)")
	specs := fs.String("specs", "", "optional comma-separated spec ids to restrict the ranking")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if *gameName != "forever" && *gameName != "classic" && *gameName != "both" {
		return nil, fmt.Errorf("-game must be forever, classic or both")
	}
	root, err := resolveRoot(c.root)
	if err != nil {
		return nil, err
	}
	d, err := loadForeverData(root)
	if err != nil {
		return nil, err
	}
	s := c.settings()
	mode, _ := c.foreverMode()
	only := map[string]bool{}
	for _, id := range strings.Split(*specs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			if _, ok := findSpec(id); !ok {
				return nil, fmt.Errorf("unknown spec %q", id)
			}
			only[id] = true
		}
	}
	selected := []SpecConfig{}
	for _, spec := range specTable {
		if (*role == "all" || spec.Role == *role) && (len(only) == 0 || only[spec.ID]) {
			selected = append(selected, spec)
		}
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("no specs match -role %s", *role)
	}
	entries := make([]RankEntry, len(selected))
	jobs := []func(){}
	for i, spec := range selected {
		i, spec := i, spec
		jobs = append(jobs, func() { entries[i] = rankOne(root, d, spec, s, mode, *gameName, c.fullProvenance) })
	}
	runJobs(jobs, c.parallel)
	sort.SliceStable(entries, func(i, j int) bool {
		if (entries[i].Error == "") != (entries[j].Error == "") {
			return entries[i].Error == ""
		}
		return entries[i].DPS > entries[j].DPS
	})
	failed := 0
	for i := range entries {
		if entries[i].Error != "" {
			failed++
		} else {
			entries[i].Rank = i + 1
		}
	}
	notes := []string{
		"Spec configs come from the repository's Go tests (gear, talents, APL, consumes, core.FullBuffs). Gear levels differ between specs (e.g. pre-BiS rogue, blank-gear paladin), so this is not a normalized tier list.",
	}
	if *gameName != "classic" {
		notes = append(notes, "Forever runs use discovery ruleset "+foreverdata.RulesetID+" ("+s.ForeverMode+") with Forever talents derived from the Classic test build (policy "+s.TalentPolicy+") and the UI's auto-rotation overlay for new Forever actions.")
	}
	return RankOutput{Game: *gameName, Role: *role, Settings: s, Ranking: entries, Failed: failed, Notes: notes, ElapsedMS: time.Since(start).Milliseconds()}, nil
}

func rankOne(root string, d *fvDataset, spec SpecConfig, s SimSettings, mode proto.ForeverMode, gameName string, full bool) (e RankEntry) {
	start := time.Now()
	e = RankEntry{SpecID: spec.ID, Class: strings.TrimPrefix(spec.Class.String(), "Class"), SpecLabel: spec.Spec, Role: spec.Role, Notes: spec.Notes}
	defer func() { e.ElapsedMS = time.Since(start).Milliseconds() }()
	req, err := buildSpecRequest(root, spec, s)
	if err != nil {
		e.Error = err.Error()
		return e
	}
	prov := map[string]interface{}{}
	pick := func(p *game.Provenance) interface{} {
		if full {
			return p
		}
		return compact(p)
	}
	var classic, fev *RunResult
	if gameName == "classic" || gameName == "both" {
		r := runGame(game.Classic, req)
		classic = &r
		prov["classic"] = pick(r.Provenance)
	}
	if gameName == "forever" || gameName == "both" {
		freq, infos, err := toForever(d, req, mode, TalentPolicy{Name: s.TalentPolicy}, s.AutoRotate)
		if err != nil {
			e.Error = "forever setup: " + err.Error()
			return e
		}
		if len(infos) > 0 {
			pts := infos[0].ForeverPoints
			e.ForeverPoints = &pts
			for _, a := range infos[0].AutoRotationAdded {
				e.AutoAdditions = append(e.AutoAdditions, a.Name)
			}
		}
		r := runGame(game.Forever, freq)
		fev = &r
		prov["forever"] = pick(r.Provenance)
	}
	e.Provenance = prov
	primary := fev
	if primary == nil {
		primary = classic
	}
	errs := []string{}
	if classic != nil && classic.Error != "" {
		errs = append(errs, "classic: "+classic.Error)
	}
	if fev != nil && fev.Error != "" {
		errs = append(errs, "forever: "+fev.Error)
	}
	if len(errs) > 0 {
		e.Error = strings.Join(errs, "; ")
	}
	if primary.Error == "" {
		e.DPS, e.Stdev, e.StdErr = primary.DPS, primary.Stdev, primary.StdErr
	}
	if gameName == "both" {
		if classic.Error == "" {
			v := classic.DPS
			e.ClassicDPS = &v
		}
		if fev.Error == "" {
			v := fev.DPS
			e.ForeverDPS = &v
		}
		if classic.Error == "" && fev.Error == "" && classic.DPS != 0 {
			v := round2((fev.DPS - classic.DPS) / classic.DPS * 100)
			e.DeltaPct = &v
		}
	}
	return e
}
