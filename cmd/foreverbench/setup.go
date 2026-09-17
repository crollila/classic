package main

// Setup optimizer (optimize-setup): joint search over the discrete setup of
// one spec: talent build, race, gear/weapon set, consumables, APL and the
// Forever auto-rotation policy.
//
// rank/delta/meta keep their normalization (identical non-spec inputs for every
// spec). optimize-setup answers a different question: what is the best
// combination for this spec. Talents, racials, weapons, consumables and the
// rotation interact (racial weapon skill with the weapon type, a two-hander
// with the talent build and the APL, Windfury with the main-hand imbue), so
// every option is valued by simulating the whole setup, never as an independent
// bonus.
//
// Dimensions ("one option per slot": a state holds exactly one option index per
// dimension, so illegal stacks cannot be expressed):
//
//	talents        the normalized build (optimized build / UI preset), the other
//	               builds for the spec in presets/*_builds.json, the UI talent
//	               presets of the spec's UI directory that keep the spec identity
//	               (Forever: their repair), and with -reoptimize-talents the
//	               talent optimizer rerun inside the chosen setup
//	race           every race legal for the class in the game (raceCandidates)
//	gear           the phase's gear sets: the preset set, other complete sets of
//	               the UI directory (GearFilter respected), donor sets
//	               (GearFrom/GearDonors); two-handed variants get every
//	               two-hander of the twoHanders table up to the phase and, when
//	               the class can dual wield, the donor set as is; DPS warriors
//	               also get their set with a two-hander
//	flask, food, main_hand_imbue, off_hand_imbue, potion, conjured, zanza
//	               one dimension per exclusive proto.Consumes enum field; options
//	               are the enum values minus dominated ranks, defensive items,
//	               undead-only items and items the class/archetype cannot use.
//	               The Windfury Totem is the main-hand imbue WeaponImbue_Windfury
//	               in the sim, so it is exclusive with stones, oils, poisons and
//	               shaman imbues by construction. Sharpening stones need an edged
//	               and weightstones a blunt weapon; off-hand imbues need an
//	               off-hand weapon (checked against the gear option at hand; the
//	               normalized preset's own values are taken as they are)
//	apl            the UI APL presets and APL files of the UI directory plus the
//	               embedded foreverbench APLs of the class
//	auto_rotation  Forever only: conservative, ui, off
//
// Algorithm (deterministic for a fixed seed unless -budget-seconds is hit),
// using the talent optimizer's evaluation engine: common random numbers, staged
// iterations N/5N/15N, accept only above max(0.05% DPS, 0.5 x combined SE):
//  1. start from the normalized preset setup (option 0 of every dimension);
//  2. cyclic coordinate ascent: per dimension, every option is simmed with the
//     other dimensions fixed and the best is adopted if it clears the noise
//     rules; cycles repeat until a full cycle changes nothing;
//  3. pair probes: for every two dimensions, the top 2 alternatives of each
//     (screening level) are combined (2x2 factorial with the current setup:
//     synergy = f(a+b) - f(a) - f(b) + f(current)); the best joint change is
//     adopted under the same noise rules even when neither single change was,
//     then the ascent resumes (at most 4 rounds);
//  4. -reoptimize-talents: when the setup differs from the preset, the talent
//     optimizer runs inside the chosen setup (remaining budget) and its build
//     is adopted if it clears the noise rules, followed by one more ascent.

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wowsims/classic/assets/database"
	"github.com/wowsims/classic/sim/core/foreverdata"
	"github.com/wowsims/classic/sim/core/proto"
	"github.com/wowsims/classic/sim/game"
	"google.golang.org/protobuf/encoding/protojson"
	googleProto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// ---------------------------------------------------------------- generic search

type setupOption struct {
	Label   string
	Note    string
	apply   func(*setupConfig)
	talents []int32 // talents dimension: the build as a model state (nil when unknown)
}

type setupDim struct {
	Name    string
	Options []setupOption
}

type SetupStep struct {
	Phase      string  `json:"phase"` // ascent | pairs | reoptimize-talents
	Round      int     `json:"round"`
	Change     string  `json:"change"`
	DPSBefore  float64 `json:"dps_before"`
	DPSAfter   float64 `json:"dps_after"`
	GainPct    float64 `json:"gain_pct"`
	Iterations int32   `json:"decided_at_iterations"`
	Candidates int     `json:"candidates"`
}

// SetupInteraction is a 2x2 factorial between options of two dimensions on the
// setup that was current when it was probed.
type SetupInteraction struct {
	DimA       string  `json:"dimension_a"`
	OptionA    string  `json:"option_a"`
	DimB       string  `json:"dimension_b"`
	OptionB    string  `json:"option_b"`
	GainA      float64 `json:"gain_a_dps"`
	GainB      float64 `json:"gain_b_dps"`
	GainAB     float64 `json:"gain_ab_dps"`
	Synergy    float64 `json:"synergy_dps"`
	SynergySE  float64 `json:"synergy_stderr"`
	Iterations int32   `json:"iterations"`
	Accepted   bool    `json:"accepted"`
}

// setupSearch searches option indices per dimension. The engine is the talent
// optimizer's (cache, staged levels, noise rules, budget); its states are the
// option indices.
type setupSearch struct {
	dims      []setupDim
	eng       *optimizer
	maxProbes int

	path   []SetupStep
	inter  map[string]SetupInteraction
	probes int
}

func newSetupSearch(dims []setupDim, base int32, parallel int, deadline time.Time, maxProbes int,
	eval func(s []int32, iterations int32) (evalResult, int64)) *setupSearch {
	return &setupSearch{dims: dims, maxProbes: maxProbes, inter: map[string]SetupInteraction{},
		eng: &optimizer{m: &talentModel{}, levels: []int32{base, base * 5, base * 15}, parallel: max(1, parallel), deadline: deadline,
			cache: map[string]evalResult{}, values: map[int]float64{}, evalFn: eval}}
}

func (ss *setupSearch) with(s []int32, d int, k int32) []int32 {
	c := cloneState(s)
	c[d] = k
	return c
}

