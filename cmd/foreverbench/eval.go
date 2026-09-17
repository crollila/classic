package main

// Evaluation engine shared by rank, delta, meta and compare.

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
	googleProto "google.golang.org/protobuf/proto"
)

type bench struct {
	d        *fvDataset
	norm     Normalization
	mode     proto.ForeverMode
	builds   []TalentBuild // Forever builds (embedded + -forever-builds)
	classic  []TalentBuild // Classic builds (embedded + -classic-builds)
	parallel int
	fullProv bool
	top      int

	raceMu    sync.Mutex
	raceCache map[string]*RaceChoice
}

// ---------------------------------------------------------------- requests

func (b *bench) classicRequest(e PresetEntry, race proto.Race, enc EncounterSpec, iterations int32) (req *proto.RaidSimRequest, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building request panicked: %v", r)
		}
	}()
	if e.Status != "ok" {
		return nil, fmt.Errorf("%s %s is not simmable: %s", e.Spec, e.Phase, e.Reason)
	}
	s, err := uiPresets()
	if err != nil {
		return nil, err
	}
	_ = s
	gearRaw, ok := presetFile(e.GearFile)
	if e.gearJSON != nil {
		gearRaw, ok = e.gearJSON, true
	}
	if !ok {
		return nil, fmt.Errorf("gear file %s missing from preset snapshot", e.GearFile)
	}
	aplRaw, ok := presetFile(e.APLFile)
	if !ok {
		return nil, fmt.Errorf("APL file %s missing from preset snapshot", e.APLFile)
	}
	v := e.variant
	raid, party, ind, debuffs := raidBuffs(b.norm.WorldBuffs)
	player := &proto.Player{
		Name:               v.ID,
		Class:              v.Class,
		Race:               race,
		Equipment:          core.EquipmentSpecFromJsonString(string(gearRaw)),
		Consumes:           consumesFor(v, b.norm.ConsumesTier),
		Buffs:              ind,
		TalentsString:      e.TalentsString,
		Profession1:        proto.Profession_Engineering,
		Rotation:           core.APLRotationFromJsonString(string(aplRaw)),
		DistanceFromTarget: e.distance,
		ReactionTimeMs:     reactionTimeMs,
		ChannelClipDelayMs: channelClipDelayMs,
	}
	if err := setSpecOptions(player, v.OneofField, e.specOptions); err != nil {
		return nil, err
	}
	raidProto := core.SinglePlayerRaidProto(player, party, raid, debuffs)
	if v.Role == "tank" {
		// Tanks are attacked by the boss (rage/threat mechanics); DPS specs are not.
		raidProto.Tanks = []*proto.UnitReference{{Type: proto.UnitReference_Player, Index: 0}}
	}
	return &proto.RaidSimRequest{
		Raid:       raidProto,
		Encounter:  makeEncounter(enc.Targets, enc.Duration, b.norm.TargetLevel),
		SimOptions: &proto.SimOptions{Iterations: iterations, RandomSeed: b.norm.Seed},
	}, nil
}

// TalentBuildRef describes the talent build a run used.
type TalentBuildRef struct {
	Game                 string  `json:"game"`
	Name                 string  `json:"name"`
	Source               string  `json:"source"`
	Phase                string  `json:"optimized_for_phase,omitempty"`
	Date                 string  `json:"date,omitempty"`
	DPS                  float64 `json:"optimizer_dps,omitempty"`
	Talents              string  `json:"talents"`
	NormalizationHash    string  `json:"normalization_hash,omitempty"`
	NormalizationMatches *bool   `json:"normalization_matches,omitempty"`
}

func (b *bench) buildRef(t TalentBuild) *TalentBuildRef {
	ref := &TalentBuildRef{Game: t.gameName(), Name: t.Name, Source: t.Source, Phase: t.Phase, Date: t.Date, DPS: t.DPS, Talents: t.Talents, NormalizationHash: t.NormalizationHash}
	if t.NormalizationHash != "" {
		m := t.NormalizationHash == normalizationHash(b.norm)
		ref.NormalizationMatches = &m
	}
	return ref
}

