package main

// predict: what should ONE ability do for THIS character against THIS target.
//
// The command builds the same normalized request as rank/compare for the spec,
// applies the requested character/target/buff state, runs the real simulator with
// the spec's rotation and reads the per-action metrics of the requested ability.
// Nothing is re-derived from formulas here.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wowsims/classic/sim/core"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/core/simsignals"
	"github.com/wowsims/classic/sim/game"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// PredictRequest is the -request file form of the predict flags. Flags given on
// the command line win over the file.
type PredictRequest struct {
	Spec        string             `json:"spec"`
	Phase       string             `json:"phase,omitempty"`
	Game        string             `json:"game,omitempty"`
	Ability     string             `json:"ability"`
	Stats       map[string]float64 `json:"stats,omitempty"`
	Talents     string             `json:"talents,omitempty"`
	Race        string             `json:"race,omitempty"`
	Gear        string             `json:"gear,omitempty"`
	TargetLevel *int               `json:"target_level,omitempty"`
	TargetArmor *float64           `json:"target_armor,omitempty"`
	Buffs       string             `json:"buffs,omitempty"`
	Debuffs     string             `json:"debuffs,omitempty"`
	Parameters  map[string]float64 `json:"parameters,omitempty"`
	Iterations  int                `json:"iterations,omitempty"`
}

// PredictAbility is the measured behaviour of one action of the player (or a pet).
type PredictAbility struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source,omitempty"`
	School string `json:"school,omitempty"`
	Melee  bool   `json:"is_melee"`

	CastsPerFight float64 `json:"casts_per_fight"`
	LandedPerCast float64 `json:"landed_hits_per_cast,omitempty"`
	Counts        struct {
		Casts        int64 `json:"casts"`
		Landed       int64 `json:"landed_hits"`
		Hits         int64 `json:"hits"`
		Crits        int64 `json:"crits"`
		Glances      int64 `json:"glances,omitempty"`
		Blocks       int64 `json:"blocks,omitempty"`
		BlockedCrits int64 `json:"blocked_crits,omitempty"`
		Misses       int64 `json:"misses,omitempty"`
		Dodges       int64 `json:"dodges,omitempty"`
		Parries      int64 `json:"parries,omitempty"`
		PartialHits  int64 `json:"partially_resisted_hits,omitempty"`
		Ticks        int64 `json:"ticks,omitempty"`
		CritTicks    int64 `json:"crit_ticks,omitempty"`
	} `json:"counts"`

	AvgHit            *float64 `json:"avg_hit,omitempty"`            // non-crit, non-glance, non-block direct hit (partial resists included)
	AvgHitUnresisted  *float64 `json:"avg_hit_unresisted,omitempty"` // same, excluding partially resisted hits
	AvgCrit           *float64 `json:"avg_crit,omitempty"`
	AvgCritUnresisted *float64 `json:"avg_crit_unresisted,omitempty"`
	AvgGlance         *float64 `json:"avg_glance,omitempty"`
	AvgBlock          *float64 `json:"avg_blocked_hit,omitempty"`
	AvgTick           *float64 `json:"avg_tick,omitempty"`
	AvgCritTick       *float64 `json:"avg_crit_tick,omitempty"`
	AvgDamagePerCast  *float64 `json:"avg_damage_per_cast,omitempty"`

	// TablePct is the outcome distribution of direct attempts, in percent.
	TablePct map[string]float64 `json:"hit_table_pct,omitempty"`

	DPS      float64 `json:"dps"`
	SharePct float64 `json:"share_pct"`
}

var rankSuffix = regexp.MustCompile(` \(Rank \d+\)| \[tag -?\d+\]| \(Forever\)`)

func cmdPredict(args []string, stderr io.Writer) (interface{}, error) {
	start := time.Now()
	fs, c := newFlagSet("predict", stderr)
	c.iterations = 2000
	fs.Lookup("iterations").DefValue = "2000"
	in := PredictRequest{}
	requestPath := fs.String("request", "", "JSON file with the same options (spec, phase, game, ability, stats{}, talents, race, gear, target_level, target_armor, buffs, debuffs, parameters{}, iterations); flags win")
	fs.StringVar(&in.Spec, "spec", "", "spec id from `list`")
	fs.StringVar(&in.Game, "game", "forever", "forever, classic or both")
	fs.StringVar(&in.Ability, "ability", "", "spell id, action id (spell:23894, other:attack:1) or case-insensitive action name")
	statsFlag := fs.String("stats", "", "FINAL character-sheet stats to force, e.g. attack_power=1500,melee_crit=28.5,melee_hit=6,spell_power=600 (keys: see -list; crit/hit in percent)")
	fs.StringVar(&in.Talents, "talents", "", "talents: a Classic talent string, or F1:<CLASS>:a-b-c for the Forever build")
	fs.StringVar(&in.Race, "race", "", "race name (default: the spec's normalized race)")
	fs.StringVar(&in.Gear, "gear", "", "UI gear preset label or file of the spec instead of the phase BiS set (see -list)")
	targetArmor := fs.Float64("target-armor", -1, "explicit base armor of the target before debuffs (default: bench target)")
	fs.StringVar(&in.Buffs, "buffs", "", "comma list of RaidBuffs/PartyBuffs/IndividualBuffs proto fields with optional =value on top of the normalized full raid; start with none to begin empty")
	fs.StringVar(&in.Debuffs, "debuffs", "", "comma list of Debuffs proto fields with optional =value (sunder_armor, curse_of_elements=false, ...); start with none to begin empty")
	paramsFlag := fs.String("parameters", "", "Forever parameters key=value,... (foreverdata.ParameterCatalog), e.g. core.combat.dodge_offset=-0.01")
	list := fs.Bool("list", false, "list the spec's abilities, the stat keys and the settable buff/debuff fields instead of predicting")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	if *requestPath != "" {
		raw, err := os.ReadFile(*requestPath)
		if err != nil {
			return nil, err
		}
		file := PredictRequest{}
		if err := json.Unmarshal(raw, &file); err != nil {
			return nil, fmt.Errorf("-request: %w", err)
		}
		if err := mergePredictFile(&in, file, c); err != nil {
			return nil, err
		}
	}
	var err error
	if *statsFlag != "" {
		if in.Stats, err = parseKeyValues(*statsFlag); err != nil {
			return nil, fmt.Errorf("-stats: %w", err)
		}
	}
	if *paramsFlag != "" {
		if in.Parameters, err = parseKeyValues(*paramsFlag); err != nil {
			return nil, fmt.Errorf("-parameters: %w", err)
		}
	}
	if *targetArmor >= 0 {
		in.TargetArmor = targetArmor
	}
	in.Phase, in.Iterations = c.phaseValue.String(), c.iterations
	level := c.targetLevel
	in.TargetLevel = &level
	return runPredict(c, in, *list, start)
}