func (ss *setupSearch) step(phase string, round int, d decision) SetupStep {
	pct := 0.0
	if d.base.DPS > 0 {
		pct = round2((d.best.DPS - d.base.DPS) / d.base.DPS * 100)
	}
	return SetupStep{Phase: phase, Round: round, Change: d.move.desc, DPSBefore: round2(d.base.DPS), DPSAfter: round2(d.best.DPS), GainPct: pct,
		Iterations: d.iterations, Candidates: d.candidates}
}

func (ss *setupSearch) change(s []int32, d int, k int32) string {
	return fmt.Sprintf("%s: %s -> %s", ss.dims[d].Name, ss.dims[d].Options[s[d]].Label, ss.dims[d].Options[k].Label)
}

// ascend is the cyclic coordinate ascent (step 2 of the header comment).
func (ss *setupSearch) ascend(s []int32, round int) []int32 {
	for cycle := 0; cycle < 10 && !ss.eng.overBudget(); cycle++ {
		changed := false
		for d, dim := range ss.dims {
			moves := []optMove{}
			for k := range dim.Options {
				if int32(k) != s[d] {
					moves = append(moves, optMove{ss.with(s, d, int32(k)), ss.change(s, d, int32(k)), 1, -1})
				}
			}
			if len(moves) == 0 || ss.eng.overBudget() {
				continue
			}
			if dec, ok := ss.eng.selectBest(s, moves, selectNormal); ok {
				ss.path = append(ss.path, ss.step("ascent", round, dec))
				s, changed = dec.move.state, true
			}
		}
		if !changed {
			break
		}
	}
	return s
}

// probe is the pair phase (step 3): it returns the jointly better setup, if any.
func (ss *setupSearch) probe(s []int32, round int) ([]int32, bool) {
	lvl := ss.eng.levels[0]
	type alt struct {
		d    int
		k    int32
		gain float64
	}
	top := [][]alt{}
	var base evalResult
	for d, dim := range ss.dims {
		states := [][]int32{s}
		ks := []int32{}
		for k := range dim.Options {
			if int32(k) != s[d] {
				states = append(states, ss.with(s, d, int32(k)))
				ks = append(ks, int32(k))
			}
		}
		res := ss.eng.evaluate(states, lvl)
		base = res[0]
		alts := []alt{}
		for i, k := range ks {
			if res[i+1].Err == "" {
				alts = append(alts, alt{d, k, res[i+1].DPS - res[0].DPS})
			}
		}
		sort.SliceStable(alts, func(i, j int) bool { return alts[i].gain > alts[j].gain })
		top = append(top, alts[:min(2, len(alts))])
	}
	if base.Err != "" {
		return s, false
	}
	type pair struct{ a, b alt }
	pairs := []pair{}
	for d1 := range top {
		for d2 := d1 + 1; d2 < len(top); d2++ {
			for _, a := range top[d1] {
				for _, b := range top[d2] {
					pairs = append(pairs, pair{a, b})
				}
			}
		}
	}
	sort.SliceStable(pairs, func(i, j int) bool { return pairs[i].a.gain+pairs[i].b.gain > pairs[j].a.gain+pairs[j].b.gain })
	pairs = pairs[:min(ss.maxProbes, len(pairs))]
	moves := []optMove{}
	for _, p := range pairs {
		st := ss.with(ss.with(s, p.a.d, p.a.k), p.b.d, p.b.k)
		moves = append(moves, optMove{st, ss.change(s, p.a.d, p.a.k) + "; " + ss.change(s, p.b.d, p.b.k), 1, -1})
	}
	ss.probes += len(moves)
	dec, ok := ss.eng.selectBest(s, moves, selectNormal)
	measure := func(p pair, iterations int32, accepted bool) (SetupInteraction, string, bool) {
		sa, sb := ss.with(s, p.a.d, p.a.k), ss.with(s, p.b.d, p.b.k)
		res := ss.eng.evaluate([][]int32{s, sa, sb, ss.with(sa, p.b.d, p.b.k)}, iterations)
		se := 0.0
		for _, r := range res {
			if r.Err != "" {
				return SetupInteraction{}, "", false
			}
			se += r.StdErr * r.StdErr
		}
		da, db := ss.dims[p.a.d], ss.dims[p.b.d]
		key := fmt.Sprintf("%s=%s|%s=%s", da.Name, da.Options[p.a.k].Label, db.Name, db.Options[p.b.k].Label)
		return SetupInteraction{DimA: da.Name, OptionA: da.Options[p.a.k].Label, DimB: db.Name, OptionB: db.Options[p.b.k].Label,
			GainA: round2(res[1].DPS - res[0].DPS), GainB: round2(res[2].DPS - res[0].DPS), GainAB: round2(res[3].DPS - res[0].DPS),
			Synergy: round2(res[3].DPS - res[1].DPS - res[2].DPS + res[0].DPS), SynergySE: round2(math.Sqrt(se)), Iterations: iterations, Accepted: accepted}, key, true
	}
	// Every probe is reported at the screening level; interactions beyond 3 SE
	// (the 8 strongest) and the accepted pair are re-measured at a higher level.
	strong := []pair{}
	for _, p := range pairs {
		if si, key, valid := measure(p, lvl, false); valid {
			if old, seen := ss.inter[key]; !seen || old.Iterations <= lvl {
				ss.inter[key] = si
			}
			if si.SynergySE > 0 && math.Abs(si.Synergy) > 3*si.SynergySE {
				strong = append(strong, p)
			}
		}
	}
	sigma := func(p pair) float64 {
		si, _, _ := measure(p, lvl, false)
		return math.Abs(si.Synergy) / si.SynergySE
	}
	sort.SliceStable(strong, func(i, j int) bool { return sigma(strong[i]) > sigma(strong[j]) })
	for _, p := range strong[:min(8, len(strong))] {
		if ss.eng.overBudget() {
			break
		}
		if si, key, valid := measure(p, ss.eng.levels[min(1, len(ss.eng.levels)-1)], false); valid {
			ss.inter[key] = si
		}
	}
	if !ok {
		return s, false
	}
	for i, mv := range moves {
		if ss.eng.m.key(mv.state) == ss.eng.m.key(dec.move.state) {
			if si, key, valid := measure(pairs[i], dec.iterations, true); valid {
				ss.inter[key] = si
			}
		}
	}
	ss.path = append(ss.path, ss.step("pairs", round, dec))
	return dec.move.state, true
}