func (b *bench) talentPolicy(e PresetEntry) (TalentPolicy, string, *TalentBuildRef, error) {
	v := e.variant
	phase, _ := parsePhase(e.Phase)
	if build, ok := pickBuild(b.builds, v.ID, phase, foreverdata.RulesetID); ok {
		t, err := b.d.decodeTalents(v.Class, build.Talents)
		if err != nil {
			return TalentPolicy{}, "", nil, fmt.Errorf("forever build %q for %s: %w", build.Name, v.ID, err)
		}
		return TalentPolicy{Name: "override", Override: t}, build.label(), b.buildRef(build), nil
	}
	if e.TalentsString == "" {
		return TalentPolicy{}, "", nil, fmt.Errorf("%s has no Forever build and no Classic talents to derive one from (run optimize-all)", v.ID)
	}
	return TalentPolicy{Name: b.norm.TalentPolicy}, "derived:" + b.norm.TalentPolicy + " from Classic talents " + e.TalentSource + " (no Forever build available)", nil, nil
}

// resolveTalents fills the Classic talents of an entry without a UI talent
// preset from the Classic builds (optimized builds).
func (b *bench) resolveTalents(e PresetEntry) PresetEntry {
	if e.TalentsString != "" || e.variant.Unavailable != "" {
		return e
	}
	phase, _ := parsePhase(e.Phase)
	if build, ok := pickBuild(b.classic, e.Spec, phase, ""); ok {
		e.TalentsString, e.TalentPreset, e.TalentSource = build.Talents, build.Name, build.label()
		return e
	}
	e.TalentSource = "none"
	return e
}

// withTalents resolves Classic talents and moves entries that cannot be simmed
// in gameName for lack of talents to the excluded list (status no_talents).
func (b *bench) withTalents(ok, excluded []PresetEntry, gameName string) ([]PresetEntry, []PresetEntry) {
	out := []PresetEntry{}
	for _, e := range ok {
		e = b.resolveTalents(e)
		phase, _ := parsePhase(e.Phase)
		_, hasForever := pickBuild(b.builds, e.Spec, phase, foreverdata.RulesetID)
		switch {
		case e.TalentsString == "" && gameName != "forever":
			e.Status, e.Reason = "no_talents", "no UI Classic talent preset and no optimized Classic build (run optimize-all -game classic)"
			excluded = append(excluded, e)
		case e.TalentsString == "" && !hasForever:
			e.Status, e.Reason = "no_talents", "no Forever build and no Classic talents to derive one from (run optimize-all)"
			excluded = append(excluded, e)
		default:
			out = append(out, e)
		}
	}
	return out, excluded
}

func (b *bench) foreverRequest(classic *proto.RaidSimRequest, e PresetEntry) (*proto.RaidSimRequest, ForeverPlayerInfo, string, *TalentBuildRef, error) {
	policy, source, ref, err := b.talentPolicy(e)
	if err != nil {
		return nil, ForeverPlayerInfo{}, "", nil, err
	}
	req, infos, err := toForever(b.d, classic, b.mode, policy)
	if err != nil {
		return nil, ForeverPlayerInfo{}, "", nil, err
	}
	if len(infos) != 1 {
		return nil, ForeverPlayerInfo{}, "", nil, fmt.Errorf("expected one player, got %d", len(infos))
	}
	if policy.Name == "override" {
		infos[0].TalentSource = source
	}
	return req, infos[0], source, ref, nil
}

// ---------------------------------------------------------------- auto rotation

type AutoDecision struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Accepted   bool     `json:"accepted"`
	DPSWithout *float64 `json:"dps_without,omitempty"`
	DPSWith    *float64 `json:"dps_with,omitempty"`
	DeltaPct   *float64 `json:"delta_pct,omitempty"`
	Reason     string   `json:"reason"`
}

type AutoRotationReport struct {
	Mode       string         `json:"mode"`
	Candidates []AutoDecision `json:"candidates"`
	Added      []string       `json:"added"`
}

func (b *bench) autoRotation(freq *proto.RaidSimRequest) (*proto.RaidSimRequest, AutoRotationReport, error) {
	req, rep, _, err := b.autoRotationN(freq, b.norm.ABIterations)
	return req, rep, err
}