// mergePredictFile copies file values for every option not set on the command line.
func mergePredictFile(in *PredictRequest, file PredictRequest, c *commonFlags) error {
	pick := func(flagName string, dst *string, value string) {
		if !c.set[flagName] && value != "" {
			*dst = value
		}
	}
	pick("spec", &in.Spec, file.Spec)
	pick("game", &in.Game, file.Game)
	pick("ability", &in.Ability, file.Ability)
	pick("talents", &in.Talents, file.Talents)
	pick("race", &in.Race, file.Race)
	pick("gear", &in.Gear, file.Gear)
	pick("buffs", &in.Buffs, file.Buffs)
	pick("debuffs", &in.Debuffs, file.Debuffs)
	in.Stats, in.Parameters, in.TargetArmor = file.Stats, file.Parameters, file.TargetArmor
	if !c.set["phase"] && file.Phase != "" {
		p, err := parsePhase(file.Phase)
		if err != nil {
			return err
		}
		c.phase, c.phaseValue = file.Phase, p
	}
	if !c.set["iterations"] && file.Iterations > 0 {
		c.iterations = file.Iterations
	}
	if !c.set["target-level"] && file.TargetLevel != nil {
		c.targetLevel = *file.TargetLevel
	}
	return nil
}

func parseKeyValues(s string) (map[string]float64, error) {
	out := map[string]float64{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("%q is not key=value", part)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("%q: value is not a number", part)
		}
		out[strings.TrimSpace(key)] = v
	}
	return out, nil
}

func runPredict(c *commonFlags, in PredictRequest, list bool, start time.Time) (interface{}, error) {
	if in.Spec == "" {
		return nil, fmt.Errorf("predict needs -spec (see `foreverbench list`)")
	}
	if in.Ability == "" && !list {
		return nil, fmt.Errorf("predict needs -ability (use -list to see the spec's abilities)")
	}
	games := []game.Version{}
	switch in.Game {
	case "forever", "":
		games = append(games, game.Forever)
	case "classic":
		games = append(games, game.Classic)
	case "both":
		games = append(games, game.Classic, game.Forever)
	default:
		return nil, fmt.Errorf("-game must be forever, classic or both")
	}
	for key := range in.Stats {
		if _, ok := foreverdata.StatKeyIndex(key); !ok {
			return nil, fmt.Errorf("unknown stat %q; stat keys: %s", key, strings.Join(foreverdata.StatKeys, ", "))
		}
	}
	for key, value := range in.Parameters {
		if !foreverdata.ParameterInBounds(key, value) {
			return nil, fmt.Errorf("parameter %s=%v is unknown or outside its registered bounds", key, value)
		}
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	caveats := []string{}
	e, err := matrixEntry(in.Spec, c.phaseValue)
	if err != nil {
		return nil, err
	}
	e = e.withGearFill(c.gearFill)
	if e.Status != "ok" {
		return nil, fmt.Errorf("%s has no simmable preset for %s: %s", e.Spec, e.Phase, e.Reason)
	}
	if in.Gear != "" {
		if e, err = withGearPreset(e, in.Gear); err != nil {
			return nil, err
		}
	}
	if strings.HasPrefix(in.Talents, "F1:") {
		b.builds = append(b.builds, TalentBuild{Spec: e.Spec, Name: "command line", Talents: in.Talents, Source: "-talents", Game: "forever", Phase: e.Phase})
	} else if in.Talents != "" {
		e.TalentsString, e.TalentPreset, e.TalentSource = in.Talents, "command line", "-talents"
		if _, hasForever := pickBuild(b.builds, e.Spec, c.phaseValue, foreverdata.RulesetID); hasForever && in.Game != "classic" {
			caveats = append(caveats, "-talents is a Classic talent string; the Forever run still uses the spec's Forever build (pass an F1: string to change it)")
		}
	}
	needClassic := in.Game != "forever" && in.Game != ""
	if ok, _ := b.withTalents([]PresetEntry{e}, nil, map[bool]string{true: "both", false: "forever"}[needClassic]); len(ok) == 0 {
		return nil, fmt.Errorf("%s has no talents for -game %s (no UI preset and no optimized build)", e.Spec, in.Game)
	}
	e = b.resolveTalents(e)
	enc := EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration}
	race := b.chooseRace(e, needClassic, games[len(games)-1], enc)
	if in.Race != "" {
		r, ok := parseRace(in.Race)
		if !ok {
			return nil, fmt.Errorf("unknown race %q", in.Race)
		}
		if !b.d.raceAllowed(e.variant.Class, r, needClassic) {
			return nil, fmt.Errorf("race %s is not legal for %s in -game %s", raceName(r), e.Class, in.Game)
		}
		race = &RaceChoice{Policy: "fixed", Race: raceName(r), Reason: "-race", race: r}
	}

	base, err := b.classicRequest(e, race.race, enc, int32(c.iterations))
	if err != nil {
		return nil, err
	}
	if err := applyBuffLists(base, in.Buffs, in.Debuffs); err != nil {
		return nil, err
	}
	if in.TargetArmor != nil {
		for _, t := range base.Encounter.Targets {
			t.Stats[proto.Stat_StatArmor] = *in.TargetArmor
		}
	}

	if list {
		return predictListing(b, e, base, games[len(games)-1])
	}

	results := map[string]interface{}{}
	for _, g := range games {
		out, err := b.predictGame(g, e, base, in)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", g, err)
		}
		results[string(g)] = out
	}
	if in.TargetArmor == nil {
		caveats = append(caveats, fmt.Sprintf("target uses the bench default base armor (%.0f); pass -target-armor (e.g. 3731 for a level 63 raid boss) when comparing physical damage with logs",
			base.Encounter.Targets[0].Stats[proto.Stat_StatArmor]))
	}
	return map[string]interface{}{
		"command":     "predict",
		"spec":        e.Spec,
		"phase":       c.phaseValue.String(),
		"ability":     in.Ability,
		"race":        race,
		"gear":        map[string]interface{}{"label": e.GearLabel, "file": e.GearFile, "status": e.GearStatus},
		"encounter":   enc,
		"iterations":  c.iterations,
		"seed":        c.seed,
		"request":     in,
		"results":     results,
		"caveats":     caveats,
		"provenance":  provenanceBlock(root, map[string]interface{}{"normalization": normalizationDoc(b.norm, e.Role)}),
		"elapsed_ms":  time.Since(start).Milliseconds(),
		"description": "Measured with the real simulator and the spec's rotation; averages are per landed result of that kind, hit_table_pct is over direct attempts.",
	}, nil
}

