package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wowsims/classic/assets/database"
	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
	googleProto "google.golang.org/protobuf/proto"
)

type SimSettings struct {
	Iterations   int32   `json:"iterations"`
	Duration     float64 `json:"duration_s"`
	Targets      int     `json:"targets"`
	TargetLevel  int32   `json:"target_level"`
	Seed         int64   `json:"seed"`
	ForeverMode  string  `json:"forever_mode"`
	AutoRotate   bool    `json:"forever_auto_rotation"`
	TalentPolicy string  `json:"forever_talent_policy"`
}

// ---------------------------------------------------------------- root/files

func resolveRoot(flagValue string) (string, error) {
	marker := filepath.FromSlash(foreverDataRelPath)
	check := func(dir string) bool {
		_, err := os.Stat(filepath.Join(dir, marker))
		return err == nil
	}
	if flagValue != "" {
		if !check(flagValue) {
			return "", fmt.Errorf("-root %s does not contain %s", flagValue, foreverDataRelPath)
		}
		return flagValue, nil
	}
	starts := []string{}
	if env := os.Getenv("FOREVERBENCH_ROOT"); env != "" {
		starts = append(starts, env)
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe))
	}
	for _, start := range starts {
		dir := start
		for i := 0; i < 6; i++ {
			if check(dir) {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", fmt.Errorf("could not find repo root (directory containing %s); pass -root or set FOREVERBENCH_ROOT", foreverDataRelPath)
}

func loadGearSet(root, dir, name string) (*proto.EquipmentSpec, error) {
	path := filepath.Join(root, "ui", dir, "gear_sets", name+".gear.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return core.EquipmentSpecFromJsonString(string(data)), nil
}

func loadApl(root, dir, name string) (*proto.APLRotation, error) {
	path := filepath.Join(root, "ui", dir, "apls", name+".apl.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return core.APLRotationFromJsonString(string(data)), nil
}

// ---------------------------------------------------------------- requests

func makeEncounter(s SimSettings) *proto.Encounter {
	targets := make([]*proto.Target, max(1, s.Targets))
	for i := range targets {
		t := googleProto.Clone(core.DefaultTargetProtoLvl60).(*proto.Target)
		t.Level = s.TargetLevel
		targets[i] = t
	}
	return &proto.Encounter{
		Duration:             s.Duration,
		ExecuteProportion_20: 0.2,
		ExecuteProportion_25: 0.25,
		ExecuteProportion_35: 0.35,
		Targets:              targets,
	}
}

// buildSpecRequest reproduces core.FullCharacterTestSuiteGenerator's defaultPlayer
// and SinglePlayerRaidProto with core.FullBuffs, using CLI encounter settings.
func buildSpecRequest(root string, spec SpecConfig, s SimSettings) (req *proto.RaidSimRequest, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("building request panicked: %v", r)
		}
	}()
	gear, err := loadGearSet(root, spec.GearDir, spec.GearSet)
	if err != nil {
		return nil, err
	}
	apl, err := loadApl(root, spec.AplDir, spec.Apl)
	if err != nil {
		return nil, err
	}
	player := core.WithSpec(&proto.Player{
		Name:               spec.ID,
		Class:              spec.Class,
		Race:               spec.Race,
		Equipment:          gear,
		Consumes:           googleProto.Clone(spec.Consumes).(*proto.Consumes),
		Buffs:              googleProto.Clone(core.FullBuffs.Player).(*proto.IndividualBuffs),
		TalentsString:      spec.Talents,
		Profession1:        proto.Profession_Engineering,
		Rotation:           apl,
		DistanceFromTarget: 5,
		ReactionTimeMs:     150,
		ChannelClipDelayMs: 50,
	}, spec.SpecOptions)
	player = googleProto.Clone(player).(*proto.Player) // detach shared spec option pointers
	raid := core.SinglePlayerRaidProto(player,
		googleProto.Clone(core.FullBuffs.Party).(*proto.PartyBuffs),
		googleProto.Clone(core.FullBuffs.Raid).(*proto.RaidBuffs),
		googleProto.Clone(core.FullBuffs.Debuffs).(*proto.Debuffs))
	return &proto.RaidSimRequest{
		Raid:      raid,
		Encounter: makeEncounter(s),
		SimOptions: &proto.SimOptions{
			Iterations: s.Iterations,
			RandomSeed: s.Seed,
		},
	}, nil
}

type ForeverPlayerInfo struct {
	PlayerIndex       int              `json:"player_index"`
	Mode              string           `json:"mode"`
	TalentSource      string           `json:"talent_source"`
	ClassicTalents    string           `json:"classic_talents,omitempty"`
	ClassicPoints     int32            `json:"classic_points,omitempty"`
	ForeverTalents    map[string]int32 `json:"forever_talents"`
	ForeverF1         string           `json:"forever_talent_string"`
	ForeverPoints     int32            `json:"forever_points"`
	DroppedClassic    []string         `json:"dropped_classic_talents,omitempty"`
	FillerPoints      map[string]int32 `json:"filler_points,omitempty"`
	Mechanics         []string         `json:"mechanics"`
	AutoRotationAdded []autoAddition   `json:"auto_rotation_additions"`
}

func droppedClassicFields(d *fvDataset, c proto.Class, text string, kept map[string]int32) []string {
	layout := d.ClassicLayout[foreverdata.ClassName(c)]
	keptFields := map[string]bool{}
	for _, r := range d.classRecords(c) {
		if kept[r.ID] > 0 {
			keptFields[r.ClassicField] = true
		}
	}
	out := []string{}
	for i, tree := range strings.Split(text, "-") {
		for j, ch := range tree {
			if i < len(layout) && j < len(layout[i]) && ch > '0' && ch <= '9' && !keptFields[layout[i][j]] {
				out = append(out, fmt.Sprintf("%s:%c", layout[i][j], ch))
			}
		}
	}
	return out
}

// TalentPolicy selects how Forever talents are derived from a Classic build.
type TalentPolicy struct {
	Name     string           // repair (default), repair-nofill, migrate, sample, override
	Override map[string]int32 // for "override": applies to player 0
}

func (d *fvDataset) talentsFor(p *proto.Player, policy TalentPolicy, first bool, info *ForeverPlayerInfo) map[string]int32 {
	info.ClassicTalents = p.TalentsString
	info.ClassicPoints = classicPoints(p.TalentsString)
	var talents map[string]int32
	switch {
	case policy.Name == "override" && first:
		info.TalentSource = "override: user-supplied F1 Forever talent string"
		talents = policy.Override
	case policy.Name == "migrate":
		info.TalentSource = "migrate: UI migrateClassicTalents (Classic nodes kept only if still legal; unspent points left unspent)"
		talents = d.migrateClassicTalents(p.Class, p.TalentsString)
	case policy.Name == "repair-nofill":
		info.TalentSource = "repair-nofill: every Classic talent with a Forever node, row gates filled; leftover points unspent"
		talents, info.FillerPoints = d.repairClassicTalents(p.Class, p.TalentsString, false)
	case policy.Name == "sample":
		probe := d.migrateClassicTalents(p.Class, p.TalentsString)
		tree := d.newBuilder(p.Class).primaryTree(probe)
		info.TalentSource = "sample: UI sampleForeverBuild for primary tree " + tree
		talents = d.sampleBuild(p.Class, tree)
	default:
		info.TalentSource = "repair: every Classic talent with a Forever node, row gates filled, leftover points spent in the primary tree using the UI sample-build order, preferring nodes that add no auto-cast action"
		talents, info.FillerPoints = d.repairClassicTalents(p.Class, p.TalentsString, true)
	}
	info.DroppedClassic = droppedClassicFields(d, p.Class, p.TalentsString, talents)
	return talents
}

// toForever converts a Classic request into the request the Forever UI would sim.
// Players that already carry Forever options keep them (unless overridden).
func toForever(d *fvDataset, classic *proto.RaidSimRequest, mode proto.ForeverMode, policy TalentPolicy, autoRotate bool) (*proto.RaidSimRequest, []ForeverPlayerInfo, error) {
	req := googleProto.Clone(classic).(*proto.RaidSimRequest)
	infos := []ForeverPlayerInfo{}
	idx := 0
	type ref struct{ party, player int }
	refs := []ref{}
	for pi, party := range req.Raid.GetParties() {
		for pj, p := range party.GetPlayers() {
			if p == nil || p.Class == proto.Class_ClassUnknown {
				continue
			}
			info := ForeverPlayerInfo{PlayerIndex: pi*5 + pj}
			if p.Forever == nil {
				f := d.defaultOptions(p.Class, p.Race, mode)
				f.Talents = d.talentsFor(p, policy, idx == 0, &info)
				p.Forever = f
				p.TalentsString = ""
			} else {
				info.TalentSource = "request forever options"
				if policy.Name == "override" && idx == 0 {
					p.Forever.Talents = policy.Override
					info.TalentSource = "override: user-supplied F1 Forever talent string"
				}
			}
			p.Database = nil
			if err := foreverdata.Validate(p); err != nil {
				return nil, nil, fmt.Errorf("player %d Forever options invalid: %w", info.PlayerIndex, err)
			}
			info.Mode = "BEST_GUESS"
			if foreverdata.IsStrict(p.Forever) {
				info.Mode = "STRICT"
			}
			info.ForeverTalents = p.Forever.Talents
			info.ForeverF1 = d.encodeTalents(p.Class, p.Forever.Talents)
			info.ForeverPoints = sumPoints(p.Forever.Talents)
			info.Mechanics = p.Forever.Mechanics
			info.AutoRotationAdded = []autoAddition{}
			infos = append(infos, info)
			refs = append(refs, ref{pi, pj})
			idx++
		}
	}
	if autoRotate && len(refs) > 0 {
		stats, err := computeAllSpells(req)
		if err != nil {
			return nil, nil, err
		}
		for i, r := range refs {
			p := req.Raid.Parties[r.party].Players[r.player]
			spells := stats[[2]int{r.party, r.player}]
			if p.Rotation == nil || p.Rotation.Type != proto.APLRotation_TypeAPL {
				continue
			}
			rot, added, err := d.withForeverRotation(p.Rotation, p.Forever, p.Class, spells, len(req.Encounter.GetTargets()))
			if err != nil {
				return nil, nil, err
			}
			p.Rotation = rot
			infos[i].AutoRotationAdded = added
		}
	}
	return req, infos, nil
}

func computeAllSpells(req *proto.RaidSimRequest) (out map[[2]int][]*proto.SpellStats, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("compute stats panicked: %v", r)
		}
	}()
	result := core.ComputeStats(&proto.ComputeStatsRequest{
		Raid:      googleProto.Clone(req.Raid).(*proto.Raid),
		Encounter: googleProto.Clone(req.Encounter).(*proto.Encounter),
	})
	if result.ErrorResult != "" {
		return nil, errors.New("compute stats: " + result.ErrorResult)
	}
	out = map[[2]int][]*proto.SpellStats{}
	for pi, party := range result.GetRaidStats().GetParties() {
		for pj, player := range party.GetPlayers() {
			out[[2]int{pi, pj}] = player.GetMetadata().GetSpells()
		}
	}
	return out, nil
}