// autoRotationN applies the auto-rotation policy with A/B sims of abIterations.
// For the conservative policy it also returns the result of the last accepted
// A/B run (the chosen rotation at abIterations), or nil when no A/B ran.
func (b *bench) autoRotationN(freq *proto.RaidSimRequest, abIterations int32) (*proto.RaidSimRequest, AutoRotationReport, *RunResult, error) {
	rep := AutoRotationReport{Mode: b.norm.AutoRotation, Candidates: []AutoDecision{}, Added: []string{}}
	p := freq.Raid.Parties[0].Players[0]
	if p.Rotation == nil || p.Rotation.Type != proto.APLRotation_TypeAPL {
		return freq, rep, nil, nil
	}
	stats, err := computeAllSpells(freq)
	if err != nil {
		return nil, rep, nil, err
	}
	cands, err := b.d.foreverRotationCandidates(p.Forever, p.Class, stats[[2]int{0, 0}], len(freq.Encounter.GetTargets()))
	if err != nil {
		return nil, rep, nil, err
	}
	if len(cands) == 0 {
		return freq, rep, nil, nil
	}
	base := p.Rotation
	with := func(list []rotationCandidate) *proto.RaidSimRequest {
		r := googleProto.Clone(freq).(*proto.RaidSimRequest)
		r.Raid.Parties[0].Players[0].Rotation, _ = applyRotationCandidates(base, list)
		return r
	}
	switch b.norm.AutoRotation {
	case "off":
		for _, c := range cands {
			rep.Candidates = append(rep.Candidates, AutoDecision{ID: c.ID, Name: c.Name, Reason: "auto-rotation off"})
		}
		return freq, rep, nil, nil
	case "ui":
		for _, c := range cands {
			rep.Candidates = append(rep.Candidates, AutoDecision{ID: c.ID, Name: c.Name, Accepted: true, Reason: "ui mode adds every candidate"})
			rep.Added = append(rep.Added, c.Name)
		}
		return with(cands), rep, nil, nil
	}
	// conservative: greedy A/B in UI priority order with common random numbers.
	accepted := []rotationCandidate{}
	abReq := func(list []rotationCandidate) *proto.RaidSimRequest {
		r := with(list)
		r.SimOptions.Iterations = abIterations
		return r
	}
	cur := runGame(game.Forever, abReq(nil))
	if cur.Error != "" {
		return nil, rep, nil, fmt.Errorf("auto-rotation A/B baseline: %s", cur.Error)
	}
	for _, c := range cands {
		trial := append(append([]rotationCandidate{}, accepted...), c)
		r := runGame(game.Forever, abReq(trial))
		without, withDPS := cur.DPS, r.DPS
		dec := AutoDecision{ID: c.ID, Name: c.Name, DPSWithout: &without}
		if r.Error != "" {
			dec.Reason = "rejected: sim error with action: " + r.Error
		} else {
			dec.DPSWith = &withDPS
			if without > 0 {
				pct := round2((withDPS - without) / without * 100)
				dec.DeltaPct = &pct
			}
			if withDPS >= without {
				dec.Accepted = true
				dec.Reason = fmt.Sprintf("kept: DPS did not drop in a %d-iteration A/B", abIterations)
				accepted = trial
				cur = r
				rep.Added = append(rep.Added, c.Name)
			} else {
				dec.Reason = fmt.Sprintf("rejected: DPS dropped in a %d-iteration A/B", abIterations)
			}
		}
		rep.Candidates = append(rep.Candidates, dec)
	}
	return with(accepted), rep, &cur, nil
}

// ---------------------------------------------------------------- races

type RaceTrial struct {
	Race  string  `json:"race"`
	DPS   float64 `json:"dps,omitempty"`
	Error string  `json:"error,omitempty"`
}

type RaceChoice struct {
	Policy string      `json:"policy"`
	Race   string      `json:"race"`
	Reason string      `json:"reason"`
	Trials []RaceTrial `json:"trials,omitempty"`
	race   proto.Race
}

func (b *bench) chooseRace(e PresetEntry, needClassic bool, primary game.Version, enc EncounterSpec) *RaceChoice {
	key := fmt.Sprintf("%s|%s|%v|%s", e.Spec, e.Phase, needClassic, primary)
	b.raceMu.Lock()
	if b.raceCache == nil {
		b.raceCache = map[string]*RaceChoice{}
	}
	if c, ok := b.raceCache[key]; ok {
		b.raceMu.Unlock()
		return c
	}
	b.raceMu.Unlock()
	v := e.variant
	choice := &RaceChoice{Policy: b.norm.RacePolicy}
	cands := b.d.raceCandidates(v.Class, needClassic)
	if b.norm.RacePolicy == "best" && len(cands) > 0 {
		best := -1.0
		for _, race := range cands {
			t := RaceTrial{Race: raceName(race)}
			dps, err := b.quickDPS(e, race, primary, enc, b.norm.RaceIters)
			if err != nil {
				t.Error = err.Error()
			} else {
				t.DPS = dps
				if dps > best {
					best, choice.race = dps, race
				}
			}
			choice.Trials = append(choice.Trials, t)
		}
		if best >= 0 {
			choice.Race = raceName(choice.race)
			choice.Reason = fmt.Sprintf("highest %s DPS among %d legal races (%d iterations, %d target(s), %.0fs, Forever auto-rotation off)", primary, len(cands), b.norm.RaceIters, enc.Targets, enc.Duration)
		}
	}
	if choice.Race == "" {
		choice.race = v.FixedRace
		choice.Reason = "fixed: " + v.RaceWhy
		if !b.d.raceAllowed(v.Class, v.FixedRace, needClassic) && len(cands) > 0 {
			choice.race = cands[0]
			choice.Reason = fmt.Sprintf("fixed race %s is not legal for %s here; first legal race used", raceName(v.FixedRace), v.Class)
		}
		if b.norm.RacePolicy == "best" {
			choice.Reason = "best-race search failed; " + choice.Reason
		}
		choice.Race = raceName(choice.race)
	}
	b.raceMu.Lock()
	b.raceCache[key] = choice
	b.raceMu.Unlock()
	return choice
}