func parseRace(name string) (proto.Race, bool) {
	want := strings.ToLower(strings.NewReplacer(" ", "", "_", "", "-", "").Replace(name))
	for v, n := range proto.Race_name {
		if v != 0 && strings.ToLower(strings.TrimPrefix(n, "Race")) == want {
			return proto.Race(v), true
		}
	}
	return proto.Race_RaceUnknown, false
}

// withGearPreset swaps the entry's gear for another UI gear preset of the same spec.
func withGearPreset(e PresetEntry, label string) (PresetEntry, error) {
	s, err := uiPresets()
	if err != nil {
		return e, err
	}
	cands, _ := s.gearCandidates(e.variant)
	names := []string{}
	for _, cand := range cands {
		names = append(names, cand.label)
		if strings.EqualFold(cand.label, label) || strings.EqualFold(cand.file, label) {
			if _, ok := presetFile(cand.file); !ok {
				return e, fmt.Errorf("gear file %s missing from preset snapshot", cand.file)
			}
			e.GearLabel, e.GearFile, e.GearSource, e.GearStatus = cand.label, cand.file, cand.source, "ok"
			e.gearJSON, e.BorrowedFrom, e.PatchedFrom, e.PatchedSlots = nil, "", "", nil
			return e, nil
		}
	}
	return e, fmt.Errorf("unknown gear preset %q for %s; available: %s", label, e.Spec, strings.Join(names, " | "))
}

// ---------------------------------------------------------------- buffs / debuffs

func buffMessages(req *proto.RaidSimRequest) (buffs, debuffs []protoreflect.Message) {
	party := req.Raid.Parties[0]
	player := party.Players[0]
	if req.Raid.Buffs == nil {
		req.Raid.Buffs = &proto.RaidBuffs{}
	}
	if party.Buffs == nil {
		party.Buffs = &proto.PartyBuffs{}
	}
	if player.Buffs == nil {
		player.Buffs = &proto.IndividualBuffs{}
	}
	if req.Raid.Debuffs == nil {
		req.Raid.Debuffs = &proto.Debuffs{}
	}
	return []protoreflect.Message{req.Raid.Buffs.ProtoReflect(), party.Buffs.ProtoReflect(), player.Buffs.ProtoReflect()},
		[]protoreflect.Message{req.Raid.Debuffs.ProtoReflect()}
}

func fullBuffDefaults() []protoreflect.Message {
	return []protoreflect.Message{core.FullRaidBuffs.ProtoReflect(), core.FullPartyBuffs.ProtoReflect(),
		core.FullIndividualBuffs.ProtoReflect(), core.FullDebuffs.ProtoReflect()}
}

// applyBuffLists applies -buffs / -debuffs on top of the normalized full raid.
func applyBuffLists(req *proto.RaidSimRequest, buffList, debuffList string) error {
	buffs, debuffs := buffMessages(req)
	if err := applyFieldList("-buffs", buffs, buffList); err != nil {
		return err
	}
	return applyFieldList("-debuffs", debuffs, debuffList)
}

func applyFieldList(flagName string, msgs []protoreflect.Message, list string) error {
	for i, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.EqualFold(part, "none") {
			if i != 0 {
				return fmt.Errorf("%s: none must come first", flagName)
			}
			for _, m := range msgs {
				m.Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
					m.Clear(fd)
					return true
				})
			}
			continue
		}
		name, value, hasValue := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		found := false
		for _, m := range msgs {
			fd := findField(m.Descriptor(), name)
			if fd == nil {
				continue
			}
			found = true
			v, err := fieldValue(fd, strings.TrimSpace(value), hasValue)
			if err != nil {
				return fmt.Errorf("%s %s: %w", flagName, name, err)
			}
			m.Set(fd, v)
		}
		if !found {
			names := []string{}
			for _, m := range msgs {
				for j := 0; j < m.Descriptor().Fields().Len(); j++ {
					names = append(names, string(m.Descriptor().Fields().Get(j).Name()))
				}
			}
			sort.Strings(names)
			return fmt.Errorf("%s: unknown field %q; fields: %s", flagName, name, strings.Join(names, ", "))
		}
	}
	return nil
}