// toClassic strips Forever options, bridging record-keyed talents back to the
// Classic typed talent string via foreverdata.Prepare when no Classic string exists.
func toClassic(req *proto.RaidSimRequest) (*proto.RaidSimRequest, []string, error) {
	out := googleProto.Clone(req).(*proto.RaidSimRequest)
	notes := []string{}
	for _, party := range out.Raid.GetParties() {
		for _, p := range party.GetPlayers() {
			if p == nil || p.Forever == nil {
				continue
			}
			prepared, err := foreverdata.Prepare(p)
			if err != nil {
				return nil, nil, err
			}
			p.TalentsString = prepared.TalentsString
			p.Forever = nil
			notes = append(notes, fmt.Sprintf("classic run for %q uses the Classic talent bridge of its Forever talents: %s", p.Name, p.TalentsString))
		}
	}
	return out, notes, nil
}

// ---------------------------------------------------------------- running

type Ability struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Source string  `json:"source,omitempty"`
	DPS    float64 `json:"dps"`
	Pct    float64 `json:"pct"`
	Casts  int64   `json:"casts"`
}

type RunResult struct {
	Game       string           `json:"game"`
	DPS        float64          `json:"dps"`
	Stdev      float64          `json:"stdev"`
	Min        float64          `json:"min"`
	Max        float64          `json:"max"`
	StdErr     float64          `json:"stderr"`
	Iterations int32            `json:"iterations"`
	ElapsedMS  int64            `json:"elapsed_ms"`
	Error      string           `json:"error,omitempty"`
	Provenance *game.Provenance `json:"-"`
	result     *proto.RaidSimResult
}