func (b *bench) quickDPS(e PresetEntry, race proto.Race, g game.Version, enc EncounterSpec, iterations int32) (float64, error) {
	req, err := b.classicRequest(e, race, enc, iterations)
	if err != nil {
		return 0, err
	}
	if g == game.Forever {
		req, _, _, _, err = b.foreverRequest(req, e)
		if err != nil {
			return 0, err
		}
	}
	r := runGame(g, req)
	if r.Error != "" {
		return 0, fmt.Errorf("%s", r.Error)
	}
	return r.DPS, nil
}

// ---------------------------------------------------------------- evaluation

type SpecRun struct {
	Entry        PresetEntry
	Race         *RaceChoice
	Encounter    EncounterSpec
	Classic      *RunResult
	Forever      *RunResult
	ForeverInfo  *ForeverPlayerInfo
	TalentSource string
	TalentBuild  *TalentBuildRef
	Auto         *AutoRotationReport
	Err          string
	ElapsedMS    int64
}

// evaluate runs one spec preset on one encounter in the requested game(s).
func (b *bench) evaluate(e PresetEntry, race *RaceChoice, enc EncounterSpec, gameName string) (out SpecRun) {
	out = SpecRun{Entry: e, Race: race, Encounter: enc}
	req, err := b.classicRequest(e, race.race, enc, b.norm.Iterations)
	if err != nil {
		out.Err = err.Error()
		return out
	}
	if gameName == "classic" || gameName == "both" {
		r := runGame(game.Classic, req)
		out.Classic = &r
	}
	if gameName == "forever" || gameName == "both" {
		freq, info, source, ref, err := b.foreverRequest(req, e)
		if err != nil {
			out.Err = "forever setup: " + err.Error()
			return out
		}
		out.ForeverInfo, out.TalentSource, out.TalentBuild = &info, source, ref
		freq, rep, err := b.autoRotation(freq)
		if err != nil {
			out.Err = "forever auto-rotation: " + err.Error()
			return out
		}
		out.Auto = &rep
		out.ForeverInfo.AutoRotationAdded = []autoAddition{}
		for _, c := range rep.Candidates {
			if c.Accepted {
				out.ForeverInfo.AutoRotationAdded = append(out.ForeverInfo.AutoRotationAdded, autoAddition{ID: c.ID, Name: c.Name})
			}
		}
		r := runGame(game.Forever, freq)
		out.Forever = &r
	}
	errs := []string{}
	if out.Classic != nil && out.Classic.Error != "" {
		errs = append(errs, "classic: "+out.Classic.Error)
	}
	if out.Forever != nil && out.Forever.Error != "" {
		errs = append(errs, "forever: "+out.Forever.Error)
	}
	out.Err = strings.Join(errs, "; ")
	return out
}

// primary returns the result used for sorting (Forever when simmed).
func (r SpecRun) primary() *RunResult {
	if r.Forever != nil {
		return r.Forever
	}
	return r.Classic
}

// selectEntries returns the phase's matrix entries for the role and spec
// filter, split into simmable and excluded.
func selectEntries(phase Phase, role, specs, gearFill string) (ok, excluded []PresetEntry, err error) {
	m, err := presetMatrix()
	if err != nil {
		return nil, nil, err
	}
	only := map[string]bool{}
	for _, id := range strings.Split(specs, ",") {
		if id = strings.TrimSpace(id); id != "" {
			if _, found := findVariant(id); !found {
				return nil, nil, fmt.Errorf("unknown spec %q (see `foreverbench list`)", id)
			}
			only[id] = true
		}
	}
	for _, e := range m {
		if e.Phase != phase.String() || (role != "all" && e.Role != role) || (len(only) > 0 && !only[e.Spec]) {
			continue
		}
		e = e.withGearFill(gearFill)
		if e.Status == "ok" {
			ok = append(ok, e)
		} else {
			excluded = append(excluded, e)
		}
	}
	// An empty selection is not an error: callers report the excluded entries.
	return ok, excluded, nil
}