// findField resolves a snake_case or camelCase proto field name.
func findField(md protoreflect.MessageDescriptor, name string) protoreflect.FieldDescriptor {
	if fd := md.Fields().ByName(protoreflect.Name(name)); fd != nil {
		return fd
	}
	if fd := md.Fields().ByJSONName(name); fd != nil {
		return fd
	}
	flat := strings.ToLower(strings.ReplaceAll(name, "_", ""))
	for i := 0; i < md.Fields().Len(); i++ {
		if fd := md.Fields().Get(i); strings.ToLower(strings.ReplaceAll(string(fd.Name()), "_", "")) == flat {
			return fd
		}
	}
	return nil
}

// fieldValue parses a scalar/enum value. Without =value a bool becomes true and
// other kinds take the full-raid default (else the highest enum value / 1).
func fieldValue(fd protoreflect.FieldDescriptor, value string, hasValue bool) (protoreflect.Value, error) {
	if fd.IsList() || fd.IsMap() || fd.Kind() == protoreflect.MessageKind {
		return protoreflect.Value{}, fmt.Errorf("only scalar and enum fields can be set")
	}
	if !hasValue {
		if fd.Kind() == protoreflect.BoolKind {
			return protoreflect.ValueOfBool(true), nil
		}
		for _, m := range fullBuffDefaults() {
			if m.Descriptor() == fd.ContainingMessage() && m.Has(fd) {
				return m.Get(fd), nil
			}
		}
		if fd.Kind() == protoreflect.EnumKind {
			best := protoreflect.EnumNumber(0)
			for i := 0; i < fd.Enum().Values().Len(); i++ {
				best = max(best, fd.Enum().Values().Get(i).Number())
			}
			return protoreflect.ValueOfEnum(best), nil
		}
		value = "1"
	}
	switch fd.Kind() {
	case protoreflect.BoolKind:
		v, err := strconv.ParseBool(value)
		return protoreflect.ValueOfBool(v), err
	case protoreflect.EnumKind:
		values := fd.Enum().Values()
		if n, err := strconv.Atoi(value); err == nil {
			if values.ByNumber(protoreflect.EnumNumber(n)) == nil {
				return protoreflect.Value{}, fmt.Errorf("enum %s has no value %d", fd.Enum().Name(), n)
			}
			return protoreflect.ValueOfEnum(protoreflect.EnumNumber(n)), nil
		}
		names := []string{}
		for i := 0; i < values.Len(); i++ {
			n := string(values.Get(i).Name())
			names = append(names, n)
			if strings.EqualFold(n, value) || strings.HasSuffix(strings.ToLower(n), strings.ToLower(value)) {
				return protoreflect.ValueOfEnum(values.Get(i).Number()), nil
			}
		}
		return protoreflect.Value{}, fmt.Errorf("unknown enum value %q; values: %s", value, strings.Join(names, ", "))
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		v, err := strconv.ParseInt(value, 10, 32)
		return protoreflect.ValueOfInt32(int32(v)), err
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		v, err := strconv.ParseInt(value, 10, 64)
		return protoreflect.ValueOfInt64(v), err
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		v, err := strconv.ParseUint(value, 10, 32)
		return protoreflect.ValueOfUint32(uint32(v)), err
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		v, err := strconv.ParseUint(value, 10, 64)
		return protoreflect.ValueOfUint64(v), err
	case protoreflect.FloatKind:
		v, err := strconv.ParseFloat(value, 32)
		return protoreflect.ValueOfFloat32(float32(v)), err
	case protoreflect.DoubleKind:
		v, err := strconv.ParseFloat(value, 64)
		return protoreflect.ValueOfFloat64(v), err
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(value), nil
	}
	return protoreflect.Value{}, fmt.Errorf("unsupported field kind %s", fd.Kind())
}

// activeFields lists the populated fields of the messages as name -> value.
func activeFields(msgs []protoreflect.Message) map[string]interface{} {
	out := map[string]interface{}{}
	for _, m := range msgs {
		m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			switch {
			case fd.IsList() || fd.IsMap() || fd.Kind() == protoreflect.MessageKind:
				out[string(fd.Name())] = "set"
			case fd.Kind() == protoreflect.EnumKind:
				if ev := fd.Enum().Values().ByNumber(v.Enum()); ev != nil {
					out[string(fd.Name())] = string(ev.Name())
				} else {
					out[string(fd.Name())] = int32(v.Enum())
				}
			default:
				out[string(fd.Name())] = v.Interface()
			}
			return true
		})
	}
	return out
}

type settableField struct {
	Message string   `json:"message"`
	Field   string   `json:"field"`
	Kind    string   `json:"kind"`
	Values  []string `json:"enum_values,omitempty"`
}

func settableFields(msgs []protoreflect.Message) []settableField {
	out := []settableField{}
	for _, m := range msgs {
		for i := 0; i < m.Descriptor().Fields().Len(); i++ {
			fd := m.Descriptor().Fields().Get(i)
			if fd.IsList() || fd.IsMap() || fd.Kind() == protoreflect.MessageKind {
				continue
			}
			f := settableField{Message: string(m.Descriptor().Name()), Field: string(fd.Name()), Kind: fd.Kind().String()}
			if fd.Kind() == protoreflect.EnumKind {
				for j := 0; j < fd.Enum().Values().Len(); j++ {
					f.Values = append(f.Values, string(fd.Enum().Values().Get(j).Name()))
				}
			}
			out = append(out, f)
		}
	}
	return out
}

// ---------------------------------------------------------------- character stats

func computeFinalStats(req *proto.RaidSimRequest) (final []float64, err error) {
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
		return nil, fmt.Errorf("compute stats: %s", result.ErrorResult)
	}
	return result.GetRaidStats().GetParties()[0].GetPlayers()[0].GetFinalStats().GetStats(), nil
}