// run is steps 1-3: ascent and pair probes until neither changes the setup.
func (ss *setupSearch) run(s []int32) []int32 {
	for round := 1; round <= 4 && !ss.eng.overBudget(); round++ {
		s = ss.ascend(s, round)
		if ss.maxProbes <= 0 || ss.eng.overBudget() {
			break
		}
		next, ok := ss.probe(s, round)
		if !ok {
			break
		}
		s = next
	}
	return s
}

func (ss *setupSearch) interactions() []SetupInteraction {
	out := []SetupInteraction{}
	for _, si := range ss.inter {
		out = append(out, si)
	}
	sort.Slice(out, func(i, j int) bool {
		if math.Abs(out[i].Synergy) != math.Abs(out[j].Synergy) {
			return math.Abs(out[i].Synergy) > math.Abs(out[j].Synergy)
		}
		return out[i].DimA+out[i].OptionA+out[i].DimB+out[i].OptionB < out[j].DimA+out[j].OptionA+out[j].DimB+out[j].OptionB
	})
	return out
}

// ---------------------------------------------------------------- setup model

// setupConfig is one concrete setup; options edit it.
type setupConfig struct {
	entry          PresetEntry // gear and APL
	race           proto.Race
	consumes       *proto.Consumes
	classicTalents string
	policy         TalentPolicy
	autoRotation   string
}

type SetupRejected struct {
	Dimension string `json:"dimension"`
	Option    string `json:"option"`
	Reason    string `json:"reason"`
}

type weaponInfo struct {
	name   string
	weapon proto.WeaponType
	hand   proto.HandType
}

var (
	weaponOnce sync.Once
	weapons    map[int32]weaponInfo
)

func weaponInfos() map[int32]weaponInfo {
	weaponOnce.Do(func() {
		weapons = map[int32]weaponInfo{}
		defer func() { recover() }()
		for _, it := range database.Load().Items {
			if it.Type == proto.ItemType_ItemTypeWeapon {
				weapons[it.Id] = weaponInfo{it.Name, it.WeaponType, it.HandType}
			}
		}
	})
	return weapons
}

func gearWeapons(raw json.RawMessage) (mh, oh weaponInfo) {
	var g struct {
		Items []struct {
			ID int32 `json:"id"`
		} `json:"items"`
	}
	if json.Unmarshal(raw, &g) != nil {
		return
	}
	db := weaponInfos()
	if i := int(proto.ItemSlot_ItemSlotMainHand); i < len(g.Items) {
		mh = db[g.Items[i].ID]
	}
	if i := int(proto.ItemSlot_ItemSlotOffHand); i < len(g.Items) {
		oh = db[g.Items[i].ID]
	}
	return
}

func (e PresetEntry) gearRaw() json.RawMessage {
	if e.gearJSON != nil {
		return e.gearJSON
	}
	raw, _ := presetFile(e.GearFile)
	return raw
}

// imbueProblem is the weapon-type legality of a weapon imbue.
func imbueProblem(imbue proto.WeaponImbue, w weaponInfo, offHand bool) string {
	edged := w.weapon == proto.WeaponType_WeaponTypeAxe || w.weapon == proto.WeaponType_WeaponTypeDagger || w.weapon == proto.WeaponType_WeaponTypeSword || w.weapon == proto.WeaponType_WeaponTypePolearm
	blunt := w.weapon == proto.WeaponType_WeaponTypeMace || w.weapon == proto.WeaponType_WeaponTypeStaff || w.weapon == proto.WeaponType_WeaponTypeFist
	name := imbue.String()
	switch {
	case imbue == proto.WeaponImbue_WeaponImbueUnknown:
		return ""
	case offHand && !edged && !blunt:
		return "no off-hand weapon in the gear set"
	case strings.Contains(name, "SharpeningStone") && !edged:
		return "sharpening stones need an edged weapon, the gear set has " + w.name
	case strings.Contains(name, "Weightstone") && !blunt:
		return "weightstones need a blunt weapon, the gear set has " + w.name
	}
	return ""
}

// ---------------------------------------------------------------- consumables

// consumeSlots are the exclusive proto.Consumes enum fields that are searched.
var consumeSlots = []struct{ dim, field string }{
	{"flask", "flask"}, {"food", "food"}, {"main_hand_imbue", "main_hand_imbue"}, {"off_hand_imbue", "off_hand_imbue"},
	{"potion", "default_potion"}, {"conjured", "default_conjured"}, {"zanza", "zanza_buff"},
}

var dominatedConsumes = map[string]string{
	"MinorWizardOil": "BrilliantWizardOil", "LesserWizardOil": "BrilliantWizardOil", "WizardOil": "BrilliantWizardOil",
	"MinorManaOil": "BrilliantManaOil", "LesserManaOil": "BrilliantManaOil", "SolidSharpeningStone": "DenseSharpeningStone",
	"SolidWeightstone": "DenseWeightstone", "LesserManaPotion": "MajorManaPotion", "ManaPotion": "MajorManaPotion",
	"GreaterManaPotion": "MajorManaPotion", "SuperiorManaPotion": "MajorManaPotion", "RagePotion": "MightyRagePotion", "GreatRagePotion": "MightyRagePotion",
}

// consumeProblem returns why an enum value is not an option for the spec ("" = legal).
func consumeProblem(v Variant, field, name string) string {
	has := func(parts ...string) bool {
		for _, p := range parts {
			if strings.Contains(name, p) {
				return true
			}
		}
		return false
	}
	physical := v.Archetype == "melee" || v.Archetype == "ranged_physical" || v.Archetype == "tank_melee"
	switch {
	case dominatedConsumes[name] != "":
		return "dominated by " + dominatedConsumes[name] + " in the same slot"
	case has("Healing", "Healthstone", "Stoneshield", "Protection", "Resistance"):
		return "defensive: no damage effect"
	case has("BlessedWizardOil", "ConsecratedSharpeningStone"):
		return "undead-only effect; the benchmark target is not undead"
	case has("RockbiterWeapon", "FlametongueWeapon", "FrostbrandWeapon", "WindfuryWeapon") && v.Class != proto.Class_ClassShaman:
		return "shaman weapon imbue"
	case has("Poison") && v.Class != proto.Class_ClassRogue:
		return "rogue poison"
	case has("ThistleTea") && v.Class != proto.Class_ClassRogue:
		return "rogue only"
	case has("RagePotion") && v.Class != proto.Class_ClassWarrior:
		return "warrior only"
	case has("Mana", "DemonicRune", "Recombobulator", "DistilledWisdom") && !isManaUser(v.Class):
		return "mana consumable for a class without mana"
	case name == "Windfury" && field == "off_hand_imbue":
		return "the Windfury Totem only affects the main hand"
	case has("WizardOil", "ManaOil") && physical:
		return "spell power / mana oil: no effect for the " + v.Archetype + " archetype"
	case has("SharpeningStone", "Weightstone", "Windfury") && v.Archetype == "caster":
		return "physical weapon imbue: no effect for the caster archetype"
	}
	return ""
}