func runGame(version game.Version, req *proto.RaidSimRequest) RunResult {
	start := time.Now()
	out := RunResult{Game: string(version), Iterations: req.GetSimOptions().GetIterations()}
	func() {
		defer func() {
			if r := recover(); r != nil {
				out.Error = fmt.Sprintf("panic: %v", r)
			}
		}()
		sel := game.Selection{Version: version, Discovery: version == game.Forever}
		result, prov, err := game.RunRaidSim(sel, req)
		out.Provenance = &prov
		if err != nil {
			out.Error = firstLine(err.Error())
			return
		}
		out.result = result
		dps := result.GetRaidMetrics().GetDps()
		out.DPS = round2(dps.GetAvg())
		out.Stdev = round2(dps.GetStdev())
		out.Min = round2(dps.GetMin())
		out.Max = round2(dps.GetMax())
		if out.Iterations > 0 {
			out.StdErr = round2(dps.GetStdev() / math.Sqrt(float64(out.Iterations)))
		}
	}()
	out.ElapsedMS = time.Since(start).Milliseconds()
	return out
}

func firstLine(s string) string {
	// Sim panics include a stack trace after the message; keep the message.
	if i := strings.Index(s, "\nStack Trace:"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

var (
	nameOnce   sync.Once
	spellNames map[int32]string
	itemNames  map[int32]string
)

func loadNames() {
	nameOnce.Do(func() {
		spellNames, itemNames = map[int32]string{}, map[int32]string{}
		defer func() { recover() }()
		db := database.Load()
		for _, s := range db.SpellIcons {
			if _, ok := spellNames[s.Id]; !ok {
				spellNames[s.Id] = s.Name
			}
		}
		for _, s := range db.ItemIcons {
			itemNames[s.Id] = s.Name
		}
		for _, it := range db.Items {
			if _, ok := itemNames[it.Id]; !ok {
				itemNames[it.Id] = it.Name
			}
		}
	})
}

func describeAction(d *fvDataset, id *proto.ActionID) (string, string) {
	loadNames()
	tagSuffix := ""
	if id.GetTag() != 0 {
		tagSuffix = fmt.Sprintf(" [tag %d]", id.GetTag())
	}
	switch raw := id.GetRawId().(type) {
	case *proto.ActionID_SpellId:
		name := spellNames[raw.SpellId]
		if name == "" {
			name = fmt.Sprintf("Spell %d", raw.SpellId)
		}
		if id.GetRank() > 0 {
			name += fmt.Sprintf(" (Rank %d)", id.GetRank())
		}
		return fmt.Sprintf("spell:%d%s", raw.SpellId, rankTag(id)), name + tagSuffix
	case *proto.ActionID_ItemId:
		name := itemNames[raw.ItemId]
		if name == "" {
			name = fmt.Sprintf("Item %d", raw.ItemId)
		}
		return fmt.Sprintf("item:%d%s", raw.ItemId, rankTag(id)), name + tagSuffix
	case *proto.ActionID_OtherId:
		switch raw.OtherId {
		case proto.OtherAction_OtherActionAttack:
			switch id.GetTag() {
			case 1:
				return "other:attack:1", "Melee (main hand)"
			case 2:
				return "other:attack:2", "Melee (off hand)"
			case 3:
				return "other:attack:3", "Melee (extra attacks)"
			}
			return fmt.Sprintf("other:attack:%d", id.GetTag()), "Melee" + tagSuffix
		case proto.OtherAction_OtherActionShoot:
			return "other:shoot", "Auto Shot"
		case proto.OtherAction_OtherActionForever:
			name := ""
			if d != nil {
				name = d.actionName(id.GetTag())
			}
			if name == "" {
				name = "Forever action"
			}
			return fmt.Sprintf("forever:%d", id.GetTag()), name + " (Forever)"
		}
		return fmt.Sprintf("other:%s:%d", raw.OtherId.String(), id.GetTag()), strings.TrimPrefix(raw.OtherId.String(), "OtherAction") + tagSuffix
	}
	return "unknown", "Unknown"
}

func rankTag(id *proto.ActionID) string {
	s := ""
	if id.GetRank() > 0 {
		s += fmt.Sprintf(":r%d", id.GetRank())
	}
	if id.GetTag() != 0 {
		s += fmt.Sprintf(":t%d", id.GetTag())
	}
	return s
}

func topAbilities(d *fvDataset, r RunResult, n int) []Ability {
	if r.result == nil {
		return []Ability{}
	}
	type acc struct {
		Ability
		dmg float64
	}
	byKey := map[string]*acc{}
	total := 0.0
	add := func(unit *proto.UnitMetrics, source string) {
		for _, a := range unit.GetActions() {
			dmg, casts := 0.0, int64(0)
			for _, t := range a.GetTargets() {
				dmg += t.GetDamage()
				casts += int64(t.GetCasts())
			}
			if dmg <= 0 {
				continue
			}
			id, name := describeAction(d, a.GetId())
			key := source + "|" + id
			e := byKey[key]
			if e == nil {
				e = &acc{Ability: Ability{ID: id, Name: name, Source: source}}
				byKey[key] = e
			}
			e.dmg += dmg
			e.Casts += casts
			total += dmg
		}
	}
	for _, party := range r.result.GetRaidMetrics().GetParties() {
		for _, p := range party.GetPlayers() {
			add(p, "")
			for _, pet := range p.GetPets() {
				add(pet, "pet:"+pet.GetName())
			}
		}
	}
	list := []Ability{}
	for _, e := range byKey {
		pct := e.dmg / total
		e.Pct = round2(pct * 100)
		e.DPS = round2(pct * r.DPS)
		if r.Iterations > 0 {
			e.Casts = int64(math.Round(float64(e.Casts) / float64(r.Iterations)))
		}
		list = append(list, e.Ability)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].DPS != list[j].DPS {
			return list[i].DPS > list[j].DPS
		}
		return list[i].ID < list[j].ID
	})
	if n > 0 && len(list) > n {
		list = list[:n]
	}
	return list
}