// forceFinalStats sets the player's bonus_stats so the normal ComputeStats path
// yields the requested final (character sheet) values. Dependent stats (strength
// -> attack power, Kings, ...) are handled by iterating to a fixed point.
func forceFinalStats(req *proto.RaidSimRequest, want map[string]float64) ([]float64, []string, error) {
	notes := []string{}
	final, err := computeFinalStats(req)
	if err != nil || len(want) == 0 {
		return final, notes, err
	}
	player := req.Raid.Parties[0].Players[0]
	if player.BonusStats == nil {
		player.BonusStats = &proto.UnitStats{}
	}
	bonus := make([]float64, len(foreverdata.StatKeys))
	copy(bonus, player.BonusStats.Stats)
	player.BonusStats.Stats = bonus
	keys := make([]string, 0, len(want))
	for key := range want {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for pass := 0; pass < 12; pass++ {
		worst := 0.0
		for _, key := range keys {
			idx, _ := foreverdata.StatKeyIndex(key)
			diff := want[key] - final[idx]
			bonus[idx] += diff
			worst = max(worst, math.Abs(diff))
		}
		if worst < 1e-6 {
			break
		}
		if final, err = computeFinalStats(req); err != nil {
			return nil, notes, err
		}
	}
	for _, key := range keys {
		idx, _ := foreverdata.StatKeyIndex(key)
		if math.Abs(want[key]-final[idx]) > 0.01 {
			notes = append(notes, fmt.Sprintf("stat %s could not be forced to %v (sim computes %.4f); it may be capped or derived", key, want[key], final[idx]))
		}
	}
	return final, notes, nil
}

func statMap(values []float64) map[string]float64 {
	out := map[string]float64{}
	for i, v := range values {
		if i < len(foreverdata.StatKeys) && v != 0 {
			out[foreverdata.StatKeys[i]] = math.Round(v*1e4) / 1e4
		}
	}
	return out
}

// ---------------------------------------------------------------- one game

type predictRun struct {
	result  *proto.RaidSimResult
	report  *core.ForeverOverrideRunReport
	prov    game.Provenance
	elapsed int64
}

func runPredictSim(g game.Version, req *proto.RaidSimRequest) (out predictRun, err error) {
	start := time.Now()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
		out.elapsed = time.Since(start).Milliseconds()
	}()
	sel := game.Selection{Version: g, Discovery: g == game.Forever}
	if g == game.Forever {
		out.result, out.prov, out.report, err = game.RunRaidSimWithOverrideReport(sel, req)
	} else {
		out.result, out.prov, err = game.RunRaidSim(sel, req)
	}
	if err != nil {
		err = fmt.Errorf("%s", firstLine(err.Error()))
	}
	return out, err
}