// consumeDims builds one dimension per exclusive consumable slot. Option 0 is
// the normalized value; every option sets only its own proto field.
func consumeDims(v Variant, tier string) ([]setupDim, []SetupRejected, []string) {
	base := consumesFor(v, tier)
	fields := base.ProtoReflect().Descriptor().Fields()
	dims, rejected, skipped := []setupDim{}, []SetupRejected{}, []string{}
	for _, slot := range consumeSlots {
		fd := fields.ByName(protoreflect.Name(slot.field))
		if slot.dim == "zanza" && tier != "max" {
			skipped = append(skipped, "zanza: the normalization's consumables tier ("+tier+") excludes Zanza/Blasted Lands buffs (-consumes max searches the slot)")
			continue
		}
		label := func(n protoreflect.EnumNumber) string {
			if n == 0 {
				return "none"
			}
			return string(fd.Enum().Values().ByNumber(n).Name())
		}
		option := func(n protoreflect.EnumNumber, note string) setupOption {
			return setupOption{Label: label(n), Note: note, apply: func(c *setupConfig) {
				c.consumes.ProtoReflect().Set(fd, protoreflect.ValueOfEnum(n))
			}}
		}
		current := base.ProtoReflect().Get(fd).Enum()
		dim := setupDim{Name: slot.dim, Options: []setupOption{option(current, "normalized preset")}}
		values := fd.Enum().Values()
		for i := 0; i < values.Len(); i++ {
			n := values.Get(i).Number()
			if n == 0 || n == current {
				continue
			}
			if why := consumeProblem(v, slot.field, string(values.Get(i).Name())); why != "" {
				rejected = append(rejected, SetupRejected{slot.dim, string(values.Get(i).Name()), why})
				continue
			}
			dim.Options = append(dim.Options, option(n, ""))
		}
		dims = append(dims, dim)
	}
	return dims, rejected, skipped
}

// ---------------------------------------------------------------- gear, APL, race, talents

func canDualWield(c proto.Class) bool {
	return c == proto.Class_ClassWarrior || c == proto.Class_ClassRogue || c == proto.Class_ClassHunter
}

func gearDim(e PresetEntry, phase Phase) (setupDim, []SetupRejected, error) {
	v := e.variant
	snapshot, err := uiPresets()
	if err != nil {
		return setupDim{}, nil, err
	}
	matrix, err := presetMatrix()
	if err != nil {
		return setupDim{}, nil, err
	}
	dim := setupDim{Name: "gear", Options: []setupOption{{Label: e.GearLabel, Note: "normalized preset (" + e.GearStatus + ")", apply: func(*setupConfig) {}}}}
	rejected := []SetupRejected{}
	seen := map[string]bool{string(normalizeJSON(e.gearRaw())): true}
	add := func(label, note, file string, raw json.RawMessage) {
		key := string(normalizeJSON(raw))
		if seen[key] {
			return
		}
		seen[key] = true
		if _, _, _, empty := gearStats(raw, v.Class); len(empty) > 0 {
			rejected = append(rejected, SetupRejected{"gear", label, "empty required slots: " + strings.Join(empty, ", ")})
			return
		}
		if mh, oh := gearWeapons(raw); mh.hand == proto.HandType_HandTypeTwoHand && oh.name != "" {
			rejected = append(rejected, SetupRejected{"gear", label, "two-hander with an off-hand weapon"})
			return
		}
		dim.Options = append(dim.Options, setupOption{Label: label, Note: note, apply: func(c *setupConfig) {
			c.entry.GearLabel, c.entry.GearFile, c.entry.gearJSON = label, file, raw
		}})
	}
	twoHanderIDs := func() []int32 {
		ids := []int32{}
		for p := phase; p >= 0; p-- {
			id := twoHanders[p]
			dup := false
			for _, x := range ids {
				dup = dup || x == id
			}
			if !dup {
				ids = append(ids, id)
			}
		}
		return ids
	}
	// source adds a complete set under the variant's weapon rules.
	source := func(label, note, file string, raw json.RawMessage) {
		withTwoHanders := func() {
			for _, id := range twoHanderIDs() {
				if g, err := borrowGear(raw, id); err == nil {
					add(fmt.Sprintf("%s + two-hander %s (%d)", label, weaponInfos()[id].name, id), note+"; main hand replaced, off hand empty", file, g)
				}
			}
		}
		switch {
		case v.TwoHander != nil:
			withTwoHanders()
			if canDualWield(v.Class) {
				add(label+" (as is)", note, file, raw)
			} else if _, oh := gearWeapons(raw); oh.name != "" {
				rejected = append(rejected, SetupRejected{"gear", label + " (as is)", v.Class.String() + " cannot dual wield"})
			}
		default:
			add(label, note, file, raw)
			if v.Class == proto.Class_ClassWarrior && v.Role == "dps" {
				withTwoHanders()
			}
		}
	}
	if raw, ok := presetFile(e.GearFile); ok {
		source(strings.TrimSuffix(e.GearLabel, fmt.Sprintf(" + two-hander %d", v.TwoHander[phase])), "preset set", e.GearFile, raw)
	}
	for _, c := range func() []gearCandidate { c, _ := snapshot.gearCandidates(v); return c }() {
		if raw, ok := presetFile(c.file); ok && c.phase == phase {
			source(c.label, "ui/"+v.UIDir+" "+c.source, c.file, raw)
		}
	}
	donors := v.GearDonors
	if v.GearFrom != "" {
		donors = []string{v.GearFrom}
	}
	for _, donor := range donors {
		for _, d := range matrix {
			if raw, ok := presetFile(d.GearFile); ok && d.Spec == donor && d.Phase == phase.String() && d.GearStatus == "ok" {
				source(d.GearLabel+" ("+donor+")", "donor set of "+donor, d.GearFile, raw)
			}
		}
	}
	return dim, rejected, nil
}