type ExcludedEntry struct {
	Spec       string `json:"spec"`
	Status     string `json:"status"`
	GearStatus string `json:"gear_status"`
	Reason     string `json:"reason"`
}

func excludedList(entries []PresetEntry) []ExcludedEntry {
	out := []ExcludedEntry{}
	for _, e := range entries {
		out = append(out, ExcludedEntry{e.Spec, e.Status, e.GearStatus, e.Reason})
	}
	return out
}

// runAll evaluates every entry x encounter with the configured parallelism.
// Races are chosen first (once per spec) so the best-race search is shared.
func (b *bench) runAll(entries []PresetEntry, encs []EncounterSpec, gameName string) [][]SpecRun {
	primary := game.Forever
	if gameName == "classic" {
		primary = game.Classic
	}
	needClassic := gameName != "forever"
	races := make([]*RaceChoice, len(entries))
	jobs := []func(){}
	for i, e := range entries {
		i, e := i, e
		jobs = append(jobs, func() { races[i] = b.chooseRace(e, needClassic, primary, encs[0]) })
	}
	runJobs(jobs, b.parallel)
	out := make([][]SpecRun, len(entries))
	jobs = jobs[:0]
	for i, e := range entries {
		out[i] = make([]SpecRun, len(encs))
		for j, enc := range encs {
			i, j, e, enc := i, j, e, enc
			jobs = append(jobs, func() {
				start := nowMS()
				out[i][j] = b.evaluate(e, races[i], enc, gameName)
				out[i][j].ElapsedMS = nowMS() - start
			})
		}
	}
	runJobs(jobs, b.parallel)
	return out
}

// ---------------------------------------------------------------- abilities

type AbilityChange struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Source     string  `json:"source,omitempty"`
	ClassicDPS float64 `json:"classic_dps"`
	ForeverDPS float64 `json:"forever_dps"`
	DeltaDPS   float64 `json:"delta_dps"`
	Status     string  `json:"status"` // changed | new_in_forever | gone_in_forever
}

func abilityChanges(d *fvDataset, classic, forever RunResult, n int) []AbilityChange {
	type key struct{ source, id string }
	all := map[key]*AbilityChange{}
	for _, a := range topAbilities(d, classic, 0) {
		all[key{a.Source, a.ID}] = &AbilityChange{ID: a.ID, Name: a.Name, Source: a.Source, ClassicDPS: a.DPS}
	}
	for _, a := range topAbilities(d, forever, 0) {
		k := key{a.Source, a.ID}
		if c, ok := all[k]; ok {
			c.ForeverDPS = a.DPS
		} else {
			all[k] = &AbilityChange{ID: a.ID, Name: a.Name, Source: a.Source, ForeverDPS: a.DPS}
		}
	}
	out := []AbilityChange{}
	for _, c := range all {
		c.DeltaDPS = round2(c.ForeverDPS - c.ClassicDPS)
		switch {
		case c.ClassicDPS == 0:
			c.Status = "new_in_forever"
		case c.ForeverDPS == 0:
			c.Status = "gone_in_forever"
		default:
			c.Status = "changed"
		}
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool {
		if math.Abs(out[i].DeltaDPS) != math.Abs(out[j].DeltaDPS) {
			return math.Abs(out[i].DeltaDPS) > math.Abs(out[j].DeltaDPS)
		}
		return out[i].ID < out[j].ID
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// ---------------------------------------------------------------- provenance

func provenanceBlock(root string, extra map[string]interface{}) map[string]interface{} {
	p := map[string]interface{}{
		"forever_ruleset_id":     foreverdata.RulesetID,
		"manifest_sha256":        foreverdata.ManifestSHA256(),
		"classic_ruleset_id":     game.ClassicRulesetID,
		"preset_snapshot_sha256": presetSnapshotSHA(),
		"preset_snapshot":        "cmd/foreverbench/presets/ui_presets.json (extract_presets.mjs from ui/<spec>/presets.ts)",
		"forever_data":           foreverDataRelPath,
		"root":                   root,
		"with_db":                core.WITH_DB,
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

func compactOrFull(p *game.Provenance, full bool) interface{} {
	if full {
		return p
	}
	return compact(p)
}

func rawJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