func (b *bench) predictGame(g game.Version, e PresetEntry, base *proto.RaidSimRequest, in PredictRequest) (map[string]interface{}, error) {
	caveats := []string{}
	req := googleProto.Clone(base).(*proto.RaidSimRequest)
	out := map[string]interface{}{"game": string(g)}
	if g == game.Forever {
		freq, info, source, ref, err := b.foreverRequest(req, e)
		if err != nil {
			return nil, fmt.Errorf("forever setup: %w", err)
		}
		req = freq
		player := req.Raid.Parties[0].Players[0]
		for key, value := range in.Parameters {
			if player.Forever.Parameters == nil {
				player.Forever.Parameters = map[string]float64{}
			}
			player.Forever.Parameters[key] = value
		}
		out["forever_talents"] = map[string]interface{}{"f1": info.ForeverF1, "source": source, "build": ref}
	} else if len(in.Parameters) > 0 {
		caveats = append(caveats, "-parameters are Forever parameters and are ignored by the Classic run")
	}
	final, notes, err := forceFinalStats(req, in.Stats)
	if err != nil {
		return nil, err
	}
	caveats = append(caveats, notes...)
	if g == game.Forever {
		withAuto, rep, err := b.autoRotation(req)
		if err != nil {
			return nil, fmt.Errorf("forever auto-rotation: %w", err)
		}
		req = withAuto
		out["auto_rotation_added"] = rep.Added
	}

	run, err := runPredictSim(g, req)
	if err != nil {
		return nil, err
	}
	rotation := "spec rotation (" + e.APLPreset + ")"
	seen := predictActions(b.d, run.result)
	matched, err := matchAbilities(in.Ability, seen)
	if err != nil {
		// Not cast by the rotation: look it up among the registered spells and cast it on its own.
		known, kerr := knownSpells(b.d, req)
		if kerr != nil {
			return nil, kerr
		}
		candidates, merr := matchAbilities(in.Ability, known)
		if merr != nil {
			return nil, fmt.Errorf("ability %q not found for %s. Abilities used by the rotation: %s. Other registered spells: %s",
				in.Ability, e.Spec, strings.Join(actionNames(seen), ", "), strings.Join(actionNames(known), ", "))
		}
		// Several ranks share a name; the highest spell id (normally the highest rank) gets priority.
		sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].action.GetSpellId() > candidates[j].action.GetSpellId() })
		solo := googleProto.Clone(req).(*proto.RaidSimRequest)
		apl := &proto.APLRotation{Type: proto.APLRotation_TypeAPL}
		for _, cand := range candidates {
			apl.PriorityList = append(apl.PriorityList, &proto.APLListItem{Action: &proto.APLAction{Action: &proto.APLAction_CastSpell{CastSpell: &proto.APLActionCastSpell{SpellId: cand.action}}}})
		}
		solo.Raid.Parties[0].Players[0].Rotation = apl
		if run, err = runPredictSim(g, solo); err != nil {
			return nil, fmt.Errorf("single-cast fallback: %w", err)
		}
		used := actionNames(seen)
		seen = predictActions(b.d, run.result)
		if matched, err = matchAbilities(in.Ability, seen); err != nil {
			return nil, fmt.Errorf("ability %q is registered but was never cast, neither by the spec rotation nor by a cast-when-ready APL (passive, proc-only or unusable in this setup). Abilities used by the rotation: %s",
				in.Ability, strings.Join(used, ", "))
		}
		req = solo
		if len(candidates) > 1 {
			caveats = append(caveats, fmt.Sprintf("%q matches %d registered spells (ranks); the cast-when-ready APL prefers the highest spell id. Pass a spell id to pick a rank", in.Ability, len(candidates)))
		}
		rotation = "fallback: cast " + strings.Join(actionNames(candidates), " / ") + " when ready (plus auto attacks)"
		caveats = append(caveats, "the spec rotation never casts this ability; it was simulated with an APL that only casts it when ready, so casts, DPS and share_pct describe that artificial rotation (per-hit averages and the hit table remain meaningful). Abilities used by the spec rotation: "+strings.Join(used, ", "))
	}

	iterations := float64(req.SimOptions.Iterations)
	duration := run.result.GetAvgIterationDuration()
	if duration <= 0 {
		duration = req.Encounter.Duration
	}
	total := 0.0
	for _, a := range seen {
		total += a.damage()
	}
	abilities := []PredictAbility{}
	for _, a := range matched {
		abilities = append(abilities, a.predict(iterations, duration, total))
	}
	if len(abilities) > 1 {
		caveats = append(caveats, fmt.Sprintf("%q matched %d actions; use the id (e.g. %s) to select one", in.Ability, len(abilities), abilities[0].ID))
	}

	buffs, debuffs := buffMessages(req)
	out["rotation"] = rotation
	out["abilities"] = abilities
	out["total_dps"] = round2(run.result.GetRaidMetrics().GetDps().GetAvg())
	out["character"] = map[string]interface{}{
		"class": e.Class, "race": raceName(req.Raid.Parties[0].Players[0].Race), "final_stats": statMap(final),
		"forced_stats": in.Stats, "bonus_stats": statMap(req.Raid.Parties[0].Players[0].GetBonusStats().GetStats()),
		"stats_note": "final_stats are the out-of-combat ComputeStats values (gear, talents, buffs, consumes); crit/hit are percent; temporary procs are not included",
	}
	target, note := predictTarget(req)
	if note != "" {
		caveats = append(caveats, note)
	}
	out["target"] = target
	out["buffs"] = activeFields(buffs)
	out["debuffs"] = activeFields(debuffs)
	if g == game.Forever {
		overrides := map[string]interface{}{"document": foreverdata.OverridesInfo()}
		if run.report != nil {
			applied, rejected := []foreverdata.OverrideEvent{}, []foreverdata.OverrideEvent{}
			for _, p := range run.report.Players {
				applied = append(applied, p.Applied...)
				rejected = append(rejected, p.Rejected...)
			}
			overrides = map[string]interface{}{"document": run.report.Overrides, "applied_count": len(applied), "rejected_count": len(rejected),
				"applied": applied, "rejected": rejected}
		}
		overrides["request_parameters"] = req.Raid.Parties[0].Players[0].Forever.Parameters
		out["forever_overrides"] = overrides
	}
	out["provenance"] = compactOrFull(&run.prov, b.fullProv)
	out["caveats"] = caveats
	out["elapsed_ms"] = run.elapsed
	return out, nil
}

// predictTarget builds the environment once more and reads the target and the
// player's main-hand attack table 10 seconds into the fight (debuffs such as
// Sunder Armor are stacked during the first second).
func predictTarget(req *proto.RaidSimRequest) (out map[string]interface{}, note string) {
	t := req.Encounter.Targets[0]
	out = map[string]interface{}{"level": t.Level, "mob_type": strings.TrimPrefix(t.MobType.String(), "MobType"), "base_armor": t.Stats[proto.Stat_StatArmor]}
	defer func() {
		if r := recover(); r != nil {
			note = fmt.Sprintf("target armor after debuffs could not be read from the simulator: %v", r)
		}
	}()
	one := googleProto.Clone(req).(*proto.RaidSimRequest)
	one.SimOptions.Iterations = 1
	sim := core.NewSim(one, simsignals.CreateSignals())
	sim.Reset()
	sim.PrePull()
	for sim.CurrentTime < 10*time.Second {
		if sim.Step() {
			break
		}
	}
	unit := sim.Encounter.TargetUnits[0]
	character := sim.Raid.Parties[0].Players[0].GetCharacter()
	at := character.AttackTables[unit.UnitIndex][proto.CastType_CastTypeMainHand]
	pct := func(v float64) float64 { return math.Round(v*1e4) / 100 }
	out["armor_after_debuffs"] = math.Round(unit.Armor()*100) / 100
	out["physical_damage_reduction_pct"] = pct(1 - at.GetArmorDamageModifier())
	out["attack_table_pct"] = map[string]float64{
		"base_melee_miss": pct(at.BaseMissChance), "dual_wield_miss_penalty": pct(at.DualWieldMissPenalty), "hit_suppression": pct(at.HitSuppression),
		"dodge": pct(at.BaseDodgeChance), "parry": pct(at.BaseParryChance), "block": pct(at.BaseBlockChance), "glance": pct(at.BaseGlanceChance),
		"melee_crit_suppression": pct(at.MeleeCritSuppression), "spell_crit_suppression": pct(at.SpellCritSuppression), "base_spell_miss": pct(at.BaseSpellMissChance),
	}
	out["glance_multiplier"] = [2]float64{at.GlanceMultiplierMin, at.GlanceMultiplierMax}
	out["attack_table_note"] = "main-hand table before the character's hit/crit/weapon-skill-independent modifiers; parry/block only apply when attacking from the front"
	return out, ""
}