// ---------------------------------------------------------------- provenance

type CompactProvenance struct {
	Game           game.Version `json:"game"`
	RulesetID      string       `json:"ruleset_id"`
	UpstreamCommit string       `json:"upstream_commit"`
	ManifestSHA256 string       `json:"manifest_sha256,omitempty"`
	PartialRuleset bool         `json:"partial_ruleset,omitempty"`
	Experimental   bool         `json:"experimental_estimated_ranks,omitempty"`
	Modes          []string     `json:"modes,omitempty"`
	MechanicCount  int          `json:"mechanic_count"`
	ActiveTalents  int          `json:"active_talents"`
	InactiveTalent int          `json:"inactive_talents"`
	Predicted      int          `json:"predicted_talent_values"`
}

func compact(p *game.Provenance) *CompactProvenance {
	if p == nil {
		return nil
	}
	c := &CompactProvenance{Game: p.Game, RulesetID: p.RulesetID, UpstreamCommit: p.UpstreamCommit, ManifestSHA256: p.ManifestSHA256,
		PartialRuleset: p.PartialRuleset, Experimental: p.Experimental, Modes: p.Modes, MechanicCount: len(p.MechanicIDs)}
	for _, t := range p.TalentEvidence {
		if t.ImplementationState == "ACTIVE" {
			c.ActiveTalents++
		} else {
			c.InactiveTalent++
		}
		if t.ValueConfidence == "PREDICTED" || len(t.PredictedComponents) > 0 {
			c.Predicted++
		}
	}
	return c
}