func aplDim(e PresetEntry) (setupDim, error) {
	v := e.variant
	snapshot, err := uiPresets()
	if err != nil {
		return setupDim{}, err
	}
	dim := setupDim{Name: "apl", Options: []setupOption{{Label: e.APLPreset, Note: "normalized preset (" + e.APLFile + ")", apply: func(*setupConfig) {}}}}
	seen := map[string]bool{e.APLFile: true}
	add := func(label, file string) {
		if _, ok := presetFile(file); !ok || seen[file] {
			return
		}
		seen[file] = true
		dim.Options = append(dim.Options, setupOption{Label: label, Note: file, apply: func(c *setupConfig) {
			c.entry.APLPreset, c.entry.APLFile = label, file
		}})
	}
	spec := snapshot.Specs[v.UIDir]
	names := make([]string, 0, len(spec.Exports))
	for k := range spec.Exports {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, exp := range names {
		for _, r := range collectRefs(spec.Exports[exp]) {
			if r.Kind == "apl" && r.File != "" {
				add(r.Name, r.File)
			}
		}
	}
	for _, f := range spec.AplFiles {
		add(strings.TrimSuffix(f, ".apl.json"), v.UIDir+"/apls/"+f)
	}
	entries, _ := embeddedAPLs.ReadDir("presets/apls")
	prefix := strings.ToLower(strings.TrimPrefix(v.Class.String(), "Class")) + "_"
	for _, f := range entries {
		if strings.HasPrefix(f.Name(), prefix) {
			add("foreverbench "+strings.TrimSuffix(f.Name(), ".apl.json"), "foreverbench/apls/"+f.Name())
		}
	}
	return dim, nil
}

func (b *bench) raceDim(e PresetEntry, gameName string, enc EncounterSpec) (setupDim, *RaceChoice) {
	choice := b.chooseRace(e, gameName == "classic", game.Version(gameName), enc)
	option := func(r proto.Race, note string) setupOption {
		return setupOption{Label: raceName(r), Note: note, apply: func(c *setupConfig) { c.race = r }}
	}
	dim := setupDim{Name: "race", Options: []setupOption{option(choice.race, "normalized preset: "+choice.Reason)}}
	for _, r := range b.d.raceCandidates(e.variant.Class, gameName == "classic") {
		if r != choice.race {
			dim.Options = append(dim.Options, option(r, ""))
		}
	}
	return dim, choice
}

// talentDim lists the talent build candidates; builds that are illegal or lose
// the spec identity are rejected.
func (b *bench) talentDim(e PresetEntry, gameName string, m *talentModel, cons buildConstraints, base setupConfig) (setupDim, []SetupRejected) {
	v := e.variant
	rejected := []SetupRejected{}
	var baseState []int32
	if gameName == "classic" {
		baseState, _ = m.decode(base.classicTalents)
	} else if base.policy.Name == "override" {
		baseState, _ = m.fromMap(base.policy.Override)
	}
	source := e.TalentSource
	if gameName == "forever" {
		_, source, _, _ = b.talentPolicy(e)
	}
	dim := setupDim{Name: "talents", Options: []setupOption{{Label: "preset", Note: "normalized preset: " + source, apply: func(*setupConfig) {}, talents: baseState}}}
	seen := map[string]bool{}
	if baseState != nil {
		seen[m.key(baseState)] = true
	}
	add := func(label, note string, s []int32, err error) {
		switch {
		case err != nil:
			rejected = append(rejected, SetupRejected{"talents", label, err.Error()})
		case seen[m.key(s)]:
		case m.validateEngine(s, v, base.race, b.mode) != nil:
			rejected = append(rejected, SetupRejected{"talents", label, "illegal build: " + m.validateEngine(s, v, base.race, b.mode).Error()})
		case !cons.satisfied(m, s):
			rejected = append(rejected, SetupRejected{"talents", label, fmt.Sprintf("loses the spec identity (%d+ points in %s, required/forbidden talents)", cons.FocusMin, cons.Focus)})
		default:
			seen[m.key(s)] = true
			dim.Options = append(dim.Options, b.talentOption(gameName, m, label+" ("+m.treeSummary(s)+")", note, s))
		}
	}
	builds := b.builds
	if gameName == "classic" {
		builds = b.classic
	}
	for _, build := range builds {
		if build.Spec == v.ID && (gameName == "classic" || build.RulesetID == "" || build.RulesetID == foreverdata.RulesetID) {
			s, err := m.decode(build.Talents)
			add("build "+build.Phase+" "+build.Source, build.Name, s, err)
		}
	}
	if snapshot, err := uiPresets(); err == nil {
		spec := snapshot.Specs[v.UIDir]
		names := make([]string, 0, len(spec.Exports))
		for k := range spec.Exports {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, exp := range names {
			for _, r := range collectRefs(spec.Exports[exp]) {
				if r.talents() == "" {
					continue
				}
				if gameName == "classic" {
					s, err := m.decode(r.talents())
					add("UI preset "+r.Name, r.talents(), s, err)
				} else {
					repaired, _ := b.d.repairClassicTalents(v.Class, r.talents(), true)
					s, err := m.fromMap(repaired)
					add("repair of UI preset "+r.Name, "Classic "+r.talents(), s, err)
				}
			}
		}
	}
	return dim, rejected
}

func (b *bench) talentOption(gameName string, m *talentModel, label, note string, s []int32) setupOption {
	return setupOption{Label: label, Note: note, talents: s, apply: func(c *setupConfig) {
		if gameName == "classic" {
			c.classicTalents = m.encode(s)
		} else {
			c.policy = TalentPolicy{Name: "override", Override: m.toMap(s)}
		}
	}}
}

func (b *bench) withAutoRotation(mode string) *bench {
	n := b.norm
	n.AutoRotation = mode
	return &bench{d: b.d, norm: n, mode: b.mode, builds: b.builds, classic: b.classic, parallel: b.parallel, fullProv: b.fullProv, top: b.top}
}

func autoRotationDim(current string) setupDim {
	dim := setupDim{Name: "auto_rotation"}
	for _, mode := range []string{current, "conservative", "ui", "off"} {
		if mode := mode; len(dim.Options) == 0 || mode != current {
			note := autoRotationDoc[mode]
			if mode == current && len(dim.Options) == 0 {
				note = "normalized preset; " + note
			}
			dim.Options = append(dim.Options, setupOption{Label: mode, Note: note, apply: func(c *setupConfig) { c.autoRotation = mode }})
		}
	}
	return dim
}

// ---------------------------------------------------------------- driver

var setupDimensionGroups = []string{"talents", "race", "gear", "consumes", "apl"}

type SetupDimensionReport struct {
	Name    string   `json:"dimension"`
	Preset  string   `json:"preset"`
	Chosen  string   `json:"chosen"`
	Changed bool     `json:"changed"`
	Note    string   `json:"chosen_note,omitempty"`
	Options []string `json:"options"`
}

type SetupResult struct {
	Spec              string                 `json:"spec"`
	Game              string                 `json:"game"`
	Phase             string                 `json:"phase"`
	Targets           int                    `json:"targets"`
	DurationS         float64                `json:"duration_s"`
	Seed              int64                  `json:"seed"`
	Iterations        []int32                `json:"iterations"`
	Setup             []SetupDimensionReport `json:"setup"`
	Talents           string                 `json:"talents,omitempty"`
	Consumes          json.RawMessage        `json:"consumes"`
	DPS               float64                `json:"dps"`
	StdErr            float64                `json:"stderr"`
	CI95              [2]float64             `json:"ci95"`
	AutoRotationAdded []string               `json:"forever_auto_rotation_added,omitempty"`
	PresetDPS         float64                `json:"preset_dps"`
	PresetStdErr      float64                `json:"preset_stderr"`
	GainDPS           float64                `json:"gain_vs_preset_dps"`
	GainStdErr        float64                `json:"gain_vs_preset_stderr"`
	GainPct           float64                `json:"gain_vs_preset_pct"`
	Path              []SetupStep            `json:"path"`
	Interactions      []SetupInteraction     `json:"interactions"`
	InteractionsTotal int                    `json:"interactions_measured"`
	PairProbes        int                    `json:"pair_probes"`
	Rejected          []SetupRejected        `json:"rejected_options"`
	Exclusivity       []string               `json:"exclusivity_rules"`
	NotSearched       []string               `json:"not_searched"`
	Reoptimized       *OptimizeResult        `json:"talent_reoptimization,omitempty"`
	NormalizationHash string                 `json:"normalization_hash"`
	Sims              int64                  `json:"sims"`
	WallSeconds       float64                `json:"wall_seconds"`
	BudgetExhausted   bool                   `json:"budget_exhausted"`
}

type setupOptions struct {
	game       string
	phase      Phase
	enc        EncounterSpec
	base       int32
	budget     time.Duration
	parallel   int
	maxProbes  int
	pairCap    int
	dimensions map[string]bool
	reoptimize bool
	maxReport  int
}

var setupExclusivity = []string{
	"a setup holds exactly one option per dimension, and every consumable dimension is one exclusive proto.Consumes enum field (flask, food, main_hand_imbue, off_hand_imbue, default_potion, default_conjured, zanza_buff): two flasks, two foods or two imbues on one weapon cannot be expressed",
	"main_hand_imbue: the sim applies the Windfury Totem as the main-hand imbue (WeaponImbue_Windfury), so it is exclusive with sharpening stones, weightstones, oils, rogue poisons and shaman weapon imbues, as in game; it is never an off-hand option",
	"sharpening stones need an edged weapon, weightstones a blunt weapon and off-hand imbues an off-hand weapon: such combinations with the gear option at hand are rejected when they come up (the normalized preset's own values are taken as they are)",
	"gear: a two-hander always clears the off hand; dual-wield sets are only offered to classes that can dual wield",
	"talents: every build passes the engine validation and keeps the spec identity (focus tree points, required/forbidden talents)",
}

var setupNotSearched = []string{
	"individual gear slots, enchants and random suffixes: only whole gear sets (and the two-hander swap) are compared; per-slot search needs an item-level optimizer",
	"world buffs, raid/party buffs and debuffs: fixed by the normalization (-world-buffs) for every spec",
	"stacking elixirs and juju (agility, strength, attack power, spell/fire/frost/shadow power, mana regen, armor, health): the normalized set already holds the best rank of every slot; Dragonbreath Chili, alcohol, explosives, sappers, pet consumables and hit consumables keep their normalized value",
	"spec options (UI DefaultOptions such as stance, pet, seals), profession, distance, reaction time: UI defaults / normalization",
	"encounter: one encounter (-targets, -duration); the best setup can differ per encounter type",
	"APL contents: only whole APL presets and the Forever auto-rotation policy are compared, actions are not reordered",
}

func (b *bench) optimizeSetup(specID string, opts setupOptions, stderr io.Writer) (SetupResult, error) {
	start := time.Now()
	e, err := matrixEntry(specID, opts.phase)
	if err != nil {
		return SetupResult{}, err
	}
	e = b.resolveTalents(e.withGearFill(b.norm.GearFill))
	if e.Status != "ok" {
		return SetupResult{}, fmt.Errorf("%s has no simmable gear for %s: %s", specID, opts.phase, e.Reason)
	}
	v := e.variant
	var m *talentModel
	if opts.game == "forever" {
		m = foreverTalentModel(b.d, v.Class, b.mode)
	} else if m, err = classicTalentModel(v.Class); err != nil {
		return SetupResult{}, err
	}
	cons, err := constraintsFor(m, v)
	if err != nil {
		return SetupResult{}, err
	}
	want := func(group string) bool { return len(opts.dimensions) == 0 || opts.dimensions[group] }

	// Normalized preset setup.
	raceDim, race := b.raceDim(e, opts.game, opts.enc)
	base := setupConfig{entry: e, race: race.race, consumes: consumesFor(v, b.norm.ConsumesTier), classicTalents: e.TalentsString, autoRotation: b.norm.AutoRotation}
	if opts.game == "forever" {
		if base.policy, _, _, err = b.talentPolicy(e); err != nil {
			return SetupResult{}, err
		}
	} else if e.TalentsString == "" {
		return SetupResult{}, fmt.Errorf("%s has no Classic talents (no UI preset and no optimized Classic build; run optimize-talents -game classic)", specID)
	}

	res := SetupResult{Spec: specID, Game: opts.game, Phase: opts.phase.String(), Targets: opts.enc.Targets, DurationS: opts.enc.Duration, Seed: b.norm.Seed,
		Rejected: []SetupRejected{}, Exclusivity: setupExclusivity, NotSearched: append([]string{}, setupNotSearched...), NormalizationHash: normalizationHash(b.norm)}
	dims := []setupDim{}
	talentsAt := -1
	if want("talents") {
		dim, rejected := b.talentDim(e, opts.game, m, cons, base)
		talentsAt = len(dims)
		dims, res.Rejected = append(dims, dim), append(res.Rejected, rejected...)
	}
	if want("race") {
		dims = append(dims, raceDim)
	}
	if want("gear") {
		dim, rejected, err := gearDim(e, opts.phase)
		if err != nil {
			return SetupResult{}, err
		}
		dims, res.Rejected = append(dims, dim), append(res.Rejected, rejected...)
	}
	switch {
	case !want("consumes"):
	case b.norm.ConsumesTier == "none":
		res.NotSearched = append(res.NotSearched, "consumables: the normalization's consumables tier is none")
	default:
		cd, rejected, skipped := consumeDims(v, b.norm.ConsumesTier)
		dims, res.Rejected, res.NotSearched = append(dims, cd...), append(res.Rejected, rejected...), append(res.NotSearched, skipped...)
	}
	if want("apl") {
		dim, err := aplDim(e)
		if err != nil {
			return SetupResult{}, err
		}
		dims = append(dims, dim)
		if opts.game == "forever" {
			dims = append(dims, autoRotationDim(b.norm.AutoRotation))
		}
	}
	for _, g := range setupDimensionGroups {
		if !want(g) {
			res.NotSearched = append(res.NotSearched, g+": excluded by -dimensions")
		}
	}

	benches := map[string]*bench{b.norm.AutoRotation: b}
	for _, mode := range []string{"conservative", "ui", "off"} {
		if benches[mode] == nil {
			benches[mode] = b.withAutoRotation(mode)
		}
	}
	config := func(s []int32) setupConfig {
		c := base
		c.consumes = googleProto.Clone(base.consumes).(*proto.Consumes)
		for d, k := range s {
			dims[d].Options[k].apply(&c)
		}
		return c
	}
	var mu sync.Mutex
	illegal := map[string]bool{}
	// problem rejects weapon imbues that do not fit the gear option's weapons.
	problem := func(c setupConfig) string {
		mh, oh := gearWeapons(c.entry.gearRaw())
		for _, x := range []struct {
			dim          string
			value, preset proto.WeaponImbue
			w            weaponInfo
			offHand      bool
		}{{"main_hand_imbue", c.consumes.MainHandImbue, base.consumes.MainHandImbue, mh, false}, {"off_hand_imbue", c.consumes.OffHandImbue, base.consumes.OffHandImbue, oh, true}} {
			if why := imbueProblem(x.value, x.w, x.offHand); why != "" && x.value != x.preset {
				mu.Lock()
				if key := x.dim + x.value.String() + why; !illegal[key] {
					illegal[key] = true
					res.Rejected = append(res.Rejected, SetupRejected{x.dim, x.value.String(), "with gear " + c.entry.GearLabel + ": " + why})
				}
				mu.Unlock()
				return "illegal setup: " + x.value.String() + ": " + why
			}
		}
		return ""
	}
	eval := func(s []int32, iterations int32) (evalResult, int64) {
		c := config(s)
		if why := problem(c); why != "" {
			return evalResult{Err: why}, 0
		}
		return benches[c.autoRotation].evalBuild(opts.game, c.entry, c.race, c.consumes, opts.enc, iterations, c.classicTalents, c.policy)
	}
	deadline := time.Time{}
	if opts.budget > 0 {
		deadline = start.Add(opts.budget)
	}
	ss := newSetupSearch(dims, opts.base, opts.parallel, deadline, opts.maxProbes, eval)
	res.Iterations = ss.eng.levels
	preset := make([]int32, len(dims))
	s := ss.run(preset)
	sims := int64(0)

	// Step 4: talent optimizer inside the chosen setup.
	changed := false
	for d := range s {
		changed = changed || (s[d] != 0 && d != talentsAt)
	}
	if opts.reoptimize && talentsAt >= 0 && changed && !ss.eng.overBudget() {
		c := config(s)
		remaining := time.Duration(0)
		if opts.budget > 0 {
			remaining = time.Until(deadline)
		}
		fmt.Fprintf(stderr, "optimize-setup %s: talent optimizer inside the chosen setup (budget %.0fs) ...\n", specID, remaining.Seconds())
		r, err := benches[c.autoRotation].optimize(specID, optimizeOptions{game: opts.game, phase: opts.phase, enc: opts.enc, base: opts.base, budget: remaining,
			parallel: opts.parallel, pairCap: opts.pairCap, setup: &setupOverride{entry: c.entry, race: c.race, consumes: c.consumes, start: dims[talentsAt].Options[s[talentsAt]].talents}})
		if err != nil {
			return SetupResult{}, err
		}
		res.Reoptimized, sims = &r, sims+r.Sims
		if state, err := m.decode(r.Talents); r.Error == "" && err == nil && cons.satisfied(m, state) {
			k := int32(len(dims[talentsAt].Options))
			dims[talentsAt].Options = append(dims[talentsAt].Options, b.talentOption(opts.game, m, "reoptimized in the chosen setup ("+m.treeSummary(state)+")", r.Talents, state))
			ss.dims = dims
			if dec, ok := ss.eng.selectBest(s, []optMove{{ss.with(s, talentsAt, k), ss.change(s, talentsAt, k), 1, -1}}, selectNormal); ok {
				ss.path = append(ss.path, ss.step("reoptimize-talents", 5, dec))
				s = ss.ascend(dec.move.state, 5)
			}
		}
	} else if opts.reoptimize && !changed {
		res.NotSearched = append(res.NotSearched, "-reoptimize-talents: the setup did not change, the preset talents were optimized in it already")
	}

	top := ss.eng.levels[len(ss.eng.levels)-1]
	final := ss.eng.evaluate([][]int32{preset, s}, top)
	if final[0].Err != "" || final[1].Err != "" {
		return SetupResult{}, fmt.Errorf("final evaluation failed: %s %s", final[0].Err, final[1].Err)
	}
	c := config(s)
	for d, dim := range dims {
		rep := SetupDimensionReport{Name: dim.Name, Preset: dim.Options[0].Label, Chosen: dim.Options[s[d]].Label, Changed: s[d] != 0, Note: dim.Options[s[d]].Note, Options: []string{}}
		for _, o := range dim.Options {
			rep.Options = append(rep.Options, o.Label)
		}
		res.Setup = append(res.Setup, rep)
	}
	if talentsAt >= 0 && dims[talentsAt].Options[s[talentsAt]].talents != nil {
		res.Talents = m.encode(dims[talentsAt].Options[s[talentsAt]].talents)
	}
	consumesJSON, _ := protojson.Marshal(c.consumes)
	res.Consumes = normalizeJSON(consumesJSON)
	res.DPS, res.StdErr = round2(final[1].DPS), round2(final[1].StdErr)
	res.CI95 = [2]float64{round2(final[1].DPS - 1.96*final[1].StdErr), round2(final[1].DPS + 1.96*final[1].StdErr)}
	res.AutoRotationAdded = final[1].Added
	res.PresetDPS, res.PresetStdErr = round2(final[0].DPS), round2(final[0].StdErr)
	res.GainDPS = round2(final[1].DPS - final[0].DPS)
	res.GainStdErr = round2(math.Sqrt(final[0].StdErr*final[0].StdErr + final[1].StdErr*final[1].StdErr))
	if final[0].DPS > 0 {
		res.GainPct = round2((final[1].DPS - final[0].DPS) / final[0].DPS * 100)
	}
	res.Path = append([]SetupStep{}, ss.path...)
	all := ss.interactions()
	res.InteractionsTotal, res.PairProbes = len(all), ss.probes
	res.Interactions = all[:min(opts.maxReport, len(all))]
	res.Sims, res.BudgetExhausted = sims+ss.eng.sims, ss.eng.budgetHit || (res.Reoptimized != nil && res.Reoptimized.BudgetExhausted)
	res.WallSeconds = math.Round(time.Since(start).Seconds()*10) / 10
	return res, nil
}

func cmdOptimizeSetup(args []string, stderr io.Writer) (interface{}, error) {
	fs, c, budget, pairMoves := optimizerFlags("optimize-setup", stderr)
	specID := fs.String("spec", "", "spec id from `list`")
	gameName := fs.String("game", "forever", "forever or classic")
	dimensions := fs.String("dimensions", "", "comma-separated dimension groups to search (default all): "+strings.Join(setupDimensionGroups, ", ")+" (consumes = flask, food, weapon imbues, potion, conjured, zanza; apl includes the Forever auto-rotation policy)")
	reoptimize := fs.Bool("reoptimize-talents", false, "rerun the talent optimizer inside the chosen setup when it differs from the preset (uses the remaining budget)")
	probes := fs.Int("pair-probes", 160, "maximum pair probes (2x2 factorials between the top 2 options of two dimensions) per round; 0 = coordinate ascent only")
	report := fs.Int("interactions", 40, "interactions to report (strongest first)")
	if err := c.parse(fs, args); err != nil {
		return nil, err
	}
	applyOptimizerDefaults(c, runtime.NumCPU())
	if *specID == "" {
		return nil, fmt.Errorf("-spec is required")
	}
	if *gameName != "forever" && *gameName != "classic" {
		return nil, fmt.Errorf("-game must be forever or classic")
	}
	groups := map[string]bool{}
	for _, g := range strings.Split(*dimensions, ",") {
		if g = strings.TrimSpace(g); g != "" {
			known := false
			for _, k := range setupDimensionGroups {
				known = known || k == g
			}
			if !known {
				return nil, fmt.Errorf("-dimensions: unknown group %q (use %s)", g, strings.Join(setupDimensionGroups, ", "))
			}
			groups[g] = true
		}
	}
	if *reoptimize && len(groups) > 0 && !groups["talents"] {
		return nil, fmt.Errorf("-reoptimize-talents needs the talents dimension")
	}
	b, root, err := c.newBench()
	if err != nil {
		return nil, err
	}
	res, err := b.optimizeSetup(*specID, setupOptions{game: *gameName, phase: c.phaseValue, base: int32(c.iterations),
		enc:    EncounterSpec{Name: "custom", Weight: 1, Targets: c.targets, Duration: c.duration},
		budget: time.Duration(*budget * float64(time.Second)), parallel: c.parallel, maxProbes: *probes, pairCap: *pairMoves,
		dimensions: groups, reoptimize: *reoptimize, maxReport: max(0, *report)}, stderr)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"command":       "optimize-setup",
		"result":        res,
		"normalization": b.norm,
		"provenance":    provenanceBlock(root, nil),
		"notes": []string{
			"optimize-setup answers \"what is the best combination for this spec\"; rank, delta and meta keep the normalized preset setup, so the chosen setup does not change rankings.",
			"Every option is simmed inside the whole setup (common random numbers, seed " + fmt.Sprint(b.norm.Seed) + "); a change is adopted only above max(0.05% DPS, 0.5 x combined SE) at the highest iteration level needed.",
			"interactions: synergy_dps = f(a+b) - f(a) - f(b) + f(current setup at probe time). Positive: the options are worth more together; negative: they overlap or conflict. synergy_stderr treats the four sims as independent (conservative under common random numbers).",
			"gain_vs_preset compares the chosen setup with the normalized preset setup at the highest iteration level.",
		},
	}, nil
}