// ---------------------------------------------------------------- actions

type predictAction struct {
	id, name, source string
	action           *proto.ActionID
	metrics          *proto.ActionMetrics
}

func (a predictAction) damage() float64 {
	sum := 0.0
	for _, t := range a.metrics.GetTargets() {
		sum += t.GetDamage()
	}
	return sum
}

func predictActions(d *fvDataset, result *proto.RaidSimResult) []predictAction {
	out := []predictAction{}
	add := func(unit *proto.UnitMetrics, source string) {
		for _, a := range unit.GetActions() {
			id, name := describeAction(d, a.GetId())
			out = append(out, predictAction{id: id, name: name, source: source, action: a.GetId(), metrics: a})
		}
	}
	for _, party := range result.GetRaidMetrics().GetParties() {
		for _, p := range party.GetPlayers() {
			add(p, "")
			for _, pet := range p.GetPets() {
				add(pet, "pet:"+pet.GetName())
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].damage() > out[j].damage() })
	return out
}

// knownSpells lists the player's registered castable spells (ComputeStats metadata).
func knownSpells(d *fvDataset, req *proto.RaidSimRequest) ([]predictAction, error) {
	spells, err := computeAllSpells(req)
	if err != nil {
		return nil, err
	}
	out := []predictAction{}
	for _, s := range spells[[2]int{0, 0}] {
		if !s.GetIsCastable() {
			continue
		}
		id, name := describeAction(d, s.GetId())
		out = append(out, predictAction{id: id, name: name, action: s.GetId()})
	}
	return out, nil
}

func actionNames(list []predictAction) []string {
	out, seen := []string{}, map[string]bool{}
	for _, a := range list {
		label := a.name + " [" + a.id + "]"
		if a.source != "" {
			label = a.source + " " + label
		}
		if !seen[label] {
			seen[label] = true
			out = append(out, label)
		}
	}
	return out
}

// matchAbilities resolves a spell id, action id or case-insensitive name. Exact
// matches win; otherwise a substring match is accepted when it is unambiguous.
func matchAbilities(query string, actions []predictAction) ([]predictAction, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	exact, partial := []predictAction{}, []predictAction{}
	partialNames := map[string]bool{}
	for _, a := range actions {
		id, name := strings.ToLower(a.id), strings.ToLower(a.name)
		baseName := strings.ToLower(rankSuffix.ReplaceAllString(a.name, ""))
		_, notNumber := strconv.Atoi(q)
		switch {
		case q == id, q == name, q == baseName,
			notNumber == nil && (id == "spell:"+q || strings.HasPrefix(id, "spell:"+q+":") || id == "item:"+q || strings.HasPrefix(id, "item:"+q+":")):
			exact = append(exact, a)
		case notNumber != nil && strings.Contains(baseName, q):
			partial = append(partial, a)
			partialNames[baseName] = true
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}
	if len(partial) > 0 && len(partialNames) == 1 {
		return partial, nil
	}
	if len(partial) > 0 {
		return nil, fmt.Errorf("ability %q is ambiguous: %s", query, strings.Join(actionNames(partial), ", "))
	}
	return nil, fmt.Errorf("ability %q not found", query)
}

func (a predictAction) predict(iterations, duration, totalDamage float64) PredictAbility {
	out := PredictAbility{ID: a.id, Name: a.name, Source: a.source, Melee: a.metrics.GetIsMelee()}
	out.School = schoolName(core.SpellSchool(a.metrics.GetSpellSchool()))
	var t proto.TargetedActionMetrics
	crushes := int64(0)
	for _, m := range a.metrics.GetTargets() {
		t.Casts += m.Casts
		t.Hits += m.Hits
		t.Crits += m.Crits
		t.Glances += m.Glances
		t.Blocks += m.Blocks
		t.BlockedCrits += m.BlockedCrits
		t.Misses += m.Misses
		t.Dodges += m.Dodges
		t.Parries += m.Parries
		t.ResistedHits += m.ResistedHits
		t.ResistedCrits += m.ResistedCrits
		t.Ticks += m.Ticks
		t.CritTicks += m.CritTicks
		t.Damage += m.Damage
		t.CritDamage += m.CritDamage
		t.GlanceDamage += m.GlanceDamage
		t.BlockDamage += m.BlockDamage
		t.BlockedCritDamage += m.BlockedCritDamage
		t.CrushDamage += m.CrushDamage
		t.TickDamage += m.TickDamage
		t.CritTickDamage += m.CritTickDamage
		t.ResistedDamage += m.ResistedDamage
		t.ResistedCritDamage += m.ResistedCritDamage
		t.ResistedTickDamage += m.ResistedTickDamage
		t.ResistedCritTickDamage += m.ResistedCritTickDamage
		crushes += int64(m.Crushes)
	}
	landed := int64(t.Hits) + int64(t.Crits) + int64(t.Glances) + int64(t.Blocks) + int64(t.BlockedCrits) + crushes
	attempts := landed + int64(t.Misses) + int64(t.Dodges) + int64(t.Parries)
	c := &out.Counts
	c.Casts, c.Landed, c.Hits, c.Crits, c.Glances, c.Blocks, c.BlockedCrits = int64(t.Casts), landed, int64(t.Hits), int64(t.Crits), int64(t.Glances), int64(t.Blocks), int64(t.BlockedCrits)
	c.Misses, c.Dodges, c.Parries, c.PartialHits, c.Ticks, c.CritTicks = int64(t.Misses), int64(t.Dodges), int64(t.Parries), int64(t.ResistedHits)+int64(t.ResistedCrits), int64(t.Ticks), int64(t.CritTicks)

	avg := func(damage float64, n int64) *float64 {
		if n <= 0 {
			return nil
		}
		v := round2(damage / float64(n))
		return &v
	}
	// Tick damage is part of damage, crit tick damage part of crit damage and tick damage.
	directCrit := t.CritDamage - t.CritTickDamage
	directHit := t.Damage - t.TickDamage - directCrit - t.GlanceDamage - t.BlockDamage - t.BlockedCritDamage - t.CrushDamage
	resistedDirectCrit := t.ResistedCritDamage - t.ResistedCritTickDamage
	resistedDirectHit := t.ResistedDamage - t.ResistedTickDamage - resistedDirectCrit
	out.AvgHit = avg(directHit, int64(t.Hits))
	out.AvgCrit = avg(directCrit, int64(t.Crits))
	if t.ResistedHits > 0 {
		out.AvgHitUnresisted = avg(directHit-resistedDirectHit, int64(t.Hits-t.ResistedHits))
	}
	if t.ResistedCrits > 0 {
		out.AvgCritUnresisted = avg(directCrit-resistedDirectCrit, int64(t.Crits-t.ResistedCrits))
	}
	out.AvgGlance = avg(t.GlanceDamage, int64(t.Glances))
	out.AvgBlock = avg(t.BlockDamage, int64(t.Blocks))
	out.AvgTick = avg(t.TickDamage-t.CritTickDamage, int64(t.Ticks))
	out.AvgCritTick = avg(t.CritTickDamage, int64(t.CritTicks))
	out.AvgDamagePerCast = avg(t.Damage, int64(t.Casts))
	if iterations > 0 {
		out.CastsPerFight = round2(float64(t.Casts) / iterations)
	}
	if t.Casts > 0 {
		out.LandedPerCast = round2(float64(landed) / float64(t.Casts))
	}
	if attempts > 0 {
		pct := func(n int64) float64 { return round2(float64(n) / float64(attempts) * 100) }
		out.TablePct = map[string]float64{"miss": pct(int64(t.Misses)), "dodge": pct(int64(t.Dodges)), "parry": pct(int64(t.Parries)),
			"glance": pct(int64(t.Glances)), "block": pct(int64(t.Blocks) + int64(t.BlockedCrits)), "crit": pct(int64(t.Crits)), "hit": pct(int64(t.Hits) + crushes)}
	}
	if iterations > 0 && duration > 0 {
		out.DPS = round2(t.Damage / iterations / duration)
	}
	if totalDamage > 0 {
		out.SharePct = round2(t.Damage / totalDamage * 100)
	}
	return out
}

// schoolName spells out the core school bit mask of an action.
func schoolName(school core.SpellSchool) string {
	names := []string{}
	for _, s := range []struct {
		school core.SpellSchool
		name   string
	}{{core.SpellSchoolPhysical, "physical"}, {core.SpellSchoolArcane, "arcane"}, {core.SpellSchoolFire, "fire"}, {core.SpellSchoolFrost, "frost"},
		{core.SpellSchoolHoly, "holy"}, {core.SpellSchoolNature, "nature"}, {core.SpellSchoolShadow, "shadow"}} {
		if school.Matches(s.school) {
			names = append(names, s.name)
		}
	}
	return strings.Join(names, "+")
}

// ---------------------------------------------------------------- -list

func predictListing(b *bench, e PresetEntry, base *proto.RaidSimRequest, g game.Version) (interface{}, error) {
	req := googleProto.Clone(base).(*proto.RaidSimRequest)
	req.SimOptions.Iterations = 50
	if g == game.Forever {
		freq, _, _, _, err := b.foreverRequest(req, e)
		if err != nil {
			return nil, fmt.Errorf("forever setup: %w", err)
		}
		if req, _, err = b.autoRotation(freq); err != nil {
			return nil, fmt.Errorf("forever auto-rotation: %w", err)
		}
	}
	run, err := runPredictSim(g, req)
	if err != nil {
		return nil, err
	}
	type listed struct {
		ID     string  `json:"id"`
		Name   string  `json:"name"`
		Source string  `json:"source,omitempty"`
		Casts  float64 `json:"casts_per_fight"`
		Share  float64 `json:"share_pct"`
	}
	seen := predictActions(b.d, run.result)
	total := 0.0
	for _, a := range seen {
		total += a.damage()
	}
	used, usedIDs := []listed{}, map[string]bool{}
	for _, a := range seen {
		p := a.predict(float64(req.SimOptions.Iterations), req.Encounter.Duration, total)
		used = append(used, listed{ID: a.id, Name: a.name, Source: a.source, Casts: p.CastsPerFight, Share: p.SharePct})
		usedIDs[a.source+a.id] = true
	}
	known, err := knownSpells(b.d, req)
	if err != nil {
		return nil, err
	}
	unused := []listed{}
	for _, a := range known {
		if !usedIDs[a.id] {
			unused = append(unused, listed{ID: a.id, Name: a.name})
		}
	}
	gear := []string{}
	if s, err := uiPresets(); err == nil {
		cands, _ := s.gearCandidates(e.variant)
		for _, cand := range cands {
			gear = append(gear, cand.label)
		}
	}
	buffs, debuffs := buffMessages(req)
	params := []foreverdata.ParameterSpec{}
	for _, p := range foreverdata.ParameterCatalog() {
		if p.Source == "core" {
			params = append(params, p)
		}
	}
	return map[string]interface{}{
		"command":                  "predict -list",
		"spec":                     e.Spec,
		"phase":                    e.Phase,
		"game":                     string(g),
		"abilities_used":           used,
		"castable_not_in_rotation": unused,
		"stat_keys":                foreverdata.StatKeys,
		"gear_presets":             gear,
		"buff_fields":              settableFields(buffs),
		"debuff_fields":            settableFields(debuffs),
		"active_buffs":             activeFields(buffs),
		"active_debuffs":           activeFields(debuffs),
		"core_forever_parameters":  params,
		"notes":                    []string{"castable_not_in_rotation: predict falls back to a cast-when-ready APL for these"},
	}, nil
}
